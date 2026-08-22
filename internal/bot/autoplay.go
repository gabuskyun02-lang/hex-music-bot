package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

var keywordBlocklist = []string{
	"live", "mix", "podcast", "tutorial", "full album",
	"interview", "reaction", "1 hour", "10 hours",
	"compilation", "megamix", "karaoke",
}

const (
	minTrackMS int64 = 90 * 1000
	maxTrackMS int64 = 10 * 60 * 1000
)

// TryAutoplay checks if autoplay is enabled and fills the queue with
// related tracks when it drains.
func (b *Bot) TryAutoplay(guildID snowflake.ID) {
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if err != nil || !settings.Autoplay {
		return
	}

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()

		tracks := b.findAutoplayCandidates(ctx, guildID, settings.AutoplayLevel)
		if len(tracks) == 0 {
			return
		}
		state := b.Player.Get(guildID)
		for _, t := range tracks {
			state.Enqueue(t)
		}
		p := b.Lavalink.ExistingPlayer(guildID)
		if p != nil && p.Track == nil {
			next, ok := state.Next()
			if !ok {
				return
			}
			if err := p.Update(ctx, disgolink.WithTrack(next)); err != nil {
				slog.Error("autoplay start failed", slog.Any("err", err))
			}
		}
		b.Cards.Refresh(guildID)
		b.Metrics.Add("hex_music_bot_autoplay_enqueued", int64(len(tracks)))
		slog.Info("autoplay enqueued", slog.String("guild", guildID.String()), slog.Int("tracks", len(tracks)))
	}()
}

func (b *Bot) findAutoplayCandidates(ctx context.Context, guildID snowflake.ID, level string) []lavalink.Track {
	limit := map[string]int{"light": 3, "normal": 5, "aggressive": 8}[level]
	if limit == 0 {
		limit = 5
	}
	seedArtists, _ := b.Store.SeedArtists(ctx, guildID.String(), limit)
	listenerIDs := b.voice.ListenerIDs(guildID)
	blended, _ := b.Store.BlendedTasteArtists(ctx, listenerIDs)

	seenSeeds := make(map[string]bool, len(seedArtists)+len(blended))
	var seeds []string
	for _, a := range seedArtists {
		if seenSeeds[a] {
			continue
		}
		seenSeeds[a] = true
		seeds = append(seeds, a)
	}
	for _, t := range blended {
		if seenSeeds[t.Artist] {
			continue
		}
		seenSeeds[t.Artist] = true
		seeds = append(seeds, t.Artist)
	}
	if len(seeds) == 0 {
		return nil
	}

	node := b.Lavalink.BestNode()
	if node == nil {
		return nil
	}

	seen := make(map[string]bool)
	for _, t := range b.Player.Get(guildID).Snapshot() {
		seen[strings.ToLower(t.Info.Title)] = true
	}

	var out []lavalink.Track
	for _, artist := range seeds {
		if len(out) >= limit {
			break
		}
		query := fmt.Sprintf("ytmsearch:%s songs", artist)
		node.Rest.LoadTracksHandler(ctx, query, disgolink.NewTrackLoadingResultHandler(
			func(t lavalink.Track) {},
			func(p lavalink.Playlist) {},
			func(tracks []lavalink.Track) {
				for _, t := range tracks {
					if len(out) >= limit {
						return
					}
					if isGoodCandidate(t, seen) {
						seen[strings.ToLower(t.Info.Title)] = true
						out = append(out, t)
					}
				}
			},
			func() {},
			func(err error) { slog.Debug("autoplay search failed", slog.Any("err", err)) },
		))
	}
	return out
}

func isGoodCandidate(t lavalink.Track, seen map[string]bool) bool {
	titleLower := strings.ToLower(t.Info.Title)
	if seen[titleLower] {
		return false
	}
	length := int64(t.Info.Length)
	if length < minTrackMS || length > maxTrackMS {
		return false
	}
	for _, kw := range keywordBlocklist {
		if strings.Contains(titleLower, kw) {
			return false
		}
	}
	return true
}
