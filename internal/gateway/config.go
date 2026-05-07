package gateway

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	SlackAppToken   string        `toml:"slack_app_token"`
	SlackBotToken   string        `toml:"slack_bot_token"`
	OwnerID         string        `toml:"owner_id"`
	IdlePollInterval time.Duration `toml:"-"`
	IdleTimeout      time.Duration `toml:"-"`
	CaptureLines    int           `toml:"capture_lines"`
}

type rawConfig struct {
	Gateway struct {
		SlackAppToken    string `toml:"slack_app_token"`
		SlackBotToken    string `toml:"slack_bot_token"`
		OwnerID          string `toml:"owner_id"`
		IdlePollInterval string `toml:"idle_poll_interval"`
		IdleTimeout      string `toml:"idle_timeout"`
		CaptureLines     int    `toml:"capture_lines"`
	} `toml:"gateway"`
}

func LoadConfig() (*Config, error) {
	cfg := &Config{
		IdlePollInterval: 2 * time.Second,
		IdleTimeout:      10 * time.Minute,
		CaptureLines:     100,
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return cfg, nil
	}

	path := filepath.Join(home, ".config", "agmux", "gateway.toml")
	if _, err := os.Stat(path); err != nil {
		// Try env vars only
		cfg.applyEnv()
		return cfg, cfg.validate()
	}

	var raw rawConfig
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}

	g := raw.Gateway
	cfg.SlackAppToken = g.SlackAppToken
	cfg.SlackBotToken = g.SlackBotToken
	cfg.OwnerID = g.OwnerID
	if g.CaptureLines > 0 {
		cfg.CaptureLines = g.CaptureLines
	}
	if g.IdlePollInterval != "" {
		if d, err := time.ParseDuration(g.IdlePollInterval); err == nil {
			cfg.IdlePollInterval = d
		}
	}
	if g.IdleTimeout != "" {
		if d, err := time.ParseDuration(g.IdleTimeout); err == nil {
			cfg.IdleTimeout = d
		}
	}

	cfg.applyEnv()
	return cfg, cfg.validate()
}

func (c *Config) applyEnv() {
	if v := os.Getenv("AGMUX_SLACK_APP_TOKEN"); v != "" {
		c.SlackAppToken = v
	}
	if v := os.Getenv("AGMUX_SLACK_BOT_TOKEN"); v != "" {
		c.SlackBotToken = v
	}
	if v := os.Getenv("AGMUX_OWNER_ID"); v != "" {
		c.OwnerID = v
	}
}

func (c *Config) validate() error {
	if c.SlackAppToken == "" {
		return fmt.Errorf("slack_app_token not set (config or AGMUX_SLACK_APP_TOKEN env)")
	}
	if c.SlackBotToken == "" {
		return fmt.Errorf("slack_bot_token not set (config or AGMUX_SLACK_BOT_TOKEN env)")
	}
	if c.OwnerID == "" {
		return fmt.Errorf("owner_id not set (config or AGMUX_OWNER_ID env)")
	}
	return nil
}
