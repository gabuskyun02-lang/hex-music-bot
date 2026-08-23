package bot

import (
	"context"
	"log/slog"
	"time"

	"github.com/disgoorg/disgolink/v4/disgolink"
)

// MarkNodeRestDead flags a node whose REST API just failed, so playback
// selection skips it while its websocket may still look "connected".
func (b *Bot) MarkNodeRestDead(nodeName string) {
	b.restDead.Store(nodeName, time.Now())
}

// ClearNodeRestDead clears a node's REST-failure flag after a successful
// REST round-trip.
func (b *Bot) ClearNodeRestDead(nodeName string) {
	b.restDead.Delete(nodeName)
}

// isNodeRestDead reports whether the node failed REST recently (within an
// hour — enough to route around a flaky host without permanent exile).
func (b *Bot) isNodeRestDead(nodeName string) bool {
	v, ok := b.restDead.Load(nodeName)
	if !ok {
		return false
	}
	return time.Since(v.(time.Time)) < time.Hour
}

// BestHealthyNode returns the lowest-load connected node that has not
// recently failed a REST call.
func (b *Bot) BestHealthyNode() *disgolink.Node {
	var best *disgolink.Node
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		if n.Status() != disgolink.StatusConnected || b.isNodeRestDead(n.Config.Name) {
			return true
		}
		if best == nil || n.Stats().Better(best.Stats()) {
			best = n
		}
		return true
	})
	if best == nil {
		// Everything flagged dead (or nothing connected) — fall back so
		// playback attempts still happen instead of hard-failing.
		return b.Lavalink.BestNode()
	}
	return best
}

// ProbeNodeHealth does one cheap REST call on every node and updates the
// health map. Called at startup; /ping refreshes it too.
func (b *Bot) ProbeNodeHealth() {
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		if _, err := n.Rest.Info(ctx); err != nil {
			b.MarkNodeRestDead(n.Config.Name)
			slog.Warn("node REST probe failed",
				slog.String("node", n.Config.Name),
				slog.Any("err", err),
			)
		} else {
			b.ClearNodeRestDead(n.Config.Name)
		}
		return true
	})
}
