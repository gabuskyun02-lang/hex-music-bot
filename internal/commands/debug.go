package commands

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"

	"github.com/disgoorg/disgolink/v4/disgolink"

	hexbot "hex-music-bot/internal/bot"
)

var startTime = time.Now()

// Debug shows system, bot, and per-node diagnostics in an embed. Bot owner only.
func Debug(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	if b.Cfg.BotOwnerID == 0 || event.User().ID != b.Cfg.BotOwnerID {
		return b.Reply(event, "⛔ Debug panel is restricted to the bot owner")
	}

	inlineFalse := false
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(startTime)

	embed := discord.Embed{
		Title:       "📄 Debug Panel",
		Color:       0x5865F2,
		Description: fmt.Sprintf("```==    System Info    ==\n• Goroutines: %d\n• Heap Alloc: %.1f MB\n• Sys Memory: %.1f MB```",
			runtime.NumGoroutine(),
			float64(mem.HeapAlloc)/(1024*1024),
			float64(mem.Sys)/(1024*1024),
		),
	}

	guildCount := 0
	for range b.Client.Caches.Guilds() {
		guildCount++
	}
	activePlayers := 0
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		activePlayers += n.Stats().PlayingPlayers
		return true
	})

	embed.Fields = append(embed.Fields,
		discord.EmbedField{
			Name:   "🤖 Bot Information",
			Value:  fmt.Sprintf("```• GUILDS:  %d\n• PLAYERS: %d```", guildCount, activePlayers),
		},
		discord.EmbedField{
			Name:   "⏱ Uptime",
			Value:  formatUptime(uptime),
		},
	)

	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		stats := n.Stats()
		statusEmoji := "🟢"
		statusStr := "Connected"
		if n.Status() != disgolink.StatusConnected {
			statusEmoji = "🔴"
			statusStr = "Disconnected"
		}
		totalMem := float64(stats.Memory.Free + stats.Memory.Used)
		memPct := 0.0
		if totalMem > 0 {
			memPct = float64(stats.Memory.Used) / totalMem * 100
		}

		var info strings.Builder
		fmt.Fprintf(&info, "```• ADDRESS: %s\n", n.Config.Address)
		fmt.Fprintf(&info, "• PLAYERS: %d (playing: %d)\n", stats.Players, stats.PlayingPlayers)
		if n.Status() == disgolink.StatusConnected {
			fmt.Fprintf(&info, "• CPU:     %.1f%% system / %.1f%% lavalink\n", stats.CPU.SystemLoad*100, stats.CPU.LavalinkLoad*100)
			fmt.Fprintf(&info, "• RAM:     %.0f/%.0f MB (%.1f%%)\n",
				float64(stats.Memory.Used)/(1024*1024),
				float64(stats.Memory.Free+stats.Memory.Used)/(1024*1024), memPct)
			fmt.Fprintf(&info, "• UPTIME:  %s", formatUptime(time.Duration(stats.Uptime)))
		}
		info.WriteString("```")

		embed.Fields = append(embed.Fields, discord.EmbedField{
			Name:   fmt.Sprintf("%s %s Node — %s", statusEmoji, n.Config.Name, statusStr),
			Value:  info.String(),
			Inline: &inlineFalse,
		})
		return true
	})

	return event.CreateMessage(discord.MessageCreate{
		Embeds: []discord.Embed{embed},
	})
}

// Ping measures REST round-trip time to the best Lavalink node.
func Ping(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	node := b.Lavalink.BestNode()
	if node == nil {
		return b.Reply(event, "📡 No Lavalink node connected")
	}

	start := time.Now()
	_, err := node.Rest.Version(context.Background())
	latency := time.Since(start)

	statusEmoji := "🟢"
	if latency > 500*time.Millisecond {
		statusEmoji = "🟡"
	}
	if latency > 2*time.Second || err != nil {
		statusEmoji = "🔴"
	}
	if err != nil {
		return b.Reply(event, fmt.Sprintf("%s Node: `%s` — unreachable (%v)", statusEmoji, node.Config.Name, err))
	}
	return b.Reply(event, fmt.Sprintf("%s **%s** — REST round-trip: `%v`",
		statusEmoji, node.Config.Name, latency.Round(time.Millisecond)))
}

// Stats shows public playback statistics.
func Stats(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	uptime := time.Since(startTime)

	activeNodes := 0
	totalPlayers := 0
	totalPlaying := 0
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		if n.Status() == disgolink.StatusConnected {
			activeNodes++
			totalPlayers += n.Stats().Players
			totalPlaying += n.Stats().PlayingPlayers
		}
		return true
	})

	guildCount := 0
	for range b.Client.Caches.Guilds() {
		guildCount++
	}

	embed := discord.Embed{
		Title:       "📊 hex-music-bot Stats",
		Color:       0x5865F2,
		Description: fmt.Sprintf("• Uptime: `%s`\n• Guilds: `%d`\n• Active Nodes: `%d`\n• Total Players: `%d`\n• Currently Playing: `%d`",
			formatUptime(uptime), guildCount, activeNodes, totalPlayers, totalPlaying),
	}
	return event.CreateMessage(discord.MessageCreate{Embeds: []discord.Embed{embed}})
}

func formatUptime(d time.Duration) string {
	days := int(d.Hours()) / 24
	hours := int(d.Hours()) % 24
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh %dm", days, hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
	default:
		return fmt.Sprintf("%dm %ds", minutes, seconds)
	}
}
