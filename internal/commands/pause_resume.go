package commands

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	hexbot "hex-music-bot/internal/bot"
)

// Pause pauses playback via the shared action.
func Pause(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	paused, hadPlayback := b.SetPaused(*event.GuildID(), true)
	if !hadPlayback {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	if !paused {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Already paused"))
	}
	b.Cards.Refresh(*event.GuildID())
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("⏸ Paused"))
}

// Resume continues a paused player via the shared action.
func Resume(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	paused, hadPlayback := b.SetPaused(*event.GuildID(), false)
	if !hadPlayback {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	if paused {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Already playing"))
	}
	b.Cards.Refresh(*event.GuildID())
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("▶ Resumed"))
}
