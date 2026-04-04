package repositories

import (
	"context"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type MovieViewEventRepository struct {
	col *mongo.Collection
}

func NewMovieViewEventRepository(db *mongo.Database) *MovieViewEventRepository {
	repo := &MovieViewEventRepository{
		col: db.Collection("movie_view_events"),
	}
	repo.ensureIndexes()
	return repo
}

// ensureIndexes creates required indexes on the movie_view_events collection
func (r *MovieViewEventRepository) ensureIndexes() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Index on createdAt for time-based queries
	createdAtIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	}

	// Compound index on movieId + createdAt for efficient grouping
	movieCreatedAtIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "movie_id", Value: 1},
			{Key: "created_at", Value: -1},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{createdAtIndex, movieCreatedAtIndex})
	if err != nil {
		// Log but don't fail - indexes might already exist
	}
}

// RecordViewEvent records a view event after user watches for ~15 seconds
func (r *MovieViewEventRepository) RecordViewEvent(
	ctx context.Context,
	movieID primitive.ObjectID,
	userID primitive.ObjectID,
	sessionID string,
) error {
	event := models.MovieViewEvent{
		MovieID:   movieID,
		UserID:    userID,
		SessionID: sessionID,
		CreatedAt: time.Now(),
	}

	_, err := r.col.InsertOne(ctx, event)
	return err
}

// GetTrendingMovieViews returns view counts grouped by movie for a given time period
func (r *MovieViewEventRepository) GetTrendingMovieViews(ctx context.Context, since time.Time, limit int) ([]struct {
	MovieID   primitive.ObjectID `bson:"_id"`
	ViewCount int64              `bson:"view_count"`
}, error) {
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"created_at": bson.M{"$gte": since},
		}}},
		{{Key: "$group", Value: bson.M{
			"_id":        "$movie_id",
			"view_count": bson.M{"$sum": 1},
		}}},
		{{Key: "$sort", Value: bson.M{
			"view_count": -1,
		}}},
		{{Key: "$limit", Value: limit}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []struct {
		MovieID   primitive.ObjectID `bson:"_id"`
		ViewCount int64              `bson:"view_count"`
	}

	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}
