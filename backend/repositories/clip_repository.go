package repositories

import (
	"context"
	"log"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type ClipRepository struct {
	col *mongo.Collection
}

func NewClipRepository(db *mongo.Database) *ClipRepository {
	col := db.Collection("clips")
	return &ClipRepository{col: col}
}

func (r *ClipRepository) Collection() *mongo.Collection {
	return r.col
}

func (r *ClipRepository) Create(ctx context.Context, clip *models.Clip) error {
	clip.CreatedAt = time.Now()
	result, err := r.col.InsertOne(ctx, clip)
	if err != nil {
		return err
	}
	clip.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *ClipRepository) CreateMany(ctx context.Context, clips []models.Clip) error {
	if len(clips) == 0 {
		return nil
	}
	docs := make([]interface{}, len(clips))
	now := time.Now()
	for i := range clips {
		clips[i].CreatedAt = now
		docs[i] = clips[i]
	}
	_, err := r.col.InsertMany(ctx, docs)
	return err
}

func (r *ClipRepository) FindByMovieID(ctx context.Context, movieID primitive.ObjectID) ([]models.Clip, error) {
	cursor, err := r.col.Find(ctx, bson.M{"movie_id": movieID}, options.Find().SetSort(bson.M{"sequence": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, err
	}
	return clips, nil
}

func (r *ClipRepository) List(ctx context.Context, limit, offset int64) ([]models.Clip, int64, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > 100 {
		limit = 100
	}

	opts := options.Find().
		SetSort(bson.M{"created_at": -1}).
		SetLimit(limit).
		SetSkip(offset)

	cursor, err := r.col.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, 0, err
	}
	defer cursor.Close(ctx)

	var clips []models.Clip
	if err := cursor.All(ctx, &clips); err != nil {
		return nil, 0, err
	}

	total, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		log.Printf("[ClipRepo] Warning: failed to count clips: %v", err)
		total = int64(len(clips))
	}

	return clips, total, nil
}

func (r *ClipRepository) DeleteByMovieID(ctx context.Context, movieID primitive.ObjectID) error {
	_, err := r.col.DeleteMany(ctx, bson.M{"movie_id": movieID})
	return err
}

func (r *ClipRepository) CountByMovieID(ctx context.Context, movieID primitive.ObjectID) (int64, error) {
	return r.col.CountDocuments(ctx, bson.M{"movie_id": movieID})
}
