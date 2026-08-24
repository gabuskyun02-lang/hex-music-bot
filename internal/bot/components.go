package bot

import (
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

// OnComponentInteraction routes card buttons and lyrics pagination.
// Button actions ack with a bare defer; the shared Actions run and the card
// re-renders through CardManager.Refresh — same path as slash commands.
func (b *Bot) OnComponentInteraction(event *events.ComponentInteractionCreate) {
	var customID string
	switch d := event.Data.(type) {
	case discord.ButtonInteractionData:
		customID = d.CustomID()
	default:
		_ = event.DeferUpdateMessage()
		return
	}

	if strings.HasPrefix(customID, "hexlyr:") {
		b.handleLyricsButton(event, customID)
		return
	}
	if !strings.HasPrefix(customID, "hex:") {
		_ = event.DeferUpdateMessage()
		return
	}
	guildID := event.GuildID()
	if guildID == nil {
		_ = event.DeferUpdateMessage()
		return
	}
	_ = event.DeferUpdateMessage()

	action := strings.TrimPrefix(customID, "hex:")
	if _, denied := b.checkCooldown(event.User().ID, "btn:"+action); denied {
		return // ack already sent; deny silently
	}

	switch action {
	case "prev":
		b.ReplayPrevious(*guildID)
	case "toggle":
		b.TogglePause(*guildID)
	case "skip":
		b.SkipNext(*guildID)
	case "loop":
		b.CycleLoop(*guildID)
	case "shuffle":
		b.ShuffleQueue(*guildID)
	case "voldown":
		b.StepVolume(*guildID, -25)
	case "volup":
		b.StepVolume(*guildID, +25)
	case "stop":
		b.Halt(*guildID) // finalizes the card itself
		return
	default:
		slog.Warn("unknown card button", slog.String("custom_id", customID))
		return
	}
	b.Cards.Refresh(*guildID)
}
