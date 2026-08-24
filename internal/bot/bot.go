// Package bot wires the Discord client, the Lavalink client, and the player
// manager together and owns every event handler.
package bot

import (
	"sync"

	disgobot "github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"

	"hex-music-bot/internal/config"
	"hex-music-bot/internal/metrics"
	"hex-music-bot/internal/player"
	"hex-music-bot/internal/store"
)

// CommandHandler handles one slash command invocation.
type CommandHandler func(event *events.ApplicationCommandInteractionCreate, data discord.SlashCommandInteractionData) error

// Bot is the shared dependency container for all handlers.
type Bot struct {
	Cfg       *config.Config
	Store     *store.Store
	Client    *disgobot.Client
	Lavalink  *disgolink.Client
	Player    *player.Manager
	Cards     *CardManager
	Lyrics    *LyricsCache
	Sync      *LyricSyncManager
	Pagers    *PagerManager
	Metrics   *metrics.Metrics
	Cooldowns *CooldownGate
	Filters   *filterManager
	// restDead tracks nodes whose REST API failed (WAF block, proxy death);
	// BestNode skips them so playback never lands on a WS-alive/REST-dead node.
	restDead sync.Map
	failover *FailoverManager
	voice    *VoiceWatch
	Votes    *VoteManager
	Handlers map[string]CommandHandler
}

// New builds the container; Client and Lavalink are attached by main after
// their constructors run, and Handlers comes from commands.All.
func New(cfg *config.Config, st *store.Store) *Bot {
	b := &Bot{
		Cfg:      cfg,
		Store:    st,
		Player:   player.NewManager(),
		failover: NewFailoverManager(),
		Handlers: make(map[string]CommandHandler),
	}
	b.Filters = newFilterManager(b)
	b.Votes = NewVoteManager()
	b.Pagers = NewPagerManager()
	b.Sync = NewLyricSyncManager(b)
	return b
}
