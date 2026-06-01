package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/filmorauz/worker/models"
	"github.com/filmorauz/worker/repositories"
	"github.com/filmorauz/worker/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Safe string truncation - never panics
func safeTruncate(s string, maxLen int) string {
	if s == "" {
		return ""
	}
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen]
}

func formatOptionalTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func formatExitCode(code *int) string {
	if code == nil {
		return ""
	}
	return strconv.Itoa(*code)
}

func normalizeDuplicateTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	re := regexp.MustCompile(`\s+`)
	return strings.ToLower(re.ReplaceAllString(trimmed, " "))
}

func normalizeQualityLabel(label string) string {
	lower := strings.ToLower(strings.TrimSpace(label))
	switch lower {
	case "", "unknown":
		return "unknown"
	case "full hd", "fhd":
		return "1080p"
	case "hd":
		return "720p"
	case "sd":
		return "480p"
	case "original", "source", "auto":
		return lower
	}
	match := regexp.MustCompile(`(2160|1440|1080|720|480|360|240)`).FindString(lower)
	if match != "" {
		return match + "p"
	}
	return lower
}

func qualityHeight(label, rawURL string) int {
	normalized := normalizeQualityLabel(label)
	match := regexp.MustCompile(`(2160|1440|1080|720|480|360|240)`).FindString(normalized)
	if match != "" {
		if n, err := strconv.Atoi(match); err == nil {
			return n
		}
	}
	match = regexp.MustCompile(`(2160|1440|1080|720|480|360|240)`).FindString(strings.ToLower(rawURL))
	if match != "" {
		if n, err := strconv.Atoi(match); err == nil {
			return n
		}
	}
	if normalized == "original" {
		return 10000
	}
	return 0
}

func normalizeMediaType(rawType, rawURL string) string {
	lowerType := strings.ToLower(strings.TrimSpace(rawType))
	lowerURL := strings.ToLower(strings.TrimSpace(rawURL))
	switch {
	case strings.Contains(lowerURL, ".m3u8") || lowerType == "hls" || lowerType == "m3u8":
		return "m3u8"
	case strings.Contains(lowerURL, ".mpd") || lowerType == "dash" || lowerType == "mpd":
		return "mpd"
	case strings.Contains(lowerURL, ".mp4") || lowerType == "mp4" || lowerType == "direct_mp4" || lowerType == "direct_download":
		return "mp4"
	case lowerType != "":
		return lowerType
	default:
		return "unknown"
	}
}

func normalizeIdentityURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return trimmed
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimSuffix(parsed.Path, "/")
	return strings.ToLower(parsed.Scheme + "://" + parsed.Host + parsed.Path)
}

func expectedQualitySatisfied(selected string, actualHeight int) bool {
	want := qualityHeight(selected, "")
	if want == 0 || actualHeight <= 0 {
		return true
	}
	switch {
	case want >= 1080:
		return actualHeight >= 900
	case want >= 720:
		return actualHeight >= 600
	case want >= 480:
		return actualHeight >= 400
	default:
		return actualHeight >= want-40
	}
}

func chooseDownloadCandidates(meta *models.ParsedMovieMetadata, selectedURL string) []models.VideoSource {
	if meta == nil {
		return nil
	}
	candidates := append([]models.VideoSource(nil), meta.VideoURLs...)
	if len(candidates) == 0 && strings.TrimSpace(meta.VideoURL) != "" {
		candidates = append(candidates, models.VideoSource{
			URL:     meta.VideoURL,
			Type:    "unknown",
			Quality: meta.Quality,
		})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		left := qualityHeight(candidates[i].Quality, candidates[i].URL)
		right := qualityHeight(candidates[j].Quality, candidates[j].URL)
		if left != right {
			return left > right
		}
		leftType := strings.ToLower(candidates[i].Type)
		rightType := strings.ToLower(candidates[j].Type)
		typeRank := map[string]int{"m3u8": 4, "hls": 4, "mpd": 3, "ism": 2, "mp4": 1}
		if typeRank[leftType] != typeRank[rightType] {
			return typeRank[leftType] > typeRank[rightType]
		}
		return len(candidates[i].URL) < len(candidates[j].URL)
	})
	if strings.TrimSpace(selectedURL) != "" {
		for idx := range candidates {
			if strings.TrimSpace(candidates[idx].URL) == strings.TrimSpace(selectedURL) {
				chosenHeight := qualityHeight(candidates[idx].Quality, candidates[idx].URL)
				topHeight := 0
				if len(candidates) > 0 {
					topHeight = qualityHeight(candidates[0].Quality, candidates[0].URL)
				}
				if chosenHeight >= topHeight {
					chosen := candidates[idx]
					candidates = append([]models.VideoSource{chosen}, append(candidates[:idx], candidates[idx+1:]...)...)
				}
				break
			}
		}
	}
	return candidates
}

type needsManualError struct {
	Reason string
}

func (e *needsManualError) Error() string {
	if e == nil || e.Reason == "" {
		return "needs manual review"
	}
	return e.Reason
}

// Config holds pipeline configuration
type Config struct {
	ParserURL                 string
	TempDir                   string
	StorageConfig             storage.Config
	TMDBAPIKey                string          // TMDB API key for metadata enrichment
	DB                        *mongo.Database // MongoDB database for movie insertion
	BackendURL                string          // Backend API URL for Telegram notifications
	WorkerToken               string          // Token for worker-to-backend authentication
	RequireClipsBeforePublish bool            // Block final completion until clips succeed
	MaxRenditionConcurrent    int             // Max concurrent FFmpeg processes (default: 2)
	SegmentUploadWorkers      int             // Concurrent segment uploads per rendition (default: 10)
	SegmentUploadRetries      int             // Max retries per segment (default: 5)
	SegmentDuration           int             // HLS segment duration in seconds (default: 6)
}

// Pipeline handles the video ingestion pipeline
type Pipeline struct {
	config     Config
	jobRepo    *repositories.JobRepository
	storage    storage.Storage
	httpClient *http.Client
	enricher   *MetadataEnricher
	movieCol   *mongo.Collection // MongoDB movies collection for direct insertion
	dbName     string            // Database name for logging
}

// NewPipeline creates a new pipeline instance
func NewPipeline(config Config, jobRepo *repositories.JobRepository) (*Pipeline, error) {
	store, err := storage.NewStorage(config.StorageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create metadata enricher with TMDB support
	enricher := NewMetadataEnricher(config.ParserURL, config.TMDBAPIKey)

	// Initialize movie collection for direct MongoDB insertion
	var movieCol *mongo.Collection
	dbName := "filmorauz" // Default database name
	if config.DB != nil {
		movieCol = config.DB.Collection("movies")
		dbName = config.DB.Name()
		log.Printf("[PIPELINE] Movie collection initialized: db=%s, collection=movies", dbName)
	} else {
		log.Printf("[PIPELINE] WARNING: No database provided, movie creation will be skipped")
	}

	return &Pipeline{
		config:  config,
		jobRepo: jobRepo,
		storage: store,
		httpClient: &http.Client{
			Timeout: 60 * time.Minute, // Increased to 60 minutes for large file downloads
		},
		enricher: enricher,
		movieCol: movieCol,
		dbName:   dbName,
	}, nil
}

// ProcessJob processes a single ingestion job
// This function is wrapped with panic recovery to ensure worker stability
func (p *Pipeline) ProcessJob(ctx context.Context, job *models.IngestionJob) error {
	// Defensive check: job must not be nil
	if job == nil {
		return fmt.Errorf("job is nil - cannot process")
	}

	// CRITICAL: Use a channel to capture panic result and ensure proper error propagation
	panicChan := make(chan error, 1)

	// Wrap entire processing in panic recovery
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic with stack trace
				log.Printf("[PANIC RECOVERY] ProcessJob panicked: %v", r)
				// CRITICAL: Mark job as FAILED when panic occurs
				jobID := job.ID.Hex()
				p.failJob(jobID, fmt.Sprintf("panic: %v", r))
				log.Printf("[PANIC] Job %s marked as FAILED due to panic", jobID)
				// Send error through channel to ensure job is marked as failed
				panicChan <- fmt.Errorf("panic: %v", r)
			}
		}()

		// Process the job - errors will be sent through channel
		jobID := job.ID.Hex()
		err := p.processJobWithRecovery(ctx, job)
		if err != nil {
			// Shutdown / cancellation is transient — leave the job in its
			// current status so RecoverStaleJobs requeues it on next worker
			// startup. Marking it as failed here turns a clean restart into
			// permanent data loss (which is exactly what bit the three
			// asilmedia jobs during a 08:57 systemctl restart mid-clip-gen).
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				log.Printf("[PIPELINE] job %s interrupted by shutdown (%v); leaving for recovery on next start", jobID, err)
				panicChan <- err
				return
			}
			// Ensure job is marked as failed on any other error
			p.failJob(jobID, fmt.Sprintf("Pipeline failed: %v", err))
			panicChan <- err
			return
		}
		// Success - send nil to indicate completion without error
		panicChan <- nil
	}()

	// Wait for processing to complete
	result := <-panicChan
	if result != nil {
		return result
	}

	log.Printf("[PIPELINE] Job %s completed successfully", job.ID.Hex())
	return nil
}

func (p *Pipeline) ProcessDownloadJob(ctx context.Context, job *models.IngestionJob) error {
	if job == nil {
		return fmt.Errorf("job is nil - cannot process download")
	}

	jobID := job.ID.Hex()
	log.Printf("[WORKER] download worker processing job_id=%s source=%s source_id=%s", jobID, job.Source, job.SourceID)

	metadata, localPath, err := p.parseMovieDetailsWithRetry(job)
	if err != nil {
		return p.failDownloadJob(jobID, fmt.Errorf("parse details failed: %w", err))
	}
	if metadata == nil {
		return p.failDownloadJob(jobID, fmt.Errorf("parser returned nil metadata"))
	}

	if err := p.jobRepo.UpdateMetadata(ctx, jobID, metadata); err != nil {
		log.Printf("[WORKER] download metadata update failed job_id=%s err=%v", jobID, err)
	}
	job.ClassifierConfidence = metadata.ClassifierConfidence
	job.ClassifierEvidence = metadata.ClassifierEvidence

	candidates := chooseDownloadCandidates(metadata, metadata.VideoURL)
	if len(candidates) == 0 {
		return p.failDownloadJob(jobID, fmt.Errorf("download_url empty after parser resolve"))
	}
	availableQualities := []string{}
	seenQualities := map[string]struct{}{}
	for _, candidate := range candidates {
		q := normalizeQualityLabel(candidate.Quality)
		if q != "" && q != "unknown" {
			if _, ok := seenQualities[q]; !ok {
				seenQualities[q] = struct{}{}
				availableQualities = append(availableQualities, q)
			}
		}
	}
	videoURL := strings.TrimSpace(candidates[0].URL)
	if videoURL == "" {
		return p.failDownloadJob(jobID, fmt.Errorf("download_url empty after parser resolve"))
	}
	// Some sources (kinochilar, uzmedia, kinolar) return the embed
	// iframe wrapper URL (e.g. /player/playerjs.html?file=https://...)
	// instead of the bare video URL. Unwrap before validation /
	// download so the worker handles every source uniformly.
	videoURL = unwrapEmbedURL(videoURL)
	selectedQuality := normalizeQualityLabel(firstNonEmptyString(candidates[0].Quality, metadata.Quality))
	mediaType := normalizeMediaType(candidates[0].Type, videoURL)
	log.Printf("[download-debug] job_id=%s source=%s detail_url=%s video_url=%s download_url=%s selected_quality=%q media_type=%s",
		jobID, job.Source, safeTruncate(job.DetailURL, 180), safeTruncate(videoURL, 180), safeTruncate(videoURL, 180), selectedQuality, mediaType)
	if err := p.jobRepo.UpdateSourceSelection(ctx, jobID, videoURL, selectedQuality, availableQualities, job.ClassifierConfidence, job.ClassifierEvidence); err != nil {
		log.Printf("[WORKER] failed to persist source selection job_id=%s err=%v", jobID, err)
	}

	if err := validateDownloadURL(videoURL); err != nil {
		return p.failDownloadJob(jobID, fmt.Errorf("invalid video_url %q: %w", videoURL, err))
	}

	if localPath != "" {
		log.Printf("[WORKER] parser already has local file job_id=%s local_path=%s", jobID, localPath)
		if err := p.jobRepo.TransitionToProcessing(ctx, jobID, localPath); err != nil {
			return p.failDownloadJob(jobID, fmt.Errorf("transition to ready_to_process failed: %w", err))
		}
		return nil
	}

	job.Metadata = metadata
	var downloadErr error
	for idx, candidate := range candidates {
		job.VideoURL = strings.TrimSpace(candidate.URL)
		if job.VideoURL == "" {
			downloadErr = fmt.Errorf("download_url empty after parser resolve")
			break
		}
		job.SourceQuality = normalizeQualityLabel(firstNonEmptyString(candidate.Quality, metadata.Quality))
		mediaType := normalizeMediaType(candidate.Type, job.VideoURL)
		log.Printf("[WORKER] downloader command job_id=%s source=%s detail_url=%s endpoint=%s/download candidate=%d/%d selected_quality=%s media_type=%s video_url=%s download_url=%s",
			jobID, job.Source, safeTruncate(job.DetailURL, 180), p.config.ParserURL, idx+1, len(candidates), job.SourceQuality, mediaType, safeTruncate(job.VideoURL, 180), safeTruncate(job.VideoURL, 180))
		if err := p.jobRepo.UpdateSourceSelection(ctx, jobID, job.VideoURL, job.SourceQuality, availableQualities, job.ClassifierConfidence, job.ClassifierEvidence); err != nil {
			log.Printf("[WORKER] failed to persist candidate selection job_id=%s err=%v", jobID, err)
		}

		localPath, err = p.startDownloadAndPoll(ctx, job, idx > 0)
		if err != nil {
			downloadErr = err
			continue
		}
		if width, height, probeErr := p.getInputResolution(localPath); probeErr == nil {
			job.SourceResolution = fmt.Sprintf("%dx%d", width, height)
			if err := p.jobRepo.UpdateSourceResolution(ctx, jobID, job.SourceResolution); err != nil {
				log.Printf("[WORKER] failed to persist source resolution job_id=%s err=%v", jobID, err)
			}
			log.Printf("[WORKER] ffprobe validation job_id=%s selected_quality=%s actual_height=%d actual_width=%d", jobID, job.SourceQuality, height, width)
			if !expectedQualitySatisfied(job.SourceQuality, height) {
				downloadErr = fmt.Errorf("selected_quality=%s but actual source resolution=%dx%d for selected_url=%s", job.SourceQuality, width, height, job.VideoURL)
				p.cleanupFile(localPath)
				localPath = ""
				log.Printf("[WORKER] source quality mismatch job_id=%s err=%v", jobID, downloadErr)
				continue
			}
		}
		downloadErr = nil
		break
	}
	if downloadErr != nil {
		return p.failDownloadJob(jobID, fmt.Errorf("download failed for url %q: %w", videoURL, downloadErr))
	}

	if err := p.jobRepo.TransitionToProcessing(ctx, jobID, localPath); err != nil {
		return p.failDownloadJob(jobID, fmt.Errorf("transition to ready_to_process failed: %w", err))
	}

	log.Printf("[WORKER] download worker completed job_id=%s local_path=%s", jobID, localPath)
	return nil
}

