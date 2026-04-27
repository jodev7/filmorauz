package services

import (
	"strings"
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

// TestValidateB2DeletePrefixRejectsOtherMovieFolder is the explicit
// regression guard for the historical bug where deleting one movie
// would walk a folder named after a *different* movie. The validator
// must reject any folder whose name does not contain this movie's
// slug, ID, or source ID.
func TestValidateB2DeletePrefixRejectsOtherMovieFolder(t *testing.T) {
	movie := models.Movie{
		ID:   primitive.NewObjectID(),
		Slug: "movie-a",
	}
	otherFolder := "videos/movies/movie-b-2025-deadbeef/"
	if err := validateB2DeletePrefix(otherFolder, movie); err == nil {
		t.Fatal("expected unrelated movie folder to be rejected")
	}
}

// TestValidateB2DeletePrefixRejectsImagesPrefix is a guard against the
// validator over-approving any prefix that *happens* to contain the
// movie's slug — the prefix must specifically be under videos/movies/.
func TestValidateB2DeletePrefixRejectsImagesPrefix(t *testing.T) {
	movie := models.Movie{
		ID:   primitive.NewObjectID(),
		Slug: "movie-a",
	}
	if err := validateB2DeletePrefix("images/posters/movie-a/", movie); err == nil {
		t.Fatal("expected images/posters/ prefix to be rejected by movie validator")
	}
}

// TestValidateB2DeletePrefixForSeriesRejectsOtherSeriesFolder is the
// series-side equivalent of the above regression guard.
func TestValidateB2DeletePrefixForSeriesRejectsOtherSeriesFolder(t *testing.T) {
	series := models.Series{
		ID:   primitive.NewObjectID(),
		Slug: "show-a",
	}
	if err := validateB2DeletePrefixForSeries("videos/serials/show-b/", series); err == nil {
		t.Fatal("expected unrelated series folder to be rejected")
	}
}

// TestSafeDelete_BoundariesForMovieFolders cross-checks the safe helper
// and the per-movie validator together: a movie's HLS prefix is safe
// in isolation AND under the validator, but a sibling movie's key
// must NOT fall under it.
func TestSafeDelete_BoundariesForMovieFolders(t *testing.T) {
	movieAID := primitive.NewObjectID()
	movieA := models.Movie{ID: movieAID, Slug: "movie-a"}
	movieAPrefix := "videos/movies/movie-a-2025-" + movieAID.Hex() + "/"

	if err := validateB2DeletePrefix(movieAPrefix, movieA); err != nil {
		t.Fatalf("movieA prefix should validate, got %v", err)
	}
	if !IsSafeB2DeletePrefix(movieAPrefix) {
		t.Fatalf("movieA prefix should be safe per allowlist")
	}
	movieBKey := "videos/movies/movie-b-2025/master.m3u8"
	if strings.HasPrefix(movieBKey, movieAPrefix) {
		t.Fatalf("movieA prefix %q would catch movieB key %q", movieAPrefix, movieBKey)
	}
}
