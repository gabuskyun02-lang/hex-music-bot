// Live synced-lyrics sessions: one scrolling [mm:ss] window per guild,
// driven by the player position with a binary search over parsed LRC lines.
// Unlike PrimeMusic's linear time-to-line mapping, real timestamps keep the
// highlight accurate even when verses cluster early.
package bot

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/lyrics"
)

const (
	syncTickInterval = 5 * time.Second
	syncWindow       = 3 // lyric lines shown above and below the active one
	syncAccentColor  = 0x5865F2
)

// LyricSyncSession is one live lyrics message plus its update pump.
type LyricSyncSession struct {
	guildID   snowflake.ID
	channelID snowflake.ID
	messageID snowflake.ID
	lines     []lyrics.Line
	// trackTitle pins the session to the track it was opened for; a player
	// reporting any other title means playback moved on — self-destruct.
	trackTitle string

	ctx     context.Context
	cancel  func()
	done    chan struct{}
	lastIdx int
}

// LyricSyncManager tracks at most one active session per guild, mirroring
// CardManager's lifecycle (lock around map ops only).
type LyricSyncManager struct {
	b        *Bot
	mu       sync.Mutex
	sessions map[snowflake.ID]*LyricSyncSession
}

func NewLyricSyncManager(b *Bot) *LyricSyncManager {
	return &LyricSyncManager{b: b, sessions: make(map[snowflake.ID]*LyricSyncSession)}
}

// StartLiveLyrics posts the initial sync-window message and launches the
// session pump. Caller has already resolved synced lines (len > 0).
func (b *Bot) StartLiveLyrics(guildID, channelID snowflake.ID, trackTitle string, lines []lyrics.Line) error {
	idx := 0
	if p := b.Lavalink.ExistingPlayer(guildID); p != nil && p.Track != nil {
		idx = currentLineIdx(lines, p.Position())
	}
	msg, err := b.Client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: syncComps(lines, idx, guildID),
	})
	if err != nil {
		return err
	}
	b.Sync.Start(guildID, channelID, msg.ID, trackTitle, lines, idx)
	return nil
}

// Start launches a session, superseding any active one for the guild.
func (m *LyricSyncManager) Start(guildID, channelID, messageID snowflake.ID, trackTitle string, lines []lyrics.Line, idx int) {
	m.Stop(guildID)

	ctx, cancel := context.WithCancel(context.Background())
	s := &LyricSyncSession{
		guildID:    guildID,
		channelID:  channelID,
		messageID:  messageID,
		lines:      lines,
		trackTitle: trackTitle,
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		lastIdx:    idx,
	}
	m.mu.Lock()
	m.sessions[guildID] = s
	m.mu.Unlock()
	go s.run(m)
}

// Stop cancels the guild's session and deletes its message (best-effort).
// Idempotent: stopping an unknown guild is a no-op.
func (m *LyricSyncManager) Stop(guildID snowflake.ID) {
	m.mu.Lock()
	s, ok := m.sessions[guildID]
	if ok {
		delete(m.sessions, guildID)
	}
	m.mu.Unlock()
	if !ok {
		return
	}
	s.cancel()
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
		slog.Debug("lyrics sync shutdown timed out", slog.String("guild", guildID.String()))
	}
	if m.b != nil && m.b.Client != nil {
		if err := m.b.Client.Rest.DeleteMessage(s.channelID, s.messageID); err != nil {
			slog.Debug("lyrics sync delete failed", slog.Any("err", err))
		}
	}
}

// remove drops the session from the map only if it is still the current one,
// so a self-destructing old session can never evict its replacement.
func (m *LyricSyncManager) remove(guildID snowflake.ID, s *LyricSyncSession) {
	m.mu.Lock()
	if m.sessions[guildID] == s {
		delete(m.sessions, guildID)
	}
	m.mu.Unlock()
}

