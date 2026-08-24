package bot

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

// PagerSession is one paginated view, addressed by a short session ID.
// Sessions hold DATA (pre-chunked pages of row strings), not frozen
// components — page turns re-render through buildListContainer.
type PagerSession struct {
	ID      string
	Header  string // "📋 Up next (70)"
	Rows    [][]string
	Page    int
	Footer  string // caller's footer; the pager prepends "Page i/n · "
	Accent  int
	Expires time.Time // creation + 5m, fixed (ponytail: sliding TTL if users hit expiry often)
}

const pagerSessionTTL = 5 * time.Minute
const pagerCacheCap = 300

// Move page-delta sentinels: clampPage resolves both to the target edge.
const (
	pagerFirst = math.MinInt
	pagerLast  = math.MaxInt
)

// PagerManager stores active pager sessions (FIFO eviction), same shape as
// LyricsCache. Anyone may paginate — no invoker lock; queue views are
// communal like card buttons. (Documented divergence from Lunox.)
type PagerManager struct {
	mu       sync.Mutex
	sessions map[string]*PagerSession
	order    []string
}

// NewPagerManager builds an empty manager.
func NewPagerManager() *PagerManager {
	return &PagerManager{sessions: make(map[string]*PagerSession)}
}

// Put stores a session, evicting the oldest when over capacity.
func (m *PagerManager) Put(s *PagerSession) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.sessions[s.ID]; !exists {
		m.order = append(m.order, s.ID)
	}
	m.sessions[s.ID] = s
	for len(m.order) > pagerCacheCap {
		oldest := m.order[0]
		m.order = m.order[1:]
		delete(m.sessions, oldest)
	}
}

// Move changes the session's page by delta, clamped to the valid range.
// Returns false when the session is unknown or past its expiry.
func (m *PagerManager) Move(sessionID string, delta int) (*PagerSession, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[sessionID]
	if !ok || time.Now().After(s.Expires) {
		return nil, false
	}
	switch {
	case delta == pagerFirst:
		s.Page = 0
	case delta == pagerLast:
		s.Page = len(s.Rows) - 1
	default:
		s.Page = clampPage(s, s.Page+delta)
	}
	return s, true
}

// clampPage resolves the target page. pagerFirst/pagerLast are handled by
// Move (MinInt/MaxInt arithmetic overflows); this only bounds ordinary deltas.
func clampPage(s *PagerSession, page int) int {
	if page < 0 {
		return 0
	}
	if page >= len(s.Rows) {
		return len(s.Rows) - 1
	}
	return page
}

// BuildListContainer renders header/rows/footer as one V2 container.
// Rows are chunked into TextDisplays at a 3500-rune safety ceiling
// (~55-60 list rows per display under Discord's 4000-char component cap).
// Shared by queue/history conversions and the pager's page turns.
//
// SplitListRows pre-chunks rows into pages; NewPagerSession assembles them
// into a PagerSession — both exported for the commands package.
func BuildListContainer(header string, rows []string, footer string, accent int) []discord.LayoutComponent {
	c := discord.ContainerComponent{AccentColor: accent}
	c.Components = append(c.Components,
		discord.TextDisplayComponent{Content: "### " + header},
		discord.SeparatorComponent{},
	)
	var current strings.Builder
	count := 0
	flush := func() {
		c.Components = append(c.Components,
			discord.TextDisplayComponent{Content: strings.TrimRight(current.String(), "\n")},
		)
		current.Reset()
		count = 0
	}
	for _, row := range rows {
		rowLen := len([]rune(row)) + 1 // trailing newline joins rows in one display
		if count+rowLen > 3500 && count > 0 {
			flush()
		}
		current.WriteString(row + "\n")
		count += rowLen
	}
	if current.Len() > 0 {
		flush()
	}
	c.Components = append(c.Components,
		discord.SeparatorComponent{},
		discord.TextDisplayComponent{Content: footer},
	)
	return []discord.LayoutComponent{c}
}

