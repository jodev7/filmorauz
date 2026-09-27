package repositories

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ─── "Keyinroq ko'raman" (watch later) ───────────────────────────────────────

// WatchlistEntry is one movie or series a user saved to watch later.
type WatchlistEntry struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"user_id" json:"-"`
	TargetType string             `bson:"target_type" json:"target_type"` // "movie" | "series"
	TargetID   primitive.ObjectID `bson:"target_id" json:"target_id"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}

// WatchlistItem is a watchlist entry joined with display fields.
type WatchlistItem struct {
	TargetType string    `json:"target_type"`
	TargetID   string    `json:"target_id"`
	Title      string    `json:"title"`
	TitleUz    string    `json:"title_uz,omitempty"`
	Slug       string    `json:"slug"`
	PosterURL  string    `json:"poster_url"`
	Year       int       `json:"year,omitempty"`
	Quality    string    `json:"quality,omitempty"`
	IsPremium  bool      `json:"is_premium"`
	RatingAvg  float64   `json:"rating_avg"`
	AddedAt    time.Time `json:"added_at"`
}

// ValidLibraryTarget reports whether t is a supported target type.
func ValidLibraryTarget(t string) bool {
	return t == "movie" || t == "series"
}

type LibraryRepository struct {
	watchlist     *mongo.Collection
	subscriptions *mongo.Collection
	movies        *mongo.Collection
	series        *mongo.Collection
}

func NewLibraryRepository(db *mongo.Database) *LibraryRepository {
	return &LibraryRepository{
		watchlist:     db.Collection("watchlist"),
		subscriptions: db.Collection("series_subscriptions"),
		movies:        db.Collection("movies"),
		series:        db.Collection("series"),
	}
}

func (r *LibraryRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if _, err := r.watchlist.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "target_type", Value: 1}, {Key: "target_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}}},
	}); err != nil {
		return err
	}
	_, err := r.subscriptions.Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}, {Key: "series_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "series_id", Value: 1}}},
	})
	return err
}

