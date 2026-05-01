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
	"strconv"
	"strings"
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

func normalizeDuplicateTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	re := regexp.MustCompile(`\s+`)
	return strings.ToLower(re.ReplaceAllString(trimmed, " "))
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
	ParserURL              string
	TempDir                string
	StorageConfig          storage.Config
	TMDBAPIKey             string          // TMDB API key for metadata enrichment
	DB                     *mongo.Database // MongoDB database for movie insertion
	BackendURL             string          // Backend API URL for Telegram notifications
	WorkerToken            string          // Token for worker-to-backend authentication
	MaxRenditionConcurrent int             // Max concurrent FFmpeg processes (default: 2)
	SegmentUploadWorkers   int             // Concurrent segment uploads per rendition (default: 10)
	SegmentUploadRetries   int             // Max retries per segment (default: 5)
	SegmentDuration        int             // HLS segment duration in seconds (default: 6)
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
			// Ensure job is marked as failed on any error
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

// processJobWithRecovery is the actual processing logic wrapped with panic recovery
func (p *Pipeline) processJobWithRecovery(ctx context.Context, job *models.IngestionJob) error {
	jobID := job.ID.Hex()
	log.Printf("[PIPELINE] Starting processing for job %s", jobID)

	// Handle direct_upload source - download from B2 temp path
	if job.Source == "direct_upload" {
		return p.processDirectUploadJob(ctx, job)
	}

	// Serial episode jobs skip the movie-enrichment + movie-creation path and
	// instead save their HLS into a per-episode folder and update the linked
	// Episode row on the backend.
	if job.ContentType == "episode" {
		return p.processEpisodeJob(ctx, job)
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
	log.Printf("[STAGE] clip_generation start — movie code=%s, folder=%s, processed_master=%s", movieResult.Code, canonicalFolderName, processedMasterPath)
	log.Printf("[CHECKPOINT] clip_input_path: %s", processedMasterPath)
	if movieResult != nil && movieResult.Code != "" {
		if clipErr := p.generateClips(ctx, canonicalFolderName, movieResult.Code, movieResult, processedMasterPath, finalUploadsPath); clipErr != nil {
			log.Printf("[CLIP] ERROR: Clip generation failed: %v", clipErr)
			clipGenerationFailed = true
		} else {
			log.Printf("[STAGE] clip_generation end — clips generated from final movie")
		}
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
		Success           bool   `json:"success"`
		FilePath          string `json:"file_path"`
		LocalPath         string `json:"local_path"`
		FileName          string `json:"file_name"`
		FileSize          int64  `json:"file_size"`
		StreamType        string `json:"stream_type"`
		DownloadCompleted bool   `json:"download_completed"`
		DownloadNeeded    bool   `json:"download_needed"`
		VideoFound        bool   `json:"video_found"`
		VideoURL          string `json:"video_url"` // Best source URL from /details
		Error             string `json:"error"`
		DownloadError     string `json:"download_error"`
		ManualReason      string `json:"manual_reason"`
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

	// Validate core fields from /details
	if !result.Success {
		return nil, "", fmt.Errorf("parser returned success=false")
	}
	if result.VideoURL == "" {
		return nil, "", fmt.Errorf("parser did not return video_url")
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
	result.ParsedMovieMetadata.VideoURL = result.VideoURL

	// Return metadata and local_path (may be empty if parser will download separately)
	return &result.ParsedMovieMetadata, localPath, nil
}

// callParserDownload calls the parser /download endpoint to start download
// Then polls /progress until download completes
func (p *Pipeline) callParserDownload(jobID, videoURL string) (string, error) {
	jobIDCopy := jobID
	log.Printf("[WORKER] download start — job_id=%s, url=%s", jobIDCopy, safeTruncate(videoURL, 80))

	// Build download endpoint URL
	parserEndpoint := fmt.Sprintf("%s/download", p.config.ParserURL)
	params := url.Values{}
	params.Set("video_url", videoURL)
	params.Set("job_id", jobIDCopy)
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

		log.Printf("[WORKER] polling — job_id=%s, status=%s, progress_percent=%d%%, downloaded=%d/%d",
			jobIDCopy, progress.Status, progress.ProgressPercent, progress.DownloadedBytes, progress.TotalBytes)

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
func (p *Pipeline) startDownloadAndPoll(ctx context.Context, job *models.IngestionJob) (string, error) {
	jobID := job.ID.Hex()
	videoURL := job.VideoURL

	log.Printf("[WORKER] startDownloadAndPoll — job_id=%s, url=%s", jobID, safeTruncate(videoURL, 60))

	// Build download endpoint URL
	parserEndpoint := fmt.Sprintf("%s/download", p.config.ParserURL)
	params := url.Values{}
	params.Set("video_url", videoURL)
	params.Set("job_id", jobID)
	safeName := regexp.MustCompile(`[^\w\s-]`).ReplaceAllString(jobID, "")
	safeName = regexp.MustCompile(`[-\s]+`).ReplaceAllString(safeName, "_")
	params.Set("output_name", safeName+".mp4")

	downloadURL := parserEndpoint + "?" + params.Encode()

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
		// Hard ceiling reduced from 7200s for testing — the watchdog should
		// catch stuck downloads long before this fires.
		maxPollSeconds        = 900
		pollInterval          = 1 * time.Second
		noProgressTimeout     = 60 * time.Second
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
	)

	for pollCount := 0; pollCount < maxPollSeconds; pollCount++ {
		time.Sleep(pollInterval)

		progressURL := fmt.Sprintf("%s/progress?job_id=%s", p.config.ParserURL, jobID)
		progressResp, err := p.httpClient.Get(progressURL)
		if err != nil {
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

		rawBody, readErr := io.ReadAll(progressResp.Body)
		statusCode := progressResp.StatusCode
		progressResp.Body.Close()

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
		if progressPercent == 0 && progress.DownloadedBytes > 0 && progress.TotalBytes > 0 {
			computed := int((float64(progress.DownloadedBytes) / float64(progress.TotalBytes)) * 100)
			if computed > 0 {
				progressPercent = computed
			}
		}

		stalledFor := time.Since(lastBytesAt)
		log.Printf("[WORKER] /progress poll=%d job_id=%s status=%s pct=%d%% bytes=%d/%d speed=%.2fMB/s eta=%ds stalled_for=%s",
			pollCount, jobID, progress.Status, progressPercent,
			progress.DownloadedBytes, progress.TotalBytes, progress.SpeedMBps, progress.EtaSeconds,
			stalledFor.Truncate(time.Second))

		if progress.Status == "downloading" || progress.Status == "starting" {
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
		} else if progress.Status == "downloading" || progress.Status == "starting" {
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
			return "", fmt.Errorf("parser /download reported failed: %s", progress.Error)
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

	// Step 5: Generate clips (optional - don't fail if clips fail)
	if movieResult != nil {
		movieCode := fmt.Sprintf("%d", time.Now().Unix()%10000) // Generate a code since we didn't get one from DB

		// Create a simple MovieCreationResult for clip generation
		clipMovieResult := &MovieCreationResult{
			MovieID:      movieResult,
			Code:         movieCode,
			Slug:         createMovieSlug(title),
			DisplayTitle: title,
		}

		log.Printf("[DIRECT_UPLOAD] Generating clips for movie: %s", title)
		if clipErr := p.generateClips(ctx, canonicalFolderName, movieCode, clipMovieResult, masterPath, hlsDir); clipErr != nil {
			log.Printf("[DIRECT_UPLOAD] WARNING: Clip generation failed: %v", clipErr)
			log.Printf("[DIRECT_UPLOAD] Movie created successfully, but clips were not generated")
		} else {
			log.Printf("[DIRECT_UPLOAD] Clips generated successfully")
		}
	}

	// Update status to completed
	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		log.Printf("[DIRECT_UPLOAD] Failed to mark job completed: %v", err)
	}

	// Step 6: Cleanup B2 temp file
	if err := p.cleanupTempFile(job); err != nil {
		log.Printf("[DIRECT_UPLOAD] Warning: failed to cleanup temp file from B2: %v", err)
	} else {
		log.Printf("[DIRECT_UPLOAD] Cleaned up B2 temp file")
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

	// Heartbeat: update job every 30 seconds during processing to prevent stale-job detection
	// FFmpeg processing can take 30-60+ minutes, so we need heartbeats to keep the job alive
	heartbeatCtx, heartbeatCancel := context.WithCancel(context.Background())
	defer heartbeatCancel()
	go func() {
		heartbeatTicker := time.NewTicker(30 * time.Second)
		defer heartbeatTicker.Stop()
		for {
			select {
			case <-heartbeatCtx.Done():
				return
			case <-heartbeatTicker.C:
				// Heartbeat update - only update timestamp, keep current progress/status
				if err := p.jobRepo.Heartbeat(context.Background(), jobID); err == nil {
					log.Printf("[WORKER] heartbeat: job %s still processing", jobID)
				}
			}
		}
	}()

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
func (p *Pipeline) createMovieInDatabase(job *models.IngestionJob, metadata *models.ParsedMovieMetadata, streamingURL string) (interface{}, error) {
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

	movieDoc := bson.M{
		"code":             code,
		"slug":             displaySlug,
		"title":            displayTitle,
		"normalized_title": normalizedDisplayTitle,
		"original_title":   metadata.Title,
		"description":      displayDescription,
		"year":             metadata.Year,
		"genre":            metadata.Genres,
		"country":          metadata.Country,
		"duration":         metadata.Duration,
		"poster_url":       metadata.Poster,
		"backdrop_url":     metadata.Backdrop,
		"video_url":        streamingURL,
		"source_type":      "direct_hls",
		"source": bson.M{
			"provider":   job.Source,
			"source_url": job.DetailURL,
			"source_id":  job.SourceID,
		},
		"quality":    job.Quality,
		"is_premium": job.IsPremium,
		"status":     "published",
		"created_at": time.Now(),
		"updated_at": time.Now(),
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
		return existingDoc["_id"], nil
	}

	// Insert the movie
	result, err := p.movieCol.InsertOne(context.Background(), movieDoc)
	if err != nil {
		log.Printf("[DIRECT_UPLOAD] ERROR: Failed to insert movie: %v", err)
		return nil, fmt.Errorf("failed to create movie: %w", err)
	}

	log.Printf("[DIRECT_UPLOAD] Movie created: id=%v, code=%s, title=%s", result.InsertedID, code, displayTitle)
	p.log(jobID, fmt.Sprintf("Movie created: %s (code: %s)", displayTitle, code), "info")

	return result.InsertedID, nil
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
		"quality":               enrichedMetadata.Quality,
		"translation":           enrichedMetadata.Translation,
		"poster_url":            posterURL,           // Generated/localized poster
		"original_poster_url":   originalPosterURL,   // Original TMDB poster
		"backdrop_url":          backdropURL,         // Generated/localized backdrop
		"original_backdrop_url": originalBackdropURL, // Original TMDB backdrop
		"video_url":             streamingURL,
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
// NOTE: Updated for adaptive HLS - checks for master.m3u8 and rendition subdirectories
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

		// Check for master playlist (adaptive HLS)
		if name == "master.m3u8" {
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
