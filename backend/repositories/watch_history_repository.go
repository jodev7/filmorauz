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

type WatchHistoryRepository struct {
	col        *mongo.Collection
	movieCol   *mongo.Collection
	episodeCol *mongo.Collection
	seriesCol  *mongo.Collection
}

func NewWatchHistoryRepository(db *mongo.Database) *WatchHistoryRepository {
	return &WatchHistoryRepository{
		col:        db.Collection("watch_history"),
		movieCol:   db.Collection("movies"),
		episodeCol: db.Collection("episodes"),
		seriesCol:  db.Collection("series"),
	}
}

func normalizeWatchTargetType(targetType string) string {
	switch targetType {
	case "episode", "series":
		return targetType
	default:
		return "movie"
	}
}

func (r *WatchHistoryRepository) watchLookupFilter(userID, targetID primitive.ObjectID, targetType string) bson.M {
	targetType = normalizeWatchTargetType(targetType)
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

func (r *WatchHistoryRepository) findExistingID(ctx context.Context, userID, targetID primitive.ObjectID, targetType string) (*primitive.ObjectID, error) {
	var existing struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	err := r.col.FindOne(ctx, r.watchLookupFilter(userID, targetID, targetType), options.FindOne().SetProjection(bson.M{"_id": 1})).Decode(&existing)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &existing.ID, nil
}

func (r *WatchHistoryRepository) upsertHistory(
	userID, targetID primitive.ObjectID,
	targetType string,
	seriesID, seasonID, episodeID *primitive.ObjectID,
	positionSec, durationSec int64,
	progressPercent float64,
	completed bool,
	setStartedAt bool,
) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	targetType = normalizeWatchTargetType(targetType)
	now := time.Now()

	setFields := bson.M{
		"target_type":       targetType,
		"target_id":         targetID,
		"last_position_sec": positionSec,
		"duration_sec":      durationSec,
		"progress_percent":  progressPercent,
		"completed":         completed,
		"watched_at":        now,
		"last_watched_at":   now,
		"updated_at":        now,
	}
	if targetType == "movie" {
		setFields["movie_id"] = targetID
	}
	if seriesID != nil {
		setFields["series_id"] = *seriesID
	}
	if seasonID != nil {
		setFields["season_id"] = *seasonID
	}
	if episodeID != nil {
		setFields["episode_id"] = *episodeID
	}

	setOnInsert := bson.M{
		"user_id":    userID,
		"created_at": now,
	}
	if setStartedAt {
		setOnInsert["started_at"] = now
	}

	existingID, err := r.findExistingID(ctx, userID, targetID, targetType)
	if err != nil {
		return err
	}

	filter := bson.M(r.watchLookupFilter(userID, targetID, targetType))
	opts := options.Update().SetUpsert(true)
	if existingID != nil {
		filter = bson.M{"_id": *existingID}
		opts = options.Update()
	}

	update := bson.M{
		"$set":         setFields,
		"$setOnInsert": setOnInsert,
	}

	_, err = r.col.UpdateOne(ctx, filter, update, opts)
	return err
}

// UpsertWatchHistory creates or updates watch history for a user/target.
func (r *WatchHistoryRepository) UpsertWatchHistory(
	userID, targetID primitive.ObjectID,
	targetType string,
	seriesID, seasonID, episodeID *primitive.ObjectID,
) error {
	return r.upsertHistory(userID, targetID, targetType, seriesID, seasonID, episodeID, 0, 0, 0, false, true)
}

// SaveProgress updates watch progress for a user/target.
func (r *WatchHistoryRepository) SaveProgress(
	userID, targetID primitive.ObjectID,
	targetType string,
	seriesID, seasonID, episodeID *primitive.ObjectID,
	positionSec, durationSec int64,
) error {
	existing, err := r.GetProgress(userID, targetID, targetType)
	if err != nil {
		return err
	}
	if existing != nil && existing.CurrentTime > 30 && positionSec <= 1 {
		return nil
	}
	progressPercent := float64(0)
	if durationSec > 0 {
		progressPercent = (float64(positionSec) / float64(durationSec)) * 100
	}
	completed := progressPercent >= 90
	return r.upsertHistory(userID, targetID, targetType, seriesID, seasonID, episodeID, positionSec, durationSec, progressPercent, completed, true)
}

func (r *WatchHistoryRepository) ResetProgress(
	userID, targetID primitive.ObjectID,
	targetType string,
	seriesID, seasonID, episodeID *primitive.ObjectID,
) error {
	return r.upsertHistory(userID, targetID, targetType, seriesID, seasonID, episodeID, 0, 0, 0, false, true)
}

// MarkComplete marks a watch history entry as completed.
func (r *WatchHistoryRepository) MarkComplete(
	userID, targetID primitive.ObjectID,
	targetType string,
	seriesID, seasonID, episodeID *primitive.ObjectID,
	durationSec int64,
) error {
	return r.upsertHistory(userID, targetID, targetType, seriesID, seasonID, episodeID, durationSec, durationSec, 100, true, false)
}

