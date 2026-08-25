package commands

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	hexbot "hex-music-bot/internal/bot"
)

// SettingsView shows all guild settings in a two-section V2 container:
// General (DJ role, request channel, auto-pause) split from Playback
// (volume, 24/7, autoplay, duplicates) by separators — Zeta-style framing.
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

	container := discord.ContainerComponent{AccentColor: 0x5865F2}
	container.Components = append(container.Components,
		discord.TextDisplayComponent{Content: "### ⚙️ Guild Settings"},
		discord.SeparatorComponent{},
		discord.TextDisplayComponent{Content: fmt.Sprintf(
			"**General**\n"+
				"**DJ Role:** %s\n"+
				"**Request Channel:** %s\n"+
				"**Auto-Pause:** `%s`",
			djRole, requestChannelDisplay(settings.RequestChannelID), autoPause,
		)},
		discord.SeparatorComponent{},
		discord.TextDisplayComponent{Content: fmt.Sprintf(
			"**Playback**\n"+
				"**Default Volume:** `%d`\n"+
				"**24/7 Mode:** `%s`\n"+
				"**Autoplay:** `%s`\n"+
				"**Duplicate Tracks:** `%s`",
			settings.DefaultVolume, mode247, autoplay, dupes,
		)},
	)
	return b.ReplyV2(event, []discord.LayoutComponent{container}, false)
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
	if threshold < 0 {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You must be in the bot's voice channel to vote"))
	}
	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}

	alreadyVoted, total, needed := b.Votes.VoteOrExecute(guildID, userID, "skip", threshold, func() {
		b.SkipNext(guildID)
		b.Cards.Refresh(guildID)
	})

	if threshold == 0 {
		return b.Reply(event, "⏭ Skipped") // DJ executed immediately
	}
	if alreadyVoted {
		return b.Reply(event, fmt.Sprintf("You already voted — %d/%d votes", total, needed))
	}
	return b.Reply(event, fmt.Sprintf("🗳 Vote registered (%d/%d)", total, needed))
}
