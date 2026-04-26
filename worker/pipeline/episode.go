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
	// Sanity: required linkage fields.
	if job.EpisodeID.IsZero() || job.SeriesSlug == "" {
		return fmt.Errorf("episode job %s missing series/episode linkage", jobID)
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

	// Folder layout: serials/<slug>/season-N/episode-M
	// Nested under serial → season → episode so master.m3u8 and the rendition
	// folders (1080p/, 720p/, …) live together at the episode root in B2 as:
	//   videos/serials/<slug>/season-N/episode-M/master.m3u8
	//   videos/serials/<slug>/season-N/episode-M/<quality>/index.m3u8
	//   videos/serials/<slug>/season-N/episode-M/<quality>/segment_*.ts
	folderName := filepath.Join(
		"serials",
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
	log.Printf("[EPISODE] b2_root=videos/%s master_b2_key=videos/%s/master.m3u8", folderName, folderName)
	log.Printf("[EPISODE] streamingURL=%s", streamingURL)

	mode := p.config.StorageConfig.Mode
	outputMode := "development"
	finalPath := streamingURL
	if mode != "prod" && mode != "production" {
		outputMode = "development"
		baseDir, _ := os.Getwd()
		finalPath = filepath.Join(baseDir, "uploads", "movies", folderName)
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
	if err := p.notifyEpisodeComplete(ctx, job, streamingURL); err != nil {
		log.Printf("[EPISODE] WARNING: notifyEpisodeComplete failed: %v", err)
		// Non-fatal: the HLS exists, admin can re-run or patch the row.
	}

	// === Clip generation ===
	// Must run AFTER the episode's HLS is saved and the backend has been
	// notified, but BEFORE processed_master.mp4 is cleaned up below —
	// clip generation consumes processed_master as its source.
	// This matches the movie pipeline's ordering (see pipeline.go where
	// generateClips is invoked ahead of the processed-master cleanup).
	log.Printf("[EPISODE] clip_generation start series_slug=%s season=%d episode=%d processed_master=%s",
		job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, processedMaster)
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

	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		log.Printf("[EPISODE] WARNING: final status update failed: %v", err)
	}
	log.Printf("[EPISODE] done job=%s series=%s S%02dE%02d streaming=%s",
		jobID, job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber, streamingURL)
	return nil
}

func (p *Pipeline) notifyEpisodeComplete(ctx context.Context, job *models.IngestionJob, streamingURL string) error {
	backendURL := p.config.BackendURL
	if backendURL == "" {
		return fmt.Errorf("BackendURL not configured; cannot notify episode completion")
	}
	endpoint := fmt.Sprintf("%s/api/ingestion/episodes/%s/complete",
		backendURL, job.EpisodeID.Hex())

	body, _ := json.Marshal(map[string]interface{}{
		"video_url": streamingURL,
		"duration":  0,
	})

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
