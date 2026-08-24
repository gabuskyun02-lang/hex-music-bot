package bot

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// OnApplicationCommand routes slash command interactions to their handlers.
func (b *Bot) OnApplicationCommand(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()
	userID := event.User().ID

	b.Metrics.Inc("hex_music_bot_commands_total")
	handler, ok := b.Handlers[data.CommandName()]
	if !ok {
		slog.Warn("unknown command interaction", slog.String("command", data.CommandName()))
		_ = event.CreateMessage(discord.MessageCreate{
			Content: "Unknown command.",
			Flags:   discord.MessageFlagEphemeral,
		})
		return
	}
	if remaining, denied := b.checkCooldown(userID, data.CommandName()); denied {
		_ = event.CreateMessage(discord.MessageCreate{
			Content: fmt.Sprintf("⏳ Please wait %.1fs before using `/%s` again.", remaining.Seconds(), data.CommandName()),
			Flags:   discord.MessageFlagEphemeral,
		})
		return
	}
	if err := handler(event, data); err != nil {
		slog.Error("command failed",
			slog.String("command", data.CommandName()),
			slog.Any("err", err),
		)
		b.replyError(event, err)
	}
}

func (b *Bot) checkCooldown(userID snowflake.ID, command string) (time.Duration, bool) {
	if b.IsOwner(userID) {
		return 0, false
	}
	ok, remaining := b.Cooldowns.Allow(userID, command)
	if !ok {
		b.Metrics.Inc("hex_music_bot_cooldown_denies")
	}
	return remaining, !ok
}
