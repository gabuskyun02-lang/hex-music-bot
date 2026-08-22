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
func (b *Bot) VoteThreshold(event *events.ApplicationCommandInteractionCreate) int {
	if b.IsDJ(event) {
		return 0
	}
	vs, ok := b.Client.Caches.VoiceState(*event.GuildID(), event.User().ID)
	if !ok || vs.ChannelID == nil {
		return 0
	}
	listeners := len(b.voice.ListenerIDs(*event.GuildID()))
	if listeners <= 1 {
		return 0
	}
	return listeners
}
