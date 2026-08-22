package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/ui"
)

// FailoverManager tracks which track identifiers already failed per guild.
// One failover attempt per identifier; if the alternate also breaks,
// we skip instead of looping.
type FailoverManager struct {
	mu     sync.Mutex
	failed map[snowflake.ID]map[string]bool
}

// NewFailoverManager builds an empty tracker.
func NewFailoverManager() *FailoverManager {
	return &FailoverManager{failed: make(map[snowflake.ID]map[string]bool)}
}

func (f *FailoverManager) markFailed(guildID snowflake.ID, identifier string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failed[guildID] == nil {
		f.failed[guildID] = make(map[string]bool)
	}
	f.failed[guildID][identifier] = true
}

func (f *FailoverManager) hasFailed(guildID snowflake.ID, identifier string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.failed[guildID][identifier]
}

// Clear resets failure tracking for a guild (on new /play or voice join).
func (f *FailoverManager) Clear(guildID snowflake.ID) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.failed, guildID)
}

// AttemptFailover tries to resolve the same song on SoundCloud when the
// original source breaks. Returns true if a replacement was started.
func (b *Bot) AttemptFailover(guildID snowflake.ID, failed lavalink.Track) bool {
	if b.failover.hasFailed(guildID, failed.Info.Identifier) {
		return false // already tried once, don't loop
	}
	b.Metrics.Inc("hex_music_bot_failovers_attempted")
	slog.Warn("attempting failover",
		slog.String("guild", guildID.String()),
		slog.String("failed_identifier", failed.Info.Identifier),
		slog.String("title", failed.Info.Title),
	)

	node := b.Lavalink.BestNode()
	if node == nil {
		return false
	}
	b.failover.markFailed(guildID, failed.Info.Identifier)
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	var alt *lavalink.Track
	node.Rest.LoadTracksHandler(ctx, "scsearch:"+failed.Info.Title, disgolink.NewTrackLoadingResultHandler(
		func(track lavalink.Track) { alt = &track },
		func(playlist lavalink.Playlist) {},
		func(tracks []lavalink.Track) {
			if len(tracks) > 0 {
				alt = &tracks[0]
			}
		},
		func() {},
		func(err error) { slog.Debug("failover search failed", slog.Any("err", err)) },
	))

	if alt == nil || alt.Info.Identifier == failed.Info.Identifier {
		slog.Warn("no alternate source found",
			slog.String("guild", guildID.String()),
			slog.String("title", failed.Info.Title),
		)
		return false
	}

	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil || p.Track.Info.Identifier != failed.Info.Identifier {
		return false // stale event, wrong track now playing
	}
	if err := p.Update(ctx, disgolink.WithTrack(*alt)); err != nil {
		slog.Error("failover playback failed", slog.Any("err", err))
		return false
	}
	b.Metrics.Inc("hex_music_bot_failovers_succeeded")
	slog.Info("failover succeeded",
		slog.String("guild", guildID.String()),
		slog.String("original", failed.Info.Title),
		slog.String("replacement", alt.Info.Title),
	)
	b.notifyChannel(guildID, fmt.Sprintf("⚠ %s broke — switching to SoundCloud: %s",
		ui.TrackMarkdown(failed), ui.TrackMarkdown(*alt)))
	return true
}

// SkipBroken marks the track as failed and advances to the next queued one.
// Sends a channel notice so users know why playback jumped.
func (b *Bot) SkipBroken(guildID snowflake.ID, broken lavalink.Track) {
	if b.failover.hasFailed(guildID, broken.Info.Identifier) {
		return // duplicate event, already handled
	}
	b.failover.markFailed(guildID, broken.Info.Identifier)
	next, stopped := b.SkipNext(guildID)
	b.Cards.Refresh(guildID)
	if stopped {
		b.notifyChannel(guildID, fmt.Sprintf("❌ `%s` failed and the queue is empty.", broken.Info.Title))
		return
	}
	if next == nil {
		return // no player/track, nothing to skip to
	}
	b.notifyChannel(guildID, fmt.Sprintf("❌ `%s` failed — skipping to %s",
		broken.Info.Title, ui.TrackMarkdown(*next)))
}

// notifyChannel posts a message to the card's channel (if a card exists).
// Fire-and-forget; failures logged at debug level.
func (b *Bot) notifyChannel(guildID snowflake.ID, text string) {
	entry := b.Cards.Lookup(guildID)
	if entry == nil {
		return
	}
	if _, err := b.Client.Rest.CreateMessage(entry.channelID, discord.MessageCreate{
		Content: text,
	}); err != nil {
		slog.Debug("notify failed", slog.String("guild", guildID.String()), slog.Any("err", err))
	}
}
