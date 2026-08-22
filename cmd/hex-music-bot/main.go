package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	disgobot "github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/cache"
	"github.com/disgoorg/disgo/gateway"

	"github.com/disgoorg/disgolink/v4/disgolink"

	"hex-music-bot/internal/bot"
	"hex-music-bot/internal/commands"
	"hex-music-bot/internal/config"
	"hex-music-bot/internal/metrics"
	"hex-music-bot/internal/store"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error(err.Error())
		os.Exit(1)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: cfg.LogLevel})))

	m := metrics.New()
	if cfg.MetricsAddr != "" {
		m.StartServer(cfg.MetricsAddr)
	}

	slog.Info("starting hex-music-bot",
		slog.String("nodes", fmt.Sprintf("%d configured", len(cfg.Nodes))),
		slog.String("db", cfg.DBPath),
	)

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open store", slog.Any("err", err))
		os.Exit(1)
	}
	defer st.Close()

	b := bot.New(cfg, st)
	b.Metrics = m
	b.Handlers = commands.All(b)

	client, err := disgo.New(cfg.Token,
		disgobot.WithGatewayConfigOpts(
			gateway.WithIntents(gateway.IntentGuilds, gateway.IntentGuildVoiceStates, gateway.IntentGuildMessages, gateway.IntentMessageContent),
		),
		disgobot.WithCacheConfigOpts(
			cache.WithCaches(cache.FlagVoiceStates),
		),
		disgobot.WithEventListenerFunc(b.OnApplicationCommand),
		disgobot.WithEventListenerFunc(b.OnVoiceStateUpdate),
		disgobot.WithEventListenerFunc(b.OnVoiceServerUpdate),
		disgobot.WithEventListenerFunc(b.OnComponentInteraction),
		disgobot.WithEventListenerFunc(b.OnAutocomplete),
		disgobot.WithEventListenerFunc(b.OnMessageCreate),
	)
	if err != nil {
		slog.Error("failed to build discord client", slog.Any("err", err))
		os.Exit(1)
	}
	b.Client = client

	if err = commands.Register(client, cfg.GuildID); err != nil {
		slog.Error("failed to register commands", slog.Any("err", err))
		os.Exit(1)
	}

	b.Lavalink = disgolink.New(client.ApplicationID,
		disgolink.WithListenerFunc(b.OnTrackStart),
		disgolink.WithListenerFunc(b.OnTrackEnd),
		disgolink.WithListenerFunc(b.OnTrackException),
		disgolink.WithListenerFunc(b.OnTrackStuck),
		disgolink.WithListenerFunc(b.OnWebSocketClosed),
	)

	b.Cards = bot.NewCardManager(b)
	b.Lyrics = bot.NewLyricsCache()

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err = client.OpenGateway(ctx); err != nil {
		slog.Error("failed to open gateway", slog.Any("err", err))
		os.Exit(1)
	}
	defer client.Close(context.TODO())

	connected := 0
	for _, n := range cfg.Nodes {
		node, err := b.Lavalink.AddNode(ctx, disgolink.NodeConfig{
			Name:     n.Name,
			Address:  n.Address,
			Password: n.Password,
			Secure:   n.Secure,
		})
		if err != nil {
			slog.Warn("failed to connect to lavalink node",
				slog.String("name", n.Name),
				slog.String("address", n.Address),
				slog.Any("err", err),
			)
			continue
		}
		version, err := node.Rest.Version(ctx)
		if err != nil {
			slog.Warn("node connected but version probe failed",
				slog.String("node", n.Name),
				slog.Any("err", err),
			)
		} else {
			slog.Info("lavalink node ready",
				slog.String("node", n.Name),
				slog.String("address", n.Address),
				slog.String("version", version),
			)
		}
		connected++
	}

	if connected == 0 && len(cfg.Nodes) > 0 {
		slog.Warn("no lavalink nodes connected — bot will retry in background")
	}

	go b.RestoreSnapshots()

	slog.Info("hex-music-bot is running. Press CTRL-C to exit.")
	s := make(chan os.Signal, 1)
	signal.Notify(s, syscall.SIGINT, syscall.SIGTERM, os.Interrupt)
	<-s
	slog.Info("shutting down")
}
