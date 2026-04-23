package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/filmorauz/worker/models"
	"github.com/filmorauz/worker/pipeline"
	"github.com/filmorauz/worker/repositories"
	"github.com/filmorauz/worker/storage"
	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// InitializeLocalStorageURLs sets up the base URLs for local storage based on environment
func InitializeLocalStorageURLs() {
	baseURL := getEnv("BASE_URL", "http://localhost:8080")
	storage.SetUploadURL(baseURL)
	log.Printf("[CONFIG] LocalStorageUploadURL initialized to: %s", baseURL)
}

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Println("Warning: .env file not found, using environment variables")
	}

	// Initialize local storage URLs for development mode
	InitializeLocalStorageURLs()

	// Connect to MongoDB
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	mongoURI := getEnv("MONGO_URI", "mongodb://localhost:27017")
	mongoDB := getEnv("MONGO_DB", "filmorauz")

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(mongoURI))
	if err != nil {
		log.Fatalf("Failed to connect to MongoDB: %v", err)
	}
	defer client.Disconnect(context.Background())

	// Ping MongoDB
	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("Failed to ping MongoDB: %v", err)
	}
	log.Println("Connected to MongoDB")

	db := client.Database(mongoDB)

	// Initialize repositories
	jobRepo := repositories.NewJobRepository(db)

	// Pipeline configuration
	pipeConfig := pipeline.Config{
		ParserURL:              getEnv("PARSER_URL", "http://localhost:8082"),
		TempDir:                getEnv("TEMP_DIR", "./tmp"),
		TMDBAPIKey:             getEnv("TMDB_API_KEY", ""),                          // TMDB API key for metadata enrichment
		DB:                     db,                                                  // Pass database for movie insertion
		BackendURL:             getEnv("BACKEND_BASE_URL", "http://localhost:8080"), // Backend API URL for Telegram notifications
		WorkerToken:            getEnv("WORKER_TOKEN", ""),                          // Token for worker-to-backend authentication
		MaxRenditionConcurrent: getEnvAsInt("MAX_RENDITION_CONCURRENT", 3),          // Max parallel FFmpeg processes
		SegmentUploadWorkers:   getEnvAsInt("SEGMENT_UPLOAD_WORKERS", 20),           // Concurrent segment uploads per rendition
		SegmentUploadRetries:   getEnvAsInt("SEGMENT_UPLOAD_RETRIES", 5),            // Max retries per segment (increased from 3→5)
		SegmentDuration:        getEnvAsInt("SEGMENT_DURATION", 6),                  // HLS segment duration in seconds (default 6s - production safe)
		StorageConfig: storage.Config{
			Mode:       getStorageMode(), // Uses ENV variable: development -> dev, production -> prod
			LocalPath:  getEnv("LOCAL_STORAGE_PATH", "./uploads"),
			B2Bucket:   getEnv("B2_BUCKET", ""),
			B2Endpoint: getEnv("B2_ENDPOINT", ""),
			B2KeyID:    getEnv("B2_KEY_ID", ""),
			B2AppKey:   getEnv("B2_APP_KEY", ""),
			CDNBaseURL: getEnv("CDN_BASE_URL", ""),
			BaseURL:    getEnv("BASE_URL", "http://localhost:8080"), // Base URL for development mode
		},
	}

	// Log configuration
	log.Printf("[CONFIG] Pipeline initialized (watermark removal and AI poster generation disabled)")
	log.Printf("[CONFIG] Storage mode: %s", getStorageMode())

	// Initialize pipeline
	pipe, err := pipeline.NewPipeline(pipeConfig, jobRepo)
	if err != nil {
		log.Fatalf("Failed to create pipeline: %v", err)
	}

	// Create temp and output directories
	tempDir := getEnv("TEMP_DIR", "./tmp")
	outputDir := getEnv("OUTPUT_DIR", "./processed")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Fatalf("Failed to create temp directory: %v", err)
	}
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	// Initialize B2 storage for profile image uploads (prod mode only)
	var b2Storage storage.Storage
	storageMode := getStorageMode()
	b2Bucket := getEnv("B2_BUCKET", "")
	b2Endpoint := getEnv("B2_ENDPOINT", "")
	b2KeyID := getEnv("B2_KEY_ID", "")
	b2AppKey := getEnv("B2_APP_KEY", "")
	cdnBaseURL := getEnv("CDN_BASE_URL", "")

	log.Printf("[CONFIG] === Storage Configuration ===")
	log.Printf("[CONFIG] Storage mode: %s (ENV=%s)", storageMode, getEnv("ENV", getEnv("MODE", "unset")))

	if storageMode == "prod" {
		log.Printf("[CONFIG] B2 bucket: %s", b2Bucket)
		log.Printf("[CONFIG] B2 endpoint: %s", b2Endpoint)
		log.Printf("[CONFIG] B2 key ID: %s", b2KeyID)
		log.Printf("[CONFIG] B2 app key: %s", b2AppKey)
		log.Printf("[CONFIG] CDN base URL: %s", cdnBaseURL)

		// Validate required B2 config
		if b2Bucket == "" || b2KeyID == "" || b2AppKey == "" {
			log.Fatalf("[CONFIG] ERROR: B2_BUCKET, B2_KEY_ID, and B2_APP_KEY are required in production mode")
		}

		var err error
		b2Storage, err = storage.NewStorage(storage.Config{
			Mode:       "prod",
			B2Bucket:   b2Bucket,
			B2Endpoint: b2Endpoint,
			B2KeyID:    b2KeyID,
			B2AppKey:   b2AppKey,
			CDNBaseURL: cdnBaseURL,
		})
		if err != nil {
			log.Fatalf("[CONFIG] ERROR: Failed to initialize B2 storage: %v", err)
		}
		log.Printf("[CONFIG] B2 storage initialized successfully for profile image uploads")
	} else {
		log.Printf("[CONFIG] Development mode: using local storage (B2 config not required)")
	}

	// Initialize HTTP handler for direct video processing
	processHandler := NewProcessHandler(outputDir, tempDir, b2Storage)

	// Start HTTP server for /process endpoint
	httpPort := getEnv("WORKER_HTTP_PORT", "8083")
	httpServer := &http.Server{
		Addr:    ":" + httpPort,
		Handler: processHandler,
	}

	go func() {
		log.Printf("Starting HTTP server on port %s for /process endpoint", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	// Start worker
	log.Println("Starting ingestion worker...")

	// Create worker context that can be cancelled
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutdown signal received, stopping worker...")
		workerCancel()
	}()

	// Stale-job protection - marks jobs stuck with no update for >180 minutes as failed.
	// This prevents jobs from being stuck forever if the worker crashes mid-upload
	// or if a non-retryable pipeline error leaves the status field in a bad state.
	// Note: FFmpeg processing can take 30-60+ minutes, so we use a longer threshold.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// Run once at startup so restarting the worker cleans up leftover stuck jobs
		if n, err := jobRepo.FailStaleProcessingJobs(workerCtx); err == nil && n > 0 {
			log.Printf("[WORKER] Startup stale-job sweep: failed %d jobs with no update for >180 minutes", n)
		}

		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if _, err := jobRepo.FailStaleProcessingJobs(workerCtx); err != nil {
					log.Printf("[WORKER] FailStaleProcessingJobs error: %v", err)
				}
			}
		}
	}()

	// Main worker loop - uses atomic job claiming for multi-worker safety
	go func() {
		pollInterval := 5 * time.Second
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		log.Println("Worker started - polling for jobs every 5 seconds")

		// Debug: Log pending job counts at startup
		if count, err := jobRepo.CountPendingJobs(workerCtx); err == nil {
			log.Printf("[WORKER] Startup: found %d pending jobs", count)
		} else {
			log.Printf("[WORKER] Startup: could not count pending jobs: %v", err)
		}

		for {
			select {
			case <-workerCtx.Done():
				log.Println("Worker stopped")
				return
			case <-ticker.C:
				// Debug: log pending job counts every poll
				if count, err := jobRepo.CountPendingJobs(workerCtx); err == nil && count > 0 {
					log.Printf("[WORKER] Poll: %d pending jobs found", count)
				}

				// Wrap job processing in panic recovery to ensure worker stability
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[PANIC RECOVERY] Worker loop panicked: %v", r)
							// Continue the loop - don't let panic kill the worker
						}
					}()

					// Poll for jobs - process all available jobs in this tick
					for {
						// Atomically claim the next pending job
						// This ensures only one worker processes each job
						log.Printf("[WORKER] Polling for pending jobs...")
						job, err := jobRepo.ClaimNextJob(workerCtx)
						if err != nil {
							log.Printf("[WORKER] Error claiming job: %v", err)
							break
						}

						// If no download job available, try claiming a processing job
						// This handles jobs where steps.download=true but steps.process=false
						if job == nil {
							log.Printf("[WORKER] No download jobs, polling for processing jobs...")
							job, err = jobRepo.ClaimNextProcessingJob(workerCtx)
							if err != nil {
								log.Printf("[WORKER] Error claiming processing job: %v", err)
								break
							}
							if job != nil {
								log.Printf("[WORKER] Claimed processing job (steps.download=true): %s", job.ID.Hex())
							}
						} else {
							log.Printf("[WORKER] Claimed download job: %s (steps.download will be set after parser)", job.ID.Hex())
						}

						if job == nil {
							// No jobs available, stop polling this tick
							log.Printf("[WORKER] No pending jobs found")
							break
						}

						title := job.Title
						if title == "" && job.Metadata != nil {
							title = job.Metadata.Title
						}
						if title == "" {
							title = job.SourceID
						}

						// DEBUG: Log job details to diagnose metadata loading issue
						log.Printf("[WORKER] Claimed job %s: %s from %s", job.ID.Hex(), title, job.Source)
						log.Printf("[WORKER] job.local_path=%s", job.LocalPath)
						log.Printf("[WORKER] metadata nil? %v", job.Metadata == nil)
						if job.Metadata != nil {
							log.Printf("[WORKER] metadata title=%s", job.Metadata.Title)
						}

						// Process the job with panic recovery
						// This ensures a single job failure doesn't crash the worker
						safeProcessJob(pipe, workerCtx, job)
					} // end for loop
				}()
			}
		}
	}()

	log.Println("Worker is running. Press Ctrl+C to stop.")

	// Wait for shutdown
	<-workerCtx.Done()
	log.Println("Worker shutdown complete")
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

