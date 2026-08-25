package commands

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
	"hex-music-bot/internal/store"
	"hex-music-bot/internal/ui"
)

// History shows recently played tracks for the guild or a specific user.
func History(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error {
	guildID := *event.GuildID()
	var records []store.PlayRecord
	var userID string
	if id, ok := data.OptSnowflake("user"); ok {
		userID = id.String()
		records, _ = b.Store.UserPlays(context.Background(), guildID.String(), userID, historyFetchLimit)
	} else {
		records, _ = b.Store.RecentPlays(context.Background(), guildID.String(), historyFetchLimit)
	}

	if len(records) == 0 {
		return b.ReplyEmbed(event, hexbot.InfoEmbed("🕘 History", "No play history yet — play something first!"))
	}

	header := "🕘 Recently played"
	if userID != "" {
		header = fmt.Sprintf("🕘 Recent plays — <@%s>", userID)
	}
	rows := historyRows(records)
	footer := fmt.Sprintf("%d track(s) · most recent first", len(records))
	if session, paged := b.NewPagerSession(header, rows, footer, 0x5865F2); paged {
		comps := append(hexbot.RenderPagerPage(session), hexbot.PagerButtons(session, session.Page))
		return b.ReplyV2(event, comps, false)
	}
	return b.ReplyV2(event, hexbot.BuildListContainer(header, rows, footer, 0x5865F2), false)
}

const historyFetchLimit = 100

// historyRows renders PlayRecords as list rows: `N.` [title](<uri>) ·
// <@requester> · `15:04`, keeping the existing trimming rules (57 runes +
// ellipsis; bare title when the record has no URI).
func historyRows(records []store.PlayRecord) []string {
	rows := make([]string, 0, len(records))
	for i, r := range records {
		ts := ""
		if !r.PlayedAt.IsZero() {
			ts = r.PlayedAt.Format("15:04")
		}
		line := strings.TrimSpace(r.Title)
		if len([]rune(line)) > 60 {
			line = string([]rune(line)[:57]) + "…"
		}
		req := ""
		if r.RequesterID != "" && r.RequesterID != "0" {
			req = fmt.Sprintf(" · <@%s>", r.RequesterID)
		}
		var row string
		if r.URI != "" {
			row = fmt.Sprintf("`%d.` [`%s`](<%s>)%s `%s`", i+1, line, r.URI, req, ts)
		} else {
			row = fmt.Sprintf("`%d.` `%s`%s `%s`", i+1, line, req, ts)
		}
		rows = append(rows, row)
	}
	return rows
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
		identifier := title
		if !urlPattern.MatchString(identifier) && !searchPattern.MatchString(identifier) {
			identifier = lavalink.SearchTypeYouTube.Apply(identifier)
		}
		resolved, _, err := resolveTrack(b, identifier)
		if err != nil || resolved == nil {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Nothing found for `%s`", title)))
		}
		uri := ""
		if resolved.Info.URI != nil {
			uri = *resolved.Info.URI
		}
		trackRecord := store.PlaylistTrack{
			Identifier: resolved.Info.Identifier,
			Title:      resolved.Info.Title,
			Author:     resolved.Info.Author,
			LengthMS:   int64(resolved.Info.Length),
			URI:        uri,
		}
		if err := b.Store.AddPlaylistTrack(ctx, pl.ID, trackRecord); err != nil {
			return err
		}
		return b.Reply(event, fmt.Sprintf("Added **%s** to **%s**", resolved.Info.Title, pl.Name))

	case "play":
		code := data.String("code")
		guildID := *event.GuildID()
		vs, ok := b.Client.Caches.VoiceState(guildID, event.User().ID)
		if !ok {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("You need to be in a voice channel first"))
		}
		pl, err := b.Store.GetPlaylistByCode(ctx, code)
		if err != nil {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Playlist not found for code `%s`", code)))
		}
		if len(pl.Tracks) == 0 {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed(fmt.Sprintf("Playlist **%s** is empty", pl.Name)))
		}
		if err := event.DeferCreateMessage(false); err != nil {
			return err
		}
		return playlistPlay(b, event, vs, pl)

	case "savequeue":
		guildID := *event.GuildID()
		name := strings.TrimSpace(data.String("name"))
		if name == "" {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Playlist name cannot be empty"))
		}
		queue := b.Player.Get(guildID)
		snap := queue.Snapshot()
		p := b.Lavalink.ExistingPlayer(guildID)
		var allTracks []lavalink.Track
		if p != nil && p.Track != nil {
			allTracks = append(allTracks, *p.Track)
		}
		allTracks = append(allTracks, snap...)
		if len(allTracks) == 0 {
			return b.ReplyEmbed(event, hexbot.ErrorEmbed("Queue is empty — nothing to save"))
		}
		pl, err := b.Store.CreatePlaylist(ctx, userID.String(), name)
		if err != nil {
			return err
		}
		for _, t := range allTracks {
			uri := ""
			if t.Info.URI != nil {
				uri = *t.Info.URI
			}
			_ = b.Store.AddPlaylistTrack(ctx, pl.ID, store.PlaylistTrack{
				Identifier: t.Info.Identifier,
				Title:      t.Info.Title,
				Author:     t.Info.Author,
				LengthMS:   int64(t.Info.Length),
				URI:        uri,
			})
		}
		return b.Reply(event, fmt.Sprintf("Saved %d track(s) to playlist **%s** (share code: `%s`)", len(allTracks), pl.Name, pl.ShareCode))

	default:
		return b.ReplyEmbed(event, hexbot.ErrorEmbed("Unknown playlist subcommand"))
	}
}

