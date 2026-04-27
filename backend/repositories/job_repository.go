package repositories

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func resolveExistingLocalPath(path string) string {
	if path == "" {
		return ""
	}
	candidates := []string{path}
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			candidates = append([]string{abs}, candidates...)
		}
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() && info.Size() > 0 {
			return candidate
		}
	}
	return ""
}

func resolveExistingDownloadedArtifact(jobID, rawPath string) string {
	canonicalize := func(path string) string {
		if !strings.HasSuffix(path, ".MUX.mp4") {
			return path
		}
		canonical := strings.TrimSuffix(path, ".MUX.mp4") + ".mp4"
		if _, err := os.Stat(path); err == nil {
			if _, dstErr := os.Stat(canonical); dstErr != nil {
				log.Printf("[DOWNLOAD RENAME] from=%s to=%s", path, canonical)
				if renameErr := os.Rename(path, canonical); renameErr == nil {
					return canonical
				}
			}
		}
		return path
	}

	if verified := resolveExistingLocalPath(rawPath); verified != "" {
		return canonicalize(verified)
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if strings.TrimSpace(downloadDir) == "" {
		downloadDir = "/opt/filmorauz/parser/downloads"
	}
	downloadDir = filepath.Clean(downloadDir)
	base := strings.TrimSpace(jobID)
	if base == "" {
		return ""
	}

	preferred := []string{
		filepath.Join(downloadDir, base+".mp4"),
		filepath.Join(downloadDir, base+".MUX.mp4"),
	}
	for _, candidate := range preferred {
		if verified := resolveExistingLocalPath(candidate); verified != "" {
			return canonicalize(verified)
		}
	}

	matches, _ := filepath.Glob(filepath.Join(downloadDir, base+"*"))
	for _, match := range matches {
		if verified := resolveExistingLocalPath(match); verified != "" {
			return canonicalize(verified)
		}
	}
	return ""
}

var topLevelGroupKeyExpr = bson.M{
	"$cond": bson.A{
		bson.M{
			"$or": bson.A{
				bson.M{"$eq": bson.A{"$content_type", "episode"}},
				bson.M{"$gt": bson.A{bson.M{"$ifNull": bson.A{"$season_number", 0}}, 0}},
				bson.M{"$gt": bson.A{bson.M{"$ifNull": bson.A{"$episode_number", 0}}, 0}},
			},
		},
		bson.M{
			"$cond": bson.A{
				bson.M{"$ne": bson.A{bson.M{"$ifNull": bson.A{"$series_slug", ""}}, ""}},
				bson.M{"$concat": bson.A{"serial:", "$series_slug"}},
				bson.M{"$concat": bson.A{"job:", bson.M{"$toString": "$_id"}}},
			},
		},
		bson.M{"$concat": bson.A{"job:", bson.M{"$toString": "$_id"}}},
	},
}

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

	// Index on updated_at for sorting and stale detection queries
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

// Create creates a new ingestion job
func (r *JobRepository) Create(ctx context.Context, job *models.IngestionJob) error {
	job.ID = primitive.NewObjectID()
	job.CreatedAt = time.Now()
	job.UpdatedAt = time.Now()
	if job.Status == "" {
		job.Status = models.IngestionStatusQueued
	}
	if job.Stage == "" {
		job.Stage = string(job.Status)
	}
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

// GetBySourceAndID retrieves a job by its (source, source_id) tuple. Returns
// (nil, nil) if no such job exists.
func (r *JobRepository) GetBySourceAndID(ctx context.Context, source, sourceID string) (*models.IngestionJob, error) {
	var job models.IngestionJob
	err := r.collection.FindOne(ctx, bson.M{"source": source, "source_id": sourceID}).Decode(&job)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
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
				models.IngestionStatusQueued,
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

// ClaimNextJob atomically claims the next queued download job.
// This uses FindOneAndUpdate to ensure only one worker can claim a job
// Returns nil if no jobs are available
func (r *JobRepository) ClaimNextJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status": models.IngestionStatusQueued,
		"retry_count": bson.M{
			"$lt": 3, // Max 3 retries
		},
		"steps.download": bson.M{"$ne": true},
	}

	now := time.Now()
	// started_at: stamp when work actually begins so the UI elapsed timer
	// does not run while the job is sitting in the pending queue. On retry
	// we re-stamp it so each attempt has its own timer.
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

// ClaimNextProcessingJob atomically claims a job ready for ffmpeg processing.
func (r *JobRepository) ClaimNextProcessingJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status":         models.IngestionStatusReadyToProcess,
		"retry_count":    bson.M{"$lt": 3},
		"steps.download": true,
		"local_path":     bson.M{"$exists": true, "$ne": ""},
		"steps.process":  bson.M{"$ne": true},
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

	verifiedPath := resolveExistingLocalPath(job.LocalPath)
	if verifiedPath == "" {
		log.Printf("[PROCESS] skipped job=%s reason=missing local_path", job.ID.Hex())
		if setErr := r.SetError(ctx, job.ID.Hex(), "download completed but local_path missing/file not found"); setErr != nil {
			return nil, setErr
		}
		return nil, nil
	}
	job.LocalPath = verifiedPath

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
	} else {
		update["$unset"] = bson.M{"completed_at": ""}
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
			"stage":      "failed",
			"updated_at": time.Now(),
		},
	})

	return err
}

