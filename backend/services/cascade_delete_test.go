package services

import (
	"strings"
	"testing"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// (1) A movie HLS folder with thousands of segment files must remain a
// valid delete target after the cap was raised. The previous 500-file
// cap was the bug. The new cap is movieB2FileCap (=10000).
func TestCascade_MovieHLSFolder_2000Files_AllowedByCap(t *testing.T) {
	if movieB2FileCap < 2000 {
		t.Fatalf("movieB2FileCap=%d must allow at least 2000 files", movieB2FileCap)
	}
	id := primitive.NewObjectID()
	movie := models.Movie{ID: id, Slug: "movie-a"}
	prefix := "videos/movies/movie-a-2025-" + id.Hex() + "/"
	if err := validateB2DeletePrefix(prefix, movie); err != nil {
		t.Fatalf("legitimate movie folder must validate, got %v", err)
	}
	if !IsSafeB2DeletePrefix(prefix) {
		t.Fatalf("legitimate movie folder must pass allowlist")
	}
}

// (2) Broad roots are rejected, even when the caller hand-constructs them.
func TestCascade_RejectsBroadRoots(t *testing.T) {
	for _, prefix := range []string{
		"",
		"videos/",
		"videos/movies/",
		"videos/serials/",
		"videos/clips/",
		"images/",
		"images/posters/",
		"images/backdrops/",
	} {
		if IsSafeB2DeletePrefix(prefix) {
			t.Errorf("broad prefix %q must be rejected", prefix)
		}
	}
}

// (3) An external poster URL (https://freekino.net/...) is not a B2 key
// and must be skipped — never attempted.
func TestCascade_ExternalPosterURL_SkippedSafely(t *testing.T) {
	svc := &B2CleanupService{bucketName: "filmorauznet"}
	key := svc.NormalizeKey("https://freekino.net/uploads/cover/abc.jpg")
	// External host without /file/<bucket>/ or /media/ prefix: NormalizeKey
	// returns the literal path, which then fails the allowlist.
	if IsSafeB2DeleteKey(key) {
		t.Fatalf("external URL %q normalized to %q must not pass IsSafeB2DeleteKey", "freekino", key)
	}
	summary := NewB2DeleteSummary()
	svc.SafeDeleteKey("https://freekino.net/uploads/cover/abc.jpg", "movie-poster", summary)
	if summary.FilesDeleted != 0 {
		t.Fatalf("external URL should not increment FilesDeleted (got %d)", summary.FilesDeleted)
	}
	if len(summary.Skipped) == 0 {
		t.Fatalf("external URL should be recorded in Skipped")
	}
}

// (5) New series uploads land at videos/serials/<series-folder>/.
func TestCascade_SeriesPrefix_VideosSerials_Validates(t *testing.T) {
	id := primitive.NewObjectID()
	series := models.Series{ID: id, Slug: "show-a"}
	prefix := "videos/serials/show-a-" + id.Hex() + "/"
	if err := validateB2DeletePrefixForSeries(prefix, series); err != nil {
		t.Fatalf("series prefix must validate, got %v", err)
	}
	if !IsSafeB2DeletePrefix(prefix) {
		t.Fatalf("series prefix must pass allowlist")
	}
}

// (6) Legacy series uploads under videos/movies/serials/ remain a safe
// delete target — series cleanup walks both the new and legacy locations.
func TestCascade_LegacySeriesPrefix_VideosMoviesSerials_Allowed(t *testing.T) {
	prefix := "videos/movies/serials/show-a-deadbeef/master.m3u8"
	// As a key under videos/movies/, this passes the allowlist:
	if !IsSafeB2DeleteKey(prefix) {
		t.Fatalf("legacy key under videos/movies/ must be safe to delete")
	}
	folder := "videos/movies/serials/show-a-deadbeef/"
	if !IsSafeB2DeletePrefix(folder) {
		t.Fatalf("legacy folder under videos/movies/ must be a valid prefix target")
	}
}

// (7) Clips folder is a first-class delete target gated on the content's
// own slug/id token.
func TestCascade_ClipsFolder_GatedByMovieIdentity(t *testing.T) {
	id := primitive.NewObjectID()
	movie := models.Movie{ID: id, Slug: "movie-a"}
	good := "videos/clips/movie-a-2025-" + id.Hex() + "/"
	if err := validateClipsDeletePrefix(good, movie); err != nil {
		t.Fatalf("clips prefix matching movie identity must validate, got %v", err)
	}
	bad := "videos/clips/movie-b-2025-deadbeef/"
	if err := validateClipsDeletePrefix(bad, movie); err == nil {
		t.Fatalf("clips prefix not matching movie identity must be rejected")
	}
	root := "videos/clips/"
	if err := validateClipsDeletePrefix(root, movie); err == nil {
		t.Fatalf("bare clips root must be rejected")
	}
}

// (8) A movie delete cannot delete a sibling movie's folder. The
// per-movie validator rejects any folder whose token does not match
// this movie's slug/id/source-id.
func TestCascade_MovieDelete_DoesNotTouchSiblingFolder(t *testing.T) {
	movieAID := primitive.NewObjectID()
	movieA := models.Movie{ID: movieAID, Slug: "movie-a"}
	siblingFolder := "videos/movies/movie-b-2025-" + primitive.NewObjectID().Hex() + "/"
	if err := validateB2DeletePrefix(siblingFolder, movieA); err == nil {
		t.Fatalf("movieA delete must not validate sibling folder %q", siblingFolder)
	}
	movieAFolder := "videos/movies/movie-a-2025-" + movieAID.Hex() + "/"
	otherKey := "videos/movies/movie-b-2025/master.m3u8"
	if strings.HasPrefix(otherKey, movieAFolder) {
		t.Fatalf("movieA prefix must not be a parent of movieB key")
	}
}

// movieFolderTokenFromPrefix is a small helper but sits on the cascade
// path — pin its rejection of broad roots so a refactor cannot widen it.
func TestMovieFolderTokenFromPrefix_Rejects(t *testing.T) {
	cases := []string{
		"",
		"videos/",
		"videos/movies/",
		"videos/serials/show-a/",
		"images/posters/movie-a/",
	}
	for _, p := range cases {
		if got := movieFolderTokenFromPrefix(p); got != "" {
			t.Errorf("movieFolderTokenFromPrefix(%q) = %q, want empty", p, got)
		}
	}
	if got := movieFolderTokenFromPrefix("videos/movies/movie-a-2025-deadbeef/"); got != "movie-a-2025-deadbeef" {
		t.Errorf("token: got %q", got)
	}
}
