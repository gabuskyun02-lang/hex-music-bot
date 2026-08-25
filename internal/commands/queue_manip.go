package commands

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/ui"
)

// resolveTrack loads a track from an identifier string.
func resolveTrack(b *hexbot.Bot, identifier string) (*lavalink.Track, []lavalink.Track, error) {
	node := b.BestHealthyNode()
	if node == nil {
		return nil, nil, fmt.Errorf("lavalink node not connected")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var toPlay *lavalink.Track
	var enqueued []lavalink.Track
	node.Rest.LoadTracksHandler(ctx, identifier, disgolink.NewTrackLoadingResultHandler(
		func(t lavalink.Track) { toPlay = &t },
		func(p lavalink.Playlist) {
			if len(p.Tracks) > 0 {
				toPlay = &p.Tracks[0]
				enqueued = p.Tracks[1:]
			}
		},
		func(ts []lavalink.Track) {
			if len(ts) > 0 {
				toPlay = &ts[0]
			}
		},
		func() {},
		func(err error) {},
	))
	return toPlay, enqueued, nil
}

// checkDuplicate returns true when the guild blocks duplicates and the track
// title already exists in the queue or is currently playing.
func checkDuplicate(b *hexbot.Bot, guildID snowflake.ID, title string) bool {
	settings, _ := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if settings == nil || settings.AllowDuplicate {
		return false
	}
	queue := b.Player.Get(guildID)
	if queue.HasDuplicate(title) {
		return true
	}
	if p := b.Lavalink.ExistingPlayer(guildID); p != nil && p.Track != nil {
		return strings.EqualFold(p.Track.Info.Title, title)
	}
	return false
}

// PlayTop resolves a track and inserts it at the front of the queue.
func PlayTop(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	vs, ok := b.Client.Caches.VoiceState(guildID, event.User().ID)
	if !ok {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need to be in a voice channel first"))
	}
	if err := event.DeferCreateMessage(false); err != nil {
		return err
	}

	identifier := data.String("identifier")
	toPlay, enqueued, err := resolveTrack(b, identifier)
	if err != nil {
		return b.EditReply(event, fmt.Sprintf("❌ %v", err))
	}
	if toPlay == nil {
		return b.EditReply(event, fmt.Sprintf("Nothing found for `%s`", identifier))
	}
	if checkDuplicate(b, guildID, toPlay.Info.Title) {
		return b.EditReply(event, "⛔ Duplicate tracks are blocked on this server")
	}

	queue := b.Player.Get(guildID)
	p := b.Lavalink.ExistingPlayer(guildID)

	if p != nil && p.Track != nil {
		if !queue.InsertAtAs(0, *toPlay, event.User().ID) {
			return b.EditReply(event, "⚠️ Queue is full")
		}
		msg := fmt.Sprintf("📌 %s will play next", ui.TrackMarkdown(*toPlay))
		if len(enqueued) > 0 {
			added, rej := queue.EnqueueAs(event.User().ID, enqueued...)
			msg += fmt.Sprintf(" (+%d playlist tracks)", added)
			if rej > 0 {
				msg += fmt.Sprintf("\n⚠️ Queue full — %d playlist track(s) rejected", rej)
			}
		}
		return b.EditReply(event, msg)
	}

	queue.EnqueueAs(event.User().ID, enqueued...)
	b.Client.UpdateVoiceState(context.TODO(), guildID, vs.ChannelID, false, false)
	if err := b.Lavalink.Player(guildID).Update(context.TODO(), disgolink.WithTrack(*toPlay)); err != nil {
		return err
	}
	b.Cards.Create(guildID, event.Channel().ID())
	return b.EditReply(event, fmt.Sprintf("Playing %s", ui.TrackMarkdown(*toPlay)))
}

// PlaySkip resolves a track, inserts at top, and immediately skips to it.
func PlaySkip(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	vs, ok := b.Client.Caches.VoiceState(guildID, event.User().ID)
	if !ok {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need to be in a voice channel first"))
	}
	if err := event.DeferCreateMessage(false); err != nil {
		return err
	}

	identifier := data.String("identifier")
	toPlay, _, err := resolveTrack(b, identifier)
	if err != nil {
		return b.EditReply(event, fmt.Sprintf("❌ %v", err))
	}
	if toPlay == nil {
		return b.EditReply(event, fmt.Sprintf("Nothing found for `%s`", identifier))
	}
	if checkDuplicate(b, guildID, toPlay.Info.Title) {
		return b.EditReply(event, "⛔ Duplicate tracks are blocked on this server")
	}
	p := b.Lavalink.ExistingPlayer(guildID)
	if p != nil && p.Track != nil {
		b.Player.Get(guildID).PushHistory(*p.Track)
	}
	b.Client.UpdateVoiceState(context.TODO(), guildID, vs.ChannelID, false, false)
	if err := b.Lavalink.Player(guildID).Update(context.TODO(), disgolink.WithTrack(*toPlay)); err != nil {
		return err
	}
	b.Cards.Refresh(guildID)
	return b.EditReply(event, fmt.Sprintf("⏭ Playing now: %s", ui.TrackMarkdown(*toPlay)))
}

// SkipTo drops all tracks before the given position and plays the next.
func SkipTo(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	player := b.Lavalink.ExistingPlayer(guildID)
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}

	pos := data.Int("position") - 1
	state := b.Player.Get(guildID)
	state.Drop(pos)

	next, ok := state.Next()
	if !ok {
		_ = player.Update(context.TODO(), disgolink.WithNullTrack())
		return b.ReplyEmbed(event, hexbot.SuccessEmbed("Skipped — queue is empty"))
	}
	if err := player.Update(context.TODO(), disgolink.WithTrack(next)); err != nil {
		return err
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed("⏭ Skipped to "+ui.TrackMarkdown(next)))
}

