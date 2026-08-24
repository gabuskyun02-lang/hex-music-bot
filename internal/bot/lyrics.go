package bot

import (
	"fmt"
	"strings"
	"sync"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/omit"
)

// LyricDoc is one paginated lyrics session, addressed by a short session ID.
type LyricDoc struct {
	SessionID string
	Pages     []string
	Page      int
	// Embed metadata for clean rendering (beatra-style lyrics embed).
	Artist    string
	Title     string
	SourceURL string
	Color     int
}

const lyricsCacheCap = 200

// LyricsCache stores active /lyrics sessions (FIFO eviction).
type LyricsCache struct {
	mu    sync.Mutex
	docs  map[string]*LyricDoc
	order []string
}

// NewLyricsCache builds an empty cache.
func NewLyricsCache() *LyricsCache {
	return &LyricsCache{docs: make(map[string]*LyricDoc)}
}

// Put stores a doc, evicting the oldest when over capacity.
func (c *LyricsCache) Put(doc *LyricDoc) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, exists := c.docs[doc.SessionID]; !exists {
		c.order = append(c.order, doc.SessionID)
	}
	c.docs[doc.SessionID] = doc
	for len(c.order) > lyricsCacheCap {
		oldest := c.order[0]
		c.order = c.order[1:]
		delete(c.docs, oldest)
	}
}

// Get returns the doc for a session ID.
func (c *LyricsCache) Get(sessionID string) (*LyricDoc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc, ok := c.docs[sessionID]
	return doc, ok
}

// Advance moves the doc's page by delta, clamped to the valid range.
// Returns the updated doc; ok=false when the session expired.
func (c *LyricsCache) Advance(sessionID string, delta int) (*LyricDoc, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	doc, ok := c.docs[sessionID]
	if !ok {
		return nil, false
	}
	doc.Page += delta
	if doc.Page < 0 {
		doc.Page = 0
	}
	if doc.Page >= len(doc.Pages) {
		doc.Page = len(doc.Pages) - 1
	}
	return doc, true
}

// LyricContainer renders a doc's current page as a V2 container:
// 🎤 title header, artist/source credit, page body, page footer.
func LyricContainer(doc *LyricDoc) []discord.LayoutComponent {
	c := discord.ContainerComponent{AccentColor: doc.Color}
	c.Components = append(c.Components,
		discord.TextDisplayComponent{Content: "### 🎤 " + doc.Title},
	)
	if doc.SourceURL != "" {
		c.Components = append(c.Components,
			discord.TextDisplayComponent{
				Content: fmt.Sprintf("%s · [source](<%s>)", doc.Artist, doc.SourceURL),
			},
		)
	} else {
		c.Components = append(c.Components,
			discord.TextDisplayComponent{Content: doc.Artist},
		)
	}
	c.Components = append(c.Components,
		discord.SeparatorComponent{},
		discord.TextDisplayComponent{Content: doc.Pages[doc.Page]},
		discord.SeparatorComponent{},
		discord.TextDisplayComponent{
			Content: fmt.Sprintf("Page %d/%d", doc.Page+1, len(doc.Pages)),
		},
	)
	return []discord.LayoutComponent{c}
}

// LyricButtons builds the pager row for a doc state.
func LyricButtons(doc *LyricDoc) discord.LayoutComponent {
	prev := discord.NewSecondaryButton("◀", "hexlyr:p:"+doc.SessionID)
	next := discord.NewSecondaryButton("▶", "hexlyr:n:"+doc.SessionID)
	prev.Disabled = doc.Page == 0
	next.Disabled = doc.Page >= len(doc.Pages)-1
	return discord.ActionRowComponent{Components: []discord.InteractiveComponent{prev, next}}
}

// handleLyricsButton serves pager clicks by updating the originating
// message directly through the interaction — no REST edit budget spent.
func (b *Bot) handleLyricsButton(event *events.ComponentInteractionCreate, customID string) {
	parts := strings.SplitN(customID, ":", 3)
	if len(parts) != 3 || (parts[1] != "p" && parts[1] != "n") {
		_ = event.DeferUpdateMessage()
		return
	}
	delta := -1
	if parts[1] == "n" {
		delta = 1
	}
	doc, ok := b.Lyrics.Advance(parts[2], delta)
	if !ok {
		_ = event.UpdateMessage(discord.MessageUpdate{
			Content:    omit.Ptr("Lyrics session expired — run /lyrics again"),
			Embeds:     &[]discord.Embed{},
			Components: &[]discord.LayoutComponent{},
		})
		return
	}
	components := append(LyricContainer(doc), LyricButtons(doc))
	_ = event.UpdateMessage(discord.NewMessageUpdateV2(components))
}
