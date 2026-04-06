package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
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
		ParserURL:   getEnv("PARSER_URL", "http://localhost:8082"),
		TempDir:     getEnv("TEMP_DIR", "./tmp"),
		TMDBAPIKey:  getEnv("TMDB_API_KEY", ""),                          // TMDB API key for metadata enrichment
		DB:          db,                                                  // Pass database for movie insertion
		BackendURL:  getEnv("BACKEND_BASE_URL", "http://localhost:8080"), // Backend API URL for Telegram notifications
		WorkerToken: getEnv("WORKER_TOKEN", ""),                          // Token for worker-to-backend authentication
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

	// Initialize HTTP handler for direct video processing
	processHandler := NewProcessHandler(outputDir, tempDir)

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

	// Main worker loop - uses atomic job claiming for multi-worker safety
	go func() {
		pollInterval := 5 * time.Second
		ticker := time.NewTicker(pollInterval)
		defer ticker.Stop()

		log.Println("Worker started - polling for jobs every 5 seconds")

		for {
			select {
			case <-workerCtx.Done():
				log.Println("Worker stopped")
				return
			case <-ticker.C:
				// Wrap job processing in panic recovery to ensure worker stability
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[PANIC RECOVERY] Worker loop panicked: %v", r)
							// Continue the loop - don't let panic kill the worker
						}
					}()

					// Atomically claim the next pending job
					// This ensures only one worker processes each job
					job, err := jobRepo.ClaimNextJob(workerCtx)
					if err != nil {
						log.Printf("Error claiming job: %v", err)
						return
					}

					// If no download job available, try claiming a processing job
					// This handles jobs where steps.download=true but steps.process=false
					if job == nil {
						job, err = jobRepo.ClaimNextProcessingJob(workerCtx)
						if err != nil {
							log.Printf("Error claiming processing job: %v", err)
							return
						}
						if job != nil {
							log.Printf("[WORKER] Claimed processing job (steps.download=true): %s", job.ID.Hex())
						}
					}

					if job == nil {
						// No jobs available
						return
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

// getStorageMode returns the storage mode based on ENV variable
// "development" -> "dev", "production" -> "prod"
// Falls back to "dev" for any other value
func getStorageMode() string {
	env := getEnv("ENV", "development")
	switch env {
	case "production":
		return "prod"
	case "development":
		fallthrough
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
