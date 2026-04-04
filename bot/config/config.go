package config

import (
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/filmorauz/bot/models"
	"github.com/joho/godotenv"
)

type Mode string

const (
	ModeDev  Mode = "DEV"
	ModeProd Mode = "PROD"
)

type Config struct {
	Mode              Mode
	IsDev             bool
	IsProd            bool
	TelegramBotToken  string
	BotUsername       string // Telegram bot username (without @)
	RequiredChannels  []models.RequiredChannel
	BackendBaseURL    string
}

// channelEnvPrefix is the prefix for channel env vars
const channelEnvPrefix = "TG_CHANNEL_"

func Load() *Config {
	// Try to load .env file
	_ = godotenv.Load()

	mode := Mode(getEnv("MODE", "DEV"))
	if mode != ModeDev && mode != ModeProd {
		log.Fatalf("Invalid MODE value: '%s'. Must be DEV or PROD", mode)
	}

	// Load required channels from env
	channels := loadRequiredChannels()

	cfg := &Config{
		Mode:              mode,
		IsDev:             mode == ModeDev,
		IsProd:            mode == ModeProd,
		TelegramBotToken:  getEnv("TELEGRAM_BOT_TOKEN", ""),
		BotUsername:       getEnv("TG_BOT_USERNAME", ""),
		RequiredChannels:  channels,
		BackendBaseURL:    getEnv("BACKEND_BASE_URL", "http://localhost:8080"),
	}

	log.Printf("Bot starting in %s mode", cfg.Mode)
	log.Printf("Loaded %d required channels", len(cfg.RequiredChannels))

	return cfg
}

// loadRequiredChannels loads channels from separate env vars
// Format: TG_CHANNEL_{KEY}_ID=-1001234567890, TG_CHANNEL_{KEY}_URL=https://t.me/...
func loadRequiredChannels() []models.RequiredChannel {
	var channels []models.RequiredChannel

	// Known channel keys - can be extended
	channelKeys := []string{"anime", "serials", "drama", "main", "fantasy"}

	for _, key := range channelKeys {
		idEnv := channelEnvPrefix + strings.ToUpper(key) + "_ID"
		urlEnv := channelEnvPrefix + strings.ToUpper(key) + "_URL"
		titleEnv := channelEnvPrefix + strings.ToUpper(key) + "_TITLE"

		idStr := os.Getenv(idEnv)
		urlStr := os.Getenv(urlEnv)
		titleStr := os.Getenv(titleEnv)

		// Skip if no ID configured
		if idStr == "" {
			continue
		}

		// Parse numeric ID
		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			log.Printf("[CONFIG] Invalid channel ID for %s: %s (error: %v)", key, idStr, err)
			continue
		}

		// Default title if not set
		if titleStr == "" {
			titleStr = key + " kanal"
		}

		channel := models.RequiredChannel{
			Key:   key,
			ID:    id,
			URL:   urlStr,
			Title: titleStr,
		}

		channels = append(channels, channel)
		log.Printf("[CONFIG] Loaded channel: %s (ID: %d, URL: %s)", titleStr, id, urlStr)
	}

	return channels
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
