package bot

import (
	"context"
	"log/slog"

	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"
)

// recordPlay persists a play_history entry after each finished track.
func (b *Bot) recordPlay(guildID snowflake.ID, track lavalink.Track) {
	requester := b.Player.Get(guildID).RequesterFor(track.Info.Identifier)
	requesterStr := ""
	if requester != 0 {
		requesterStr = requester.String()
	}
	err := b.Store.RecordPlay(context.Background(),
		guildID.String(),
		track.Info.Title,
		derefString(track.Info.URI),
		track.Info.Author,
		int64(track.Info.Length),
		requesterStr,
	)
	if err != nil {
		slog.Debug("failed to record play", slog.Any("err", err))
	}
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
