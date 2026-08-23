package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/ui"
)

const cardTickInterval = 10 * time.Second

type cardEntry struct {
	channelID snowflake.ID
	messageID snowflake.ID
	cancel    context.CancelFunc
}

// CardManager owns one live player card per guild plus its refresh ticker.
type CardManager struct {
	b     *Bot
	mu    sync.Mutex
	cards map[snowflake.ID]*cardEntry
}

func NewCardManager(b *Bot) *CardManager {
	return &CardManager{b: b, cards: make(map[snowflake.ID]*cardEntry)}
}

// Create posts a fresh live card. Serialized per guild: two concurrent
// Creates would otherwise race past the cancel window and orphan a ticker
// on a dead message.
func (m *CardManager) Create(guildID, channelID snowflake.ID) {
	m.mu.Lock()
	if old, ok := m.cards[guildID]; ok {
		old.cancel()
		delete(m.cards, guildID)
	}
	// Reserve the slot so concurrent Create/Refresh see no card until the
	// new message exists.
	reserve := &cardEntry{}
	m.cards[guildID] = reserve
	m.mu.Unlock()

	msg, err := m.b.Client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: m.b.renderCard(guildID),
	})
	m.mu.Lock()
	if m.cards[guildID] != reserve { // superseded while creating
		m.mu.Unlock()
		if err == nil {
			_ = m.b.Client.Rest.DeleteMessage(channelID, msg.ID)
		}
		return
	}
	m.mu.Unlock()
	if err != nil {
		slog.Error("card create failed", slog.String("guild", guildID.String()), slog.Any("err", err))
		m.mu.Lock()
		delete(m.cards, guildID)
		m.mu.Unlock()
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry := &cardEntry{channelID: channelID, messageID: msg.ID, cancel: cancel}
	m.mu.Lock()
	if m.cards[guildID] != reserve { // superseded while creating
		cancel()
		m.mu.Unlock()
		_ = m.b.Client.Rest.DeleteMessage(channelID, msg.ID)
		return
	}
	m.cards[guildID] = entry
	m.mu.Unlock()
	go m.ticker(ctx, guildID, entry)
}

func (m *CardManager) ticker(ctx context.Context, guildID snowflake.ID, entry *cardEntry) {
	t := time.NewTicker(cardTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			m.edit(guildID, entry, m.b.renderCard(guildID))
		}
	}
}

func (m *CardManager) Refresh(guildID snowflake.ID) {
	m.mu.Lock()
	entry, ok := m.cards[guildID]
	m.mu.Unlock()
	if ok && entry.messageID != 0 { // zero = creation in progress
		m.edit(guildID, entry, m.b.renderCard(guildID))
	}
}

func (m *CardManager) Finalize(guildID snowflake.ID, reason string) {
	m.mu.Lock()
	entry, ok := m.removeLocked(guildID)
	m.mu.Unlock()
	if !ok || entry.messageID == 0 { // zero = creation in progress
		return
	}
	if _, err := m.b.Client.Rest.UpdateMessage(entry.channelID, entry.messageID,
		discord.NewMessageUpdateV2(renderCardLocked(reason))); err != nil {
		slog.Debug("card finalize failed", slog.Any("err", err))
	}
}

func (m *CardManager) Drop(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(guildID)
}

func (m *CardManager) removeLocked(guildID snowflake.ID) (*cardEntry, bool) {
	entry, ok := m.cards[guildID]
	if ok {
		entry.cancel()
		delete(m.cards, guildID)
	}
	return entry, ok
}

func (m *CardManager) Lookup(guildID snowflake.ID) *cardEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cards[guildID]
	if !ok || entry.messageID == 0 { // zero = creation in progress
		return nil
	}
	return entry
}

func (m *CardManager) edit(guildID snowflake.ID, entry *cardEntry, comps []discord.LayoutComponent) {
	m.b.Metrics.Inc("hex_music_bot_card_edits")
	if _, err := m.b.Client.Rest.UpdateMessage(entry.channelID, entry.messageID,
		discord.NewMessageUpdateV2(comps)); err != nil {
		slog.Debug("card edit failed", slog.String("guild", guildID.String()), slog.Any("err", err))
	}
}

