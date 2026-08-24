package bot

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/omit"
)

// Reply answers an interaction with a plain text message.
func (b *Bot) Reply(event *events.ApplicationCommandInteractionCreate, content string) error {
	return event.CreateMessage(discord.MessageCreate{Content: content})
}

// ReplyEmbed answers an interaction with an embed message.
func (b *Bot) ReplyEmbed(event *events.ApplicationCommandInteractionCreate, embed discord.Embed) error {
	return event.CreateMessage(discord.MessageCreate{Embeds: []discord.Embed{embed}})
}

// ReplyV2 answers an interaction with Components V2 layout components.
// V2 messages reject Embeds — converted surfaces must build containers only.
func (b *Bot) ReplyV2(event *events.ApplicationCommandInteractionCreate,
	comps []discord.LayoutComponent, ephemeral bool) error {
	flags := discord.MessageFlagIsComponentsV2
	if ephemeral {
		flags |= discord.MessageFlagEphemeral
	}
	return event.CreateMessage(discord.MessageCreate{Flags: flags, Components: comps})
}

// EditReply replaces a deferred interaction response with plain text.
func (b *Bot) EditReply(event *events.ApplicationCommandInteractionCreate, content string) error {
	_, err := b.Client.Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Content: omit.Ptr(content)},
	)
	return err
}

// SuccessEmbed returns a green-tinted confirmation embed.
func SuccessEmbed(message string) discord.Embed {
	return discord.Embed{
		Description: "✅ " + message,
		Color:       0x57F287,
	}
}

// ErrorEmbed returns a red-tinted error embed.
func ErrorEmbed(message string) discord.Embed {
	return discord.Embed{
		Description: "❌ " + message,
		Color:       0xED4245,
	}
}

// InfoEmbed returns a blue-tinted info embed with title and description.
func InfoEmbed(title, description string) discord.Embed {
	return discord.Embed{
		Title:       title,
		Description: description,
		Color:       0x5865F2,
	}
}
