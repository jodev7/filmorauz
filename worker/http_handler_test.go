package main

import (
	"regexp"
	"strings"
	"testing"
)

func TestSafeUploadKeyUsesSafeProfileImagePath(t *testing.T) {
	filename, key := safeUploadKey("images/profile", "profile", "My Profile, 50% готово.PNG", "image/png", ".jpg")

	if !regexp.MustCompile(`^images/profile/[0-9]+_profile\.png$`).MatchString(key) {
		t.Fatalf("unexpected key: %q", key)
	}
	if !regexp.MustCompile(`^[0-9]+_profile\.png$`).MatchString(filename) {
		t.Fatalf("unexpected filename: %q", filename)
	}
	if strings.ContainsAny(key, " ,%") {
		t.Fatalf("key contains unsafe characters: %q", key)
	}
}

func TestSafeUploadKeyUsesSafeTelegramPostPath(t *testing.T) {
	filename, key := safeUploadKey("images/telegram-post", "telegram_post", "poster, sale @ 50%.jpg", "image/jpeg", ".jpg")

	if !regexp.MustCompile(`^images/telegram-post/[0-9]+_telegram_post\.jpg$`).MatchString(key) {
		t.Fatalf("unexpected key: %q", key)
	}
	if !regexp.MustCompile(`^[0-9]+_telegram_post\.jpg$`).MatchString(filename) {
		t.Fatalf("unexpected filename: %q", filename)
	}
	if strings.ContainsAny(key, " ,%@") {
		t.Fatalf("key contains unsafe characters: %q", key)
	}
}

func TestSafeUploadKeyUsesFinalMovieAssetPath(t *testing.T) {
	_, posterKey := safeUploadKey("posters", "poster", "poster, final.png", "image/png", ".jpg")
	if !regexp.MustCompile(`^posters/[0-9]+_poster\.png$`).MatchString(posterKey) {
		t.Fatalf("unexpected poster key: %q", posterKey)
	}

	_, backdropKey := safeUploadKey("backdrops", "backdrop", "backdrop, final.png", "image/png", ".jpg")
	if !regexp.MustCompile(`^backdrops/[0-9]+_backdrop\.png$`).MatchString(backdropKey) {
		t.Fatalf("unexpected backdrop key: %q", backdropKey)
	}
}

func TestSafeUploadKeyFallsBackToSafeExtension(t *testing.T) {
	_, key := safeUploadKey("temp/raw", "video", "movie name, final", "", ".mp4")

	if !regexp.MustCompile(`^temp/raw/[0-9]+_video\.mp4$`).MatchString(key) {
		t.Fatalf("unexpected key: %q", key)
	}
}
