package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/player"
	"hex-music-bot/internal/store"
)

// SaveSnapshot captures current playback state for 24/7 restart survival.
func (b *Bot) SaveSnapshot(guildID snowflake.ID) {
	p := b.Lavalink.ExistingPlayer(guildID)
	state := b.Player.Get(guildID)

	snap := &store.PlayerSnapshot{GuildID: guildID.String()}
	if chID, ok := b.voice.ChannelFor(guildID); ok {
		snap.VoiceChannelID = chID.String()
	}
	if entry := b.Cards.Lookup(guildID); entry != nil {
		snap.TextChannelID = entry.channelID.String()
	}
	if p != nil && p.Track != nil {
		snap.CurrentIdentifier = p.Track.Info.Identifier
		snap.CurrentPositionMS = int64(p.Position())
		snap.Volume = p.Volume
	}
	for _, t := range state.Snapshot() {
		snap.Queue = append(snap.Queue, t.Info.Identifier)
	}
	snap.LoopMode = state.LoopMode().String()

	if err := b.Store.SavePlayerSnapshot(context.Background(), snap); err != nil {
		slog.Debug("snapshot save failed", slog.Any("err", err))
	} else {
		slog.Debug("snapshot saved", slog.String("guild", guildID.String()))
	}
}

// RestoreSnapshots checks for saved player states on boot and resumes any
// guilds that had 24/7 mode enabled.
func (b *Bot) RestoreSnapshots() {
	snaps, err := b.Store.AllSnapshots(context.Background())
	if err != nil {
		slog.Error("snapshot load failed", slog.Any("err", err))
		return
	}
	for _, snap := range snaps {
		guildID, err := snowflake.Parse(snap.GuildID)
		if err != nil {
			continue
		}
		settings, err := b.Store.GetGuildSettings(context.Background(), snap.GuildID)
		if err != nil || !settings.Mode247 || snap.VoiceChannelID == "" {
			continue
		}

		vcID, vcErr := snowflake.Parse(snap.VoiceChannelID)
		if vcErr != nil {
			continue
		}
		if err := b.Client.UpdateVoiceState(context.TODO(), guildID, &vcID, false, false); err != nil {
			slog.Error("24/7 rejoin failed", slog.String("guild", snap.GuildID), slog.Any("err", err))
			continue
		}

		node := b.Lavalink.BestNode()
		if node == nil {
			continue
		}
		pl := b.Lavalink.Player(guildID)

		if snap.LoopMode == "track" {
			b.Player.Get(guildID).SetLoopMode(player.LoopTrack)
		} else if snap.LoopMode == "queue" {
			b.Player.Get(guildID).SetLoopMode(player.LoopQueue)
		}

		if snap.CurrentIdentifier != "" {
			go b.restorePlayback(node, pl, guildID, snap)
		} else if len(snap.Queue) > 0 {
			go func() {
				time.Sleep(2 * time.Second) // allow the voice connection to establish
				b.restoreQueue(node, guildID, snap.Queue)
				if first, ok := b.Player.Get(guildID).Next(); ok {
					if snap.Volume > 0 && snap.Volume != 100 {
						_ = pl.Update(context.TODO(), disgolink.WithTrack(first), disgolink.WithVolume(snap.Volume))
					} else {
						_ = pl.Update(context.TODO(), disgolink.WithTrack(first))
					}
				}
			}()
		}
		slog.Info("24/7 session restored", slog.String("guild", snap.GuildID))
	}
}

func (b *Bot) restorePlayback(node *disgolink.Node, pl *disgolink.Player, guildID snowflake.ID, snap *store.PlayerSnapshot) {
	time.Sleep(2 * time.Second) // allow the voice connection to establish
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var resolved *lavalink.Track
	node.Rest.LoadTracksHandler(ctx, snap.CurrentIdentifier, disgolink.NewTrackLoadingResultHandler(
		func(t lavalink.Track) { resolved = &t },
		func(p lavalink.Playlist) {},
		func(ts []lavalink.Track) {
			if len(ts) > 0 {
				resolved = &ts[0]
			}
		},
		func() {},
		func(err error) {},
	))
	if resolved == nil {
		b.restoreQueue(node, guildID, snap.Queue)
		return
	}

	if snap.Volume > 0 && snap.Volume != 100 {
		_ = pl.Update(context.TODO(), disgolink.WithTrack(*resolved), disgolink.WithVolume(snap.Volume))
	} else {
		_ = pl.Update(context.TODO(), disgolink.WithTrack(*resolved))
	}
	if snap.CurrentPositionMS > 0 {
		_ = pl.Update(context.TODO(), disgolink.WithPosition(lavalink.Duration(snap.CurrentPositionMS)))
	}
	b.restoreQueue(node, guildID, snap.Queue)
	slog.Info("24/7 restored",
		slog.String("guild", snap.GuildID),
		slog.String("track", resolved.Info.Title),
	)
}

func (b *Bot) restoreQueue(node *disgolink.Node, guildID snowflake.ID, queue []string) {
	for _, id := range queue {
		resolve := func(id string) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			node.Rest.LoadTracksHandler(ctx, id, disgolink.NewTrackLoadingResultHandler(
				func(t lavalink.Track) { b.Player.Get(guildID).Enqueue(t) },
				func(p lavalink.Playlist) {
					for _, t := range p.Tracks {
						b.Player.Get(guildID).Enqueue(t)
					}
				},
				func(ts []lavalink.Track) {
					for _, t := range ts {
						b.Player.Get(guildID).Enqueue(t)
					}
				},
				func() {},
				func(err error) {},
			))
		}
		resolve(id)
	}
}
