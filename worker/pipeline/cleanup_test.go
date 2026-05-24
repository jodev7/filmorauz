package pipeline

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/filmorauz/worker/models"
	"github.com/filmorauz/worker/storage"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// prodPipeline builds a Pipeline whose cleanup allowlist points ParserDownloads
// at downloadsDir (via the DOWNLOAD_DIR env that cleanupAllowlist reads) and
// runs in the given storage mode.
func prodPipeline(t *testing.T, downloadsDir, mode string) *Pipeline {
	t.Helper()
	t.Setenv("DOWNLOAD_DIR", downloadsDir)
	return &Pipeline{
		config: Config{
			StorageConfig: storage.Config{Mode: mode},
		},
	}
}

// writeFileAged creates a file of the given size and back-dates its mtime.
func writeFileAged(t *testing.T, path string, size int, age time.Duration) {
	t.Helper()
	if err := os.WriteFile(path, make([]byte, size), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	mt := time.Now().Add(-age)
	if err := os.Chtimes(path, mt, mt); err != nil {
		t.Fatalf("chtimes %s: %v", path, err)
	}
}

func TestSweepOrphanDownloads(t *testing.T) {
	dir := t.TempDir()
	p := prodPipeline(t, dir, "prod")

	// Detritus that should be removed.
	writeFileAged(t, filepath.Join(dir, "zombie.aria2"), 16, 2*time.Hour)  // aria2 marker, >1h
	writeFileAged(t, filepath.Join(dir, "stub.mp4"), 0, 2*time.Hour)       // zero-byte, >1h
	writeFileAged(t, filepath.Join(dir, "orphan.mp4"), 1024, 72*time.Hour) // orphan mp4, >48h, not active

	// Files that must be preserved.
	writeFileAged(t, filepath.Join(dir, "active.mp4"), 1024, 72*time.Hour)  // old but belongs to an active job
	writeFileAged(t, filepath.Join(dir, "fresh.mp4"), 1024, 1*time.Hour)    // too young to be an orphan
	writeFileAged(t, filepath.Join(dir, "recent.aria2"), 16, 1*time.Minute) // aria2 marker, too young

	active := map[string]struct{}{"active.mp4": {}}

	deleted, freed := p.SweepOrphanDownloads(active, time.Hour, time.Hour, 48*time.Hour)

	if deleted != 3 {
		t.Errorf("deleted = %d, want 3", deleted)
	}
	// freed sums every removed file: orphan.mp4 (1024) + zombie.aria2 (16) + stub.mp4 (0).
	if freed != 1040 {
		t.Errorf("bytesFreed = %d, want 1040", freed)
	}

	gone := []string{"zombie.aria2", "stub.mp4", "orphan.mp4"}
	for _, n := range gone {
		if _, err := os.Stat(filepath.Join(dir, n)); !os.IsNotExist(err) {
			t.Errorf("%s should have been removed", n)
		}
	}
	kept := []string{"active.mp4", "fresh.mp4", "recent.aria2"}
	for _, n := range kept {
		if _, err := os.Stat(filepath.Join(dir, n)); err != nil {
			t.Errorf("%s should have been kept: %v", n, err)
		}
	}
}

func TestSweepOrphanDownloads_DevModeNoop(t *testing.T) {
	dir := t.TempDir()
	p := prodPipeline(t, dir, "dev")

	writeFileAged(t, filepath.Join(dir, "orphan.mp4"), 1024, 72*time.Hour)
	writeFileAged(t, filepath.Join(dir, "zombie.aria2"), 16, 2*time.Hour)

	deleted, freed := p.SweepOrphanDownloads(nil, time.Hour, time.Hour, 48*time.Hour)
	if deleted != 0 || freed != 0 {
		t.Errorf("dev mode should delete nothing, got deleted=%d freed=%d", deleted, freed)
	}
	if _, err := os.Stat(filepath.Join(dir, "orphan.mp4")); err != nil {
		t.Errorf("dev mode must keep orphan.mp4: %v", err)
	}
}

func TestCleanupCompletedJobArtifacts_TerminalFailedDeletesSource(t *testing.T) {
	dir := t.TempDir()
	p := prodPipeline(t, dir, "prod")

	src := filepath.Join(dir, "deadbeef.mp4")
	writeFileAged(t, src, 2048, 0)

	job := &models.IngestionJob{
		ID:         primitive.NewObjectID(),
		Status:     models.IngestionStatusDownloadFailed,
		RetryCount: 3, // retry budget exhausted -> terminal
		LocalPath:  src,
	}

	actions, err := p.CleanupCompletedJobArtifacts(job)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, statErr := os.Stat(src); !os.IsNotExist(statErr) {
		t.Errorf("terminal-failed source should be deleted; still present (actions=%v)", actions)
	}
}

func TestCleanupCompletedJobArtifacts_RefusesNonTerminal(t *testing.T) {
	dir := t.TempDir()
	p := prodPipeline(t, dir, "prod")

	src := filepath.Join(dir, "inflight.mp4")
	writeFileAged(t, src, 2048, 0)

	// processing job: still in-flight, must never be cleaned.
	processing := &models.IngestionJob{
		ID:        primitive.NewObjectID(),
		Status:    models.IngestionStatusProcessing,
		LocalPath: src,
	}
	if _, err := p.CleanupCompletedJobArtifacts(processing); err == nil {
		t.Error("expected refusal for processing job, got nil error")
	}

	// failed but with retries still remaining: not terminal yet.
	retriable := &models.IngestionJob{
		ID:         primitive.NewObjectID(),
		Status:     models.IngestionStatusFailed,
		RetryCount: 1,
		LocalPath:  src,
	}
	if _, err := p.CleanupCompletedJobArtifacts(retriable); err == nil {
		t.Error("expected refusal for failed job with retries left, got nil error")
	}

	if _, err := os.Stat(src); err != nil {
		t.Errorf("source must survive refused cleanups: %v", err)
	}
}

func TestCleanupCompletedJobArtifacts_DevModeKeepsFiles(t *testing.T) {
	dir := t.TempDir()
	p := prodPipeline(t, dir, "dev")

	src := filepath.Join(dir, "completed.mp4")
	writeFileAged(t, src, 2048, 0)

	job := &models.IngestionJob{
		ID:        primitive.NewObjectID(),
		Status:    models.IngestionStatusCompleted,
		LocalPath: src,
	}
	if _, err := p.CleanupCompletedJobArtifacts(job); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := os.Stat(src); err != nil {
		t.Errorf("dev mode must keep completed source: %v", err)
	}
}
