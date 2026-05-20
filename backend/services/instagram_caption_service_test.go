package services

import (
	"context"
	"testing"

	"github.com/filmorauz/backend/models"
)

func TestBuildInstagramClipCaptionMovie(t *testing.T) {
	got := BuildInstagramClipCaption("1234", false)
	wantHead := "🎬 Kinoni profildagi bot orqali toping!\n🔢 Kino Kodi: 1234"
	if !startsWith(got, wantHead) {
		t.Fatalf("caption head mismatch\nwant prefix: %q\ngot:         %q", wantHead, got)
	}
	if !contains(got, "#kinolar") {
		t.Fatalf("caption missing movie hashtag block, got: %q", got)
	}
}

func TestBuildInstagramClipCaptionSeries(t *testing.T) {
	got := BuildInstagramClipCaption("1234", true)
	wantHead := "📺 Serialni profildagi bot orqali toping!\n🔢 Serial Kodi: 1234"
	if !startsWith(got, wantHead) {
		t.Fatalf("series caption head mismatch\nwant prefix: %q\ngot:         %q", wantHead, got)
	}
	if !contains(got, "#seriallar") {
		t.Fatalf("series caption missing series hashtag block, got: %q", got)
	}
}

func startsWith(s, prefix string) bool { return len(s) >= len(prefix) && s[:len(prefix)] == prefix }
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestResolveInstagramClipCodeMovie(t *testing.T) {
	clip := &models.Clip{
		ContentKind: "movie",
		MovieCode:   "4321",
	}

	got := ResolveInstagramClipCode(context.Background(), clip, nil)
	if got != "4321" {
		t.Fatalf("expected movie code, got %q", got)
	}
}

func TestResolveInstagramClipCodeSeriesFallsBackToClipCode(t *testing.T) {
	clip := &models.Clip{
		ContentKind: "series",
		MovieCode:   "7777",
	}

	got := ResolveInstagramClipCode(context.Background(), clip, nil)
	if got != "7777" {
		t.Fatalf("expected series clip code fallback, got %q", got)
	}
}
