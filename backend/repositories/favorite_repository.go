package repositories

import (
	"context"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type FavoriteRepository struct {
	col      *mongo.Collection
	movieCol *mongo.Collection
}

func NewFavoriteRepository(db *mongo.Database) *FavoriteRepository {
	return &FavoriteRepository{
		col:      db.Collection("favorites"),
		movieCol: db.Collection("movies"),
	}
}

// AddFavorite adds a movie to user's favorites (idempotent)
func (r *FavoriteRepository) AddFavorite(userID, movieID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// Check if already exists
	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	var existing models.Favorite
	err := r.col.FindOne(ctx, filter).Decode(&existing)
	if err == nil {
		// Already exists, no-op (idempotent)
		return nil
	}
	if err != mongo.ErrNoDocuments {
		return err
	}

	// Create new favorite
	favorite := models.Favorite{
		UserID:    userID,
		MovieID:   movieID,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = r.col.InsertOne(ctx, favorite)
	return err
}

// RemoveFavorite removes a movie from user's favorites (safe if not exists)
func (r *FavoriteRepository) RemoveFavorite(userID, movieID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	_, err := r.col.DeleteOne(ctx, filter)
	if err == mongo.ErrNoDocuments {
		// Not found, safe to ignore
		return nil
	}
	return err
}

// GetUserFavorites returns all favorites for a user
func (r *FavoriteRepository) GetUserFavorites(userID primitive.ObjectID) ([]models.FavoriteWithMovie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{
			"$match": bson.M{"user_id": userID},
		},
		{
			"$sort": bson.D{{Key: "created_at", Value: -1}},
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
				"created_at":  1,
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

	var results []models.FavoriteWithMovie
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}

	return results, nil
}

// IsFavorite checks if a movie is in user's favorites
func (r *FavoriteRepository) IsFavorite(userID, movieID primitive.ObjectID) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id":  userID,
		"movie_id": movieID,
	}

	count, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

// EnsureIndexes creates required indexes
func (r *FavoriteRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "movie_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}
