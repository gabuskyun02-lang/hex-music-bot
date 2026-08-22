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
// Button presses ack via the interaction itself; only progress ticks and
// action refreshes consume REST edits — that is the coalescing budget.
type CardManager struct {
	b     *Bot
	mu    sync.Mutex
	cards map[snowflake.ID]*cardEntry
}

// NewCardManager builds the manager for a Bot.
func NewCardManager(b *Bot) *CardManager {
	return &CardManager{b: b, cards: make(map[snowflake.ID]*cardEntry)}
}

// Create posts a fresh card in channelID, replacing any existing one.
func (m *CardManager) Create(guildID, channelID snowflake.ID) {
	m.mu.Lock()
	m.removeLocked(guildID)
	m.mu.Unlock()

	msg, err := m.b.Client.Rest.CreateMessage(channelID, discord.MessageCreate{
		Flags:      discord.MessageFlagIsComponentsV2,
		Components: m.b.renderCard(guildID),
	})
	if err != nil {
		slog.Error("card create failed", slog.String("guild", guildID.String()), slog.Any("err", err))
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	entry := &cardEntry{channelID: channelID, messageID: msg.ID, cancel: cancel}
	m.mu.Lock()
	m.removeLocked(guildID) // a concurrent Create may have posted while we did
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

// Refresh immediately re-renders the card (after actions / track changes).
func (m *CardManager) Refresh(guildID snowflake.ID) {
	m.mu.Lock()
	entry, ok := m.cards[guildID]
	m.mu.Unlock()
	if ok {
		m.edit(guildID, entry, m.b.renderCard(guildID))
	}
}

// Finalize locks the card with a reason and stops its ticker.
func (m *CardManager) Finalize(guildID snowflake.ID, reason string) {
	m.mu.Lock()
	entry, ok := m.removeLocked(guildID)
	m.mu.Unlock()
	if !ok {
		return
	}
	if _, err := m.b.Client.Rest.UpdateMessage(entry.channelID, entry.messageID,
		discord.NewMessageUpdateV2(renderCardLocked(reason))); err != nil {
		slog.Debug("card finalize failed", slog.Any("err", err))
	}
}

// removeLocked cancels and forgets the guild's card entry.
// Caller must hold m.mu.
func (m *CardManager) removeLocked(guildID snowflake.ID) (*cardEntry, bool) {
	entry, ok := m.cards[guildID]
	if ok {
		entry.cancel()
		delete(m.cards, guildID)
	}
	return entry, ok
}

// Drop forgets the card without editing (bot left voice).
func (m *CardManager) Drop(guildID snowflake.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.removeLocked(guildID)
}

func (m *CardManager) edit(guildID snowflake.ID, entry *cardEntry, comps []discord.LayoutComponent) {
	m.b.Metrics.Inc("hex_music_bot_card_edits")
	if _, err := m.b.Client.Rest.UpdateMessage(entry.channelID, entry.messageID,
		discord.NewMessageUpdateV2(comps)); err != nil {
		slog.Debug("card edit failed", slog.String("guild", guildID.String()), slog.Any("err", err))
	}
}

// renderCard builds the live Components V2 card from current player state.
func (b *Bot) renderCard(guildID snowflake.ID) []discord.LayoutComponent {
	container := discord.ContainerComponent{AccentColor: 0x5865F2}
	state := b.Player.Get(guildID)
	p := b.Lavalink.ExistingPlayer(guildID)

	if p == nil || p.Track == nil {
		container.Components = append(container.Components,
			discord.TextDisplayComponent{
				Content: fmt.Sprintf("**Nothing playing**\nQueue: `%d` track(s) · loop `%s`",
					state.Len(), state.LoopMode()),
			},
		)
		container.Components = append(container.Components, asContainerSubs(cardButtons(true))...)
		return []discord.LayoutComponent{container}
	}

	track := *p.Track
	status := "Now playing"
	if p.Paused {
		status = "Paused"
	}
	header := discord.TextDisplayComponent{
		Content: fmt.Sprintf("**%s**\n%s\n%s · `%s`",
			status, ui.TrackMarkdown(track), track.Info.Author, track.Info.SourceName),
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
	footer := discord.TextDisplayComponent{
		Content: fmt.Sprintf("%s\n%s\n🔁 `%s` · 🔊 `%d` · queue `%d`",
			progress, upNext, state.LoopMode(), p.Volume, state.Len()),
	}
	container.Components = append(container.Components,
		discord.SeparatorComponent{},
		footer,
	)
	container.Components = append(container.Components, asContainerSubs(cardButtons(false))...)
	return []discord.LayoutComponent{container}
}

// renderCardLocked builds the inert end-state card.
func renderCardLocked(reason string) []discord.LayoutComponent {
	container := discord.ContainerComponent{AccentColor: 0x2B2D31}
	container.Components = append(container.Components,
		discord.TextDisplayComponent{Content: "**Session ended** — " + reason},
	)
	container.Components = append(container.Components, asContainerSubs(cardButtons(true))...)
	return []discord.LayoutComponent{container}
}

// cardButtons builds the control deck; disabled locks every button.
func cardButtons(disabled bool) []discord.LayoutComponent {
	row := func(btns ...discord.ButtonComponent) discord.ActionRowComponent {
		r := discord.ActionRowComponent{}
		for _, x := range btns {
			x.Disabled = disabled
			r.Components = append(r.Components, x)
		}
		return r
	}
	return []discord.LayoutComponent{
		row(
			discord.NewSecondaryButton("⏮", "hex:prev"),
			discord.NewPrimaryButton("⏯", "hex:toggle"),
			discord.NewSecondaryButton("⏭", "hex:skip"),
		),
		row(
			discord.NewSecondaryButton("🔁", "hex:loop"),
			discord.NewSecondaryButton("🔀", "hex:shuffle"),
			discord.NewSecondaryButton("🔉", "hex:voldown"),
			discord.NewSecondaryButton("🔊", "hex:volup"),
		),
		row(discord.NewDangerButton("⏹", "hex:stop")),
	}
}

// asContainerSubs converts layout rows (ActionRow) into the
// ContainerSubComponent type that ContainerComponent accepts.
func asContainerSubs(rows []discord.LayoutComponent) []discord.ContainerSubComponent {
	out := make([]discord.ContainerSubComponent, len(rows))
	for i, r := range rows {
		out[i] = r.(discord.ContainerSubComponent)
	}
	return out
}

// Lookup returns the card's channel/message IDs without side effects.
// Returns nil when no active card exists for the guild.
func (m *CardManager) Lookup(guildID snowflake.ID) *cardEntry {
	m.mu.Lock()
	defer m.mu.Unlock()
	entry, ok := m.cards[guildID]
	if !ok {
		return nil
	}
	return entry
}
