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

// EpisodeRating mirrors MovieRating but stored under `episode_id`.
type EpisodeRating struct {
	ID        primitive.ObjectID `bson:"_id,omitempty"`
	EpisodeID primitive.ObjectID `bson:"episode_id"`
	UserID    primitive.ObjectID `bson:"user_id"`
	Rating    int                `bson:"rating"`
	CreatedAt time.Time          `bson:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at"`
}

type EpisodeRatingRepository struct {
	col        *mongo.Collection
	episodeCol *mongo.Collection
}

func NewEpisodeRatingRepository(db *mongo.Database) *EpisodeRatingRepository {
	return &EpisodeRatingRepository{
		col:        db.Collection("episode_ratings"),
		episodeCol: db.Collection("episodes"),
	}
}

func (r *EpisodeRatingRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{Keys: bson.D{{Key: "episode_id", Value: 1}, {Key: "user_id", Value: 1}}, Options: options.Index().SetUnique(true)},
		{Keys: bson.D{{Key: "episode_id", Value: 1}}},
	})
	return err
}

func (r *EpisodeRatingRepository) UpsertRating(userID, episodeID primitive.ObjectID, rating int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	now := time.Now()
	filter := bson.M{"user_id": userID, "episode_id": episodeID}
	update := bson.M{
		"$set":         bson.M{"rating": rating, "updated_at": now},
		"$setOnInsert": bson.M{"created_at": now, "user_id": userID, "episode_id": episodeID},
	}
	_, err := r.col.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

func (r *EpisodeRatingRepository) DeleteRating(userID, episodeID primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := r.col.DeleteOne(ctx, bson.M{"user_id": userID, "episode_id": episodeID})
	if err == mongo.ErrNoDocuments {
		return nil
	}
	return err
}

func (r *EpisodeRatingRepository) GetUserRating(userID, episodeID primitive.ObjectID) (*int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var er EpisodeRating
	err := r.col.FindOne(ctx, bson.M{"user_id": userID, "episode_id": episodeID}).Decode(&er)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &er.Rating, nil
}

func (r *EpisodeRatingRepository) GetRatingSummary(episodeID primitive.ObjectID) (float64, int64, error) {
	return r.aggregate(bson.M{"episode_id": episodeID})
}

// GetSeriesRatingFromEpisodes computes the average across all ratings for episodes
// belonging to the given series.
func (r *EpisodeRatingRepository) GetSeriesRatingFromEpisodes(seriesID primitive.ObjectID) (float64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cursor, err := r.episodeCol.Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetProjection(bson.M{"_id": 1}))
	if err != nil {
		return 0, 0, err
	}
	defer cursor.Close(ctx)

	var ids []primitive.ObjectID
	for cursor.Next(ctx) {
		var doc struct {
			ID primitive.ObjectID `bson:"_id"`
		}
		if err := cursor.Decode(&doc); err != nil {
			return 0, 0, err
		}
		ids = append(ids, doc.ID)
	}
	if len(ids) == 0 {
		return 0, 0, nil
	}
	return r.aggregate(bson.M{"episode_id": bson.M{"$in": ids}})
}

func (r *EpisodeRatingRepository) aggregate(match bson.M) (float64, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	pipeline := []bson.M{
		{"$match": match},
		{"$group": bson.M{"_id": nil, "avg": bson.M{"$avg": "$rating"}, "count": bson.M{"$sum": 1}}},
	}
	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, 0, err
	}
	defer cursor.Close(ctx)
	if !cursor.Next(ctx) {
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

// Avoid unused import warning when EpisodeRating struct fields aren't used elsewhere.
var _ = models.MovieRating{}
