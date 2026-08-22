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

type filterManager struct {
	mu      sync.Mutex
	b       *Bot
	filters map[snowflake.ID]*lavalink.Filters
	names   map[snowflake.ID][]string
}

func newFilterManager(b *Bot) *filterManager {
	return &filterManager{
		b:       b,
		filters: make(map[snowflake.ID]*lavalink.Filters),
		names:   make(map[snowflake.ID][]string),
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
		delete(fm.filters, guildID)
		delete(fm.names, guildID)
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
	filters = *preset

	fm.mu.Lock()
	existing := fm.filters[guildID]
	if existing != nil {
		filters.Volume = existing.Volume
	}
	fm.filters[guildID] = &filters
	names := fm.names[guildID]
	found := false
	for _, n := range names {
		if n == name {
			found = true
			break
		}
	}
	if !found {
		fm.names[guildID] = append(names, name)
	}
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

// ActiveFilters returns the list of currently applied filter names for a guild.
func (fm *filterManager) ActiveFilters(guildID snowflake.ID) []string {
	fm.mu.Lock()
	defer fm.mu.Unlock()
	return fm.names[guildID]
}
