package config

import (
	"log"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

type Mode string

const (
	ModeDev  Mode = "DEV"
	ModeProd Mode = "PROD"
)

type Config struct {
	Mode                    Mode
	Port                    string
	MongoURI                string
	DBName                  string
	JWTSecret               string
	AdminTelegramID         int64
	IsDev                   bool
	IsProd                  bool
	BaseSiteURL             string
	AllowedOrigin           string
	TelegramBotUsername     string
	TelegramChannelUsername string
	WorkerUploadsDir        string
	AIEndpoint              string
	UploadsDir              string
	CDNURL                  string
	TelegramChannels        []string // loaded from TELEGRAM_CHANNELS env (comma-separated)
	TelegramSerialsChannel  string   // loaded from TELEGRAM_SERIALS_CHANNEL env
}

func Load() *Config {
	// Always try to load .env — works in both dev and prod
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, reading from environment variables")
	}

	mode := Mode(getEnv("MODE", "DEV"))
	if mode != ModeDev && mode != ModeProd {
		log.Fatalf("Invalid MODE value: '%s'. Must be DEV or PROD", mode)
	}

	cfg := &Config{
		Mode:                    mode,
		IsDev:                   mode == ModeDev,
		IsProd:                  mode == ModeProd,
		Port:                    getEnv("PORT", "8080"),
		MongoURI:                getEnv("MONGO_URI", "mongodb://localhost:27017"),
		DBName:                  getEnv("DB_NAME", "filmorauz"),
		JWTSecret:               getEnv("JWT_SECRET", ""),
		AdminTelegramID:         parseTelegramID(getEnv("ADMIN_TELEGRAM_ID", "0")),
		BaseSiteURL:             getEnv("BASE_SITE_URL", "https://filmorauz.net"),
		AllowedOrigin:           getEnv("ALLOWED_ORIGIN", ""),
		TelegramBotUsername:     getEnv("TELEGRAM_BOT_USERNAME", "FilmoraUzBot"),
		TelegramChannelUsername: getEnv("TG_CHANNEL_USERNAME", ""),
		WorkerUploadsDir:        getEnv("WORKER_UPLOADS_DIR", "../worker/uploads"),
		AIEndpoint:              getEnv("AI_ENDPOINT", ""),
		UploadsDir:              getEnv("UPLOADS_DIR", "./uploads"),
		CDNURL:                  getEnv("CDN_URL", ""),
		TelegramChannels:        parseTelegramChannels(getEnv("TELEGRAM_CHANNELS", "")),
		TelegramSerialsChannel:  getEnv("TELEGRAM_SERIALS_CHANNEL", ""),
	}

	// Validate required fields
	if cfg.JWTSecret == "" {
		if cfg.IsDev {
			log.Println("WARNING: JWT_SECRET not set, using insecure default for DEV")
			cfg.JWTSecret = "dev-only-insecure-secret"
		} else {
			log.Fatal("JWT_SECRET is required in PROD mode")
		}
	}

	if cfg.AdminTelegramID == 0 {
		log.Println("WARNING: ADMIN_TELEGRAM_ID not set, admin must be promoted via UI")
	}

	log.Printf("Starting in %s mode", cfg.Mode)
	return cfg
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// parseTelegramID parses a string Telegram ID to int64
func parseTelegramID(s string) int64 {
	var id int64
	for _, c := range s {
		if c >= '0' && c <= '9' {
			id = id*10 + int64(c-'0')
		}
	}
	return id
}

// GetEnv is exported for use in main.go
func GetEnv(key, fallback string) string {
	return getEnv(key, fallback)
}

// parseTelegramChannels splits a comma-separated channel list, trims spaces, drops empty values.
func parseTelegramChannels(s string) []string {
	var channels []string
	for _, ch := range strings.Split(s, ",") {
		ch = strings.TrimSpace(ch)
		if ch != "" {
			channels = append(channels, ch)
		}
	}
	return channels
}

// GetBaseURL returns the base URL for media assets based on MODE.
// In DEV: returns http://localhost:{PORT}/uploads
// In PROD: returns CDN_URL from environment
func (c *Config) GetBaseURL() string {
	if c.IsDev {
		return "http://localhost:" + c.Port + "/uploads"
	}
	return c.CDNURL
}
