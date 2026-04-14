package repositories

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// JobRepository handles ingestion job persistence
type JobRepository struct {
	collection *mongo.Collection
}

// GetCollection returns the MongoDB collection for direct queries
func (r *JobRepository) GetCollection() *mongo.Collection {
	return r.collection
}

// NewJobRepository creates a new job repository
func NewJobRepository(db *mongo.Database) *JobRepository {
	log.Printf("[JOB REPO] Database name: %s", db.Name())
	log.Printf("[JOB REPO] Using collection: ingestion_jobs")
	collection := db.Collection("ingestion_jobs")

	// Create indexes
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Index on status for querying pending jobs
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "status", Value: 1},
			{Key: "created_at", Value: 1},
		},
	})

	// Unique index on source + source_id to prevent duplicates
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "source", Value: 1},
			{Key: "source_id", Value: 1},
		},
		Options: options.Index().SetUnique(true),
	})

	return &JobRepository{
		collection: collection,
	}
}

// Create creates a new ingestion job
func (r *JobRepository) Create(ctx context.Context, job *models.IngestionJob) error {
	job.ID = primitive.NewObjectID()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	job.Status = models.IngestionStatusPending
	job.Progress = 0
	job.Steps = models.JobSteps{}
	job.Logs = []models.IngestionLog{}
	job.RetryCount = 0

	_, err := r.collection.InsertOne(ctx, job)
	return err
}

// GetByID retrieves a job by ID
func (r *JobRepository) GetByID(ctx context.Context, id string) (*models.IngestionJob, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid id")
	}

	var job models.IngestionJob
	err = r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&job)
	if err != nil {
		return nil, err
	}

	return &job, nil
}

// GetPendingJobs retrieves pending jobs that are ready to process (non-atomic)
// Use ClaimNextJob for atomic claiming in multi-worker deployments
func (r *JobRepository) GetPendingJobs(ctx context.Context, limit int) ([]*models.IngestionJob, error) {
	filter := bson.M{
		"status": bson.M{
			"$in": []models.IngestionStatus{
				models.IngestionStatusPending,
				models.IngestionStatusFailed,
			},
		},
		"retry_count": bson.M{
			"$lt": 3, // Max 3 retries
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*models.IngestionJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

// ClaimNextJob atomically claims the next pending job
// This uses FindOneAndUpdate to ensure only one worker can claim a job
// Returns nil if no jobs are available
// Note: Only claims jobs where steps.download is NOT true (download not complete)
func (r *JobRepository) ClaimNextJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status": models.IngestionStatusPending,
		"retry_count": bson.M{
			"$lt": 3, // Max 3 retries
		},
		// Only claim jobs where download is NOT complete - skip to processing
		// This ensures worker doesn't re-run parser for jobs already downloaded
		"steps.download": bson.M{
			"$ne": true,
		},
	}

	update := bson.M{
		"$set": bson.M{
			"status":     models.IngestionStatusProcessing,
			"updated_at": time.Now(),
		},
	}

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	var job models.IngestionJob
	err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // No jobs available
		}
		return nil, err
	}

	return &job, nil
}

// ClaimNextProcessingJob atomically claims a job ready for ffmpeg processing
// This is called when steps.download=true but steps.process=false
func (r *JobRepository) ClaimNextProcessingJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status":         models.IngestionStatusPending,
		"retry_count":    bson.M{"$lt": 3},
		"steps.download": true,
		"steps.process":  bson.M{"$ne": true},
	}

	update := bson.M{
		"$set": bson.M{
			"status":     models.IngestionStatusProcessing,
			"stage":      "process",
			"updated_at": time.Now(),
		},
	}

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	var job models.IngestionJob
	err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}

	return &job, nil
}

// UpdateStatus updates the status of a job
func (r *JobRepository) UpdateStatus(ctx context.Context, id string, status models.IngestionStatus, progress int) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	update := bson.M{
		"$set": bson.M{
			"status":     status,
			"progress":   progress,
			"updated_at": time.Now(),
		},
	}

	if status == models.IngestionStatusCompleted || status == models.IngestionStatusFailed {
		completedAt := time.Now()
		update["$set"].(bson.M)["completed_at"] = completedAt
	}

	_, err = r.collection.UpdateByID(ctx, objID, update)
	return err
}

