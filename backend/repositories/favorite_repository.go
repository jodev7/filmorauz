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
	col        *mongo.Collection
	movieCol   *mongo.Collection
	episodeCol *mongo.Collection
	seriesCol  *mongo.Collection
}

func NewFavoriteRepository(db *mongo.Database) *FavoriteRepository {
	return &FavoriteRepository{
		col:        db.Collection("favorites"),
		movieCol:   db.Collection("movies"),
		episodeCol: db.Collection("episodes"),
		seriesCol:  db.Collection("series"),
	}
}

func normalizeFavoriteTargetType(targetType string) string {
	switch targetType {
	case "episode", "series":
		return targetType
	default:
		return "movie"
	}
}

func (r *FavoriteRepository) favoriteLookupFilter(userID, targetID primitive.ObjectID, targetType string) bson.M {
	targetType = normalizeFavoriteTargetType(targetType)
	if targetType == "movie" {
		return bson.M{
			"user_id": userID,
			"$or": []bson.M{
				{"target_type": "movie", "target_id": targetID},
				{"movie_id": targetID},
			},
		}
	}
	return bson.M{
		"user_id":     userID,
		"target_type": targetType,
		"target_id":   targetID,
	}
}

// AddFavorite adds a target to user's favorites (idempotent).
func (r *FavoriteRepository) AddFavorite(userID, targetID primitive.ObjectID, targetType string, seriesID, seasonID *primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	targetType = normalizeFavoriteTargetType(targetType)

	var existing models.Favorite
	err := r.col.FindOne(ctx, r.favoriteLookupFilter(userID, targetID, targetType)).Decode(&existing)
	if err == nil {
		update := bson.M{
			"$set": bson.M{
				"target_type": targetType,
				"target_id":   targetID,
				"updated_at":  time.Now(),
			},
		}
		if targetType == "movie" {
			update["$set"].(bson.M)["movie_id"] = targetID
		}
		if seriesID != nil {
			update["$set"].(bson.M)["series_id"] = *seriesID
		}
		if seasonID != nil {
			update["$set"].(bson.M)["season_id"] = *seasonID
		}
		_, err = r.col.UpdateByID(ctx, existing.ID, update)
		return err
	}
	if err != mongo.ErrNoDocuments {
		return err
	}

	now := time.Now()
	favorite := models.Favorite{
		UserID:     userID,
		TargetType: targetType,
		TargetID:   targetID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if targetType == "movie" {
		favorite.MovieID = targetID
	}
	if seriesID != nil {
		favorite.SeriesID = *seriesID
	}
	if seasonID != nil {
		favorite.SeasonID = *seasonID
	}

	_, err = r.col.InsertOne(ctx, favorite)
	return err
}

// RemoveFavorite removes a target from user's favorites (safe if not exists).
func (r *FavoriteRepository) RemoveFavorite(userID, targetID primitive.ObjectID, targetType string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.DeleteMany(ctx, r.favoriteLookupFilter(userID, targetID, targetType))
	if err == mongo.ErrNoDocuments {
		return nil
	}
	return err
}

// GetUserFavorites returns all favorites for a user.
func (r *FavoriteRepository) GetUserFavorites(userID primitive.ObjectID) ([]models.FavoriteListItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"user_id": userID}}},
		{{Key: "$addFields", Value: bson.M{
			"effective_target_type": bson.M{
				"$cond": bson.M{
					"if":   bson.M{"$eq": []interface{}{"$target_type", ""}},
					"then": "movie",
					"else": bson.M{"$ifNull": []interface{}{"$target_type", "movie"}},
				},
			},
			"effective_target_id": bson.M{"$ifNull": []interface{}{"$target_id", "$movie_id"}},
			"effective_movie_id":  bson.M{"$ifNull": []interface{}{"$movie_id", "$target_id"}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "created_at", Value: -1}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "movies",
			"localField":   "effective_movie_id",
			"foreignField": "_id",
			"as":           "movie",
		}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "episodes",
			"localField":   "effective_target_id",
			"foreignField": "_id",
			"as":           "episode",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$movie", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$unwind", Value: bson.M{"path": "$episode", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "series",
			"localField":   "episode.series_id",
			"foreignField": "_id",
			"as":           "episode_series",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$episode_series", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$match", Value: bson.M{
			"$or": []bson.M{
				{"effective_target_type": "movie", "movie._id": bson.M{"$ne": nil}},
				{"effective_target_type": "episode", "episode._id": bson.M{"$ne": nil}},
			},
		}}},
		{{Key: "$project", Value: bson.M{
			"_id":         1,
			"target_type": "$effective_target_type",
			"target_id":   "$effective_target_id",
			"movie_id":    "$effective_movie_id",
			"series_id":   bson.M{"$ifNull": []interface{}{"$series_id", "$episode.series_id"}},
			"season_id":   bson.M{"$ifNull": []interface{}{"$season_id", "$episode.season_id"}},
			"created_at":  1,
			"title": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode.title",
				"$movie.title",
			}},
			"poster_url": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_series.poster_url", "$episode_series.backdrop_url", "$episode.thumbnail_url", ""}},
				bson.M{"$ifNull": []interface{}{"$movie.poster_url", "$movie.posterUrl", ""}},
			}},
			"backdrop_url": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_series.backdrop_url", "$episode.thumbnail_url", ""}},
				bson.M{"$ifNull": []interface{}{"$movie.backdrop_url", ""}},
			}},
			"slug": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$toString": "$effective_target_id"},
				"$movie.slug",
			}},
			"code": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_series.code", ""}},
				bson.M{"$ifNull": []interface{}{"$movie.code", ""}},
			}},
			"year": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_series.year", 0}},
				bson.M{"$ifNull": []interface{}{"$movie.year", 0}},
			}},
			"quality":     bson.M{"$ifNull": []interface{}{"$movie.quality", ""}},
			"website_url": bson.M{"$ifNull": []interface{}{"$movie.website_url", ""}},
			"type":        "$effective_target_type",
			"series_title": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode_series.title",
				"",
			}},
			"series_slug": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode_series.slug",
				"",
			}},
			"episode_number": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode.episode_number", 0}},
				0,
			}},
		}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.FavoriteListItem
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if results == nil {
		return []models.FavoriteListItem{}, nil
	}
	return results, nil
}

// IsFavorite checks if a target is in user's favorites.
func (r *FavoriteRepository) IsFavorite(userID, targetID primitive.ObjectID, targetType string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := r.col.CountDocuments(ctx, r.favoriteLookupFilter(userID, targetID, targetType))
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

// EnsureIndexes creates required indexes.
func (r *FavoriteRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"target_id": bson.M{"$exists": true}}),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "movie_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetPartialFilterExpression(bson.M{"movie_id": bson.M{"$exists": true}}),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}
