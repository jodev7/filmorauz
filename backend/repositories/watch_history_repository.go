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

type WatchHistoryRepository struct {
	col      *mongo.Collection
	movieCol *mongo.Collection
}

func NewWatchHistoryRepository(db *mongo.Database) *WatchHistoryRepository {
	return &WatchHistoryRepository{
		col:      db.Collection("watch_history"),
		movieCol: db.Collection("movies"),
	}
}

// UpsertWatchHistory creates or updates watch history for user/movie
func (r *WatchHistoryRepository) UpsertWatchHistory(userID, movieID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// Use upsert to avoid duplicates
	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	// Check if this is a new entry
	var existing models.WatchHistory
	err := r.col.FindOne(ctx, filter).Decode(&existing)
	isNew := err != nil

	update := bson.M{
		"$set": bson.M{
			"watched_at":      now,
			"last_watched_at": now,
			"updated_at":      now,
		},
	}

	if isNew {
		// Set started_at for new entries
		update["$setOnInsert"] = bson.M{
			"user_id":    userID,
			"movie_id":   movieID,
			"created_at": now,
			"started_at": now,
		}
	}

	opts := options.Update().SetUpsert(true)
	_, err = r.col.UpdateOne(ctx, filter, update, opts)
	return err
}

// SaveProgress updates watch progress for a user/movie
func (r *WatchHistoryRepository) SaveProgress(userID, movieID primitive.ObjectID, positionSec, durationSec int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// Calculate progress percentage
	progressPercent := float64(0)
	if durationSec > 0 {
		progressPercent = (float64(positionSec) / float64(durationSec)) * 100
	}

	// Mark as completed if >= 95%
	completed := progressPercent >= 95

	// Use upsert to avoid duplicates
	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	update := bson.M{
		"$set": bson.M{
			"last_position_sec": positionSec,
			"duration_sec":      durationSec,
			"progress_percent":  progressPercent,
			"completed":         completed,
			"last_watched_at":   now,
			"updated_at":        now,
		},
		"$setOnInsert": bson.M{
			"user_id":    userID,
			"movie_id":   movieID,
			"started_at": now,
			"created_at": now,
		},
	}

	opts := options.Update().SetUpsert(true)
	_, err := r.col.UpdateOne(ctx, filter, update, opts)
	return err
}

// MarkComplete marks a watch history entry as completed
func (r *WatchHistoryRepository) MarkComplete(userID, movieID primitive.ObjectID, durationSec int64) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	update := bson.M{
		"$set": bson.M{
			"completed":         true,
			"progress_percent":  100,
			"last_position_sec": durationSec,
			"last_watched_at":   now,
			"updated_at":        now,
		},
	}

	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

// GetContinueWatching returns movies the user is still watching (not completed)
func (r *WatchHistoryRepository) GetContinueWatching(userID primitive.ObjectID, limit int) ([]models.ContinueWatchingItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 10
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{
			"user_id":          userID,
			"completed":        false,
			"progress_percent": bson.M{"$gt": 0, "$lt": 95},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "last_watched_at", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "movies",
			"localField":   "movie_id",
			"foreignField": "_id",
			"as":           "movie",
		}}},
		{{Key: "$unwind", Value: bson.M{
			"path":                       "$movie",
			"preserveNullAndEmptyArrays": false,
		}}},
		{{Key: "$project", Value: bson.M{
			"movie_id":          "$movie_id",
			"title":             "$movie.title",
			"slug":              "$movie.slug",
			"poster_url":        bson.M{"$ifNull": []interface{}{"$movie.poster_url", ""}},
			"last_position_sec": "$last_position_sec",
			"duration_sec":      "$duration_sec",
			"progress_percent":  "$progress_percent",
			"last_watched_at":   "$last_watched_at",
		}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.ContinueWatchingItem
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	if results == nil {
		return []models.ContinueWatchingItem{}, nil
	}

	return results, nil
}

// GetUserWatchHistory returns watch history for a user, sorted by watched_at descending
func (r *WatchHistoryRepository) GetUserWatchHistory(userID primitive.ObjectID, limit int) ([]models.WatchHistoryWithMovie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First get watch history entries
	pipeline := []bson.M{
		{
			"$match": bson.M{"user_id": userID},
		},
		{
			"$sort": bson.D{{Key: "watched_at", Value: -1}},
		},
		{
			"$limit": int64(limit),
		},
		{
			"$lookup": bson.M{
				"from":         "movies",
				"localField":   "movie_id",
				"foreignField": "_id",
				"as":           "movie",
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$movie",
				"preserveNullAndEmptyArrays": false,
			},
		},
		{
			"$project": bson.M{
				"_id":         1,
				"movie_id":    1,
				"watched_at":  1,
				"title":       "$movie.title",
				"poster_url":  bson.M{"$ifNull": []interface{}{"$movie.poster_url", "$movie.posterUrl", ""}},
				"slug":        "$movie.slug",
				"code":        "$movie.code",
				"year":        "$movie.year",
				"quality":     "$movie.quality",
				"website_url": "$movie.website_url",
			},
		},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.WatchHistoryWithMovie
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	log.Printf("[DEBUG] GetUserWatchHistory returned %d items", len(results))
	for i, r := range results {
		log.Printf("[DEBUG] History item %d: id=%s, movie_id=%s, title=%s, poster_url=%s", i, r.ID.Hex(), r.MovieID.Hex(), r.Title, r.PosterURL)
	}

	return results, nil
}

// EnsureIndexes creates required indexes
func (r *WatchHistoryRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "movie_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "watched_at", Value: -1}},
			Options: options.Index(),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}
