package pipeline

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"

	"github.com/filmorauz/worker/models"
)

const (
	// Ceiling on any single network read/write inside the HLS pull. Passed to
	// ffmpeg as -rw_timeout so a CDN connection that goes silent surfaces as an
	// error instead of an indefinite block.
	clipHLSRWTimeout = 30 * time.Second
)

// Stall-watchdog timings. Vars rather than consts so tests can shorten them.
var (
	// How often the stall watchdog samples the growing mp4.
	clipHLSStallPollInterval = 30 * time.Second
	// If the mp4 has not grown at all for this long the pull is declared dead
	// and ffmpeg is killed.
	clipHLSStallTimeout = 10 * time.Minute
	// Absolute cap on one HLS pull, however healthy it looks. A full episode
	// copy-mux runs in minutes; anything near this is pathological.
	clipHLSPullMaxDuration = 2 * time.Hour
	// How long cmd.Run may still block on outstanding I/O after the kill.
	clipHLSKillGrace = 15 * time.Second
)

// downloadHLSToMP4 pulls an HLS playlist into a local mp4 via stream copy.
//
// Three guards wrap the ffmpeg call. They were added after clip_only pulls were
// found wedged for over a day against a CDN that had silently dropped the
// connection: ffmpeg sat in a blocking read with the output file frozen, and
// because those jobs go on heartbeating, RecoverStaleJobs never reclaimed them.
// Each wedged pull permanently consumed one of PROCESS_CONCURRENCY slots, so
// three of them starved the processing queue outright.
func (p *Pipeline) downloadHLSToMP4(ctx context.Context, jobID, masterURL, mp4Path string) error {
	pullCtx, cancelPull := context.WithTimeout(ctx, clipHLSPullMaxDuration)
	defer cancelPull()

	cmd := exec.CommandContext(pullCtx, "ffmpeg",
		"-y",
		"-hide_banner",
		"-loglevel", "error",
		"-rw_timeout", strconv.FormatInt(clipHLSRWTimeout.Microseconds(), 10),
		"-i", masterURL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		mp4Path,
	)
	// Run ffmpeg in its own process group and kill the group on cancel. Killing
	// only the direct child leaves any grandchild holding the stderr pipe open,
	// and cmd.Run blocks on that EOF long after the kill — which would defeat
	// the whole point of the watchdog. WaitDelay is the last-resort backstop if
	// the group kill still leaves I/O outstanding.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = clipHLSKillGrace

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	// -rw_timeout covers a hung socket, but not every wedge reaches ffmpeg as a
	// stuck read — watch actual output progress as the backstop.
	stalled := make(chan struct{})
	watchdogDone := make(chan struct{})
	go func() {
		defer close(watchdogDone)
		ticker := time.NewTicker(clipHLSStallPollInterval)
		defer ticker.Stop()

		lastSize := int64(-1)
		lastGrowth := time.Now()
		for {
			select {
			case <-pullCtx.Done():
				return
			case <-ticker.C:
			}

			var size int64
			if fi, err := os.Stat(mp4Path); err == nil {
				size = fi.Size()
			}
			if size != lastSize {
				lastSize = size
				lastGrowth = time.Now()
				continue
			}
			if time.Since(lastGrowth) >= clipHLSStallTimeout {
				log.Printf("[CLIP_ONLY] stalled job=%s: output frozen at %d bytes for %s, killing ffmpeg",
					jobID, size, clipHLSStallTimeout)
				close(stalled)
				cancelPull()
				return
			}
		}
	}()

	runErr := cmd.Run()
	cancelPull()
	<-watchdogDone

	if runErr == nil {
		return nil
	}
	select {
	case <-stalled:
		return fmt.Errorf("ffmpeg HLS→mp4 stalled: no output growth for %s: %s",
			clipHLSStallTimeout, stderr.String())
	default:
	}
	if errors.Is(pullCtx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("ffmpeg HLS→mp4 exceeded %s: %s", clipHLSPullMaxDuration, stderr.String())
	}
	return fmt.Errorf("ffmpeg HLS→mp4 failed: %v: %s", runErr, stderr.String())
}

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

	if err := p.downloadHLSToMP4(ctx, jobID, job.MasterPlaylistURL, mp4Path); err != nil {
		return err
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
