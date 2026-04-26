package repositories

import (
	"context"
	"fmt"
	"log"
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

	// Index on updated_at for stale job detection and sorting
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "updated_at", Value: -1},
		},
	})

	// Index on stage for filtering active jobs
	collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys: bson.D{
			{Key: "status", Value: 1},
			{Key: "stage", Value: 1},
		},
	})

	return &JobRepository{
		collection: collection,
	}
}

// UpdateVideoURL updates the video_url field of a job
func (r *JobRepository) UpdateVideoURL(ctx context.Context, id, videoURL string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"video_url":  videoURL,
			"updated_at": time.Now(),
		},
	})

	return err
}

// ClaimNextJob atomically claims the next queued download job.
func (r *JobRepository) ClaimNextJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status":      models.IngestionStatusQueued,
		"retry_count": bson.M{"$lt": 3},
		"$or": []bson.M{
			{"steps.download": bson.M{"$exists": false}},
			{"steps.download": false},
		},
	}

	now := time.Now()
	// started_at: stamp when work actually begins so the UI elapsed timer
	// does not run while the job is sitting in the pending queue.
	update := bson.M{
		"$set": bson.M{
			"status":     models.IngestionStatusDownloading,
			"stage":      "download",
			"updated_at": now,
			"started_at": now,
		},
	}

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	log.Printf("[REPO] ClaimNextJob: querying for pending download jobs...")
	log.Printf("[REPO] ClaimNextJob filter: status=queued, retry_count<3, steps.download=$exists:false OR steps.download=false")
	log.Printf("[REPO] ClaimNextJob FINAL QUERY: %+v", filter)

	var job models.IngestionJob
	err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("[REPO] ClaimNextJob: no pending download jobs found")
			return nil, nil
		}
		log.Printf("[REPO] ClaimNextJob: ERROR - %v", err)
		return nil, err
	}

	log.Printf("[REPO] ClaimNextJob: CLAIMED job %s (status: queued -> downloading, title: %s, source: %s)",
		job.ID.Hex(), job.Title, job.Source)
	return &job, nil
}

// ClaimNextProcessingJob atomically claims a job ready for ffmpeg processing.
func (r *JobRepository) ClaimNextProcessingJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status":         models.IngestionStatusReadyToProcess,
		"retry_count":    bson.M{"$lt": 3},
		"steps.download": true,
		"$or": []bson.M{
			{"steps.process": bson.M{"$exists": false}},
			{"steps.process": bson.M{"$ne": true}},
		},
	}

	now := time.Now()
	// Preserve started_at if download already stamped it; only initialize
	// when missing so the elapsed timer is continuous across stages.
	update := bson.A{
		bson.M{
			"$set": bson.M{
				"status":     models.IngestionStatusProcessing,
				"stage":      "processing",
				"updated_at": now,
				"started_at": bson.M{"$ifNull": bson.A{"$started_at", now}},
			},
		},
	}

	log.Printf("[REPO] ClaimNextProcessingJob: querying for pending processing jobs...")
	log.Printf("[REPO] ClaimNextProcessingJob filter: status=ready_to_process, retry_count<3, steps.download=true, steps.process=$exists:false OR steps.process!=true")
	log.Printf("[REPO] ClaimNextProcessingJob FINAL QUERY: %+v", filter)

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	var job models.IngestionJob
	err := r.collection.FindOneAndUpdate(ctx, filter, update, opts).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("[REPO] ClaimNextProcessingJob: no pending processing jobs found (0 documents)")
			return nil, nil
		}
		log.Printf("[REPO] ClaimNextProcessingJob: ERROR - %v", err)
		return nil, err
	}

	log.Printf("[REPO] ClaimNextProcessingJob: CLAIMED job %s (status: ready_to_process -> processing, title: %s, source: %s)",
		job.ID.Hex(), job.Title, job.Source)
	return &job, nil
}

