// Package lyrics resolves synced and plain lyrics from LRCLIB and parses
// messy player titles into (artist, title) pairs.
package lyrics

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const apiBase = "https://lrclib.net/api"

const userAgent = "hex-music-bot/0.1 (self-hosted Discord music bot)"

// Line is one synced lyric line.
type Line struct {
	At   time.Duration
	Text string
}

// Lyrics is a resolved lyrics document; exactly one of Synced/Plain is set.
type Lyrics struct {
	Artist string
	Title  string
	Synced []Line
	Plain  string
}

type apiResult struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	SyncedLyrics *string `json:"syncedLyrics"`
	PlainLyrics  *string `json:"plainLyrics"`
}

// Fetch resolves lyrics for artist/title. Returns (nil, nil) when nothing
// matches — callers distinguish "not found" from failure.
func Fetch(ctx context.Context, client *http.Client, artist, title string) (*Lyrics, error) {
	if client == nil {
		client = http.DefaultClient
	}
	try := func(query url.Values) ([]apiResult, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiBase+"/search?"+query.Encode(), nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", userAgent)
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("lrclib: status %d", resp.StatusCode)
		}
		var out []apiResult
		if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
			return nil, err
		}
		return out, nil
	}

	q := url.Values{}
	if artist != "" {
		q.Set("artist_name", artist)
	}
	q.Set("track_name", title)
	results, err := try(q)
	if err != nil {
		return nil, err
	}
	if len(results) == 0 && artist != "" {
		fallback := url.Values{}
		fallback.Set("q", artist+" "+title)
		results, err = try(fallback)
		if err != nil {
			return nil, err
		}
	}

	for _, r := range results {
		if r.SyncedLyrics != nil && *r.SyncedLyrics != "" {
			return &Lyrics{Artist: r.ArtistName, Title: r.TrackName, Synced: ParseLRC(*r.SyncedLyrics)}, nil
		}
	}
	for _, r := range results {
		if r.PlainLyrics != nil && *r.PlainLyrics != "" {
			return &Lyrics{Artist: r.ArtistName, Title: r.TrackName, Plain: *r.PlainLyrics}, nil
		}
	}
	return nil, nil
}

var lrcLine = regexp.MustCompile(`^\[(\d{1,2}):(\d{2}(?:\.\d{1,3})?)\]\s*(.*)$`)

// ParseLRC converts LRC text into sorted timestamped lines.
func ParseLRC(raw string) []Line {
	rawLines := strings.Split(raw, "\n")
	out := make([]Line, 0, len(rawLines))
	for _, ln := range rawLines {
		m := lrcLine.FindStringSubmatch(strings.TrimRight(ln, "\r"))
		if m == nil {
			continue
		}
		minutes, _ := strconv.Atoi(m[1])
		seconds, _ := strconv.ParseFloat(m[2], 64)
		text := strings.TrimSpace(m[3])
		out = append(out, Line{
			At:   time.Duration(minutes)*time.Minute + time.Duration(seconds*float64(time.Second)),
			Text: text,
		})
	}
	return out
}

var (
	parensRe = regexp.MustCompile(`\([^)]*\)`)
	bracksRe = regexp.MustCompile(`\[[^\]]*\]`)
	spaceRe  = regexp.MustCompile(`\s+`)
)

// SplitTitle extracts (artist, title) from messy stream titles like
// "Artist - Title (Official Video)". Falls back to ("", cleaned title).
func SplitTitle(raw string) (string, string) {
	s := parensRe.ReplaceAllString(raw, " ")
	s = bracksRe.ReplaceAllString(s, " ")
	s = spaceRe.ReplaceAllString(strings.TrimSpace(s), " ")
	for _, sep := range []string{" - ", " – ", " | "} {
		if idx := strings.Index(s, sep); idx > 0 {
			return strings.TrimSpace(s[:idx]), strings.TrimSpace(s[idx+len(sep):])
		}
	}
	return "", s
}
