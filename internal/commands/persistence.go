package commands

import (
	"context"
	"fmt"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/store"
)


const historyDisplayLimit = 10

// History shows recently played tracks for the guild or a specific user.
func History(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	var records []store.PlayRecord
	var label string

	if userID, ok := data.OptSnowflake("user"); ok {
		records, _ = b.Store.UserPlays(context.Background(), guildID.String(), userID.String(), historyDisplayLimit)
		label = fmt.Sprintf("**Recent plays** for <@%s>", userID)
	} else {
		records, _ = b.Store.RecentPlays(context.Background(), guildID.String(), historyDisplayLimit)
		label = "**Recently played**"
	}

	if len(records) == 0 {
		return b.Reply(event, "No play history yet — play something first!")
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "%s (%d):\n", label, len(records))
	for i, r := range records {
		ts := ""
		if !r.PlayedAt.IsZero() {
			ts = r.PlayedAt.Format("15:04")
		}
		line := strings.TrimSpace(r.Title)
		if len([]rune(line)) > 60 {
			line = string([]rune(line)[:57]) + "…"
		}
		if r.URI != "" {
			fmt.Fprintf(&sb, "%d. [`%s`](<%s>) `%s`\n", i+1, line, r.URI, ts)
		} else {
			fmt.Fprintf(&sb, "%d. `%s` `%s`\n", i+1, line, ts)
		}
	}
	return b.Reply(event, sb.String())
}

// Taste manages per-user preferred artists for autoplay blending.
func Taste(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	action := data.String("action")
	userID := event.User().ID

	switch action {
	case "add":
		artist := strings.TrimSpace(data.String("artist"))
		if artist == "" {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Artist name cannot be empty"))
		}
		if err := b.Store.AddTasteArtist(context.Background(), userID.String(), artist); err != nil {
			return err
		}
		return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("Added **%s** to your taste profile", artist)))

	case "remove":
		artist := strings.TrimSpace(data.String("artist"))
		if artist == "" {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Artist name cannot be empty"))
		}
		if err := b.Store.RemoveTasteArtist(context.Background(), userID.String(), artist); err != nil {
			return err
		}
		return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("Removed **%s** from your taste profile", artist)))

	case "list":
		tastes, _ := b.Store.TasteArtists(context.Background(), userID.String())
		if len(tastes) == 0 {
			return b.Reply(event, "Your taste profile is empty. Use `/taste add` to add preferred artists.")
		}
		var sb strings.Builder
		sb.WriteString("**Your taste profile:**\n")
		for i, t := range tastes {
			fmt.Fprintf(&sb, "%d. %s (`%.1f`)\n", i+1, t.Artist, t.Weight)
		}
		return b.Reply(event, sb.String())

	default:
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Unknown action"))
	}
}

// ToggleAutoplay turns autoplay on/off for the guild.
func ToggleAutoplay(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	guildID := event.GuildID().String()
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID)
	if err != nil {
		return err
	}
	newEnabled := !settings.Autoplay
	level := settings.AutoplayLevel
	if level == "" {
		level = "normal"
	}
	if err := b.Store.SetGuildAutoplay(context.Background(), guildID, newEnabled, level); err != nil {
		return err
	}
	status := "disabled"
	if newEnabled {
		status = "enabled"
	}
	return b.ReplyEmbed(event, hexbot.SuccessEmbed(fmt.Sprintf("Autoplay %s (level: `%s`). When the queue drains, I'll keep the music going.", status, level)))
}

// Set247 toggles 24/7 mode — bot stays connected and survives restarts.
func Set247(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	guildID := event.GuildID().String()
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID)
	if err != nil {
		return err
	}
	newMode := !settings.Mode247
	if err := b.Store.SetGuild247(context.Background(), guildID, newMode); err != nil {
		return err
	}
	status := "disabled"
	if newMode {
		status = "enabled — I'll survive restarts and stay in voice"
		b.SaveSnapshot(*event.GuildID())
	} else {
		b.Store.DeletePlayerSnapshot(context.Background(), guildID)
	}
	return b.Reply(event, "24/7 mode "+status)
}

// SetRequestChannel designates a text channel as the song-request channel.
func SetRequestChannel(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	channelID := event.Channel().ID()
	guildID := event.GuildID().String()
	if err := b.Store.SetGuildRequestChannel(context.Background(), guildID, channelID.String()); err != nil {
		return err
	}
	return b.Reply(event, fmt.Sprintf("This channel (<#%s>) is now the song-request channel.\nPost any link or search query and I'll play it!", channelID))
}

// Playlist manages playlists via an action string option.
func PlaylistGroup(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	action := data.String("action")
	userID := event.User().ID
	ctx := context.Background()

	switch action {
	case "create":
		name := strings.TrimSpace(data.String("name"))
		if name == "" {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Playlist name cannot be empty"))
		}
		pl, err := b.Store.CreatePlaylist(ctx, userID.String(), name)
		if err != nil {
			return err
		}
		return b.Reply(event, fmt.Sprintf("Created playlist **%s** (share code: `%s`)", pl.Name, pl.ShareCode))

	case "list":
		playlists, _ := b.Store.ListUserPlaylists(ctx, userID.String())
		if len(playlists) == 0 {
			return b.Reply(event, "You have no playlists. Use `/playlist create` to make one.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**Your playlists** (%d):\n", len(playlists)))
		for i, pl := range playlists {
			fmt.Fprintf(&sb, "%d. **%s** (%d tracks) — code: `%s`\n", i+1, pl.Name, pl.TrackCnt, pl.ShareCode)
		}
		return b.Reply(event, sb.String())

	case "show":
		code := data.String("code")
		pl, err := b.Store.GetPlaylistByCode(ctx, code)
		if err != nil {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Playlist not found for code `%s`", code)))
		}
		var sb strings.Builder
		fmt.Fprintf(&sb, "**%s** (%d tracks, code: `%s`):\n", pl.Name, len(pl.Tracks), pl.ShareCode)
		for i, t := range pl.Tracks {
			line := t.Title
			if len([]rune(line)) > 55 {
				line = string([]rune(line)[:52]) + "…"
			}
			fmt.Fprintf(&sb, "%d. `%s`\n", i+1, line)
		}
		return b.Reply(event, sb.String())

	case "delete":
		code := data.String("code")
		pl, err := b.Store.GetPlaylistByCode(ctx, code)
		if err != nil {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Playlist not found"))
		}
		if pl.OwnerID != userID.String() {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("You can only delete your own playlists"))
		}
		if err := b.Store.DeletePlaylist(ctx, pl.ID, pl.OwnerID); err != nil {
			return err
		}
		return b.Reply(event, fmt.Sprintf("Deleted playlist **%s**", pl.Name))

	case "add":
		code := data.String("code")
		title := strings.TrimSpace(data.String("title"))
		if title == "" {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Track title cannot be empty"))
		}
		pl, err := b.Store.GetPlaylistByCode(ctx, code)
		if err != nil {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Playlist not found"))
		}
		if pl.OwnerID != userID.String() {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("You can only add tracks to your own playlists"))
		}
		if err := b.Store.AddPlaylistTrack(ctx, pl.ID, store.PlaylistTrack{Title: title}); err != nil {
			return err
		}
		return b.Reply(event, fmt.Sprintf("Added **%s** to **%s**", title, pl.Name))

	default:
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Unknown playlist subcommand"))
	}
}
