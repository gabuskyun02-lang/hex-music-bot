package commands

import (
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

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
		return b.Reply(event, "The queue is empty")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "**Up next** (%d tracks) — loop: `%s`\n", len(tracks), state.LoopMode())
	for i, track := range tracks[:min(len(tracks), queueDisplayLimit)] {
		fmt.Fprintf(&sb, "%d. %s\n", i+1, ui.TrackMarkdown(track))
	}
	if remaining := len(tracks) - queueDisplayLimit; remaining > 0 {
		fmt.Fprintf(&sb, "...and %d more", remaining)
	}
	return b.Reply(event, sb.String())
}

// NowPlaying reports the current track with position and length.
func NowPlaying(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.Reply(event, "Nothing is playing")
	}
	track := *player.Track
	status := "playing"
	if player.Paused {
		status = "paused"
	}
	return b.Reply(event, fmt.Sprintf("%s %s\n`%s / %s`",
		status,
		ui.TrackMarkdown(track),
		ui.FormatDuration(player.Position()),
		ui.FormatDuration(track.Info.Length),
	))
}

// Shuffle randomizes the queue order via the shared action.
func Shuffle(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	b.ShuffleQueue(*event.GuildID())
	return b.Reply(event, "Queue shuffled")
}

// Loop switches the guild's loop mode via the shared action.
func Loop(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	mode, ok := player.ParseLoopMode(data.String("mode"))
	if !ok {
		return b.Reply(event, "Unknown loop mode")
	}
	b.Player.Get(*event.GuildID()).SetLoopMode(mode)
	b.Cards.Refresh(*event.GuildID())
	return b.Reply(event, "Loop set to `"+mode.String()+"`")
}