// CountPendingJobs returns the count of pending jobs for debugging
// This helps diagnose why worker isn't picking up jobs
func (r *JobRepository) CountPendingJobs(ctx context.Context) (int64, error) {
	// Count all pending jobs (regardless of steps)
	filter := bson.M{
		"status": models.IngestionStatusQueued,
	}
	total, err := r.collection.CountDocuments(ctx, filter)
	if err != nil {
		return 0, err
	}

	// Count jobs needing download
	downloadFilter := bson.M{
		"status": models.IngestionStatusQueued,
		"$or": []bson.M{
			{"steps.download": bson.M{"$exists": false}},
			{"steps.download": false},
		},
	}
	log.Printf("[REPO] CountPendingJobs download filter: %+v", downloadFilter)
	downloadJobs, err := r.collection.CountDocuments(ctx, downloadFilter)
	if err != nil {
		return 0, err
	}

	// Count jobs needing processing (download done, process not done)
	processFilter := bson.M{
		"status":         models.IngestionStatusReadyToProcess,
		"steps.download": true,
		"$or": []bson.M{
			{"steps.process": bson.M{"$exists": false}},
			{"steps.process": false},
		},
	}
	processJobs, err := r.collection.CountDocuments(ctx, processFilter)
	if err != nil {
		return 0, err
	}

	log.Printf("[REPO] Pending jobs: total=%d, download_needed=%d, process_needed=%d",
		total, downloadJobs, processJobs)
	return total, nil
}

func (r *JobRepository) ResetStaleJobs(ctx context.Context) (int64, error) {
	staleThreshold := 180 * time.Minute
	staleCutoff := time.Now().Add(-staleThreshold)
	filter := bson.M{
		"status":     bson.M{"$in": activeIngestionStatuses},
		"updated_at": bson.M{"$lt": staleCutoff},
	}

	update := bson.M{
		"$set": bson.M{
			"status":     models.IngestionStatusPending,
			"progress":   0,
			"updated_at": time.Now(),
		},
	}

	result, err := r.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		log.Printf("[REPO] ResetStaleJobs: ERROR - %v", err)
		return 0, err
	}

	if result.ModifiedCount > 0 {
		log.Printf("[REPO] ResetStaleJobs: reset %d stale jobs (no update for >180 minutes)",
			result.ModifiedCount)
	}
	return result.ModifiedCount, nil
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

	if status == models.IngestionStatusCompleted || status == models.IngestionStatusFailed || status == models.IngestionStatusNeedsManual {
		completedAt := time.Now()
		update["$set"].(bson.M)["completed_at"] = completedAt
	}

	_, err = r.collection.UpdateByID(ctx, objID, update)
	return err
}

// Heartbeat updates only the timestamp, keeping progress/status unchanged.
// This prevents stale-job detection during long-running processing (FFmpeg can take 30-60+ min).
func (r *JobRepository) Heartbeat(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	})
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
			"stage":      "failed",
			"updated_at": time.Now(),
		},
	})

	return err
}

func (r *JobRepository) MarkNeedsManual(ctx context.Context, id string, errMsg string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	now := time.Now()
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"error":        errMsg,
			"message":      errMsg,
			"status":       models.IngestionStatusNeedsManual,
			"stage":        string(models.IngestionStatusNeedsManual),
			"progress":     0,
			"completed_at": now,
			"updated_at":   now,
		},
	})
	return err
}

