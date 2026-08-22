package commands

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	hexbot "hex-music-bot/internal/bot"
)

// SettingsView shows all guild settings in one embed.
func SettingsView(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	guildID := event.GuildID().String()
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID)
	if err != nil {
		return err
	}

	djRole := "None"
	if settings.DJRoleID != "" {
		djRole = "<@&" + settings.DJRoleID + ">"
	}
	autoPause := "enabled"
	if !b.Cfg.AutoPause {
		autoPause = "disabled"
	}
	mode247 := "disabled"
	if settings.Mode247 {
		mode247 = "✅ enabled"
	}
	autoplay := "disabled"
	if settings.Autoplay {
		autoplay = "✅ enabled (level: " + settings.AutoplayLevel + ")"
	}
	dupes := "allowed"
	if !settings.AllowDuplicate {
		dupes = "blocked"
	}
	locale := settings.Locale
	if locale == "" {
		locale = "en"
	}

	return b.Reply(event, fmt.Sprintf(
		"**⚙️ Guild Settings**\n\n"+
			"**Language:** `%s`\n"+
			"**Default Volume:** `%d`\n"+
			"**DJ Role:** %s\n"+
			"**Auto-Pause:** `%s`\n"+
			"**24/7 Mode:** `%s`\n"+
			"**Autoplay:** `%s`\n"+
			"**Duplicate Tracks:** `%s`\n"+
			"**Request Channel:** %s",
		locale, settings.DefaultVolume, djRole,
		autoPause, mode247, autoplay, dupes,
		requestChannelDisplay(settings.RequestChannelID),
	))
}

func requestChannelDisplay(id string) string {
	if id == "" {
		return "not set"
	}
	return "<#" + id + ">"
}

// SetDJRole sets or clears the DJ role for the guild. Requires Manage Server permission.
func SetDJRole(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	role, ok := data.OptSnowflake("role")
	if !ok {
		if err := b.Store.SetGuildDJRole(context.Background(), event.GuildID().String(), ""); err != nil {
			return err
		}
		return b.Reply(event, "🎧 DJ role **cleared** — everyone can control music")
	}
	if err := b.Store.SetGuildDJRole(context.Background(), event.GuildID().String(), role.String()); err != nil {
		return err
	}
	return b.Reply(event, fmt.Sprintf("🎧 DJ role set to <@&%s> — only members with this role can skip/stop/volume/leave", role))
}
// VoteSkip implements vote-based skipping when no DJ is set.
func VoteSkip(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	userID := event.User().ID

	threshold := b.VoteThreshold(event)
	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil || player.Track == nil {
		return b.Reply(event, "Nothing is playing")
	}

	alreadyVoted, total, needed := b.Votes.VoteOrExecute(guildID, userID, "skip", threshold, func() {
		b.SkipNext(guildID)
		b.Cards.Refresh(guildID)
	})

	if threshold <= 0 {
		return b.Reply(event, "⏭ Skipped") // DJ executed immediately
	}
	if alreadyVoted {
		return b.Reply(event, fmt.Sprintf("You already voted — %d/%d votes", total, needed))
	}
	return b.Reply(event, fmt.Sprintf("🗳 Vote registered (%d/%d)", total, needed))
}
