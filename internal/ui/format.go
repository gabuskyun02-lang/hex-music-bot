// Package ui formats track and duration strings for Discord messages.
package ui

import (
	"fmt"

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