// IncrementRetry increments the retry count for a job.
// If the incremented count is below the max (3), the job is reset to pending for another attempt.
// If the incremented count reaches or exceeds the max, the job stays in a terminal failed state
// (status=failed, stage=failed) so it will never be picked up again and the UI shows it as failed.
func (r *JobRepository) IncrementRetry(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	const maxRetries = 3

	var current models.IngestionJob
	if err := r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&current); err != nil {
		log.Printf("[REPO] IncrementRetry: failed to fetch job %s: %v", id, err)
		return err
	}

	newRetry := current.RetryCount + 1
	setFields := bson.M{"updated_at": time.Now()}

	if newRetry >= maxRetries {
		setFields["status"] = models.IngestionStatusFailed
		setFields["stage"] = "failed"
		setFields["completed_at"] = time.Now()
		log.Printf("[REPO] IncrementRetry: job %s reached max retries (%d) — marking as failed (terminal)", id, newRetry)
	} else {
		setFields["status"] = models.IngestionStatusQueued
		setFields["stage"] = "queued"
		log.Printf("[REPO] IncrementRetry: job %s retry %d/%d — resetting to queued", id, newRetry, maxRetries)
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$inc": bson.M{"retry_count": 1},
		"$set": setFields,
	})

	return err
}

var activeIngestionStatuses = []interface{}{
	models.IngestionStatusProcessing,
	models.IngestionStatusDownloading,
	models.IngestionStatusUploading,
}

func (r *JobRepository) FailStaleProcessingJobs(ctx context.Context) (int64, error) {
	staleThreshold := 180 * time.Minute
	staleCutoff := time.Now().Add(-staleThreshold)
	filter := bson.M{
		"status":     bson.M{"$in": activeIngestionStatuses},
		"updated_at": bson.M{"$lt": staleCutoff},
	}

	update := bson.M{
		"$set": bson.M{
			"status":       models.IngestionStatusFailed,
			"stage":        "failed",
			"error":        "Job stuck with no update for over 180 minutes — marked as failed by stale-job protection",
			"completed_at": time.Now(),
			"updated_at":   time.Now(),
		},
	}

	result, err := r.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		log.Printf("[REPO] FailStaleProcessingJobs: ERROR - %v", err)
		return 0, err
	}

	if result.ModifiedCount > 0 {
		log.Printf("[REPO] FailStaleProcessingJobs: failed %d stale jobs (no update for >180 minutes)",
			result.ModifiedCount)
	}
	return result.ModifiedCount, nil
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

// MarkDownloadStarted marks that /download was called for this job
// This prevents duplicate /download calls on retries
func (r *JobRepository) MarkDownloadStarted(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"steps.download_started": true,
			"updated_at":             time.Now(),
		},
	})

	return err
}

// UpdateDownloadProgress updates download progress fields in the job
// Ensures monotonicity: never overwrite with lower progress or downloaded_bytes.
// Sets last_progress_at to now whenever downloaded_bytes actually advances; the
// watchdog in the worker poll loop reads this to decide if the parser-side
// download is stalled (process alive but not making progress).
func (r *JobRepository) UpdateDownloadProgress(ctx context.Context, id string, progress int, downloadedBytes, totalBytes int64, speedMbps float64, etaSeconds int, message string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	// Fetch current job to check monotonicity (protect against stale responses)
	var currentJob models.IngestionJob
	err = r.collection.FindOne(ctx, bson.M{"_id": objID}).Decode(&currentJob)
	if err != nil {
		log.Printf("[REPO] UpdateDownloadProgress: failed to fetch current job %s: %v (will attempt update anyway)", id, err)
	} else {
		// Only block if downloaded_bytes decreased (stale response) OR progress is 0 while we previously had progress > 0 AND downloaded_bytes also decreased.
		// Allow progress to fluctuate due to total_bytes changes; bytes should only increase.
		if currentJob.DownloadedBytes > 0 && downloadedBytes < currentJob.DownloadedBytes {
			log.Printf("[REPO] UpdateDownloadProgress: STALE BYTES DETECTED — job_id=%s, new_bytes=%d < current_bytes=%d (ignoring update)",
				id, downloadedBytes, currentJob.DownloadedBytes)
			return nil
		}
		// Additional guard: if progress drops to 0 but we already have non-zero downloaded bytes, likely stale (worker should have computed from bytes)
		if progress == 0 && currentJob.Progress > 0 && currentJob.DownloadedBytes > 0 && downloadedBytes <= currentJob.DownloadedBytes {
			log.Printf("[REPO] UpdateDownloadProgress: STALE ZERO PROGRESS — job_id=%s, new_progress=0, current_progress=%d, bytes=%d->%d (ignoring)",
				id, currentJob.Progress, currentJob.DownloadedBytes, downloadedBytes)
			return nil
		}
	}

	now := time.Now()
	setFields := bson.M{
		"progress":         progress,
		"downloaded_bytes": downloadedBytes,
		"total_bytes":      totalBytes,
		"speed_mbps":       speedMbps,
		"eta_seconds":      etaSeconds,
		"message":          message,
		"status":           models.IngestionStatusDownloading,
		"updated_at":       now,
	}
	// Only stamp last_progress_at when bytes actually moved forward — this is
	// what the watchdog uses to detect a hung downloader. If the parser keeps
	// responding with the same byte count, updated_at still ticks (heartbeat),
	// but last_progress_at stays frozen and the watchdog will fire.
	if downloadedBytes > currentJob.DownloadedBytes {
		setFields["last_progress_at"] = now
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{"$set": setFields})
	if err != nil {
		log.Printf("[REPO] UpdateDownloadProgress: UPDATE FAILED — job_id=%s, progress=%d, err=%v", id, progress, err)
		return err
	}

	log.Printf("[REPO] UpdateDownloadProgress: job_id=%s, progress=%d%%, downloaded=%d/%d, speed=%.2f MB/s, eta=%ds",
		id, progress, downloadedBytes, totalBytes, speedMbps, etaSeconds)
	return nil
}

