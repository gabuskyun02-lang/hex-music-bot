package bot

import (
	"sync"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// cooldownLimits maps cooled commands (slash names and "btn:"+action) to
// their per-user cooldown. Commands absent from the map are unlimited.
var cooldownLimits = map[string]time.Duration{
	"play": 5 * time.Second, "search": 5 * time.Second, "playtop": 5 * time.Second,
	"skip": 2 * time.Second, "previous": 2 * time.Second, "skipto": 2 * time.Second,
	"filter":  3 * time.Second,
	"volume":  1 * time.Second,
	"shuffle": 2 * time.Second, "clear": 2 * time.Second, "move": 2 * time.Second,
	"swap": 2 * time.Second, "remove": 2 * time.Second,

	"btn:prev":    1500 * time.Millisecond,
	"btn:toggle":  1500 * time.Millisecond,
	"btn:skip":    1500 * time.Millisecond,
	"btn:loop":    1500 * time.Millisecond,
	"btn:shuffle": 1500 * time.Millisecond,
	"btn:voldown": 1500 * time.Millisecond,
	"btn:volup":   1500 * time.Millisecond,
	"btn:stop":    1500 * time.Millisecond,
}

const cooldownMaxLimit = 5 * time.Second

// CooldownGate enforces per-user command cooldowns. Entries are keyed
// "userID:command"; a janitor evicts stale ones so the map stays bounded.
type CooldownGate struct {
	mu     sync.Mutex
	limits map[string]time.Duration
	last   map[string]time.Time
}

// NewCooldownGate builds a gate and starts its janitor goroutine.
func NewCooldownGate() *CooldownGate {
	g := &CooldownGate{
		limits: cooldownLimits,
		last:   make(map[string]time.Time),
	}
	go g.janitor()
	return g
}

// Allow reports whether userID may invoke command now. On allow it records
// the invocation; on deny it returns the remaining wait.
func (g *CooldownGate) Allow(userID snowflake.ID, command string) (bool, time.Duration) {
	limit, ok := g.limits[command]
	if !ok {
		return true, 0
	}
	key := userID.String() + ":" + command

	now := time.Now()
	g.mu.Lock()
	defer g.mu.Unlock()
	if last, hit := g.last[key]; hit {
		if remaining := limit - now.Sub(last); remaining > 0 {
			return false, remaining
		}
	}
	g.last[key] = now
	return true, 0
}

// janitor periodically evicts entries older than the longest cooldown.
func (g *CooldownGate) janitor() {
	ticker := time.NewTicker(time.Minute)
	for range ticker.C {
		cutoff := time.Now().Add(-cooldownMaxLimit)
		g.mu.Lock()
		for key, last := range g.last {
			if last.Before(cutoff) {
				delete(g.last, key)
			}
		}
		g.mu.Unlock()
	}
}

// IsOwner reports whether userID is the configured bot owner.
func (b *Bot) IsOwner(userID snowflake.ID) bool {
	return b.Cfg != nil && b.Cfg.BotOwnerID != 0 && b.Cfg.BotOwnerID == userID
}
