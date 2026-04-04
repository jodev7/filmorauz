package repositories

import (
	"context"
	"errors"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/filmorauz/backend/models"
)

type ShareRepository struct {
	col *mongo.Collection
	db  *mongo.Database
}

func NewShareRepository(db *mongo.Database) *ShareRepository {
	return &ShareRepository{
		col: db.Collection("shares"),
		db:  db,
	}
}

// EnsureIndexes creates necessary indexes for the shares collection
func (r *ShareRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "movie_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_by_user_id", Value: 1}},
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
	}

	_, err := r.col.Indexes().CreateMany(ctx, indexes)
	return err
}

// Create creates a new share
func (r *ShareRepository) Create(share *models.Share) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	share.CreatedAt = time.Now()
	share.Clicks = 0

	result, err := r.col.InsertOne(ctx, share)
	if err != nil {
		return err
	}

	share.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

// FindByCode finds a share by its code
func (r *ShareRepository) FindByCode(code string) (*models.Share, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var share models.Share
	err := r.col.FindOne(ctx, bson.M{"code": code}).Decode(&share)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, err
	}
	return &share, nil
}

// FindByMovieID finds all shares for a movie
func (r *ShareRepository) FindByMovieID(movieID primitive.ObjectID) ([]models.Share, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{"movie_id": movieID})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var shares []models.Share
	if err := cursor.All(ctx, &shares); err != nil {
		return nil, err
	}

	return shares, nil
}

// IncrementClicks increments the click count for a share
func (r *ShareRepository) IncrementClicks(code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now()
	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"code": code},
		bson.M{
			"$inc": bson.M{"clicks": 1},
			"$set": bson.M{"last_clicked_at": now},
		},
	)
	return err
}

// GetMovieShareStats returns share statistics for a movie
func (r *ShareRepository) GetMovieShareStats(movieID primitive.ObjectID) (*models.ShareStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Count shares created for this movie
	sharesCreatedCount, err := r.col.CountDocuments(ctx, bson.M{"movie_id": movieID})
	if err != nil {
		return nil, err
	}

	// Sum all clicks for shares of this movie
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"movie_id": movieID}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "total_clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}}}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		TotalClicks int64 `bson:"total_clicks"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}

	var totalClicks int64
	if len(result) > 0 {
		totalClicks = result[0].TotalClicks
	}

	return &models.ShareStats{
		SharesCreatedCount: sharesCreatedCount,
		TotalShareOpens:    totalClicks,
	}, nil
}

