package repositories

import (
	"context"
	"errors"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type PublishJobRepository struct {
	col *mongo.Collection
}

func NewPublishJobRepository(db *mongo.Database) *PublishJobRepository {
	return &PublishJobRepository{col: db.Collection("publish_jobs")}
}

func (r *PublishJobRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "clip_id", Value: 1}}},
		{Keys: bson.D{{Key: "status", Value: 1}, {Key: "scheduled_for", Value: 1}}},
		{Keys: bson.D{{Key: "platform", Value: 1}}},
	})
	return err
}

func (r *PublishJobRepository) CreateMany(jobs []*models.PublishJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	docs := make([]interface{}, len(jobs))
	for i, j := range jobs {
		j.ID = primitive.NewObjectID()
		j.CreatedAt = now
		j.UpdatedAt = now
		j.Status = models.PublishJobStatusPending
		docs[i] = j
	}
	_, err := r.col.InsertMany(ctx, docs)
	return err
}

// RecordCompleted inserts a single PublishJob that was already executed (status success or failed).
// Used by UploadNow to persist immediate upload results for all platforms.
func (r *PublishJobRepository) RecordCompleted(job *models.PublishJob) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	if job.ID.IsZero() {
		job.ID = primitive.NewObjectID()
	}
	job.CreatedAt = now
	job.UpdatedAt = now
	if job.ScheduledFor.IsZero() {
		job.ScheduledFor = now
	}
	if job.ExecutedAt == nil {
		job.ExecutedAt = &now
	}
	_, err := r.col.InsertOne(ctx, job)
	return err
}

func (r *PublishJobRepository) FindByClipID(clipID primitive.ObjectID) ([]models.PublishJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cursor, err := r.col.Find(ctx, bson.M{"clip_id": clipID},
		options.Find().SetSort(bson.D{{Key: "scheduled_for", Value: -1}}).SetLimit(50))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var results []models.PublishJob
	return results, cursor.All(ctx, &results)
}

func (r *PublishJobRepository) ListAll(limit, offset int64, platform string) ([]models.PublishJob, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	filter := bson.M{}
	if platform != "" {
		filter["platform"] = platform
	}
	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cursor, err := r.col.Find(ctx, filter,
		options.Find().
			SetSort(bson.D{{Key: "scheduled_for", Value: -1}}).
			SetSkip(offset).
			SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)
	var results []models.PublishJob
	return results, total, cursor.All(ctx, &results)
}

// ClaimDue atomically finds one pending job that is due (scheduled_for <= now)
// and transitions it to "processing". Returns nil if nothing is ready.
func (r *PublishJobRepository) ClaimDue() (*models.PublishJob, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	filter := bson.M{
		"status":        models.PublishJobStatusPending,
		"scheduled_for": bson.M{"$lte": now},
	}
	update := bson.M{"$set": bson.M{
		"status":     models.PublishJobStatusProcessing,
		"updated_at": now,
	}}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	var j models.PublishJob
	err := r.col.FindOneAndUpdate(ctx, filter, update, opts).Decode(&j)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *PublishJobRepository) MarkSuccess(id primitive.ObjectID, mediaID, postURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	set := bson.M{
		"status":       models.PublishJobStatusSuccess,
		"executed_at":  now,
		"updated_at":   now,
		"published_at": now,
		"error":        "",
	}
	if mediaID != "" {
		set["instagram_media_id"] = mediaID
	}
	if postURL != "" {
		set["instagram_post_url"] = postURL
	}
	_, err := r.col.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	return err
}

// MarkFailed will not overwrite a row that has already been marked success.
func (r *PublishJobRepository) MarkFailed(id primitive.ObjectID, errMsg string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now().UTC()
	_, err := r.col.UpdateOne(ctx,
		bson.M{"_id": id, "status": bson.M{"$ne": models.PublishJobStatusSuccess}},
		bson.M{
			"$set": bson.M{
				"status":      models.PublishJobStatusFailed,
				"error":       errMsg,
				"executed_at": now,
				"updated_at":  now,
			},
			"$inc": bson.M{"retry_count": 1},
		})
	return err
}

// UpdateScheduledTime changes the scheduled_for of a pending job.
// Returns false if not found or already executing/done.
func (r *PublishJobRepository) UpdateScheduledTime(id primitive.ObjectID, newTime time.Time) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.UpdateOne(ctx,
		bson.M{"_id": id, "status": models.PublishJobStatusPending},
		bson.M{"$set": bson.M{"scheduled_for": newTime.UTC(), "updated_at": time.Now().UTC()}},
	)
	if err != nil {
		return false, err
	}
	return res.MatchedCount > 0, nil
}

// Cancel deletes a pending job. Returns false if not found or already executing/done.
func (r *PublishJobRepository) Cancel(id primitive.ObjectID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	res, err := r.col.DeleteOne(ctx, bson.M{"_id": id, "status": models.PublishJobStatusPending})
	if err != nil {
		return false, err
	}
	return res.DeletedCount > 0, nil
}

// DeleteByClipIDs removes every publish job (regardless of status) whose
// clip_id is in the given list. Used when a movie or series is deleted
// so the schedule queue does not later try to upload a clip that no
// longer exists.
//
// Returns the number of documents deleted.
func (r *PublishJobRepository) DeleteByClipIDs(ctx context.Context, clipIDs []primitive.ObjectID) (int64, error) {
	if len(clipIDs) == 0 {
		return 0, nil
	}
	res, err := r.col.DeleteMany(ctx, bson.M{"clip_id": bson.M{"$in": clipIDs}})
	if err != nil {
		return 0, err
	}
	return res.DeletedCount, nil
}
