package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

var filterPresets = map[string]func() *lavalink.Filters{
	"bassboost": func() *lavalink.Filters {
		eq := lavalink.Equalizer{}
		for i := range 3 {
			eq[i] = 0.35
		}
		return &lavalink.Filters{Equalizer: &eq}
	},
	"nightcore": func() *lavalink.Filters {
		return &lavalink.Filters{Timescale: &lavalink.Timescale{Rate: 1.25, Pitch: 1.25}}
	},
	"vaporwave": func() *lavalink.Filters {
		return &lavalink.Filters{Timescale: &lavalink.Timescale{Rate: 0.8, Pitch: 0.8}}
	},
	"8d": func() *lavalink.Filters {
		return &lavalink.Filters{Rotation: &lavalink.Rotation{RotationHz: 1}}
	},
	"tremolo": func() *lavalink.Filters {
		return &lavalink.Filters{Tremolo: &lavalink.Tremolo{Frequency: 4, Depth: 0.6}}
	},
	"vibrato": func() *lavalink.Filters {
		return &lavalink.Filters{Vibrato: &lavalink.Vibrato{Frequency: 4, Depth: 0.6}}
	},
	"karaoke": func() *lavalink.Filters {
		return &lavalink.Filters{Karaoke: &lavalink.Karaoke{Level: 1, MonoLevel: 1}}
	},
	"lowpass": func() *lavalink.Filters {
		return &lavalink.Filters{LowPass: &lavalink.LowPass{Smoothing: 20}}
	},
}

type guildFilters struct {
	composite lavalink.Filters
	// owners maps each filter field to the preset name that set it.
	// A missing entry means no active preset owns that field.
	owners map[string]string
}

type filterManager struct {
	mu     sync.Mutex
	b      *Bot
	states map[snowflake.ID]*guildFilters
}

func newFilterManager(b *Bot) *filterManager {
	return &filterManager{
		b:      b,
		states: make(map[snowflake.ID]*guildFilters),
	}
}

// SetFilter applies a named filter preset to the guild's player.
// Preset "reset" clears all filters.
func (fm *filterManager) SetFilter(guildID snowflake.ID, name string) error {
	p := fm.b.Lavalink.ExistingPlayer(guildID)
	if p == nil || p.Track == nil {
		return fmt.Errorf("nothing is playing")
	}
	name = strings.ToLower(name)

	var filters lavalink.Filters
	switch name {
	case "reset", "off":
		fm.mu.Lock()
		delete(fm.states, guildID)
		fm.mu.Unlock()
		return p.Update(context.TODO(), disgolink.WithFilters(filters))
	case "clear":
		return fm.SetFilter(guildID, "reset")
	}

	fn, ok := filterPresets[name]
	if !ok {
		return fmt.Errorf("unknown filter %q — use: bassboost, nightcore, vaporwave, 8d, tremolo, vibrato, karaoke, lowpass, reset", name)
	}

	preset := fn()

	fm.mu.Lock()
	state := fm.states[guildID]
	if state == nil {
		state = &guildFilters{owners: make(map[string]string)}
		fm.states[guildID] = state
	}
	for _, old := range mergePreset(&state.composite, name, preset, state.owners) {
		slog.Debug("filter evicted",
			slog.String("guild", guildID.String()),
			slog.String("filter", old),
		)
	}
	filters = state.composite
	fm.mu.Unlock()

	if err := p.Update(context.TODO(), disgolink.WithFilters(filters)); err != nil {
		return err
	}
	fm.b.Cards.Refresh(guildID)
	slog.Info("filter applied",
		slog.String("guild", guildID.String()),
		slog.String("filter", name),
	)
	return nil
}

// mergePreset copies each non-nil preset field into the guild's composite,
// recording `name` as its owner. A field already owned by another preset is
// overwritten (latest wins) and the loser's name is returned for eviction.
func mergePreset(composite *lavalink.Filters, name string, preset *lavalink.Filters, owners map[string]string) (evicted []string) {
	evict := func(field string) {
		if prev, ok := owners[field]; ok && prev != name {
			evicted = append(evicted, prev)
		}
		owners[field] = name
	}
	if preset.Volume != nil {
		evict("volume")
		composite.Volume = preset.Volume
	}
	if preset.Equalizer != nil {
		evict("equalizer")
		composite.Equalizer = preset.Equalizer
	}
	if preset.Timescale != nil {
		evict("timescale")
		composite.Timescale = preset.Timescale
	}
	if preset.Tremolo != nil {
		evict("tremolo")
		composite.Tremolo = preset.Tremolo
	}
	if preset.Vibrato != nil {
		evict("vibrato")
		composite.Vibrato = preset.Vibrato
	}
	if preset.Rotation != nil {
		evict("rotation")
		composite.Rotation = preset.Rotation
	}
	if preset.Karaoke != nil {
		evict("karaoke")
		composite.Karaoke = preset.Karaoke
	}
	if preset.LowPass != nil {
		evict("lowpass")
		composite.LowPass = preset.LowPass
	}
	return evicted
}

// ActiveFilters returns the names of presets that still own at least one
// filter field for a guild, in application order.
func (fm *filterManager) ActiveFilters(guildID snowflake.ID) []string {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	state := fm.states[guildID]
	if state == nil {
		return nil
	}
	var active []string
	seen := make(map[string]bool)
	for _, field := range filterFields {
		owner, ok := state.owners[field]
		if !ok || seen[owner] {
			continue
		}
		seen[owner] = true
		active = append(active, owner)
	}
	return active
}

// filterFields lists every Lavalink filter field mergePreset tracks.
var filterFields = []string{
	"volume", "equalizer", "timescale", "tremolo", "vibrato",
	"rotation", "karaoke", "lowpass",
}
