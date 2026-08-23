package bot

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/player"
)

// Shared playback actions. Slash commands AND card buttons call these —
// one code path per behavior (plan non-negotiable #2). Actions touch
// Lavalink and player state only; callers translate results into replies.

// SetPaused drives pause state to an explicit target.
func (b *Bot) SetPaused(guildID snowflake.ID, target bool) (bool, bool) {
	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil {
		return false, false
	}
	if p.Paused == target {
		return target, true // no-op, already there
	}
	if err := p.Update(context.TODO(), disgolink.WithPaused(target)); err != nil {
		slog.Error("set paused failed", slog.Any("err", err))
		return !target, true
	}
	return target, true
}

// TogglePause flips pause state for the card's ⏯ button.
func (b *Bot) TogglePause(guildID snowflake.ID) (bool, bool) {
	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil {
		return false, false
	}
	return b.SetPaused(guildID, !p.Paused)
}

// SkipNext advances to the next queued track. Returns (next, stopped);
// stopped means the queue drained and playback was nulled.
func (b *Bot) SkipNext(guildID snowflake.ID) (*lavalink.Track, bool) {
	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil {
		return nil, false
	}
	state := b.Player.Get(guildID)
	next, ok := state.Next()
	if !ok {
		if err := p.Update(context.TODO(), disgolink.WithNullTrack()); err != nil {
			slog.Error("skip null-track failed", slog.Any("err", err))
		}
		b.onQueueDrained(guildID)
		return nil, true
	}
	if p.Track != nil {
		state.PushHistory(*p.Track)
	}
	if err := p.Update(context.TODO(), disgolink.WithTrack(next)); err != nil {
		slog.Error("skip failed", slog.Any("err", err))
	}
	return &next, false
}

// onQueueDrained finalizes the card, arms the idle disconnect timer, and
// attempts autoplay. Shared drain path for SkipNext and OnTrackEnd.
func (b *Bot) onQueueDrained(guildID snowflake.ID) {
	b.Cards.Finalize(guildID, "queue ended")
	b.voice.ScheduleIdleDisconnect(guildID)
	b.TryAutoplay(guildID)
}

// ReplayPrevious restarts the most recently finished track.
// Returns (track, noHistory).
func (b *Bot) ReplayPrevious(guildID snowflake.ID) (*lavalink.Track, bool) {
	state := b.Player.Get(guildID)
	prev, ok := state.PopHistory()
	if !ok {
		return nil, true
	}
	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil {
		state.PushHistory(prev) // restore; nothing is playing
		return nil, true
	}
	if p.Track != nil {
		state.PushHistory(*p.Track)
	}
	if err := p.Update(context.TODO(), disgolink.WithTrack(prev)); err != nil {
		slog.Error("previous failed", slog.Any("err", err))
	}
	return &prev, false
}

// CycleLoop advances off -> track -> queue -> off.
func (b *Bot) CycleLoop(guildID snowflake.ID) player.LoopMode {
	state := b.Player.Get(guildID)
	next := (state.LoopMode() + 1) % 3
	state.SetLoopMode(next)
	return next
}

// ShuffleQueue randomizes queue order.
func (b *Bot) ShuffleQueue(guildID snowflake.ID) {
	b.Player.Get(guildID).Shuffle()
}

// Halt stops playback, clears the queue, and locks the card.
func (b *Bot) Halt(guildID snowflake.ID) {
	b.Player.Get(guildID).ClearQueue()
	if p := b.Lavalink.ExistingPlayer(guildID); p != nil {
		if err := p.Update(context.TODO(), disgolink.WithNullTrack()); err != nil {
			slog.Error("halt null-track failed", slog.Any("err", err))
		}
	}
	b.Cards.Finalize(guildID, "stopped")
}

// ForgetGuild releases all per-guild bookkeeping when the bot leaves voice.
func (b *Bot) ForgetGuild(guildID snowflake.ID) {
	b.Votes.Cancel(guildID, "skip")
	b.failover.Clear(guildID)
}

// StepVolume adjusts volume by delta, clamped to Lavalink's 0-1000 range.
// Returns the applied level; ok=false when no player exists.
func (b *Bot) StepVolume(guildID snowflake.ID, delta int) (int, bool) {
	p := b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil {
		return 0, false
	}
	level := clampInt(p.Volume+delta, 0, 1000)
	if err := p.Update(context.TODO(), disgolink.WithVolume(level)); err != nil {
		slog.Error("volume step failed", slog.Any("err", err))
		return p.Volume, true
	}
	return level, true
}

func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
