package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/lyrics"
)

const (
	lyricsPageSize  = 1700 // runes per page, under Discord's 2000 content cap
	lyricsMaxPages  = 6
	lyricsFetchTime = 10 * time.Second
)

// Lyrics resolves LRCLIB lyrics for the current track and replies with a
// paginated session (◀ ▶ buttons). Synced lyrics keep [mm:ss] timestamps.
func Lyrics(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	p := b.Lavalink.ExistingPlayer(*event.GuildID())
	if p == nil || p.Track == nil {
		return b.Reply(event, "Nothing is playing")
	}
	track := *p.Track

	artist, title := lyrics.SplitTitle(track.Info.Title)
	if artist == "" {
		artist = track.Info.Author
	}

	if err := event.DeferCreateMessage(true); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), lyricsFetchTime)
	defer cancel()

	res, err := lyrics.Fetch(ctx, nil, artist, title)
	if err != nil {
		return b.EditReply(event, fmt.Sprintf("Lyrics lookup failed: `%v`", err))
	}
	if res == nil {
		return b.EditReply(event, fmt.Sprintf("No lyrics found for `%s`", track.Info.Title))
	}

	pages := buildLyricPages(res, track.Info.Title)
	if len(pages) > lyricsMaxPages {
		pages = pages[:lyricsMaxPages]
		pages[lyricsMaxPages-1] += "\n…truncated"
	}

	token := event.Token()
	doc := &hexbot.LyricDoc{Token: token, Pages: pages, Page: 0}
	b.Lyrics.Put(doc)

	components := []discord.LayoutComponent{hexbot.LyricButtons(doc)}
	_, err = b.Client.Rest.CreateMessage(event.Channel().ID(), discord.MessageCreate{
		Content:    pages[0],
		Components: components,
	})
	if err != nil {
		return err
	}
	// The interaction itself stays as an ephemeral "thinking" ack; the real
	// message above is the persistent paginated one.
	return b.EditReply(event, fmt.Sprintf("Lyrics: **%s — %s** ↑", res.Artist, res.Title))
}

// buildLyricPages chunks lyrics text into rune-safe pages.
func buildLyricPages(res *lyrics.Lyrics, fallbackTitle string) []string {
	var text string
	if len(res.Synced) > 0 {
		var sb strings.Builder
		for _, ln := range res.Synced {
			display := ln.Text
			if display == "" {
				display = "♪"
			}
			fmt.Fprintf(&sb, "`[%02d:%02d]` %s\n",
				int(ln.At.Minutes()), int(ln.At.Seconds())%60, display)
		}
		text = sb.String()
	} else {
		text = res.Plain
	}
	if strings.TrimSpace(text) == "" {
		return []string{fmt.Sprintf("No lyrics content for `%s`", fallbackTitle)}
	}

	var pages []string
	var current strings.Builder
	count := 0
	for _, line := range strings.SplitAfter(text, "\n") {
		lineLen := len([]rune(line))
		if count+lineLen > lyricsPageSize && count > 0 {
			pages = append(pages, strings.TrimRight(current.String(), "\n"))
			current.Reset()
			count = 0
		}
		current.WriteString(line)
		count += lineLen
	}
	if current.Len() > 0 {
		pages = append(pages, strings.TrimRight(current.String(), "\n"))
	}
	if len(pages) == 0 {
		pages = []string{strings.TrimSpace(text)}
	}
	return pages
}

