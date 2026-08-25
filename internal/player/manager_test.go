package player

import (
	"testing"

	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

func TestQueueCap(t *testing.T) {
	tr := func(id string) lavalink.Track {
		return lavalink.Track{Info: lavalink.TrackInfo{Identifier: id}}
	}

	s := NewManager(3).Get(1)
	added, rejected := s.EnqueueAs(42, tr("a"), tr("b"), tr("c"), tr("d"), tr("e"))
	if added != 3 || rejected != 2 {
		t.Fatalf("cap 3: want added=3 rejected=2, got added=%d rejected=%d", added, rejected)
	}
	if s.Len() != 3 {
		t.Fatalf("queue must hold exactly cap, got %d", s.Len())
	}

	// InsertAtAs refuses at cap, works once room frees up.
	if s.InsertAtAs(0, tr("z"), 42) {
		t.Fatal("InsertAtAs must fail at cap")
	}
	s.Drop(1)
	if added, rejected = s.Enqueue(tr("f")); added != 1 || rejected != 0 {
		t.Fatalf("one slot free after Drop: want added=1 rejected=0, got added=%d rejected=%d", added, rejected)
	}

	// Single track under cap unchanged.
	s2 := NewManager(3).Get(2)
	if added, rejected = s2.EnqueueAs(7, tr("solo")); added != 1 || rejected != 0 {
		t.Fatalf("under-cap enqueue untouched: got added=%d rejected=%d", added, rejected)
	}

	// maxQueue 0 means unlimited.
	s3 := NewManager(0).Get(3)
	added, rejected = 0, 0
	for i := 0; i < 10; i++ {
		a, r := s3.Enqueue(tr(string(rune('a' + i))))
		added, rejected = added+a, rejected+r
	}
	if added != 10 || rejected != 0 {
		t.Fatalf("uncapped queue: want added=10 rejected=0, got added=%d rejected=%d", added, rejected)
	}
}

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
