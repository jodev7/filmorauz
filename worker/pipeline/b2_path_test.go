package pipeline

import (
	"strings"
	"testing"

	"github.com/filmorauz/worker/models"
)

func TestB2VideoRoot_Movie(t *testing.T) {
	got := B2VideoRoot(&models.IngestionJob{ContentType: ""})
	if got != B2VideoRootMovies {
		t.Fatalf("movie root: got %q want %q", got, B2VideoRootMovies)
	}
}

func TestB2VideoRoot_Episode(t *testing.T) {
	got := B2VideoRoot(&models.IngestionJob{ContentType: "episode"})
	if got != B2VideoRootSerials {
		t.Fatalf("episode root: got %q want %q", got, B2VideoRootSerials)
	}
}

func TestBuildVideoB2Path_Movie(t *testing.T) {
	got, err := BuildVideoB2Path(B2VideoRootMovies, "some-movie_abcd1234", "1080p", "index.m3u8")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "videos/movies/some-movie_abcd1234/1080p/index.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if !strings.HasPrefix(got, B2VideoRootMovies+"/") {
		t.Fatalf("expected prefix %q on %q", B2VideoRootMovies+"/", got)
	}
}

func TestBuildVideoB2Path_SeriesEpisode(t *testing.T) {
	got, err := BuildVideoB2Path(B2VideoRootSerials, "my-show/season-1/episode-3", "master.m3u8")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "videos/serials/my-show/season-1/episode-3/master.m3u8"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if strings.HasPrefix(got, B2VideoRootMovies+"/") {
		t.Fatalf("episode must not land under %s: got %q", B2VideoRootMovies, got)
	}
}

func TestBuildVideoB2Path_Clip(t *testing.T) {
	got, err := BuildVideoB2Path(B2VideoRootClips, "my-show/season-1/episode-3", "clip_01.mp4")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	want := "videos/clips/my-show/season-1/episode-3/clip_01.mp4"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// The legacy bug: callers built `videos/movies/serials/...` for episodes.
// New uploads must reject that shape so a regression is caught at the seam.
func TestBuildVideoB2Path_RejectsLegacySerialsUnderMovies(t *testing.T) {
	_, err := BuildVideoB2Path(B2VideoRootMovies, "serials/my-show/season-1/episode-3", "master.m3u8")
	if err == nil {
		t.Fatalf("expected rejection of videos/movies/serials/... legacy path")
	}
}

func TestJobContentTypeLabel(t *testing.T) {
	if got := JobContentTypeLabel(&models.IngestionJob{ContentType: "episode"}); got != ContentTypeEpisode {
		t.Fatalf("episode label: got %q", got)
	}
	if got := JobContentTypeLabel(&models.IngestionJob{}); got != ContentTypeMovie {
		t.Fatalf("movie label: got %q", got)
	}
}
