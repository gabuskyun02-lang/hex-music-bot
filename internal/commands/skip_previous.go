package commands

import (
	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/ui"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

)

// Skip drops ahead in the queue and starts what follows via the shared action.
func Skip(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to skip tracks"))
	}
	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}

	amount := 1
	if v, ok := data.OptInt("amount"); ok {
		amount = v
	}
	state := b.Player.Get(guildID)
	state.Drop(amount - 1)

	b.Metrics.Inc("hex_music_bot_skips")
	next, stopped := b.SkipNext(guildID)
	b.Cards.Refresh(guildID)
	if stopped {
		return b.ReplyEmbed(event, hexbot.SuccessEmbed("Skipped — the queue is empty"))
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Skipped to "+ui.TrackMarkdown(*next)))
}

// Previous replays the most recently finished track via the shared action.
func Previous(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	prev, noHistory := b.ReplayPrevious(*event.GuildID())
	b.Cards.Refresh(*event.GuildID())
	if noHistory {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("No previous track yet"))
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Rewound to "+ui.TrackMarkdown(*prev)))
}