// PagerButtons builds the ⏮ ◀ ✖ ▶ ⏭ row for a session page state.
func PagerButtons(s *PagerSession, page int) discord.LayoutComponent {
	id := func(action string) string { return "hexp:" + action + ":" + s.ID }
	buttons := []discord.ButtonComponent{
		discord.NewSecondaryButton("⏮", id("f")),
		discord.NewSecondaryButton("◀", id("p")),
		discord.NewDangerButton("✖", id("c")),
		discord.NewSecondaryButton("▶", id("n")),
		discord.NewSecondaryButton("⏭", id("l")),
	}
	atStart := page == 0
	atEnd := page >= len(s.Rows)-1
	buttons[0].Disabled = atStart
	buttons[1].Disabled = atStart
	buttons[3].Disabled = atEnd
	buttons[4].Disabled = atEnd
	row := discord.ActionRowComponent{}
	for _, b := range buttons {
		row.Components = append(row.Components, b)
	}
	return row
}

// RenderPagerPage renders a session's current page through the shared
// builder, with the page indicator prepended to the footer.
func RenderPagerPage(s *PagerSession) []discord.LayoutComponent {
	footer := fmt.Sprintf("Page %d/%d · %s", s.Page+1, len(s.Rows), s.Footer)
	return BuildListContainer(s.Header, s.Rows[s.Page], footer, s.Accent)
}

// NewPagerSession chunks rows into ~3500-rune pages, registers the session
// with the bot's PagerManager, and reports whether pagination is needed
// (>1 page). Single-page lists render directly via BuildListContainer.
func (b *Bot) NewPagerSession(header string, rows []string, footer string, accent int) (*PagerSession, bool) {
	if len(rows) <= 1 {
		return nil, false
	}
	var pages [][]string
	var current []string
	count := 0
	for _, row := range rows {
		n := len([]rune(row)) + 1 // trailing newline joins rows in one display
		if count+n > 3500 && count > 0 {
			pages = append(pages, current)
			current = nil
			count = 0
		}
		current = append(current, row)
		count += n
	}
	if current != nil {
		pages = append(pages, current)
	}
	s := &PagerSession{
		ID:      newPagerSessionID(),
		Header:  header,
		Rows:    pages,
		Footer:  footer,
		Accent:  accent,
		Expires: time.Now().Add(pagerSessionTTL),
	}
	b.Pagers.Put(s)
	return s, true
}

// handlePagerButton serves hexp: clicks: page turns re-render the session
// through buildListContainer via interaction update; ✖ strips the message
// The click is acked by whichever update path runs — no bare defer needed.
func (b *Bot) handlePagerButton(event *events.ComponentInteractionCreate, customID string) {
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 {
		_ = event.DeferUpdateMessage()
		return
	}
	action, sessionID := parts[1], parts[2]

	if action == "c" {
		closed := discord.ContainerComponent{}
		closed.Components = append(closed.Components,
			discord.TextDisplayComponent{Content: "*View closed*"},
		)
		_ = event.UpdateMessage(discord.NewMessageUpdateV2([]discord.LayoutComponent{closed}))
		return
	}

	var delta int
	switch action {
	case "f":
		delta = pagerFirst
	case "p":
		delta = -1
	case "n":
		delta = 1
	case "l":
		delta = pagerLast
	default:
		_ = event.DeferUpdateMessage()
		return
	}
	s, ok := b.Pagers.Move(sessionID, delta)
	if !ok {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "Session expired",
			Flags:   discord.MessageFlagEphemeral,
		})
		return
	}
	components := append(RenderPagerPage(s), PagerButtons(s, s.Page))
	_ = event.UpdateMessage(discord.NewMessageUpdateV2(components))
}

// newPagerSessionID mints a short unique-enough session ID — same trick as
// the lyrics session IDs (commands/lyrics.go).
func newPagerSessionID() string {
	return fmt.Sprintf("pg%d", time.Now().UnixNano()%1e9)
}
