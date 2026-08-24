package bot

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgolink/v4/lavalink"

	"hex-music-bot/internal/lyrics"
)

func syncLines() []lyrics.Line {
	return []lyrics.Line{
		{At: 0, Text: "first"},
		{At: 10 * time.Second, Text: "second"},
		{At: 20 * time.Second, Text: "third"},
		{At: 30 * time.Second, Text: ""},
		{At: 40 * time.Second, Text: "fifth"},
	}
}

func TestCurrentLineIdx(t *testing.T) {
	lines := syncLines()
	cases := []struct {
		name string
		pos  lavalink.Duration
		want int
	}{
		{"before first clamps to zero", 0, 0},
		{"mid gap holds previous line", lavalink.Duration(15 * time.Second / time.Millisecond), 1},
		{"exactly on line", lavalink.Duration(30 * time.Second / time.Millisecond), 3},
		{"past end stays at last", lavalink.Duration(999 * time.Second / time.Millisecond), 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := currentLineIdx(lines, tc.pos); got != tc.want {
				t.Fatalf("currentLineIdx(%v) = %d, want %d", tc.pos.Milliseconds(), got, tc.want)
			}
		})
	}
}

func TestRenderSyncWindow(t *testing.T) {
	lines := syncLines()

	got := renderSyncWindow(lines, 2)
	if !strings.Contains(got, "**> `0:20` third**") {
		t.Errorf("active line not bolded:\n%s", got)
	}
	if strings.Count(got, "\n") != 4 {
		t.Errorf("window of ±3 at interior line should span all 5 lines:\n%s", got)
	}

	// At index 0 the window clamps high; no negative-index lines.
	head := renderSyncWindow(lines, 0)
	if !strings.Contains(head, "second") || !strings.Contains(head, "♪") {
		t.Errorf("head window missing expected neighbors:\n%s", head)
	}
	if strings.Contains(head, "**>") && strings.Count(head, "**>") != 1 {
		t.Errorf("exactly one active line expected:\n%s", head)
	}

	// Empty text renders as ♪.
	if got := renderSyncWindow(lines, 3); !strings.Contains(got, "`0:30` ♪") {
		t.Errorf("empty line should render as ♪:\n%s", got)
	}

	// Tail window clamps to last line.
	tail := renderSyncWindow(lines, 4)
	if tail == "" || strings.Contains(tail, "first") {
		t.Errorf("tail window should clamp to final lines:\n%s", tail)
	}
}

func TestSyncManagerStopIdempotent(t *testing.T) {
	m := NewLyricSyncManager(nil)
	m.Stop(123) // unknown guild: must be a no-op, not a panic
	if len(m.sessions) != 0 {
		t.Fatalf("expected empty session map, got %d entries", len(m.sessions))
	}
}
