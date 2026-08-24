// Package metrics provides lightweight Prometheus-compatible counters and
// an HTTP endpoint for scraping. Zero external dependencies.
package metrics

import (
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Metrics tracks bot-level counters using atomic operations.
type Metrics struct {
	mu        sync.Mutex
	startTime time.Time
	counters  map[string]*counter
}

type counter struct {
	name   string
	help   string
	value  int64
	labels map[string]string // optional static labels
}

// New creates a Metrics instance with the standard counters registered.
func New() *Metrics {
	m := &Metrics{
		startTime: time.Now(),
		counters:  make(map[string]*counter),
	}
	m.counter("hex_music_bot_tracks_played", "Total tracks started")
	m.counter("hex_music_bot_tracks_ended", "Total tracks finished naturally")
	m.counter("hex_music_bot_tracks_failed", "Tracks that threw exceptions")
	m.counter("hex_music_bot_failovers_attempted", "Alternate-source failover attempts")
	m.counter("hex_music_bot_failovers_succeeded", "Successful failovers")
	m.counter("hex_music_bot_skips", "Total skip commands/button presses")
	m.counter("hex_music_bot_card_edits", "Player card REST edits sent")
	m.counter("hex_music_bot_commands_total", "Slash commands executed")
	m.counter("hex_music_bot_autoplay_enqueued", "Tracks enqueued by autoplay")
	m.counter("hex_music_bot_cooldown_denies", "Commands and buttons blocked by cooldown")
	return m
}

func (m *Metrics) counter(name, help string) {
	m.counters[name] = &counter{name: name, help: help}
}

// Inc increments a named counter by 1.
func (m *Metrics) Inc(name string) {
	m.Add(name, 1)
}

// Add increments a named counter by n.
func (m *Metrics) Add(name string, n int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c, ok := m.counters[name]; ok {
		c.value += n
	}
}

// Handler returns an http.HandlerFunc that serves Prometheus text format.
func (m *Metrics) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		var sb strings.Builder

		sb.WriteString("# TYPE hex_music_bot_uptime_seconds gauge\n")
		sb.WriteString(fmt.Sprintf("hex_music_bot_uptime_seconds %d\n",
			int64(time.Since(m.startTime).Seconds())))

		names := make([]string, 0, len(m.counters))
		for name := range m.counters {
			names = append(names, name)
		}
		sort.Strings(names)

		for _, name := range names {
			c := m.counters[name]
			fmt.Fprintf(&sb, "# HELP %s %s\n# TYPE %s counter\n%s %d\n",
				c.name, c.help, c.name, c.name, c.value)
		}
		m.mu.Unlock()

		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		if _, err := w.Write([]byte(sb.String())); err != nil {
			slog.Debug("metrics write failed", slog.Any("err", err))
		}
	}
}

// StartServer starts the metrics HTTP server on the given address in a
// goroutine. Returns immediately.
func (m *Metrics) StartServer(addr string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", m.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	go func() {
		slog.Info("metrics server listening", slog.String("addr", addr))
		if err := http.ListenAndServe(addr, mux); err != nil {
			slog.Error("metrics server failed", slog.Any("err", err))
		}
	}()
}