func (r *JobRepository) ClearError(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"error":      "",
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
		"$set": bson.M{
			"status":     models.IngestionStatusQueued,
			"stage":      "queued",
			"updated_at": time.Now(),
		},
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
	Stage              string  `json:"stage"`
	Status             string  `json:"status"`
	Progress           int     `json:"progress_percent"`
	DownloadedBytes    int64   `json:"downloaded_bytes"`
	TotalBytes         int64   `json:"total_bytes"`
	SpeedMBps          float64 `json:"speed_mbps"`
	EtaSeconds         int     `json:"eta_seconds"`
	Message            string  `json:"message"`
	StepsDownload      bool    `json:"steps_download"`
	FilePath           string  `json:"file_path"` // NEW: Local file path after download completes
	LocalPath          string  `json:"local_path"`
	DownloadedFilePath string  `json:"downloaded_file_path"`
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
	case "ready_to_process":
		return "ready_to_process"
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

	normalizedProgress := progress.Progress
	if normalizedProgress < 0 {
		normalizedProgress = 0
	}
	if normalizedProgress > 100 {
		normalizedProgress = 100
	}

	// Build update - always set all fields from progress (allow 0 values)
	update := bson.M{
		"$set": bson.M{
			"updated_at":       time.Now(),
			"stage":            progress.Stage,
			"status":           effectiveStatus,
			"progress":         normalizedProgress,
			"downloaded_bytes": progress.DownloadedBytes,
			"total_bytes":      progress.TotalBytes,
			"speed_mbps":       progress.SpeedMBps,
			"eta_seconds":      progress.EtaSeconds,
		},
	}

	// CRITICAL: Always update stage and status to current values from parser/worker
	// This overwrites stale UI state with fresh progress
	if progress.Stage != "" {
		update["$set"].(bson.M)["stage"] = progress.Stage
	}
	if progress.Status != "" {
		update["$set"].(bson.M)["status"] = progress.Status
		update["$set"].(bson.M)["stage"] = progress.Status
	} else if progress.Stage != "" {
		// Derive status from stage if not explicitly provided
		derived := DeriveStatusFromStage(progress.Stage)
		if derived != "" {
			update["$set"].(bson.M)["status"] = derived
			update["$set"].(bson.M)["stage"] = derived
		}
	}

	// Set progress value
	update["$set"].(bson.M)["progress"] = normalizedProgress

	// CRITICAL: Set steps.download = true ONLY when download is 100% complete
	downloadComplete := (progress.Stage == "download" && normalizedProgress >= 100) || progress.StepsDownload
	if downloadComplete {
		candidatePath := progress.LocalPath
		if candidatePath == "" {
			candidatePath = progress.FilePath
		}
		if candidatePath == "" {
			candidatePath = progress.DownloadedFilePath
		}
		log.Printf("[INGESTION] complete download job=%s local_path=%s", id, candidatePath)
		log.Printf("[DOWNLOAD] completed job=%s file=%s", id, candidatePath)
		verifiedPath := resolveExistingDownloadedArtifact(id, candidatePath)
		if verifiedPath == "" {
			errMsg := "download completed but local_path missing/file not found"
			log.Printf("[DOWNLOAD] failed verification job=%s reason=%s raw_file=%s", id, errMsg, candidatePath)
			update["$set"].(bson.M)["status"] = string(models.IngestionStatusFailed)
			update["$set"].(bson.M)["stage"] = string(models.IngestionStatusFailed)
			update["$set"].(bson.M)["error"] = errMsg
			update["$set"].(bson.M)["local_path"] = ""
			update["$set"].(bson.M)["file_path"] = ""
			update["$set"].(bson.M)["downloaded_file_path"] = ""
			update["$set"].(bson.M)["steps.download"] = false
		} else {
			if candidatePath == "" || verifiedPath != candidatePath {
				log.Printf("[AUTO_RECOVER] job=%s found file=%s -> repaired", id, verifiedPath)
			}
			log.Printf("[DOWNLOAD] verified file exists job=%s file=%s", id, verifiedPath)
			update["$set"].(bson.M)["steps.download"] = true
			update["$set"].(bson.M)["progress"] = normalizedProgress
			update["$set"].(bson.M)["status"] = string(models.IngestionStatusReadyToProcess)
			update["$set"].(bson.M)["stage"] = "processing"
			update["$set"].(bson.M)["local_path"] = verifiedPath
			update["$set"].(bson.M)["file_path"] = verifiedPath
			update["$set"].(bson.M)["downloaded_file_path"] = verifiedPath
			update["$set"].(bson.M)["steps.process"] = false
			update["$set"].(bson.M)["error"] = ""
			log.Printf("[DOWNLOAD] moved to ready_to_process job=%s file=%s", id, verifiedPath)
		}
	} else {
		log.Printf("[JOB REPO] NOT setting steps.download (progress=%d%%, not complete)", normalizedProgress)
	}

	if progress.Message != "" {
		update["$set"].(bson.M)["message"] = progress.Message
	}

	log.Printf("[JOB REPO] Calling UpdateOne with filter: _id=%s", objID.Hex())
	log.Printf("[JOB REPO] Update document: %v", update)

	result, err := r.collection.UpdateByID(ctx, objID, update)
	if err != nil {
		log.Printf("[JOB REPO] ERROR: MongoDB update failed: %v", err)
		return fmt.Errorf("failed to update progress: %v", err)
	}

	log.Printf("[JOB REPO] RESULT: MatchedCount=%d, ModifiedCount=%d", result.MatchedCount, result.ModifiedCount)
	if downloadComplete {
		savedPath, _ := update["$set"].(bson.M)["local_path"].(string)
		log.Printf("[DOWNLOAD_COMPLETE] job=%s saved local_path=%s exists=%v", id, savedPath, savedPath != "" && resolveExistingLocalPath(savedPath) != "")
		log.Printf("[DOWNLOAD_COMPLETE] backend update response=%d", result.ModifiedCount)
	}

	if result.MatchedCount == 0 {
		log.Printf("[JOB REPO] ERROR: No document matched _id=%s", objID.Hex())
	}

	log.Printf("[JOB REPO] ========== UPDATE PROGRESS END ==========")
	return nil
}

