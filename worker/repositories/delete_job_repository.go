package repositories

import (
	"context"
	"time"

	"github.com/filmorauz/worker/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

type DeleteJobRepository struct {
	collection *mongo.Collection
}

func NewDeleteJobRepository(db *mongo.Database) *DeleteJobRepository {
	return &DeleteJobRepository{
		collection: db.Collection("delete_jobs"),
	}
}

func (r *DeleteJobRepository) FindQueued(ctx context.Context) ([]*models.DeleteJob, error) {
	cursor, err := r.collection.Find(ctx, bson.M{"status": "queued"})
	if err != nil {
		return nil, err
	}
	var jobs []*models.DeleteJob
	if err := cursor.All(ctx, &jobs); err != nil {
		return nil, err
	}
	return jobs, nil
}

func (r *DeleteJobRepository) Update(ctx context.Context, job *models.DeleteJob) error {
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": job.ID}, job)
	return err
}

func (r *DeleteJobRepository) FailStaleJobs(ctx context.Context, threshold time.Duration) (int64, error) {
	cutoff := time.Now().Add(-threshold)
	filter := bson.M{
		"status":     "deleting",
		"updated_at": bson.M{"$lt": cutoff},
	}
	update := bson.M{
		"$set": bson.M{
			"status":       "failed",
			"error":        "Deletion timed out after 5 minutes",
			"updated_at":   time.Now(),
		},
	}
	res, err := r.collection.UpdateMany(ctx, filter, update)
	if err != nil {
		return 0, err
	}
	return res.ModifiedCount, nil
}