// AddToWatchlist is idempotent.
func (r *LibraryRepository) AddToWatchlist(ctx context.Context, userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	_, err := r.watchlist.UpdateOne(ctx,
		bson.M{"user_id": userID, "target_type": targetType, "target_id": targetID},
		bson.M{"$setOnInsert": bson.M{"created_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (r *LibraryRepository) RemoveFromWatchlist(ctx context.Context, userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) error {
	_, err := r.watchlist.DeleteOne(ctx, bson.M{"user_id": userID, "target_type": targetType, "target_id": targetID})
	return err
}

func (r *LibraryRepository) InWatchlist(ctx context.Context, userID primitive.ObjectID, targetType string, targetID primitive.ObjectID) (bool, error) {
	n, err := r.watchlist.CountDocuments(ctx, bson.M{"user_id": userID, "target_type": targetType, "target_id": targetID}, options.Count().SetLimit(1))
	return n > 0, err
}

// ListWatchlist returns the user's saved items (newest first) with display
// fields. Entries whose movie/series no longer exists are skipped.
func (r *LibraryRepository) ListWatchlist(ctx context.Context, userID primitive.ObjectID, limit int) ([]WatchlistItem, error) {
	if limit <= 0 || limit > 500 {
		limit = 200
	}
	cursor, err := r.watchlist.Find(ctx, bson.M{"user_id": userID},
		options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var entries []WatchlistEntry
	if err := cursor.All(ctx, &entries); err != nil {
		return nil, err
	}

	refs := make([]TargetRef, 0, len(entries))
	for _, e := range entries {
		refs = append(refs, TargetRef{TargetType: e.TargetType, TargetID: e.TargetID, AddedAt: e.CreatedAt})
	}
	return r.HydrateTargets(ctx, refs)
}

// TargetRef points at a movie or series (with when it was added).
type TargetRef struct {
	TargetType string             `bson:"target_type" json:"target_type"`
	TargetID   primitive.ObjectID `bson:"target_id" json:"target_id"`
	AddedAt    time.Time          `bson:"added_at" json:"added_at"`
}

// HydrateTargets joins refs with display fields, keeping their order and
// dropping ones whose content no longer exists.
func (r *LibraryRepository) HydrateTargets(ctx context.Context, refs []TargetRef) ([]WatchlistItem, error) {
	var movieIDs, seriesIDs []primitive.ObjectID
	for _, e := range refs {
		if e.TargetType == "series" {
			seriesIDs = append(seriesIDs, e.TargetID)
		} else {
			movieIDs = append(movieIDs, e.TargetID)
		}
	}
	proj := bson.M{"title": 1, "title_uz": 1, "slug": 1, "poster_url": 1, "year": 1, "quality": 1, "is_premium": 1, "rating_avg": 1}
	load := func(col *mongo.Collection, ids []primitive.ObjectID) (map[primitive.ObjectID]bson.M, error) {
		out := map[primitive.ObjectID]bson.M{}
		if len(ids) == 0 {
			return out, nil
		}
		cur, err := col.Find(ctx, bson.M{"_id": bson.M{"$in": ids}}, options.Find().SetProjection(proj))
		if err != nil {
			return nil, err
		}
		defer cur.Close(ctx)
		var docs []bson.M
		if err := cur.All(ctx, &docs); err != nil {
			return nil, err
		}
		for _, d := range docs {
			if id, ok := d["_id"].(primitive.ObjectID); ok {
				out[id] = d
			}
		}
		return out, nil
	}
	movies, err := load(r.movies, movieIDs)
	if err != nil {
		return nil, err
	}
	series, err := load(r.series, seriesIDs)
	if err != nil {
		return nil, err
	}

	items := make([]WatchlistItem, 0, len(refs))
	for _, e := range refs {
		src := movies
		if e.TargetType == "series" {
			src = series
		}
		d, ok := src[e.TargetID]
		if !ok {
			continue
		}
		items = append(items, WatchlistItem{
			TargetType: e.TargetType,
			TargetID:   e.TargetID.Hex(),
			Title:      docString(d, "title"),
			TitleUz:    docString(d, "title_uz"),
			Slug:       docString(d, "slug"),
			PosterURL:  docString(d, "poster_url"),
			Year:       int(docInt(d, "year")),
			Quality:    docString(d, "quality"),
			IsPremium:  d["is_premium"] == true,
			RatingAvg:  docFloat(d, "rating_avg"),
			AddedAt:    e.AddedAt,
		})
	}
	return items, nil
}

// ─── Series subscriptions (new-episode notifications) ─────────────────────────

func (r *LibraryRepository) Subscribe(ctx context.Context, userID, seriesID primitive.ObjectID) error {
	_, err := r.subscriptions.UpdateOne(ctx,
		bson.M{"user_id": userID, "series_id": seriesID},
		bson.M{"$setOnInsert": bson.M{"created_at": time.Now()}},
		options.Update().SetUpsert(true),
	)
	if mongo.IsDuplicateKeyError(err) {
		return nil
	}
	return err
}

func (r *LibraryRepository) Unsubscribe(ctx context.Context, userID, seriesID primitive.ObjectID) error {
	_, err := r.subscriptions.DeleteOne(ctx, bson.M{"user_id": userID, "series_id": seriesID})
	return err
}

func (r *LibraryRepository) IsSubscribed(ctx context.Context, userID, seriesID primitive.ObjectID) (bool, error) {
	n, err := r.subscriptions.CountDocuments(ctx, bson.M{"user_id": userID, "series_id": seriesID}, options.Count().SetLimit(1))
	return n > 0, err
}

func (r *LibraryRepository) CountSubscribers(ctx context.Context, seriesID primitive.ObjectID) (int64, error) {
	return r.subscriptions.CountDocuments(ctx, bson.M{"series_id": seriesID})
}

// SubscriberIDs returns the ids of every user subscribed to a series.
func (r *LibraryRepository) SubscriberIDs(ctx context.Context, seriesID primitive.ObjectID) ([]primitive.ObjectID, error) {
	cursor, err := r.subscriptions.Find(ctx, bson.M{"series_id": seriesID}, options.Find().SetProjection(bson.M{"user_id": 1}))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []struct {
		UserID primitive.ObjectID `bson:"user_id"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	ids := make([]primitive.ObjectID, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.UserID)
	}
	return ids, nil
}

// SubscribedSeries lists the series a user follows (newest subscription first).
func (r *LibraryRepository) SubscribedSeries(ctx context.Context, userID primitive.ObjectID) ([]WatchlistItem, error) {
	cursor, err := r.subscriptions.Find(ctx, bson.M{"user_id": userID}, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(200))
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)
	var rows []struct {
		SeriesID  primitive.ObjectID `bson:"series_id"`
		CreatedAt time.Time          `bson:"created_at"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []WatchlistItem{}, nil
	}
	ids := make([]primitive.ObjectID, len(rows))
	for i, row := range rows {
		ids[i] = row.SeriesID
	}
	cur, err := r.series.Find(ctx, bson.M{"_id": bson.M{"$in": ids}},
		options.Find().SetProjection(bson.M{"title": 1, "title_uz": 1, "slug": 1, "poster_url": 1, "year": 1, "is_premium": 1, "rating_avg": 1}))
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var docs []bson.M
	if err := cur.All(ctx, &docs); err != nil {
		return nil, err
	}
	byID := map[primitive.ObjectID]bson.M{}
	for _, d := range docs {
		if id, ok := d["_id"].(primitive.ObjectID); ok {
			byID[id] = d
		}
	}
	out := make([]WatchlistItem, 0, len(rows))
	for _, row := range rows {
		d, ok := byID[row.SeriesID]
		if !ok {
			continue
		}
		out = append(out, WatchlistItem{
			TargetType: "series",
			TargetID:   row.SeriesID.Hex(),
			Title:      docString(d, "title"),
			TitleUz:    docString(d, "title_uz"),
			Slug:       docString(d, "slug"),
			PosterURL:  docString(d, "poster_url"),
			Year:       int(docInt(d, "year")),
			IsPremium:  d["is_premium"] == true,
			RatingAvg:  docFloat(d, "rating_avg"),
			AddedAt:    row.CreatedAt,
		})
	}
	return out, nil
}

// ─── small bson helpers ──────────────────────────────────────────────────────

func docString(d bson.M, key string) string {
	s, _ := d[key].(string)
	return s
}

func docInt(d bson.M, key string) int64 {
	switch v := d[key].(type) {
	case int32:
		return int64(v)
	case int64:
		return v
	case float64:
		return int64(v)
	}
	return 0
}

func docFloat(d bson.M, key string) float64 {
	switch v := d[key].(type) {
	case float64:
		return v
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	}
	return 0
}

// ErrLibraryTargetNotFound is returned when the movie/series doesn't exist.
var ErrLibraryTargetNotFound = errors.New("content not found")

// TargetExists reports whether the movie/series id exists.
func (r *LibraryRepository) TargetExists(ctx context.Context, targetType string, id primitive.ObjectID) (bool, error) {
	col := r.movies
	if targetType == "series" {
		col = r.series
	}
	n, err := col.CountDocuments(ctx, bson.M{"_id": id}, options.Count().SetLimit(1))
	return n > 0, err
}
