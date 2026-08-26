package bot

import (
	"context"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// IsDJ checks if the user has DJ privileges for this guild.
// Returns true when: DJ role is not configured, user has the DJ role,
// or user is the bot owner (GUILD_ID config).
func (b *Bot) IsDJ(event *events.ApplicationCommandInteractionCreate) bool {
	guildID := *event.GuildID()

	settings, err := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if err != nil || settings.DJRoleID == "" {
		return true // no DJ role configured, everyone can control
	}

	djRoleID, err := snowflake.Parse(settings.DJRoleID)
	if err != nil {
		return true
	}

	if event.Member() == nil {
		return false
	}
	for _, roleID := range event.Member().RoleIDs {
		if roleID == djRoleID {
			return true
		}
	}
	return false
}

// VoteThreshold returns the number of votes required for the guild.
// Returns 0 if no voting needed (user is DJ or alone in channel).
// Returns -1 if the requester is not in the bot's current voice channel
// (callers should treat this as a denial).
func (b *Bot) VoteThreshold(event *events.ApplicationCommandInteractionCreate) int {
	if b.IsDJ(event) {
		return 0
	}
	vs, ok := b.Client.Caches.VoiceState(*event.GuildID(), event.User().ID)
	botCh, botConnected := b.voice.ChannelFor(*event.GuildID())
	inBotChannel := ok && vs.ChannelID != nil && botConnected && *vs.ChannelID == botCh
	return voteThresholdFor(inBotChannel, len(b.voice.ListenerIDs(*event.GuildID())))
}

// voteThreshold returns the majority vote count for a given listener count.
func voteThreshold(listeners int) int {
	return listeners/2 + 1
}

// voteThresholdFor classifies a voter's access: -1 when they are not in the
// bot's voice channel, 0 when no vote is needed (alone), otherwise the
// majority threshold for the current listener count.
func voteThresholdFor(inBotChannel bool, listeners int) int {
	if !inBotChannel {
		return -1
	}
	if listeners <= 1 {
		return 0
	}
	return voteThreshold(listeners)
}
