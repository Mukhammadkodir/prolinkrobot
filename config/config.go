package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"

	"github.com/sulton0011/errs"
)

type Config struct {
	App      App      `json:"app"`
	Telegram Telegram `json:"telegram"`
	Mongo    Mongo    `json:"mongo"`
	Limits   Limits   `json:"limits"`
	Admin    Admin    `json:"admin"`
	Cache    Cache    `json:"cache"`
	Freepik  Freepik  `json:"freepik"`
}

type App struct {
	Name            string `yaml:"name" env:"APP_NAME"`
	Version         string `yaml:"version" env:"APP_VERSION"`
	UpdateWorkers   int    `env:"UPDATE_WORKERS"`
	UpdateQueueSize int    `env:"UPDATE_QUEUE_SIZE"`
	CacheWorkers    int    `env:"CACHE_WORKERS"`
	CacheQueueSize  int    `env:"CACHE_QUEUE_SIZE"`
}

type Telegram struct {
	BotToken         string `yaml:"bot_token" env:"BOT_TOKEN"`
	Offset           int    `yaml:"offset" env:"OFFSET"`
	Timeout          int    `yaml:"timeout" env:"TIMEOUT"`
	APIEndpoint      string `env:"TELEGRAM_API_ENDPOINT"`
	CacheAPIEndpoint string `env:"TELEGRAM_CACHE_API_ENDPOINT"`
}

type Mongo struct {
	URI                     string `env:"MONGODB_URI"`
	Database                string `env:"MONGODB_DB"`
	TelegramUsersCollection string `env:"MONGODB_TELEGRAM_USERS_COLLECTION"`
}

type Limits struct {
	Timezone          string `env:"TIMEZONE"`
	TrialPeriodDays   int    `env:"TRIAL_PERIOD_DAYS"`
	DefaultDailyLimit int    `env:"DEFAULT_DAILY_LIMIT"`
}

type Admin struct {
	TelegramIDs []int64 `env:"ADMIN_TELEGRAM_IDS"`
}

type Cache struct {
	ChannelID       int64  `env:"CACHE_CHANNEL_ID"`
	AssetCollection string `env:"MONGODB_ASSET_CACHE_COLLECTION"`
	MaxUploadBytes  int64  `env:"CACHE_MAX_UPLOAD_BYTES"`
}

type Freepik struct {
	AuthCheckIntervalMinutes int `env:"FREEPIK_AUTH_CHECK_INTERVAL_MINUTES"`
	WarnMinutes              int `env:"FREEPIK_AUTH_WARN_MINUTES"`
	CriticalMinutes          int `env:"FREEPIK_AUTH_CRITICAL_MINUTES"`
}

func NewConfig() (cfg *Config, err error) {
	defer errs.WrapLog(&err, nil, "NewConfig")

	loadDotEnv(".env")

	cfg = &Config{}
	cfg.App = App{
		Name:            envString("APP_NAME", "get-link-tg-bot"),
		Version:         envString("APP_VERSION", "0.0.1"),
		UpdateWorkers:   envInt("UPDATE_WORKERS", 8),
		UpdateQueueSize: envInt("UPDATE_QUEUE_SIZE", 128),
		CacheWorkers:    envInt("CACHE_WORKERS", 2),
		CacheQueueSize:  envInt("CACHE_QUEUE_SIZE", 128),
	}

	botToken := strings.TrimSpace(os.Getenv("BOT_TOKEN"))
	if botToken == "" {
		return nil, errs.New("BOT_TOKEN is required")
	}

	mongoURI := strings.TrimSpace(os.Getenv("MONGODB_URI"))
	if mongoURI == "" {
		return nil, errs.New("MONGODB_URI is required")
	}

	cfg.Telegram = Telegram{
		BotToken:         botToken,
		Offset:           envInt("OFFSET", 0),
		Timeout:          envInt("TIMEOUT", 60),
		APIEndpoint:      envString("TELEGRAM_API_ENDPOINT", ""),
		CacheAPIEndpoint: envString("TELEGRAM_CACHE_API_ENDPOINT", ""),
	}

	cfg.Mongo = Mongo{
		URI:                     mongoURI,
		Database:                envString("MONGODB_DB", "freepikusers"),
		TelegramUsersCollection: envString("MONGODB_TELEGRAM_USERS_COLLECTION", "telegram_users"),
	}

	cfg.Limits = Limits{
		Timezone:          envString("TIMEZONE", "Asia/Tashkent"),
		TrialPeriodDays:   envInt("TRIAL_PERIOD_DAYS", 7),
		DefaultDailyLimit: envInt("DEFAULT_DAILY_LIMIT", 1),
	}

	cfg.Admin = Admin{TelegramIDs: envInt64Slice("ADMIN_TELEGRAM_IDS")}
	cfg.Cache = Cache{
		ChannelID:       envInt64("CACHE_CHANNEL_ID", 0),
		AssetCollection: envString("MONGODB_ASSET_CACHE_COLLECTION", "asset_cache"),
		MaxUploadBytes:  envInt64("CACHE_MAX_UPLOAD_BYTES", 50*1024*1024),
	}
	cfg.Freepik = Freepik{
		AuthCheckIntervalMinutes: envInt("FREEPIK_AUTH_CHECK_INTERVAL_MINUTES", 5),
		WarnMinutes:              envInt("FREEPIK_AUTH_WARN_MINUTES", 20),
		CriticalMinutes:          envInt("FREEPIK_AUTH_CRITICAL_MINUTES", 5),
	}
	return cfg, nil
}

func envString(key, fallback string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(key string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64Slice(key string) []int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}

	parts := strings.Split(v, ",")
	ids := make([]int64, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		n, err := strconv.ParseInt(part, 10, 64)
		if err != nil {
			continue
		}
		ids = append(ids, n)
	}
	return ids
}

func envInt64(key string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}

	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func loadDotEnv(path string) {
	file, err := os.Open(path)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			continue
		}

		if len(value) >= 2 {
			if (value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'') {
				value = value[1 : len(value)-1]
			}
		}

		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}