// Move moves a queued song to a different position.
func Move(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	fromIdx := data.Int("from") - 1
	toIdx := data.Int("to") - 1
	state := b.Player.Get(*event.GuildID())

	if !state.MoveTrack(fromIdx, toIdx) {
		count := state.Len()
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Position out of range — queue has %d track(s)", count)))
	}
	snap := state.Snapshot()
	title := snap[toIdx].Info.Title
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("🔀 Moved **%s** to position #%d", title, toIdx+1)))
}

// Swap exchanges two queued songs by position.
func Swap(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	aIdx := data.Int("first") - 1
	cIdx := data.Int("second") - 1
	state := b.Player.Get(*event.GuildID())

	if !state.SwapTracks(aIdx, cIdx) {
		count := state.Len()
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Position out of range — queue has %d track(s)", count)))
	}
	snap := state.Snapshot()
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("🔁 Swapped **%s** ↔ **%s**", snap[aIdx].Info.Title, snap[cIdx].Info.Title)))
}

// RemoveDupes removes duplicate tracks from the queue.
func RemoveDupes(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	removed := b.Player.Get(*event.GuildID()).RemoveDuplicates()
	if removed == 0 {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("No duplicates found"))
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("🧹 Removed %d duplicate track(s)", removed)))
}

// Filter applies an audio filter preset.
func Filter(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	name := data.String("preset")
	num := func(n string) *float64 {
		if v, ok := data.OptFloat(n); ok {
			return &v
		}
		return nil
	}
	if err := b.Filters.SetFilter(*event.GuildID(), name, num("speed"), num("pitch"), num("rate")); err != nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("❌ %v", err)))
	}
	if name == "reset" || name == "off" {
		return b.ReplyEmbed(event, hexbot.SuccessEmbed("🔄 All filters cleared"))
	}
	active := b.Filters.ActiveFilters(*event.GuildID())
	msg := "🎵 Applied **" + name + "** filter"
	if len(active) > 0 {
		msg += "\nActive: `" + strings.Join(active, ", ") + "`"
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(msg))
}

// Forward skips forward by a time amount (default 10s).
func Forward(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	durStr := "10"
	if v, ok := data.OptString("time"); ok {
		durStr = v
	}
	ms := ui.ParseTimeStr(durStr)
	if ms <= 0 {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Invalid time format `%s` — use `10`, `1:30`, or `1:00:00`", durStr)))
	}

	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	newPos := clampSeek(int64(player.Position())+ms, int64(player.Track.Info.Length))
	if err := player.Update(context.TODO(), disgolink.WithPosition(lavalink.Duration(newPos))); err != nil {
		return err
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("⏩ Forwarded to `%s`", ui.FormatDuration(lavalink.Duration(newPos)))))
}

// Rewind goes backward by a time amount (default 10s).
func Rewind(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	durStr := "10"
	if v, ok := data.OptString("time"); ok {
		durStr = v
	}
	ms := ui.ParseTimeStr(durStr)
	if ms <= 0 {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Invalid time format `%s` — use `10`, `1:30`, or `1:00:00`", durStr)))
	}

	player := b.Lavalink.ExistingPlayer(*event.GuildID())
	if player == nil || player.Track == nil {
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Nothing is playing"))
	}
	newPos := clampSeek(int64(player.Position())-ms, int64(player.Track.Info.Length))
	if err := player.Update(context.TODO(), disgolink.WithPosition(lavalink.Duration(newPos))); err != nil {
		return err
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("⏪ Rewound to `%s`", ui.FormatDuration(lavalink.Duration(newPos)))))
}
