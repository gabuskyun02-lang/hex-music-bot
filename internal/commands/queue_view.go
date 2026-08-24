package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/player"
	"hex-music-bot/internal/ui"
)

const queueDisplayLimit = 10

// Queue lists the queued tracks with loop-mode status.
func Queue(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	state := b.Player.Get(*event.GuildID())
	tracks := state.Snapshot()
	if len(tracks) == 0 {
		return b.ReplyEmbed(event, hexbot.InfoEmbed("📋 Queue", "The queue is empty — add something with `/play`"))
	}

	var total lavalink.Duration
	for _, t := range tracks {
		total += t.Info.Length
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Up next** (%d tracks)\n\n", len(tracks))
	for i, track := range tracks[:min(len(tracks), queueDisplayLimit)] {
		fmt.Fprintf(&sb, "`%d.` %s · `%s`\n", i+1, ui.TrackMarkdown(track), ui.FormatDuration(track.Info.Length))
	}
	if remaining := len(tracks) - queueDisplayLimit; remaining > 0 {
		fmt.Fprintf(&sb, "\n…and %d more", remaining)
	}

	status := []string{"🔁 Loop: `" + state.LoopMode().String() + "`"}
	if b.Cfg != nil {
		settings, _ := b.Store.GetGuildSettings(context.Background(), event.GuildID().String())
		if settings != nil && settings.Autoplay {
			status = append(status, "🔄 Autoplay")
		}
	}

	inlineTrue := true
	return b.ReplyEmbed(event, discord.Embed{
		Title:       "📋 Queue",
		Description: sb.String(),
		Color:       0x5865F2,
		Footer:      &discord.EmbedFooter{Text: strings.Join(status, " • ")},
		Fields: []discord.EmbedField{
			{Name: "Total runtime", Value: "`" + ui.FormatDuration(total) + "`", Inline: &inlineTrue},
		},
	})
}

// NowPlaying reports the current track with position and length as an embed.
func NowPlaying(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	track := *player.Track
	badge := ui.SourceBadgeFor(track.Info.SourceName)
	status := "▶ Now Playing"
	if player.Paused {
		status = "⏸ Paused"
	}
	status = badge.Emoji + " " + status

	pos, total := player.Position(), track.Info.Length
	progress := ui.ProgressBar(pos, total, 18)

	inlineTrue := true
	embed := discord.Embed{
		Title:       status,
		Description: fmt.Sprintf("**%s**\n%s", ui.TrackMarkdown(track), track.Info.Author),
		Color:       badge.Color,
		Fields: []discord.EmbedField{
			{Name: "Progress", Value: progress},
			{Name: "Source", Value: badge.Emoji + " `" + track.Info.SourceName + "`", Inline: &inlineTrue},
			{Name: "Volume", Value: fmt.Sprintf("`%d`", player.Volume), Inline: &inlineTrue},
			{Name: "Loop", Value: "`" + b.Player.Get(*event.GuildID()).LoopMode().String() + "`", Inline: &inlineTrue},
			{Name: "Queue", Value: fmt.Sprintf("`%d` tracks", b.Player.Get(*event.GuildID()).Len()), Inline: &inlineTrue},
		},
	}
	if track.Info.ArtworkURL != nil && *track.Info.ArtworkURL != "" {
		embed.Thumbnail = &discord.EmbedResource{URL: *track.Info.ArtworkURL}
	}

	return b.ReplyEmbed(event, embed)
}

// Shuffle randomizes the queue order via the shared action.
func Shuffle(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to shuffle"))
	}
	b.ShuffleQueue(*event.GuildID())
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Queue shuffled 🔀"))
}

// Loop switches the guild's loop mode via the shared action.
func Loop(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to change loop mode"))
	}
	mode, ok := player.ParseLoopMode(data.String("mode"))
	if !ok {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Unknown loop mode"))
	}
	b.Player.Get(*event.GuildID()).SetLoopMode(mode)
	b.Cards.Refresh(*event.GuildID())

	label := map[player.LoopMode]string{
		player.LoopOff:   "🔁 Off",
		player.LoopTrack: "🔂 Track",
		player.LoopQueue: "🔁 Queue",
	}[mode]
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Loop set to **"+label+"**"))
}
