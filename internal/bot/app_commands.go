package bot

import (
	"log/slog"

	"github.com/disgoorg/disgo/events"
)

// OnApplicationCommand routes slash command interactions to their handlers.
func (b *Bot) OnApplicationCommand(event *events.ApplicationCommandInteractionCreate) {
	data := event.SlashCommandInteractionData()

	b.Metrics.Inc("hex_music_bot_commands_total")
	handler, ok := b.Handlers[data.CommandName()]
	if !ok {
		slog.Warn("unknown command interaction", slog.String("command", data.CommandName()))
		return
	}
	if err := handler(event, data); err != nil {
		slog.Error("command failed",
			slog.String("command", data.CommandName()),
			slog.Any("err", err),
		)
	}
}