// processJobWithRecovery is the actual processing logic wrapped with panic recovery
func (p *Pipeline) processJobWithRecovery(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[PIPELINE] Starting processing for job %s", jobID)

	// Global heartbeat for the entire pipeline execution
	heartbeatCtx, heartbeatCancel := context.WithCancel(ctx)
	defer heartbeatCancel()
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-ticker.C:
				if err := p.jobRepo.Heartbeat(context.Background(), jobID); err != nil {
					log.Printf("[HEARTBEAT] Error updating heartbeat for job %s: %v", jobID, err)
				}
			}
		}
	}()

	// Handle direct_upload source - download from B2 temp path
	if job.Source == "direct_upload" {
		// A manually-uploaded serial episode reuses the episode pipeline
		// (per-episode HLS folder + Episode-row linkage) instead of the
		// movie-creation path.
		if job.ContentType == "episode" {
			return p.processEpisodeDirectUploadJob(ctx, job)
		}
		return p.processDirectUploadJob(ctx, job)
	}

	// Serial episode jobs skip the movie-enrichment + movie-creation path and
	// instead save their HLS into a per-episode folder and update the linked
	// Episode row on the backend.
	if job.ContentType == "episode" {
		return p.processEpisodeJob(ctx, job)
	}

	// Clip-only jobs are enqueued by the backend serial finalizer once the
	// Episode row exists. They re-download the HLS via ffmpeg and run the
	// standard episode clip generator with EpisodeID populated.
	if job.ContentType == "clip_only" {
		return p.processClipOnlyJob(ctx, job)
	}

	// CRITICAL: Create safe variables for metadata fields to prevent nil pointer access
	// These will be used throughout the processing instead of direct job.Metadata.* access
	title := "video"
	year := 0
	if job.Metadata != nil && job.Metadata.Title != "" {
		title = job.Metadata.Title
		year = job.Metadata.Year
	}
	log.Printf("[WORKER] Processing job with title=%s, year=%d", title, year)

	// Update status to parsing
	if err := p.updateStatus(jobID, models.IngestionStatusParsing, 10); err != nil {
		return fmt.Errorf("failed to update status to parsing: %w", err)
	}

	var metadata *models.ParsedMovieMetadata
	localPath := job.LocalPath
	var err error

	if localPath == "" {
		log.Printf("[PROCESS] skipped job=%s reason=missing local_path", jobID)
		return fmt.Errorf("job %s is not ready_to_process: local_path is empty", jobID)
	}
	if fileInfo, statErr := os.Stat(localPath); statErr != nil || fileInfo.Size() == 0 {
		if statErr != nil {
			log.Printf("[ERROR] file missing at %s", localPath)
			return fmt.Errorf("downloaded file is unavailable: %w", statErr)
		}
		log.Printf("[ERROR] file missing at %s", localPath)
		return fmt.Errorf("downloaded file is empty: %s", localPath)
	}

	metadata = job.Metadata
	meta, _, parseErr := p.parseMovieDetails(job)
	if parseErr != nil {
		var manualErr *needsManualError
		if errors.As(parseErr, &manualErr) {
			reason := strings.TrimSpace(manualErr.Reason)
			if reason == "" {
				reason = "parser could not resolve video metadata"
			}
			if markErr := p.markJobNeedsManual(jobID, reason); markErr != nil {
				return fmt.Errorf("mark needs_manual: %w", markErr)
			}
			log.Printf("[PIPELINE] job %s moved to needs_manual: %s", jobID, reason)
			return nil
		}
		log.Printf("[PIPELINE] WARNING: parser metadata refresh failed for job %s: %v", jobID, parseErr)
	} else if meta != nil {
		metadata = meta
	}

	// Defensive check: validate metadata is not nil after parsing
	if metadata == nil {
		return fmt.Errorf("parser returned nil metadata")
	}

	log.Printf("[PIPELINE] Parsed movie: %s (%d)", metadata.Title, metadata.Year)

	// Save metadata
	if err := p.jobRepo.UpdateMetadata(ctx, jobID, metadata); err != nil {
		log.Printf("[PIPELINE] Failed to update metadata: %v", err)
		// Continue processing - this is not fatal
	}

	// Step 2: Get video path
	// If parser already downloaded the file (localPath != ""), use it
	// Otherwise, downloadVideo will check job.LocalPath and use that if available
	var videoPath string
	videoPath = localPath
	log.Printf("[STAGE] download complete — using parser-downloaded file: %s", videoPath)

	// Probe the downloaded source so we can record what we actually got and
	// surface a warning if the parser claimed a higher quality than the file
	// proves out. Non-fatal — we still process whatever was delivered.
	if videoPath != "" {
		if fi, statErr := os.Stat(videoPath); statErr == nil {
			log.Printf("[WORKER] downloaded file size=%d bytes (%.2f MB) path=%s", fi.Size(), float64(fi.Size())/(1024*1024), videoPath)
			p.log(jobID, fmt.Sprintf("Downloaded file size: %.2f MB", float64(fi.Size())/(1024*1024)), "info")
		}
		if w, h, probeErr := p.getInputResolution(videoPath); probeErr == nil {
			log.Printf("[WORKER] ffprobe source resolution: %dx%d (selected_quality=%q)", w, h, job.SourceQuality)
			p.log(jobID, fmt.Sprintf("Source resolution: %dx%d (selected_quality=%s)", w, h, job.SourceQuality), "info")
			if strings.EqualFold(strings.TrimSpace(job.SourceQuality), "1080p") && h > 0 && h < 900 {
				warn := fmt.Sprintf("WARNING: selected_quality=1080p but downloaded source height=%d (<900) — parser may have picked the wrong quality", h)
				log.Printf("[WORKER] %s", warn)
				p.log(jobID, warn, "warn")
			}
		} else {
			log.Printf("[WORKER] ffprobe source resolution failed: %v", probeErr)
		}
	}
	// NOTE: do NOT defer cleanupFile here — file must survive retries until processing succeeds

	// Update status to processing
	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 50); err != nil {
		return fmt.Errorf("failed to update status to processing: %w", err)
	}

	// ===== COMPUTE CANONICAL MOVIE FOLDER ONCE =====
	// This ensures ALL assets (HLS, poster, backdrop) are in the SAME folder
	// Use the same title source as processVideo (job.Title or job.Metadata.Title)
	canonicalTitle := "video"
	if job.Title != "" {
		canonicalTitle = job.Title
	} else if job.Metadata != nil && job.Metadata.Title != "" {
		canonicalTitle = job.Metadata.Title
	}
	canonicalSlug := createMovieSlug(canonicalTitle)
	shortJobId := job.ID.Hex()
	if len(shortJobId) > 8 {
		shortJobId = shortJobId[:8]
	}
	canonicalFolderName := fmt.Sprintf("%s_%s", canonicalSlug, shortJobId)
	log.Printf("[PIPELINE] Canonical movie folder (computed once): %s", canonicalFolderName)
	log.Printf("[PIPELINE] Canonical title source: %s", canonicalTitle)

	// ===== PROCESSING STAGE: FFmpeg logo overlay + HLS =====
	// Step 3: Process video (FFmpeg logo overlay + HLS conversion)
	// No watermark removal - use source video directly
	log.Printf("[CHECKPOINT] raw_downloaded_path: %s", localPath)
	log.Printf("[STAGE] process start — input: %s, folder: %s", videoPath, canonicalFolderName)
	hlsPath, processedMasterPath, err := p.processVideo(job, localPath, canonicalFolderName)
	if err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}
	log.Printf("[CHECKPOINT] processed_master_path: %s", processedMasterPath)
	log.Printf("[CHECKPOINT] hls_dir: %s", hlsPath)
	log.Printf("[STAGE] process complete — hlsPath: %s, processedMaster: %s", hlsPath, processedMasterPath)

	// Mark process step done so ClaimNextProcessingJob won't re-pick this job
	if stepErr := p.jobRepo.UpdateStep(ctx, jobID, "process"); stepErr != nil {
		log.Printf("[STAGE] WARNING: failed to mark process step: %v", stepErr)
	}

	// Update status to uploading
	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 70); err != nil {
		return fmt.Errorf("failed to update status to uploading: %w", err)
	}

	// Step 4: Upload/save processed files (MODE-based finalization)
	// Development: save to worker/uploads/movies/<folder>/
	// Production: upload to B2/CDN
	log.Printf("[STAGE] upload_save start — hlsPath: %s, mode: %s", hlsPath, p.config.StorageConfig.Mode)
	streamingURL, finalUploadsPath, err := p.uploadProcessedFiles(job, hlsPath, localPath, canonicalFolderName)
	if err != nil {
		return fmt.Errorf("upload/save failed: %w", err)
	}
	job.MasterPlaylistURL = streamingURL
	log.Printf("[STAGE] upload_save end — streamingURL: %s", streamingURL)
	log.Printf("[PIPELINE] Final uploads path for clip generation: %s", finalUploadsPath)

	// Update final output path based on MODE
	// After finalization, the output_path should reflect the final location
	mode := p.config.StorageConfig.Mode
	outputMode := "development"
	finalPath := ""
	if mode == "prod" || mode == "production" {
		outputMode = "production"
		finalPath = streamingURL // CDN URL for production
	} else {
		outputMode = "development"
		// For dev, construct local path using canonical folder name
		baseDir, _ := os.Getwd()
		finalPath = filepath.Join(baseDir, "uploads", "movies", canonicalFolderName)
	}

	// Update final output info in database
	if err := p.jobRepo.UpdateFinalOutputPath(ctx, jobID, finalPath, streamingURL, outputMode); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update final output path: %v", err)
		// Continue - not fatal
	} else {
		log.Printf("[PIPELINE] Updated final output: mode=%s, path=%s", outputMode, finalPath)
	}

	// Clear local_path only after processing completed and the output is safely stored.
	if err := p.jobRepo.MarkSourceFileDeleted(ctx, jobID, hlsPath); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to mark source file deleted: %v", err)
	} else {
		log.Printf("[PIPELINE] Marked source file deleted after upload, output_path=%s, cleared local_path", hlsPath)
	}

	// Update progress to 90%
	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 90); err != nil {
		return fmt.Errorf("failed to update status to 90%%: %w", err)
	}

	// Log final message with naming convention
	movieSlug := sanitizeFilename(title)
	p.log(jobID, fmt.Sprintf("%s+uploaded", movieSlug), "info")

	// ===== STEP 5: ENRICH METADATA (from TMDB) =====
	p.log(jobID, "Starting metadata enrichment", "info")
	log.Printf("[STAGE] enrich_metadata start")
	if err := p.updateStatus(jobID, models.IngestionStatusEnrichingMetadata, 75); err != nil {
		return fmt.Errorf("failed to update status to enriching_metadata: %w", err)
	}

	var enrichedMetadata *models.EnrichedMetadata
	metadataSource := "parser"

	if p.enricher != nil && metadata != nil {
		enrichedMetadata, metadataSource, err = p.enricher.EnrichMetadata(ctx, metadata, job.Source)
		if err != nil {
			log.Printf("[PIPELINE] Metadata enrichment failed: %v", err)
			enrichedMetadata = MergeMetadata(nil, metadata)
			metadataSource = "parser"
		}

		if enrichedMetadata != nil {
			// Merge with parser metadata as fallback
			enrichedMetadata = MergeMetadata(enrichedMetadata, metadata)

			// Update job with enriched metadata
			if err := p.jobRepo.UpdateEnrichedMetadata(ctx, jobID, enrichedMetadata, metadataSource); err != nil {
				log.Printf("[PIPELINE] Failed to update enriched metadata: %v", err)
			}

			log.Printf("[PIPELINE] Enriched metadata: title=%s, year=%d, source=%s",
				enrichedMetadata.Title, enrichedMetadata.Year, metadataSource)
		}
	} else {
		// Fallback: convert parser metadata to enriched format
		enrichedMetadata = MergeMetadata(nil, metadata)
	}

	if enrichedMetadata == nil {
		return fmt.Errorf("failed to get enriched metadata")
	}

	p.log(jobID, fmt.Sprintf("Metadata enriched (source: %s)", metadataSource), "info")
	log.Printf("[STAGE] enrich_metadata end — title=%s, year=%d, source=%s", enrichedMetadata.Title, enrichedMetadata.Year, metadataSource)

	// ===== POSTER / BACKDROP (use TMDB/parser URLs, no generation) =====
	// No poster generation, no backdrop generation, no AI generation
	// Use URLs directly from enriched metadata
	log.Printf("[STAGE] poster_backdrop — using enriched metadata URLs")
	log.Printf("[STAGE] poster_url=%s backdrop_url=%s", enrichedMetadata.PosterURL, enrichedMetadata.BackdropURL)
	finalPosterURL := enrichedMetadata.PosterURL
	finalBackdropURL := enrichedMetadata.BackdropURL

	// ===== CREATE MOVIE IN DATABASE =====
	p.log(jobID, "Creating movie in database", "info")
	log.Printf("[STAGE] mongodb_save start — title=%s, code=%s, streamingURL=%s", enrichedMetadata.Title, "TBD", streamingURL)
	if err := p.updateStatus(jobID, models.IngestionStatusCreatingMovie, 95); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to creating_movie: %v", err)
	}

	// Log final metadata before DB write
	log.Printf("[PIPELINE] metadataSource=%s title=%q year=%d posterURL=%s", metadataSource, enrichedMetadata.Title, enrichedMetadata.Year, finalPosterURL)

	// Create movie — CRITICAL step
	log.Printf("[STAGE] saving movie to MongoDB — title: %s, streamingURL: %s", enrichedMetadata.Title, streamingURL)
	movieResult, err := p.createMovieInDatabaseWithEnrichment(ctx, job, enrichedMetadata, streamingURL, finalPosterURL, finalPosterURL, false, finalBackdropURL, finalBackdropURL, false, metadataSource)
	if err != nil {
		return fmt.Errorf("failed to create movie in database: %w", err)
	}
	log.Printf("[STAGE] mongodb_save end — movie_id=%v, code=%s, slug=%s", movieResult.MovieID, movieResult.Code, movieResult.Slug)

	// Persist hover-preview thumbnails URL on the movie row. Best-effort: if the
	// thumbnail step failed earlier, base URL will be empty and we skip the write.
	if thumbBase := thumbnailsBaseURLFromMaster(streamingURL); thumbBase != "" && p.movieCol != nil {
		var movieOID primitive.ObjectID
		switch v := movieResult.MovieID.(type) {
		case primitive.ObjectID:
			movieOID = v
		case string:
			if oid, err := primitive.ObjectIDFromHex(v); err == nil {
				movieOID = oid
			}
		}
		if !movieOID.IsZero() {
			_, err := p.movieCol.UpdateOne(ctx, bson.M{"_id": movieOID}, bson.M{
				"$set": bson.M{
					"thumbnails_base_url": thumbBase,
					"thumbnail_interval":  ThumbnailIntervalSeconds,
					"preview_sprite_url":  thumbBase + SpriteFileName,
					"preview_vtt_url":     thumbBase + VTTFileName,
				},
			})
			if err != nil {
				log.Printf("[PIPELINE] WARNING: failed to set thumbnail fields on movie: %v", err)
			} else {
				log.Printf("[PIPELINE] thumbnails_base_url=%s interval=%d", thumbBase, ThumbnailIntervalSeconds)
			}
		}
	}

	// Both upload and MongoDB save succeeded — safe to delete the original source file
	log.Printf("[STAGE] deleting source file after full success: %s", videoPath)
	p.cleanupFile(videoPath)

	// Update job with movie ID and movie data
	if movieResult != nil {
		if movieIDStr, ok := movieResult.MovieID.(string); ok {
			if objID, err := primitive.ObjectIDFromHex(movieIDStr); err == nil {
				if err := p.jobRepo.SetMovieID(ctx, jobID, objID); err != nil {
					log.Printf("[PIPELINE] Failed to set movie ID: %v", err)
				}
			}
		}
		// Update movie code and slug for Telegram notification
		if movieResult.Code != "" {
			if err := p.jobRepo.UpdateMovieData(ctx, jobID, movieResult.Code, movieResult.Slug); err != nil {
				log.Printf("[PIPELINE] Failed to update movie data: %v", err)
			}
		}
	}

	// ===== CLIP GENERATION =====
	// Generate 12 promotional clips after successful video + MongoDB save.
	// Clips are generated from the final processed movie (base_video.mp4).
	var clipGenerationFailed bool
	var clipInterrupted bool
	log.Printf("[STAGE] clip_generation start — movie code=%s, folder=%s, processed_master=%s", movieResult.Code, canonicalFolderName, processedMasterPath)
	log.Printf("[CHECKPOINT] clip_input_path: %s", processedMasterPath)
	if movieResult != nil && movieResult.Code != "" {
		if clipErr := p.generateClips(ctx, canonicalFolderName, movieResult.Code, movieResult, processedMasterPath, finalUploadsPath); clipErr != nil {
			log.Printf("[CLIP] ERROR: Clip generation failed: %v", clipErr)
			// Worker shutdown mid-clip is not a real clip failure — surface
			// the ctx error so ProcessJob's shutdown path can leave the job
			// in its current status for the next-start recovery, instead of
			// permanently failing a movie that's otherwise fully ingested.
			if errors.Is(clipErr, context.Canceled) || ctx.Err() != nil {
				clipInterrupted = true
			} else {
				clipGenerationFailed = true
			}
		} else {
			log.Printf("[STAGE] clip_generation end — clips generated from final movie")
		}
	}
	if clipInterrupted {
		return fmt.Errorf("clip generation interrupted by shutdown: %w", context.Canceled)
	}
	if movieResult != nil && movieResult.MovieID != nil {
		clipsStatus := "completed"
		pipelineStatus := "ready"
		pipelineComplete := true
		if clipGenerationFailed {
			clipsStatus = "failed"
			pipelineStatus = "waiting_for_clips"
			pipelineComplete = !p.config.RequireClipsBeforePublish
		}
		if err := p.updateMoviePipelineState(ctx, movieResult.MovieID, pipelineStatus, clipsStatus, pipelineComplete); err != nil {
			log.Printf("[PIPELINE] WARNING: failed to update movie pipeline state: %v", err)
		}
	}
	if clipGenerationFailed && p.config.RequireClipsBeforePublish {
		return fmt.Errorf("clip generation failed and REQUIRE_CLIPS_BEFORE_PUBLISH=true")
	}

	// ===== CLEANUP PROCESSED MASTER =====
	// Delete processed_master.mp4 now that HLS is uploaded and clips are saved.
	// Only reached here if HLS succeeded (processedMasterPath is set).
	log.Printf("[CLEANUP] processed_master cleanup start — path: %s", processedMasterPath)
	if processedMasterPath != "" {
		if _, statErr := os.Stat(processedMasterPath); statErr == nil {
			if rmErr := os.Remove(processedMasterPath); rmErr != nil {
				log.Printf("[CLEANUP] WARNING: failed to delete processed_master.mp4: %v", rmErr)
			} else {
				log.Printf("[CLEANUP] processed_master.mp4 deleted successfully: %s", processedMasterPath)
			}
		} else {
			log.Printf("[CLEANUP] processed_master.mp4 already gone: %s", processedMasterPath)
		}
	}
	log.Printf("[CLEANUP] processed_master cleanup end")

	// ===== CLEANUP READYVIDEO FOLDER =====
	// Delete readyvideo folder after movie save + clip stage complete
	log.Printf("[STAGE] cleanup start — hlsPath: %s", hlsPath)
	log.Printf("[CHECKPOINT] cleanup_target_path: %s", hlsPath)
	if hlsPath != "" {
		if _, err := os.Stat(hlsPath); err == nil {
			log.Printf("[CLEANUP] Deleting readyvideo folder: %s", hlsPath)
			if err := os.RemoveAll(hlsPath); err != nil {
				log.Printf("[CLEANUP] WARNING: Failed to delete readyvideo folder: %v", err)
			} else {
				log.Printf("[CLEANUP] Successfully deleted readyvideo folder: %s", hlsPath)
			}
		} else {
			log.Printf("[CLEANUP] readyvideo folder already deleted or does not exist: %s", hlsPath)
		}
	}
	log.Printf("[STAGE] cleanup end")

	// ===== TELEGRAM NOTIFICATION =====
	// Send Telegram notification after successful movie creation
	// This is idempotent - if already notified, it will be skipped
	if movieResult != nil && !movieResult.IsDuplicate {
		if err := p.sendTelegramNotification(ctx, jobID, job, enrichedMetadata, streamingURL, finalPosterURL, movieResult); err != nil {
			// Log error but don't fail the job - movie is already created
			log.Printf("[PIPELINE] WARNING: Telegram notification failed: %v", err)
			log.Printf("[PIPELINE] Movie is created successfully, Telegram failure is non-critical")
		}
	} else if movieResult != nil && movieResult.IsDuplicate {
		log.Printf("[TELEGRAM] SKIP duplicate movie job=%s movie_id=%v", jobID, movieResult.MovieID)
	}

	// ===== ASSET VALIDATION =====
	// Verify all assets are stored in the canonical folder
	p.validateAssetStorage(jobID, canonicalFolderName, finalPosterURL, finalBackdropURL, streamingURL)

	// ===== FINAL STATUS UPDATE =====
	// Mark job as completed - this is critical but we log the result
	// If clip generation failed, mark job with partial success
	if clipGenerationFailed {
		log.Printf("[PIPELINE] WARNING: Job completed with CLIP GENERATION FAILURE")
		log.Printf("[PIPELINE] Movie created successfully, but clips were not generated")
		// Keep movie as created, just log the clip failure
	}
	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		// Even if status update fails, log success because the work is done
		log.Printf("[PIPELINE] CRITICAL: Failed to update status to COMPLETED: %v", err)
		log.Printf("[PIPELINE] But job processing SUCCEEDED - movie created, poster handled")
		log.Printf("[PIPELINE] Please check database connectivity")
	} else {
		if clipGenerationFailed {
			log.Printf("[PIPELINE] Status updated to COMPLETED (100%%) with CLIP FAILURE")
		} else {
			log.Printf("[PIPELINE] Status updated to COMPLETED (100%%)")
		}

		// Prefix-safe post-completion sweep. The status was just set to
		// completed above; mirror it on the in-memory copy so
		// CleanupAfterSuccess's "must be completed" guard passes. Inline
		// cleanup blocks earlier in this function still run for the common
		// case — this is the safety net that catches anything they missed
		// (e.g. a later-bound LocalPath set after the first delete attempt).
		job.Status = models.IngestionStatusCompleted
		p.CleanupAfterSuccess(job, job.LocalPath, hlsPath)
	}

	// ===== FINAL JOB RESULT =====
	// Log comprehensive job completion summary
	// Compute slug for logging using the same logic as movieSlug function
	var finalSlugBuilder strings.Builder
	for _, c := range strings.ToLower(enrichedMetadata.Title) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			finalSlugBuilder.WriteRune(c)
		}
	}
	finalSlug := finalSlugBuilder.String()
	log.Printf("[PIPELINE] ===== JOB COMPLETED SUCCESSFULLY =====")
	log.Printf("[PIPELINE] Job ID: %s", jobID)
	log.Printf("[PIPELINE] Job Title: %s", enrichedMetadata.Title)
	log.Printf("[PIPELINE] Job Year: %d", enrichedMetadata.Year)
	log.Printf("[PIPELINE] Job Slug: %s", finalSlug)
	log.Printf("[PIPELINE] Canonical Movie Folder: %s", canonicalFolderName)
	log.Printf("[PIPELINE] Metadata Source: %s", metadataSource)

	// Asset URLs
	log.Printf("[PIPELINE] HLS Streaming URL: %s", streamingURL)
	log.Printf("[PIPELINE] Final Poster URL: %s", finalPosterURL)
	log.Printf("[PIPELINE] Final Backdrop URL: %s", finalBackdropURL)

	// Asset storage verification
	mode = p.config.StorageConfig.Mode
	if mode == "dev" {
		log.Printf("[PIPELINE] Storage Mode: development")
		log.Printf("[PIPELINE] Assets Location: /stream/movies/%s/", canonicalFolderName)
	} else {
		log.Printf("[PIPELINE] Storage Mode: production")
		log.Printf("[PIPELINE] Assets Location: B2/videos/movies/%s/", canonicalFolderName)
	}

	log.Printf("[PIPELINE] Movie ID: %v", movieResult.MovieID)
	log.Printf("[PIPELINE] Movie Code: %s", movieResult.Code)
	log.Printf("[PIPELINE] Movie DB: %s", p.dbName)
	log.Printf("[PIPELINE] Movie Collection: movies")

	// Clip generation status
	if clipGenerationFailed {
		log.Printf("[PIPELINE] Clip Generation: FAILED")
	} else {
		log.Printf("[PIPELINE] Clip Generation: SUCCESS")
	}

	// Cleanup status
	log.Printf("[PIPELINE] Readyvideo Cleanup: %s", hlsPath)

	log.Printf("[PIPELINE] ===== END JOB RESULT =====")

	if clipGenerationFailed {
		log.Printf("[PIPELINE] Job %s completed with CLIP FAILURE (movie created successfully)", jobID)
	} else {
		log.Printf("[PIPELINE] Job %s completed successfully", jobID)
	}
	return nil
}

