package services

import (
	"strings"
	"testing"
)

// newTestB2 returns a B2CleanupService configured only with bucket name
// for URL-to-key normalization. It cannot make network calls but does
// not need to — the tests only exercise pure helpers.
func newTestB2() *B2CleanupService {
	return &B2CleanupService{
		bucketName: "filmorauznet",
		cdnURL:     "https://cdn.filmorauz.net",
	}
}

func TestNormalizeKey_FromCDNURL(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("https://cdn.filmorauz.net/file/filmorauznet/videos/movies/abc/master.m3u8")
	want := "videos/movies/abc/master.m3u8"
	if got != want {
		t.Fatalf("NormalizeKey CDN URL = %q, want %q", got, want)
	}
}

func TestNormalizeKey_FromB2DirectURL(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("https://f005.backblazeb2.com/file/filmorauznet/images/posters/123.jpg")
	want := "images/posters/123.jpg"
	if got != want {
		t.Fatalf("NormalizeKey f005 URL = %q, want %q", got, want)
	}
}

func TestNormalizeKey_FromMediaProxyPath(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("/media/images/posters/xxx.webp")
	want := "images/posters/xxx.webp"
	if got != want {
		t.Fatalf("NormalizeKey /media/ path = %q, want %q", got, want)
	}
}

func TestNormalizeKey_FromMediaProxyURL(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("https://cdn.filmorauz.net/media/videos/clips/movies/foo/clip_001.mp4")
	want := "videos/clips/movies/foo/clip_001.mp4"
	if got != want {
		t.Fatalf("NormalizeKey CDN /media/ URL = %q, want %q", got, want)
	}
}

func TestNormalizeKey_FromFilePathPrefix(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("/file/filmorauznet/videos/serials/show/season-1/episode-1/master.m3u8")
	want := "videos/serials/show/season-1/episode-1/master.m3u8"
	if got != want {
		t.Fatalf("NormalizeKey /file/<bucket>/ = %q, want %q", got, want)
	}
}

func TestNormalizeKey_LeadingSlashOnly(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("/videos/movies/abc/master.m3u8")
	want := "videos/movies/abc/master.m3u8"
	if got != want {
		t.Fatalf("NormalizeKey /<key> = %q, want %q", got, want)
	}
}

func TestNormalizeKey_BareKey(t *testing.T) {
	s := newTestB2()
	got := s.NormalizeKey("videos/clips/movies/foo/clip.mp4")
	want := "videos/clips/movies/foo/clip.mp4"
	if got != want {
		t.Fatalf("NormalizeKey bare key = %q, want %q", got, want)
	}
}

func TestNormalizeKey_Empty(t *testing.T) {
	s := newTestB2()
	if got := s.NormalizeKey(""); got != "" {
		t.Fatalf("NormalizeKey empty = %q, want empty", got)
	}
	if got := s.NormalizeKey(" "); got != "" {
		t.Fatalf("NormalizeKey blank = %q, want empty", got)
	}
	if got := s.NormalizeKey("/"); got != "" {
		t.Fatalf("NormalizeKey root slash = %q, want empty", got)
	}
}

func TestIsSafeB2DeletePrefix_Rejects(t *testing.T) {
	cases := []string{
		"",
		"/",
		"videos/",
		"videos",
		"videos/movies/",
		"videos/movies",
		"videos/serials/",
		"videos/series/",
		"videos/clips/",
		"images/",
		"images/posters/",
		"images/backdrops/",
		"images/series/",
		"images/episodes/",
		"random/folder/",       // not in allowlist
		"backups/movies/abc/",  // not in allowlist
	}
	for _, c := range cases {
		if IsSafeB2DeletePrefix(c) {
			t.Errorf("IsSafeB2DeletePrefix(%q) = true, want false", c)
		}
	}
}

func TestIsSafeB2DeletePrefix_Accepts(t *testing.T) {
	cases := []string{
		"videos/movies/some-folder/",
		"videos/movies/abc-def-2025-id/",
		"videos/serials/show/",
		"videos/serials/show/season-1/",
		"videos/series/show/",
		"videos/clips/movies/abc/",
		"images/posters/1234_poster.png",  // single-file accepted as a "prefix" too
		"images/backdrops/foo/",
		"images/series/abc/",
		"images/episodes/abc/",
	}
	for _, c := range cases {
		if !IsSafeB2DeletePrefix(c) {
			t.Errorf("IsSafeB2DeletePrefix(%q) = false, want true", c)
		}
	}
}

func TestIsSafeB2DeleteKey_Rejects(t *testing.T) {
	cases := []string{
		"",
		"/",
		"videos/",
		"videos/movies/",
		"videos/serials/",
		"videos/clips/",
		"images/",
		"random/file.txt",  // not in allowlist
		"file.txt",          // bare file, not under root
		"videos/movies/",    // bare root
	}
	for _, c := range cases {
		if IsSafeB2DeleteKey(c) {
			t.Errorf("IsSafeB2DeleteKey(%q) = true, want false", c)
		}
	}
}

