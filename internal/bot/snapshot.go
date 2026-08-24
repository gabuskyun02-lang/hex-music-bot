package bot

import (
	"context"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/disgoorg/disgolink/v4/disgolink"
	"github.com/disgoorg/disgolink/v4/lavalink"
	"github.com/disgoorg/snowflake/v2"

	"hex-music-bot/internal/player"
	"hex-music-bot/internal/store"
)

// SaveSnapshot captures current playback state for 24/7 restart survival.
func (b *Bot) SaveSnapshot(guildID snowflake.ID) {
	p := b.Lavalink.ExistingPlayer(guildID)
	state := b.Player.Get(guildID)

	snap := &store.PlayerSnapshot{GuildID: guildID.String()}
	if chID, ok := b.voice.ChannelFor(guildID); ok {
		snap.VoiceChannelID = chID.String()
	}
	if entry := b.Cards.Lookup(guildID); entry != nil {
		snap.TextChannelID = entry.channelID.String()
	}
	if p != nil && p.Track != nil {
		snap.CurrentIdentifier = p.Track.Info.Identifier
		snap.CurrentPositionMS = int64(p.Position())
		snap.Volume = p.Volume
	}
	for _, t := range state.Snapshot() {
		snap.Queue = append(snap.Queue, t.Info.Identifier)
	}
	snap.LoopMode = state.LoopMode().String()
	if p != nil {
		snap.Paused = p.Paused
	}
	snap.Shuffled = state.Shuffled()
	if prev, ok := state.PeekHistory(); ok {
		snap.PreviousIdentifier = prev.Info.Identifier
	}

	if err := b.Store.SavePlayerSnapshot(context.Background(), snap); err != nil {
		slog.Debug("snapshot save failed", slog.Any("err", err))
	} else {
		slog.Debug("snapshot saved", slog.String("guild", guildID.String()))
	}
}

// RestoreSnapshots checks for saved player states on boot and resumes any
// guilds that had 24/7 mode enabled.
func (b *Bot) RestoreSnapshots() {
	snaps, err := b.Store.AllSnapshots(context.Background())
	if err != nil {
		slog.Error("snapshot load failed", slog.Any("err", err))
		return
	}
	for _, snap := range snaps {
		guildID, err := snowflake.Parse(snap.GuildID)
		if err != nil {
			continue
		}
		settings, err := b.Store.GetGuildSettings(context.Background(), snap.GuildID)
		if err != nil || !settings.Mode247 || snap.VoiceChannelID == "" {
			continue
		}

		vcID, vcErr := snowflake.Parse(snap.VoiceChannelID)
		if vcErr != nil {
			continue
		}
		if err := b.Client.UpdateVoiceState(context.TODO(), guildID, &vcID, false, false); err != nil {
			slog.Error("24/7 rejoin failed", slog.String("guild", snap.GuildID), slog.Any("err", err))
			continue
		}

		node := b.BestHealthyNode()
		if node == nil {
			continue
		}
		pl := b.Lavalink.Player(guildID)

		if snap.LoopMode == "track" {
			b.Player.Get(guildID).SetLoopMode(player.LoopTrack)
		} else if snap.LoopMode == "queue" {
			b.Player.Get(guildID).SetLoopMode(player.LoopQueue)
		}
		if snap.Shuffled {
			b.Player.Get(guildID).SetShuffled(true)
		}
		if snap.CurrentIdentifier != "" {
			go b.restorePlayback(node, pl, guildID, snap)
		} else if len(snap.Queue) > 0 {
			go func() {
				time.Sleep(2 * time.Second) // allow the voice connection to establish
				b.restoreQueue(node, guildID, snap.Queue)
				if first, ok := b.Player.Get(guildID).Next(); ok {
					if snap.Volume > 0 && snap.Volume != 100 {
						_ = pl.Update(context.TODO(), disgolink.WithTrack(first), disgolink.WithVolume(snap.Volume))
					} else {
						_ = pl.Update(context.TODO(), disgolink.WithTrack(first))
					}
				}
			}()
		}
		if snap.PreviousIdentifier != "" {
			go func() {
				node.Rest.LoadTracksHandler(context.Background(), snap.PreviousIdentifier, disgolink.NewTrackLoadingResultHandler(
					func(t lavalink.Track) { b.Player.Get(guildID).PushHistory(t) },
					func(p lavalink.Playlist) {
						if len(p.Tracks) > 0 {
							b.Player.Get(guildID).PushHistory(p.Tracks[0])
						}
					},
					func(ts []lavalink.Track) {
						if len(ts) > 0 {
							b.Player.Get(guildID).PushHistory(ts[0])
						}
					},
					func() {},
					func(err error) {}, // convenience, not critical: silent skip
				))
			}()
		}
		if snap.TextChannelID != "" {
			if chID, err := snowflake.Parse(snap.TextChannelID); err == nil {
				go func() {
					time.Sleep(3 * time.Second) // after playback restore starts
					b.Cards.Create(guildID, chID)
				}()
			}
		}
		slog.Info("24/7 session restored", slog.String("guild", snap.GuildID))
	}
}

