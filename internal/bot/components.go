package bot

import (
	"context"
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

// OnComponentInteraction routes card buttons, lyrics pagination, and list
// pagination. Button actions ack with a bare defer; the shared Actions run
// and the card re-renders through CardManager.Refresh — same path as slash
// commands.
func (b *Bot) OnComponentInteraction(event *events.ComponentInteractionCreate) {
	var customID string
	switch d := event.Data.(type) {
	case discord.ButtonInteractionData:
		customID = d.CustomID()
	default:
		_ = event.DeferUpdateMessage()
		return
	}

	if strings.HasPrefix(customID, "hexsync:") {
		b.handleSyncStopButton(event, customID)
		return
	}
	if strings.HasPrefix(customID, "hexlyr:") {
		b.handleLyricsButton(event, customID)
		return
	}
	if strings.HasPrefix(customID, "hexp:") {
		b.handlePagerButton(event, customID)
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

	action := strings.TrimPrefix(customID, "hex:")

	// When CardDJGated is enabled, destructive buttons require DJ role.
	// Default off — keeps current guild behavior when unset.
	if b.Cfg.CardDJGated && isDestructiveAction(action) {
		if !b.isComponentDJ(*guildID, event.Member()) {
			_ = event.CreateMessage(discord.MessageCreate{
				Content: "❌ You need the DJ role to use this button",
				Flags:   discord.MessageFlagEphemeral,
			})
			return
		}
	}

	_ = event.DeferUpdateMessage()

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

// isDestructiveAction returns true for card button actions that alter
// playback state destructively (stop, skip, clear-like).
func isDestructiveAction(action string) bool {
	switch action {
	case "stop", "skip", "prev":
		return true
	}
	return false
}

// isComponentDJ checks DJ privileges for a component interaction member.
// Same logic as IsDJ but operates on raw guild ID + member, avoiding the
// ApplicationCommandInteractionCreate dependency.
func (b *Bot) isComponentDJ(guildID snowflake.ID, member *discord.ResolvedMember) bool {
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if err != nil || settings.DJRoleID == "" {
		return true // no DJ role configured
	}
	djRoleID, err := snowflake.Parse(settings.DJRoleID)
	if err != nil {
		return true
	}
	if member == nil {
		return false
	}
	for _, roleID := range member.RoleIDs {
		if roleID == djRoleID {
			return true
		}
	}
	return false
}
