package config

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Discord   DiscordConfig   `yaml:"discord"`
	AI        AIConfig        `yaml:"ai"`
	Claude    ClaudeConfig    `yaml:"claude"`
	Gemini    GeminiConfig    `yaml:"gemini"`
	Pet       PetConfig       `yaml:"pet"`
	Monitor   MonitorConfig   `yaml:"monitor"`
	Shell     ShellConfig     `yaml:"shell"`
	Proactive ProactiveConfig `yaml:"proactive"`
}

type AIConfig struct {
	Provider string `yaml:"provider"` // "claude", "gemini", or "" (auto-detect)
}

type DiscordConfig struct {
	BotToken          string   `yaml:"bot_token"`
	ChannelID         string   `yaml:"channel_id"`
	OwnerIDs          []string `yaml:"owner_ids"`
	AllowSpectatorPet bool     `yaml:"allow_spectator_pet"`
	UseThreads        bool     `yaml:"use_threads"`
}

type ClaudeConfig struct {
	APIKey    string `yaml:"api_key"`
	Model     string `yaml:"model"`
	MaxTokens int64  `yaml:"max_tokens"`
	MaxTools  int    `yaml:"max_tool_iterations"`
	// Sliding window rate limiter
	RateLimit  int           `yaml:"rate_limit"`
	RateWindow time.Duration `yaml:"rate_window"`
}

type GeminiConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

type PetConfig struct {
	StatePath    string        `yaml:"state_path"`
	SaveInterval time.Duration `yaml:"save_interval"`
}

type MonitorConfig struct {
	Interval time.Duration `yaml:"interval"`
}

type ShellConfig struct {
	Timeout        time.Duration `yaml:"timeout"`
	MaxOutputBytes int           `yaml:"max_output_bytes"`
}

type ProactiveConfig struct {
	Enabled          bool          `yaml:"enabled"`
	CheckInterval    time.Duration `yaml:"check_interval"`
	MorningHour      int           `yaml:"morning_hour"`
	BoredomMinutes   int           `yaml:"boredom_minutes"`
	DistressCooldown time.Duration `yaml:"distress_cooldown"`
}

func Load(path string) (*Config, error) {
	cfg := defaults()

	// Load .env file first (from same directory as binary, or working dir)
	loadDotEnv(".env")

	// Load YAML config if it exists
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("reading config: %w", err)
		}
		// File doesn't exist — use defaults + env vars
	} else {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config: %w", err)
		}
	}

	// Env vars override config file (secrets live in .env or environment)
	if env := os.Getenv("DISCORD_BOT_TOKEN"); env != "" {
		cfg.Discord.BotToken = env
	}
	if env := os.Getenv("DISCORD_CHANNEL_ID"); env != "" {
		cfg.Discord.ChannelID = env
	}
	if env := os.Getenv("DISCORD_OWNER_IDS"); env != "" {
		// Comma-separated list of IDs
		ids := strings.Split(env, ",")
		var cleaned []string
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id != "" {
				cleaned = append(cleaned, id)
			}
		}
		if len(cleaned) > 0 {
			cfg.Discord.OwnerIDs = cleaned
		}
	}
	if env := os.Getenv("ANTHROPIC_API_KEY"); env != "" {
		cfg.Claude.APIKey = env
	}
	if env := os.Getenv("GOOGLE_API_KEY"); env != "" {
		cfg.Gemini.APIKey = env
	}
	if env := os.Getenv("AI_PROVIDER"); env != "" {
		cfg.AI.Provider = env
	}

	if err := validate(cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// loadDotEnv reads a .env file and sets env vars that aren't already set.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return // no .env, that's fine
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, val, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}

		key = strings.TrimSpace(key)
		val = strings.TrimSpace(val)

		// Strip surrounding quotes
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		// Only set if not already in environment
		if os.Getenv(key) == "" && val != "" {
			os.Setenv(key, val)
		}
	}
}

func defaults() *Config {
	return &Config{
		Discord: DiscordConfig{
			AllowSpectatorPet: true,
			UseThreads:        true,
		},
		Claude: ClaudeConfig{
			Model:      "claude-sonnet-4-5-20250929",
			MaxTokens:  1024,
			MaxTools:   5,
			RateLimit:  10,
			RateWindow: time.Minute,
		},
		Gemini: GeminiConfig{
			Model: "gemini-2.5-flash",
		},
		Pet: PetConfig{
			StatePath:    "state.json",
			SaveInterval: 5 * time.Minute,
		},
		Monitor: MonitorConfig{
			Interval: 30 * time.Second,
		},
		Shell: ShellConfig{
			Timeout:        10 * time.Second,
			MaxOutputBytes: 10240,
		},
		Proactive: ProactiveConfig{
			Enabled:          true,
			CheckInterval:    60 * time.Second,
			MorningHour:      8,
			BoredomMinutes:   120,
			DistressCooldown: 30 * time.Minute,
		},
	}
}

func validate(cfg *Config) error {
	if cfg.Discord.BotToken == "" {
		return fmt.Errorf("missing DISCORD_BOT_TOKEN — run ./setup.sh to configure")
	}
	if cfg.Discord.ChannelID == "" {
		return fmt.Errorf("missing DISCORD_CHANNEL_ID — run ./setup.sh to configure")
	}
	if len(cfg.Discord.OwnerIDs) == 0 {
		return fmt.Errorf("missing DISCORD_OWNER_IDS — run ./setup.sh to configure")
	}
	return nil
}
