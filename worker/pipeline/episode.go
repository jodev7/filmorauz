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
//	videos/serials/<series-slug>/season-<N>/episode-<M>_<shortJobId>/
//
// and, on success, notifies the backend to attach the master playlist URL
// to the linked Episode row. This is deliberately a reduced pipeline: no
// TMDB enrichment, no movie-code assignment, no Telegram notification.
func (p *Pipeline) processEpisodeJob(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[EPISODE] start job=%s series=%s S%02dE%02d",
		jobID, job.SeriesSlug, job.SeasonNumber, job.EpisodeNumber)

	if job.Metadata == nil {
		return fmt.Errorf("episode job %s has no metadata", jobID)
	}
	videoURL := job.VideoURL
	if videoURL == "" {
		videoURL = job.Metadata.VideoURL
	}
	if videoURL == "" {
		return fmt.Errorf("episode job %s missing video_url (parser should have pre-resolved it)", jobID)
	}

	// Sanity: required linkage fields.
	if job.EpisodeID.IsZero() || job.SeriesSlug == "" {
		return fmt.Errorf("episode job %s missing series/episode linkage", jobID)
	}

	// Persist video_url onto the job (pollDownloadProgress reads job.VideoURL
	// for restart safety, matching the movie pipeline's contract).
	if job.VideoURL == "" {
		if err := p.jobRepo.UpdateVideoURL(ctx, jobID, videoURL); err != nil {
			log.Printf("[EPISODE] WARNING: persist video_url failed: %v", err)
		}
		job.VideoURL = videoURL
	}

	// === Download phase ===
	if err := p.updateStatus(jobID, models.IngestionStatusDownloading, 10); err != nil {
		return fmt.Errorf("episode update_status downloading: %w", err)
	}

	var localPath string
	if job.LocalPath != "" {
		if fi, err := os.Stat(job.LocalPath); err == nil && fi.Size() > 0 {
			log.Printf("[EPISODE] reusing existing local_path=%s", job.LocalPath)
			localPath = job.LocalPath
		}
	}
	if localPath == "" {
		if !job.Steps.DownloadStarted {
			if err := p.jobRepo.MarkDownloadStarted(ctx, jobID); err != nil {
				log.Printf("[EPISODE] WARNING: MarkDownloadStarted: %v", err)
			}
			lp, err := p.startDownloadAndPoll(ctx, job)
			if err != nil {
				return fmt.Errorf("episode download failed: %w", err)
			}
			localPath = lp
		} else {
			lp, err := p.pollDownloadProgress(ctx, job, videoURL)
			if err != nil {
				return fmt.Errorf("episode resume-poll failed: %w", err)
			}
			localPath = lp
		}
	}

	if localPath == "" {
		return fmt.Errorf("episode download returned empty local_path")
	}
	if err := p.jobRepo.TransitionToProcessing(ctx, jobID, localPath); err != nil {
		log.Printf("[EPISODE] WARNING: TransitionToProcessing: %v", err)
	}

	// === Process + upload phase ===
	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 50); err != nil {
		return fmt.Errorf("episode update_status processing: %w", err)
	}

	// Folder layout: serials/<slug>/season-N/episode-M_<shortJobId>
	// This keeps each episode's HLS assets isolated while preserving the
	// grouping the user asked for.
	shortID := jobID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	folderName := filepath.Join(
		"serials",
		job.SeriesSlug,
		fmt.Sprintf("season-%d", job.SeasonNumber),
		fmt.Sprintf("episode-%d_%s", job.EpisodeNumber, shortID),
	)
	log.Printf("[EPISODE] folder=%s", folderName)

	hlsDir, processedMaster, err := p.processVideo(job, localPath, folderName)
	if err != nil {
		return fmt.Errorf("episode processVideo failed: %w", err)
	}
	if stepErr := p.jobRepo.UpdateStep(ctx, jobID, "process"); stepErr != nil {
		log.Printf("[EPISODE] WARNING: mark process step failed: %v", stepErr)
	}
	if err := p.jobRepo.MarkSourceFileDeleted(ctx, jobID, hlsDir); err != nil {
		log.Printf("[EPISODE] WARNING: MarkSourceFileDeleted: %v", err)
	}

	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 70); err != nil {
		return fmt.Errorf("episode update_status uploading: %w", err)
	}
	streamingURL, _, err := p.uploadProcessedFiles(job, hlsDir, localPath, folderName)
	if err != nil {
		return fmt.Errorf("episode uploadProcessedFiles failed: %w", err)
	}
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

	// === Notify backend so the Episode row gets the playback URL ===
	if err := p.notifyEpisodeComplete(ctx, job, streamingURL); err != nil {
		log.Printf("[EPISODE] WARNING: notifyEpisodeComplete failed: %v", err)
		// Non-fatal: the HLS exists, admin can re-run or patch the row.
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
