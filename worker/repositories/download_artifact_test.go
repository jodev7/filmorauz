package repositories

import (
	"os"
	"path/filepath"
	"testing"
)

// aria2c preallocates the full target size the moment a transfer starts, so a
// bare stat() of a multi-GB file proves nothing about completeness. Only the
// ".aria2" control file (removed once aria2c verifies the download) does.
func TestResolveExistingLocalPathRejectsInProgressDownload(t *testing.T) {
	dir := t.TempDir()
	media := filepath.Join(dir, "job123.mp4")
	if err := os.WriteFile(media, make([]byte, 4096), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := resolveExistingLocalPath(media); got != media {
		t.Fatalf("complete download should resolve: got %q, want %q", got, media)
	}

	if err := os.WriteFile(media+".aria2", []byte("control"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := resolveExistingLocalPath(media); got != "" {
		t.Fatalf("in-progress download must not resolve, got %q", got)
	}

	// Once aria2c finishes it removes the control file and the media is usable.
	if err := os.Remove(media + ".aria2"); err != nil {
		t.Fatal(err)
	}
	if got := resolveExistingLocalPath(media); got != media {
		t.Fatalf("finished download should resolve again: got %q", got)
	}
}

// The job-ID glob fallback used to match "<jobID>.mp4.aria2" and hand back a
// few hundred bytes of aria2 bookkeeping as if it were the movie.
func TestResolveExistingDownloadedArtifactIgnoresSidecars(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "job456.mp4.aria2"), []byte("control"), 0o644); err != nil {
		t.Fatal(err)
	}

	if got := resolveExistingDownloadedArtifact("job456", "", dir); got != "" {
		t.Fatalf("sidecar must never be treated as media, got %q", got)
	}

	media := filepath.Join(dir, "job456.mp4")
	if err := os.WriteFile(media, make([]byte, 2048), 0o644); err != nil {
		t.Fatal(err)
	}
	// Media now exists but the transfer is still running.
	if got := resolveExistingDownloadedArtifact("job456", "", dir); got != "" {
		t.Fatalf("must not resolve while transfer is running, got %q", got)
	}

	if err := os.Remove(filepath.Join(dir, "job456.mp4.aria2")); err != nil {
		t.Fatal(err)
	}
	if got := resolveExistingDownloadedArtifact("job456", "", dir); got != media {
		t.Fatalf("got %q, want %q", got, media)
	}
}
