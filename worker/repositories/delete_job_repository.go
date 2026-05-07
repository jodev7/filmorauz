package repositories

import (
	"context"
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