// UpdateLastProgressAt stamps last_progress_at without touching the byte
// counters. Used when the worker first enters the polling loop so the watchdog
// has a baseline to compare against.
func (r *JobRepository) UpdateLastProgressAt(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}
	now := time.Now()
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"last_progress_at": now,
			"updated_at":       now,
		},
	})
	return err
}

// TransitionToProcessing atomically transitions job from download to processing stage
// This is called when parser successfully downloads the file and returns local_path
// Clears error field, sets status=processing, stage=processing, steps.download=true
// Also handles case where job might already have status=processing from being claimed
func (r *JobRepository) TransitionToProcessing(ctx context.Context, id, localPath string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	// Use $set to ensure stage is explicitly set to "processing"
	// This handles edge case where job was claimed but stuck at stage=download
	update := bson.M{
		"$set": bson.M{
			"status":         models.IngestionStatusReadyToProcess,
			"stage":          "ready_to_process",
			"progress":       100,
			"local_path":     localPath,
			"steps.download": true,
			"error":          "",
			"updated_at":     time.Now(),
		},
	}

	_, err = r.collection.UpdateByID(ctx, objID, update)
	if err != nil {
		log.Printf("[REPO] TransitionToProcessing: failed for job %s: %v", id, err)
		return err
	}

	log.Printf("[REPO] TransitionToProcessing: job %s transitioned to ready_to_process (local_path=%s)", id, localPath)
	return nil
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
			"status":         models.IngestionStatusQueued,
			"stage":          "queued",
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
			"status":         models.IngestionStatusReadyToProcess,
			"stage":          "ready_to_process",
			"progress":       100,
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
			"status":         models.IngestionStatusReadyToProcess,
			"stage":          "ready_to_process",
			"progress":       100,
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

// UpdateQualityInfo updates source quality and generated renditions info
func (r *JobRepository) UpdateQualityInfo(ctx context.Context, id, sourceQuality, sourceResolution string, generatedQualities []string, masterPlaylistURL string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	update := bson.M{
		"$set": bson.M{
			"source_quality":      sourceQuality,
			"source_resolution":   sourceResolution,
			"generated_qualities": generatedQualities,
			"master_playlist_url": masterPlaylistURL,
			"updated_at":          time.Now(),
		},
	}

	_, err = r.collection.UpdateByID(ctx, objID, update)
	return err
}