// renderCard builds the live Components V2 card.
func (b *Bot) renderCard(guildID snowflake.ID) []discord.LayoutComponent {
	container := discord.ContainerComponent{AccentColor: ui.SourceBadgeFor("").Color}
	state := b.Player.Get(guildID)
	p := b.Lavalink.ExistingPlayer(guildID)

	if p == nil || p.Track == nil {
		container.Components = append(container.Components,
			discord.TextDisplayComponent{
				Content: fmt.Sprintf("**Nothing playing**\nQueue: `%d` track(s) · loop `%s`",
					state.Len(), state.LoopMode()),
			},
		)
		container.Components = append(container.Components, asContainerSubs(cardButtons(cardState{locked: true}))...)
		return []discord.LayoutComponent{container}
	}

	track := *p.Track
	badge := ui.SourceBadgeFor(track.Info.SourceName)
	sourceName := formatSourceName(track.Info.SourceName)
	status := "▶ Now playing"
	if p.Paused {
		status = "⏸ Paused"
	}
	header := discord.TextDisplayComponent{
		Content: fmt.Sprintf("%s **%s**\n%s\n%s · %s `%s`",
			badge.Emoji, status, ui.TrackMarkdown(track), track.Info.Author, badge.Emoji, sourceName),
	}
	section := discord.SectionComponent{
		Components: []discord.SectionSubComponent{header},
	}
	if track.Info.ArtworkURL != nil && *track.Info.ArtworkURL != "" {
		section.Accessory = discord.ThumbnailComponent{
			Media:       discord.UnfurledMediaItem{URL: *track.Info.ArtworkURL},
			Description: track.Info.Title,
		}
	}
	container.Components = append(container.Components, section)

	// Progress bar
	progress := "🔴 LIVE"
	if !track.Info.IsStream {
		pos, total := p.Position(), track.Info.Length
		const cells = 18
		filled := 0
		if total > 0 {
			filled = int(float64(pos) / float64(total) * cells)
		}
		filled = clampInt(filled, 0, cells)
		bar := strings.Repeat("▬", filled)
		if filled < cells {
			bar += "🔘" + strings.Repeat("─", cells-filled-1)
		} else {
			bar += "🔘"
		}
		progress = fmt.Sprintf("`%s` %s `%s`", ui.FormatDuration(pos), bar, ui.FormatDuration(total))
	}

	upNext := "Queue empty — add more with /play"
	if next, ok := state.PeekNext(); ok {
		upNext = "Up next: " + ui.TrackMarkdown(next)
	}

	requesterLine := ""
	req := state.GetCurrentRequester()
	if req != 0 {
		requesterLine = fmt.Sprintf("\n👤 <@%d>", uint64(req))
	}

	footer := discord.TextDisplayComponent{
		Content: fmt.Sprintf("%s\n%s\n🔁 `%s` · 🔊 `%d` · queue `%d`%s",
			progress, upNext, state.LoopMode(), p.Volume, state.Len(), requesterLine),
	}
	container.Components = append(container.Components,
		discord.SeparatorComponent{},
		footer,
		discord.SeparatorComponent{},
	)

	loopStr := state.LoopMode().String()
	cs := cardState{
		paused:     p.Paused,
		loop:       loopStr,
		queueEmpty: state.Len() == 0,
	}
	container.AccentColor = badge.Color
	container.Components = append(container.Components, asContainerSubs(cardButtons(cs))...)

	return []discord.LayoutComponent{container}
}

func renderCardLocked(reason string) []discord.LayoutComponent {
	container := discord.ContainerComponent{AccentColor: 0x2B2D31}
	container.Components = append(container.Components,
		discord.TextDisplayComponent{Content: "**Session ended** — " + reason},
	)
	container.Components = append(container.Components, asContainerSubs(cardButtons(cardState{locked: true}))...)
	return []discord.LayoutComponent{container}
}

type cardState struct {
	locked     bool
	paused     bool
	loop       string
	queueEmpty bool
}

func cardButtons(cs cardState) []discord.LayoutComponent {
	row := func(btns ...discord.ButtonComponent) discord.ActionRowComponent {
		r := discord.ActionRowComponent{}
		for _, x := range btns {
			if cs.locked {
				x.Disabled = true
			}
			r.Components = append(r.Components, x)
		}
		return r
	}

	toggleEmoji := "⏸"
	if cs.paused {
		toggleEmoji = "▶"
	}
	toggleStyle := discord.ButtonStylePrimary

	loopEmoji := "🔁"
	loopStyle := discord.ButtonStyleSecondary
	if cs.loop != "off" {
		loopStyle = discord.ButtonStyleSuccess
	}

	return []discord.LayoutComponent{
		row(
			discord.NewSecondaryButton("⏮", "hex:prev"),
			discord.ButtonComponent{CustomID: "hex:toggle", Emoji: &discord.ComponentEmoji{Name: toggleEmoji}, Style: toggleStyle},
			discord.NewSecondaryButton("⏭", "hex:skip"),
		),
		row(
			discord.ButtonComponent{CustomID: "hex:loop", Emoji: &discord.ComponentEmoji{Name: loopEmoji}, Style: loopStyle},
			discord.NewSecondaryButton("🔀", "hex:shuffle"),
			discord.NewSecondaryButton("🔉", "hex:voldown"),
			discord.NewSecondaryButton("🔊", "hex:volup"),
		),
		row(discord.NewDangerButton("⏹", "hex:stop")),
	}
}

func asContainerSubs(rows []discord.LayoutComponent) []discord.ContainerSubComponent {
	out := make([]discord.ContainerSubComponent, len(rows))
	for i, r := range rows {
		out[i] = r.(discord.ContainerSubComponent)
	}
	return out
}

// formatSourceName converts raw Lavalink source names to friendly labels.
func formatSourceName(raw string) string {
	switch raw {
	case "youtube":
		return "YouTube"
	case "youtubemusic":
		return "YT Music"
	case "soundcloud":
		return "SoundCloud"
	case "spotify":
		return "Spotify"
	case "deezer":
		return "Deezer"
	case "applemusic":
		return "Apple Music"
	case "http":
		return "Direct URL"
	default:
		return raw
	}
}
