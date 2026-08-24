package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/player"
	"hex-music-bot/internal/ui"
)

// Queue lists every queued track in a V2 container: per-track source badge,
// linked title, duration, requester mention; footer carries total runtime and
// non-default mode flags. Long queues paginate through a PagerSession.
func Queue(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	state := b.Player.Get(*event.GuildID())
	tracks := state.Snapshot()
	if len(tracks) == 0 {
		return b.ReplyEmbed(event, hexbot.InfoEmbed("📋 Queue", "The queue is empty — add something with `/play`"))
	}

	var total lavalink.Duration
	rows := make([]string, 0, len(tracks))
	for i, track := range tracks {
		total += track.Info.Length
		row := fmt.Sprintf("`%d.` %s %s · `%s`", i+1,
			ui.SourceBadgeFor(track.Info.SourceName).Emoji, ui.TrackMarkdown(track),
			ui.FormatDuration(track.Info.Length))
		if req := state.RequesterFor(track.Info.Identifier); req != 0 {
			row += fmt.Sprintf(" · <@%s>", req)
		}
		rows = append(rows, row)
	}

	status := []string{}
	if state.LoopMode() != player.LoopOff {
		status = append(status, "🔁 Loop "+state.LoopMode().String())
	}
	if b.Cfg != nil {
		settings, _ := b.Store.GetGuildSettings(context.Background(), event.GuildID().String())
		if settings != nil && settings.Autoplay {
			status = append(status, "🔄 Autoplay")
		}
	}
	footer := "⏱ " + ui.FormatDuration(total)
	if len(status) > 0 {
		footer += "\n" + strings.Join(status, " · ")
	}

	header := fmt.Sprintf("📋 Up next (%d)", len(tracks))
	if session, paged := b.NewPagerSession(header, rows, footer, 0x5865F2); paged {
		comps := append(hexbot.RenderPagerPage(session), hexbot.PagerButtons(session, session.Page))
		return b.ReplyV2(event, comps, false)
	}
	return b.ReplyV2(event, hexbot.BuildListContainer(header, rows, footer, 0x5865F2), false)
}

// NowPlaying reports the current track as a static V2 snapshot of the live
// card body (no buttons — the in-channel card stays the control surface).
func NowPlaying(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	return b.ReplyV2(event, b.RenderCardBody(*event.GuildID()), false)
}

// Shuffle randomizes the queue order via the shared action.
func Shuffle(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to shuffle"))
	}
	b.ShuffleQueue(*event.GuildID())
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Queue shuffled 🔀"))
}

// Loop switches the guild's loop mode via the shared action.
func Loop(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	if !b.IsDJ(event) {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need the DJ role to change loop mode"))
	}
	mode, ok := player.ParseLoopMode(data.String("mode"))
	if !ok {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Unknown loop mode"))
	}
	b.Player.Get(*event.GuildID()).SetLoopMode(mode)
	b.Cards.Refresh(*event.GuildID())

	label := map[player.LoopMode]string{
		player.LoopOff:   "🔁 Off",
		player.LoopTrack: "🔂 Track",
		player.LoopQueue: "🔁 Queue",
	}[mode]
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("Loop set to **"+label+"**"))
}
