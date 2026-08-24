package bot

import (
	"log/slog"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
)

// classifyError converts a raw handler error into a user-facing message.
// Detection patterns are borrowed from Beatra's ErrorHandler.classify (MIT).
func classifyError(err error) string {
	msg := strings.ToLower(err.Error())
	switch {
	case containsAny(msg,
		"sign in to confirm", "confirm you", "bot detection", "not a robot",
		"please sign in", "inappropriate", "this video is unavailable",
	) || (strings.Contains(msg, "youtube") && strings.Contains(msg, "403")):
		return "YouTube flagged this request — try again shortly or pick a different source."

	case containsAny(msg,
		"age-restricted", "age restricted", "confirm your age", "only available to registered users"):
		return "That track is age-restricted and can't be played here."

	case containsAny(msg,
		"private video", "video unavailable", "no longer available",
		"has been deleted", "is not available"):
		return "That track is private, deleted, or otherwise unavailable."

	case containsAny(msg, "copyright"):
		return "That track is blocked due to copyright restrictions."

	case containsAny(msg,
		"no results", "not found", "no entries", "no tracks", "nothing is playing"):
		return "Nothing found — the track or queue entry doesn't exist."

	case containsAny(msg, "timeout", "timed out", "deadline exceeded"):
		return "The request timed out — please try again."

	case containsAny(msg,
		"no nodes", "no available nodes", "node disconnected", "not connected",
		"connection refused", "econnrefused"):
		return "⚠️ No audio nodes available — try again shortly."

	default:
		return "Something went wrong — please try again."
	}
}

// containsAny reports whether msg contains any of the needles.
func containsAny(msg string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(msg, n) {
			return true
		}
	}
	return false
}

// replyError surfaces a handler failure as an ephemeral error embed.
// Handlers may already have acknowledged the interaction (deferrals), so
// CreateMessage failures fall back to updating the original response.
func (b *Bot) replyError(event *events.ApplicationCommandInteractionCreate, err error) {
	embed := ErrorEmbed(classifyError(err))
	createErr := event.CreateMessage(discord.MessageCreate{
		Embeds: []discord.Embed{embed},
		Flags:  discord.MessageFlagEphemeral,
	})
	if createErr == nil {
		return
	}
	if _, updateErr := b.Client.Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Embeds: &[]discord.Embed{embed}, Components: &[]discord.LayoutComponent{}},
	); updateErr != nil {
		slog.Error("failed to deliver error response",
			slog.Any("err", err),
			slog.String("create_err", createErr.Error()),
			slog.String("update_err", updateErr.Error()),
		)
	}
}