// run ticks every 5s: follow player position, repaint the window on line
// change. Self-destructs when playback disappears or moves to another track;
// the winner-stays map check in remove keeps that race-safe against Start.
func (s *LyricSyncSession) run(m *LyricSyncManager) {
	defer close(s.done)
	ticker := time.NewTicker(syncTickInterval)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
		}
		if s.tick(m) {
			continue
		}
		m.remove(s.guildID, s)
		if m.b != nil && m.b.Client != nil {
			if err := m.b.Client.Rest.DeleteMessage(s.channelID, s.messageID); err != nil {
				slog.Debug("lyrics sync delete failed", slog.Any("err", err))
			}
		}
		return
	}
}

// tick advances the highlight once. Returns false when the session is stale
// (player gone or switched tracks) and should self-destruct.
func (s *LyricSyncSession) tick(m *LyricSyncManager) bool {
	p := m.b.Lavalink.ExistingPlayer(s.guildID)
	if p == nil || p.Track == nil || p.Track.Info.Title != s.trackTitle {
		return false
	}
	idx := currentLineIdx(s.lines, p.Position())
	if idx == s.lastIdx {
		return true
	}
	if _, err := m.b.Client.Rest.UpdateMessage(s.channelID, s.messageID,
		discord.NewMessageUpdateV2(syncComps(s.lines, idx, s.guildID))); err != nil {
		slog.Debug("lyrics sync edit failed", slog.String("guild", s.guildID.String()), slog.Any("err", err))
		return true
	}
	s.lastIdx = idx
	return true
}

// handleSyncStopButton serves hexsync:stop clicks: ack the interaction, then
// let the manager tear the session down and delete the message via REST.
func (b *Bot) handleSyncStopButton(event *events.ComponentInteractionCreate, customID string) {
	guildID, err := snowflake.Parse(strings.TrimPrefix(customID, "hexsync:stop:"))
	if err != nil {
		_ = event.DeferUpdateMessage()
		return
	}
	_ = event.DeferUpdateMessage()
	b.Sync.Stop(guildID)
}

// currentLineIdx binary-searches the last line whose timestamp is <= pos;
// clamps to 0 before the first line.
func currentLineIdx(lines []lyrics.Line, pos lavalink.Duration) int {
	idx := sort.Search(len(lines), func(i int) bool {
		return lines[i].At.Milliseconds() > pos.Milliseconds()
	}) - 1
	if idx < 0 {
		return 0
	}
	return idx
}

// renderSyncWindow formats ±syncWindow lines around idx, bolding the active
// one. Timestamps stay visible so users see the sync is real.
func renderSyncWindow(lines []lyrics.Line, idx int) string {
	lo := max(idx-syncWindow, 0)
	hi := min(idx+syncWindow, len(lines)-1)
	var sb strings.Builder
	for i := lo; i <= hi; i++ {
		text := lines[i].Text
		if text == "" {
			text = "♪"
		}
		if i == idx {
			fmt.Fprintf(&sb, "**> `%s` %s**\n", formatStamp(lines[i].At), text)
		} else {
			fmt.Fprintf(&sb, "`%s` %s\n", formatStamp(lines[i].At), text)
		}
	}
	return strings.TrimSuffix(sb.String(), "\n")
}

func formatStamp(d time.Duration) string {
	return fmt.Sprintf("%d:%02d", int(d.Minutes()), int(d.Seconds())%60)
}

// syncComps renders the V2 layout: one accent container holding the lyric
// window and the stop button.
func syncComps(lines []lyrics.Line, idx int, guildID snowflake.ID) []discord.LayoutComponent {
	c := discord.ContainerComponent{AccentColor: syncAccentColor}
	c.Components = append(c.Components,
		discord.TextDisplayComponent{Content: renderSyncWindow(lines, idx)},
		discord.ActionRowComponent{Components: []discord.InteractiveComponent{
			discord.NewDangerButton("⏹", "hexsync:stop:"+guildID.String()),
		}},
	)
	return []discord.LayoutComponent{c}
}
