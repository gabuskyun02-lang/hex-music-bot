package bot

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgo/events"
)

// OnVoiceStateUpdate routes voice state changes: the bot's own go to
// Lavalink + tracking; other users' feed the auto-pause/resume watcher.
func (b *Bot) OnVoiceStateUpdate(event *events.GuildVoiceStateUpdate) {
	guildID := event.VoiceState.GuildID
	isBot := event.VoiceState.UserID == b.Client.ApplicationID

	if isBot {
		b.Lavalink.OnVoiceStateUpdate(context.TODO(),
			event.VoiceState.GuildID,
			event.VoiceState.ChannelID,
			event.VoiceState.SessionID,
		)
		if event.VoiceState.ChannelID != nil {
			b.voice.OnBotJoined(guildID, *event.VoiceState.ChannelID)
		} else {
			b.voice.OnBotLeft(guildID)
			b.Player.Delete(guildID)
			b.Cards.Drop(guildID)
			slog.Debug("left voice, dropped player state", slog.String("guild", guildID.String()))
		}
		return
	}

	oldChannelID := event.OldVoiceState.ChannelID
	b.voice.OnUserVoiceChange(guildID, event.VoiceState.UserID, oldChannelID, event.VoiceState.ChannelID)
}

// OnVoiceServerUpdate completes the Lavalink voice handshake.
func (b *Bot) OnVoiceServerUpdate(event *events.VoiceServerUpdate) {
	if event.Endpoint == nil {
		slog.Warn("voice server update with nil endpoint", slog.String("guild", event.GuildID.String()))
		return
	}
	b.Lavalink.OnVoiceServerUpdate(context.TODO(), event.GuildID, event.Token, *event.Endpoint)
}