// AddLog adds a log entry to a job
func (r *JobRepository) AddLog(ctx context.Context, id string, message string, level string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	log := models.IngestionLog{
		Timestamp: time.Now(),
		Message:   message,
		Level:     level,
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$push": bson.M{"logs": log},
		"$set":  bson.M{"updated_at": time.Now()},
	})

	return err
}

// UpdateMetadata updates the metadata of a job
func (r *JobRepository) UpdateMetadata(ctx context.Context, id string, metadata *models.ParsedMovieMetadata) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"metadata":   metadata,
			"updated_at": time.Now(),
		},
	})

	return err
}

// SetMovieID sets the movie ID for a job after successful ingestion
func (r *JobRepository) SetMovieID(ctx context.Context, id string, movieID primitive.ObjectID) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"movie_id":   movieID,
			"updated_at": time.Now(),
		},
	})

	return err
}

// SetError sets the error message for a failed job
func (r *JobRepository) SetError(ctx context.Context, id string, errMsg string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"error":      errMsg,
			"status":     models.IngestionStatusFailed,
			"updated_at": time.Now(),
		},
	})

	return err
}

// IncrementRetry increments the retry count for a job
func (r *JobRepository) IncrementRetry(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$inc": bson.M{"retry_count": 1},
		"$set": bson.M{"status": models.IngestionStatusPending, "updated_at": time.Now()},
	})

	return err
}

// UpdateLocalPath updates the local file path after download
func (r *JobRepository) UpdateLocalPath(ctx context.Context, id, path string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"local_path": path,
			"updated_at": time.Now(),
		},
	})

	return err
}

// UpdateOutputPath updates the output path after processing
func (r *JobRepository) UpdateOutputPath(ctx context.Context, id, path string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"output_path": path,
			"updated_at":  time.Now(),
		},
	})

	return err
}

// UpdateVideoURL updates the final video URL after upload
func (r *JobRepository) UpdateVideoURL(ctx context.Context, id, url string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"video_url":  url,
			"updated_at": time.Now(),
		},
	})

	return err
}

// UpdateStep marks a pipeline step as completed
func (r *JobRepository) UpdateStep(ctx context.Context, id string, step string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	updateField := fmt.Sprintf("steps.%s", step)
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			updateField:  true,
			"updated_at": time.Now(),
		},
	})

	return err
}

// ProgressUpdate represents download progress data from parser
type ProgressUpdate struct {
	Stage           string  `json:"stage"`
	Status          string  `json:"status"`
	Progress        int     `json:"progress_percent"`
	DownloadedBytes int64   `json:"downloaded_bytes"`
	TotalBytes      int64   `json:"total_bytes"`
	SpeedMBps       float64 `json:"speed_mbps"`
	EtaSeconds      int     `json:"eta_seconds"`
	Message         string  `json:"message"`
	StepsDownload   bool    `json:"steps_download"`
	FilePath        string  `json:"file_path"` // NEW: Local file path after download completes
}

// DeriveStatusFromStage maps pipeline stage to job status
// This ensures status is always updated when stage changes, even if parser doesn't send explicit status
// FIX: Status and progress are treated as separate concepts - stage changes update status immediately
func DeriveStatusFromStage(stage string) string {
	switch stage {
	case "parse":
		return "parsing"
	case "download":
		return "downloading"
	case "process", "watermark", "ffmpeg", "hls", "poster", "backdrop":
		return "processing"
	case "done":
		return "completed"
	case "failed":
		return "failed"
	case "upload":
		return "uploading"
	default:
		// Unknown stage - return empty to let explicit status win
		return ""
	}
}

