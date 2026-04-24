package handlers

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
)

type mediaTokenOptions struct {
	ClientIP string
	UAHash   string
}

var mediaPathPrefixes = []string{
	"/videos/",
	"/images/",
	"/movies/",
	"/series/",
	"/collections/",
	"/ads/",
	"/telegram-posts/",
	"/suggestions/",
	"/posters/",
	"/backdrops/",
	"/avatars/",
}

func requiresMediaToken(mediaPath string) bool {
	lower := strings.ToLower(strings.TrimSpace(mediaPath))
	if lower == "" {
		return false
	}
	if strings.HasSuffix(lower, ".m3u8") || strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".m4s") {
		return true
	}
	return strings.HasPrefix(lower, "/videos/")
}

func protectMediaURL(raw string) string {
	cfg := config.Current()
	if cfg == nil || raw == "" || cfg.MediaSigningSecret == "" {
		return raw
	}

	mediaPath, originQS, ok := extractMediaPath(raw)
	if !ok {
		return raw
	}

	token, expiresAt := buildMediaToken(cfg.MediaSigningSecret, mediaPath, cfg.MediaTokenTTLSeconds, mediaTokenOptions{})
	base := strings.TrimSuffix(cfg.MediaProtectedBaseURL, "/")
	if base == "" {
		base = "/media"
	}

	if !requiresMediaToken(mediaPath) {
		protected := base + mediaPath
		if originQS != "" {
			protected += "?origin_qs=" + url.QueryEscape(originQS)
		}
		return protected
	}

	protected := base + mediaPath + "?token=" + url.QueryEscape(token)
	if originQS != "" {
		protected += "&origin_qs=" + url.QueryEscape(originQS)
	}
	_ = expiresAt
	return protected
}

func buildMediaToken(secret, mediaPath string, ttlSeconds int, opts mediaTokenOptions) (string, time.Time) {
	if ttlSeconds <= 0 {
		ttlSeconds = 60
	}
	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	scopePath := tokenScopePath(mediaPath)
	exp := strconv.FormatInt(expiresAt.Unix(), 10)
	sig := signMediaScope(secret, scopePath, exp, opts.ClientIP, opts.UAHash)
	payload := scopePath + "\n" + exp + "\n" + sig + "\n" + opts.ClientIP + "\n" + opts.UAHash
	return base64.RawURLEncoding.EncodeToString([]byte(payload)), expiresAt
}

func hashUserAgent(userAgent string) string {
	sum := sha256.Sum256([]byte(userAgent))
	return hex.EncodeToString(sum[:])
}

func extractMediaPath(raw string) (string, string, bool) {
	u, err := url.Parse(raw)
	if err != nil {
		return "", "", false
	}

	candidates := []string{u.Path, raw}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "/media/") {
			return "/" + strings.TrimPrefix(candidate, "/media/"), u.RawQuery, true
		}
		if idx := strings.Index(candidate, "/file/filmorauznet/"); idx >= 0 {
			return "/" + strings.TrimPrefix(candidate[idx+len("/file/filmorauznet/"):], "/"), u.RawQuery, true
		}
		for _, prefix := range mediaPathPrefixes {
			if idx := strings.Index(candidate, prefix); idx >= 0 {
				return candidate[idx:], u.RawQuery, true
			}
		}
	}

	return "", "", false
}

func tokenScopePath(requestPath string) string {
	lower := strings.ToLower(requestPath)
	if strings.HasSuffix(lower, ".m3u8") {
		dir := path.Dir(requestPath)
		if dir == "." || dir == "/" {
			return "/"
		}
		return strings.TrimSuffix(dir, "/") + "/"
	}
	if strings.HasSuffix(lower, ".ts") || strings.HasSuffix(lower, ".m4s") {
		dir := path.Dir(requestPath)
		if dir == "." || dir == "/" {
			return "/"
		}
		return strings.TrimSuffix(dir, "/") + "/"
	}
	return requestPath
}

func signMediaScope(secret, scopePath, exp, clientIP, uaHash string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	if clientIP == "" && uaHash == "" {
		mac.Write([]byte(scopePath))
		mac.Write([]byte("\n"))
		mac.Write([]byte(exp))
		return hex.EncodeToString(mac.Sum(nil))
	}
	mac.Write([]byte(scopePath))
	mac.Write([]byte("\n"))
	mac.Write([]byte(exp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(clientIP))
	mac.Write([]byte("\n"))
	mac.Write([]byte(uaHash))
	return hex.EncodeToString(mac.Sum(nil))
}
