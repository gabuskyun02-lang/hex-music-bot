package commands

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/ui"
)

var (
	urlPattern    = regexp.MustCompile(`^https?://[-a-zA-Z0-9+&@#/%?=~_|!:,.;]*[-a-zA-Z0-9+&@#/%=~_|]?`)
	searchPattern = regexp.MustCompile(`^(.{2})search:(.+)`)
)

// Play resolves an identifier, joins voice if idle, and starts playback.
// When something is already playing, new tracks queue behind it instead of
// cutting it off; playlists enqueue everything after the opener.
func Play(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	identifier := data.String("identifier")
	if source, ok := data.OptString("source"); ok {
		identifier = lavalink.SearchType(source).Apply(identifier)
	} else if !urlPattern.MatchString(identifier) && !searchPattern.MatchString(identifier) {
		identifier = lavalink.SearchTypeYouTube.Apply(identifier)
	}

	guildID := *event.GuildID()
	voiceState, ok := b.Client.Caches.VoiceState(guildID, event.User().ID)
	if !ok {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need to be in a voice channel first"))
	}
	if err := event.DeferCreateMessage(false); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var (
		toPlay   *lavalink.Track
		enqueued []lavalink.Track
	)
	node := b.Lavalink.BestNode()
	if node == nil {
		return b.EditReply(event, "Lavalink node not connected — try again in a moment")
	}
	node.Rest.LoadTracksHandler(ctx, identifier, disgolink.NewTrackLoadingResultHandler(
		func(track lavalink.Track) {
			toPlay = &track
		},
		func(playlist lavalink.Playlist) {
			if len(playlist.Tracks) > 0 {
				toPlay = &playlist.Tracks[0]
				enqueued = playlist.Tracks[1:]
			}
		},
		func(tracks []lavalink.Track) {
			toPlay = &tracks[0]
		},
		func() {},
		func(err error) {},
	))
	if toPlay == nil {
		return b.EditReply(event, fmt.Sprintf("Nothing found for `%s`", identifier))
	}

	queue := b.Player.Get(guildID)
	settings, _ := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if settings != nil && !settings.AllowDuplicate {
		// Check toPlay against queue + current track
		dupes := queue.HasDuplicate(toPlay.Info.Title)
		for _, t := range enqueued {
			if !dupes && queue.HasDuplicate(t.Info.Title) {
				dupes = true
			}
		}
		if p := b.Lavalink.ExistingPlayer(guildID); p != nil && p.Track != nil {
			if strings.EqualFold(p.Track.Info.Title, toPlay.Info.Title) {
				dupes = true
			}
		}
		if dupes {
			return b.EditReply(event, "⛔ Duplicate tracks are blocked on this server")
		}
	}
	player := b.Lavalink.ExistingPlayer(guildID)
	if player != nil && player.Track != nil {
		queue.EnqueueAs(event.User().ID, *toPlay)
		queue.EnqueueAs(event.User().ID, enqueued...)
		message := fmt.Sprintf("Added %s to the queue", ui.TrackMarkdown(*toPlay))
		if len(enqueued) > 0 {
			message += fmt.Sprintf(" (+%d playlist tracks)", len(enqueued))
		}
		return b.EditReply(event, message)
	}
	queue.EnqueueAs(event.User().ID, enqueued...)

	if err := b.Client.UpdateVoiceState(context.TODO(), guildID, voiceState.ChannelID, false, false); err != nil {
		return err
	}
	if err := b.Lavalink.Player(guildID).Update(ctx, disgolink.WithTrack(*toPlay)); err != nil {
		return err
	}
	queue.SetCurrentRequester(event.User().ID)
	b.Cards.Create(guildID, event.Channel().ID())

	message := fmt.Sprintf("Playing %s", ui.TrackMarkdown(*toPlay))
	if remaining := queue.Len(); remaining > 0 {
		message += fmt.Sprintf(" — %d track(s) queued", remaining)
	}
	return b.EditReply(event, message)
}
