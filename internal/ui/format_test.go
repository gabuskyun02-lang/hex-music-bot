package ui

import (
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgolink/v4/lavalink"
)

func TestProgressBar(t *testing.T) {
	d := func(ms int) lavalink.Duration { return lavalink.Duration(time.Duration(ms) * time.Millisecond) }
	got := ProgressBar(d(50_000), d(100_000), 10)
	if !strings.Contains(got, "▬▬▬▬▬") || !strings.Contains(got, "🔘") || strings.Count(got, "─") != 4 {
		t.Fatalf("half bar wrong: %q", got)
	}
	full := ProgressBar(d(100_000), d(100_000), 3)
	if strings.Count(full, "🔘") != 1 || strings.Contains(full, "─") {
		t.Fatalf("full bar wrong: %q", full)
	}
	if got := ProgressBar(d(0), d(0), 5); !strings.Contains(got, "🔘") || strings.Count(got, "─") != 4 {
		t.Fatalf("zero total should render empty bar: %q", got)
	}
}
