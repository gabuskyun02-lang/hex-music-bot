package commands

import (
	"context"
	"fmt"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/ui"
)

// Volume sets the Lavalink player volume (Discord validates 0-1000).
func Volume(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to change volume"))
	}
	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("No active player — play something first"))
	}
	level := data.Int("level")
	if err := player.Update(context.TODO(), disgolink.WithVolume(level)); err != nil {
		return err
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("🔊 Volume set to `%d`", level)))
}

// Seek jumps within the current track.
func Seek(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to seek"))
	}
	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	if player.Track.Info.IsStream {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("⏩ Can't seek in a live stream"))
	}

	unit := int(lavalink.Second)
	if v, ok := data.OptInt("unit"); ok {
		unit = v
	}
	position := lavalink.Duration(clampSeek(int64(data.Int("position"))*int64(unit), int64(player.Track.Info.Length)))
	if err := player.Update(context.TODO(), disgolink.WithPosition(position)); err != nil {
		return err
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("⏩ Seeked to `%s`", ui.FormatDuration(position))))
}

// Remove deletes one queued track by 1-based position.
func Remove(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	state := b.Player.Get(*event.GuildID())
	index := data.Int("position")

	track, ok := state.RemoveAt(index - 1)
	if !ok {
		count := state.Len()
		if count == 0 {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("The queue is empty"))
		}
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Position out of range — queue has %d track(s)", count)))
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("🗑 Removed #%d: %s", index, ui.TrackMarkdown(track))))
}

// clampSeek confines pos to [0, dur); a non-positive dur yields 0.
func clampSeek(pos, dur int64) int64 {
	switch {
	case dur <= 0 || pos < 0:
		return 0
	case pos >= dur:
		return dur - 1 // stay below length — Lavalink rejects pos == duration
	default:
		return pos
	}
}
