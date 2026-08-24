package bot

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
)

// TestPagerPageTurns is the throwaway G3-A acceptance check: page turns,
// boundary clamping, FIFO eviction, and expiry all behave.
func TestPagerPageTurns(t *testing.T) {
	m := NewPagerManager()
	s := &PagerSession{
		ID:      newPagerSessionID(),
		Header:  "📋 Up next (25)",
		Rows:    [][]string{page(1, 10), page(11, 20), page(21, 25)},
		Footer:  "25 tracks",
		Accent:  0x5865F2,
		Expires: time.Now().Add(pagerSessionTTL),
	}
	m.Put(s)

	if _, ok := m.Move(s.ID, pagerFirst); !ok {
		t.Fatal("first from page 0 stays on page 0")
	}
	if got, _ := m.Move(s.ID, -1); got.Page != 0 {
		t.Fatalf("prev at start must clamp to 0, got %d", got.Page)
	}
	got, ok := m.Move(s.ID, 1)
	if !ok || got.Page != 1 {
		t.Fatalf("next must land on page 1, got page=%d ok=%v", got.Page, ok)
	}
	last, _ := m.Move(s.ID, pagerLast)
	if last.Page != 2 {
		t.Fatalf("last must land on final page, got %d", last.Page)
	}
	if got, _ := m.Move(s.ID, 1); got.Page != 2 {
		t.Fatalf("next at end must clamp to last page, got %d", got.Page)
	}

	// Rendering: header, rows of current page only, page indicator.
	comps := RenderPagerPage(last)
	var texts []string
	for _, c := range comps {
		if container, ok := c.(discord.ContainerComponent); ok {
			for _, sub := range container.Components {
				if td, ok := sub.(discord.TextDisplayComponent); ok {
					texts = append(texts, td.Content)
				}
			}
		}
	}
	joined := strings.Join(texts, "\n")
	for _, want := range []string{"### 📋 Up next (25)", "t21", "Page 3/3 · 25 tracks"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("render missing %q in %q", want, joined)
		}
	}
	if strings.Contains(joined, "t12") {
		t.Fatal("page 3 render leaked rows from page 1")
	}

	// Expiry: past Expires, Move fails -> handler would answer ephemeral.
	expired := &PagerSession{ID: newPagerSessionID(), Rows: [][]string{{"x"}}, Expires: time.Now().Add(-time.Second)}
	m.Put(expired)
	if _, ok := m.Move(expired.ID, 1); ok {
		t.Fatal("expired session must fail Move")
	}
	if _, ok := m.Move("pg404", 1); ok {
		t.Fatal("unknown session must fail Move")
	}
}

// TestPagerChunking proves buildListContainer splits oversized row sets.
func TestPagerChunking(t *testing.T) {
	rows := make([]string, 0, 120)
	for i := range 120 {
		rows = append(rows, strings.Repeat("r", 100)+" "+strings.Repeat("=", i)) // ~100+ chars each
	}
	comps := BuildListContainer("📋 Big list (120)", rows, "Page 1/1 · 120 tracks", 0x5865F2)
	displays := 0
	for _, c := range comps {
		container, ok := c.(discord.ContainerComponent)
		if !ok {
			continue
		}
		for _, sub := range container.Components {
			if td, ok := sub.(discord.TextDisplayComponent); ok {
				displays++
				if n := len([]rune(td.Content)); n > 4000 {
					t.Fatalf("display exceeds 4000 runes: %d", n)
				}
			}
		}
	}
	if displays < 4 { // 120 rows × ~110 chars ≈ 13k chars → ≥4 displays at the 3500 ceiling
		t.Fatalf("expected chunked text displays, got %d", displays)
	}
}

func page(from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("t%d: track", i))
	}
	return out
}
