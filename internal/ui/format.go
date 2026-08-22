// Package ui formats track and duration strings for Discord messages.
package ui

import (
	"fmt"
	"strings"

	"github.com/disgoorg/disgolink/v4/lavalink"
)


// FormatDuration renders milliseconds as h:mm:ss or m:ss.
func FormatDuration(d lavalink.Duration) string {
	totalMinutes := d.Minutes()
	seconds := d.SecondsPart()
	if hours := totalMinutes / 60; hours > 0 {
		return fmt.Sprintf("%d:%02d:%02d", hours, totalMinutes%60, seconds)
	}
	return fmt.Sprintf("%d:%02d", totalMinutes, seconds)
}

// TrackMarkdown renders [`title`](<uri>), falling back to a bare title when
// the source provides no URI (some local/stream inputs).
func TrackMarkdown(track lavalink.Track) string {
	if track.Info.URI == nil {
		return fmt.Sprintf("`%s`", track.Info.Title)
	}
	return fmt.Sprintf("[`%s`](<%s>)", track.Info.Title, *track.Info.URI)
}

// ParseTimeStr accepts "HH:MM:SS", "MM:SS", or "SS" and returns milliseconds.
// Returns 0 on parse failure.
func ParseTimeStr(s string) int64 {
	parts := strings.Split(s, ":")
	var h, m, sec int64
	switch len(parts) {
	case 3:
		fmt.Sscanf(parts[0], "%d", &h)
		fmt.Sscanf(parts[1], "%d", &m)
		fmt.Sscanf(parts[2], "%d", &sec)
	case 2:
		fmt.Sscanf(parts[0], "%d", &m)
		fmt.Sscanf(parts[1], "%d", &sec)
	case 1:
		fmt.Sscanf(parts[0], "%d", &sec)
	default:
		return 0
	}
	return (h*3600 + m*60 + sec) * 1000
}
