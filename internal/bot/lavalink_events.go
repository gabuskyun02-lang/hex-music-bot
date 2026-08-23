package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgolink/v4/disgolink"

	"hex-music-bot/internal/player"
)

// OnTrackStart cancels idle disconnect, resets skip votes for the incoming
// track, persists the 24/7 snapshot, and refreshes the card.
func (b *Bot) OnTrackStart(event *disgolink.PlayerTrackStartEvent) {
	b.Metrics.Inc("hex_music_bot_tracks_played")
	slog.Info("track started",
		slog.String("guild", event.GetGuildID().String()),
		slog.String("title", event.Track.Info.Title),
	)
	b.voice.CancelIdleDisconnect(event.GetGuildID())
	b.Votes.Cancel(event.GetGuildID(), "skip")
	go b.SaveSnapshot(event.GetGuildID())
	b.Cards.Refresh(event.GetGuildID())
}

// OnTrackEnd advances playback: honors loop modes, records history, and
// starts the next queued track. Finalizes card + arms idle timer on drain.
func (b *Bot) OnTrackEnd(event *disgolink.PlayerTrackEndEvent) {
	if !event.Reason.MayStartNext() {
		return
	}
	guildID := event.GetGuildID()
	state := b.Player.Get(guildID)

	if state.LoopMode() == player.LoopTrack {
		err := event.Player.Update(context.TODO(), disgolink.WithTrack(event.Track))
		if err == nil {
			return
		}
		slog.Error("loop-track replay failed", slog.Any("err", err))
		// fall through: record history and advance/drain normally
	}
	state.PushHistory(event.Track)
	b.Metrics.Inc("hex_music_bot_tracks_ended")
	b.recordPlay(guildID, event.Track)
	if state.LoopMode() == player.LoopQueue {
		state.Enqueue(event.Track)
	}

	next, ok := state.Next()
	if !ok {
		slog.Info("queue drained",
			slog.String("guild", guildID.String()),
			slog.String("last_track", event.Track.Info.Title),
		)
		b.onQueueDrained(guildID)
		return
	}
	if err := event.Player.Update(context.TODO(), disgolink.WithTrack(next)); err != nil {
		slog.Error("failed to start next track", slog.Any("err", err))
	}
}

// OnTrackException attempts alternate-source failover; if that fails too,
// skips to the next track with a channel notice.
func (b *Bot) OnTrackException(event *disgolink.PlayerTrackExceptionEvent) {
	guildID := event.GetGuildID()
	b.Metrics.Inc("hex_music_bot_tracks_failed")
	slog.Error("track exception",
		slog.String("guild", guildID.String()),
		slog.String("identifier", event.Track.Info.Identifier),
		slog.Any("event", event),
	)
	if b.AttemptFailover(guildID, event.Track) {
		return // replacement started; its own TrackStart will fire
	}
	b.SkipBroken(guildID, event.Track)
}

// OnTrackStuck attempts failover for stuck streams.
func (b *Bot) OnTrackStuck(event *disgolink.PlayerTrackStuckEvent) {
	guildID := event.GetGuildID()
	slog.Warn("track stuck",
		slog.String("guild", guildID.String()),
		slog.String("identifier", event.Track.Info.Identifier),
	)
	if b.AttemptFailover(guildID, event.Track) {
		return
	}
	b.SkipBroken(guildID, event.Track)
}

func (b *Bot) OnWebSocketClosed(event *disgolink.PlayerWebSocketClosedEvent) {
	slog.Warn("lavalink voice websocket closed", slog.Any("event", event))

	// 4006 session no longer valid, 4009 voice server crashed, 4015 session
	// timed out — Discord will not resume these; rejoin to rebuild the
	// voice connection. 24/7 guilds come back automatically.
	if event.Code != 4006 && event.Code != 4009 && event.Code != 4015 {
		return
	}
	guildID := event.GuildID
	chID, ok := b.voice.ChannelFor(guildID)
	if !ok {
		return // user-initiated leave or never connected
	}
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if err == nil && !settings.Mode247 {
		return // normal mode: let users rejoin manually
	}
	slog.Info("rejoining voice after fatal WS close",
		slog.String("guild", guildID.String()),
		slog.Int("code", event.Code),
	)
	go func() {
		time.Sleep(2 * time.Second)
		if err := b.Client.UpdateVoiceState(context.TODO(), guildID, &chID, false, false); err != nil {
			slog.Error("WS-close rejoin failed", slog.Any("err", err))
		}
	}()
}
