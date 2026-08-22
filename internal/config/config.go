// Package config loads and validates all runtime configuration.
// Validation collects every problem before failing, so operators fix
// everything in one run instead of one error per restart.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"
)

// Config is the fully validated runtime configuration.
type Config struct {
	Token   string
	GuildID snowflake.ID // zero = global command registration

	LogLevel slog.Level
	DBPath   string

	Nodes []LavalinkNode

	AutoPause    bool          // pause when voice channel empties
	LeaveTimeout time.Duration // idle auto-disconnect; 0 = disabled
	MetricsAddr  string        // Prometheus /metrics; empty = disabled

	BotOwnerID snowflake.ID // /debug access control; zero = disabled
}

// LavalinkNode represents one Lavalink server connection.
type LavalinkNode struct {
	Name     string
	Address  string
	Password string
	Secure   bool
}

// Load reads .env (if present), then the process environment, and validates.
func Load() (*Config, error) {
	loadDotEnv(".env")

	var errs []error
	cfg := &Config{
		Token:       os.Getenv("DISCORD_TOKEN"),
		DBPath:      envOr("DB_PATH", "./data/hex-music-bot.db"),
		AutoPause:   envBool("AUTO_PAUSE", true),
		MetricsAddr: os.Getenv("METRICS_ADDR"),
	}
	nodes, nodeErrs := parseNodes(envOr("LAVALINK_NODES", "main:youshallnotpass@localhost:2333"))
	cfg.Nodes = nodes
	errs = append(errs, nodeErrs...)
	if raw := envOr("LEAVE_TIMEOUT_SECONDS", "300"); raw != "0" {
		if secs, err := strconv.Atoi(raw); err == nil && secs > 0 {
			cfg.LeaveTimeout = time.Duration(secs) * time.Second
		} else {
			errs = append(errs, fmt.Errorf("LEAVE_TIMEOUT_SECONDS %q invalid: use a positive integer or 0 to disable", raw))
		}
	}

	if cfg.Token == "" {
		errs = append(errs, fmt.Errorf("DISCORD_TOKEN is required (https://discord.com/developers/applications)"))
	}
	if raw := os.Getenv("GUILD_ID"); raw != "" {
		id, err := snowflake.Parse(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("GUILD_ID %q is not a valid snowflake ID", raw))
		}
		cfg.GuildID = id
	}
	if raw := os.Getenv("BOT_OWNER_ID"); raw != "" {
		id, err := snowflake.Parse(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("BOT_OWNER_ID %q is not a valid snowflake ID", raw))
		}
		cfg.BotOwnerID = id
	} else {
		errs = append(errs, fmt.Errorf("BOT_OWNER_ID is required — set it to your Discord user ID for /debug access"))
	}
	switch v := strings.ToLower(os.Getenv("LOG_LEVEL")); v {
	case "", "info":
		cfg.LogLevel = slog.LevelInfo
	case "debug":
		cfg.LogLevel = slog.LevelDebug
	case "warn":
		cfg.LogLevel = slog.LevelWarn
	case "error":
		cfg.LogLevel = slog.LevelError
	default:
		errs = append(errs, fmt.Errorf("LOG_LEVEL %q invalid: use debug|info|warn|error", v))
	}
	if cfg.DBPath == "" {
		errs = append(errs, fmt.Errorf("DB_PATH must not be empty"))
	}

	if len(errs) > 0 {
		return nil, joinErrs(errs)
	}
	return cfg, nil
}

// parseNodes parses LAVALINK_NODES entries in the format
// "name:password@host:port,name:password@host:port,..."
func parseNodes(raw string) ([]LavalinkNode, []error) {
	var nodes []LavalinkNode
	var errs []error
	for _, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		node, err := parseNode(entry)
		if err != nil {
			errs = append(errs, fmt.Errorf("LAVALINK_NODES entry %q invalid: %v", entry, err))
			continue
		}
		nodes = append(nodes, node)
	}
	if len(nodes) == 0 && len(errs) == 0 {
		errs = append(errs, fmt.Errorf("LAVALINK_NODES produced zero valid nodes"))
	}
	return nodes, errs
}

func parseNode(entry string) (LavalinkNode, error) {
	var node LavalinkNode

	// name:password@host:port
	atIdx := strings.LastIndex(entry, "@")
	if atIdx < 0 {
		return node, fmt.Errorf("missing @ separator")
	}
	userPart := entry[:atIdx]
	hostPart := entry[atIdx+1:]

	colonIdx := strings.Index(userPart, ":")
	if colonIdx >= 0 {
		node.Name = userPart[:colonIdx]
		node.Password = userPart[colonIdx+1:]
	} else {
		node.Name = userPart
	}

	if node.Name == "" {
		return node, fmt.Errorf("empty node name")
	}

	// Explicit secure flag via ?secure or ?insecure suffix takes priority,
	// then protocol prefix on host, then port 443 heuristic.
	secureExplicit := ""
	if idx := strings.LastIndex(hostPart, "?"); idx >= 0 {
		secureExplicit = strings.ToLower(hostPart[idx+1:])
		hostPart = hostPart[:idx]
	}

	switch secureExplicit {
	case "secure":
		node.Secure = true
	case "insecure":
		node.Secure = false
	default:
		if strings.HasPrefix(hostPart, "wss://") || strings.HasPrefix(hostPart, "https://") {
			node.Secure = true
			hostPart = strings.TrimPrefix(strings.TrimPrefix(hostPart, "wss://"), "https://")
		} else if strings.HasPrefix(hostPart, "ws://") || strings.HasPrefix(hostPart, "http://") {
			node.Secure = false
			hostPart = strings.TrimPrefix(strings.TrimPrefix(hostPart, "ws://"), "http://")
		} else if strings.HasSuffix(hostPart, ":443") {
			node.Secure = true
		}
	}

	if hostPart == "" {
		return node, fmt.Errorf("empty address")
	}
	node.Address = hostPart
	return node, nil
}


func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if parsed, err := strconv.ParseBool(v); err == nil {
			return parsed
		}
	}
	return fallback
}

func joinErrs(errs []error) error {
	msgs := make([]string, len(errs))
	for i, err := range errs {
		msgs[i] = fmt.Sprintf("  - %v", err)
	}
	return fmt.Errorf("invalid configuration:\n%s", strings.Join(msgs, "\n"))
}

// loadDotEnv reads simple KEY=VALUE pairs into the process environment.
// Variables already present always win.
func loadDotEnv(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if !found || key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