// playlistPlay enqueues playlist tracks with legacy backfill and starts playback if idle.
func playlistPlay(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, vs discord.VoiceState, pl *store.Playlist) error {
	guildID := *event.GuildID()
	node := b.BestHealthyNode()
	if node == nil {
		return b.EditReply(event, "Lavalink node not connected — try again in a moment")
	}

	tracks := make([]lavalink.Track, len(pl.Tracks))
	var unresTitles []string
	var mu sync.Mutex

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, pt := range pl.Tracks {
		wg.Add(1)
		go func(i int, pt store.PlaylistTrack) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			query := pt.Identifier
			isLegacy := pt.Identifier == "" || pt.URI == ""
			if isLegacy {
				query = pt.Title
				if !urlPattern.MatchString(query) && !searchPattern.MatchString(query) {
					query = lavalink.SearchTypeYouTube.Apply(query)
				}
			}
			t, _, err := resolveTrack(b, query)
			if err == nil && t != nil {
				tracks[i] = *t
				if isLegacy {
					uri := ""
					if t.Info.URI != nil {
						uri = *t.Info.URI
					}
					_ = b.Store.UpdatePlaylistTrack(context.Background(), pl.ID, pt.Title, store.PlaylistTrack{
						Identifier: t.Info.Identifier,
						Title:      t.Info.Title,
						Author:     t.Info.Author,
						LengthMS:   int64(t.Info.Length),
						URI:        uri,
					})
				}
			} else {
				mu.Lock()
				unresTitles = append(unresTitles, pt.Title)
				mu.Unlock()
			}
		}(i, pt)
	}
	wg.Wait()

	var resolvedTracks []lavalink.Track
	for _, t := range tracks {
		if t.Info.Title != "" || t.Info.Identifier != "" {
			resolvedTracks = append(resolvedTracks, t)
		}
	}

	if len(resolvedTracks) == 0 {
		return b.EditReply(event, fmt.Sprintf("❌ Could not resolve any tracks in playlist **%s**", pl.Name))
	}

	queue := b.Player.Get(guildID)
	settings, _ := b.Store.GetGuildSettings(context.Background(), guildID.String())

	var toEnqueue []lavalink.Track
	if settings != nil && !settings.AllowDuplicate {
		for _, t := range resolvedTracks {
			if !queue.HasDuplicate(t.Info.Title) {
				if p := b.Lavalink.ExistingPlayer(guildID); p != nil && p.Track != nil && strings.EqualFold(p.Track.Info.Title, t.Info.Title) {
					continue
				}
				toEnqueue = append(toEnqueue, t)
			}
		}
		if len(toEnqueue) == 0 {
			return b.EditReply(event, "⛔ All playlist tracks are duplicates and blocked on this server")
		}
	} else {
		toEnqueue = resolvedTracks
	}

	p := b.Lavalink.ExistingPlayer(guildID)
	if p != nil && p.Track != nil {
		added, rej := queue.EnqueueAs(event.User().ID, toEnqueue...)
		if added == 0 {
			return b.EditReply(event, "⚠️ Queue is full — nothing was added")
		}
		msg := fmt.Sprintf("Added **%d** track(s) from playlist **%s** to the queue", added, pl.Name)
		if rej > 0 {
			msg += fmt.Sprintf(" (%d rejected: queue full)", rej)
		}
		if len(unresTitles) > 0 {
			msg += fmt.Sprintf(" · %d track(s) failed to resolve", len(unresTitles))
		}
		return b.EditReply(event, msg)
	}

	first := toEnqueue[0]
	rest := toEnqueue[1:]
	added, rej := queue.EnqueueAs(event.User().ID, rest...)

	_ = b.Client.UpdateVoiceState(context.TODO(), guildID, vs.ChannelID, false, false)
	updateOpts := []disgolink.PlayerUpdateOpt{disgolink.WithTrack(first)}
	if settings != nil && settings.DefaultVolume > 0 && settings.DefaultVolume != 100 {
		updateOpts = append(updateOpts, disgolink.WithVolume(settings.DefaultVolume))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := b.Lavalink.Player(guildID).Update(ctx, updateOpts...); err != nil {
		return err
	}
	b.Cards.Create(guildID, event.Channel().ID())

	msg := fmt.Sprintf("Playing %s from playlist **%s**", ui.TrackMarkdown(first), pl.Name)
	if added > 0 {
		msg += fmt.Sprintf(" (+%d track(s) queued)", added)
	}
	if rej > 0 {
		msg += fmt.Sprintf(" (%d rejected: queue full)", rej)
	}
	if len(unresTitles) > 0 {
		msg += fmt.Sprintf(" · %d track(s) failed to resolve", len(unresTitles))
	}
	return b.EditReply(event, msg)
}