// GetContinueWatching returns items the user is still watching (not completed).
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
			"progress_percent": bson.M{"$gt": 0, "$lt": 90},
		}}},
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
		{{Key: "$sort", Value: bson.D{{Key: "last_watched_at", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
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
		{{Key: "$lookup", Value: bson.M{
			"from":         "seasons",
			"localField":   "episode.season_id",
			"foreignField": "_id",
			"as":           "episode_season",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$episode_season", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$match", Value: bson.M{
			"$or": []bson.M{
				{"effective_target_type": "movie", "movie._id": bson.M{"$ne": nil}},
				{"effective_target_type": "episode", "episode._id": bson.M{"$ne": nil}},
			},
		}}},
		{{Key: "$project", Value: bson.M{
			"movie_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "movie"}},
				bson.M{"$toString": "$effective_movie_id"},
				"",
			}},
			"target_type": bson.M{"$ifNull": []interface{}{"$effective_target_type", "movie"}},
			"target_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$toString": "$episode._id"},
				bson.M{"$toString": "$effective_target_id"},
			}},
			"episode_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$toString": "$episode._id"},
				"",
			}},
			"title": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode.title",
				"$movie.title",
			}},
			"slug": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$toString": "$effective_target_id"},
				"$movie.slug",
			}},
			"poster_url": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_series.poster_url", "$episode_series.backdrop_url", "$episode.thumbnail_url", ""}},
				bson.M{"$ifNull": []interface{}{"$movie.poster_url", ""}},
			}},
			"type":              "$effective_target_type",
			"series_title":      "$episode_series.title",
			"series_slug":       "$episode_series.slug",
			"season_number":     bson.M{"$ifNull": []interface{}{"$episode_season.season_number", 0}},
			"episode_number":    bson.M{"$ifNull": []interface{}{"$episode.episode_number", 0}},
			"episode_title":     "$episode.title",
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

func (r *WatchHistoryRepository) GetProgress(
	userID, targetID primitive.ObjectID,
	targetType string,
) (*models.WatchProgressResponse, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var item models.WatchHistory
	err := r.col.FindOne(ctx, r.watchLookupFilter(userID, targetID, targetType)).Decode(&item)
	if err == mongo.ErrNoDocuments {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &models.WatchProgressResponse{
		CurrentTime:     item.LastPositionSec,
		Duration:        item.DurationSec,
		ProgressPercent: item.ProgressPercent,
		Completed:       item.Completed,
	}, nil
}

// GetUserWatchHistory returns watch history for a user, sorted by watched_at descending.
func (r *WatchHistoryRepository) GetUserWatchHistory(userID primitive.ObjectID, limit int) ([]models.WatchHistoryListItem, error) {
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
		{{Key: "$sort", Value: bson.D{{Key: "watched_at", Value: -1}, {Key: "last_watched_at", Value: -1}}}},
		{{Key: "$limit", Value: int64(limit)}},
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
		{{Key: "$lookup", Value: bson.M{
			"from":         "seasons",
			"localField":   "episode.season_id",
			"foreignField": "_id",
			"as":           "episode_season",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$episode_season", "preserveNullAndEmptyArrays": true}}},
		{{Key: "$match", Value: bson.M{
			"$or": []bson.M{
				{"effective_target_type": "movie", "movie._id": bson.M{"$ne": nil}},
				{"effective_target_type": "episode", "episode._id": bson.M{"$ne": nil}},
			},
		}}},
		{{Key: "$project", Value: bson.M{
			"_id":       1,
			"record_id": "$_id",
			"movie_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "movie"}},
				"$effective_movie_id",
				primitive.NilObjectID,
			}},
			"target_type": "$effective_target_type",
			"target_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode._id",
				"$effective_target_id",
			}},
			"series_id": bson.M{"$ifNull": []interface{}{"$series_id", "$episode.series_id"}},
			"season_id": bson.M{"$ifNull": []interface{}{"$season_id", "$episode.season_id"}},
			"episode_id": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode._id",
				primitive.NilObjectID,
			}},
			"watched_at":        bson.M{"$ifNull": []interface{}{"$watched_at", "$last_watched_at", "$updated_at"}},
			"last_position_sec": bson.M{"$ifNull": []interface{}{"$last_position_sec", 0}},
			"duration_sec":      bson.M{"$ifNull": []interface{}{"$duration_sec", 0}},
			"progress_percent":  bson.M{"$ifNull": []interface{}{"$progress_percent", 0}},
			"completed":         bson.M{"$ifNull": []interface{}{"$completed", false}},
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
			"season_number": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode_season.season_number", 0}},
				0,
			}},
			"episode_number": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				bson.M{"$ifNull": []interface{}{"$episode.episode_number", 0}},
				0,
			}},
			"episode_title": bson.M{"$cond": []interface{}{
				bson.M{"$eq": []interface{}{"$effective_target_type", "episode"}},
				"$episode.title",
				"",
			}},
		}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.WatchHistoryListItem
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if results == nil {
		return []models.WatchHistoryListItem{}, nil
	}
	return results, nil
}

// EnsureIndexes creates required indexes.
func (r *WatchHistoryRepository) EnsureIndexes() error {
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
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "watched_at", Value: -1}},
			Options: options.Index(),
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}