// List retrieves all jobs with optional filters
func (r *JobRepository) List(ctx context.Context, filter bson.M, limit, skip int) ([]*models.IngestionJob, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if duration > 500*time.Millisecond {
			log.Printf("[JOB REPO] List slow query: filter=%v, limit=%d, duration=%v", filter, limit, duration)
		}
	}()

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

// ListLight retrieves jobs without logs and metadata (for polling)
func (r *JobRepository) ListLight(ctx context.Context, filter bson.M, limit, skip int) ([]*models.IngestionJob, error) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if duration > 200*time.Millisecond {
			log.Printf("[JOB REPO] ListLight slow query: filter=%v, limit=%d, duration=%v", filter, limit, duration)
		}
	}()

	projection := bson.D{
		{Key: "logs", Value: 0},
		{Key: "metadata", Value: 0},
		{Key: "enriched_metadata", Value: 0},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip)).
		SetProjection(projection)

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

// CountTopLevelGroups counts the number of top-level items after serial jobs are
// grouped together for the admin tree view.
func (r *JobRepository) CountTopLevelGroups(ctx context.Context, filter bson.M) (int64, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$addFields", Value: bson.M{"top_level_group_key": topLevelGroupKeyExpr}}},
		{{Key: "$group", Value: bson.M{"_id": "$top_level_group_key"}}},
		{{Key: "$count", Value: "total"}},
	}

	cursor, err := r.collection.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		Total int64 `bson:"total"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return 0, err
	}
	if len(result) == 0 {
		return 0, nil
	}

	return result[0].Total, nil
}

// ListByTopLevelGroups returns all jobs that belong to the paginated top-level
// groups used by the serial tree UI. Movies are single-item groups; episodes are
// grouped by series_slug when available.
func (r *JobRepository) ListByTopLevelGroups(ctx context.Context, filter bson.M, limit, skip int, light bool) ([]*models.IngestionJob, error) {
	groupPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$addFields", Value: bson.M{"top_level_group_key": topLevelGroupKeyExpr}}},
		{{Key: "$group", Value: bson.M{
			"_id":               "$top_level_group_key",
			"latest_created_at": bson.M{"$max": "$created_at"},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "latest_created_at", Value: -1}}}},
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: limit}},
	}

	groupCursor, err := r.collection.Aggregate(ctx, groupPipeline)
	if err != nil {
		return nil, err
	}
	defer groupCursor.Close(ctx)

	var groups []struct {
		ID string `bson:"_id"`
	}
	if err := groupCursor.All(ctx, &groups); err != nil {
		return nil, err
	}
	if len(groups) == 0 {
		return []*models.IngestionJob{}, nil
	}

	groupKeys := make([]string, 0, len(groups))
	for _, group := range groups {
		groupKeys = append(groupKeys, group.ID)
	}

	jobsPipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$addFields", Value: bson.M{"top_level_group_key": topLevelGroupKeyExpr}}},
		{{Key: "$match", Value: bson.M{"top_level_group_key": bson.M{"$in": groupKeys}}}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
	}

	if light {
		jobsPipeline = append(jobsPipeline, bson.D{{Key: "$project", Value: bson.D{
			{Key: "logs", Value: 0},
			{Key: "metadata", Value: 0},
			{Key: "enriched_metadata", Value: 0},
			{Key: "top_level_group_key", Value: 0},
		}}})
	} else {
		jobsPipeline = append(jobsPipeline, bson.D{{Key: "$project", Value: bson.D{
			{Key: "top_level_group_key", Value: 0},
		}}})
	}

	cursor, err := r.collection.Aggregate(ctx, jobsPipeline)
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
