package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
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
	"github.com/filmorauz/worker/services"
	"github.com/filmorauz/worker/storage"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// Config holds pipeline configuration
type Config struct {
	ParserURL     string
	TempDir       string
	StorageConfig storage.Config
	AIEndpoint    string                 // Legacy: Optional AI image generation endpoint (custom API)
	TMDBAPIKey    string                 // TMDB API key for metadata enrichment
	DB            *mongo.Database        // MongoDB database for movie insertion
	BackendURL    string                 // Backend API URL for Telegram notifications
	WorkerToken   string                 // Token for worker-to-backend authentication
	OpenAIConfig  *services.OpenAIConfig // OpenAI configuration (API key, model, etc.)
}

// Pipeline handles the video ingestion pipeline
type Pipeline struct {
	config           Config
	jobRepo          *repositories.JobRepository
	storage          storage.Storage
	httpClient       *http.Client
	enricher         *MetadataEnricher
	posterGen        *PosterGenerator
	openAIClient     *services.OpenAIClient          // OpenAI client for poster generation
	movieCol         *mongo.Collection               // MongoDB movies collection for direct insertion
	dbName           string                          // Database name for logging
	watermarkService *services.WatermarkRemovalService // Dynamic watermark removal
}

// NewPipeline creates a new pipeline instance
func NewPipeline(config Config, jobRepo *repositories.JobRepository) (*Pipeline, error) {
	store, err := storage.NewStorage(config.StorageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Create metadata enricher with TMDB support
	enricher := NewMetadataEnricher(config.ParserURL, config.TMDBAPIKey)

	// Initialize OpenAI client if configured
	var openAIClient *services.OpenAIClient
	if config.OpenAIConfig != nil && config.OpenAIConfig.APIKey != "" {
		openAIClient, err = services.NewOpenAIClient(config.OpenAIConfig)
		if err != nil {
			log.Printf("[PIPELINE] WARNING: Failed to create OpenAI client: %v", err)
			// Continue without OpenAI - will use legacy endpoint or skip
		} else if openAIClient.IsConfigured() {
			log.Printf("[PIPELINE] OpenAI client initialized successfully")
		}
	} else {
		log.Printf("[PIPELINE] OpenAI not configured - API key not provided")
	}

	// Create poster generator with optional OpenAI client
	posterGen, err := NewPosterGeneratorWithOpenAI(config.StorageConfig, config.TempDir, config.AIEndpoint, config.StorageConfig.BaseURL, openAIClient)
	if err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to create poster generator: %v", err)
		// Continue without poster generator - it's optional
		posterGen = nil
	}

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

	// Initialize watermark removal service
	watermarkConfig := services.DefaultWatermarkRemovalConfig()
	if config.TempDir != "" {
		watermarkConfig.TempDir = config.TempDir
	}
	watermarkService := services.NewWatermarkRemovalService(watermarkConfig)

	return &Pipeline{
		config:  config,
		jobRepo: jobRepo,
		storage: store,
		httpClient: &http.Client{
			Timeout: 30 * time.Minute, // Very long timeout for parser downloads (can take 10-20+ minutes)
		},
		enricher:         enricher,
		posterGen:        posterGen,
		openAIClient:     openAIClient,
		movieCol:         movieCol,
		dbName:           dbName,
		watermarkService: watermarkService,
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

	// Step 1: Check if we can skip download (file already exists)
	// This allows retry without re-downloading
	var metadata *models.ParsedMovieMetadata
	var localPath string
	var err error // Declare err at function level for download step

	// Check if job already has local_path from previous download
	if job.LocalPath != "" && job.Metadata != nil {
		log.Printf("[PIPELINE] Job has local_path: %s", job.LocalPath)
		// Verify file exists and is valid
		if fileInfo, err := os.Stat(job.LocalPath); err == nil && fileInfo.Size() > 0 {
			log.Printf("[PIPELINE] Skipping download - file already exists (size: %d bytes)", fileInfo.Size())
			localPath = job.LocalPath
			metadata = job.Metadata
			// CRITICAL: Validate metadata is not nil and has required fields
			if metadata == nil || metadata.Title == "" {
				log.Printf("[PIPELINE] WARNING: metadata is nil or empty, will re-parse")
				localPath = ""
			} else {
				// Mark download as complete if not already set
				if !job.Steps.Download {
					if err := p.jobRepo.UpdateStep(ctx, jobID, "download"); err != nil {
						log.Printf("[PIPELINE] Failed to mark download step: %v", err)
					}
				}
				log.Printf("[PIPELINE] Using existing metadata: %s (%d)", metadata.Title, metadata.Year)
			}
		} else {
			log.Printf("[PIPELINE] File does not exist or is empty, will re-download")
			localPath = ""
		}
	}

	// If no valid local_path, call parser to download
	// CRITICAL: Parser contract now requires parser to download the file
	// If local_path is empty, parser has failed to download - fail immediately
	if localPath == "" {
		log.Printf("[PIPELINE] No local_path found, calling parser to download video...")
		log.Printf("[PIPELINE] NOTE: Parser is expected to download and return local_path")

		// Call parser - it should download the file and return local_path
		// If parser returns empty local_path, it means parser download failed
		var parseErr error
		metadata, localPath, parseErr = p.parseMovieDetails(job)
		if parseErr != nil {
			return fmt.Errorf("parsing failed: %w", parseErr)
		}

		// DEFENSIVE: If parser provided local_path, update the job immediately
		if localPath != "" {
			log.Printf("[PIPELINE] Parser downloaded file to: %s", localPath)
			// Update job with local_path so worker can find the file
			if err := p.jobRepo.UpdateLocalPath(ctx, jobID, localPath); err != nil {
				log.Printf("[PIPELINE] Failed to update local_path: %v", err)
				// Continue - this is not fatal, we can still proceed
			}
			// Also mark download as complete
			if err := p.jobRepo.UpdateStep(ctx, jobID, "download"); err != nil {
				log.Printf("[PIPELINE] Failed to mark download step: %v", err)
			}
		} else {
			// Parser returned empty local_path - this is a CONTRACT VIOLATION
			// Parser MUST download the file and return local_path
			// Do NOT retry - parser download has failed
			log.Printf("[PIPELINE] ERROR: Parser returned empty local_path - download failed!")
			log.Printf("[PIPELINE] Parser contract violation: parser must download file before returning")

			// Mark job as download_failed for clearer admin visibility
			if err := p.failJobWithStatus(jobID, models.IngestionStatusDownloadFailed, fmt.Sprintf("parser failed to download file - returned empty local_path. Parser must download the video and return a valid local_path before worker can proceed.")); err != nil {
				log.Printf("[PIPELINE] Failed to mark job as download_failed: %v", err)
			}

			return fmt.Errorf("parser failed to download file - returned empty local_path. Parser must download the video and return a valid local_path before worker can proceed.")
		}
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
	if localPath != "" {
		videoPath = localPath
		log.Printf("[STAGE] download complete — using parser-downloaded file: %s", videoPath)
	} else {
		videoPath, err = p.downloadVideo(job, metadata)
		if err != nil {
			return fmt.Errorf("download failed: %w", err)
		}
		log.Printf("[STAGE] download complete — videoPath: %s", videoPath)
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
	shortJobId := job.ID.Hex()[:8]
	canonicalFolderName := fmt.Sprintf("%s_%s", canonicalSlug, shortJobId)
	log.Printf("[PIPELINE] Canonical movie folder (computed once): %s", canonicalFolderName)
	log.Printf("[PIPELINE] Canonical title source: %s", canonicalTitle)

	// Step 3: Process video (watermark removal + HLS conversion)
	log.Printf("[STAGE] process starting — input: %s, folder: %s", videoPath, canonicalFolderName)
	hlsPath, err := p.processVideo(job, localPath, canonicalFolderName)
	if err != nil {
		return fmt.Errorf("processing failed: %w", err)
	}
	defer p.cleanupDir(hlsPath)
	log.Printf("[STAGE] process complete — hlsPath: %s", hlsPath)

	// Mark process step done so ClaimNextProcessingJob won't re-pick this job
	if stepErr := p.jobRepo.UpdateStep(ctx, jobID, "process"); stepErr != nil {
		log.Printf("[STAGE] WARNING: failed to mark process step: %v", stepErr)
	}

	// CRITICAL: Mark source file as deleted and update with processed path
	// This clears the stale local_path (parser/downloads) and sets output_path
	if err := p.jobRepo.MarkSourceFileDeleted(ctx, jobID, hlsPath); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to mark source file deleted: %v", err)
		// Continue - not fatal, but dashboard will show stale info
	} else {
		log.Printf("[PIPELINE] Marked source file deleted, output_path=%s, cleared local_path", hlsPath)
	}

	// Update status to uploading
	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 70); err != nil {
		return fmt.Errorf("failed to update status to uploading: %w", err)
	}

	// Step 4: Upload processed files (MODE-based finalization)
	log.Printf("[STAGE] upload starting — hlsPath: %s", hlsPath)
	if err := p.updateStatus(jobID, models.IngestionStatusFinalizingStorage, 65); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to finalizing_storage: %v", err)
	}
	streamingURL, err := p.uploadProcessedFiles(job, hlsPath, localPath, canonicalFolderName)
	if err != nil {
		return fmt.Errorf("upload failed: %w", err)
	}
	log.Printf("[STAGE] upload complete — streamingURL: %s", streamingURL)

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

	// Update progress to 90%
	if err := p.updateStatus(jobID, models.IngestionStatusUploading, 90); err != nil {
		return fmt.Errorf("failed to update status to 90%%: %w", err)
	}

	// Log final message with naming convention
	movieSlug := sanitizeFilename(title)
	p.log(jobID, fmt.Sprintf("%s+uploaded", movieSlug), "info")

	// ===== STEP 5: ENRICH METADATA =====
	p.log(jobID, "Starting metadata enrichment", "info")
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

	// ===== STEP 6: GENERATE POSTER =====
	p.log(jobID, "Starting poster generation", "info")

	// Use the canonical folder name computed earlier (same as HLS output)
	// This ensures ALL assets (HLS, poster, backdrop) are in the SAME folder
	log.Printf("[PIPELINE] Using canonical movie folder for poster: %s", canonicalFolderName)

	var finalPosterURL string
	var originalPosterURL string
	posterGenerated := false
	posterGenStatus := "not_available"
	localizationAttempted := false
	localizationSucceeded := false

	// NOTE: Status update for finding_poster is logged but does NOT fail the job
	// Poster generation is non-critical - we continue even if status update fails
	if err := p.updateStatus(jobID, models.IngestionStatusFindingPoster, 80); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to finding_poster: %v (continuing anyway)", err)
	}

	if p.posterGen != nil {
		log.Printf("[PIPELINE] ===== POSTER GENERATION START =====")
		log.Printf("[PIPELINE] PosterGenerator available, attempting poster generation")

		posterResult, err := p.posterGen.GeneratePoster(ctx, enrichedMetadata, canonicalFolderName)
		if err != nil {
			log.Printf("[PIPELINE] Poster generation error: %v", err)
			// Fallback to enriched metadata poster
			finalPosterURL = enrichedMetadata.PosterURL
			originalPosterURL = enrichedMetadata.PosterURL
			posterGenerated = false
			posterGenStatus = "error_fallback_to_original"
			log.Printf("[PIPELINE] POSTER RESULT: Error occurred, FALLBACK to original poster from parser/enricher")
			log.Printf("[PIPELINE] POSTER RESULT: Original poster URL: %s", finalPosterURL)
		} else if posterResult != nil {
			finalPosterURL = posterResult.GeneratedPosterURL
			if finalPosterURL == "" {
				finalPosterURL = posterResult.OriginalPosterURL
			}
			originalPosterURL = posterResult.OriginalPosterURL
			posterGenerated = posterResult.PosterGenerated

			if posterGenerated {
				posterGenStatus = "ai_generated"
				localizationAttempted = true
				localizationSucceeded = true
				log.Printf("[PIPELINE] POSTER RESULT: AI poster GENERATED successfully")
				log.Printf("[PIPELINE] POSTER RESULT: Generated poster URL: %s", finalPosterURL)
				log.Printf("[PIPELINE] POSTER RESULT: Original poster URL: %s", originalPosterURL)
			} else {
				posterGenStatus = "fallback_to_original"
				localizationAttempted = true
				localizationSucceeded = false
				log.Printf("[PIPELINE] POSTER RESULT: AI generation was NOT used")
				log.Printf("[PIPELINE] POSTER RESULT: Reason: AI endpoint not configured or generation skipped")
				log.Printf("[PIPELINE] POSTER RESULT: FALLBACK to original poster URL: %s", finalPosterURL)
			}
		} else {
			log.Printf("[PIPELINE] POSTER RESULT: No poster result returned")
			finalPosterURL = enrichedMetadata.PosterURL
			originalPosterURL = enrichedMetadata.PosterURL
			posterGenerated = false
			posterGenStatus = "no_result_fallback"
			localizationAttempted = true
			localizationSucceeded = false
		}

		log.Printf("[PIPELINE] ===== POSTER GENERATION END =====")

		// ===== STEP 6b: ADD LOGO OVERLAY AND LOCALIZATION =====
		// Use canonicalFolderName for consistent storage in SAME movie folder

		// If we have a generated poster (or original), add logo overlay and localize
		if finalPosterURL != "" {
			// First we need to download the poster locally to add logo
			// Check if poster is a local path or needs to be downloaded
			var posterLocalPath string

			// If poster is a local path (dev mode), use it directly
			if strings.HasPrefix(finalPosterURL, "/uploads") || !strings.HasPrefix(finalPosterURL, "http") {
				// It's already a local path
				posterLocalPath = strings.TrimPrefix(finalPosterURL, "/uploads/")
				baseDir, _ := os.Getwd()
				posterLocalPath = filepath.Join(baseDir, "uploads", posterLocalPath)
			} else {
				// It's a remote URL, download it
				if p.config.StorageConfig.Mode == "dev" {
					// Download to local path for development
					posterLocalPath = filepath.Join(p.config.TempDir, "poster_download.jpg")
					if err := p.downloadFile(finalPosterURL, posterLocalPath); err != nil {
						log.Printf("[PIPELINE] Failed to download poster for logo overlay: %v", err)
						posterLocalPath = ""
					}
				} else {
					// Production: download to temp, add logo, upload
					posterLocalPath = filepath.Join(p.config.TempDir, "poster_original.jpg")
					if err := p.downloadFile(finalPosterURL, posterLocalPath); err != nil {
						log.Printf("[PIPELINE] Failed to download poster for logo overlay: %v", err)
						posterLocalPath = ""
					}
				}
			}

			// Add logo overlay and localize if we have a local poster
			if posterLocalPath != "" {
				log.Printf("[PIPELINE] Using canonical folder for poster: %s", canonicalFolderName)

				// Localize and add premium branding
				brandedPath, logoErr := p.posterGen.LocalizeAndBrandPoster(ctx, posterLocalPath, canonicalFolderName, enrichedMetadata)
				if logoErr != nil {
					log.Printf("[PIPELINE] Logo overlay/localization failed: %v, trying basic branding", logoErr)
					// Fallback to basic branding without localization
					brandedPath, logoErr = p.posterGen.AddLogoOverlay(ctx, posterLocalPath, canonicalFolderName)
					if logoErr != nil {
						log.Printf("[PIPELINE] Basic logo overlay also failed: %v, using unbranded poster", logoErr)
						brandedPath = posterLocalPath
					}
				}

				if brandedPath != "" && brandedPath != posterLocalPath {
					// Upload branded poster
					brandedURL, uploadErr := p.posterGen.UploadBrandedPoster(ctx, brandedPath, canonicalFolderName)
					if uploadErr != nil {
						log.Printf("[PIPELINE] Failed to upload branded poster: %v, using unbranded", uploadErr)
					} else {
						finalPosterURL = brandedURL
						posterGenerated = true // branded+localized poster was produced
						log.Printf("[PIPELINE] Branded poster ready: %s (poster_generated=true)", brandedURL)
						log.Printf("[PIPELINE] Localization attempted: %v, succeeded: %v", localizationAttempted, localizationSucceeded)

						// Cleanup temp file
						if strings.Contains(brandedPath, p.config.TempDir) {
							os.Remove(brandedPath)
						}
					}
				}

				// Cleanup downloaded original poster
				if strings.Contains(posterLocalPath, p.config.TempDir) {
					os.Remove(posterLocalPath)
				}
			}
		}

		// Update job with poster info
		if err := p.jobRepo.UpdatePosterInfo(ctx, jobID, originalPosterURL, finalPosterURL, posterGenerated); err != nil {
			log.Printf("[PIPELINE] Failed to update poster info: %v", err)
		}
	} else {
		// Use enriched poster without generation
		finalPosterURL = enrichedMetadata.PosterURL
		originalPosterURL = enrichedMetadata.PosterURL
		posterGenerated = false
		posterGenStatus = "not_available"
		log.Printf("[PIPELINE] PosterGenerator NOT available (nil), using original poster from parser: %s", finalPosterURL)
	}

	// Log poster generation status summary
	log.Printf("[PIPELINE] ===== POSTER STATUS SUMMARY =====")
	log.Printf("[PIPELINE] posterGenStatus: %s", posterGenStatus)
	log.Printf("[PIPELINE] posterGenerated: %v", posterGenerated)
	log.Printf("[PIPELINE] finalPosterURL: %s", finalPosterURL)
	log.Printf("[PIPELINE] originalPosterURL: %s", originalPosterURL)
	log.Printf("[PIPELINE] ===== END POSTER STATUS =====")

	if finalPosterURL == "" {
		log.Printf("[PIPELINE] WARNING: No poster URL available, will use placeholder or skip")
	}

	p.log(jobID, fmt.Sprintf("Poster ready (generated=%v)", posterGenerated), "info")

	// ===== STEP 6c: GENERATE BACKDROP =====
	p.log(jobID, "Starting backdrop generation", "info")
	if err := p.updateStatus(jobID, models.IngestionStatusGeneratingBackdrop, 85); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to generating_backdrop: %v", err)
	}

	var finalBackdropURL string
	var originalBackdropURL string
	backdropGenerated := false
	backdropGenStatus := "not_available"

	if p.posterGen != nil {
		log.Printf("[PIPELINE] ===== BACKDROP GENERATION START =====")
		log.Printf("[PIPELINE] PosterGenerator available, attempting backdrop generation")

		backdropResult, backdropErr := p.posterGen.GenerateBackdrop(ctx, enrichedMetadata, canonicalFolderName)
		if backdropErr != nil {
			log.Printf("[PIPELINE] Backdrop generation error: %v", backdropErr)
			// Fallback to enriched metadata backdrop
			finalBackdropURL = enrichedMetadata.BackdropURL
			originalBackdropURL = enrichedMetadata.BackdropURL
			backdropGenerated = false
			backdropGenStatus = "error_fallback_to_original"
			log.Printf("[PIPELINE] BACKDROP RESULT: Error occurred, FALLBACK to original backdrop from parser/enricher")
			log.Printf("[PIPELINE] BACKDROP RESULT: Original backdrop URL: %s", finalBackdropURL)
		} else if backdropResult != nil {
			finalBackdropURL = backdropResult.GeneratedBackdropURL
			if finalBackdropURL == "" {
				finalBackdropURL = backdropResult.OriginalBackdropURL
			}
			originalBackdropURL = backdropResult.OriginalBackdropURL
			backdropGenerated = backdropResult.BackdropGenerated

			if backdropGenerated {
				backdropGenStatus = "ai_generated"
				log.Printf("[PIPELINE] BACKDROP RESULT: AI backdrop GENERATED successfully")
				log.Printf("[PIPELINE] BACKDROP RESULT: Generated backdrop URL: %s", finalBackdropURL)
				log.Printf("[PIPELINE] BACKDROP RESULT: Original backdrop URL: %s", originalBackdropURL)
			} else {
				backdropGenStatus = "fallback_to_original"
				log.Printf("[PIPELINE] BACKDROP RESULT: AI generation was NOT used")
				log.Printf("[PIPELINE] BACKDROP RESULT: Reason: AI endpoint not configured or generation skipped")
				log.Printf("[PIPELINE] BACKDROP RESULT: FALLBACK to original backdrop URL: %s", finalBackdropURL)
			}
		} else {
			log.Printf("[PIPELINE] BACKDROP RESULT: No backdrop result returned")
			finalBackdropURL = enrichedMetadata.BackdropURL
			originalBackdropURL = enrichedMetadata.BackdropURL
			backdropGenerated = false
			backdropGenStatus = "no_result_fallback"
		}

		log.Printf("[PIPELINE] ===== BACKDROP GENERATION END =====")

		// ===== STEP 6d: ADD LOGO OVERLAY TO BACKDROP =====
		// If we have a generated backdrop (or original), add logo overlay
		if finalBackdropURL != "" {
			// First we need to download the backdrop locally to add logo
			// Check if backdrop is a local path or needs to be downloaded
			var backdropLocalPath string

			// If backdrop is a local path (dev mode), use it directly
			if strings.HasPrefix(finalBackdropURL, "/uploads") || !strings.HasPrefix(finalBackdropURL, "http") {
				// It's already a local path
				backdropLocalPath = strings.TrimPrefix(finalBackdropURL, "/uploads/")
				baseDir, _ := os.Getwd()
				backdropLocalPath = filepath.Join(baseDir, "uploads", backdropLocalPath)
			} else {
				// It's a remote URL, download it
				if p.config.StorageConfig.Mode == "dev" {
					// Download to local path for development
					backdropLocalPath = filepath.Join(p.config.TempDir, "backdrop_download.jpg")
					if err := p.downloadFile(finalBackdropURL, backdropLocalPath); err != nil {
						log.Printf("[PIPELINE] Failed to download backdrop for logo overlay: %v", err)
						backdropLocalPath = ""
					}
				} else {
					// Production: download to temp, add logo, upload
					backdropLocalPath = filepath.Join(p.config.TempDir, "backdrop_original.jpg")
					if err := p.downloadFile(finalBackdropURL, backdropLocalPath); err != nil {
						log.Printf("[PIPELINE] Failed to download backdrop for logo overlay: %v", err)
						backdropLocalPath = ""
					}
				}
			}

			// Add logo overlay if we have a local backdrop
			if backdropLocalPath != "" {
				log.Printf("[PIPELINE] Using canonical folder for backdrop: %s", canonicalFolderName)

				brandedPath, logoErr := p.posterGen.AddLogoOverlayForBackdrop(ctx, backdropLocalPath, canonicalFolderName)
				if logoErr != nil {
					log.Printf("[PIPELINE] Logo overlay failed: %v, using unbranded backdrop", logoErr)
				} else if brandedPath != "" && brandedPath != backdropLocalPath {
					// Upload branded backdrop
					brandedURL, uploadErr := p.posterGen.UploadBrandedBackdrop(ctx, brandedPath, canonicalFolderName)
					if uploadErr != nil {
						log.Printf("[PIPELINE] Failed to upload branded backdrop: %v, using unbranded", uploadErr)
					} else {
						finalBackdropURL = brandedURL
						log.Printf("[PIPELINE] Branded backdrop ready: %s", brandedURL)

						// Cleanup temp file
						if strings.Contains(brandedPath, p.config.TempDir) {
							os.Remove(brandedPath)
						}
					}
				}

				// Cleanup downloaded original backdrop
				if strings.Contains(backdropLocalPath, p.config.TempDir) {
					os.Remove(backdropLocalPath)
				}
			}
		}
	} else {
		// Use enriched backdrop without generation
		finalBackdropURL = enrichedMetadata.BackdropURL
		originalBackdropURL = enrichedMetadata.BackdropURL
		backdropGenerated = false
		backdropGenStatus = "not_available"
		log.Printf("[PIPELINE] PosterGenerator NOT available (nil), using original backdrop from parser: %s", finalBackdropURL)
	}

	// Log backdrop generation status summary
	log.Printf("[PIPELINE] ===== BACKDROP STATUS SUMMARY =====")
	log.Printf("[PIPELINE] backdropGenStatus: %s", backdropGenStatus)
	log.Printf("[PIPELINE] backdropGenerated: %v", backdropGenerated)
	log.Printf("[PIPELINE] finalBackdropURL: %s", finalBackdropURL)
	log.Printf("[PIPELINE] originalBackdropURL: %s", originalBackdropURL)
	log.Printf("[PIPELINE] ===== END BACKDROP STATUS =====")

	if finalBackdropURL == "" {
		log.Printf("[PIPELINE] WARNING: No backdrop URL available, will use placeholder or skip")
	}

	p.log(jobID, fmt.Sprintf("Backdrop ready (generated=%v)", backdropGenerated), "info")

	// Update progress to 90%
	// NOTE: Status update failure is logged but does NOT fail the job
	// Poster step is non-critical - we continue even if status update fails
	if err := p.updateStatus(jobID, models.IngestionStatusUploadingPoster, 90); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to 90%%: %v (continuing anyway)", err)
		// Continue processing - poster step is non-critical
	}

	// ===== STEP 7: CREATE MOVIE IN DATABASE =====
	p.log(jobID, "Creating movie in database", "info")
	if err := p.updateStatus(jobID, models.IngestionStatusCreatingMovie, 95); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to creating_movie: %v", err)
		// Continue - movie creation is more important than status update
	}

	// Log final metadata source before DB write
	log.Printf("[PIPELINE] ===== FINAL METADATA FOR MONGODB =====")
	log.Printf("[PIPELINE] metadataSource: %s", metadataSource)
	log.Printf("[PIPELINE] title: %s", enrichedMetadata.Title)
	log.Printf("[PIPELINE] originalTitle: %s", enrichedMetadata.OriginalTitle)
	log.Printf("[PIPELINE] year: %d", enrichedMetadata.Year)
	log.Printf("[PIPELINE] genres: %v", enrichedMetadata.Genres)
	log.Printf("[PIPELINE] countries: %v", enrichedMetadata.Countries)
	log.Printf("[PIPELINE] duration: %d", enrichedMetadata.Duration)
	log.Printf("[PIPELINE] posterURL: %s", finalPosterURL)
	log.Printf("[PIPELINE] originalPosterURL: %s", originalPosterURL)
	log.Printf("[PIPELINE] ===== END METADATA SUMMARY =====")

	// Create movie with all enriched data
	// This is a CRITICAL step - movie MUST be created
	log.Printf("[STAGE] saving movie to MongoDB — title: %s, streamingURL: %s", enrichedMetadata.Title, streamingURL)
	movieResult, err := p.createMovieInDatabaseWithEnrichment(ctx, job, enrichedMetadata, streamingURL, finalPosterURL, originalPosterURL, posterGenerated, finalBackdropURL, originalBackdropURL, backdropGenerated, metadataSource)
	if err != nil {
		return fmt.Errorf("failed to create movie in database: %w", err)
	}
	log.Printf("[STAGE] movie saved to MongoDB")

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

	// ===== TELEGRAM NOTIFICATION =====
	// Send Telegram notification after successful movie creation
	// This is idempotent - if already notified, it will be skipped
	if movieResult != nil {
		if err := p.sendTelegramNotification(ctx, jobID, job, enrichedMetadata, streamingURL, finalPosterURL, movieResult); err != nil {
			// Log error but don't fail the job - movie is already created
			log.Printf("[PIPELINE] WARNING: Telegram notification failed: %v", err)
			log.Printf("[PIPELINE] Movie is created successfully, Telegram failure is non-critical")
		}
	}

	// ===== ASSET VALIDATION =====
	// Verify all assets are stored in the canonical folder
	p.validateAssetStorage(jobID, canonicalFolderName, finalPosterURL, finalBackdropURL, streamingURL)

	// ===== FINAL STATUS UPDATE =====
	// Mark job as completed - this is critical but we log the result
	if err := p.updateStatus(jobID, models.IngestionStatusCompleted, 100); err != nil {
		// Even if status update fails, log success because the work is done
		log.Printf("[PIPELINE] CRITICAL: Failed to update status to COMPLETED: %v", err)
		log.Printf("[PIPELINE] But job processing SUCCEEDED - movie created, poster handled")
		log.Printf("[PIPELINE] Please check database connectivity")
	} else {
		log.Printf("[PIPELINE] Status updated to COMPLETED (100%%)")
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

	// Asset generation status
	log.Printf("[PIPELINE] Poster Generated: %v", posterGenerated)
	log.Printf("[PIPELINE] Poster Gen Status: %s", posterGenStatus)
	log.Printf("[PIPELINE] Localization Attempted: %v", localizationAttempted)
	log.Printf("[PIPELINE] Localization Succeeded: %v", localizationSucceeded)
	log.Printf("[PIPELINE] Backdrop Generated: %v", backdropGenerated)
	log.Printf("[PIPELINE] Backdrop Gen Status: %s", backdropGenStatus)

	// Asset storage verification
	mode = p.config.StorageConfig.Mode
	if mode == "dev" {
		log.Printf("[PIPELINE] Storage Mode: development")
		log.Printf("[PIPELINE] Assets Location: /stream/%s/", canonicalFolderName)
	} else {
		log.Printf("[PIPELINE] Storage Mode: production")
		log.Printf("[PIPELINE] Assets Location: B2/videos/%s/", canonicalFolderName)
	}

	log.Printf("[PIPELINE] Movie ID: %v", movieResult.MovieID)
	log.Printf("[PIPELINE] Movie Code: %s", movieResult.Code)
	log.Printf("[PIPELINE] Movie DB: %s", p.dbName)
	log.Printf("[PIPELINE] Movie Collection: movies")
	log.Printf("[PIPELINE] ===== END JOB RESULT =====")

	log.Printf("[PIPELINE] Job %s completed successfully", jobID)
	return nil
}

