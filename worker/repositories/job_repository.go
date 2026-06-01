package repositories

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var nonFilenameChars = regexp.MustCompile(`[^\w\s-]`)
var repeatedSeparators = regexp.MustCompile(`[-\s]+`)

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

func sanitizeDownloadName(name string) string {
	cleaned := strings.TrimSpace(name)
	if cleaned == "" {
		return ""
	}
	cleaned = nonFilenameChars.ReplaceAllString(cleaned, "")
	cleaned = repeatedSeparators.ReplaceAllString(cleaned, "_")
	return strings.Trim(cleaned, "_")
}

func candidateDownloadPaths(job *models.IngestionJob, downloadDir string) []string {
	if downloadDir == "" {
		return nil
	}
	names := []string{
		job.ID.Hex(),
		job.Title,
		job.SourceID,
	}
	if job.Metadata != nil {
		names = append([]string{job.Metadata.Title}, names...)
	}
	seen := map[string]struct{}{}
	var candidates []string
	for _, name := range names {
		safe := sanitizeDownloadName(name)
		if safe == "" {
			continue
		}
		filename := safe + ".mp4"
		if _, ok := seen[filename]; ok {
			continue
		}
		seen[filename] = struct{}{}
		candidates = append(candidates, filepath.Join(downloadDir, filename))
	}
	return candidates
}

func resolveExistingDownloadedArtifact(jobID, rawPath, downloadDir string) string {
	if verified := resolveExistingLocalPath(rawPath); verified != "" {
		return verified
	}
	if strings.TrimSpace(downloadDir) == "" {
		downloadDir = os.Getenv("DOWNLOAD_DIR")
		if strings.TrimSpace(downloadDir) == "" {
			downloadDir = "/opt/filmorauz/parser/downloads"
		}
	}
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
			return verified
		}
	}
	matches, _ := filepath.Glob(filepath.Join(downloadDir, base+"*"))
	for _, match := range matches {
		if verified := resolveExistingLocalPath(match); verified != "" {
			return verified
		}
	}
	return ""
}

// unset is a helper for safe Mongo unset: unset(field string) => field: ""
func unset(fields ...string) bson.M {
	m := bson.M{}
	for _, f := range fields {
		m[f] = ""
	}
	return m
}

// JobRepository handles ingestion job persistence
type JobRepository struct {
	collection *mongo.Collection
	workerID   string
}

// NewJobRepository creates a new job repository
func NewJobRepository(db *mongo.Database, workerID string) *JobRepository {
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
		workerID:   workerID,
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
	if normalized, err := r.NormalizeQueuedJobs(ctx); err != nil {
		log.Printf("[worker] normalize queued error=%v", err)
	} else if normalized > 0 {
		log.Printf("[worker] normalized queued jobs count=%d", normalized)
	}

	lockCutoff := time.Now()
	filter := bson.M{
		"status":      models.IngestionStatusQueued,
		"retry_count": bson.M{"$lt": 3},
		"$or": []bson.M{
			{"steps.download": bson.M{"$exists": false}},
			{"steps.download": false},
		},
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"locked_until": bson.M{"$exists": false}},
					{"locked_until": nil},
					{"locked_until": bson.M{"$lte": lockCutoff}},
				},
			},
		},
	}

	now := time.Now()
	lockedUntil := now.Add(2 * time.Minute)
	// started_at: stamp when work actually begins so the UI elapsed timer
	// does not run while the job is sitting in the pending queue.
	update := bson.M{
		"$set": bson.M{
			"status":              models.IngestionStatusDownloading,
			"stage":               "download",
			"progress":            1,
			"steps.download":      true,
			"updated_at":          now,
			"started_at":          now,
			"download_started_at": now,
			"last_progress_at":    now,
			"worker_id":           r.workerID,
			"locked_until":        lockedUntil,
			"message":             "Worker claimed job; starting download",
		},
		"$unset": unset("completed_at"),
	}

	opts := options.FindOneAndUpdate().
		SetSort(bson.D{{Key: "created_at", Value: 1}})

	log.Printf("[REPO] ClaimNextJob: querying for pending download jobs...")
	log.Printf("[REPO] ClaimNextJob filter: status=queued, retry_count<3, steps.download=$exists:false OR steps.download=false, lock expired or absent")
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

