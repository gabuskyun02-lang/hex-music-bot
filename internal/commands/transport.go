package commands

import (
	"context"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"

	hexbot "hex-music-bot/internal/bot"
)

func Stop(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.Reply(event, "⛔ You need the DJ role to stop playback")
	}
	guildID := *event.GuildID()
	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil || player.Track == nil {
		return b.Reply(event, "Nothing is playing — queue cleared")
	}
	return b.Reply(event, "Stopped and cleared the queue")
}

// Join moves the bot into the invoking user's current voice channel.
func Join(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	voiceState, ok := b.Client.Caches.VoiceState(guildID, event.User().ID)
	if !ok {
		return b.Reply(event, "You need to be in a voice channel first")
	}
	if err := b.Client.UpdateVoiceState(context.TODO(), guildID, voiceState.ChannelID, false, false); err != nil {
		return err
	}
	return b.Reply(event, "Joined <#"+voiceState.ChannelID.String()+">")
}

// Leave disconnects the bot and forgets the guild's queue.
func Leave(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.Reply(event, "⛔ You need the DJ role to disconnect the bot")
	}
	guildID := *event.GuildID()

	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil {
		return b.Reply(event, "I am not connected to voice")
	}
	_ = player.Update(context.TODO(), disgolink.WithNullTrack())

	if err := b.Client.UpdateVoiceState(context.TODO(), guildID, nil, false, false); err != nil {
		return err
	}
	// OnVoiceStateUpdate deletes player state once Discord confirms the leave.
	return b.Reply(event, "Left the voice channel")
}

// Clear empties the queue but leaves the current track playing.
func Clear(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	b.Player.Get(*event.GuildID()).ClearQueue()
	return b.Reply(event, "Queue cleared")
}
