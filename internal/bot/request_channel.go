package bot

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
)

// OnMessageCreate listens for song requests in designated channels.
func (b *Bot) OnMessageCreate(event *events.MessageCreate) {
	if event.GuildID == nil {
		return
	}
	if event.Message.Author.Bot {
		return
	}

	guildID := *event.GuildID
	settings, err := b.Store.GetGuildSettings(context.Background(), guildID.String())
	if err != nil || settings.RequestChannelID == "" {
		return
	}
	if event.ChannelID.String() != settings.RequestChannelID {
		return
	}
	content := strings.TrimSpace(event.Message.Content)
	if content == "" || strings.HasPrefix(content, "/") || strings.HasPrefix(content, "!") {
		return
	}

	go b.handleSongRequest(guildID, event.ChannelID, event.Message.ID, content)
}

func (b *Bot) handleSongRequest(guildID snowflake.ID, textChannelID snowflake.ID, messageID snowflake.ID, content string) {
	node := b.Lavalink.BestNode()
	if node == nil {
		b.notifyChannel(guildID, "📡 No Lavalink node connected — try again in a moment.")
		return
	}

	identifier := content
	if !strings.Contains(content, "http") && !strings.Contains(content, "search:") {
		identifier = lavalink.SearchTypeYouTube.Apply(content)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var toPlay *lavalink.Track
	node.Rest.LoadTracksHandler(ctx, identifier, disgolink.NewTrackLoadingResultHandler(
		func(t lavalink.Track) { toPlay = &t },
		func(p lavalink.Playlist) {},
		func(ts []lavalink.Track) { if len(ts) > 0 { toPlay = &ts[0] } },
		func() {},
		func(err error) {},
	))

	_ = b.Client.Rest.AddReaction(textChannelID, messageID, "✅")

	if toPlay == nil {
		b.notifyChannel(guildID, fmt.Sprintf("🔍 Nothing found for `%s`", content))
		return
	}

	queue := b.Player.Get(guildID)
	p := b.Lavalink.ExistingPlayer(guildID)

	vs, vsOK := b.Client.Caches.VoiceState(guildID, b.Client.ApplicationID)
	if !vsOK || vs.ChannelID == nil {
		b.notifyChannel(guildID, "💡 I'm not in voice — use /play or /join first to start listening.")
		return
	}

	if p != nil && p.Track != nil {
		queue.Enqueue(*toPlay)
		b.Cards.Refresh(guildID)
	} else {
		b.Client.UpdateVoiceState(context.TODO(), guildID, vs.ChannelID, false, false)
		_ = b.Lavalink.Player(guildID).Update(ctx, disgolink.WithTrack(*toPlay))
		b.Cards.Create(guildID, textChannelID)
	}
	slog.Info("song request queued",
		slog.String("guild", guildID.String()),
		slog.String("title", toPlay.Info.Title),
	)
}