func (r *JobRepository) NormalizeQueuedJobs(ctx context.Context) (int64, error) {
	now := time.Now()
	res, err := r.collection.UpdateMany(ctx, bson.M{
		"status": models.IngestionStatusQueued,
		"$or": []bson.M{
			{"stage": bson.M{"$ne": "download"}},
			{"progress": bson.M{"$ne": 0}},
			{"steps.download": bson.M{"$ne": false}},
			{"worker_id": bson.M{"$exists": true, "$ne": ""}},
			{"locked_until": bson.M{"$exists": true}},
			{"download_started_at": bson.M{"$exists": true}},
			{"last_progress_at": bson.M{"$exists": true}},
		},
	}, bson.M{
		"$set": bson.M{
			"stage":          "download",
			"progress":       0,
			"updated_at":     now,
			"message":        "Waiting for worker",
			"steps.download": false,
		},
		"$unset": unset(
			"worker_id",
			"locked_until",
			"download_started_at",
			"last_progress_at",
		),
	})
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}

// ClaimNextProcessingJob atomically claims a job ready for ffmpeg processing.
func (r *JobRepository) ClaimNextProcessingJob(ctx context.Context) (*models.IngestionJob, error) {
	filter := bson.M{
		"status":      models.IngestionStatusReadyToProcess,
		"retry_count": bson.M{"$lt": 3},
		"$and": []bson.M{
			// The process step must not already be done.
			{"$or": []bson.M{
				{"steps.process": bson.M{"$exists": false}},
				{"steps.process": bson.M{"$ne": true}},
			}},
			// Either a regular job whose download finished and produced a local
			// file, OR a clip_only job which re-pulls its own HLS from
			// master_playlist_url (no prior download step / local file needed).
			{"$or": []bson.M{
				{
					"steps.download": true,
					"local_path":     bson.M{"$exists": true, "$ne": ""},
				},
				{
					"content_type":        "clip_only",
					"master_playlist_url": bson.M{"$exists": true, "$ne": ""},
				},
			}},
		},
	}

	now := time.Now()
	// Preserve started_at if download already stamped it; only initialize
	// when missing so the elapsed timer is continuous across stages.
	update := bson.A{
		bson.M{
			"$set": bson.M{
				"status":                "processing",
				"stage":                 "processing",
				"updated_at":            now,
				"started_at":            bson.M{"$ifNull": bson.A{"$started_at", now}},
				"processing_started_at": now,
				// Backfill phase markers for legacy jobs that arrived without them.
				"download_finished_at":     bson.M{"$ifNull": bson.A{"$download_finished_at", now}},
				"queued_for_processing_at": bson.M{"$ifNull": bson.A{"$queued_for_processing_at", now}},
			},
		},
	}

	log.Printf("[REPO] ClaimNextProcessingJob: querying for pending processing jobs...")
	log.Printf("[REPO] ClaimNextProcessingJob filter: status=ready_to_process, retry_count<3, steps.process!=true, AND (steps.download=true+local_path OR content_type=clip_only+master_playlist_url)")
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

	// clip_only jobs carry a remote HLS URL in local_path and download it
	// themselves in processClipOnlyJob, so the on-disk artifact check below
	// (which expects a real local file) does not apply to them.
	if job.ContentType == "clip_only" {
		log.Printf("[REPO] ClaimNextProcessingJob: CLAIMED clip_only job %s (title: %s, master=%s)",
			job.ID.Hex(), job.Title, job.MasterPlaylistURL)
		return &job, nil
	}

	verifiedPath := resolveExistingDownloadedArtifact(job.ID.Hex(), job.LocalPath, "")
	if verifiedPath == "" {
		log.Printf("[PROCESS] skipped job=%s reason=missing local_path", job.ID.Hex())
		if job.Progress >= 100 {
			log.Printf("[PROCESS] deferred failure job=%s progress=100 waiting for repair", job.ID.Hex())
			return nil, nil
		}
		_ = r.SetError(ctx, job.ID.Hex(), "download completed but local_path missing/file not found")
		return nil, nil
	}
	if verifiedPath != job.LocalPath {
		log.Printf("[AUTO_RECOVER] job=%s found file=%s -> repaired", job.ID.Hex(), verifiedPath)
		_, _ = r.collection.UpdateByID(ctx, job.ID, bson.M{
			"$set": bson.M{
				"local_path":           verifiedPath,
				"file_path":            verifiedPath,
				"downloaded_file_path": verifiedPath,
				"steps.download":       true,
				"status":               models.IngestionStatusReadyToProcess,
				"stage":                "ready_to_process",
				"error":                "",
				"updated_at":           time.Now(),
			},
		})
	}
	job.LocalPath = verifiedPath

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
	return r.RecoverStaleJobs(ctx)
}

