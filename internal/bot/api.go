// Read-only status API served on the metrics listener: one JSON snapshot of
// guild count, playback, and lavalink node health for external dashboards.
package bot

import (
	"encoding/json"
	"net/http"
	"time"
)

// appVersion matches the lyrics User-Agent ("hex-music-bot/0.1"); nothing in
// the repo defines a version today, so it lives here as a single const.
const appVersion = "0.1.0"

var startTime = time.Now()

type nodeStatus struct {
	Name     string `json:"name"`
	Status   string `json:"status"`
	RestDead bool   `json:"rest_dead"`
	Players  int    `json:"players"`
}

type statusPayload struct {
	Guilds        int          `json:"guilds"`
	ActivePlayers int          `json:"active_players"`
	UptimeSeconds int64        `json:"uptime_seconds"`
	Nodes         []nodeStatus `json:"nodes"`
	Version       string       `json:"version"`
}

// StatusHandler serves GET /api/v1/status as a read-only JSON snapshot.
func (b *Bot) StatusHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		payload := statusPayload{
			Guilds:        b.Client.Caches.GuildsLen(),
			UptimeSeconds: int64(time.Since(startTime).Seconds()),
			Version:       appVersion,
			Nodes:         []nodeStatus{},
		}
		for n := range b.Lavalink.Nodes() {
			name := n.Config.Name
			payload.Nodes = append(payload.Nodes, nodeStatus{
				Name:     name,
				Status:   string(n.Status()),
				RestDead: b.isNodeRestDead(name),
				Players:  n.Stats().Players,
			})
		}
		for p := range b.Lavalink.Players() {
			if p.Track != nil {
				payload.ActivePlayers++
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(payload)
	}
}
