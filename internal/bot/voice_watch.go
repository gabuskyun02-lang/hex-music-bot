package bot

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// VoiceWatch tracks per-guild voice channel occupancy for auto-pause/resume
// and schedules idle auto-disconnect timers.
type VoiceWatch struct {
	mu sync.Mutex
	b  *Bot
	// guildID -> bot's current voice channel ID
	channels map[snowflake.ID]snowflake.ID
	// guildID -> non-bot listener user IDs in bot's channel
	listeners map[snowflake.ID]map[snowflake.ID]bool
	// guildID -> idle disconnect timer
	idleTimers map[snowflake.ID]*time.Timer
}

// NewVoiceWatch builds the watcher.
func NewVoiceWatch(b *Bot) *VoiceWatch {
	return &VoiceWatch{
		b:          b,
		channels:   make(map[snowflake.ID]snowflake.ID),
		listeners:  make(map[snowflake.ID]map[snowflake.ID]bool),
		idleTimers: make(map[snowflake.ID]*time.Timer),
	}
}

// OnBotJoined records the bot's voice channel; the joining user is the
// first listener.
func (w *VoiceWatch) OnBotJoined(guildID, channelID snowflake.ID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.channels[guildID] == channelID {
		return // same channel; keep existing listener tracking
	}
	w.channels[guildID] = channelID
	w.listeners[guildID] = make(map[snowflake.ID]bool)
}

// OnBotLeft clears all tracking for the guild.
func (w *VoiceWatch) OnBotLeft(guildID snowflake.ID) {
	w.mu.Lock()
	delete(w.channels, guildID)
	delete(w.listeners, guildID)
	timer := w.idleTimers[guildID]
	delete(w.idleTimers, guildID)
	w.mu.Unlock()
	if timer != nil {
		timer.Stop()
	}
}

// OnUserVoiceChange adjusts the listener count based on old/new voice states
// relative to the bot's channel. oldChannel may be nil (user wasn't in any).
func (w *VoiceWatch) OnUserVoiceChange(guildID snowflake.ID, userID snowflake.ID, oldChannelID, newChannelID *snowflake.ID) {
	if userID == w.b.Client.ApplicationID {
		return // bot itself is never a "listener"
	}
	w.mu.Lock()
	defer w.mu.Unlock()

	channelID, hasBot := w.channels[guildID]
	if !hasBot {
		return
	}
	set := w.listeners[guildID]
	if set == nil {
		set = make(map[snowflake.ID]bool)
		w.listeners[guildID] = set
	}

	wasIn := oldChannelID != nil && *oldChannelID == channelID
	isIn := newChannelID != nil && *newChannelID == channelID

	switch {
	case isIn && !wasIn:
		set[userID] = true
	case wasIn && !isIn:
		delete(set, userID)
	default:
		return // no change relative to bot's channel
	}
	count := len(set)

	// React outside the lock. A join resumes playback for the arriving
	// listener; a leave only matters when the channel emptied.
	go func() {
		if isIn && !wasIn {
			w.autoResume(guildID)
			return
		}
		if count == 0 {
			w.autoPause(guildID)
		}
	}()
}

func (w *VoiceWatch) autoPause(guildID snowflake.ID) {
	if !w.b.Cfg.AutoPause {
		return
	}
	paused, hadPlayback := w.b.SetPaused(guildID, true)
	if hadPlayback && paused {
		w.b.Cards.Refresh(guildID)
		slog.Info("auto-paused on empty voice channel", slog.String("guild", guildID.String()))
	}
}

func (w *VoiceWatch) autoResume(guildID snowflake.ID) {
	if !w.b.Cfg.AutoPause {
		return
	}
	p := w.b.Lavalink.ExistingPlayer(guildID)
	if p == nil || !p.Paused || p.Track == nil {
		return
	}
	paused, _ := w.b.SetPaused(guildID, false)
	if !paused {
		w.b.Cards.Refresh(guildID)
		slog.Debug("auto-resumed on listener join", slog.String("guild", guildID.String()))
	}
}

// ScheduleIdleDisconnect arms a timer that disconnects the bot after the
// configured idle timeout. Called when the queue drains.
func (w *VoiceWatch) ScheduleIdleDisconnect(guildID snowflake.ID) {
	timeout := w.b.Cfg.LeaveTimeout
	if timeout <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.idleTimers[guildID]; ok {
		t.Stop()
	}
	timer := time.AfterFunc(timeout, func() {
		w.mu.Lock()
		delete(w.idleTimers, guildID)
		w.mu.Unlock()

		if p := w.b.Lavalink.ExistingPlayer(guildID); p != nil && p.Track != nil {
			return // something started playing since the timer was armed
		}
		slog.Info("idle timeout reached, leaving voice", slog.String("guild", guildID.String()))
		w.b.notifyChannel(guildID, "💤 Idle timeout — leaving voice.")
		if err := w.b.Client.UpdateVoiceState(context.Background(), guildID, nil, false, false); err != nil {
			slog.Error("idle disconnect failed", slog.Any("err", err))
		}
	})
	w.idleTimers[guildID] = timer
}

// CancelIdleDisconnect clears a pending timer (called on track start or play).
func (w *VoiceWatch) CancelIdleDisconnect(guildID snowflake.ID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if t, ok := w.idleTimers[guildID]; ok {
		t.Stop()
		delete(w.idleTimers, guildID)
	}
}

// ChannelFor returns the bot's current voice channel for a guild.
func (w *VoiceWatch) ChannelFor(guildID snowflake.ID) (snowflake.ID, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	id, ok := w.channels[guildID]
	return id, ok
}

// ListenerIDs returns the user IDs of non-bot listeners in the bot's VC.
func (w *VoiceWatch) ListenerIDs(guildID snowflake.ID) []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	set := w.listeners[guildID]
	if len(set) == 0 {
		return nil
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id.String())
	}
	return ids
}
