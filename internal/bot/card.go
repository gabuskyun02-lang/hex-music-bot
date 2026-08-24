package bot

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/ui"
)

const (
	cardTickInterval = 10 * time.Second

	// ponytail: cardMinEditInterval is a flat per-guild floor against the
	// 5-edits/5s channel bucket, not a real rate limiter. If multi-guild
	// load ever brushes 429s anyway, upgrade to a shared token bucket keyed
	// by channel in the edit chokepoint.
	cardMinEditInterval = 1500 * time.Millisecond
)

type cardEntry struct {
	channelID snowflake.ID
	messageID snowflake.ID
	cancel    context.CancelFunc
	wake      chan struct{} // buffered(1) wakeup token; dirty carries intent
	dirty     atomic.Bool   // render+edit pending
	lastHash  string        // fnv64a of last edited payload
	lastEdit  time.Time
	done      chan struct{} // closed when the pump exits
}

// cardBody couples the rendered body with the button-deck state captured
// during the render pass.
type cardBody struct {
	comps []discord.LayoutComponent
	cs    cardState
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
	entry := &cardEntry{
		channelID: channelID,
		messageID: msg.ID,
		cancel:    cancel,
		wake:      make(chan struct{}, 1),
		done:      make(chan struct{}),
	}
	m.mu.Lock()
	if m.cards[guildID] != reserve { // superseded while creating
		cancel()
		m.mu.Unlock()
		_ = m.b.Client.Rest.DeleteMessage(channelID, msg.ID)
		return
	}
	m.cards[guildID] = entry
	m.mu.Unlock()
	go m.pump(ctx, guildID, entry)
}

// pump is the per-guild card editor: it coalesces Refresh signals and the
// periodic tick into at most one REST edit per change, skipping edits whose
// serialized payload is byte-identical to the last one sent.
func (m *CardManager) pump(ctx context.Context, guildID snowflake.ID, entry *cardEntry) {
	defer close(entry.done)
	t := time.NewTicker(cardTickInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-entry.wake:
		case <-t.C:
			entry.dirty.Store(true) // periodic check even when idle/paused
		}
		if !entry.dirty.CompareAndSwap(true, false) {
			continue // spurious wake, nothing pending
		}

		comps := m.b.renderCard(guildID)
		hash := componentHash(comps)
		if hash == entry.lastHash {
			m.b.Metrics.Inc("hex_music_bot_card_coalesced") // byte-identical payload
			continue
		}
		if since := time.Since(entry.lastEdit); since < cardMinEditInterval {
			time.Sleep(cardMinEditInterval - since)
			comps = m.b.renderCard(guildID) // latest-wins re-render
			hash = componentHash(comps)
			if hash == entry.lastHash {
				m.b.Metrics.Inc("hex_music_bot_card_coalesced")
				continue
			}
		}
		m.edit(guildID, entry, comps)
		entry.lastHash = hash
		entry.lastEdit = time.Now()
	}
}

// componentHash fingerprints a rendered payload so identical renders can be
// skipped without a REST round trip.
func componentHash(comps []discord.LayoutComponent) string {
	data, err := json.Marshal(comps)
	if err != nil {
		return fmt.Sprintf("%p", comps) // unmarshalable payload always edits
	}
	h := fnv.New64a()
	h.Write(data)
	return string(h.Sum(nil))
}

func (m *CardManager) Refresh(guildID snowflake.ID) {
	m.mu.Lock()
	entry, ok := m.cards[guildID]
	m.mu.Unlock()
	if !ok || entry.messageID == 0 { // zero = creation in progress
		return
	}
	entry.dirty.Store(true)
	select {
	case entry.wake <- struct{}{}:
	default: // pump already has a pending wake
	}
}

func (m *CardManager) Finalize(guildID snowflake.ID, reason string) {
	entry, ok := m.removeAndWait(guildID)
	m.b.Sync.Stop(guildID)
	if !ok || entry.messageID == 0 { // zero = creation in progress
		return
	}
	if _, err := m.b.Client.Rest.UpdateMessage(entry.channelID, entry.messageID,
		discord.NewMessageUpdateV2(renderCardLocked(reason))); err != nil {
		slog.Debug("card finalize failed", slog.Any("err", err))
	}
}

