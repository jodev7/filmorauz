package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/filmorauz/worker/models"
)

// processEpisodeJob runs a serial-episode ingestion job through the same
// download + HLS pipeline used for movies, but writes the output under
//
//	videos/serials/<series-slug>/season-<N>/episode-<M>/
//
// and, on success, notifies the backend to attach the master playlist URL
// to the linked Episode row. This is deliberately a reduced pipeline: no
// TMDB enrichment, no movie-code assignment, no Telegram notification.
// The episode folder intentionally has no job-id suffix: a re-run for the
// same (series, season, episode) must overwrite the previous HLS so the
// tree stays clean and matches the required serial→season→episode layout.
func (p *Pipeline) processEpisodeJob(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[EPISODE] start job=%s series=%s S%02dE%02d",
		jobID, job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber)

	if job.Metadata == nil {
		return fmt.Errorf("episode job %s has no metadata", jobID)
	}
	// Series-slug is still required (drives the B2 folder layout). EpisodeID
	// is OPTIONAL in deferred-DB mode: the backend's serial finalizer creates
	// the Episode row only after every child completes, so the worker can
	// finish ingestion before an Episode row exists.
	if job.SeriesSlug == "" {
		return fmt.Errorf("episode job %s missing series_slug", jobID)
	}
	localPath := job.LocalPath
	if localPath == "" {
		log.Printf("[PROCESS] skipped job=%s reason=missing local_path", jobID)
		return fmt.Errorf("episode job %s is not ready_to_process: local_path is empty", jobID)
	}
	if fi, err := os.Stat(localPath); err != nil || fi.Size() == 0 {
		if err != nil {
			log.Printf("[ERROR] file missing at %s", localPath)
			return fmt.Errorf("episode local_path unavailable: %w", err)
		}
		log.Printf("[ERROR] file missing at %s", localPath)
		return fmt.Errorf("episode local_path is empty: %s", localPath)
	}

	// === Process + upload phase ===
	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 50); err != nil {
		return fmt.Errorf("episode update_status processing: %w", err)
	}

	// Folder layout under the videos/serials/ B2 root:
	//   videos/serials/<slug>/season-N/episode-M/index.m3u8
	//   videos/serials/<slug>/season-N/episode-M/<quality>/index.m3u8
	//   videos/serials/<slug>/season-N/episode-M/<quality>/segment_*.ts
	// The "videos/serials" root is supplied by B2VideoRoot(job) at upload time,
	// so this folder name must NOT include any "serials/" prefix — doing so
	// would land assets at videos/serials/serials/... or, before this fix,
	// at videos/movies/serials/...
	folderName := filepath.Join(
		job.SeriesSlug,
		fmt.Sprintf("season-%d", job.SeasonNumber),
		fmt.Sprintf("episode-%d", job.EpisodeNumber),
	)
	log.Printf("[EPISODE] layout series_slug=%s season=%d episode=%d folder=%s",
		job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, folderName)

	hlsDir, processedMaster, err := p.processVideo(job, localPath, folderName)
	if err != nil {
		return fmt.Errorf("episode processVideo failed: %w", err)
	}
	log.Printf("[EPISODE] local_hls_dir=%s processed_master=%s", hlsDir, processedMaster)
	if stepErr := p.jobRepo.UpdateStep(ctx, jobID, "process"); stepErr != nil {
		log.Printf("[EPISODE] WARNING: mark process step failed: %v", stepErr)
	}

	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 70); err != nil {
		return fmt.Errorf("episode update_status uploading: %w", err)
	}
	streamingURL, _, err := p.uploadProcessedFiles(job, hlsDir, localPath, folderName)
	if err != nil {
		return fmt.Errorf("episode uploadProcessedFiles failed: %w", err)
	}
	log.Printf("[EPISODE] b2_root=%s/%s master_b2_key=%s/%s/%s", B2VideoRootSerials, folderName, B2VideoRootSerials, folderName, MasterPlaylistName)
	log.Printf("[EPISODE] streamingURL=%s", streamingURL)

	// Persist the uploaded (B2/CDN in prod) master URL onto the job. The
	// serial finalizer reads job.master_playlist_url to set each Episode's
	// video_url — without this it stays the local working-dir path written
	// by UpdateQualityInfo during processing, which is the "episode video_url
	// is local" bug. The movie pipeline already does the equivalent.
	job.MasterPlaylistURL = streamingURL
	if err := p.jobRepo.UpdateMasterPlaylistURL(ctx, jobID, streamingURL); err != nil {
		log.Printf("[EPISODE] WARNING: UpdateMasterPlaylistURL: %v", err)
	}

	mode := p.config.StorageConfig.Mode
	outputMode := "development"
	finalPath := streamingURL
	if mode != "prod" && mode != "production" {
		outputMode = "development"
		baseDir, _ := os.Getwd()
		finalPath = filepath.Join(baseDir, "uploads", "serials", folderName)
	} else {
		outputMode = "production"
	}
	if err := p.jobRepo.UpdateFinalOutputPath(ctx, jobID, finalPath, streamingURL, outputMode); err != nil {
		log.Printf("[EPISODE] WARNING: UpdateFinalOutputPath: %v", err)
	}
	if err := p.jobRepo.MarkSourceFileDeleted(ctx, jobID, hlsDir); err != nil {
		log.Printf("[EPISODE] WARNING: MarkSourceFileDeleted: %v", err)
	}

	// === Notify backend so the Episode row gets the playback URL ===
	// In deferred-DB mode the Episode row does not exist yet — the backend's
	// serial finalizer creates it later using job.MasterPlaylistURL (already
	// stored by UpdateFinalOutputPath above). Only notify the legacy
	// per-episode endpoint when we DO have an EpisodeID linkage.
	thumbBase := thumbnailsBaseURLFromMaster(streamingURL)
	if !job.EpisodeID.IsZero() {
		if err := p.notifyEpisodeComplete(ctx, job, streamingURL, thumbBase, ThumbnailIntervalSeconds); err != nil {
			log.Printf("[EPISODE] WARNING: notifyEpisodeComplete failed: %v", err)
			// Non-fatal: the HLS exists, admin can re-run or patch the row.
		}
	} else {
		log.Printf("[EPISODE] deferred-DB mode: EpisodeID is zero — backend finalizer will create the row")
	}

	// === Clip generation ===
	// Trigger at the episode level before the job is marked completed so the
	// status reflects the full ingestion lifecycle, not just HLS availability.
	log.Printf("[CLIPS] episode completed → generating clips")
	log.Printf("[EPISODE] clip_generation start series_slug=%s season=%d episode=%d processed_master=%s local_path=%s",
		job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, processedMaster, localPath)
	if clipErr := p.generateEpisodeClips(ctx, job, processedMaster); clipErr != nil {
		// Non-fatal: episode video is live; surface the failure in logs so
		// an operator can re-run the clip stage if needed.
		log.Printf("[EPISODE] WARNING: clip generation failed series_slug=%s S%02dE%02d: %v",
			job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, clipErr)
	} else {
		log.Printf("[EPISODE] clip_generation end series_slug=%s season=%d episode=%d",
			job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber)
	}

	// Source file is no longer needed.
	p.cleanupFile(localPath)

	// Clean the processed master + readyvideo working dir.
	if processedMaster != "" {
		if _, statErr := os.Stat(processedMaster); statErr == nil {
			if rmErr := os.Remove(processedMaster); rmErr != nil {
				log.Printf("[EPISODE] WARNING: remove processed_master: %v", rmErr)
			}
		}
	}
	if hlsDir != "" {
		if _, err := os.Stat(hlsDir); err == nil {
			if err := os.RemoveAll(hlsDir); err != nil {
				log.Printf("[EPISODE] WARNING: cleanup readyvideo %s: %v", hlsDir, err)
			}
		}
	}

	// Prefix-safe post-completion sweep — same guarantees as the movie
	// pipeline. Catches the parser MP4 if `localPath` ever pointed at
	// something the inline cleanupFile refused (path mismatch, race with a
	// parallel write, etc.). In dev mode this is a no-op.
	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		log.Printf("[EPISODE] WARNING: final status update failed: %v", err)
	}
	job.Status = models.IngestionStatusCompleted
	p.CleanupAfterSuccess(job, localPath, hlsDir)

	log.Printf("[EPISODE] done job=%s series=%s S%02dE%02d streaming=%s",
		jobID, job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, streamingURL)
	return nil
}

func (p *Pipeline) notifyEpisodeComplete(ctx context.Context, job *models.IngestionJob, streamingURL, thumbnailsBaseURL string, thumbnailInterval int) error {
	backendURL := p.config.BackendURL
	if backendURL == "" {
		return fmt.Errorf("BackendURL not configured; cannot notify episode completion")
	}
	endpoint := fmt.Sprintf("%s/api/ingestion/episodes/%s/complete",
		backendURL, job.EpisodeID.Hex())

	payload := map[string]interface{}{
		"video_url": streamingURL,
		"duration":  0,
	}
	if thumbnailsBaseURL != "" {
		payload["thumbnails_base_url"] = thumbnailsBaseURL
		payload["thumbnail_interval"] = thumbnailInterval
	}
	body, _ := json.Marshal(payload)

	reqCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.config.WorkerToken != "" {
		req.Header.Set("X-Worker-Token", p.config.WorkerToken)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("backend returned %d on episode-complete", resp.StatusCode)
	}
	return nil
}