func TestIsSafeB2DeleteKey_Accepts(t *testing.T) {
	cases := []string{
		"videos/movies/abc/master.m3u8",
		"videos/movies/abc/480p/segment_001.ts",
		"videos/serials/show/season-1/episode-1/master.m3u8",
		"videos/clips/movies/abc/clip_001.mp4",
		"images/posters/1234_poster.png",
		"images/backdrops/foo.webp",
		"images/series/abc.jpg",
		"images/episodes/episode_thumb.jpg",
	}
	for _, c := range cases {
		if !IsSafeB2DeleteKey(c) {
			t.Errorf("IsSafeB2DeleteKey(%q) = false, want true", c)
		}
	}
}

// TestSafeDeleteKey_RejectsUnsafe asserts SafeDeleteKey records a Skipped
// entry without attempting any network call when the input does not pass
// the safety check. Because the service has no httpClient, a real delete
// attempt would panic — so a clean "skipped" exit also proves the guard
// fires before any network code runs.
func TestSafeDeleteKey_RejectsUnsafe(t *testing.T) {
	s := newTestB2()
	cases := []string{
		"",                      // empty
		"/",                     // root
		"videos/movies/",        // bare allowlist root
		"random/path/file.bin",  // not under any allowlist root
	}
	for _, raw := range cases {
		summary := NewB2DeleteSummary()
		s.SafeDeleteKey(raw, "test", summary)
		if summary.FilesDeleted != 0 {
			t.Errorf("SafeDeleteKey(%q) deleted %d files, want 0", raw, summary.FilesDeleted)
		}
		if len(summary.Errors) != 0 {
			t.Errorf("SafeDeleteKey(%q) errors=%v, want none", raw, summary.Errors)
		}
		if len(summary.Skipped) == 0 {
			t.Errorf("SafeDeleteKey(%q) had no Skipped entry", raw)
		}
	}
}

// TestSafeDeletePrefix_RejectsUnsafe asserts SafeDeletePrefix refuses to
// proceed for any of the historically-dangerous prefixes that wiped data.
func TestSafeDeletePrefix_RejectsUnsafe(t *testing.T) {
	s := newTestB2()
	cases := []string{
		"",
		"/",
		"videos",
		"videos/",
		"videos/movies/",
		"videos/serials/",
		"videos/clips/",
		"images/",
		"images/posters/",
	}
	for _, raw := range cases {
		summary := NewB2DeleteSummary()
		s.SafeDeletePrefix(raw, "test", summary)
		if summary.FilesDeleted != 0 {
			t.Errorf("SafeDeletePrefix(%q) deleted %d files, want 0", raw, summary.FilesDeleted)
		}
		if len(summary.Errors) != 0 {
			t.Errorf("SafeDeletePrefix(%q) errors=%v, want none", raw, summary.Errors)
		}
		if len(summary.Skipped) == 0 {
			t.Errorf("SafeDeletePrefix(%q) had no Skipped entry", raw)
		}
	}
}

// TestSafeDelete_DoesNotTouchUnrelatedKeys asserts that even when a movie
// has a sibling movie's URL accidentally stored in one of its fields, the
// allowlist + per-folder validation keeps the unrelated content safe.
//
// This is a static check on derivation logic — we feed an unrelated key
// to NormalizeKey + IsSafe and confirm it would be ACCEPTED for a
// per-key delete (movie A's poster) but would NOT slip into a prefix
// delete that walks all of movie B's data.
func TestSafeDelete_DoesNotTouchUnrelatedKeys(t *testing.T) {
	s := newTestB2()
	movieAKey := s.NormalizeKey("https://cdn.filmorauz.net/file/filmorauznet/videos/movies/movie-a-2025/master.m3u8")
	movieBKey := s.NormalizeKey("https://cdn.filmorauz.net/file/filmorauznet/videos/movies/movie-b-2025/master.m3u8")

	if movieAKey == "" || movieBKey == "" {
		t.Fatalf("setup: failed to derive keys (a=%q b=%q)", movieAKey, movieBKey)
	}
	// Both single keys are safe to delete on their own.
	if !IsSafeB2DeleteKey(movieAKey) {
		t.Errorf("movie A key not accepted as safe delete target: %q", movieAKey)
	}
	if !IsSafeB2DeleteKey(movieBKey) {
		t.Errorf("movie B key not accepted as safe delete target: %q", movieBKey)
	}
	// Movie A's folder prefix must NOT contain Movie B's slug.
	movieAPrefix := "videos/movies/movie-a-2025/"
	if !IsSafeB2DeletePrefix(movieAPrefix) {
		t.Errorf("movie A prefix rejected: %q", movieAPrefix)
	}
	if strings.HasPrefix(movieBKey, movieAPrefix) {
		t.Errorf("movie A prefix %q would catch movie B key %q — boundary violation", movieAPrefix, movieBKey)
	}
}
