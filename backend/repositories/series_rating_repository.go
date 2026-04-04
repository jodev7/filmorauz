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

type SeriesRatingRepository struct {
	col       *mongo.Collection
	seriesCol *mongo.Collection
}

func NewSeriesRatingRepository(db *mongo.Database) *SeriesRatingRepository {
	return &SeriesRatingRepository{
		col:       db.Collection("series_ratings"),
		seriesCol: db.Collection("series"),
	}
}

// EnsureIndexes creates required indexes on the series_ratings collection
func (r *SeriesRatingRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "series_id", Value: 1}, {Key: "user_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "series_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "user_id", Value: 1}},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

// UpsertRating creates or updates a user's rating for a series
// Returns true if a new rating was created, false if updated
func (r *SeriesRatingRepository) UpsertRating(userID, seriesID primitive.ObjectID, rating int) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()

	// Try to find existing rating
	filter := bson.M{
		"user_id":   userID,
		"series_id": seriesID,
	}

	var existing models.SeriesRating
	err := r.col.FindOne(ctx, filter).Decode(&existing)
	if err == nil {
		// Rating exists - update it
		update := bson.M{
			"$set": bson.M{
				"rating":     rating,
				"updated_at": now,
			},
		}
		_, err = r.col.UpdateOne(ctx, filter, update)
		if err != nil {
			return false, err
		}
		return false, nil
	}
	if err != mongo.ErrNoDocuments {
		return false, err
	}

	// Create new rating
	newRating := models.SeriesRating{
		SeriesID:  seriesID,
		UserID:    userID,
		Rating:    rating,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err = r.col.InsertOne(ctx, newRating)
	if err != nil {
		return false, err
	}

	return true, nil
}

// DeleteRating removes a user's rating for a series
func (r *SeriesRatingRepository) DeleteRating(userID, seriesID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id":   userID,
		"series_id": seriesID,
	}

	_, err := r.col.DeleteOne(ctx, filter)
	if err == mongo.ErrNoDocuments {
		return nil // Already deleted
	}
	return err
}

// GetUserRating returns a user's rating for a series (nil if not rated)
func (r *SeriesRatingRepository) GetUserRating(userID, seriesID primitive.ObjectID) (*int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"user_id":   userID,
		"series_id": seriesID,
	}

	var rating models.SeriesRating
	err := r.col.FindOne(ctx, filter).Decode(&rating)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rating.Rating, nil
}

// GetRatingSummary returns the average rating and count for a series
func (r *SeriesRatingRepository) GetRatingSummary(seriesID primitive.ObjectID) (float64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{
			"$match": bson.M{"series_id": seriesID},
		},
		{
			"$group": bson.M{
				"_id":   "$series_id",
				"avg":   bson.M{"$avg": "$rating"},
				"count": bson.M{"$sum": 1},
			},
		},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, err
	}
	defer cursor.Close(ctx)

	if !cursor.Next(ctx) {
		// No ratings yet
		return 0, 0, nil
	}

	var result struct {
		Avg   float64 `bson:"avg"`
		Count int64   `bson:"count"`
	}
	if err := cursor.Decode(&result); err != nil {
		return 0, 0, err
	}

	return result.Avg, result.Count, nil
}

// UpdateSeriesRatingFields updates the rating_avg and rating_count fields on a series
func (r *SeriesRatingRepository) UpdateSeriesRatingFields(seriesID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	avg, count, err := r.GetRatingSummary(seriesID)
	if err != nil {
		return err
	}

	filter := bson.M{"_id": seriesID}
	update := bson.M{
		"$set": bson.M{
			"rating_avg":   avg,
			"rating_count": count,
			"updated_at":   time.Now(),
		},
	}

	_, err = r.seriesCol.UpdateOne(ctx, filter, update)
	return err
}
