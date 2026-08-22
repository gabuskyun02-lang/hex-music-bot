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

	NodeName     string
	NodeAddress  string
	NodePassword string
	NodeSecure   bool

	AutoPause    bool          // pause when voice channel empties
	LeaveTimeout time.Duration // idle auto-disconnect; 0 = disabled
	MetricsAddr  string        // Prometheus /metrics; empty = disabled
}

// Load reads .env (if present), then the process environment, and validates.
func Load() (*Config, error) {
	loadDotEnv(".env")

	var errs []error
	cfg := &Config{
		Token:        os.Getenv("DISCORD_TOKEN"),
		DBPath:       envOr("DB_PATH", "./data/hex-music-bot.db"),
		NodeName:     envOr("LAVALINK_NODE_NAME", "main"),
		NodeAddress:  envOr("LAVALINK_ADDRESS", "localhost:2333"),
		NodePassword: os.Getenv("LAVALINK_PASSWORD"),
		AutoPause:    envBool("AUTO_PAUSE", true),
		MetricsAddr:  os.Getenv("METRICS_ADDR"),
	}
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
	if cfg.NodePassword == "" {
		cfg.NodePassword = "youshallnotpass"
		slog.Warn("LAVALINK_PASSWORD not set, falling back to Lavalink default")
	}
	if raw := os.Getenv("GUILD_ID"); raw != "" {
		id, err := snowflake.Parse(raw)
		if err != nil {
			errs = append(errs, fmt.Errorf("GUILD_ID %q is not a valid snowflake ID", raw))
		}
		cfg.GuildID = id
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
	switch v := strings.ToLower(os.Getenv("LAVALINK_SECURE")); v {
	case "", "false", "0", "no":
		cfg.NodeSecure = false
	case "true", "1", "yes":
		cfg.NodeSecure = true
	default:
		errs = append(errs, fmt.Errorf("LAVALINK_SECURE %q invalid: use true|false", v))
	}
	if cfg.DBPath == "" {
		errs = append(errs, fmt.Errorf("DB_PATH must not be empty"))
	}

	if len(errs) > 0 {
		return nil, joinErrs(errs)
	}
	return cfg, nil
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