func (m *CardManager) Drop(guildID snowflake.ID) {
	m.removeAndWait(guildID)
}

// removeAndWait cancels the guild's pump and waits (bounded) for it to exit,
// so an in-flight pump edit can never interleave with a subsequent direct
// write such as the Finalize locked-card edit.
func (m *CardManager) removeAndWait(guildID snowflake.ID) (*cardEntry, bool) {
	m.mu.Lock()
	entry, ok := m.cards[guildID]
	if ok {
		entry.cancel()
		delete(m.cards, guildID)
	}
	m.mu.Unlock()
	if !ok || entry.done == nil {
		return entry, ok
	}
	select {
	case <-entry.done:
	case <-time.After(2 * time.Second):
		slog.Debug("card pump shutdown timed out", slog.String("guild", guildID.String()))
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

// RenderCardBody exposes the card's information section (header, progress,
// up-next, requester line, footer stats — no control buttons) for the
// /now-playing snapshot reply.
func (b *Bot) RenderCardBody(guildID snowflake.ID) []discord.LayoutComponent {
	return b.renderCardBodyState(guildID).comps
}

// renderCardBodyState renders the card body and returns the button-deck state
// captured during the pass, so renderCard never re-reads player state.
func (b *Bot) renderCardBodyState(guildID snowflake.ID) cardBody {
	container := discord.ContainerComponent{AccentColor: ui.SourceBadgeFor("").Color}
	state := b.Player.Get(guildID)
	p := b.Lavalink.ExistingPlayer(guildID)
	out := cardBody{comps: []discord.LayoutComponent{container}}

	if p == nil || p.Track == nil {
		container.Components = append(container.Components,
			discord.TextDisplayComponent{
				Content: fmt.Sprintf("**Nothing playing**\nQueue: `%d` track(s) · loop `%s`",
					state.Len(), state.LoopMode()),
			},
		)
		return out
	}
	track := *p.Track
	badge := ui.SourceBadgeFor(track.Info.SourceName)
	sourceName := formatSourceName(track.Info.SourceName)
	cs := cardState{paused: p.Paused}
	status := "▶ Now Playing"
	if p.Paused {
		status = "⏸ Paused" + pauseReasonSuffix(cs.pauseReason)
	}
	header := discord.TextDisplayComponent{
		Content: fmt.Sprintf("%s **%s**\n%s\n%s · %s `%s`",
			badge.Emoji, status, ui.TrackMarkdown(track), track.Info.Author, badge.Emoji, sourceName),
	}
	section := discord.SectionComponent{
		Components: []discord.SectionSubComponent{header},
	}
	progress := "🔴 LIVE"
	if !track.Info.IsStream {
		progress = ui.ProgressBar(p.Position(), track.Info.Length, 18)
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
		section,
		discord.SeparatorComponent{},
	)
	if track.Info.ArtworkURL != nil && *track.Info.ArtworkURL != "" {
		container.Components = append(container.Components,
			discord.NewMediaGallery(discord.MediaGalleryItem{
				Media:       discord.UnfurledMediaItem{URL: *track.Info.ArtworkURL},
				Description: track.Info.Title,
			}),
		)
	}
	container.Components = append(container.Components,
		footer,
		discord.SeparatorComponent{},
	)
	out.cs = cs
	return out
}

// renderCard builds the live Components V2 card: body plus the hex:* button
// deck. Only the live card carries controls; snapshot replies stay static.
func (b *Bot) renderCard(guildID snowflake.ID) []discord.LayoutComponent {
	body := b.renderCardBodyState(guildID)
	buttons := asContainerSubs(cardButtons(body.cs))
	for _, comp := range body.comps {
		if c, ok := comp.(discord.ContainerComponent); ok {
			c.Components = append(c.Components, buttons...)
			return []discord.LayoutComponent{c}
		}
	}
	return body.comps
}

// pauseReasonSuffix appends a Beatra-style auto-pause indicator to the status
// line. Reason stays unpopulated until the future auto-pause feature lands.
func pauseReasonSuffix(reason string) string {
	switch reason {
	case "mute":
		return " 🔇"
	case "alone":
		return " ⏳"
	default:
		return ""
	}
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
	locked      bool
	paused      bool
	pauseReason string
	loop        string
	queueEmpty  bool
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