// parseMovieDetailsWithRetry wraps parseMovieDetails with bounded retries for
// transient upstream failures (502/503/504 from the source site bubbling up
// through the parser as HTTP 500 + "Bad Gateway" / "Gateway Timeout" in the
// body, or context/network timeouts on the parser call itself).
//
// Non-transient errors (needs_manual, empty video_url, parser error fields)
// bubble up immediately so we don't waste time on jobs that genuinely cannot
// be resolved.
func (p *Pipeline) parseMovieDetailsWithRetry(job *models.IngestionJob) (*models.ParsedMovieMetadata, string, error) {
	const maxAttempts = 3
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		metadata, localPath, err := p.parseMovieDetails(job)
		if err == nil {
			return metadata, localPath, nil
		}
		lastErr = err
		if _, ok := err.(*needsManualError); ok {
			return nil, "", err
		}
		if !isTransientParserError(err) || attempt == maxAttempts {
			return nil, "", err
		}
		wait := time.Duration(attempt*attempt) * 2 * time.Second
		log.Printf("[PARSER] transient error on /details job_id=%s attempt=%d/%d err=%v — retrying in %s",
			job.ID.Hex(), attempt, maxAttempts, err, wait)
		time.Sleep(wait)
	}
	return nil, "", lastErr
}

// isTransientParserError reports whether the given parseMovieDetails error
// indicates a temporary upstream condition that's worth retrying.
func isTransientParserError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	transientSignals := []string{
		"502", "503", "504",
		"bad gateway", "gateway timeout", "service unavailable",
		"connection reset", "connection refused",
		"timeout", "temporarily", "eof",
	}
	for _, s := range transientSignals {
		if strings.Contains(msg, s) {
			return true
		}
	}
	return false
}

// parseMovieDetails calls the parser service to get movie details
// Returns metadata and local_path if downloaded by parser
func (p *Pipeline) parseMovieDetails(job *models.IngestionJob) (*models.ParsedMovieMetadata, string, error) {
	// Defensive check: job must not be nil
	if job == nil {
		return nil, "", fmt.Errorf("job is nil - cannot parse movie details")
	}

	// Defensive check: source must be provided
	if job.Source == "" {
		return nil, "", fmt.Errorf("job source is empty - cannot parse movie details")
	}

	// Defensive check: source ID or detail URL must be provided
	if job.SourceID == "" && job.DetailURL == "" {
		return nil, "", fmt.Errorf("job source_id and detail_url are both empty - cannot parse movie details")
	}

	jobID := job.ID.Hex()
	log.Printf("[PARSER] parser request started — job_id=%s, source=%s, source_id=%s, url=%s", jobID, job.Source, job.SourceID, job.DetailURL)
	log.Printf("[PIPELINE] parseMovieDetails: job=%s, source=%s, sourceID=%s", jobID, job.Source, job.SourceID)

	parserEndpoint := fmt.Sprintf("%s/details", p.config.ParserURL)
	params := url.Values{}
	params.Set("source", job.Source)
	params.Set("id", job.SourceID)
	params.Set("url", job.DetailURL)
	params.Set("job_id", jobID)
	// For manual jobs, forward admin-entered metadata so the parser can use it
	// as override / fallback instead of returning generic placeholder values.
	if job.Source == "manual" && job.Metadata != nil {
		if job.Metadata.Title != "" {
			params.Set("title", job.Metadata.Title)
		}
		if job.Metadata.Year > 0 {
			params.Set("year", fmt.Sprintf("%d", job.Metadata.Year))
		}
		if job.Metadata.Poster != "" {
			params.Set("poster_url", job.Metadata.Poster)
		}
		if job.Metadata.Backdrop != "" {
			params.Set("backdrop_url", job.Metadata.Backdrop)
		}
	}
	parserURL := parserEndpoint + "?" + params.Encode()

	log.Printf("[PARSER] calling parser /details — job_id=%s, url=%s", jobID, parserURL)

	resp, err := p.httpClient.Get(parserURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to call parser: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		models.ParsedMovieMetadata
		// NEW: Add download response fields for file handoff
		Success              bool     `json:"success"`
		FilePath             string   `json:"file_path"`
		LocalPath            string   `json:"local_path"`
		FileName             string   `json:"file_name"`
		FileSize             int64    `json:"file_size"`
		StreamType           string   `json:"stream_type"`
		DownloadCompleted    bool     `json:"download_completed"`
		DownloadNeeded       bool     `json:"download_needed"`
		VideoFound           bool     `json:"video_found"`
		VideoURL             string   `json:"video_url"` // Best source URL from /details
		SelectedVideoURL     string   `json:"selected_video_url"`
		SelectedQuality      string   `json:"selected_quality"`
		SourceQuality        string   `json:"source_quality"`
		AvailableQualities   []string `json:"available_qualities"`
		ClassifierConfidence float64  `json:"classifier_confidence"`
		ClassifierEvidence   string   `json:"classifier_evidence"`
		SourceID             string   `json:"source_id"`
		DetailURL            string   `json:"detail_url"`
		Type                 string   `json:"type"`
		Poster               string   `json:"poster"`
		Error                string   `json:"error"`
		DownloadError        string   `json:"download_error"`
		ManualReason         string   `json:"manual_reason"`
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read parser response: %w", err)
	}

	if err := json.Unmarshal(body, &result); err != nil {
		if resp.StatusCode != http.StatusOK {
			return nil, "", fmt.Errorf("parser returned status %d: %s", resp.StatusCode, string(body))
		}
		return nil, "", fmt.Errorf("failed to decode parser response: %w", err)
	}

	if result.Error == "needs_manual" {
		reason := strings.TrimSpace(result.ManualReason)
		if reason == "" {
			reason = "video_url_not_found"
		}
		return nil, "", &needsManualError{Reason: fmt.Sprintf("needs manual review: %s", reason)}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("parser returned status %d: %s", resp.StatusCode, string(body))
	}

	if strings.TrimSpace(result.VideoURL) == "" {
		log.Printf("[PIPELINE] Parser returned empty video_url job_id=%s source=%s source_id=%s detail_url=%s error=%s reason=%s",
			jobID, job.Source, job.SourceID, job.DetailURL, result.Error, result.ManualReason)
		return nil, "", fmt.Errorf("download_url empty after parser resolve")
	}

	// Check for parser-level errors
	if result.Error != "" {
		log.Printf("[PIPELINE] Parser returned error: %s", result.Error)
		return nil, "", fmt.Errorf("parser error: %s", result.Error)
	}

	// Check for download-specific errors
	if result.DownloadError != "" {
		log.Printf("[PIPELINE] Parser returned download_error: %s", result.DownloadError)
		return nil, "", fmt.Errorf("download failed: %s", result.DownloadError)
	}

	// Check for video_url_not_found or site_blocked - non-retryable error
	if result.Error == "video_url_not_found" || result.Error == "site_blocked" {
		log.Printf("[PIPELINE] ERROR: Parser returned error=%q - marking as non-retryable", result.Error)
		return nil, "", fmt.Errorf("parser error: %s - non-retryable", result.Error)
	}

	// Check for success=false with error
	if !result.Success && result.Error != "" {
		log.Printf("[PARSER] ERROR: Parser returned success=false with error: %s", result.Error)
		return nil, "", fmt.Errorf("parser error: %s", result.Error)
	}

	// NEW: Log parser response summary
	log.Printf("[PARSER] parser request finished — job_id=%s, status=%d, success=%v, local_path=%s, error=%s, download_error=%s",
		jobID, resp.StatusCode, result.Success, result.LocalPath, result.Error, result.DownloadError)
	log.Printf("[PIPELINE] Parser response: success=%v, local_path=%s, error=%s, download_error=%s",
		result.Success, result.LocalPath, result.Error, result.DownloadError)

	// Log key fields from /details
	log.Printf("[DETAILS] response received — job_id=%s, title=%s, video_url=%s, download_needed=%v, local_path=%s",
		jobID, safeTruncate(result.Title, 60), safeTruncate(result.VideoURL, 80), result.DownloadNeeded, result.LocalPath)

	// NEW: Improved identity check for serials
	if strings.TrimSpace(result.SourceID) != "" && strings.TrimSpace(job.SourceID) != "" && strings.TrimSpace(result.SourceID) != strings.TrimSpace(job.SourceID) {
		// Fix 1: Serial Episode Extraction Mismatch
		// If job.SourceID contains :s and e, and result.SourceID is strictly numeric,
		// dynamically construct the canonical string and compare it.
		if strings.Contains(job.SourceID, ":s") && strings.Contains(job.SourceID, "e") {
			parts := strings.Split(job.SourceID, ":s")
			if len(parts) == 2 {
				parentID := parts[0]
				seasonEp := parts[1]
				seasonParts := strings.Split(seasonEp, "e")
				if len(seasonParts) == 2 {
					seasonStr := seasonParts[0]
					rawFetched := strings.TrimSpace(result.SourceID)

					// Check if rawFetched is strictly numeric
					if numericEpisode, convErr := strconv.Atoi(rawFetched); convErr == nil {
						// Dynamically construct canonical string: {parentID}:s{seasonStr}e{paddedEpisode}
						canonicalFetched := fmt.Sprintf("%s:s%se%02d", parentID, seasonStr, numericEpisode)

						log.Printf("[identity-check] serial_mismatch_fix job_id=%s requested=%s raw_fetched=%s canonical_fetched=%s",
							jobID, job.SourceID, rawFetched, canonicalFetched)

						if canonicalFetched == job.SourceID {
							result.SourceID = job.SourceID
							goto identity_check_ok
						}
					}
				}
			}
		}
		return nil, "", fmt.Errorf("identity mismatch: expected source_id=%s fetched source_id=%s", job.SourceID, result.SourceID)
	}
identity_check_ok:

	if strings.TrimSpace(result.DetailURL) != "" && strings.TrimSpace(job.DetailURL) != "" &&
		normalizeIdentityURL(result.DetailURL) != normalizeIdentityURL(job.DetailURL) {
		return nil, "", fmt.Errorf("identity mismatch: expected detail_url=%s fetched detail_url=%s", job.DetailURL, result.DetailURL)
	}

	// Validate core fields from /details
	if !result.Success {
		return nil, "", fmt.Errorf("parser returned success=false")
	}
	log.Printf("[WORKER] /details returned video_url — job_id=%s, url=%s", jobID, safeTruncate(result.VideoURL, 80))
	log.Printf("[WORKER] /details returned download_needed=%v — job_id=%s", result.DownloadNeeded, jobID)

	// NEW: local_path from /details is OPTIONAL
	// If parser already downloaded (download_needed=false), local_path should be present
	// If download_needed=true, local_path will be empty — worker handles download
	localPath := result.LocalPath
	if localPath == "" {
		localPath = result.FilePath
	}

	// If parser claims it already downloaded (download_needed=false), validate file exists
	if !result.DownloadNeeded && localPath == "" {
		log.Printf("[PIPELINE] ERROR: Parser returned download_needed=false but no local_path")
		return nil, "", fmt.Errorf("parser contract: download_needed=false requires local_path")
	}
	if localPath != "" {
		if _, err := os.Stat(localPath); err != nil {
			log.Printf("[PIPELINE] ERROR: Parser returned local_path but file does not exist: %s", localPath)
			return nil, "", fmt.Errorf("parser returned local_path but file does not exist: %s", localPath)
		}
		fileInfo, _ := os.Stat(localPath)
		if fileInfo != nil && fileInfo.Size() == 0 {
			log.Printf("[PIPELINE] ERROR: Parser returned local_path but file is empty: %s", localPath)
			return nil, "", fmt.Errorf("parser returned local_path but file is empty: %s", localPath)
		}
		log.Printf("[PARSER] local_path received — job_id=%s, path=%s, size=%d bytes",
			jobID, localPath, fileInfo.Size())
	} else {
		// localPath is empty and download_needed=true — worker will download
		log.Printf("[WORKER] /details accepted without local_path — job_id=%s, worker will download via /download", jobID)
	}

	log.Printf("[PIPELINE] parseMovieDetails completed: title=%s, year=%d, video_urls=%d, local_path=%s",
		result.Title, result.Year, len(result.VideoURLs), localPath)
	log.Printf("[STAGE] parser end — title=%s, year=%d, local_path=%s", result.Title, result.Year, localPath)

	p.log(jobID, fmt.Sprintf("Parsed: %s (%d)", result.Title, result.Year), "info")

	// Copy selected video URL into metadata for later use
	if strings.TrimSpace(result.SelectedVideoURL) != "" {
		result.ParsedMovieMetadata.VideoURL = result.SelectedVideoURL
	} else {
		result.ParsedMovieMetadata.VideoURL = result.VideoURL
	}
	result.ParsedMovieMetadata.Quality = firstNonEmptyString(result.SelectedQuality, result.SourceQuality, result.ParsedMovieMetadata.Quality)
	result.ParsedMovieMetadata.ClassifierConfidence = result.ClassifierConfidence
	result.ParsedMovieMetadata.ClassifierEvidence = result.ClassifierEvidence

	// Return metadata and local_path (may be empty if parser will download separately)
	return &result.ParsedMovieMetadata, localPath, nil
}

// callParserDownload calls the parser /download endpoint to start download
// Then polls /progress until download completes
func (p *Pipeline) callParserDownload(jobID, videoURL string) (string, error) {
	return p.callParserDownloadWithReferer(jobID, videoURL, "")
}

func (p *Pipeline) callParserDownloadWithReferer(jobID, videoURL, referer string) (string, error) {
	jobIDCopy := jobID
	log.Printf("[WORKER] download start — job_id=%s, url=%s, referer=%s", jobIDCopy, safeTruncate(videoURL, 80), safeTruncate(referer, 80))

	// Build download endpoint URL
	parserEndpoint := fmt.Sprintf("%s/download", p.config.ParserURL)
	params := url.Values{}
	params.Set("video_url", videoURL)
	params.Set("job_id", jobIDCopy)
	if referer != "" {
		params.Set("referer", referer)
	}
	safeName := regexp.MustCompile(`[^\w\s-]`).ReplaceAllString(jobIDCopy, "")
	safeName = regexp.MustCompile(`[-\s]+`).ReplaceAllString(safeName, "_")
	params.Set("output_name", safeName+".mp4")

	downloadURL := parserEndpoint + "?" + params.Encode()
	log.Printf("[WORKER] calling parser /download — job_id=%s", jobIDCopy)

	// Call /download to start background download
	resp, err := p.httpClient.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to call parser /download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("parser /download returned status %d: %s", resp.StatusCode, string(body))
	}

	var downloadResp struct {
		Success         bool   `json:"success"`
		AlreadyRunning  bool   `json:"already_running"`
		Status          string `json:"status"`
		ProgressPercent int    `json:"progress_percent"`
		Message         string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&downloadResp); err != nil {
		return "", fmt.Errorf("failed to decode /download response: %w", err)
	}

	if !downloadResp.Success {
		return "", fmt.Errorf("parser /download failed: %s", downloadResp.Message)
	}

	// If already running, just poll progress (don't restart)
	if downloadResp.AlreadyRunning {
		log.Printf("[WORKER] existing active download detected — job_id=%s, continuing polling", jobIDCopy)
	}

	log.Printf("[WORKER] download started — job_id=%s, status=%s", jobIDCopy, downloadResp.Status)

	// Poll /progress until download completes
	maxPollCount := 7200 // Max 2 hours (7200 * 1 second)
	pollInterval := 1 * time.Second
	staleSeconds := 0

	for pollCount := 0; pollCount < maxPollCount; pollCount++ {
		time.Sleep(pollInterval)

		// Call /progress endpoint
		progressURL := fmt.Sprintf("%s/progress?job_id=%s", p.config.ParserURL, jobIDCopy)
		progressResp, err := p.httpClient.Get(progressURL)
		if err != nil {
			log.Printf("[WORKER] /progress error — job_id=%s, error=%v, retrying...", jobIDCopy, err)
			continue
		}

		// Read raw body for debug
		rawBody, err := io.ReadAll(progressResp.Body)
		if err != nil {
			progressResp.Body.Close()
			log.Printf("[WORKER] /progress read error — job_id=%s, error=%v", jobIDCopy, err)
			continue
		}
		log.Printf("[WORKER] /progress raw response — job_id=%s, body=%s", jobIDCopy, string(rawBody))

		var progress struct {
			Success         bool    `json:"success"`
			Status          string  `json:"status"`
			ProgressPercent int     `json:"progress_percent"`
			Progress        int     `json:"progress,omitempty"` // legacy fallback
			DownloadedBytes int64   `json:"downloaded_bytes"`
			TotalBytes      int64   `json:"total_bytes"`
			SpeedMBps       float64 `json:"speed_mbps"`
			EtaSeconds      int     `json:"eta_seconds"`
			LocalPath       string  `json:"local_path"`
			FileSize        int64   `json:"file_size"`
			Error           string  `json:"error"`
			Done            bool    `json:"done"`
			PID             int     `json:"pid"`
		}

		if err := json.Unmarshal(rawBody, &progress); err != nil {
			progressResp.Body.Close()
			log.Printf("[WORKER] /progress decode error — job_id=%s, error=%v", jobIDCopy, err)
			continue
		}
		progressResp.Body.Close()

		// Fallback: if progress_percent not set but legacy 'progress' field exists, use it
		if progress.ProgressPercent == 0 && progress.Progress > 0 {
			progress.ProgressPercent = progress.Progress
		}

		// Compute from bytes if still zero
		if progress.ProgressPercent == 0 && progress.DownloadedBytes > 0 && progress.TotalBytes > 0 {
			computed := int((float64(progress.DownloadedBytes) / float64(progress.TotalBytes)) * 100)
			if computed > 0 {
				progress.ProgressPercent = computed
				log.Printf("[WORKER] computed progress from bytes — job_id=%s, percent=%d%%", jobIDCopy, progress.ProgressPercent)
			}
		}

		// NEW: Check for stale download process
		if progress.Status == "downloading" || progress.Status == "starting" {
			// Removed kill -0 local check since the PID belongs to the parser instance,
			// not the worker.
			if progress.ProgressPercent == 0 && progress.PID == 0 {
				staleSeconds++
			} else {
				staleSeconds = 0
			}
		}

		log.Printf("[WORKER] polling — job_id=%s, status=%s, progress_percent=%d%%, downloaded=%d/%d, pid=%d, stale=%ds",
			jobIDCopy, progress.Status, progress.ProgressPercent, progress.DownloadedBytes, progress.TotalBytes, progress.PID, staleSeconds)

		// Check if download completed
		if progress.Status == "completed" {
			localPath := progress.LocalPath
			if localPath == "" {
				return "", fmt.Errorf("parser /progress returned completed but empty local_path")
			}

			// Validate the file exists
			if _, err := os.Stat(localPath); err != nil {
				return "", fmt.Errorf("parser /progress returned local_path but file does not exist: %s", localPath)
			}

			fileSize := int64(0)
			if fileInfo, err := os.Stat(localPath); err == nil {
				fileSize = fileInfo.Size()
			}

			log.Printf("[WORKER] download done — job_id=%s, local_path=%s, size=%d bytes",
				jobIDCopy, localPath, fileSize)
			log.Printf("[WORKER] worker switched to processing — job_id=%s", jobIDCopy)
			return localPath, nil
		}

		// Check if download failed
		if progress.Status == "failed" {
			return "", fmt.Errorf("parser /download failed: %s", progress.Error)
		}

		// Continue polling if still downloading or starting
	}

	return "", fmt.Errorf("download polling timeout after %d seconds", maxPollCount)
}

