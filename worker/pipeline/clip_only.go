package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/filmorauz/worker/models"
)

// processClipOnlyJob handles ingestion jobs whose ContentType is "clip_only".
// These are enqueued by the backend serial finalizer after the Episode row is
// created — at that point the HLS already lives on B2, the EpisodeID is known,
// but the original mp4 used for clip generation has been cleaned up. We pull
// the HLS down via ffmpeg, run the standard episode clip generator, and
// complete the job.
func (p *Pipeline) processClipOnlyJob(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[CLIP_ONLY] start job=%s series=%s S%02dE%02d master=%s",
		jobID, job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, job.MasterPlaylistURL)

	if job.EpisodeID.IsZero() {
		return fmt.Errorf("clip_only job %s missing episode_id", jobID)
	}
	if job.MasterPlaylistURL == "" {
		return fmt.Errorf("clip_only job %s missing master_playlist_url", jobID)
	}

	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 20); err != nil {
		return fmt.Errorf("clip_only update_status processing: %w", err)
	}

	// Heartbeat goroutine: HLS download + clip generation for a full episode
	// routinely exceed the 10-minute "processing stalled" watchdog window
	// (RecoverStaleJobs), which would otherwise reset the job to
	// ready_to_process mid-flight and loop until retry_count hits the limit.
	// Periodically bump updated_at to keep the watchdog satisfied. Mirrors the
	// heartbeat used during HLS ParallelUpload.
	doneHeartbeat := make(chan struct{})
	var stopHeartbeatOnce sync.Once
	stopHeartbeat := func() { stopHeartbeatOnce.Do(func() { close(doneHeartbeat) }) }
	go func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				log.Printf("[CLIP_ONLY] heartbeat job=%s (keeping status fresh during clip generation)", jobID)
				p.updateStatus(jobID, models.IngestionStatusProcessing, 60)
			case <-doneHeartbeat:
				return
			}
		}
	}()
	// Guarantees the goroutine is torn down on every return path (incl. errors).
	defer stopHeartbeat()

	tmpDir, err := os.MkdirTemp("", "clip_only_*")
	if err != nil {
		return fmt.Errorf("clip_only create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	mp4Path := filepath.Join(tmpDir, "source.mp4")
	log.Printf("[CLIP_ONLY] downloading HLS → %s", mp4Path)

	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-i", job.MasterPlaylistURL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		mp4Path,
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg HLS→mp4 failed: %v: %s", err, stderr.String())
	}

	fi, err := os.Stat(mp4Path)
	if err != nil || fi.Size() == 0 {
		return fmt.Errorf("clip_only downloaded mp4 missing or empty: %v", err)
	}
	log.Printf("[CLIP_ONLY] downloaded mp4 size=%d", fi.Size())

	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 60); err != nil {
		log.Printf("[CLIP_ONLY] WARN: update_status mid: %v", err)
	}

	if clipErr := p.generateEpisodeClips(ctx, job, mp4Path); clipErr != nil {
		return fmt.Errorf("clip_only generateEpisodeClips: %w", clipErr)
	}

	// Stop the heartbeat before the terminal update so a late tick cannot
	// flip the job back to "processing" after we mark it completed.
	stopHeartbeat()

	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		log.Printf("[CLIP_ONLY] WARN: complete: %v", err)
	}
	log.Printf("[CLIP_ONLY] done job=%s", jobID)
	return nil
}
