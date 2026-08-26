package bot

import (
	"context"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
)

// OnAutocomplete serves live /search and /play suggestions. Chosen values
// carry the full track URI, so /play re-resolves exactly — no server-side
// cache needed.
func (b *Bot) OnAutocomplete(event *events.AutocompleteInteractionCreate) {
	data := event.Data
	// Exact match: a HasSuffix("/play") would also catch "/playlist play",
	// firing track searches for its share-code option.
	path := data.CommandPath()
	if path != "/search" && path != "/play" {
		_ = event.Acknowledge()
		return
	}
	query := strings.TrimSpace(data.String("identifier"))
	if err := event.AutocompleteResult(b.searchSuggestions(query)); err != nil {
		slog.Debug("autocomplete respond failed", slog.Any("err", err))
	}
}

func (b *Bot) searchSuggestions(query string) []discord.AutocompleteChoice {
	if utf8.RuneCountInString(query) < 3 {
		return []discord.AutocompleteChoice{}
	}
	node := b.BestHealthyNode()
	if node == nil {
		return []discord.AutocompleteChoice{}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	var tracks []lavalink.Track
	node.Rest.LoadTracksHandler(ctx, lavalink.SearchTypeYouTube.Apply(query), disgolink.NewTrackLoadingResultHandler(
		func(track lavalink.Track) { tracks = []lavalink.Track{track} },
		func(playlist lavalink.Playlist) { tracks = playlist.Tracks },
		func(list []lavalink.Track) { tracks = list },
		func() {},
		func(err error) { slog.Debug("search suggest failed", slog.Any("err", err)) },
	))

	if len(tracks) > 5 {
		tracks = tracks[:5]
	}
	choices := make([]discord.AutocompleteChoice, 0, len(tracks))
	for _, t := range tracks {
		choices = append(choices, discord.AutocompleteChoiceString{
			Name:  truncateRunes(t.Info.Title, 100),
			Value: derefString(t.Info.URI),
		})
	}
	return choices
}

func truncateRunes(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	return string(runes[:max-1]) + "…"
}