func getEnvAsInt(key string, defaultValue int) int {
	if value, exists := os.LookupEnv(key); exists {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getStorageMode returns the storage mode based on ENV or MODE variable
// "production"/"prod" -> "prod", anything else -> "dev"
func getStorageMode() string {
	// Check both ENV and MODE for flexibility
	env := getEnv("ENV", getEnv("MODE", "development"))
	switch strings.ToLower(env) {
	case "production", "prod":
		return "prod"
	default:
		return "dev"
	}
}

// safeProcessJob wraps job processing with panic recovery
// This ensures the worker continues running even if a job causes a panic
func safeProcessJob(pipe *pipeline.Pipeline, ctx context.Context, job *models.IngestionJob) {
	defer func() {
		if r := recover(); r != nil {
			jobID := job.ID.Hex()
			log.Printf("[PANIC RECOVERY] Job %s caused panic: %v", jobID, r)
			// Mark job as failed due to panic
			// Note: We can't access jobRepo directly here, but the pipeline's
			// ProcessJob already handles marking jobs as failed
			log.Printf("Job %s failed due to panic - will be retried", jobID)
		}
	}()

	// Process the job
	if err := pipe.ProcessJob(ctx, job); err != nil {
		log.Printf("Job %s failed: %v", job.ID.Hex(), err)
	} else {
		log.Printf("Job %s completed successfully", job.ID.Hex())
	}
}
