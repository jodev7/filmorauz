package repositories

import (
	"context"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

func (r *DeleteJobRepository) Create(ctx context.Context, job *models.DeleteJob) error {
	job.ID = primitive.NewObjectID()
	_, err := r.collection.InsertOne(ctx, job)
	return err
}

func (r *DeleteJobRepository) GetByID(ctx context.Context, id primitive.ObjectID) (*models.DeleteJob, error) {
	var job models.DeleteJob
	err := r.collection.FindOne(ctx, bson.M{"_id": id}).Decode(&job)
	return &job, err
}

func (r *DeleteJobRepository) FindPending(ctx context.Context, contentID primitive.ObjectID) (*models.DeleteJob, error) {
	var job models.DeleteJob
	err := r.collection.FindOne(ctx, bson.M{"content_id": contentID, "status": bson.M{"$in": []string{"queued", "deleting"}}}).Decode(&job)
	return &job, err
}

func (r *DeleteJobRepository) Update(ctx context.Context, job *models.DeleteJob) error {
	_, err := r.collection.ReplaceOne(ctx, bson.M{"_id": job.ID}, job)
	return err
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
