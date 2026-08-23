package commands

import (
	"context"
	"fmt"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"runtime"
	"strings"
	"sync"
	"time"

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
		Title: "📄 Debug Panel",
		Color: 0x5865F2,
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
			Name:  "🤖 Bot Information",
			Value: fmt.Sprintf("```• GUILDS:  %d\n• PLAYERS: %d```", guildCount, activePlayers),
		},
		discord.EmbedField{
			Name:  "⏱ Uptime",
			Value: formatUptime(uptime),
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

// Ping measures REST round-trip time on every connected node. Uses /v4/info
// (the route playback actually depends on) — some hosts' WAFs block the bare
// /version path while /v4/* works fine.
func Ping(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	// Acknowledge immediately — probing every node can exceed Discord's
	// 3-second interaction window (one dead node costs ~3s on its own).
	if err := event.DeferCreateMessage(false); err != nil {
		return err
	}

	nodes := map[string]*disgolink.Node{}
	var mu sync.Mutex
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		mu.Lock()
		nodes[n.Config.Name] = n
		mu.Unlock()
		return true
	})
	if len(nodes) == 0 {
		return b.EditReply(event, "📡 No Lavalink node connected")
	}

	type result struct {
		line string
	}
	results := make(chan result, len(nodes))
	for _, n := range nodes {
		go func(n *disgolink.Node) {
			start := time.Now()
			ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
			defer cancel()
			info, err := n.Rest.Info(ctx)
			latency := time.Since(start).Round(time.Millisecond)
			if err != nil {
				b.MarkNodeRestDead(n.Config.Name)
			} else {
				b.ClearNodeRestDead(n.Config.Name)
			}

			status := "🟢"
			switch {
			case err != nil:
				status = "🔴"
			case latency > 500*time.Millisecond:
				status = "🟡"
			}
			line := fmt.Sprintf("%s `%s` — %v", status, n.Config.Name, latency)
			if err == nil && info != nil {
				line += fmt.Sprintf(" · v%s · %d source(s)", info.Version.Semver[:min(7, len(info.Version.Semver))], len(info.SourceManagers))
			}
			if err != nil {
				line += " — unreachable"
			}
			results <- result{line: line}
		}(n)
	}

	lines := make([]string, 0, len(nodes))
	for range nodes {
		lines = append(lines, (<-results).line)
	}

	sb := strings.Builder{}
	sb.WriteString("**📡 Lavalink nodes**\n")
	for _, l := range lines {
		sb.WriteString(l + "\n")
	}
	return b.EditReply(event, sb.String())
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
		Title: "📊 hex-music-bot Stats",
		Color: 0x5865F2,
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