// GetUserShareStats returns share statistics for a user
func (r *ShareRepository) GetUserShareStats(userID primitive.ObjectID) (*models.UserShareStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Count total shares created by user
	sharesCreatedCount, err := r.col.CountDocuments(ctx, bson.M{"created_by_user_id": userID})
	if err != nil {
		return nil, err
	}

	// Sum all clicks for shares created by user
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_by_user_id": userID}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "total_clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}}}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		TotalClicks int64 `bson:"total_clicks"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}

	var totalClicks int64
	if len(result) > 0 {
		totalClicks = result[0].TotalClicks
	}

	// Get unique movies shared
	uniqueMoviesPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_by_user_id": userID}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$movie_id"}}}},
	}

	uniqueMoviesCursor, err := r.col.Aggregate(ctx, uniqueMoviesPipeline)
	if err != nil {
		return nil, err
	}
	defer uniqueMoviesCursor.Close(ctx)

	var uniqueMovies []struct {
		ID primitive.ObjectID `bson:"_id"`
	}
	if err := uniqueMoviesCursor.All(ctx, &uniqueMovies); err != nil {
		return nil, err
	}

	// Get top shared movie
	topMoviePipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_by_user_id": userID}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$movie_id"}, {Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "count", Value: -1}}}},
		{{Key: "$limit", Value: 1}},
	}

	topMovieCursor, err := r.col.Aggregate(ctx, topMoviePipeline)
	if err != nil {
		return nil, err
	}
	defer topMovieCursor.Close(ctx)

	var topMovies []struct {
		ID    primitive.ObjectID `bson:"_id"`
		Count int64               `bson:"count"`
	}
	if err := topMovieCursor.All(ctx, &topMovies); err != nil {
		return nil, err
	}

	var topSharedMovie *models.TopSharedMovie
	if len(topMovies) > 0 && !topMovies[0].ID.IsZero() {
		// Get movie title from movies collection
		var movie struct {
			Title string `bson:"title"`
		}
		err := r.db.Collection("movies").FindOne(ctx, bson.M{"_id": topMovies[0].ID}).Decode(&movie)
		if err == nil {
			topSharedMovie = &models.TopSharedMovie{
				MovieID:    topMovies[0].ID.Hex(),
				Title:      movie.Title,
				ShareCount: topMovies[0].Count,
			}
		}
	}

	return &models.UserShareStats{
		TotalSharesCreated: sharesCreatedCount,
		TotalShareOpens:    totalClicks,
		TotalMoviesShared: int64(len(uniqueMovies)),
		TopSharedMovie:    topSharedMovie,
	}, nil
}

// GetAdminShareStats returns admin-level share statistics
func (r *ShareRepository) GetAdminShareStats() (*models.AdminShareStats, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// Total shares created
	totalShares, err := r.col.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}

	// Total clicks
	pipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: nil}, {Key: "total_clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}}}}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var result []struct {
		TotalClicks int64 `bson:"total_clicks"`
	}
	if err := cursor.All(ctx, &result); err != nil {
		return nil, err
	}

	var totalClicks int64
	if len(result) > 0 {
		totalClicks = result[0].TotalClicks
	}

	// Top shared movies
	topMoviesPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$movie_id"}, {Key: "shares_created", Value: bson.D{{Key: "$sum", Value: 1}}}, {Key: "total_clicks", Value: bson.D{{Key: "$sum", Value: "$clicks"}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "shares_created", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}

	topMoviesCursor, err := r.col.Aggregate(ctx, topMoviesPipeline)
	if err != nil {
		return nil, err
	}
	defer topMoviesCursor.Close(ctx)

	var topMovies []struct {
		MovieID       primitive.ObjectID `bson:"_id"`
		SharesCreated int64               `bson:"shares_created"`
		TotalClicks   int64               `bson:"total_clicks"`
	}
	if err := topMoviesCursor.All(ctx, &topMovies); err != nil {
		return nil, err
	}

	// Get movie titles
	var topSharedMovies []models.TopSharedMovieStat
	for _, tm := range topMovies {
		var movie struct {
			Title string `bson:"title"`
		}
		err := r.db.Collection("movies").FindOne(ctx, bson.M{"_id": tm.MovieID}).Decode(&movie)
		title := "Unknown"
		if err == nil {
			title = movie.Title
		}
		topSharedMovies = append(topSharedMovies, models.TopSharedMovieStat{
			MovieID:            tm.MovieID.Hex(),
			Title:              title,
			SharesCreatedCount: tm.SharesCreated,
			TotalShareOpens:    tm.TotalClicks,
		})
	}

	// Top users by shares
	topUsersPipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"created_by_user_id": bson.M{"$ne": primitive.ObjectID{}}}}},
		{{Key: "$group", Value: bson.D{{Key: "_id", Value: "$created_by_user_id"}, {Key: "shares_created", Value: bson.D{{Key: "$sum", Value: 1}}}}}},
		{{Key: "$sort", Value: bson.D{{Key: "shares_created", Value: -1}}}},
		{{Key: "$limit", Value: 10}},
	}

	topUsersCursor, err := r.col.Aggregate(ctx, topUsersPipeline)
	if err != nil {
		return nil, err
	}
	defer topUsersCursor.Close(ctx)

	var topUsers []struct {
		UserID        primitive.ObjectID `bson:"_id"`
		SharesCreated int64               `bson:"shares_created"`
	}
	if err := topUsersCursor.All(ctx, &topUsers); err != nil {
		return nil, err
	}

	// Get user display names
	var topUsersByShares []models.TopUserShareStat
	for _, tu := range topUsers {
		var user struct {
			DisplayName string `bson:"display_name"`
		}
		err := r.db.Collection("users").FindOne(ctx, bson.M{"_id": tu.UserID}).Decode(&user)
		displayName := "Unknown"
		if err == nil && user.DisplayName != "" {
			displayName = user.DisplayName
		} else if err == nil {
			// Try telegram username
			var telegramUser struct {
				TelegramUser string `bson:"telegram_user"`
			}
			err := r.db.Collection("users").FindOne(ctx, bson.M{"_id": tu.UserID}).Decode(&telegramUser)
			if err == nil && telegramUser.TelegramUser != "" {
				displayName = "@" + telegramUser.TelegramUser
			}
		}
		topUsersByShares = append(topUsersByShares, models.TopUserShareStat{
			UserID:              tu.UserID.Hex(),
			DisplayName:         displayName,
			SharesCreatedCount: tu.SharesCreated,
		})
	}

	// Recent shares
	recentShares, err := r.col.Find(ctx, bson.M{}, &options.FindOptions{
		Sort:  bson.D{{Key: "created_at", Value: -1}},
		Limit: func() *int64 { v := int64(10); return &v }(),
	})
	if err != nil {
		return nil, err
	}
	defer func() { _ = recentShares.Close(ctx) }()

	var recent []models.Share
	if err := recentShares.All(ctx, &recent); err != nil {
		return nil, err
	}

	return &models.AdminShareStats{
		TotalSharesCreated: totalShares,
		TotalShareOpens:    totalClicks,
		TopSharedMovies:   topSharedMovies,
		TopUsersByShares:  topUsersByShares,
		RecentShares:      recent,
	}, nil
}

// FindRecentShares returns the most recent shares
func (r *ShareRepository) FindRecentShares(limit int) ([]models.Share, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	limitInt64 := int64(limit)
	cursor, err := r.col.Find(ctx, bson.M{}, &options.FindOptions{
		Sort:  bson.D{{Key: "created_at", Value: -1}},
		Limit: &limitInt64,
	})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var shares []models.Share
	if err := cursor.All(ctx, &shares); err != nil {
		return nil, err
	}

	return shares, nil
}