// startDownloadAndPoll calls /download once then polls /progress
// This is the main download flow for new jobs
func (p *Pipeline) startDownloadAndPoll(ctx context.Context, job *models.IngestionJob, force bool) (string, error) {
	jobID := job.ID.Hex()
	videoURL := job.VideoURL
	if strings.TrimSpace(videoURL) == "" {
		return "", fmt.Errorf("download_url empty after parser resolve")
	}
	// Pass the source page as Referer — freekino/uzmovi/asilmedia CDNs reject
	// segment requests without it. Fall back from the explicit video_page_url
	// (set by /details) to the listing detail_url.
	var referer string
	if job.Metadata != nil {
		referer = strings.TrimSpace(job.Metadata.VideoPageURL)
	}
	if referer == "" {
		referer = strings.TrimSpace(job.DetailURL)
	}

	log.Printf("[WORKER] startDownloadAndPoll — job_id=%s, url=%s, referer=%s", jobID, safeTruncate(videoURL, 60), safeTruncate(referer, 60))

	// Build download endpoint URL
	parserEndpoint := fmt.Sprintf("%s/download", p.config.ParserURL)
	params := url.Values{}
	params.Set("source", job.Source)
	params.Set("video_url", videoURL)
	params.Set("job_id", jobID)
	params.Set("selected_quality", strings.TrimSpace(job.SourceQuality))
	if referer != "" {
		params.Set("referer", referer)
	}
	if force {
		params.Set("force", "1")
	}
	safeName := regexp.MustCompile(`[^\w\s-]`).ReplaceAllString(jobID, "")
	safeName = regexp.MustCompile(`[-\s]+`).ReplaceAllString(safeName, "_")
	params.Set("output_name", safeName+".mp4")

	downloadURL := parserEndpoint + "?" + params.Encode()
	log.Printf("[download-debug] job_id=%s source=%s detail_url=%s video_url=%s download_url=%s selected_quality=%s media_type=%s parser_download_url=%s",
		jobID, job.Source, safeTruncate(referer, 180), safeTruncate(videoURL, 180), safeTruncate(videoURL, 180),
		strings.TrimSpace(job.SourceQuality), normalizeMediaType("", videoURL), safeTruncate(downloadURL, 240))

	// Call /download ONCE - it returns immediately
	log.Printf("[WORKER] /download called once — job_id=%s", jobID)
	resp, err := p.httpClient.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("failed to call parser /download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("parser /download returned status %d: %s", resp.StatusCode, string(body))
	}

	var downloadResp struct {
		Success bool   `json:"success"`
		Status  string `json:"status"` // started, already_running, already_done
		JobID   string `json:"job_id"`
		Message string `json:"message"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&downloadResp); err != nil {
		return "", fmt.Errorf("failed to decode /download response: %w", err)
	}

	log.Printf("[WORKER] /download response — job_id=%s, status=%s", jobID, downloadResp.Status)

	if !downloadResp.Success {
		return "", fmt.Errorf("parser /download failed: %s", downloadResp.Message)
	}

	// Now poll /progress until completion
	// NO more /download calls - only /progress
	return p.pollDownloadProgress(ctx, job, videoURL)
}

// pollDownloadProgress polls the parser's /progress endpoint until the
// download completes, fails, or the watchdog catches a stuck/dead downloader.
//
// Three independent failure modes (fastest one wins):
//
//  1. Stalled-bytes watchdog: if downloaded_bytes does not advance for
//     `noProgressTimeout`, the download is considered hung and the job fails
//     immediately. This is the primary guard — it catches the case where the
//     parser is still responding to /progress with the same byte counter.
//  2. Dead-parser detection: HTTP 404 from /progress means the parser does
//     not know this job (its download process exited or parser was
//     restarted) — fail immediately. Repeated transport/5xx errors fail the
//     job after `consecutiveErrorLimit` attempts.
//  3. Overall hard cap: `maxPollSeconds` as a final safety net — the
//     watchdog should always fire first.
func (p *Pipeline) pollDownloadProgress(ctx context.Context, job *models.IngestionJob, videoURL string) (string, error) {
	jobID := job.ID.Hex()

	const (
		// Hard ceiling: 2 hours. Long-form HLS downloads (films) frequently take
		// 30–60 minutes; the no-progress watchdog catches actual stalls so this
		// is just a final safety net.
		maxPollSeconds = 7200
		pollInterval   = 1 * time.Second
		// Only consider the download stalled if the parser stops advancing
		// downloaded_bytes for 5 minutes. Shorter windows were tripping mid-
		// download on slow segments and recycling the job in a loop.
		noProgressTimeout     = 5 * time.Minute
		consecutiveErrorLimit = 5
	)

	log.Printf("[WORKER] pollDownloadProgress START — job_id=%s, video_url=%s, max=%ds, watchdog=%s, poll=%s",
		jobID, safeTruncate(videoURL, 80), maxPollSeconds, noProgressTimeout, pollInterval)

	// Seed last_progress_at so any external observer (admin UI, stale-job
	// scanner) sees a baseline when polling begins, even before the first
	// byte arrives.
	if err := p.jobRepo.UpdateLastProgressAt(ctx, jobID); err != nil {
		log.Printf("[WORKER] WARN seed last_progress_at — job_id=%s, err=%v", jobID, err)
	}

	var (
		lastBytes         int64 = -1
		lastBytesAt             = time.Now()
		consecutiveErrors       = 0
		noPIDSince        time.Time
		lastProgressAt    time.Time
	)

	for pollCount := 0; pollCount < maxPollSeconds; pollCount++ {
		time.Sleep(pollInterval)

		progressURL := fmt.Sprintf("%s/progress?job_id=%s", p.config.ParserURL, jobID)
		
		reqCtx, reqCancel := context.WithTimeout(ctx, 15*time.Second)
		req, reqErr := http.NewRequestWithContext(reqCtx, "GET", progressURL, nil)
		var progressResp *http.Response
		var err error
		if reqErr == nil {
			progressResp, err = p.httpClient.Do(req)
		} else {
			err = reqErr
		}

		if err != nil {
			reqCancel()
			consecutiveErrors++
			log.Printf("[WORKER] /progress transport error — job_id=%s, err=%v, consecutive=%d/%d",
				jobID, err, consecutiveErrors, consecutiveErrorLimit)
			if consecutiveErrors >= consecutiveErrorLimit {
				return "", fmt.Errorf("parser /progress unreachable %d polls in a row (last err: %w) — assuming downloader/parser dead",
					consecutiveErrors, err)
			}
			if dur := time.Since(lastBytesAt); dur >= noProgressTimeout {
				return "", fmt.Errorf("download watchdog: no byte progress for %s while parser unreachable — failing fast",
					dur.Truncate(time.Second))
			}
			continue
		}

		// IMPORTANT: do not cancel reqCtx until the body is fully read.
		// http.Client.Do returns once the response headers arrive; the body is
		// streamed lazily and stays bound to the request context. Cancelling
		// here (before io.ReadAll) aborts the in-flight body read with
		// "context canceled" — which under parser load surfaced as
		// "parser /progress body unreadable N polls in a row: context canceled"
		// and failed otherwise-healthy downloads.
		rawBody, readErr := io.ReadAll(progressResp.Body)
		statusCode := progressResp.StatusCode
		progressResp.Body.Close()
		reqCancel()

		if readErr != nil {
			consecutiveErrors++
			log.Printf("[WORKER] /progress read error — job_id=%s, err=%v, consecutive=%d/%d",
				jobID, readErr, consecutiveErrors, consecutiveErrorLimit)
			if consecutiveErrors >= consecutiveErrorLimit {
				return "", fmt.Errorf("parser /progress body unreadable %d polls in a row: %w", consecutiveErrors, readErr)
			}
			continue
		}

		// 404 means the parser dropped this job — its download process
		// exited or the parser was restarted. No point in polling further.
		if statusCode == http.StatusNotFound {
			return "", fmt.Errorf("parser /progress returned 404 — downloader process exited or job unknown to parser (body=%s)",
				safeTruncate(string(rawBody), 200))
		}
		if statusCode >= 500 {
			consecutiveErrors++
			log.Printf("[WORKER] /progress 5xx — job_id=%s, status=%d, body=%s, consecutive=%d/%d",
				jobID, statusCode, safeTruncate(string(rawBody), 200), consecutiveErrors, consecutiveErrorLimit)
			if consecutiveErrors >= consecutiveErrorLimit {
				return "", fmt.Errorf("parser /progress returned %d on %d polls in a row — parser likely dead",
					statusCode, consecutiveErrors)
			}
			continue
		}

		var progress struct {
			Success         bool    `json:"success"`
			Status          string  `json:"status"` // starting, downloading, completed, failed
			ProgressPercent int     `json:"progress_percent"`
			Progress        int     `json:"progress,omitempty"` // fallback for legacy
			DownloadedBytes int64   `json:"downloaded_bytes"`
			TotalBytes      int64   `json:"total_bytes"`
			SpeedMBps       float64 `json:"speed_mbps"`
			EtaSeconds      int     `json:"eta_seconds"`
			LocalPath       string  `json:"local_path"`
			FileSize        int64   `json:"file_size"`
			Error           string  `json:"error"`
			PID             int     `json:"pid"`
			Source          string  `json:"source"`
			DetailURL       string  `json:"detail_url"`
			VideoURL        string  `json:"video_url"`
			DownloadURL     string  `json:"download_url"`
			SelectedQuality string  `json:"selected_quality"`
			MediaType       string  `json:"media_type"`
			CommandString   string  `json:"downloader_command_string"`
			StdoutTail      string  `json:"stdout_tail"`
			StderrTail      string  `json:"stderr_tail"`
			TempOutputPath  string  `json:"temp_output_path"`
			LastProgressAt  float64 `json:"last_progress_at"`
			ExitCode        *int    `json:"exit_code"`
		}

		if err := json.Unmarshal(rawBody, &progress); err != nil {
			consecutiveErrors++
			log.Printf("[WORKER] /progress decode error — job_id=%s, err=%v, body=%s, consecutive=%d/%d",
				jobID, err, safeTruncate(string(rawBody), 200), consecutiveErrors, consecutiveErrorLimit)
			if consecutiveErrors >= consecutiveErrorLimit {
				return "", fmt.Errorf("parser /progress emitted invalid JSON %d polls in a row: %w", consecutiveErrors, err)
			}
			continue
		}
		consecutiveErrors = 0

		if progress.ProgressPercent == 0 && progress.Progress > 0 {
			progress.ProgressPercent = progress.Progress
		}

		progressPercent := progress.ProgressPercent
		if progressPercent < 0 {
			progressPercent = 0
		}
		if progressPercent == 0 && progress.DownloadedBytes > 0 && progress.TotalBytes > 0 {
			computed := int((float64(progress.DownloadedBytes) / float64(progress.TotalBytes)) * 100)
			if computed > 0 {
				progressPercent = computed
			}
		}

		if progress.LastProgressAt > 0 {
			sec := int64(progress.LastProgressAt)
			nsec := int64((progress.LastProgressAt - float64(sec)) * 1e9)
			lastProgressAt = time.Unix(sec, nsec)
		}
		stalledFor := time.Since(lastBytesAt)
		log.Printf("[download-debug] poll=%d job_id=%s source=%s detail_url=%s video_url=%s download_url=%s selected_quality=%s media_type=%s pid=%d status=%s pct=%d%% bytes=%d/%d file_size=%d temp_output_path=%s last_progress_at=%s stalled_for=%s command=%s exit_code=%s stdout=%s stderr=%s",
			pollCount, jobID, firstNonEmptyString(progress.Source, job.Source), safeTruncate(firstNonEmptyString(progress.DetailURL, job.DetailURL), 180),
			safeTruncate(firstNonEmptyString(progress.VideoURL, videoURL), 180), safeTruncate(firstNonEmptyString(progress.DownloadURL, videoURL), 180),
			firstNonEmptyString(progress.SelectedQuality, job.SourceQuality), firstNonEmptyString(progress.MediaType, normalizeMediaType("", videoURL)),
			progress.PID, progress.Status, progressPercent, progress.DownloadedBytes, progress.TotalBytes, progress.FileSize,
			safeTruncate(progress.TempOutputPath, 180), formatOptionalTime(lastProgressAt), stalledFor.Truncate(time.Second),
			safeTruncate(progress.CommandString, 400), formatExitCode(progress.ExitCode), safeTruncate(progress.StdoutTail, 400), safeTruncate(progress.StderrTail, 600))

		if progress.Status == "downloading" || progress.Status == "starting" {
			if progressPercent == 0 && progress.DownloadedBytes == 0 && progress.PID == 0 {
				if noPIDSince.IsZero() {
					noPIDSince = time.Now()
				}
				
				// Reset noPID timer if we see recent activity from the parser thread
				// (e.g. it updated last_progress_at while resolving or initializing)
				if !lastProgressAt.IsZero() && time.Since(lastProgressAt) < 30*time.Second {
					noPIDSince = time.Now()
				}

				if time.Since(noPIDSince) >= 90*time.Second {
					reason := strings.TrimSpace(progress.Error)
					if reason == "" {
						reason = strings.TrimSpace(progress.StderrTail)
					}
					if reason == "" {
						reason = "downloader process did not start"
					}
					return "", fmt.Errorf("downloader process did not start: %s", safeTruncate(reason, 1000))
				}
			} else {
				noPIDSince = time.Time{}
			}
			msg := fmt.Sprintf("Downloading: %d%%", progressPercent)
			if progress.SpeedMBps > 0 {
				msg += fmt.Sprintf(" (%.1f MB/s)", progress.SpeedMBps)
			}
			if progress.EtaSeconds > 0 {
				msg += fmt.Sprintf(", ETA %ds", progress.EtaSeconds)
			}
			p.jobRepo.UpdateDownloadProgress(ctx, jobID, progressPercent,
				progress.DownloadedBytes, progress.TotalBytes, progress.SpeedMBps, progress.EtaSeconds, msg)
		}

		// Watchdog: if bytes advanced, reset timer; otherwise check stall.
		if progress.DownloadedBytes > lastBytes {
			lastBytes = progress.DownloadedBytes
			lastBytesAt = time.Now()
			lastProgressAt = lastBytesAt
		} else if progress.Status == "downloading" || progress.Status == "starting" {
			// Fix 2: Fast-Fail for 0% Stuck Downloads
			// If download is stuck at 0 bytes for 60 seconds, fail immediately.
			// This typically indicates a 403 Forbidden, expired token, or blocked CDN.
			if progress.DownloadedBytes == 0 && stalledFor >= 60*time.Second {
				return "", fmt.Errorf("fast-fail: download stuck at 0 bytes for 60s (possible 403 Forbidden)")
			}

			if stalledFor >= noProgressTimeout {
				return "", fmt.Errorf("download watchdog: no byte progress for %s (status=%s, bytes=%d/%d) — downloader appears stuck or process dead",
					stalledFor.Truncate(time.Second), progress.Status, progress.DownloadedBytes, progress.TotalBytes)
			}
		}

		if progress.Status == "completed" {
			localPath := progress.LocalPath
			if localPath == "" {
				return "", fmt.Errorf("parser /progress returned completed but empty local_path")
			}
			if _, err := os.Stat(localPath); err != nil {
				return "", fmt.Errorf("parser /progress returned local_path but file does not exist: %s", localPath)
			}

			fileSize := int64(0)
			if fileInfo, err := os.Stat(localPath); err == nil {
				fileSize = fileInfo.Size()
			}
			log.Printf("[WORKER] pollDownloadProgress END (completed) — job_id=%s, local_path=%s, size=%d bytes, polls=%d",
				jobID, localPath, fileSize, pollCount+1)
			return localPath, nil
		}

		if progress.Status == "failed" {
			reason := strings.TrimSpace(progress.Error)
			if reason == "" {
				reason = strings.TrimSpace(progress.StderrTail)
			}
			if reason == "" && progress.ExitCode != nil {
				reason = fmt.Sprintf("downloader exited with code %d", *progress.ExitCode)
			}
			if reason == "" {
				reason = "unknown parser download failure"
			}
			return "", fmt.Errorf("parser /download reported failed: %s", safeTruncate(reason, 1200))
		}
		if progress.ExitCode != nil && *progress.ExitCode != 0 {
			reason := strings.TrimSpace(progress.StderrTail)
			if reason == "" {
				reason = strings.TrimSpace(progress.Error)
			}
			return "", fmt.Errorf("downloader exited with code %d: %s", *progress.ExitCode, safeTruncate(reason, 1200))
		}
	}

	return "", fmt.Errorf("download polling hard timeout after %d seconds (watchdog should have fired earlier — investigate)", maxPollSeconds)
}

// downloadVideo - PARSER NOW HANDLES DOWNLOADING
// This function is kept for backward compatibility but should NOT be called
// Parser now downloads the video and provides local_path in the job
func (p *Pipeline) downloadVideo(job *models.IngestionJob, metadata *models.ParsedMovieMetadata) (string, error) {
	// DEFENSIVE: Check if local path is already provided by parser
	// Parser should have already downloaded the file
	if job.LocalPath != "" {
		log.Printf("[PIPELINE] Using pre-downloaded file from parser: %s", job.LocalPath)
		// Verify the file exists
		if _, err := os.Stat(job.LocalPath); err != nil {
			return "", fmt.Errorf("parser-provided file does not exist: %w", err)
		}
		return job.LocalPath, nil
	}

	// ERROR: If no local path, parser failed to download
	// Worker should NOT attempt to download - this is now parser's responsibility
	log.Printf("[PIPELINE] ERROR: No local_path provided by parser!")
	log.Printf("[PIPELINE] Worker should NOT download - parser must download first")
	return "", fmt.Errorf("parser did not download file - worker cannot proceed. Parser must download first.")
}

// getInputResolution returns the width and height of the input video using ffprobe
func (p *Pipeline) getInputResolution(inputPath string) (int, int, error) {
	// Run ffprobe to get video dimensions
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=width,height",
		"-of", "csv=s=,:p=0",
		inputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, 0, fmt.Errorf("ffprobe failed to get resolution: %w", err)
	}

	// Parse output (format: "width,height")
	result := strings.TrimSpace(string(output))
	parts := strings.Split(result, ",")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("unexpected ffprobe output format: %s", result)
	}

	width, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse width: %w", err)
	}

	height, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse height: %w", err)
	}

	return width, height, nil
}

// getVideoDurationMs returns the video duration in milliseconds using ffprobe
func (p *Pipeline) getVideoDurationMs(inputPath string) (int64, error) {
	// Run ffprobe to get duration in seconds
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffprobe failed to get duration: %w", err)
	}

	result := strings.TrimSpace(string(output))
	durationSec, err := strconv.ParseFloat(result, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration: %w", err)
	}

	// Convert to milliseconds
	durationMs := int64(durationSec * 1000)
	log.Printf("[WORKER] ffprobe duration=%d ms (%.2f seconds)", durationMs, durationSec)

	return durationMs, nil
}

// getVideoFPS returns the video frame rate using ffprobe
func (p *Pipeline) getVideoFPS(inputPath string) (float64, error) {
	cmd := exec.Command("ffprobe",
		"-v", "error",
		"-select_streams", "v:0",
		"-show_entries", "stream=r_frame_rate",
		"-of", "default=noprint_wrappers=1:nokey=1",
		inputPath,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return 24.0, fmt.Errorf("ffprobe failed to get fps: %w", err)
	}

	result := strings.TrimSpace(string(output))
	// Parse frame rate as fraction (e.g., "30000/1001" = 29.97)
	if strings.Contains("/", result) {
		parts := strings.Split(result, "/")
		if len(parts) == 2 {
			num, err1 := strconv.ParseFloat(parts[0], 64)
			den, err2 := strconv.ParseFloat(parts[1], 64)
			if err1 == nil && err2 == nil && den > 0 {
				fps := num / den
				log.Printf("[WORKER] ffprobe fps=%.2f (from %s)", fps, result)
				return fps, nil
			}
		}
	}

	// Try direct float parse
	fps, err := strconv.ParseFloat(result, 64)
	if err != nil {
		return 24.0, fmt.Errorf("failed to parse fps: %w", err)
	}

	log.Printf("[WORKER] ffprobe fps=%.2f", fps)
	return fps, nil
}

// runFFmpegWithProgress runs FFmpeg with progress tracking
// Returns duration in milliseconds and any error
func (p *Pipeline) runFFmpegWithProgress(
	inputPath string,
	ffmpegArgs []string,
	jobID string,
	progressCallback func(processedMs int64, totalMs int64),
) (int64, error) {
	// First get total duration
	totalMs, err := p.getVideoDurationMs(inputPath)
	if err != nil {
		log.Printf("[WORKER] WARNING: Could not get video duration: %v, running without progress", err)
		// Run without progress tracking
		cmd := exec.Command("ffmpeg", ffmpegArgs...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return 0, fmt.Errorf("ffmpeg failed: %w, output: %s", err, string(output))
		}
		return 0, nil
	}

	// Add -progress pipe:1 to ffmpeg args
	ffmpegArgs = append(ffmpegArgs, "-progress", "pipe:1")

	cmd := exec.Command("ffmpeg", ffmpegArgs...)

	// Create pipe to read progress output
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return 0, fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read progress output
	buffer := make([]byte, 4096)
	var outTimeMs int64 = 0

	for {
		n, err := stdout.Read(buffer)
		if err != nil {
			break
		}
		if n > 0 {
			// Parse progress output for out_time_ms
			output := string(buffer[:n])
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "out_time_ms=") {
					value := strings.TrimPrefix(line, "out_time_ms=")
					if t, err := strconv.ParseInt(value, 10, 64); err == nil {
						outTimeMs = t / 1000 // Convert to ms
						if progressCallback != nil && totalMs > 0 {
							progressCallback(outTimeMs, totalMs)
						}
					}
				}
			}
		}
	}

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		return 0, fmt.Errorf("ffmpeg failed: %w", err)
	}

	return totalMs, nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// validateMediaFile validates that the downloaded file is actually a media file
// This prevents ffmpeg from failing with "moov atom not found" or "invalid data"
func (p *Pipeline) validateMediaFile(filePath string, urlType string) error {
	// Minimum file size check (a real video should be at least 100KB)
	const minVideoSize = 100 * 1024 // 100KB

	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("cannot stat file: %w", err)
	}

	fileSize := fileInfo.Size()
	log.Printf("[PIPELINE] Validating media file: %s, size=%d bytes", filePath, fileSize)

	if fileSize < minVideoSize {
		return fmt.Errorf("file too small (%d bytes) - likely not a complete video (minimum: %d bytes)", fileSize, minVideoSize)
	}

	// Read first 512 bytes to check file signature
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("cannot open file: %w", err)
	}
	defer file.Close()

	buffer := make([]byte, 512)
	n, err := file.Read(buffer)
	if err != nil && err != io.EOF {
		return fmt.Errorf("cannot read file header: %w", err)
	}
	buffer = buffer[:n]

	// Check for HTML signature in first bytes
	htmlSignatures := []string{"<!DOCTYPE", "<html", "<HTML", "<head", "<HEAD"}
	for _, sig := range htmlSignatures {
		if len(buffer) >= len(sig) && string(buffer[:len(sig)]) == sig {
			// Read a bit more to confirm it's HTML
			if strings.Contains(string(buffer), "</html>") || strings.Contains(string(buffer), "</body>") || strings.Contains(string(buffer), "<!DOCTYPE") {
				return fmt.Errorf("file contains HTML - downloaded content is a web page, not a video")
			}
		}
	}

	// Check for JSON error response
	if len(buffer) > 0 && buffer[0] == '{' {
		return fmt.Errorf("file appears to be JSON - downloaded content is an API error response, not a video")
	}

	// Try to use ffprobe to validate the file
	if err := p.validateWithFFprobe(filePath); err != nil {
		log.Printf("[PIPELINE] FFprobe validation failed: %v", err)
		return fmt.Errorf("ffprobe validation failed: %w", err)
	}

	log.Printf("[PIPELINE] Media file validation passed")
	return nil
}

// validateWithFFprobe uses ffprobe to verify the file is a valid media file
func (p *Pipeline) validateWithFFprobe(filePath string) error {
	// Check if ffprobe is available
	cmd := exec.Command("which", "ffprobe")
	if err := cmd.Run(); err != nil {
		log.Printf("[PIPELINE] ffprobe not found, skipping ffprobe validation")
		return nil
	}

	// Run ffprobe to get media info
	ffprobeCmd := exec.Command("ffprobe",
		"-v", "error",
		"-show_entries", "stream=codec_type,codec_name",
		"-of", "json",
		filePath,
	)

	output, err := ffprobeCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("ffprobe failed: %w (output: %s)", err, string(output))
	}

	log.Printf("[PIPELINE] FFprobe output: %s", string(output))

	// Check if output contains video or audio streams
	if !strings.Contains(string(output), "video") && !strings.Contains(string(output), "audio") {
		return fmt.Errorf("ffprobe output does not contain video or audio streams")
	}

	return nil
}

// processDirectUploadJob processes a direct upload job - downloads from B2 temp, processes, uploads to final
func (p *Pipeline) processDirectUploadJob(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[DIRECT_UPLOAD] Starting processing for job %s", jobID)

	metadata := job.Metadata
	if metadata == nil {
		return fmt.Errorf("job metadata is nil")
	}

	title := metadata.Title
	if title == "" {
		title = "untitled"
	}
	log.Printf("[DIRECT_UPLOAD] Processing: title=%s, temp_url=%s", title, job.TempFileURL)

	// Update status to downloading
	if err := p.updateStatus(jobID, models.IngestionStatusDownloading, 10); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Step 1: Download temp file from B2 to local temp storage
	localTempPath, err := p.downloadDirectUploadTempFile(job)
	if err != nil {
		errMsg := fmt.Sprintf("failed to download temp file: %v", err)
		log.Printf("[DIRECT_UPLOAD] ERROR: %s", errMsg)
		if fErr := p.failJobWithStatus(jobID, models.IngestionStatusDownloadFailed, errMsg); fErr != nil {
			log.Printf("[DIRECT_UPLOAD] Failed to mark job as failed: %v", fErr)
		}
		return fmt.Errorf("%s", errMsg)
	}
	log.Printf("[DIRECT_UPLOAD] Downloaded temp file to: %s", localTempPath)

	// Disk preflight: the transcode produces an intermediate base video plus
	// several HLS renditions, all on local disk before upload. For a multi-GB
	// source this can easily need 3-4× the source size. Bail out early with a
	// clear message rather than filling the disk halfway through a long encode.
	if srcInfo, statErr := os.Stat(localTempPath); statErr == nil {
		needed := srcInfo.Size() * 3
		if free, dErr := freeDiskBytes(p.config.TempDir); dErr == nil && free < needed {
			errMsg := fmt.Sprintf("not enough disk space to process: need ~%.1f GB free, have %.1f GB (source %.1f GB)",
				float64(needed)/(1<<30), float64(free)/(1<<30), float64(srcInfo.Size())/(1<<30))
			log.Printf("[DIRECT_UPLOAD] ERROR: %s", errMsg)
			p.cleanupFile(localTempPath)
			if fErr := p.failJobWithStatus(jobID, models.IngestionStatusFailed, errMsg); fErr != nil {
				log.Printf("[DIRECT_UPLOAD] Failed to mark job as failed: %v", fErr)
			}
			return fmt.Errorf("%s", errMsg)
		}
	}

	// Update status to processing
	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 30); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Step 2: Process video (add logo, generate HLS)
	canonicalFolderName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), createMovieSlug(title))
	hlsDir, masterPath, err := p.processVideo(job, localTempPath, canonicalFolderName)
	if err != nil {
		errMsg := fmt.Sprintf("video processing failed: %v", err)
		log.Printf("[DIRECT_UPLOAD] ERROR: %s", errMsg)
		if fErr := p.failJobWithStatus(jobID, models.IngestionStatusFailed, errMsg); fErr != nil {
			log.Printf("[DIRECT_UPLOAD] Failed to mark job as failed: %v", fErr)
		}
		return fmt.Errorf("%s", errMsg)
	}
	log.Printf("[DIRECT_UPLOAD] Processed video: hls_dir=%s, master=%s", hlsDir, masterPath)

	// NOTE: Do NOT delete localTempPath here — the pipeline may still fail
	// (upload or DB save), and retries need the source file. Cleanup runs
	// after createMovieInDatabase succeeds below.

	// Update status to uploading
	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 60); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Step 3: Upload HLS output to B2 final path
	folderName := canonicalFolderName
	streamingURL, err := p.uploadAdaptiveHLSFiles(job, hlsDir, folderName)
	if err != nil {
		errMsg := fmt.Sprintf("failed to upload processed files: %v", err)
		log.Printf("[DIRECT_UPLOAD] ERROR: %s", errMsg)
		if fErr := p.failJobWithStatus(jobID, models.IngestionStatusFailed, errMsg); fErr != nil {
			log.Printf("[DIRECT_UPLOAD] Failed to mark job as failed: %v", fErr)
		}
		return fmt.Errorf("%s", errMsg)
	}
	if streamingURL == "" {
		return fmt.Errorf("no master playlist found")
	}
	job.MasterPlaylistURL = streamingURL
	log.Printf("[DIRECT_UPLOAD] Uploaded HLS to: %s", streamingURL)

	// Update job with output paths
	if err := p.jobRepo.UpdateOutputPath(ctx, jobID, streamingURL); err != nil {
		log.Printf("[DIRECT_UPLOAD] Failed to update output path: %v", err)
	}
	// Also mark source file as deleted
	if err := p.jobRepo.MarkSourceFileDeleted(ctx, jobID, "movies/"+folderName); err != nil {
		log.Printf("[DIRECT_UPLOAD] Failed to mark source deleted: %v", err)
	}

	// Update status to creating movie
	if err := p.updateStatus(jobID, models.IngestionStatusCreatingMovie, 80); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	// Step 4: Create movie in MongoDB
	movieResult, err := p.createMovieInDatabase(job, metadata, streamingURL)
	if err != nil {
		errMsg := fmt.Sprintf("failed to create movie in DB: %v", err)
		log.Printf("[DIRECT_UPLOAD] ERROR: %s", errMsg)
		if fErr := p.failJobWithStatus(jobID, models.IngestionStatusFailed, errMsg); fErr != nil {
			log.Printf("[DIRECT_UPLOAD] Failed to mark job as failed: %v", err)
		}
		return fmt.Errorf("%s", errMsg)
	}

	// Upload + DB save both succeeded — now safe to delete the local temp source.
	log.Printf("[DIRECT_UPLOAD] deleting temp input after full success: %s", localTempPath)
	p.cleanupFile(localTempPath)

	// Step 5: Generate clips and keep movie unpublished until clip policy is satisfied.
	clipGenerationFailed := false
	if movieResult != nil {
		log.Printf("[DIRECT_UPLOAD] Generating clips for movie: %s", title)
		if clipErr := p.generateClips(ctx, canonicalFolderName, movieResult.Code, movieResult, masterPath, hlsDir); clipErr != nil {
			clipGenerationFailed = true
			log.Printf("[DIRECT_UPLOAD] WARNING: Clip generation failed: %v", clipErr)
		} else {
			log.Printf("[DIRECT_UPLOAD] Clips generated successfully")
		}
		if movieResult.MovieID != nil {
			clipsStatus := "completed"
			pipelineStatus := "ready"
			pipelineComplete := true
			if clipGenerationFailed {
				clipsStatus = "failed"
				pipelineStatus = "waiting_for_clips"
				pipelineComplete = !p.config.RequireClipsBeforePublish
			}
			if err := p.updateMoviePipelineState(ctx, movieResult.MovieID, pipelineStatus, clipsStatus, pipelineComplete); err != nil {
				log.Printf("[DIRECT_UPLOAD] WARNING: failed to update movie pipeline state: %v", err)
			}
		}
	}
	if clipGenerationFailed && p.config.RequireClipsBeforePublish {
		return fmt.Errorf("clip generation failed and REQUIRE_CLIPS_BEFORE_PUBLISH=true")
	}

	// Step 6: Cleanup B2 temp file
	if err := p.cleanupTempFile(job); err != nil {
		return fmt.Errorf("cleanup temp file from B2: %w", err)
	}
	log.Printf("[DIRECT_UPLOAD] Cleaned up B2 temp file")

	// Update status to completed only after clips/cleanup are done.
	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		log.Printf("[DIRECT_UPLOAD] Failed to mark job completed: %v", err)
	}

	log.Printf("[DIRECT_UPLOAD] Job %s completed successfully", jobID)
	return nil
}

// downloadDirectUploadTempFile downloads the temp file from B2 to local storage
func (p *Pipeline) downloadDirectUploadTempFile(job *models.IngestionJob) (string, error) {
	if job.TempFileURL == "" {
		return "", fmt.Errorf("temp file URL is empty")
	}

	// Extract the B2 key from the URL
	remotePath := extractB2KeyFromURL(job.TempFileURL)
	if remotePath == "" {
		// If extraction fails, try using the DetailURL as fallback
		remotePath = extractB2KeyFromURL(job.DetailURL)
	}
	if remotePath == "" {
		remotePath = job.TempFileURL
	}

	// Generate local temp filename
	ext := ".mp4"
	if idx := strings.LastIndex(remotePath, "."); idx != -1 {
		ext = remotePath[idx:]
	}
	localFilename := fmt.Sprintf("%d_temp%s", time.Now().UnixNano(), ext)
	localPath := filepath.Join(p.config.TempDir, localFilename)

	log.Printf("[DIRECT_UPLOAD] Downloading from B2: remote=%s, local=%s", remotePath, localPath)

	// Download from B2
	if err := p.storage.Download(remotePath, localPath); err != nil {
		return "", fmt.Errorf("failed to download from B2: %w", err)
	}

	// Verify file exists
	if fileInfo, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("downloaded file not found: %w", err)
	} else {
		log.Printf("[DIRECT_UPLOAD] Downloaded file: size=%d", fileInfo.Size())
	}
	return localPath, nil
}

// cleanupTempFile removes the temp file from B2
func (p *Pipeline) cleanupTempFile(job *models.IngestionJob) error {
	// Prefer TempFileKey if available (set from frontend ingestion job creation)
	tempKey := job.TempFileKey

	// Fallback: extract from URL if key not provided
	if tempKey == "" && job.TempFileURL != "" {
		tempKey = extractB2KeyFromURL(job.TempFileURL)
	}

	// Last fallback: use DetailURL
	if tempKey == "" && job.DetailURL != "" {
		tempKey = extractB2KeyFromURL(job.DetailURL)
	}

	if tempKey == "" {
		log.Printf("[DIRECT_UPLOAD] No temp file key to cleanup")
		return nil
	}

	log.Printf("[DIRECT_UPLOAD] Deleting temp file: %s", tempKey)

	if err := p.storage.Delete(tempKey); err != nil {
		return fmt.Errorf("failed to delete temp file from B2: %w", err)
	}

	log.Printf("[DIRECT_UPLOAD] Deleted temp file: %s", tempKey)
	return nil
}

// extractB2KeyFromURL extracts the B2 key from a CDN URL
func extractB2KeyFromURL(cdnURL string) string {
	// Expected format: https://cdn.example.com/file/filmorauznet/temp/movies/filename.mp4
	// or: https://cdn.example.com/file/filmorauznet/movies/...
	parts := strings.Split(cdnURL, "/file/filmorauznet/")
	if len(parts) < 2 {
		return ""
	}
	return parts[1]
}

// processVideo processes the video with FFmpeg (watermark removal + adaptive HLS)
// This function now generates multi-bitrate adaptive HLS with a master playlist
func (p *Pipeline) processVideo(job *models.IngestionJob, inputPath string, canonicalFolderName string) (hlsDir, processedMasterPath string, err error) {
	if job == nil {
		return "", "", fmt.Errorf("job is nil - cannot process video")
	}

	jobID := job.ID.Hex()

	metadata := job.Metadata
	if metadata == nil {
		log.Printf("[PIPELINE] WARNING: job metadata is nil, using fallback values")
	}

	title := "video"
	year := 0
	if metadata != nil {
		title = metadata.Title
		year = metadata.Year
	}
	if title == "" && job.Title != "" {
		title = job.Title
	}
	log.Printf("[PIPELINE] Using title=%s, year=%d for processing", title, year)

	if inputPath == "" {
		return "", "", fmt.Errorf("input path is empty - cannot process video")
	}
	if _, statErr := os.Stat(inputPath); os.IsNotExist(statErr) {
		return "", "", fmt.Errorf("input file does not exist: %s", inputPath)
	}

	log.Printf("[CHECKPOINT] raw_input_path: %s", inputPath)
	log.Printf("[PIPELINE] processVideo: job=%s", jobID)

	if err = p.validateMediaFile(inputPath, "pre-processing-check"); err != nil {
		return "", "", fmt.Errorf("pre-processing validation failed: %w", err)
	}

	folderName := canonicalFolderName
	log.Printf("[WORKER] Using canonical folder name: %s", folderName)

	baseDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get working directory: %w", err)
	}

	readyVideoDir := filepath.Join(baseDir, "readyvideo")
	if err = os.MkdirAll(readyVideoDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create readyvideo dir: %w", err)
	}

	outputDir := filepath.Join(readyVideoDir, folderName)
	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create output dir: %w", err)
	}
	log.Printf("[WORKER] readyDir=%s", outputDir)

	p.log(jobID, fmt.Sprintf("%s+adaptive-hls", folderName), "info")
	if err = p.updateStatus(jobID, models.IngestionStatusProcessing, 35); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to processing: %v", err)
	}
	log.Printf("[STAGE] hls_processing start — raw_input: %s", inputPath)

	const defaultCutSeconds = 10

	var lastProgress int
	progressCallback := func(status models.IngestionStatus, progress int) {
		if progress-lastProgress >= 5 || progress == 100 {
			lastProgress = progress
			message := fmt.Sprintf("Processing video... %d%%", progress)
			log.Printf("[WORKER] Adaptive HLS progress: %d%%", progress)
			p.updateStatus(jobID, status, progress)
			p.updateMessage(jobID, message)
			p.log(jobID, message, "info")
		}
	}

	// processAdaptiveHLS returns the HLS master playlist path AND the processed master video path.
	// The processed master (cut + logo) is NOT deleted inside processAdaptiveHLS;
	// it stays alive so clip generation can use it, then the whole readyvideo dir is cleaned up.
	var masterPlaylistPath string
	var generatedQualities []string
	masterPlaylistPath, processedMasterPath, generatedQualities, err = p.processAdaptiveHLS(jobID, inputPath, outputDir, folderName, B2VideoRoot(job), defaultCutSeconds, progressCallback, p.config.MaxRenditionConcurrent, p.config.SegmentUploadWorkers, p.config.SegmentUploadRetries)
	if err != nil {
		return "", "", fmt.Errorf("adaptive HLS processing failed: %w", err)
	}
	log.Printf("[CHECKPOINT] processed_master_path: %s", processedMasterPath)
	log.Printf("[CHECKPOINT] hls_master_playlist: %s", masterPlaylistPath)
	log.Printf("[PIPELINE] Source resolution from ffprobe, generated qualities: %v", generatedQualities)

	files, readErr := os.ReadDir(outputDir)
	if readErr != nil {
		return "", "", fmt.Errorf("failed to read output dir: %w", readErr)
	}
	if len(files) == 0 {
		return "", "", fmt.Errorf("no output files generated")
	}

	log.Printf("[PIPELINE] processVideo completed: %d files/dirs in %s", len(files), outputDir)
	log.Printf("[STAGE] hls_processing end — master: %s", masterPlaylistPath)
	p.log(jobID, fmt.Sprintf("%s+processed", folderName), "info")

	// Update quality info in database
	sourceResolution := ""
	if inputWidth, inputHeight, err := p.getInputResolution(inputPath); err == nil {
		sourceResolution = fmt.Sprintf("%dx%d", inputWidth, inputHeight)
	}
	job.SourceResolution = sourceResolution
	job.GeneratedQualities = append([]string(nil), generatedQualities...)
	job.AvailableQualities = append([]string(nil), generatedQualities...)
	if err := p.jobRepo.UpdateQualityInfo(context.Background(), jobID, job.SourceQuality, sourceResolution, generatedQualities, masterPlaylistPath); err != nil {
		log.Printf("[PIPELINE] WARNING: failed to update quality info: %v", err)
	}

	// Generate hover-preview thumbnails from the processed master. Non-fatal:
	// if ffmpeg fails or the source isn't seekable the player falls back to
	// the static poster.
	thumbDir := filepath.Join(outputDir, "thumbnails")
	if processedMasterPath != "" {
		if _, err := generateThumbnails(processedMasterPath, thumbDir, ThumbnailIntervalSeconds); err != nil {
			log.Printf("[PIPELINE] WARNING: thumbnail generation failed: %v", err)
		}
	}

	return outputDir, processedMasterPath, nil
}

// uploadProcessedFiles handles finalization based on MODE
// - development: copy files to worker/uploads/<slug>/
// - production: upload to B2/CDN
// inputPath is the original parser-downloaded file for cleanup
//
// NOTE: This function now handles adaptive HLS output which includes:
// - master.m3u8 (master playlist)
// - 360p/index.m3u8 + segments/
// - 480p/index.m3u8 + segments/
// - 720p/index.m3u8 + segments/
// - 1080p/index.m3u8 + segments/
//
// Returns: (streamingURL, finalUploadsPath, error)
// finalUploadsPath is the local path where base_video.mp4 can be found for clip generation
func (p *Pipeline) uploadProcessedFiles(job *models.IngestionJob, hlsDir string, inputPath string, canonicalFolderName string) (string, string, error) {
	// Defensive check: job must not be nil
	if job == nil {
		return "", "", fmt.Errorf("job is nil - cannot upload files")
	}

	jobID := job.ID.Hex()

	// Defensive check: metadata must not be nil
	metadata := job.Metadata
	if metadata == nil {
		log.Printf("[PIPELINE] WARNING: job metadata is nil for upload, using fallback values")
	}

	// Defensive check: hlsDir must not be empty
	if hlsDir == "" {
		return "", "", fmt.Errorf("hls directory is empty - nothing to upload")
	}

	// Defensive check: hlsDir must exist
	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		return "", "", fmt.Errorf("hls directory does not exist: %s", hlsDir)
	}

	log.Printf("[PIPELINE] uploadProcessedFiles: job=%s, hlsDir=%s", jobID, hlsDir)

	// CRITICAL: Use the canonical folder name passed from processJobWithRecovery
	// This ensures ALL assets (HLS, poster, backdrop) are in the SAME folder
	folderName := canonicalFolderName

	log.Printf("[PIPELINE] Finalizing adaptive HLS files for movie using canonical folder: %s", folderName)

	// Check MODE from storage config
	mode := p.config.StorageConfig.Mode
	log.Printf("[WORKER] MODE=%s", mode)

	var streamingURL string
	var finalUploadsPath string

	if mode == "prod" || mode == "production" {
		// === PRODUCTION MODE: Upload to B2/CDN ===
		log.Printf("[WORKER] MODE=production")
		log.Printf("[WORKER] Generated adaptive HLS in: %s", hlsDir)
		log.Printf("[WORKER] Uploading adaptive HLS folder from readyvideo/%s to B2", folderName)

		// Preserve base_video.mp4 for clip generation before deleting readyvideo
		baseVideoPath := filepath.Join(hlsDir, "base_video.mp4")
		if _, err := os.Stat(baseVideoPath); err == nil {
			// Copy base_video.mp4 to a permanent location
			clipsBaseDir := filepath.Join(p.config.StorageConfig.LocalPath, "movies", "clips_base", folderName)
			if err := os.MkdirAll(clipsBaseDir, 0755); err != nil {
				log.Printf("[WORKER] WARNING: Failed to create clips_base directory: %v", err)
			} else {
				destPath := filepath.Join(clipsBaseDir, "base_video.mp4")
				if err := copyFile(baseVideoPath, destPath); err != nil {
					log.Printf("[WORKER] WARNING: Failed to copy base_video.mp4 for clips: %v", err)
				} else {
					finalUploadsPath = clipsBaseDir
					log.Printf("[WORKER] Preserved base_video.mp4 for clip generation: %s", destPath)
				}
			}
		}

		// Use the adaptive HLS upload function which handles recursive directory structure
		var err error
		streamingURL, err = p.uploadAdaptiveHLSFiles(job, hlsDir, folderName)
		if err != nil {
			return "", "", fmt.Errorf("failed to upload adaptive HLS files: %w", err)
		}

		if streamingURL == "" {
			return "", "", fmt.Errorf("no master playlist found")
		}

		log.Printf("[WORKER] Final CDN master playlist URL: %s", streamingURL)
		log.Printf("[PIPELINE] uploadProcessedFiles completed: streamingURL=%s", streamingURL)

	} else {
		// === DEVELOPMENT MODE: Copy to worker/uploads/movies/<slug>/ ===
		log.Printf("[WORKER] MODE=development")

		// Preserve base_video.mp4 for clip generation before copying readyvideo
		baseVideoPath := filepath.Join(hlsDir, "base_video.mp4")
		if _, err := os.Stat(baseVideoPath); err == nil {
			clipsBaseDir := filepath.Join(p.config.StorageConfig.LocalPath, "movies", "clips_base", folderName)
			if err := os.MkdirAll(clipsBaseDir, 0755); err != nil {
				log.Printf("[WORKER] WARNING: Failed to create clips_base directory: %v", err)
			} else {
				destPath := filepath.Join(clipsBaseDir, "base_video.mp4")
				if err := copyFile(baseVideoPath, destPath); err != nil {
					log.Printf("[WORKER] WARNING: Failed to copy base_video.mp4 for clips: %v", err)
				} else {
					finalUploadsPath = clipsBaseDir
					log.Printf("[WORKER] Preserved base_video.mp4 for clip generation: %s", destPath)
				}
			}
		} else {
			log.Printf("[WORKER] WARNING: base_video.mp4 not found at %s for clip generation", baseVideoPath)
		}

		// Use the adaptive HLS upload function which handles recursive directory structure
		var err error
		streamingURL, err = p.uploadAdaptiveHLSFiles(job, hlsDir, folderName)
		if err != nil {
			return "", "", fmt.Errorf("failed to copy adaptive HLS files: %w", err)
		}

		if streamingURL == "" {
			return "", "", fmt.Errorf("no master playlist found")
		}

		// Set finalUploadsPath for development mode (where files are copied)
		// NOTE: clips_base takes priority for clip generation
		if finalUploadsPath == "" {
			finalUploadsPath = filepath.Join(p.config.StorageConfig.LocalPath, "movies", folderName)
		}

		log.Printf("[WORKER] Development master playlist URL: %s", streamingURL)
		log.Printf("[PIPELINE] uploadProcessedFiles completed: streamingURL=%s, finalUploadsPath=%s", streamingURL, finalUploadsPath)
	}

	// NOTE: Source file cleanup is deliberately NOT performed here. It runs
	// in the main pipeline (see cleanupFile call after createMovieInDatabase)
	// so the source survives a DB-save failure and a subsequent retry.
	_ = inputPath

	return streamingURL, finalUploadsPath, nil
}

// createMovieInDatabase creates the movie entry in the main database
// Uses the same logic as createMovieInDatabaseWithEnrichment for direct upload
func (p *Pipeline) createMovieInDatabase(job *models.IngestionJob, metadata *models.ParsedMovieMetadata, streamingURL string) (*MovieCreationResult, error) {
	jobID := job.ID.Hex()

	if metadata == nil {
		return nil, fmt.Errorf("metadata is nil")
	}

	// Check if movie collection is available
	if p.movieCol == nil {
		log.Printf("[DIRECT_UPLOAD] ERROR: Movie collection not available, cannot create movie")
		return nil, fmt.Errorf("movie collection not initialized")
	}

	// Generate sequential code for the movie
	code, err := p.getNextContentCode(context.Background())
	if err != nil {
		log.Printf("[DIRECT_UPLOAD] ERROR: Failed to generate sequential code: %v", err)
		return nil, fmt.Errorf("failed to generate movie code: %w", err)
	}
	log.Printf("[DIRECT_UPLOAD] Generated sequential movie code: %s", code)

	// Build the movie document
	displayTitle := cleanMovieTitle(metadata.Title)
	if metadata.Title == "" {
		displayTitle = "Untitled"
	}

	displaySlug := createMovieSlug(displayTitle)
	normalizedDisplayTitle := normalizeDuplicateTitle(displayTitle)
	displayDescription := metadata.Description
	if displayDescription == "" {
		displayDescription = "No description available"
	}
	availableQualities := append([]string(nil), job.GeneratedQualities...)
	if len(availableQualities) == 0 {
		availableQualities = append([]string(nil), job.AvailableQualities...)
	}
	maxQuality := highestAvailableQuality(availableQualities)
	defaultQuality := maxQuality
	if maxQuality == "" {
		maxQuality = job.Quality
	}

	movieDoc := bson.M{
		"code":                code,
		"slug":                displaySlug,
		"title":               displayTitle,
		"normalized_title":    normalizedDisplayTitle,
		"original_title":      metadata.Title,
		"description":         displayDescription,
		"year":                metadata.Year,
		"genre":               metadata.Genres,
		"country":             metadata.Country,
		"duration":            metadata.Duration,
		"poster_url":          metadata.Poster,
		"backdrop_url":        metadata.Backdrop,
		"video_url":           streamingURL,
		"master_playlist_url": streamingURL,
		"available_qualities": availableQualities,
		"generated_qualities": availableQualities,
		"default_quality":     defaultQuality,
		"source_resolution":   job.SourceResolution,
		"source_type":         "direct_hls",
		"source": bson.M{
			"provider":   job.Source,
			"source_url": job.DetailURL,
			"source_id":  job.SourceID,
		},
		"quality":            maxQuality,
		"is_premium":         job.IsPremium,
		"status":             "processing",
		"pipeline_status":    "processing",
		"pipeline_complete":  false,
		"clips_status":       "pending",
		"selected_video_url": job.VideoURL,
		"created_at":         time.Now(),
		"updated_at":         time.Now(),
	}

	existingDoc, duplicateReason, err := p.findExistingMovieForImport(
		context.Background(),
		displaySlug,
		job.Source,
		job.DetailURL,
		job.SourceID,
		normalizedDisplayTitle,
		metadata.Title,
		metadata.Title,
		"",
		metadata.Year,
	)
	if err != nil {
		return nil, fmt.Errorf("failed duplicate check: %w", err)
	}
	if existingDoc != nil {
		log.Printf("[DIRECT_UPLOAD] DUPLICATE BLOCKED: %s - existing movie reused", duplicateReason)
		return p.movieCreationResultFromDoc(existingDoc, displayTitle, true), nil
	}

	// Insert the movie
	result, err := p.movieCol.InsertOne(context.Background(), movieDoc)
	if err != nil {
		log.Printf("[DIRECT_UPLOAD] ERROR: Failed to insert movie: %v", err)
		return nil, fmt.Errorf("failed to create movie: %w", err)
	}

	log.Printf("[DIRECT_UPLOAD] Movie created: id=%v, code=%s, title=%s", result.InsertedID, code, displayTitle)
	p.log(jobID, fmt.Sprintf("Movie created: %s (code: %s)", displayTitle, code), "info")

	return &MovieCreationResult{
		MovieID:      result.InsertedID,
		Code:         code,
		Slug:         displaySlug,
		DisplayTitle: displayTitle,
	}, nil
}

// createMovieInDatabaseWithEnrichment creates movie with enriched metadata
// MovieCreationResult holds the result of movie creation
type MovieCreationResult struct {
	MovieID      interface{}
	Code         string
	Slug         string
	DisplayTitle string
	IsDuplicate  bool
}

func (p *Pipeline) updateMoviePipelineState(ctx context.Context, movieID interface{}, pipelineStatus, clipsStatus string, pipelineComplete bool) error {
	if p.movieCol == nil || movieID == nil {
		return nil
	}
	filterID := movieID
	if raw, ok := movieID.(string); ok {
		if oid, err := primitive.ObjectIDFromHex(raw); err == nil {
			filterID = oid
		}
	}
	filter := bson.M{"_id": filterID}
	_, err := p.movieCol.UpdateOne(ctx, filter, bson.M{
		"$set": bson.M{
			"pipeline_status":   pipelineStatus,
			"pipeline_complete": pipelineComplete,
			"clips_status":      clipsStatus,
			"status":            pipelineStatus,
			"updated_at":        time.Now(),
		},
	})
	return err
}

// Uses upsert to handle duplicate slugs - updates existing or inserts new
func (p *Pipeline) createMovieInDatabaseWithEnrichment(
	ctx context.Context,
	job *models.IngestionJob,
	enrichedMetadata *models.EnrichedMetadata,
	streamingURL string,
	posterURL string,
	originalPosterURL string,
	posterGenerated bool,
	backdropURL string,
	originalBackdropURL string,
	backdropGenerated bool,
	metadataSource string,
) (*MovieCreationResult, error) {
	jobID := job.ID.Hex()

	if enrichedMetadata == nil {
		return nil, fmt.Errorf("enriched metadata is nil")
	}

	// Check if movie collection is available
	if p.movieCol == nil {
		log.Printf("[PIPELINE] ERROR: Movie collection not available, cannot create movie")
		return nil, fmt.Errorf("movie collection not initialized")
	}

	// Generate slug from title
	_ = createMovieSlug(enrichedMetadata.Title) // Deprecated: using displaySlug instead

	// Generate sequential code for the movie using atomic counter
	code, err := p.getNextContentCode(ctx)
	if err != nil {
		log.Printf("[PIPELINE] ERROR: Failed to generate sequential code: %v", err)
		return nil, fmt.Errorf("failed to generate movie code: %w", err)
	}
	log.Printf("[PIPELINE] Generated sequential movie code: %s", code)

	// Build the movie document
	// CRITICAL: User-facing fields should be Uzbek. If title_uz is available, use it as primary title.
	displayTitle := cleanMovieTitle(enrichedMetadata.Title)
	if enrichedMetadata.TitleUz != "" {
		displayTitle = cleanMovieTitle(enrichedMetadata.TitleUz)
		log.Printf("[PIPELINE] Using Uzbek title as display title: %s", displayTitle)
	}
	log.Printf("[PIPELINE] Title after cleaning: %q", displayTitle)

	// Generate slug from display title (Uzbek) for consistency
	displaySlug := createMovieSlug(displayTitle)
	normalizedDisplayTitle := normalizeDuplicateTitle(displayTitle)

	// For description, prefer Uzbek if available
	displayDescription := enrichedMetadata.Description
	if enrichedMetadata.DescriptionUz != "" {
		displayDescription = enrichedMetadata.DescriptionUz
		log.Printf("[PIPELINE] Using Uzbek description (length: %d)", len(displayDescription))
	}
	availableQualities := append([]string(nil), job.GeneratedQualities...)
	if len(availableQualities) == 0 {
		availableQualities = append([]string(nil), job.AvailableQualities...)
	}
	maxQuality := highestAvailableQuality(availableQualities)
	defaultQuality := maxQuality
	if maxQuality == "" {
		maxQuality = enrichedMetadata.Quality
	}

	movieDoc := bson.M{
		"code":                  code,
		"slug":                  displaySlug,  // Use Uzbek slug for consistency
		"title":                 displayTitle, // Uzbek title for user-facing display
		"normalized_title":      normalizedDisplayTitle,
		"original_title":        enrichedMetadata.OriginalTitle, // Keep original English title
		"original_description":  enrichedMetadata.Description,   // Keep original English description
		"description":           displayDescription,             // Uzbek description for user-facing display
		"year":                  enrichedMetadata.Year,
		"genre":                 enrichedMetadata.Genres,                        // English genres for search/filter
		"genres_uz":             enrichedMetadata.GenresUz,                      // Uzbek genres for display
		"country":               strings.Join(enrichedMetadata.Countries, ", "), // English countries for storage (joined string)
		"countries_uz":          enrichedMetadata.CountriesUz,                   // Uzbek countries as array for frontend display
		"duration":              enrichedMetadata.Duration,
		"quality":               maxQuality,
		"translation":           enrichedMetadata.Translation,
		"poster_url":            posterURL,           // Generated/localized poster
		"original_poster_url":   originalPosterURL,   // Original TMDB poster
		"backdrop_url":          backdropURL,         // Generated/localized backdrop
		"original_backdrop_url": originalBackdropURL, // Original TMDB backdrop
		"video_url":             streamingURL,
		"master_playlist_url":   streamingURL,
		"available_qualities":   availableQualities,
		"generated_qualities":   availableQualities,
		"default_quality":       defaultQuality,
		"source_resolution":     job.SourceResolution,
		"source_type":           "direct_hls",
		"source": bson.M{
			"provider":   job.Source,
			"source_url": job.DetailURL,
			"source_id":  job.SourceID,
		},
		"metadata_source":    metadataSource, // "tmdb" or "parser"
		"poster_generated":   posterGenerated,
		"backdrop_generated": backdropGenerated,
		"views":              0,
		"rating_avg":         0,
		"rating_count":       0,
		"website_url":        calculateWebsiteURL(displaySlug),
		"pipeline_status":    "processing",
		"pipeline_complete":  false,
		"clips_status":       "pending",
		"selected_video_url": job.VideoURL,
		"created_at":         time.Now(),
		"updated_at":         time.Now(),
	}

	// Log localization status
	log.Printf("[PIPELINE] LOCALIZATION: displayTitle=%s, originalTitle=%s, displayDescription_len=%d, genres_uz=%v, countries_uz=%v",
		displayTitle, enrichedMetadata.OriginalTitle, len(displayDescription), enrichedMetadata.GenresUz, enrichedMetadata.CountriesUz)

	// EXPLICIT LOGGING: Database and collection names
	log.Printf("[PIPELINE] ===== MOVIE CREATION START =====")
	log.Printf("[PIPELINE] Database: db=%s", p.dbName)
	log.Printf("[PIPELINE] Collection: collection=movies")
	log.Printf("[PIPELINE] Target movie: displayTitle=%s, year=%d, slug=%s, code=%s", displayTitle, enrichedMetadata.Year, displaySlug, code)
	log.Printf("[PIPELINE] Poster: posterURL=%s, originalPosterURL=%s, posterGenerated=%v", posterURL, originalPosterURL, posterGenerated)
	log.Printf("[PIPELINE] Backdrop: backdropURL=%s, originalBackdropURL=%s, backdropGenerated=%v", backdropURL, originalBackdropURL, backdropGenerated)
	log.Printf("[PIPELINE] Source: provider=%s, sourceUrl=%s, sourceID=%s", job.Source, job.DetailURL, job.SourceID)
	log.Printf("[PIPELINE] Metadata source: %s", metadataSource)
	log.Printf("[PIPELINE] Original title: %s", enrichedMetadata.OriginalTitle)
	log.Printf("[PIPELINE] Genres: %v", enrichedMetadata.Genres)
	log.Printf("[PIPELINE] Countries: %v", enrichedMetadata.Countries)
	log.Printf("[PIPELINE] Duration: %d minutes", enrichedMetadata.Duration)

	existingDoc, duplicateReason, err := p.findExistingMovieForImport(
		ctx,
		displaySlug,
		job.Source,
		job.DetailURL,
		job.SourceID,
		normalizedDisplayTitle,
		enrichedMetadata.OriginalTitle,
		enrichedMetadata.Title,
		enrichedMetadata.TitleUz,
		enrichedMetadata.Year,
	)
	if err != nil {
		return nil, fmt.Errorf("failed duplicate check: %w", err)
	}
	if existingDoc != nil {
		log.Printf("[PIPELINE] DUPLICATE BLOCKED: %s - reusing existing movie", duplicateReason)
		return p.movieCreationResultFromDoc(existingDoc, displayTitle, true), nil
	}

	filter := bson.M{"slug": displaySlug}
	update := bson.M{
		"$set": movieDoc,
		"$setOnInsert": bson.M{
			"approval_status": "pending",
			"is_published":    false,
		},
	}

	result, err := p.movieCol.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	if err != nil {
		log.Printf("[PIPELINE] ERROR: DB operation failed: %v", err)
		return nil, fmt.Errorf("failed to upsert movie: %w", err)
	}

	// EXPLICIT LOGGING: Full result details
	log.Printf("[PIPELINE] ===== DB OPERATION RESULT =====")
	log.Printf("[PIPELINE] Operation: UpdateOne with upsert=true")
	log.Printf("[PIPELINE] Filter: {\"slug\": \"%s\"}", displaySlug)
	log.Printf("[PIPELINE] matchedCount: %d (movies matched by filter)", result.MatchedCount)
	log.Printf("[PIPELINE] modifiedCount: %d (movies modified)", result.ModifiedCount)
	log.Printf("[PIPELINE] upsertedCount: %d (movies inserted)", result.UpsertedCount)
	log.Printf("[PIPELINE] upsertedID: %v", result.UpsertedID)

	// For description, prefer Uzbek if available
	displayDescription = enrichedMetadata.Description
	if enrichedMetadata.DescriptionUz != "" {
		displayDescription = enrichedMetadata.DescriptionUz
		log.Printf("[PIPELINE] Using Uzbek description (length: %d)", len(displayDescription))
	}

	// Determine the final movie ID and action taken
	var finalMovieID interface{}
	var movieAction string

	if result.UpsertedCount > 0 {
		// New movie was inserted
		finalMovieID = result.UpsertedID
		movieAction = "created"
		log.Printf("[PIPELINE] ACTION: New movie CREATED with ID=%v", finalMovieID)
	} else if result.ModifiedCount > 0 {
		// Existing movie was updated
		// We need to fetch the existing ID
		var existingMovieDoc bson.M
		if err := p.movieCol.FindOne(ctx, bson.M{"slug": displaySlug}).Decode(&existingMovieDoc); err == nil {
			if id, ok := existingMovieDoc["_id"].(primitive.ObjectID); ok {
				finalMovieID = id
			} else {
				finalMovieID = existingMovieDoc["_id"]
			}
		} else {
			finalMovieID = "unknown"
		}
		movieAction = "updated"
		log.Printf("[PIPELINE] ACTION: Existing movie UPDATED with ID=%v", finalMovieID)
	} else if result.MatchedCount > 0 {
		// Movie matched but no changes needed
		var existingMovieDoc bson.M
		if err := p.movieCol.FindOne(ctx, bson.M{"slug": displaySlug}).Decode(&existingMovieDoc); err == nil {
			if id, ok := existingMovieDoc["_id"].(primitive.ObjectID); ok {
				finalMovieID = id
			} else {
				finalMovieID = existingMovieDoc["_id"]
			}
		} else {
			finalMovieID = "unknown"
		}
		movieAction = "matched_no_change"
		log.Printf("[PIPELINE] ACTION: Movie MATCHED (no changes) with ID=%v", finalMovieID)
	} else {
		// Should not happen with upsert=true
		movieAction = "unknown"
		log.Printf("[PIPELINE] WARNING: No match and no upsert - unexpected state")
	}

	log.Printf("[PIPELINE] ===== MOVIE %s =====", strings.ToUpper(movieAction))
	log.Printf("[PIPELINE] Final movie ID: %v", finalMovieID)
	log.Printf("[PIPELINE] Final database: %s", p.dbName)
	log.Printf("[PIPELINE] Final collection: movies")
	log.Printf("[PIPELINE] Final poster URL: %s", posterURL)
	log.Printf("[PIPELINE] Final posterGenerated: %v", posterGenerated)
	log.Printf("[PIPELINE] ===== MOVIE CREATION END =====")

	// Log to job history
	p.log(jobID, fmt.Sprintf("Movie %s: %s (id=%v, slug=%s, posterGenerated=%v)", movieAction, displayTitle, finalMovieID, displaySlug, posterGenerated), "info")

	// Return the movie ID with additional data
	return &MovieCreationResult{
		MovieID:      finalMovieID,
		Code:         code,
		Slug:         displaySlug,
		DisplayTitle: displayTitle,
	}, nil
}

func (p *Pipeline) movieCreationResultFromDoc(doc bson.M, fallbackTitle string, isDuplicate bool) *MovieCreationResult {
	var movieID interface{} = doc["_id"]
	if oid, ok := doc["_id"].(primitive.ObjectID); ok {
		movieID = oid
	}

	displayTitle := fallbackTitle
	if title, ok := doc["title"].(string); ok && strings.TrimSpace(title) != "" {
		displayTitle = title
	}

	return &MovieCreationResult{
		MovieID:      movieID,
		Code:         fmt.Sprintf("%v", doc["code"]),
		Slug:         fmt.Sprintf("%v", doc["slug"]),
		DisplayTitle: displayTitle,
		IsDuplicate:  isDuplicate,
	}
}

func (p *Pipeline) findExistingMovieForImport(
	ctx context.Context,
	displaySlug string,
	sourceProvider string,
	sourceURL string,
	sourceID string,
	normalizedTitle string,
	originalTitle string,
	title string,
	titleUz string,
	year int,
) (bson.M, string, error) {
	checks := []struct {
		rule   string
		filter bson.M
	}{
		{
			rule: "source+source_url",
			filter: bson.M{
				"source.provider":   sourceProvider,
				"source.source_url": sourceURL,
			},
		},
		{
			rule: "external_id",
			filter: bson.M{
				"source.provider":  sourceProvider,
				"source.source_id": sourceID,
			},
		},
		{
			rule:   "slug",
			filter: bson.M{"slug": displaySlug},
		},
	}

	for _, check := range checks {
		if hasEmptyDuplicateFilterValue(check.filter) {
			continue
		}
		var existing bson.M
		err := p.movieCol.FindOne(ctx, check.filter).Decode(&existing)
		if err == nil {
			return existing, check.rule, nil
		}
		if err != mongo.ErrNoDocuments {
			return nil, "", err
		}
	}

	if normalizedTitle == "" || year <= 0 {
		return nil, "", nil
	}

	cursor, err := p.movieCol.Find(ctx, bson.M{"year": year})
	if err != nil {
		return nil, "", err
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, "", err
	}

	candidates := []string{normalizedTitle, normalizeDuplicateTitle(title), normalizeDuplicateTitle(titleUz), normalizeDuplicateTitle(originalTitle)}
	for _, doc := range docs {
		for _, field := range []string{"normalized_title", "title", "original_title", "title_uz"} {
			if value, ok := doc[field].(string); ok {
				docTitle := normalizeDuplicateTitle(value)
				for _, candidate := range candidates {
					if candidate != "" && docTitle == candidate {
						return doc, "normalized_title+year", nil
					}
				}
			}
		}
	}

	return nil, "", nil
}

func hasEmptyDuplicateFilterValue(filter bson.M) bool {
	for _, value := range filter {
		if str, ok := value.(string); ok && strings.TrimSpace(str) == "" {
			return true
		}
	}
	return false
}

// updateStatus updates job status and progress
func (p *Pipeline) updateStatus(jobID string, status models.IngestionStatus, progress int) error {
	if err := p.jobRepo.UpdateStatus(context.Background(), jobID, status, progress); err != nil {
		return err
	}
	p.log(jobID, fmt.Sprintf("Status: %s (%d%%)", status, progress), "info")
	return nil
}

// updateMessage updates the job message field
func (p *Pipeline) updateMessage(jobID string, message string) error {
	if err := p.jobRepo.UpdateMessage(context.Background(), jobID, message); err != nil {
		log.Printf("[WORKER] WARNING: Failed to update message: %v", err)
		return err
	}
	log.Printf("[WORKER] Updated job message: %s", message)
	return nil
}

// log adds a log entry to the job
func (p *Pipeline) log(jobID, message, level string) {
	p.jobRepo.AddLog(context.Background(), jobID, message, level)
	log.Printf("[%s] %s", level, message)
}

// unwrapEmbedURL extracts the real video URL from player-embed wrappers
// that some sources return (kinochilar, uzmedia, kinolar). Examples:
//
//	/player/playerjs.html?file=https://.../master.m3u8
//	/embed.html?file=http://.../*.mp4
//	/player/playerjs.html?file=URL1,URL2,URL3&qualities=480P,720P,1080P
//
// For the multi-URL kinolar variant we pick the LAST entry — that's
// usually the highest quality (1080p) for these sources. The simple
// single-URL case returns the embedded URL verbatim. URLs that don't
// match the wrapper pattern are returned unchanged.
func unwrapEmbedURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	path := strings.ToLower(u.Path)
	isEmbed := strings.Contains(path, "playerjs.html") ||
		strings.HasSuffix(path, "/embed.html") ||
		path == "/embed.html"
	if !isEmbed {
		return raw
	}
	file := strings.TrimSpace(u.Query().Get("file"))
	if file == "" {
		return raw
	}
	// kinolar packs multiple qualities into one file= value separated
	// by commas. Pick the last (highest quality) entry.
	if strings.Contains(file, ",") {
		parts := strings.Split(file, ",")
		for i := len(parts) - 1; i >= 0; i-- {
			candidate := strings.TrimSpace(parts[i])
			if strings.HasPrefix(candidate, "http://") || strings.HasPrefix(candidate, "https://") {
				return candidate
			}
		}
		return raw
	}
	if strings.HasPrefix(file, "http://") || strings.HasPrefix(file, "https://") {
		return file
	}
	return raw
}

func validateDownloadURL(raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	if strings.TrimSpace(u.Host) == "" {
		return fmt.Errorf("missing host")
	}
	return nil
}

func (p *Pipeline) failDownloadJob(jobID string, err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	p.log(jobID, msg, "error")
	if failErr := p.jobRepo.SetError(context.Background(), jobID, msg); failErr != nil {
		log.Printf("[WORKER] failDownloadJob set error failed job_id=%s err=%v", jobID, failErr)
	}
	if failErr := p.jobRepo.UpdateStatus(context.Background(), jobID, models.IngestionStatusDownloadFailed, 0); failErr != nil {
		log.Printf("[WORKER] failDownloadJob secondary failure job_id=%s err=%v", jobID, failErr)
	}
	return err
}

// failJob marks a job as failed
func (p *Pipeline) failJob(jobID, errorMsg string) {
	p.jobRepo.SetError(context.Background(), jobID, errorMsg)
	p.jobRepo.IncrementRetry(context.Background(), jobID)
	log.Printf("Job %s failed: %s", jobID, errorMsg)
}

func (p *Pipeline) markJobNeedsManual(jobID, reason string) error {
	if reason == "" {
		reason = "needs manual review"
	}
	if err := p.jobRepo.MarkNeedsManual(context.Background(), jobID, reason); err != nil {
		return err
	}
	p.log(jobID, fmt.Sprintf("Needs manual review: %s", reason), "warning")
	log.Printf("[PIPELINE] needs_manual job=%s reason=%s", jobID, reason)
	return nil
}

// failJobWithStatus marks a job as failed with a specific status
// This provides clearer status for admin UI
func (p *Pipeline) failJobWithStatus(jobID string, status models.IngestionStatus, errorMsg string) error {
	ctx := context.Background()

	// Set the error message
	if err := p.jobRepo.SetError(ctx, jobID, errorMsg); err != nil {
		log.Printf("Failed to set error for job %s: %v", jobID, err)
		return err
	}

	// Update status to the specified status
	if err := p.jobRepo.UpdateStatus(ctx, jobID, status, 0); err != nil {
		log.Printf("Failed to update status for job %s: %v", jobID, err)
		return err
	}

	// Increment retry count
	if err := p.jobRepo.IncrementRetry(ctx, jobID); err != nil {
		log.Printf("Failed to increment retry for job %s: %v", jobID, err)
		return err
	}

	log.Printf("Job %s marked as %s: %s", jobID, status, errorMsg)
	return nil
}

// freeDiskBytes returns the available bytes on the filesystem backing dir.
func freeDiskBytes(dir string) (int64, error) {
	if dir == "" {
		dir = "."
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, err
	}
	return int64(st.Bavail) * int64(st.Bsize), nil
}

// cleanupFile removes a temporary file
// getNextContentCode gets the next global sequential content code shared by
// both movies and series. It scans both collections, finds the highest numeric
// code, and skips any collision already present in either collection.
func (p *Pipeline) getNextContentCode(ctx context.Context) (string, error) {
	if p.config.DB == nil {
		return "", fmt.Errorf("database not initialized")
	}

	highestSeq := int64(0)
	for _, collectionName := range []string{"movies", "series"} {
		cursor, err := p.config.DB.Collection(collectionName).Find(ctx, bson.M{})
		if err != nil {
			return "", fmt.Errorf("failed to query %s collection: %w", collectionName, err)
		}

		for cursor.Next(ctx) {
			var doc struct {
				Code string `bson:"code"`
			}
			if err := cursor.Decode(&doc); err != nil {
				continue
			}

			code := strings.TrimSpace(doc.Code)
			if code == "" {
				continue
			}

			var seq int64
			_, err := fmt.Sscanf(code, "%d", &seq)
			if err != nil || seq <= 0 {
				seq = parseNumericCode(code)
			}
			if seq > highestSeq {
				highestSeq = seq
			}
		}
		if err := cursor.Err(); err != nil {
			cursor.Close(ctx)
			return "", fmt.Errorf("cursor error while scanning %s codes: %w", collectionName, err)
		}
		cursor.Close(ctx)
	}

	for nextSeq := highestSeq + 1; nextSeq <= 999999; nextSeq++ {
		formattedCode := formatContentCode(nextSeq)
		inMovies, err := p.config.DB.Collection("movies").CountDocuments(ctx, bson.M{"code": formattedCode})
		if err != nil {
			return "", fmt.Errorf("check movie code exists %s: %w", formattedCode, err)
		}
		if inMovies > 0 {
			continue
		}

		inSeries, err := p.config.DB.Collection("series").CountDocuments(ctx, bson.M{"code": formattedCode})
		if err != nil {
			return "", fmt.Errorf("check series code exists %s: %w", formattedCode, err)
		}
		if inSeries > 0 {
			continue
		}

		log.Printf("[PIPELINE] Content code generated: highest_existing=%d next=%d formatted=%s", highestSeq, nextSeq, formattedCode)
		return formattedCode, nil
	}

	return "", fmt.Errorf("content code limit exceeded: %d", 999999)
}

func formatContentCode(seq int64) string {
	switch {
	case seq <= 9999:
		return fmt.Sprintf("%04d", seq)
	case seq <= 99999:
		return fmt.Sprintf("%05d", seq)
	default:
		return fmt.Sprintf("%06d", seq)
	}
}

// parseNumericCode parses a zero-padded code string to its numeric value
func parseNumericCode(code string) int64 {
	if code == "" {
		return 0
	}

	// Remove leading zeros
	var result int64
	for _, c := range code {
		if c == '0' && result == 0 {
			continue
		}
		if c >= '0' && c <= '9' {
			result = result*10 + int64(c-'0')
		}
	}
	return result
}

func highestAvailableQuality(qualities []string) string {
	best := ""
	bestHeight := -1
	for _, quality := range qualities {
		q := strings.TrimSpace(strings.TrimSuffix(quality, "p"))
		height, err := strconv.Atoi(q)
		if err != nil {
			continue
		}
		if height > bestHeight {
			bestHeight = height
			best = fmt.Sprintf("%dp", height)
		}
	}
	return best
}

// sendTelegramNotification sends a Telegram notification after successful movie creation
// This is idempotent - if the job already has telegram_notified=true, it will be skipped
func (p *Pipeline) sendTelegramNotification(ctx context.Context, jobID string, job *models.IngestionJob, metadata *models.EnrichedMetadata, streamingURL, posterURL string, movieResult *MovieCreationResult) error {
	log.Printf("[TELEGRAM] ===== STARTING TELEGRAM NOTIFICATION =====")
	log.Printf("[TELEGRAM] Job ID: %s", jobID)

	// Idempotency check - skip if already notified
	if job.TelegramNotified {
		log.Printf("[TELEGRAM] Job already notified, skipping (idempotency check)")
		return nil
	}

	// Check if backend URL is configured
	if p.config.BackendURL == "" {
		log.Printf("[TELEGRAM] Backend URL not configured, skipping Telegram notification")
		log.Printf("[TELEGRAM] Set BACKEND_BASE_URL in worker config to enable notifications")
		return nil
	}

	// Update status to sending_notification
	if err := p.updateStatus(jobID, models.IngestionStatusSendingNotification, 95); err != nil {
		log.Printf("[TELEGRAM] WARNING: Failed to update status to sending_notification: %v", err)
	}

	// Build movie URL based on environment
	movieURL := p.buildMovieURL(movieResult.Slug)

	// Build genres arrays
	var genres []string
	var genresUz []string
	if metadata != nil {
		genres = metadata.Genres
		genresUz = metadata.GenresUz
	}

	// Build the notification payload
	payload := map[string]interface{}{
		"title":       movieResult.DisplayTitle,
		"year":        metadata.Year,
		"genres":      genres,
		"genres_uz":   genresUz,
		"code":        movieResult.Code,
		"poster_url":  posterURL,
		"quality":     metadata.Quality,
		"duration":    metadata.Duration,
		"description": metadata.Description,
		"movie_url":   movieURL,
		"slug":        movieResult.Slug,
	}

	// Call the backend API
	result, err := p.callTelegramAPI(payload)
	if err != nil {
		// Log the error but don't fail - movie is already created
		errMsg := fmt.Sprintf("Telegram API call failed: %v", err)
		log.Printf("[TELEGRAM] ERROR: %s", errMsg)
		p.log(jobID, errMsg, "error")

		// Update job with error status
		if updateErr := p.jobRepo.UpdateTelegramNotification(ctx, jobID, false, nil, false, errMsg); updateErr != nil {
			log.Printf("[TELEGRAM] WARNING: Failed to update Telegram notification status: %v", updateErr)
		}

		return err
	}

	// Update job with success status
	var channelsPosted []string
	var botNotified bool
	if result != nil {
		channelsPosted = result.ChannelPosted
		botNotified = result.BotNotified
		if !result.Success {
			log.Printf("[TELEGRAM] WARNING: Telegram notification completed with issues")
			if result.ErrorMessage != "" {
				log.Printf("[TELEGRAM] Error: %s", result.ErrorMessage)
			}
		}
	}

	if updateErr := p.jobRepo.UpdateTelegramNotification(ctx, jobID, true, channelsPosted, botNotified, ""); updateErr != nil {
		log.Printf("[TELEGRAM] WARNING: Failed to update Telegram notification status: %v", updateErr)
	}

	log.Printf("[TELEGRAM] ===== TELEGRAM NOTIFICATION COMPLETED =====")
	log.Printf("[TELEGRAM] Channels posted: %v", channelsPosted)
	log.Printf("[TELEGRAM] Bot notified: %v", botNotified)

	// Log success
	p.log(jobID, fmt.Sprintf("Telegram notification sent: channels=%v, bot=%v", channelsPosted, botNotified), "info")

	return nil
}

// TelegramNotificationResult holds the result from the Telegram API
type TelegramNotificationResult struct {
	Success       bool
	ChannelPosted []string
	BotNotified   bool
	ErrorMessage  string
}

// callTelegramAPI makes the HTTP call to the backend API to send Telegram notification
func (p *Pipeline) callTelegramAPI(payload map[string]interface{}) (*TelegramNotificationResult, error) {
	if p.config.BackendURL == "" {
		return nil, fmt.Errorf("backend URL not configured")
	}

	// Marshal payload
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Build URL
	url := fmt.Sprintf("%s/api/telegram/notify-movie", p.config.BackendURL)

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonPayload))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if p.config.WorkerToken != "" {
		req.Header.Set("X-Worker-Token", p.config.WorkerToken)
	}

	// Create client with timeout
	client := &http.Client{
		Timeout: 60 * time.Second,
	}

	// Make request
	log.Printf("[TELEGRAM] Calling backend API: %s", url)
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	log.Printf("[TELEGRAM] Backend callback: url=%s, status=%d, body=%s", url, resp.StatusCode, string(body))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("backend returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var result TelegramNotificationResult
	if err := json.Unmarshal(body, &result); err != nil {
		// If parsing fails, assume partial success
		log.Printf("[TELEGRAM] WARNING: Failed to parse response: %v", err)
		return &TelegramNotificationResult{
			Success:      true, // Backend returned 200, so it's considered success
			ErrorMessage: fmt.Sprintf("partial success, response parse error: %v", err),
		}, nil
	}

	return &result, nil
}

// buildMovieURL builds the full movie URL based on environment
func (p *Pipeline) buildMovieURL(slug string) string {
	if p.config.BackendURL == "" {
		return ""
	}

	// Extract base URL (remove /api or /v1 suffix if present)
	baseURL := p.config.BackendURL
	baseURL = strings.TrimSuffix(baseURL, "/api")
	baseURL = strings.TrimSuffix(baseURL, "/v1")
	baseURL = strings.TrimSuffix(baseURL, "/")

	return fmt.Sprintf("%s/movies/%s", baseURL, slug)
}

// validateAssetStorage verifies all assets are stored in the canonical movie folder
// This prevents silent asset scattering across multiple directories
//
// NOTE: Updated for adaptive HLS - checks for the master playlist (index.m3u8;
// legacy master.m3u8 also tolerated) and rendition subdirectories
func (p *Pipeline) validateAssetStorage(jobID, canonicalFolder, posterURL, backdropURL, streamingURL string) {
	log.Printf("[VALIDATION] ===== ASSET STORAGE VALIDATION =====")
	log.Printf("[VALIDATION] Canonical folder: %s", canonicalFolder)

	baseDir, err := os.Getwd()
	if err != nil {
		log.Printf("[VALIDATION] WARNING: Cannot get working directory: %v", err)
		return
	}

	movieDir := filepath.Join(baseDir, "uploads", "movies", canonicalFolder)

	// Check if movie directory exists
	if _, err := os.Stat(movieDir); os.IsNotExist(err) {
		log.Printf("[VALIDATION] WARNING: Movie directory does not exist: %s", movieDir)
		return
	}

	// List all files in the canonical folder
	files, err := os.ReadDir(movieDir)
	if err != nil {
		log.Printf("[VALIDATION] WARNING: Cannot read movie directory: %v", err)
		return
	}

	log.Printf("[VALIDATION] Found %d items in canonical folder:", len(files))

	// Track expected and found assets
	hlsFound := false
	posterFound := false
	backdropFound := false
	renditionsFound := make(map[string]bool)

	for _, file := range files {
		name := file.Name()
		isDir := file.IsDir()

		// Check for master playlist (adaptive HLS) — index.m3u8 is the
		// canonical name; master.m3u8 is the legacy name kept for older
		// content that hasn't been re-ingested yet.
		if name == MasterPlaylistName || name == "master.m3u8" {
			hlsFound = true
			log.Printf("[VALIDATION]   ✓ Master playlist: %s", name)
		}

		// Check for rendition subdirectories (360p, 480p, 720p, 1080p)
		if isDir && (name == "360p" || name == "480p" || name == "720p" || name == "1080p") {
			renditionsFound[name] = true
			log.Printf("[VALIDATION]   ✓ Rendition directory: %s/", name)

			// Check for index.m3u8 in rendition directory
			renditionFiles, _ := os.ReadDir(filepath.Join(movieDir, name))
			for _, rf := range renditionFiles {
				if rf.Name() == "index.m3u8" {
					log.Printf("[VALIDATION]     ✓ %s/index.m3u8", name)
				}
			}
		}

		// Check for old-style single-bitrate HLS files (for backward compatibility)
		if name == "index.m3u8" || strings.HasPrefix(name, "segment_") {
			hlsFound = true
			log.Printf("[VALIDATION]   ✓ HLS file: %s", name)
		}

		if strings.Contains(name, "poster") && !isDir {
			posterFound = true
			log.Printf("[VALIDATION]   ✓ Poster: %s", name)
		}
		if strings.Contains(name, "backdrop") && !isDir {
			backdropFound = true
			log.Printf("[VALIDATION]   ✓ Backdrop: %s", name)
		}
	}

	// Log rendition summary
	if len(renditionsFound) > 0 {
		log.Printf("[VALIDATION]   Renditions found: %v", getMapKeys(renditionsFound))
	}

	// Validate storage consistency
	log.Printf("[VALIDATION] Storage validation results:")
	log.Printf("[VALIDATION]   HLS files: %v", hlsFound)
	log.Printf("[VALIDATION]   Poster: %v", posterFound)
	log.Printf("[VALIDATION]   Backdrop: %v", backdropFound)

	// Check if URLs point to the canonical folder
	if strings.Contains(posterURL, canonicalFolder) {
		log.Printf("[VALIDATION]   ✓ Poster URL contains canonical folder: %s", canonicalFolder)
	} else if posterURL != "" {
		log.Printf("[VALIDATION]   ⚠ WARNING: Poster URL does NOT contain canonical folder!")
		log.Printf("[VALIDATION]     Expected folder: %s", canonicalFolder)
		log.Printf("[VALIDATION]     Actual URL: %s", posterURL)
	}

	if strings.Contains(backdropURL, canonicalFolder) {
		log.Printf("[VALIDATION]   ✓ Backdrop URL contains canonical folder: %s", canonicalFolder)
	} else if backdropURL != "" {
		log.Printf("[VALIDATION]   ⚠ WARNING: Backdrop URL does NOT contain canonical folder!")
		log.Printf("[VALIDATION]     Expected folder: %s", canonicalFolder)
		log.Printf("[VALIDATION]     Actual URL: %s", backdropURL)
	}

	if strings.Contains(streamingURL, canonicalFolder) {
		log.Printf("[VALIDATION]   ✓ Streaming URL contains canonical folder: %s", canonicalFolder)
	} else if streamingURL != "" {
		log.Printf("[VALIDATION]   ⚠ WARNING: Streaming URL does NOT contain canonical folder!")
		log.Printf("[VALIDATION]     Expected folder: %s", canonicalFolder)
		log.Printf("[VALIDATION]     Actual URL: %s", streamingURL)
	}

	log.Printf("[VALIDATION] ===== END ASSET VALIDATION =====")
}

// getMapKeys returns the keys of a map as a slice
func getMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// cleanupFile removes a temporary source file (parser download or B2 temp
// download) after the pipeline has successfully processed it AND saved the
// movie to the database. Safe to call with an empty path or a non-existent
// path — both are logged and skipped.
//
// Why the old `.mp4` blanket refusal was removed: the final output is HLS
// (.m3u8 + .ts), never the source .mp4. Parser downloads land in
// parser/downloads/... as .mp4 and are strictly intermediate, so the old
// guard was silently keeping every source file on disk.
func (p *Pipeline) cleanupFile(path string) {
	if path == "" {
		log.Printf("[CLEANUP] SKIP: empty path")
		return
	}

	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			log.Printf("[CLEANUP] SKIP: file does not exist: %s", path)
		} else {
			log.Printf("[CLEANUP] SKIP: stat failed for %s: %v", path, err)
		}
		return
	}
	if info.IsDir() {
		log.Printf("[CLEANUP] SKIP: path is a directory, not a file: %s", path)
		return
	}

	if err := os.Remove(path); err != nil {
		log.Printf("[CLEANUP] FAILED: remove %s: %v", path, err)
		return
	}
	log.Printf("[CLEANUP] DELETED: source file %s", path)

	// Best-effort: remove parent dir if empty (e.g. parser/downloads/<job_id>/).
	parent := filepath.Dir(path)
	if entries, rerr := os.ReadDir(parent); rerr == nil && len(entries) == 0 {
		if rmErr := os.Remove(parent); rmErr == nil {
			log.Printf("[CLEANUP] DELETED: empty parent dir %s", parent)
		}
	}
}

// cleanupDir removes a temporary directory
// But keeps the final HLS output (worker/uploads/... and worker/readyvideo/... for dev)
func (p *Pipeline) cleanupDir(path string) {
	if path == "" {
		return
	}

	// Safety check: Never delete worker/uploads directories (final output)
	if strings.Contains(path, "worker/uploads") {
		log.Printf("[CLEANUP] SKIP: Not deleting uploads dir: %s", path)
		return
	}

	// Safety check: Never delete readyvideo directories during processing
	// These are cleaned up after finalization via defer
	if strings.Contains(path, "readyvideo") {
		log.Printf("[CLEANUP] SKIP: Not deleting readyvideo dir: %s", path)
		return
	}

	log.Printf("[CLEANUP] Deleting temp dir: %s", path)
	os.RemoveAll(path)
}

// downloadFile downloads a file from URL to local path
// resolveStreamURLToLocalPath converts a dev-mode stream URL such as
// "http://localhost:8080/stream/{folder}/poster.jpg" to the on-disk path
// "uploads/movies/{folder}/poster.jpg" so we never need an HTTP round-trip
// to the backend just to read a file we already wrote.
func (p *Pipeline) resolveStreamURLToLocalPath(streamURL string) string {
	baseURL := p.config.StorageConfig.BaseURL
	prefix := baseURL + "/stream/"
	if baseURL != "" && strings.HasPrefix(streamURL, prefix) {
		rel := strings.TrimPrefix(streamURL, prefix) // e.g. "folder/poster.jpg"
		baseDir, _ := os.Getwd()
		return filepath.Join(baseDir, "uploads", "movies", rel)
	}
	return ""
}

func (p *Pipeline) downloadFile(url, localPath string) error {
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download %s returned HTTP %d", url, resp.StatusCode)
	}

	// Reject non-image responses (HTML error pages, JSON, etc.) before writing to disk.
	// TMDB occasionally returns an HTML page when an image path is wrong or expired.
	ct := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(ct, "image/") {
		return fmt.Errorf("download %s returned non-image content-type %q — not saving", url, ct)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response for %s: %w", url, err)
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file %s: %w", localPath, err)
	}

	log.Printf("[PIPELINE] Downloaded %d bytes (%s) → %s", len(data), ct, localPath)
	return nil
}

// createMovieSlug creates a safe filesystem slug from movie title
// Examples:
//   - "Forsaj 8" -> "forsaj8"
//   - "Spider-Man: No Way Home" -> "spidermannowayhome"
//
// junkPattern matches quality tags, episode markers, year numbers, and filler words common in
// Uzbek source titles: "480p", "1080p", "2021", "1-qism", "+41", "seriali", etc.
var junkPattern = regexp.MustCompile(
	`(?i)\b(` +
		`\d{3,4}p` + // 480p 720p 1080p
		`|4k` +
		`|hd|fhd` +
		`|\d{4}` + // 4-digit years like 2021, 2020, 2022, etc.
		`|\d+-?qism\b` + // 1-qism, 2qism
		`|qism\s*\d*` +
		`|\+\d+` + // +41
		`|seriali?` +
		`|barcha\s+qismlar` +
		`|o['` + "\u2019" + `]zbek(cha)?(\s+tilida)?` +
		`|uzbek(\s+tilida)?` +
		`|tilida` +
		`)\b`,
)

// cleanMovieTitle strips quality/episode/language junk from parser titles.
// "Bosqin seriali 1080p uzbek tilida" → "Bosqin"
func cleanMovieTitle(title string) string {
	cleaned := junkPattern.ReplaceAllString(title, " ")
	// collapse multiple spaces and trim
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	cleaned = strings.Trim(cleaned, " -–—.,:")
	if cleaned == "" {
		return title // fallback to original if everything was stripped
	}
	return cleaned
}

func createMovieSlug(title string) string {
	// Convert to lowercase
	s := strings.ToLower(title)

	// Replace non-alphanumeric chars with hyphens
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug := re.ReplaceAllString(s, "-")
	// Trim leading/trailing dashes
	slug = strings.Trim(slug, "-")

	return slug
}

// calculateWebsiteURL generates the website URL for a movie based on its slug
// Uses the same format as backend service for consistency
func calculateWebsiteURL(slug string) string {
	baseURL := os.Getenv("BASE_SITE_URL")
	if baseURL == "" {
		baseURL = "https://filmorauz.net"
	}
	return fmt.Sprintf("%s/movies/%s", baseURL, slug)
}
