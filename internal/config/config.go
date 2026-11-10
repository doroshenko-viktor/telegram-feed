package config

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ilyakaznacheev/cleanenv"

	"telegram_feed_summarizer/internal/domain"
)

// Config holds all configuration parameters for the application.
type Config struct {
	Telegram TelegramConfig `yaml:"telegram"`
	Storage  StorageConfig  `yaml:"storage"`
	Gemini   GeminiConfig   `yaml:"gemini"`
	Daemon   DaemonConfig   `yaml:"daemon"`
	Pipeline PipelineConfig `yaml:"pipeline"`
}

type TelegramConfig struct {
	AppID       int    `yaml:"app_id" env:"TELEGRAM_APP_ID" env-description:"Telegram MTProto App ID"`
	AppHash     string `yaml:"app_hash" env:"TELEGRAM_APP_HASH" env-description:"Telegram MTProto App Hash"`
	SessionPath string `yaml:"session_path" env:"TELEGRAM_SESSION_PATH" env-default:"~/.config/telegram-feed/session.json"`
	BotToken    string `yaml:"bot_token" env:"TELEGRAM_BOT_TOKEN" env-description:"Optional Telegram Bot Token for push digests"`
	BotChatID   string `yaml:"bot_chat_id" env:"TELEGRAM_BOT_CHAT_ID" env-description:"Optional Target Chat ID for bot push digests"`
}

type StorageConfig struct {
	Driver string `yaml:"driver" env:"STORAGE_DRIVER" env-default:"auto" env-description:"Database driver: auto, sqlite, or postgres"`
	DSN    string `yaml:"dsn" env:"STORAGE_DSN" env-default:"~/.config/telegram-feed/feed.db" env-description:"Database DSN"`
}

type GeminiConfig struct {
	APIKey      string  `yaml:"api_key" env:"GEMINI_API_KEY" env-description:"Google AI Studio API Key"`
	Model       string  `yaml:"model" env:"GEMINI_MODEL" env-default:"gemini-3.5-flash"`
	Temperature float32 `yaml:"temperature" env:"GEMINI_TEMPERATURE" env-default:"0.2"`
	MaxTokens   int     `yaml:"max_tokens" env:"GEMINI_MAX_TOKENS" env-default:"4096"`
}

type DaemonConfig struct {
	CronSchedule   string `yaml:"cron_schedule" env:"DAEMON_CRON" env-default:"0 9 * * *"`
	DeliverToSaved bool   `yaml:"deliver_to_saved" env:"DAEMON_DELIVER_TO_SAVED" env-default:"true"`
	OutputDir      string `yaml:"output_dir" env:"DAEMON_OUTPUT_DIR" env-default:"~/.config/telegram-feed/digests"`
}

type PipelineConfig struct {
	FilterAds     bool     `yaml:"filter_ads" env:"PIPELINE_FILTER_ADS" env-default:"true"`
	MinPostLength int      `yaml:"min_post_length" env:"PIPELINE_MIN_LENGTH" env-default:"20"`
	AdKeywords    []string `yaml:"ad_keywords"`
}

// HasLocalAI returns true if a valid Gemini API key is configured.
func (c *Config) HasLocalAI() bool {
	key := strings.ToLower(strings.TrimSpace(c.Gemini.APIKey))
	return key != "" && key != "no" && key != "none" && key != "false" && key != "skip"
}

// LoadConfig loads configuration from an optional file and environment variables.
func LoadConfig(path string) (*Config, error) {
	var cfg Config

	if path != "" {
		if _, err := os.Stat(path); err == nil {
			if err := cleanenv.ReadConfig(path, &cfg); err != nil {
				return nil, fmt.Errorf("reading config file %s: %w", path, err)
			}
		} else if !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking config file %s: %w", path, err)
		} else {
			// File does not exist, read pure environment variables with defaults
			if err := cleanenv.ReadEnv(&cfg); err != nil {
				return nil, fmt.Errorf("reading env variables: %w", err)
			}
		}
	} else {
		// Check standard default config path
		defaultPath := ExpandPath("~/.config/telegram-feed/config.yaml")
		if _, err := os.Stat(defaultPath); err == nil {
			if err := cleanenv.ReadConfig(defaultPath, &cfg); err != nil {
				return nil, fmt.Errorf("reading default config file %s: %w", defaultPath, err)
			}
		} else {
			if err := cleanenv.ReadEnv(&cfg); err != nil {
				return nil, fmt.Errorf("reading env variables: %w", err)
			}
		}
	}

	// Expand paths
	cfg.Telegram.SessionPath = ExpandPath(cfg.Telegram.SessionPath)
	cfg.Daemon.OutputDir = ExpandPath(cfg.Daemon.OutputDir)

	// Auto-detect storage driver
	dsn := cfg.Storage.DSN
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		cfg.Storage.Driver = "postgres"
	} else if cfg.Storage.Driver == "auto" || cfg.Storage.Driver == "" {
		cfg.Storage.Driver = "sqlite"
		cfg.Storage.DSN = ExpandPath(dsn)
	} else if cfg.Storage.Driver == "sqlite" {
		cfg.Storage.DSN = ExpandPath(dsn)
	}

	return &cfg, nil
}

