package player

import (
	"testing"

	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

func TestShuffleFlagLifecycle(t *testing.T) {
	s := &State{requesters: map[string]snowflake.ID{}}
	for i := 0; i < 6; i++ {
		s.Enqueue(lavalink.Track{Info: lavalink.TrackInfo{Identifier: string(rune('a' + i))}})
	}

	if s.Shuffled() {
		t.Fatal("fresh state must not report shuffled")
	}

	s.Shuffle()
	if !s.Shuffled() {
		t.Fatal("Shuffle must set the shuffled flag")
	}

	// Enqueue after shuffle leaves the flag up: queue stays shuffle-descended.
	s.Enqueue(lavalink.Track{Info: lavalink.TrackInfo{Identifier: "z"}})
	if !s.Shuffled() {
		t.Fatal("Enqueue must not reset the shuffled flag")
	}

	// Surgical edits don't touch it.
	s.SwapTracks(0, 1)
	if !s.Shuffled() {
		t.Fatal("SwapTracks must not reset the shuffled flag")
	}

	// Only a full clear resets (and /stop routes through ClearQueue via Halt).
	s.ClearQueue()
	if s.Shuffled() {
		t.Fatal("ClearQueue must reset the shuffled flag")
	}
}