// ListCompletedWithLocalArtifacts returns completed jobs that still carry a
// local_path or output_path on disk — i.e. the worker's post-success
// cleanup didn't run or didn't reach the file. Used by the cleanup janitor
// to mop up after partial failures (process killed mid-cleanup, mode was
// dev but later flipped to prod, etc.).
func (r *JobRepository) ListCompletedWithLocalArtifacts(ctx context.Context, limit int64) ([]*models.IngestionJob, error) {
	filter := bson.M{
		"status": models.IngestionStatusCompleted,
		"$or": []bson.M{
			{"local_path": bson.M{"$exists": true, "$ne": ""}},
			{"output_path": bson.M{"$exists": true, "$ne": ""}},
		},
	}
	opts := options.Find().SetLimit(limit).SetSort(bson.M{"completed_at": -1})
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

// ListTerminalFailedWithLocalArtifacts returns jobs that have exhausted all
// retries (status=failed/download_failed AND retry_count >= 3) but still
// carry local files on disk. The cleanup janitor consumes this list so the
// parser/downloads source MP4 and readyvideo staging dir don't pile up after
// a job is permanently dead.
func (r *JobRepository) ListTerminalFailedWithLocalArtifacts(ctx context.Context, limit int64) ([]*models.IngestionJob, error) {
	filter := bson.M{
		"status": bson.M{"$in": bson.A{
			models.IngestionStatusFailed,
			models.IngestionStatusDownloadFailed,
		}},
		"retry_count": bson.M{"$gte": 3},
		"$or": []bson.M{
			{"local_path": bson.M{"$exists": true, "$ne": ""}},
			{"output_path": bson.M{"$exists": true, "$ne": ""}},
		},
	}
	opts := options.Find().SetLimit(limit).SetSort(bson.M{"updated_at": -1})
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

// ListActiveDownloadBasenames returns the set of basenames (e.g.
// "6a1048b6fbd321b2b41f5b7f.mp4") that belong to jobs still in non-terminal
// states. The orphan-file sweep uses this as an allowlist — anything in
// parser/downloads/ NOT in this set is fair game for deletion after the
// max-age threshold.
func (r *JobRepository) ListActiveDownloadBasenames(ctx context.Context) (map[string]struct{}, error) {
	filter := bson.M{
		"status": bson.M{"$in": bson.A{
			models.IngestionStatusQueued,
			models.IngestionStatusDownloading,
			models.IngestionStatusDownloaded,
			models.IngestionStatusReadyToProcess,
			models.IngestionStatusProcessing,
			models.IngestionStatusUploading,
			models.IngestionStatusParsing,
			models.IngestionStatusEnrichingMetadata,
			models.IngestionStatusCreatingMovie,
			models.IngestionStatusSendingNotification,
			models.IngestionStatusHLSProcessing,
			models.IngestionStatusFinalizingStorage,
		}},
		"local_path": bson.M{"$exists": true, "$ne": ""},
	}
	cursor, err := r.collection.Find(ctx, filter, options.Find().SetProjection(bson.M{"local_path": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	out := make(map[string]struct{})
	for cursor.Next(ctx) {
		var doc struct {
			LocalPath string `bson:"local_path"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}
		if doc.LocalPath == "" {
			continue
		}
		out[filepath.Base(doc.LocalPath)] = struct{}{}
	}
	return out, nil
}

func (r *JobRepository) RepairCompletedDownloads(ctx context.Context, downloadDir string) (int64, error) {
	filter := bson.M{
		"progress": bson.M{"$gte": 100},
		"status": bson.M{"$in": bson.A{
			models.IngestionStatusQueued,
			models.IngestionStatusDownloading,
			models.IngestionStatusParsing,
		}},
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var jobs []*models.IngestionJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return 0, err
	}

	var repaired int64
	now := time.Now()
	for _, job := range jobs {
		jobID := job.ID.Hex()
		localPath := resolveExistingDownloadedArtifact(jobID, job.LocalPath, downloadDir)
		if localPath == "" {
			for _, candidate := range candidateDownloadPaths(job, downloadDir) {
				if resolved := resolveExistingLocalPath(candidate); resolved != "" {
					localPath = resolved
					break
				}
			}
		}

		if localPath == "" {
			errMsg := "download completed but local_path missing/file not found"
			log.Printf("[DOWNLOAD] repair failed job=%s reason=%s", jobID, errMsg)
			_, err = r.collection.UpdateByID(ctx, job.ID, bson.M{
				"$set": bson.M{
					"status":     models.IngestionStatusFailed,
					"stage":      "failed",
					"error":      errMsg,
					"updated_at": now,
					"local_path": "",
				},
			})
			if err != nil {
				return repaired, err
			}
			repaired++
			continue
		}

		log.Printf("[DOWNLOAD] completed job=%s file=%s", jobID, localPath)
		log.Printf("[AUTO_RECOVER] job=%s found file=%s -> repaired", jobID, localPath)
		log.Printf("[DOWNLOAD] verified file exists job=%s", jobID)
		_, err = r.collection.UpdateByID(ctx, job.ID, bson.A{
			bson.M{
				"$set": bson.M{
					"local_path":               localPath,
					"file_path":                localPath,
					"downloaded_file_path":     localPath,
					"status":                   models.IngestionStatusReadyToProcess,
					"stage":                    "ready_to_process",
					"steps.download":           true,
					"progress":                 100,
					"updated_at":               now,
					"error":                    "",
					"download_finished_at":     bson.M{"$ifNull": bson.A{"$download_finished_at", now}},
					"queued_for_processing_at": bson.M{"$ifNull": bson.A{"$queued_for_processing_at", now}},
				},
			},
		})
		if err != nil {
			return repaired, err
		}
		log.Printf("[DOWNLOAD] moved to ready_to_process job=%s", jobID)
		repaired++
	}

	return repaired, nil
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
		update["$set"].(bson.M)["processing_finished_at"] = completedAt
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
			"message":    errMsg,
			"status":     models.IngestionStatusFailed,
			"stage":      "failed",
			"updated_at": time.Now(),
		},
		"$unset": unset(
			"worker_id",
			"locked_until",
			"download_started_at",
			"last_progress_at",
		),
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
		setFields["message"] = fmt.Sprintf("Retry limit exceeded (%d/%d)", newRetry, maxRetries)
		setFields["completed_at"] = time.Now()
		log.Printf("[REPO] IncrementRetry: job %s reached max retries (%d) — marking as failed (terminal)", id, newRetry)
	} else {
		setFields["status"] = models.IngestionStatusQueued
		setFields["stage"] = "download"
		setFields["progress"] = 0
		setFields["message"] = "Waiting for worker"
		setFields["steps.download"] = false
		log.Printf("[REPO] IncrementRetry: job %s retry %d/%d — resetting to queued", id, newRetry, maxRetries)
	}

	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$inc": bson.M{"retry_count": 1},
		"$set": setFields,
		"$unset": unset(
			"worker_id",
			"locked_until",
			"download_started_at",
			"last_progress_at",
		),
	})

	return err
}

var activeIngestionStatuses = []interface{}{
	models.IngestionStatusDownloading,
	models.IngestionStatusProcessing,
	models.IngestionStatusUploading,
}

func (r *JobRepository) RecoverStaleJobs(ctx context.Context) (int64, error) {
	// Downloads have two watchdog windows:
	//   - zero-byte starts fail/retry after 90 seconds so "Downloading 0%" does
	//     not sit silently until the broad stale sweep.
	//   - downloads that have moved bytes keep the older 5 minute stall window.
	zeroByteStartThreshold := 90 * time.Second
	downloadStaleThreshold := 5 * time.Minute
	zeroByteStartCutoff := time.Now().Add(-zeroByteStartThreshold)
	downloadStaleCutoff := time.Now().Add(-downloadStaleThreshold)
	now := time.Now()
	totalRecovered := int64(0)

	// Match downloading jobs that either never moved bytes in the first 90s, or
	// stopped moving bytes for the broader stale window.
	cursor, err := r.collection.Find(ctx, bson.M{
		"status": models.IngestionStatusDownloading,
		"$or": []bson.M{
			{
				"$and": []bson.M{
					{"progress": bson.M{"$lte": 1}},
					{
						"$or": []bson.M{
							{"downloaded_bytes": bson.M{"$exists": false}},
							{"downloaded_bytes": bson.M{"$lte": int64(0)}},
						},
					},
					{
						"$or": []bson.M{
							{"last_progress_at": bson.M{"$lt": zeroByteStartCutoff}},
							{
								"last_progress_at": bson.M{"$exists": false},
								"updated_at":       bson.M{"$lt": zeroByteStartCutoff},
							},
						},
					},
				},
			},
			{
				"$or": []bson.M{
					{"progress": bson.M{"$gt": 1}},
					{"downloaded_bytes": bson.M{"$gt": int64(0)}},
				},
				"last_progress_at": bson.M{"$lt": downloadStaleCutoff},
			},
			{
				"last_progress_at": bson.M{"$exists": false},
				"updated_at":       bson.M{"$lt": downloadStaleCutoff},
			},
		},
	})
	if err != nil {
		log.Printf("[REPO] RecoverStaleJobs(download): ERROR - %v", err)
		return 0, err
	}
	defer cursor.Close(ctx)

	var staleDownloadJobs []models.IngestionJob
	if err := cursor.All(ctx, &staleDownloadJobs); err != nil {
		log.Printf("[REPO] RecoverStaleJobs(download): decode error - %v", err)
		return 0, err
	}

	const maxRetries = 3
	for _, job := range staleDownloadJobs {
		reason := "Recovered stale download job after no progress for 5 minutes; returned to queue automatically"
		if job.Progress <= 1 && job.DownloadedBytes <= 0 {
			reason = "Download watchdog: progress stayed at 0 with no active downloader PID for 90 seconds; returned to queue automatically"
		}
		newRetry := job.RetryCount + 1
		updateSet := bson.M{
			"updated_at": now,
		}
		// Build the $unset list per-branch. The failed branch SETS completed_at,
		// so it must NOT also appear in $unset — MongoDB rejects the whole
		// update with "Updating the path 'completed_at' would create a conflict
		// at 'completed_at'", which left stuck jobs at retry=2 status=downloading
		// forever (the recovery couldn't progress them to failed or back to queued).
		unsetFields := []string{
			"worker_id",
			"locked_until",
			"download_started_at",
			"last_progress_at",
			"processing_started_at",
		}
		logLevel := "warning"
		logMessage := reason

		if newRetry >= maxRetries {
			finalReason := fmt.Sprintf("%s Retry limit exceeded (%d/%d).", reason, newRetry, maxRetries)
			updateSet["status"] = models.IngestionStatusFailed
			updateSet["stage"] = "failed"
			updateSet["error"] = finalReason
			updateSet["message"] = finalReason
			updateSet["completed_at"] = now
			logMessage = finalReason
			logLevel = "error"
		} else {
			// Re-queued jobs have no completion timestamp; clear any leftover.
			unsetFields = append(unsetFields, "completed_at")
			updateSet["status"] = models.IngestionStatusQueued
			updateSet["stage"] = "download"
			updateSet["progress"] = 0
			updateSet["error"] = reason
			updateSet["message"] = "Waiting for worker"
			updateSet["steps.download"] = false
			updateSet["downloaded_bytes"] = int64(0)
			updateSet["total_bytes"] = int64(0)
			updateSet["speed_mbps"] = 0.0
			updateSet["eta_seconds"] = 0
		}

		_, updateErr := r.collection.UpdateByID(ctx, job.ID, bson.M{
			"$inc":   bson.M{"retry_count": 1},
			"$set":   updateSet,
			"$unset": unset(unsetFields...),
			"$push": bson.M{"logs": models.IngestionLog{
				Timestamp: now,
				Message:   logMessage,
				Level:     logLevel,
			}},
		})
		if updateErr != nil {
			log.Printf("[REPO] RecoverStaleJobs(download): update failed job=%s err=%v", job.ID.Hex(), updateErr)
			continue
		}
		totalRecovered++
	}

	processStages := []struct {
		status    models.IngestionStatus
		threshold time.Duration
		message   string
	}{
		{status: models.IngestionStatusProcessing, threshold: 10 * time.Minute, message: "Processing stage stalled for 10 minutes; returned to processing queue automatically"},
		{status: models.IngestionStatusUploading, threshold: 5 * time.Minute, message: "Upload stage stalled for 5 minutes; returned to processing queue automatically"},
	}
	for _, stage := range processStages {
		stageCutoff := now.Add(-stage.threshold)
		cursor, findErr := r.collection.Find(ctx, bson.M{
			"status":     stage.status,
			"updated_at": bson.M{"$lt": stageCutoff},
			// Exclude serial-parent jobs sitting in waiting_for_episodes —
			// they're not "stuck", they're intentionally waiting for child
			// episode jobs to finish so the finalizer can build Series /
			// Seasons / Episodes from the completed children.
			"stage": bson.M{"$ne": "waiting_for_episodes"},
		})
		if findErr != nil {
			log.Printf("[REPO] RecoverStaleJobs(%s): ERROR - %v", stage.status, findErr)
			return totalRecovered, findErr
		}
		var staleJobs []models.IngestionJob
		if err := cursor.All(ctx, &staleJobs); err != nil {
			cursor.Close(ctx)
			log.Printf("[REPO] RecoverStaleJobs(%s): decode error - %v", stage.status, err)
			return totalRecovered, err
		}
		cursor.Close(ctx)

		for _, job := range staleJobs {
			newRetry := job.RetryCount + 1
			updateSet := bson.M{
				"updated_at": now,
			}
			// Same conflict guard as the download branch: don't $unset
			// completed_at when we're $set-ing it.
			unsetFields := []string{
				"worker_id",
				"locked_until",
				"processing_started_at",
				"last_progress_at",
			}
			logLevel := "warning"
			logMessage := stage.message

			if newRetry >= maxRetries {
				finalReason := fmt.Sprintf("%s Retry limit exceeded (%d/%d).", stage.message, newRetry, maxRetries)
				updateSet["status"] = models.IngestionStatusFailed
				updateSet["stage"] = "failed"
				updateSet["error"] = finalReason
				updateSet["message"] = finalReason
				updateSet["completed_at"] = now
				logMessage = finalReason
				logLevel = "error"
			} else {
				updateSet["status"] = models.IngestionStatusReadyToProcess
				updateSet["stage"] = "ready_to_process"
				updateSet["progress"] = 100
				updateSet["error"] = stage.message
				updateSet["message"] = "Waiting for processing retry"
				unsetFields = append(unsetFields, "completed_at")
			}

			_, updateErr := r.collection.UpdateByID(ctx, job.ID, bson.M{
				"$inc":   bson.M{"retry_count": 1},
				"$set":   updateSet,
				"$unset": unset(unsetFields...),
				"$push": bson.M{"logs": models.IngestionLog{
					Timestamp: now,
					Message:   logMessage,
					Level:     logLevel,
				}},
			})
			if updateErr != nil {
				log.Printf("[REPO] RecoverStaleJobs(%s): update failed job=%s err=%v", stage.status, job.ID.Hex(), updateErr)
				continue
			}
			totalRecovered++
		}
	}

	if totalRecovered > 0 {
		log.Printf("[REPO] RecoverStaleJobs: recovered %d stale jobs", totalRecovered)
	}
	return totalRecovered, nil
}

func (r *JobRepository) FailStaleProcessingJobs(ctx context.Context) (int64, error) {
	return r.RecoverStaleJobs(ctx)
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

// UpdateMasterPlaylistURL persists the final playable master playlist URL
// (the B2/CDN streaming URL in prod) onto the job. The serial finalizer reads
// this field to set each Episode's video_url, so it MUST hold the uploaded
// URL — not the local working-dir path that UpdateQualityInfo writes earlier
// during processing.
func (r *JobRepository) UpdateMasterPlaylistURL(ctx context.Context, id, url string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"master_playlist_url": url,
			"updated_at":          time.Now(),
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
		"stage":            "download",
		"steps.download":   true,
		"updated_at":       now,
	}
	// Only stamp last_progress_at when bytes actually moved forward — this is
	// what the watchdog uses to detect a hung downloader. If the parser keeps
	// responding with the same byte count, updated_at still ticks (heartbeat),
	// but last_progress_at stays frozen and the watchdog will fire.
	if downloadedBytes > currentJob.DownloadedBytes || (totalBytes <= 0 && progress > 0) {
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
	now := time.Now()
	// Aggregation pipeline so download_finished_at / queued_for_processing_at
	// are stamped on the FIRST transition only. If TransitionToProcessing is
	// called again (e.g. parser status sync), keep the original timestamps so
	// queue-wait time stays accurate.
	update := bson.A{
		bson.M{
			"$set": bson.M{
				"status":         models.IngestionStatusReadyToProcess,
				"stage":          "ready_to_process",
				"progress":       100,
				"local_path":     localPath,
				"steps.download": true,
				"error":          "",
				"message":        "Download complete but waiting for processing",
				"worker_id":      "",
				"updated_at":     now,
				// Reset retry_count so the processing claim filter
				// (retry_count<3) doesn't skip jobs that succeeded on a
				// later download attempt. Prior retries were about download
				// failures, not processing — start the processing budget
				// fresh now that we have a valid local_path.
				"retry_count":              0,
				"download_finished_at":     bson.M{"$ifNull": bson.A{"$download_finished_at", now}},
				"queued_for_processing_at": bson.M{"$ifNull": bson.A{"$queued_for_processing_at", now}},
			},
		},
	}

	update = append(update, bson.M{
		"$unset": bson.A{
			"locked_until",
			"last_progress_at",
		},
	})

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

	now := time.Now()
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"status":                   models.IngestionStatusReadyToProcess,
			"stage":                    "ready_to_process",
			"progress":                 100,
			"steps.download":           true, // Keep download as complete
			"steps.process":            false,
			"steps.upload":             false,
			"error":                    "",
			"updated_at":               now,
			"queued_for_processing_at": now,
		},
		"$unset": unset(
			"processing_started_at",
			"processing_finished_at",
		),
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
			"available_qualities": generatedQualities,
			"generated_qualities": generatedQualities,
			"master_playlist_url": masterPlaylistURL,
			"updated_at":          time.Now(),
		},
	}

	_, err = r.collection.UpdateByID(ctx, objID, update)
	return err
}

func (r *JobRepository) UpdateSourceSelection(ctx context.Context, id, selectedVideoURL, selectedQuality string, availableQualities []string, classifierConfidence float64, classifierEvidence string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}

	setFields := bson.M{
		"video_url":           selectedVideoURL,
		"source_quality":      selectedQuality,
		"available_qualities": availableQualities,
		"selected_video_url":  selectedVideoURL,
		"selected_quality":    selectedQuality,
		"updated_at":          time.Now(),
	}
	if classifierConfidence > 0 {
		setFields["classifier_confidence"] = classifierConfidence
	}
	if strings.TrimSpace(classifierEvidence) != "" {
		setFields["classifier_evidence"] = classifierEvidence
	}
	update := bson.M{
		"$set": setFields,
	}
	_, err = r.collection.UpdateByID(ctx, objID, update)
	return err
}

func (r *JobRepository) UpdateSourceResolution(ctx context.Context, id, resolution string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid id")
	}
	_, err = r.collection.UpdateByID(ctx, objID, bson.M{
		"$set": bson.M{
			"source_resolution": resolution,
			"updated_at":        time.Now(),
		},
	})
	return err
}