func (b *Bot) restorePlayback(node *disgolink.Node, pl *disgolink.Player, guildID snowflake.ID, snap *store.PlayerSnapshot) {
	time.Sleep(2 * time.Second) // allow the voice connection to establish
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var resolved *lavalink.Track
	node.Rest.LoadTracksHandler(ctx, snap.CurrentIdentifier, disgolink.NewTrackLoadingResultHandler(
		func(t lavalink.Track) { resolved = &t },
		func(p lavalink.Playlist) {},
		func(ts []lavalink.Track) {
			if len(ts) > 0 {
				resolved = &ts[0]
			}
		},
		func() {},
		func(err error) {},
	))
	if resolved == nil {
		b.restoreQueue(node, guildID, snap.Queue)
		return
	}

	// Single REST call: track + volume + position + paused land atomically, so
	// the track never plays audibly before the seek lands.
	opts := []disgolink.PlayerUpdateOpt{
		disgolink.WithTrack(*resolved),
		disgolink.WithPosition(lavalink.Duration(snap.CurrentPositionMS)),
		disgolink.WithPaused(snap.Paused),
	}
	if snap.Volume > 0 && snap.Volume != 100 {
		opts = append(opts, disgolink.WithVolume(snap.Volume))
	}
	_ = pl.Update(ctx, opts...)
	b.restoreQueue(node, guildID, snap.Queue)
	slog.Info("24/7 restored",
		slog.String("guild", snap.GuildID),
		slog.String("track", resolved.Info.Title),
	)
}

func (b *Bot) restoreQueue(node *disgolink.Node, guildID snowflake.ID, queue []string) {
	// Resolve in parallel, collect by index: order follows the saved queue,
	// not HTTP completion order. Serial fallback on any resolution error.
	resolved := make([][]lavalink.Track, len(queue))
	failed := make([]bool, len(queue))

	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // bound concurrent LoadTracks calls per node
	for i, id := range queue {
		wg.Add(1)
		go func(i int, id string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			node.Rest.LoadTracksHandler(ctx, id, disgolink.NewTrackLoadingResultHandler(
				func(t lavalink.Track) { resolved[i] = append(resolved[i], t) },
				func(p lavalink.Playlist) { resolved[i] = append(resolved[i], p.Tracks...) },
				func(ts []lavalink.Track) { resolved[i] = append(resolved[i], ts...) },
				func() {},
				func(err error) { failed[i] = true },
			))
		}(i, id)
	}
	wg.Wait()

	state := b.Player.Get(guildID)
	for _, tracks := range resolved {
		for _, t := range tracks {
			state.Enqueue(t)
		}
	}
	if slices.Contains(failed, true) {
		// Serial fallback: clear the partial queue (loop mode and history are
		// set by the caller, so ClearQueue — not state deletion) and re-resolve
		// one at a time in strict order.
		state.ClearQueue()
		for _, id := range queue {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			node.Rest.LoadTracksHandler(ctx, id, disgolink.NewTrackLoadingResultHandler(
				func(t lavalink.Track) { state.Enqueue(t) },
				func(p lavalink.Playlist) {
					for _, t := range p.Tracks {
						state.Enqueue(t)
					}
				},
				func(ts []lavalink.Track) {
					for _, t := range ts {
						state.Enqueue(t)
					}
				},
				func() {},
				func(err error) {},
			))
		}
	}
}