// ExpandPath expands the tilde '~' in a filesystem path to the user's home directory.
func ExpandPath(path string) string {
	if strings.HasPrefix(path, "~/") || path == "~" {
		home, err := os.UserHomeDir()
		if err == nil {
			if path == "~" {
				return home
			}
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// MergeWithDB falls back to persistent database settings for any configuration options
// not explicitly provided via environment variables or the YAML config file.
func (c *Config) MergeWithDB(ctx context.Context, repo domain.Repository) {
	if repo == nil {
		return
	}

	// Telegram App ID
	if c.Telegram.AppID == 0 {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyTelegramAppID, ""); err == nil && val != "" {
			if id, err := strconv.Atoi(val); err == nil {
				c.Telegram.AppID = id
			}
		}
	}

	// Telegram App Hash
	if c.Telegram.AppHash == "" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyTelegramAppHash, ""); err == nil && val != "" {
			c.Telegram.AppHash = val
		}
	}

	// Gemini API Key
	if c.Gemini.APIKey == "" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyGeminiAPIKey, ""); err == nil && val != "" {
			c.Gemini.APIKey = val
		}
	}

	// Gemini Model
	if c.Gemini.Model == "" || c.Gemini.Model == "gemini-3.8-flash" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyGeminiModel, ""); err == nil && val != "" {
			c.Gemini.Model = val
		}
	}

	// Telegram Bot credentials
	if c.Telegram.BotToken == "" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyBotToken, ""); err == nil && val != "" {
			c.Telegram.BotToken = val
		}
	}
	if c.Telegram.BotChatID == "" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyBotChatID, ""); err == nil && val != "" {
			c.Telegram.BotChatID = val
		}
	}

	// Cron Schedule
	if c.Daemon.CronSchedule == "" || c.Daemon.CronSchedule == "0 9 * * *" {
		if val, err := repo.GetSetting(ctx, domain.SettingKeyCronSchedule, ""); err == nil && val != "" {
			c.Daemon.CronSchedule = val
		}
	}

	// Pipeline filters
	if val, err := repo.GetSetting(ctx, domain.SettingKeyFilterAds, ""); err == nil && val != "" {
		c.Pipeline.FilterAds = (val == "true" || val == "1")
	}
	if val, err := repo.GetSetting(ctx, domain.SettingKeyMinPostLength, ""); err == nil && val != "" {
		if l, err := strconv.Atoi(val); err == nil && l > 0 {
			c.Pipeline.MinPostLength = l
		}
	}
}

// SaveToDB writes the active credentials and settings into the database settings table.
func (c *Config) SaveToDB(ctx context.Context, repo domain.Repository) error {
	if repo == nil {
		return nil
	}

	if c.Telegram.AppID != 0 {
		_ = repo.SetSetting(ctx, domain.SettingKeyTelegramAppID, strconv.Itoa(c.Telegram.AppID))
	}
	if c.Telegram.AppHash != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyTelegramAppHash, c.Telegram.AppHash)
	}
	if c.Gemini.APIKey != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyGeminiAPIKey, c.Gemini.APIKey)
	}
	if c.Gemini.Model != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyGeminiModel, c.Gemini.Model)
	}
	if c.Telegram.BotToken != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyBotToken, c.Telegram.BotToken)
	}
	if c.Telegram.BotChatID != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyBotChatID, c.Telegram.BotChatID)
	}
	if c.Daemon.CronSchedule != "" {
		_ = repo.SetSetting(ctx, domain.SettingKeyCronSchedule, c.Daemon.CronSchedule)
	}

	return nil
}
