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

func protectMediaURL(raw string) string {
	cfg := config.Current()
	if cfg == nil || raw == "" || cfg.MediaSigningSecret == "" {
		return raw
	}

	mediaPath, originQS, ok := extractMediaPath(raw)
	if !ok {
		return raw
	}

	token, expiresAt := buildMediaToken(cfg.MediaSigningSecret, mediaPath, cfg.MediaTokenTTLSeconds)
	base := strings.TrimSuffix(cfg.MediaProtectedBaseURL, "/")
	if base == "" {
		base = "/media"
	}

	protected := base + mediaPath + "?token=" + url.QueryEscape(token)
	if originQS != "" {
		protected += "&origin_qs=" + url.QueryEscape(originQS)
	}
	_ = expiresAt
	return protected
}

func buildMediaToken(secret, mediaPath string, ttlSeconds int) (string, time.Time) {
	if ttlSeconds <= 0 {
		ttlSeconds = 900
	}
	expiresAt := time.Now().Add(time.Duration(ttlSeconds) * time.Second)
	scopePath := scopedMediaPath(mediaPath)
	exp := strconv.FormatInt(expiresAt.Unix(), 10)
	sig := signMediaScope(secret, scopePath, exp)
	payload := scopePath + "\n" + exp + "\n" + sig
	return base64.RawURLEncoding.EncodeToString([]byte(payload)), expiresAt
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
		if idx := strings.Index(candidate, "/file/filmorauznet/"); idx >= 0 {
			return "/" + strings.TrimPrefix(candidate[idx+len("/file/filmorauznet/"):], "/"), u.RawQuery, true
		}
		for _, prefix := range []string{"/videos/", "/avatars/", "/movies/", "/series/", "/collections/", "/ads/", "/telegram-posts/", "/suggestions/", "/posters/", "/backdrops/"} {
			if idx := strings.Index(candidate, prefix); idx >= 0 {
				return candidate[idx:], u.RawQuery, true
			}
		}
	}

	return "", "", false
}

func scopedMediaPath(requestPath string) string {
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

func signMediaScope(secret, scopePath, exp string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(scopePath))
	mac.Write([]byte("\n"))
	mac.Write([]byte(exp))
	return hex.EncodeToString(mac.Sum(nil))
}