// parseMovieDetails calls the parser service to get movie details
// Returns metadata and local_path if downloaded by parser
func (p *Pipeline) parseMovieDetails(job *models.IngestionJob) (*models.ParsedMovieMetadata, string, error) {
	// Defensive check: job must not be nil
	if job == nil {
		return nil, "", fmt.Errorf("job is nil - cannot parse movie details")
	}

	jobID := job.ID.Hex()

	// Defensive check: source must be provided
	if job.Source == "" {
		return nil, "", fmt.Errorf("job source is empty - cannot parse movie details")
	}

	// Defensive check: source ID or detail URL must be provided
	if job.SourceID == "" && job.DetailURL == "" {
		return nil, "", fmt.Errorf("job source_id and detail_url are both empty - cannot parse movie details")
	}

	log.Printf("[PIPELINE] parseMovieDetails: job=%s, source=%s, sourceID=%s", jobID, job.Source, job.SourceID)

	parserEndpoint := fmt.Sprintf("%s/details", p.config.ParserURL)
	params := url.Values{}
	params.Set("source", job.Source)
	params.Set("id", job.SourceID)
	params.Set("url", job.DetailURL)
	params.Set("job_id", jobID)
	parserURL := parserEndpoint + "?" + params.Encode()

	log.Printf("[WORKER] Calling parser /details with job_id=%s", jobID)

	resp, err := p.httpClient.Get(parserURL)
	if err != nil {
		return nil, "", fmt.Errorf("failed to call parser: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, "", fmt.Errorf("parser returned status %d: %s", resp.StatusCode, string(body))
	}

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
		Error             string `json:"error"`
		DownloadError     string `json:"download_error"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("failed to decode parser response: %w", err)
	}

	// Check for parser-level errors
	if result.Error != "" {
		return nil, "", fmt.Errorf("parser error: %s", result.Error)
	}

	// Check for download-specific errors
	if result.DownloadError != "" {
		return nil, "", fmt.Errorf("download failed: %s", result.DownloadError)
	}

	// CRITICAL: Validate parser returned success=true for download
	// Parser contract requires: if success=true, then local_path must be non-empty
	if result.Success && result.LocalPath == "" && result.FilePath == "" {
		log.Printf("[PIPELINE] ERROR: Parser returned success=true but empty local_path!")
		return nil, "", fmt.Errorf("parser contract violation: success=true but local_path is empty. Parser must download the video before returning.")
	}

	// CRITICAL: If parser says download is needed, something is wrong
	// According to contract, parser should download first
	if result.DownloadNeeded {
		log.Printf("[PIPELINE] ERROR: Parser returned download_needed=true - contract violation!")
		return nil, "", fmt.Errorf("parser contract violation: download_needed=true. Parser must download the video, not worker.")
	}

	// Defensive check: ensure we got valid metadata
	if result.Title == "" {
		return nil, "", fmt.Errorf("parser returned empty title")
	}

	// NEW: Get local_path from parser response (prefer LocalPath, fallback to FilePath)
	localPath := result.LocalPath
	if localPath == "" {
		localPath = result.FilePath
	}

	// CRITICAL: Validate that the downloaded file actually exists
	if localPath != "" {
		if _, err := os.Stat(localPath); err != nil {
			log.Printf("[PIPELINE] ERROR: Parser returned local_path but file does not exist: %s", localPath)
			return nil, "", fmt.Errorf("parser returned local_path but file does not exist: %s", localPath)
		}

		// Validate file size > 0
		fileInfo, _ := os.Stat(localPath)
		if fileInfo != nil && fileInfo.Size() == 0 {
			log.Printf("[PIPELINE] ERROR: Parser returned local_path but file is empty: %s", localPath)
			return nil, "", fmt.Errorf("parser returned local_path but file is empty: %s", localPath)
		}

		log.Printf("[PIPELINE] Parser provided local_path: %s (size: %d bytes)", localPath, fileInfo.Size())
	}

	log.Printf("[PIPELINE] parseMovieDetails completed: title=%s, year=%d, video_urls=%d, local_path=%s",
		result.Title, result.Year, len(result.VideoURLs), localPath)

	p.log(jobID, fmt.Sprintf("Parsed: %s (%d)", result.Title, result.Year), "info")

	// NEW: Return local_path so caller can update the job
	return &result.ParsedMovieMetadata, localPath, nil
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

// processVideo processes the video with FFmpeg (watermark removal + adaptive HLS)
// This function now generates multi-bitrate adaptive HLS with a master playlist
func (p *Pipeline) processVideo(job *models.IngestionJob, inputPath string, canonicalFolderName string) (string, error) {
	// Defensive check: job must not be nil
	if job == nil {
		return "", fmt.Errorf("job is nil - cannot process video")
	}

	jobID := job.ID.Hex()

	// Defensive check: metadata must not be nil
	metadata := job.Metadata
	if metadata == nil {
		log.Printf("[PIPELINE] WARNING: job metadata is nil, using fallback values")
		// Fallback: use job title if available
		if job.Title != "" {
			log.Printf("[PIPELINE] Using job.Title as fallback: %s", job.Title)
		}
	}

	// Create safe variables for metadata fields to avoid nil pointer access
	title := "video"
	year := 0
	if metadata != nil {
		title = metadata.Title
		year = metadata.Year
	}
	// Use job.Title as fallback if title is still empty
	if title == "" && job.Title != "" {
		title = job.Title
	}

	log.Printf("[PIPELINE] Using title=%s, year=%d for processing", title, year)

	// Defensive check: inputPath must not be empty
	if inputPath == "" {
		return "", fmt.Errorf("input path is empty - cannot process video")
	}

	// Defensive check: input file must exist
	if _, err := os.Stat(inputPath); os.IsNotExist(err) {
		return "", fmt.Errorf("input file does not exist: %s", inputPath)
	}

	log.Printf("[PIPELINE] processVideo: job=%s, inputPath=%s", jobID, inputPath)

	// FINAL VALIDATION: Double-check the file is valid before running ffmpeg
	// This is a second line of defense in case download validation passed but file changed
	if err := p.validateMediaFile(inputPath, "pre-processing-check"); err != nil {
		return "", fmt.Errorf("pre-processing validation failed - file is not a valid media file: %w", err)
	}

	// CRITICAL: Use the canonical folder name passed from processJobWithRecovery
	// This ensures ALL assets (HLS, poster, backdrop) are in the SAME folder
	folderName := canonicalFolderName
	log.Printf("[WORKER] Using canonical folder name: %s", folderName)

	log.Printf("[WORKER] Using input file: %s", inputPath)
	log.Printf("[WORKER] Generating adaptive HLS in: worker/readyvideo/%s", folderName)

	// CRITICAL: Build uploads path from actual working directory
	// This prevents "mkdir worker: not a directory" error
	baseDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}
	log.Printf("[WORKER] baseDir=%s", baseDir)

	// Create readyvideo directory for HLS processing output
	readyVideoDir := filepath.Join(baseDir, "readyvideo")
	if err := os.MkdirAll(readyVideoDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create readyvideo dir: %w", err)
	}

	outputDir := filepath.Join(readyVideoDir, folderName)
	log.Printf("[WORKER] readyDir=%s", outputDir)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output dir: %w", err)
	}

	log.Printf("[WORKER] Creating adaptive HLS output in %s", outputDir)
	log.Printf("[WORKER] Generated adaptive HLS in: %s", outputDir)

	// Log where HLS is being generated
	log.Printf("[WORKER] Generated adaptive HLS in: %s", outputDir)

	// =============================================================================
	// ADAPTIVE BITRATE HLS PIPELINE
	// =============================================================================
	// This new pipeline generates multi-bitrate adaptive HLS with a master playlist.
	//
	// Pipeline steps:
	// 1. Create base video with delogo + logo filters applied (one-time processing)
	// 2. Generate multiple quality renditions from the base video
	// 3. Create master.m3u8 playlist referencing all variants
	//
	// Renditions generated:
	// - 360p -> ~800k video, 480x360
	// - 480p -> ~1400k video, 854x480
	// - 720p -> ~2800k video, 1280x720
	// - 1080p -> ~5000k video, 1920x1080 (if input supports it)
	//
	// Key features:
	// - Consistent GOP across all renditions (2-second keyframe interval)
	// - -sc_threshold 0 to disable scene cut detection
	// - 6-second HLS segments
	// - VOD playlist type
	// - Master playlist for quality switching
	// =============================================================================

	p.log(jobID, fmt.Sprintf("%s+adaptive-hls", folderName), "info")
	if err := p.updateStatus(jobID, models.IngestionStatusProcessing, 35); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status to processing: %v", err)
	}
	log.Printf("[WORKER] ADAPTIVE HLS PIPELINE: Generating multi-bitrate HLS with master playlist")

	// Cut first 10 seconds from every imported video by default
	const defaultCutSeconds = 10

	// Progress callback to update backend
	var lastProgress int = 0
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

	// Dynamic watermark removal — runs before HLS encoding
	// Falls back to original inputPath on any error
	hlsInput := inputPath
	if p.watermarkService != nil {
		if err := p.updateStatus(jobID, models.IngestionStatusRemovingWatermark, 33); err != nil {
			log.Printf("[PIPELINE] WARNING: Failed to update status to removing_watermark: %v", err)
		}
		cleanPath, wmErr := p.removeWatermarkDynamic(context.Background(), job, inputPath)
		if wmErr != nil {
			log.Printf("[PIPELINE] Dynamic watermark removal failed, using original: %v", wmErr)
		} else {
			hlsInput = cleanPath
		}
	}

	// Call the adaptive HLS generation function
	masterPath, err := p.processAdaptiveHLS(jobID, hlsInput, outputDir, defaultCutSeconds, progressCallback)
	if err != nil {
		return "", fmt.Errorf("adaptive HLS processing failed: %w", err)
	}

	// Verify the output directory has files
	files, err := os.ReadDir(outputDir)
	if err != nil {
		return "", fmt.Errorf("failed to read output dir: %w", err)
	}
	if len(files) == 0 {
		return "", fmt.Errorf("no output files generated")
	}

	log.Printf("[PIPELINE] processVideo completed: %d files/dirs in %s", len(files), outputDir)
	log.Printf("[WORKER] Adaptive HLS master playlist created: %s", masterPath)
	p.log(jobID, fmt.Sprintf("%s+processed", folderName), "info")

	return outputDir, nil
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
func (p *Pipeline) uploadProcessedFiles(job *models.IngestionJob, hlsDir string, inputPath string, canonicalFolderName string) (string, error) {
	// Defensive check: job must not be nil
	if job == nil {
		return "", fmt.Errorf("job is nil - cannot upload files")
	}

	jobID := job.ID.Hex()

	// Defensive check: metadata must not be nil
	metadata := job.Metadata
	if metadata == nil {
		log.Printf("[PIPELINE] WARNING: job metadata is nil for upload, using fallback values")
	}

	// Defensive check: hlsDir must not be empty
	if hlsDir == "" {
		return "", fmt.Errorf("hls directory is empty - nothing to upload")
	}

	// Defensive check: hlsDir must exist
	if _, err := os.Stat(hlsDir); os.IsNotExist(err) {
		return "", fmt.Errorf("hls directory does not exist: %s", hlsDir)
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

	if mode == "prod" || mode == "production" {
		// === PRODUCTION MODE: Upload to B2/CDN ===
		log.Printf("[WORKER] MODE=production")
		log.Printf("[WORKER] Generated adaptive HLS in: %s", hlsDir)
		log.Printf("[WORKER] Uploading adaptive HLS folder from readyvideo/%s to B2", folderName)

		// Use the adaptive HLS upload function which handles recursive directory structure
		var err error
		streamingURL, err = p.uploadAdaptiveHLSFiles(job, hlsDir, folderName)
		if err != nil {
			return "", fmt.Errorf("failed to upload adaptive HLS files: %w", err)
		}

		if streamingURL == "" {
			return "", fmt.Errorf("no master playlist found")
		}

		log.Printf("[WORKER] Final CDN master playlist URL: %s", streamingURL)
		log.Printf("[PIPELINE] uploadProcessedFiles completed: streamingURL=%s", streamingURL)

		// Cleanup: Remove temporary readyvideo folder after successful upload
		log.Printf("[WORKER] Removing temporary readyvideo folder: %s", hlsDir)
		if err := os.RemoveAll(hlsDir); err != nil {
			log.Printf("[WORKER] WARNING: Failed to remove readyvideo folder: %v", err)
		} else {
			log.Printf("[WORKER] Removed temporary readyvideo folder: %s", hlsDir)
		}

	} else {
		// === DEVELOPMENT MODE: Copy to worker/uploads/movies/<slug>/ ===
		log.Printf("[WORKER] MODE=development")

		// Use the adaptive HLS upload function which handles recursive directory structure
		var err error
		streamingURL, err = p.uploadAdaptiveHLSFiles(job, hlsDir, folderName)
		if err != nil {
			return "", fmt.Errorf("failed to copy adaptive HLS files: %w", err)
		}

		if streamingURL == "" {
			return "", fmt.Errorf("no master playlist found")
		}

		log.Printf("[WORKER] Development master playlist URL: %s", streamingURL)
		log.Printf("[PIPELINE] uploadProcessedFiles completed: streamingURL=%s", streamingURL)
	}

	// Cleanup: Delete original downloaded file after successful HLS finalization
	if inputPath != "" {
		log.Printf("[WORKER] Deleting original downloaded mp4: %s", inputPath)
		if err := os.Remove(inputPath); err != nil {
			log.Printf("[WORKER] WARNING: Failed to delete original file: %v", err)
		} else {
			log.Printf("[WORKER] Deleted original downloaded mp4: %s", inputPath)
		}
	}

	return streamingURL, nil
}

// createMovieInDatabase creates the movie entry in the main database
func (p *Pipeline) createMovieInDatabase(job *models.IngestionJob, metadata *models.ParsedMovieMetadata, streamingURL string) (interface{}, error) {
	// This would call the backend API to create a movie
	// For now, return a placeholder
	p.log(job.ID.Hex(), "Movie created in database", "info")

	// Return a dummy object ID
	return "placeholder-movie-id", nil
}

// createMovieInDatabaseWithEnrichment creates movie with enriched metadata
// MovieCreationResult holds the result of movie creation
type MovieCreationResult struct {
	MovieID      interface{}
	Code         string
	Slug         string
	DisplayTitle string
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
	code, err := p.getNextMovieCode(ctx)
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

	// For description, prefer Uzbek if available
	displayDescription := enrichedMetadata.Description
	if enrichedMetadata.DescriptionUz != "" {
		displayDescription = enrichedMetadata.DescriptionUz
		log.Printf("[PIPELINE] Using Uzbek description (length: %d)", len(displayDescription))
	}

	movieDoc := bson.M{
		"code":                  code,
		"slug":                  displaySlug,                    // Use Uzbek slug for consistency
		"title":                 displayTitle,                   // Uzbek title for user-facing display
		"original_title":        enrichedMetadata.OriginalTitle, // Keep original English title
		"original_description":  enrichedMetadata.Description,   // Keep original English description
		"description":           displayDescription,             // Uzbek description for user-facing display
		"year":                  enrichedMetadata.Year,
		"genre":                 enrichedMetadata.Genres,                          // English genres for search/filter
		"genres_uz":             enrichedMetadata.GenresUz,                        // Uzbek genres for display
		"country":               strings.Join(enrichedMetadata.Countries, ", "),   // English countries for storage
		"countries_uz":          strings.Join(enrichedMetadata.CountriesUz, ", "), // Uzbek countries for display
		"duration":              enrichedMetadata.Duration,
		"quality":               enrichedMetadata.Quality,
		"translation":           enrichedMetadata.Translation,
		"poster_url":            posterURL,           // Generated/localized poster
		"original_poster_url":   originalPosterURL,   // Original TMDB poster
		"backdrop_url":          backdropURL,         // Generated/localized backdrop
		"original_backdrop_url": originalBackdropURL, // Original TMDB backdrop
		"video_url":             streamingURL,
		"source_type":           "ingestion",
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

	// FIRST: Check if movie with this slug already exists
	existingMovie := bson.M{"slug": displaySlug}
	existingCount := int64(0)

	countResult, countErr := p.movieCol.CountDocuments(ctx, existingMovie)
	if countErr == nil {
		existingCount = countResult
		log.Printf("[PIPELINE] Existing movie check: slug=%s, found=%d", displaySlug, existingCount)
	} else {
		log.Printf("[PIPELINE] Existing movie check failed: %v", countErr)
	}

	// Use upsert to handle duplicates - this is safer than raw InsertOne
	// Upsert will INSERT new movie OR UPDATE existing movie with same slug
	filter := bson.M{"slug": displaySlug}
	update := bson.M{"$set": movieDoc}
	upsert := true

	result, err := p.movieCol.UpdateOne(ctx, filter, update, options.Update().SetUpsert(upsert))
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
// getNextMovieCode gets the next sequential movie code from MongoDB atomic counter.
// Skips codes that already exist in the movies collection to handle pre-existing data.
// Returns zero-padded string (e.g., "0001", "0008", "0100").
func (p *Pipeline) getNextMovieCode(ctx context.Context) (string, error) {
	if p.movieCol == nil {
		return "", fmt.Errorf("movie collection not initialized")
	}

	countersCol := p.movieCol.Database().Collection("counters")
	filter := bson.M{"_id": "movieCode"}
	update := bson.M{"$inc": bson.M{"seq": 1}}
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)

	var result struct {
		ID  string `bson:"_id"`
		Seq int    `bson:"seq"`
	}

	const maxRetries = 100
	for i := 0; i < maxRetries; i++ {
		if err := countersCol.FindOneAndUpdate(ctx, filter, update, opts).Decode(&result); err != nil {
			return "", fmt.Errorf("failed to increment movie code counter: %w", err)
		}

		code := fmt.Sprintf("%04d", result.Seq)

		// Check if this code is already taken by an existing movie
		count, err := p.movieCol.CountDocuments(ctx, bson.M{"code": code})
		if err != nil {
			return "", fmt.Errorf("failed to check code uniqueness: %w", err)
		}
		if count == 0 {
			log.Printf("[PIPELINE] Movie code counter: seq=%d, formatted=%s", result.Seq, code)
			return code, nil
		}
		log.Printf("[PIPELINE] Movie code %s already exists, skipping to next", code)
	}

	return "", fmt.Errorf("failed to find unique movie code after %d attempts", maxRetries)
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

	log.Printf("[TELEGRAM] Backend response status: %d, body: %s", resp.StatusCode, string(body))

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

// CRITICAL: Only deletes .tmp files, never deletes .mp4 files
func (p *Pipeline) cleanupFile(path string) {
	if path == "" {
		return
	}

	// Safety check: Never delete .mp4 files (final output from parser)
	if strings.HasSuffix(path, ".mp4") {
		log.Printf("[CLEANUP] SKIP: Not deleting .mp4 file: %s", path)
		return
	}

	// Only delete temporary files
	if strings.HasSuffix(path, ".tmp") || strings.HasPrefix(filepath.Base(path), "part_") {
		log.Printf("[CLEANUP] Deleting temp file: %s", path)
		os.Remove(path)
		return
	}

	log.Printf("[CLEANUP] SKIP: Not a temp file, keeping: %s", path)
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
func (p *Pipeline) downloadFile(url, localPath string) error {
	resp, err := p.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if err := os.WriteFile(localPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	log.Printf("[PIPELINE] Downloaded file to: %s", localPath)
	return nil
}

// sanitizeFilename creates a safe filename from a movie title
func sanitizeFilename(title string) string {
	// Replace spaces with underscores and remove unsafe characters
	s := strings.ToLower(title)
	s = strings.ReplaceAll(s, " ", "+")
	// Keep only alphanumeric and some safe chars
	var result strings.Builder
	for _, c := range s {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '+' || c == '-' || c == '_' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// createMovieSlug creates a safe filesystem slug from movie title
// Examples:
//   - "Forsaj 8" -> "forsaj8"
//   - "Spider-Man: No Way Home" -> "spidermannowayhome"
// junkPattern matches quality tags, episode markers, and filler words common in
// Uzbek source titles: "480p", "1080p", "1-qism", "+41", "seriali", etc.
var junkPattern = regexp.MustCompile(
	`(?i)\b(` +
		`\d{3,4}p` + // 480p 720p 1080p
		`|4k` +
		`|hd|fhd` +
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

	// Remove unsafe characters and normalize spaces
	var result strings.Builder
	for _, c := range s {
		// Allow alphanumeric, hyphens, and underscores
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			result.WriteRune(c)
		} else if c == ' ' || c == ':' || c == '.' || c == ',' || c == '!' || c == '?' {
			// Skip special chars, just remove them
			// e.g., "Spider-Man: No Way Home" -> "spidermannotwayhome"
		}
	}

	return result.String()
}
