package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/filmorauz/worker/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// JobRepository handles ingestion job persistence
type JobRepository struct {
	collection *mongo.Collection
}

// NewJobRepository creates a new job repository
func NewJobRepository(db *mongo.Database) *JobRepository {
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
// CRITICAL: Only claims jobs where steps.download=true (download complete)
// This prevents worker from starting before download is 100% complete
func (r *JobRepository) ClaimNextJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status": models.IngestionStatusPending,
		"retry_count": bson.M{
			"$lt": 3, // Max 3 retries
		},
		// CRITICAL: Only claim jobs where download IS complete
		"steps.download": true,
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
			"stage":      string(status),
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

// UpdateMessage updates the message field of a job
func (r *JobRepository) UpdateMessage(ctx context.Context, id string, message string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"message":    message,
			"updated_at": time.Now(),
		},
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

// UpdateEnrichedMetadata updates the enriched metadata of a job
func (r *JobRepository) UpdateEnrichedMetadata(ctx context.Context, id string, metadata *models.EnrichedMetadata, source string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"enriched_metadata": metadata,
			"metadata_source":   source,
			"updated_at":        time.Now(),
		},
	})

	return err
}

// UpdatePosterInfo updates the poster URLs and generation status
func (r *JobRepository) UpdatePosterInfo(ctx context.Context, id, originalPosterURL, generatedPosterURL string, posterGenerated bool) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"original_poster_url":  originalPosterURL,
			"generated_poster_url": generatedPosterURL,
			"poster_generated":     posterGenerated,
			"updated_at":           time.Now(),
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

// UpdateTelegramNotification updates the Telegram notification status for a job
func (r *JobRepository) UpdateTelegramNotification(ctx context.Context, id string, notified bool, channelsPosted []string, botNotified bool, errorMsg string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	updateFields := bson.M{
		"telegram_notified":     notified,
		"telegram_bot_notified": botNotified,
		"telegram_notified_at":  time.Now(),
		"updated_at":            time.Now(),
	}

	if len(channelsPosted) > 0 {
		updateFields["telegram_channels_posted"] = channelsPosted
	}

	if errorMsg != "" {
		updateFields["telegram_error"] = errorMsg
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": updateFields,
	})

	return err
}

// UpdateMovieData updates the movie code and slug for a job
func (r *JobRepository) UpdateMovieData(ctx context.Context, id, movieCode, movieSlug string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"movie_code": movieCode,
			"movie_slug": movieSlug,
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
			"output_path":         path,
			"playlist_path":       path + "/index.m3u8", // HLS playlist path
			"source_file_deleted": true,
			"local_path":          "", // Clear source file path since it's deleted
			"updated_at":          time.Now(),
		},
	})

	return err
}

// MarkSourceFileDeleted marks the source file as deleted after processing
// This is called when worker processes the downloaded file and deletes the source
func (r *JobRepository) MarkSourceFileDeleted(ctx context.Context, id, outputPath string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	playlistPath := ""
	if outputPath != "" {
		playlistPath = outputPath + "/index.m3u8"
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"source_file_deleted": true,
			"local_path":          "", // Clear source file path
			"output_path":         outputPath,
			"playlist_path":       playlistPath,
			"updated_at":          time.Now(),
		},
	})

	return err
}

// UpdateFinalOutputPath updates the final output path after HLS finalization
// This is called after MODE-based finalization (development=local, production=B2)
func (r *JobRepository) UpdateFinalOutputPath(ctx context.Context, id, finalPath, playlistPath, outputMode string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"output_path":   finalPath,
			"playlist_path": playlistPath,
			"output_mode":   outputMode,
			"updated_at":    time.Now(),
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

// RetryFromDownload resets job to retry from download stage
// Clears all progress, local_path, and steps
func (r *JobRepository) RetryFromDownload(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"status":         models.IngestionStatusPending,
			"stage":          "download",
			"progress":       0,
			"local_path":     "",
			"steps.download": false,
			"steps.process":  false,
			"steps.upload":   false,
			"steps.metadata": false,
			"error":          "",
			"updated_at":     time.Now(),
		},
	})

	return err
}

// RetryFromProcess resets job to retry from process (ffmpeg) stage
// Keeps local_path and download step, only resets process and upload
func (r *JobRepository) RetryFromProcess(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"status":         models.IngestionStatusPending,
			"stage":          "process",
			"progress":       50,
			"steps.download": true, // Keep download as complete
			"steps.process":  false,
			"steps.upload":   false,
			"error":          "",
			"updated_at":     time.Now(),
		},
	})

	return err
}

// RetryFromUpload resets job to retry from upload stage
// Keeps download and process as complete, only resets upload
func (r *JobRepository) RetryFromUpload(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"status":         models.IngestionStatusPending,
			"stage":          "upload",
			"progress":       80,
			"steps.download": true,
			"steps.process":  true,
			"steps.upload":   false,
			"error":          "",
			"updated_at":     time.Now(),
		},
	})

	return err
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
