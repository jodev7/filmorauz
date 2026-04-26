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
	processConcurrency := getEnvAsInt("PROCESS_CONCURRENCY", 3)
	if processConcurrency < 1 {
		processConcurrency = 1
	}

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

	// Stale-job protection - resets stuck jobs back into the appropriate queue.
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()

		// Run once at startup so restarting the worker recovers leftover stuck jobs.
		if n, err := jobRepo.RecoverStaleJobs(workerCtx); err == nil && n > 0 {
			log.Printf("[QUEUE] recovered stale jobs count=%d", n)
		}

		for {
			select {
			case <-workerCtx.Done():
				return
			case <-ticker.C:
				if _, err := jobRepo.RecoverStaleJobs(workerCtx); err != nil {
					log.Printf("[QUEUE] stale recovery error: %v", err)
				}
			}
		}
	}()

	// Fixed-size processing pool - each goroutine owns one processing slot.
	log.Printf("[QUEUE] processing worker started concurrency=%d", processConcurrency)
	for workerIndex := 0; workerIndex < processConcurrency; workerIndex++ {
		go func(slot int) {
			pollInterval := 5 * time.Second
			log.Printf("[QUEUE] processing slot started index=%d/%d", slot+1, processConcurrency)

			for {
				select {
				case <-workerCtx.Done():
					return
				default:
				}

				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[PANIC RECOVERY] Worker slot %d panicked: %v", slot+1, r)
						}
					}()

					job, err := jobRepo.ClaimNextProcessingJob(workerCtx)
					if err != nil {
						log.Printf("[QUEUE] processing claim error slot=%d: %v", slot+1, err)
						time.Sleep(pollInterval)
						return
					}
					if job == nil {
						log.Printf("[QUEUE] no ready_to_process jobs")
						time.Sleep(pollInterval)
						return
					}

					title := job.Title
					if title == "" && job.Metadata != nil {
						title = job.Metadata.Title
					}
					if title == "" {
						title = job.SourceID
					}

					log.Printf("[QUEUE] claimed job id=%s queue=processing slot=%d title=%s source=%s", job.ID.Hex(), slot+1, title, job.Source)
					safeProcessJob(pipe, workerCtx, job)
				}()
			}
		}(workerIndex)
	}

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
