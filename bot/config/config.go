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
	Mode             Mode
	IsDev            bool
	IsProd           bool
	TelegramBotToken string
	BotUsername      string // Telegram bot username (without @)
	RequiredChannels []models.RequiredChannel
	BackendBaseURL   string
	// CDNBaseURL is the absolute prefix used to resolve relative poster paths
	// (e.g. "posters/x.jpg") returned by the backend before they're handed to
	// Telegram. Required because B2 bucket visibility flips broke the older
	// inferred URLs.
	CDNBaseURL string
	// BotInternalToken is the shared secret sent to the backend when granting
	// premium after a Telegram Stars successful_payment. Must match the backend
	// BOT_INTERNAL_TOKEN env var. Required for the Stars purchase flow.
	BotInternalToken string
	// SiteURL is the public website URL shown to users who haven't linked
	// their Telegram account yet (so they can log in and try again).
	SiteURL string

	// SuperAdminTelegramIDs gates the universal video-downloader flow. Only
	// these Telegram user IDs may paste a link and download it. Loaded from
	// SUPERADMIN_TELEGRAM_IDS (comma-separated).
	SuperAdminTelegramIDs []int64
	// ParserBaseURL is where the bot reaches the Python parser (yt-dlp) for
	// the media probe/download endpoints. PARSER_SERVICE_URL (fallback PARSER_URL).
	ParserBaseURL string
	// TelegramBotAPIURL optionally points the bot at a self-hosted Telegram
	// Bot API server (raises the upload limit from ~50MB to 2GB). Empty ⇒ the
	// public api.telegram.org endpoint.
	TelegramBotAPIURL string
	// MediaMaxUploadBytes caps the file size the bot will try to upload to
	// Telegram; larger files are delivered as a download link instead.
	MediaMaxUploadBytes int64
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

	// Accept either CDN_BASE_URL (server-side) or NEXT_PUBLIC_CDN_BASE_URL
	// (shared with frontend) so the bot can reuse the existing prod env.
	cdnBase := getEnv("CDN_BASE_URL", "")
	if cdnBase == "" {
		cdnBase = getEnv("NEXT_PUBLIC_CDN_BASE_URL", "")
	}
	cdnBase = strings.TrimRight(strings.TrimSpace(cdnBase), "/")

	cfg := &Config{
		Mode:             mode,
		IsDev:            mode == ModeDev,
		IsProd:           mode == ModeProd,
		TelegramBotToken: getEnv("TELEGRAM_BOT_TOKEN", ""),
		BotUsername:      getEnv("TG_BOT_USERNAME", ""),
		RequiredChannels: channels,
		BackendBaseURL:   getEnv("BACKEND_BASE_URL", "http://localhost:8080"),
		CDNBaseURL:       cdnBase,
		BotInternalToken: getEnv("BOT_INTERNAL_TOKEN", ""),
		SiteURL:          getEnv("SITE_URL", "https://filmorauz.net"),

		SuperAdminTelegramIDs: parseInt64List(getEnv("SUPERADMIN_TELEGRAM_IDS", "")),
		ParserBaseURL: strings.TrimRight(strings.TrimSpace(
			firstNonEmptyEnv("PARSER_SERVICE_URL", "PARSER_URL", "http://127.0.0.1:8082")), "/"),
		TelegramBotAPIURL:   strings.TrimSpace(getEnv("TELEGRAM_BOT_API_URL", "")),
		MediaMaxUploadBytes: parseInt64(getEnv("MEDIA_MAX_UPLOAD_BYTES", "52428800")), // 50 MiB
	}

	log.Printf("Bot starting in %s mode", cfg.Mode)
	log.Printf("Loaded %d required channels", len(cfg.RequiredChannels))
	log.Printf("Loaded %d superadmin(s); parser=%s", len(cfg.SuperAdminTelegramIDs), cfg.ParserBaseURL)

	return cfg
}

// firstNonEmptyEnv returns the first set env var among keys, or the final
// argument as the default fallback.
func firstNonEmptyEnv(keysThenDefault ...string) string {
	if len(keysThenDefault) == 0 {
		return ""
	}
	fallback := keysThenDefault[len(keysThenDefault)-1]
	for _, k := range keysThenDefault[:len(keysThenDefault)-1] {
		if v := os.Getenv(k); v != "" {
			return v
		}
	}
	return fallback
}

// parseInt64List parses a comma-separated list of int64 Telegram IDs.
func parseInt64List(raw string) []int64 {
	var out []int64
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if v, err := strconv.ParseInt(part, 10, 64); err == nil {
			out = append(out, v)
		} else {
			log.Printf("[CONFIG] Invalid SUPERADMIN_TELEGRAM_IDS entry: %q", part)
		}
	}
	return out
}

// parseInt64 parses a single int64 with a zero fallback on error.
func parseInt64(raw string) int64 {
	if v, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64); err == nil {
		return v
	}
	return 0
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
