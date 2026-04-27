package services

import (
	"testing"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func TestMovieDeletePrefixFromKey(t *testing.T) {
	got := movieDeletePrefixFromKey("videos/movies/dovud-multfilm-premyera-2025-69ee/master.m3u8")
	want := "videos/movies/dovud-multfilm-premyera-2025-69ee/"
	if got != want {
		t.Fatalf("movieDeletePrefixFromKey() = %q, want %q", got, want)
	}
}

func TestValidateB2DeletePrefixRejectsBroadPrefix(t *testing.T) {
	movie := models.Movie{
		ID:   primitive.NewObjectID(),
		Slug: "dovud-multfilm-premyera",
	}
	if err := validateB2DeletePrefix("videos/movies/", movie); err == nil {
		t.Fatal("expected broad prefix to be rejected")
	}
}

func TestValidateB2DeletePrefixAcceptsExactMovieFolder(t *testing.T) {
	id := primitive.NewObjectID()
	movie := models.Movie{
		ID:   id,
		Slug: "dovud-multfilm-premyera",
	}
	prefix := "videos/movies/dovud-multfilm-premyera-2025-" + id.Hex() + "/"
	if err := validateB2DeletePrefix(prefix, movie); err != nil {
		t.Fatalf("expected exact movie folder to validate, got %v", err)
	}
}
