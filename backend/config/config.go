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
	B2Bucket                string
	B2BucketID              string
	B2KeyID                 string
	B2AppKey                string
	B2Endpoint              string
	B2PublicURL             string
	MediaProtectedBaseURL   string
	MediaSigningSecret      string
	MediaTokenTTLSeconds    int
	MediaCookieName         string
	WorkerUploadURL         string
	TelegramChannels        []string // loaded from TELEGRAM_CHANNELS env (comma-separated)
	TelegramSerialsChannel  string   // loaded from TELEGRAM_SERIALS_CHANNEL env
}

var current *Config

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
		B2Bucket:                getEnvAny([]string{"B2_BUCKET_NAME", "B2_BUCKET"}, ""),
		B2BucketID:              getEnv("B2_BUCKET_ID", ""),
		B2KeyID:                 getEnv("B2_KEY_ID", ""),
		B2AppKey:                getEnvAny([]string{"B2_APPLICATION_KEY", "B2_APP_KEY"}, ""),
		B2Endpoint:              getEnv("B2_ENDPOINT", ""),
		B2PublicURL:             getEnv("B2_PUBLIC_URL", ""),
		MediaProtectedBaseURL:   getEnv("MEDIA_PROTECTED_BASE_URL", ""),
		MediaSigningSecret:      getEnv("MEDIA_SIGNING_SECRET", ""),
		MediaTokenTTLSeconds:    parseInt(getEnv("MEDIA_TOKEN_TTL_SECONDS", "60"), 60),
		MediaCookieName:         getEnv("MEDIA_COOKIE_NAME", "filmorauz_media"),
		WorkerUploadURL:         getEnv("WORKER_UPLOAD_URL", ""),
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

	cfg.MediaProtectedBaseURL = normalizeMediaProtectedBaseURL(cfg)

	log.Printf("=== Backend Configuration ===")
	log.Printf("Mode: %s", cfg.Mode)
	log.Printf("Worker Upload URL: %s", cfg.WorkerUploadURL)
	log.Printf("CDN URL: %s", cfg.CDNURL)
	log.Printf("Media Protected Base URL: %s", cfg.MediaProtectedBaseURL)
	if cfg.B2PublicURL != "" {
		log.Printf("B2 Public URL: %s", cfg.B2PublicURL)
	}

	if cfg.IsProd {
		if cfg.WorkerUploadURL == "" {
			log.Printf("WARNING: Worker upload service not configured - profile image uploads will fail")
		} else {
			log.Printf("Upload service: configured (%s)", cfg.WorkerUploadURL)
		}
	}

	log.Printf("Starting in %s mode", cfg.Mode)
	current = cfg
	return cfg
}

func Current() *Config {
	return current
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func normalizeMediaProtectedBaseURL(cfg *Config) string {
	if cfg == nil {
		return "/media"
	}

	value := strings.TrimSpace(cfg.MediaProtectedBaseURL)
	if value == "" {
		if cfg.IsProd {
			return "https://cdn.filmorauz.net/media"
		}
		return "/media"
	}

	if strings.HasPrefix(value, "http://") || strings.HasPrefix(value, "https://") {
		return strings.TrimSuffix(value, "/")
	}

	if value == "/media" && cfg.IsProd {
		return "https://cdn.filmorauz.net/media"
	}

	return strings.TrimSuffix(value, "/")
}

func getEnvAny(keys []string, fallback string) string {
	for _, key := range keys {
		if v := os.Getenv(key); v != "" {
			return v
		}
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

func parseInt(s string, fallback int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			if n == 0 {
				return fallback
			}
			break
		}
		n = n*10 + int(c-'0')
	}
	if n <= 0 {
		return fallback
	}
	return n
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

// GetCDNFileURL returns the full CDN URL for a file path.
// In PROD: returns CDN_URL + "/file/filmorauznet/" + path
func (c *Config) GetCDNFileURL(path string) string {
	if c.IsDev {
		return "http://localhost:" + c.Port + "/uploads/" + path
	}
	if c.B2PublicURL != "" {
		return strings.TrimSuffix(c.B2PublicURL, "/") + "/" + strings.TrimPrefix(path, "/")
	}
	return c.CDNURL + "/file/filmorauznet/" + path
}

// MediaKeyKind defines the category of media for path building
type MediaKeyKind string

const (
	MediaKeyProfile      MediaKeyKind = "profile"
	MediaKeyTelegramPost MediaKeyKind = "telegram-post"
	MediaKeyMovie        MediaKeyKind = "movie"
	MediaKeyAd           MediaKeyKind = "ad"
)

// BuildMediaKey builds the B2 object key for a media file.
// Rule: images/<category>/<filename>
// Examples:
//   - profile image: images/profile/1234567890.jpg
//   - telegram post: images/telegram-posts/1234567890.jpg
//   - movie poster: images/posters/1234567890.jpg
func BuildMediaKey(kind MediaKeyKind, filename string) string {
	switch kind {
	case MediaKeyTelegramPost:
		return "images/telegram-posts/" + filename
	case MediaKeyMovie:
		return "images/posters/" + filename
	case MediaKeyAd:
		return "images/ads/" + filename
	default:
		return "images/profile/" + filename
	}
}
