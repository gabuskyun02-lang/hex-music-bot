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
	"github.com/disgoorg/disgolink/v4/lavalink"

	hexbot "hex-music-bot/internal/bot"
)

var startTime = time.Now()

// Debug shows system, bot, and per-node diagnostics. Bot owner only.
func Debug(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	ownerID := b.Cfg.GuildID // TODO: dedicated BOT_OWNER_ID env var
	if ownerID == 0 || event.User().ID != ownerID {
		return b.Reply(event, "⛔ Debug panel is restricted to the bot owner")
	}

	var sb strings.Builder

	// System info
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	uptime := time.Since(startTime)
	fmt.Fprintf(&sb, "**🖥 System**\n")
	fmt.Fprintf(&sb, "• Uptime: `%s`\n", formatUptime(uptime))
	fmt.Fprintf(&sb, "• Goroutines: `%d`\n", runtime.NumGoroutine())
	fmt.Fprintf(&sb, "• Heap Alloc: `%.1f MB`\n", float64(mem.HeapAlloc)/(1024*1024))
	fmt.Fprintf(&sb, "• Sys Memory: `%.1f MB`\n\n", float64(mem.Sys)/(1024*1024))

	// Bot info
	guildCount := 0
	for range b.Client.Caches.Guilds() {
		guildCount++
	}
	activePlayers := 0
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		activePlayers += n.Stats().PlayingPlayers
		return true
	})
	fmt.Fprintf(&sb, "**🤖 Bot**\n")
	fmt.Fprintf(&sb, "• Guilds: `%d`\n", guildCount)
	fmt.Fprintf(&sb, "• Active Players: `%d`\n", activePlayers)
	fmt.Fprintf(&sb, "• DB: `%s`\n\n", b.Cfg.DBPath)

	// Per-node status
	nodeCount := 0
	b.Lavalink.Nodes()(func(n *disgolink.Node) bool {
		nodeCount++
		stats := n.Stats()
		status := "🟢 Connected"
		if n.Status() != disgolink.StatusConnected {
			status = "🔴 Disconnected"
		}
		fmt.Fprintf(&sb, "**📡 %s** (%s)\n", n.Config.Name, status)
		if n.Status() == disgolink.StatusConnected {
			memFree := float64(stats.Memory.Free) / (1024 * 1024)
			memUsed := float64(stats.Memory.Used) / (1024 * 1024)
			totalMem := memFree + memUsed
			var pct float64
			if totalMem > 0 {
				pct = memUsed / totalMem * 100
			}
			fmt.Fprintf(&sb, "• Address: `%s`\n", n.Config.Address)
			fmt.Fprintf(&sb, "• Players: `%d` (playing: `%d`)\n", stats.Players, stats.PlayingPlayers)
			fmt.Fprintf(&sb, "• CPU: `%.1f%%` system / `%.1f%%` lavalink\n", stats.CPU.SystemLoad*100, stats.CPU.LavalinkLoad*100)
			fmt.Fprintf(&sb, "• RAM: `%.0f/%.0f MB` (`%.1f%%`)\n", memUsed, totalMem, pct)
			fmt.Fprintf(&sb, "• Uptime: `%s`\n", formatUptime(time.Duration(stats.Uptime)))
		} else {
			fmt.Fprintf(&sb, "• Address: `%s`\n", n.Config.Address)
		}
		sb.WriteString("\n")
		return true
	})
	if nodeCount == 0 {
		sb.WriteString("**📡 Nodes:** None connected\n")
	}

	return b.Reply(event, sb.String())
}

// Ping shows gateway latency.
func Ping(b *hexbot.Bot, event *events.ApplicationCommandInteractionCreate, _ discord.SlashCommandInteractionData) error {
	return b.Reply(event, fmt.Sprintf("🏓 Pong!"))
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

	return b.Reply(event, fmt.Sprintf(
		"📊 **hex-music-bot Stats**\n"+
			"• Uptime: `%s`\n"+
			"• Guilds: `%d`\n"+
			"• Active Nodes: `%d`\n"+
			"• Total Players: `%d`\n"+
			"• Currently Playing: `%d`",
		formatUptime(uptime), guildCount, activeNodes, totalPlayers, totalPlaying,
	))
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

// unused imports guard — remove if not needed after wiring
var _ = lavalink.Duration(0)
var _ = context.Background
