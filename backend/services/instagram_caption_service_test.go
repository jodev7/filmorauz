package services

import (
	"context"
	"testing"

	"github.com/filmorauz/backend/models"
)

func TestBuildInstagramClipCaption(t *testing.T) {
	got := BuildInstagramClipCaption("1234")
	want := "🎬 Kinoni profildagi bot orqali toping!\n🔢 Kino Kodi: 1234"
	if got != want {
		t.Fatalf("caption mismatch\nwant: %q\ngot:  %q", want, got)
	}
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
