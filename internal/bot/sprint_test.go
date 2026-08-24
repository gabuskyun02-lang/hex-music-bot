package bot

import (
	"testing"
	"time"

	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

func TestCooldownGate(t *testing.T) {
	g := &CooldownGate{limits: cooldownLimits, last: map[string]time.Time{}}
	u := snowflake.ID(1)

	if ok, _ := g.Allow(u, "play"); !ok {
		t.Fatal("first play must pass")
	}
	if ok, rem := g.Allow(u, "play"); ok || rem <= 0 || rem > 5*time.Second {
		t.Fatalf("second play must deny with remaining in (0,5s], got ok=%v rem=%v", ok, rem)
	}
	if ok, _ := g.Allow(snowflake.ID(2), "play"); !ok {
		t.Fatal("different user must pass (per-user gate)")
	}
	if ok, _ := g.Allow(u, "ping"); !ok {
		t.Fatal("absent command is unlimited")
	}
	if ok, _ := g.Allow(u, "voteskip"); !ok {
		t.Fatal("voteskip never cooled")
	}
	if ok, _ := g.Allow(u, "btn:toggle"); !ok {
		t.Fatal("button first click passes")
	}
	if _, rem := g.Allow(u, "btn:toggle"); rem > 1500*time.Millisecond {
		t.Fatalf("button remaining %v exceeds 1.5s limit", rem)
	}
}

func TestMergePresetLatestWins(t *testing.T) {
	preset := func(name string) *lavalink.Filters {
		return filterPresets[name]()
	}
	composite := lavalink.Filters{}
	owners := map[string]string{}

	if evicted := mergePreset(&composite, "bassboost", preset("bassboost"), owners); len(evicted) != 0 {
		t.Fatalf("fresh apply must not evict: %v", evicted)
	}
	if evicted := mergePreset(&composite, "nightcore", preset("nightcore"), owners); len(evicted) != 0 {
		t.Fatalf("non-conflicting apply must not evict: %v", evicted)
	}
	if composite.Equalizer == nil || composite.Timescale == nil {
		t.Fatal("composite must hold both Equalizer and Timescale")
	}

	evicted := mergePreset(&composite, "vaporwave", preset("vaporwave"), owners)
	if len(evicted) != 1 || evicted[0] != "nightcore" {
		t.Fatalf("vaporwave must evict nightcore from timescale, got %v", evicted)
	}
	if composite.Timescale.Rate != 0.8 {
		t.Fatalf("timescale must be latest-wins (vaporwave), got %v", composite.Timescale)
	}
}

func TestActiveFiltersLifecycle(t *testing.T) {
	fm := newFilterManager(&Bot{})
	g := snowflake.ID(99)

	fm.states[g] = &guildFilters{owners: map[string]string{}}
	st := fm.states[g]
	mergePreset(&st.composite, "bassboost", filterPresets["bassboost"](), st.owners)
	mergePreset(&st.composite, "nightcore", filterPresets["nightcore"](), st.owners)

	active := fm.ActiveFilters(g)
	if len(active) != 2 {
		t.Fatalf("expected bassboost+nightcore active, got %v", active)
	}

	// vaporwave conflicts with nightcore on timescale -> nightcore evicted.
	mergePreset(&st.composite, "vaporwave", filterPresets["vaporwave"](), st.owners)
	active = fm.ActiveFilters(g)
	if len(active) != 2 || active[0] != "bassboost" || active[1] != "vaporwave" {
		t.Fatalf("nightcore should be evicted, got %v", active)
	}

	delete(fm.states, g) // reset path
	if got := fm.ActiveFilters(g); got != nil {
		t.Fatalf("reset must clear actives, got %v", got)
	}
}

func TestCooldownGateStop(t *testing.T) {
	g := NewCooldownGate()
	// Janitor is running — Stop must shut it down without panic.
	g.Stop()
	// Double-stop must be safe.
	g.Stop()
	// Allow still works after stop (gate itself is still usable, just no janitor).
	if ok, _ := g.Allow(snowflake.ID(1), "play"); !ok {
		t.Fatal("Allow must still work after Stop")
	}
}