// UpdateProgress updates the download progress for a job
func (r *JobRepository) UpdateProgress(ctx context.Context, id string, progress *ProgressUpdate) error {
	log.Printf("[JOB REPO] ========== UPDATE PROGRESS START ==========")
	log.Printf("[JOB REPO] Input id: %q", id)

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		log.Printf("[JOB REPO] ERROR: invalid ObjectID: %v", err)
		return fmt.Errorf("invalid id: %v", err)
	}
	log.Printf("[JOB REPO] Converted to ObjectID: %s", objID.Hex())

	// FIX: Derive status from stage if explicit status not provided
	// This ensures status is updated when stage changes, even if progress is 0
	derivedStatus := DeriveStatusFromStage(progress.Stage)
	effectiveStatus := progress.Status
	if effectiveStatus == "" && derivedStatus != "" {
		effectiveStatus = derivedStatus
		log.Printf("[JOB REPO] Deriving status from stage: stage=%q -> status=%q", progress.Stage, effectiveStatus)
	}

	log.Printf("[JOB REPO] UpdateProgress: id=%s stage=%q status=%q progress=%d downloaded=%d total=%d steps_download=%v",
		id, progress.Stage, effectiveStatus, progress.Progress, progress.DownloadedBytes, progress.TotalBytes, progress.StepsDownload)

	// Build update - always set all fields from progress (allow 0 values)
	update := bson.M{
		"$set": bson.M{
			"updated_at":       time.Now(),
			"stage":            progress.Stage,
			"status":           effectiveStatus,
			"progress":         progress.Progress,
			"downloaded_bytes": progress.DownloadedBytes,
			"total_bytes":      progress.TotalBytes,
			"speed_mbps":       progress.SpeedMBps,
			"eta_seconds":      progress.EtaSeconds,
		},
	}

	// CRITICAL: Set steps.download = true ONLY when download is 100% complete
	// This prevents worker from starting before download finishes
	downloadComplete := (progress.Stage == "download" && progress.Progress >= 100) || progress.StepsDownload
	if downloadComplete {
		log.Printf("[JOB REPO] Download complete (progress=%d%%, steps_download=%v) - marking steps.download=true", progress.Progress, progress.StepsDownload)
		update["$set"].(bson.M)["steps.download"] = true

		// On download complete: save local_path but do NOT force stage/status —
		// worker sets those explicitly when it starts processing.
		if progress.StepsDownload || progress.Progress >= 100 {
			log.Printf("[JOB REPO] Download finished - setting status to downloaded")
			update["$set"].(bson.M)["status"] = "downloaded"
			if progress.FilePath != "" {
				update["$set"].(bson.M)["local_path"] = progress.FilePath
				log.Printf("[JOB REPO] Updated local_path=%s (download completed)", progress.FilePath)
			}
		}
	} else {
		log.Printf("[JOB REPO] NOT setting steps.download (progress=%d%%, not complete)", progress.Progress)
	}

	if progress.Message != "" {
		update["$set"].(bson.M)["message"] = progress.Message
	}

	// Update status if provided
	if progress.Status != "" {
		update["$set"].(bson.M)["status"] = progress.Status
	}

	log.Printf("[JOB REPO] Calling UpdateOne with filter: _id=%s", objID.Hex())
	log.Printf("[JOB REPO] Update document: %v", update)

	result, err := r.collection.UpdateByID(ctx, objID, update)
	if err != nil {
		log.Printf("[JOB REPO] ERROR: MongoDB update failed: %v", err)
		return fmt.Errorf("failed to update progress: %v", err)
	}

	log.Printf("[JOB REPO] RESULT: MatchedCount=%d, ModifiedCount=%d", result.MatchedCount, result.ModifiedCount)

	if result.MatchedCount == 0 {
		log.Printf("[JOB REPO] ERROR: No document matched _id=%s", objID.Hex())
	}

	log.Printf("[JOB REPO] ========== UPDATE PROGRESS END ==========")
	return nil
}

// List retrieves all jobs with optional filters
func (r *JobRepository) List(ctx context.Context, filter bson.M, limit, skip int) ([]*models.IngestionJob, error) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var jobs []*models.IngestionJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}

	return jobs, nil
}

// Count counts jobs matching a filter
func (r *JobRepository) Count(ctx context.Context, filter bson.M) (int64, error) {
	return r.collection.CountDocuments(ctx, filter)
}
