package bot

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/omit"
)

// Reply answers an interaction with a new message.
func (b *Bot) Reply(event *events.ApplicationCommandInteractionCreate, content string) error {
	return event.CreateMessage(discord.MessageCreate{Content: content})
}

// EditReply replaces a previously deferred interaction response. Callers
// must have called event.DeferCreateMessage first.
func (b *Bot) EditReply(event *events.ApplicationCommandInteractionCreate, content string) error {
	_, err := b.Client.Rest.UpdateInteractionResponse(
		event.ApplicationID(),
		event.Token(),
		discord.MessageUpdate{Content: omit.Ptr(content)},
	)
	return err
}
