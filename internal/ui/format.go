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

// SourceBadge is the per-source emoji + brand accent color, following the
// convention used by lucky/beatra (MIT).
type SourceBadge struct {
	Emoji string
	Color int
}

var sourceBadges = map[string]SourceBadge{
	"youtube":      {Emoji: "🔴", Color: 0xFF0000},
	"youtubemusic": {Emoji: "🔴", Color: 0xFF0000},
	"spotify":      {Emoji: "🟢", Color: 0x1DB954},
	"soundcloud":   {Emoji: "🟠", Color: 0xFF5500},
	"applemusic":   {Emoji: "🍎", Color: 0xFA2D48},
	"deezer":       {Emoji: "🟣", Color: 0xA238FF},
	"tidal":        {Emoji: "🔵", Color: 0x00B3E3},
	"http":         {Emoji: "🔗", Color: 0x5865F2},
}

var defaultBadge = SourceBadge{Emoji: "🎵", Color: 0x5865F2}

// SourceBadgeFor returns the badge for a Lavalink source name (lowercased
// match; unknown sources get a neutral default).
func SourceBadgeFor(sourceName string) SourceBadge {
	if b, ok := sourceBadges[strings.ToLower(sourceName)]; ok {
		return b
	}
	return defaultBadge
}

// ProgressBar renders a "`m:ss` ▬…🔘───… `m:ss`" progress line at the
// requested cell width.
func ProgressBar(pos, total lavalink.Duration, cells int) string {
	filled := 0
	if total > 0 {
		filled = int(float64(pos) / float64(total) * float64(cells))
	}
	filled = min(filled, cells)
	bar := strings.Repeat("▬", filled)
	if filled < cells {
		bar += "🔘" + strings.Repeat("─", cells-filled-1)
	} else {
		bar += "🔘"
	}
	return fmt.Sprintf("`%s` %s `%s`", FormatDuration(pos), bar, FormatDuration(total))
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
