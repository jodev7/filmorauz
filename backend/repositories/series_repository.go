package repositories

import (
	"context"
	"fmt"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type SeriesRepository struct {
	seriesCol  *mongo.Collection
	seasonCol  *mongo.Collection
	episodeCol *mongo.Collection
}

// Collection returns the underlying mongo collection for admin operations
func (r *SeriesRepository) Collection() *mongo.Collection {
	return r.seriesCol
}

// CountTotalViews returns total views across all series
func (r *SeriesRepository) CountTotalViews() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$views"},
		}},
	}

	cursor, err := r.seriesCol.Aggregate(ctx, pipeline)
	if err != nil {
		return 0, err
	}
	defer cursor.Close(ctx)

	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		return 0, err
	}

	if len(results) == 0 {
		return 0, nil
	}

	total, ok := results[0]["total"].(int64)
	if !ok {
		return 0, nil
	}

	return total, nil
}

func NewSeriesRepository(db *mongo.Database) *SeriesRepository {
	return &SeriesRepository{
		seriesCol:  db.Collection("series"),
		seasonCol:  db.Collection("seasons"),
		episodeCol: db.Collection("episodes"),
	}
}

// Series CRUD

func (r *SeriesRepository) Create(series *models.Series) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	series.CreatedAt = time.Now()
	series.UpdatedAt = time.Now()

	result, err := r.seriesCol.InsertOne(ctx, series)
	if err != nil {
		return err
	}

	series.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *SeriesRepository) GetByID(id primitive.ObjectID) (*models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var series models.Series
	err := r.seriesCol.FindOne(ctx, bson.M{"_id": id}).Decode(&series)
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (r *SeriesRepository) GetBySlug(slug string) (*models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var series models.Series
	err := r.seriesCol.FindOne(ctx, bson.M{"slug": slug}).Decode(&series)
	if err != nil {
		return nil, err
	}
	return &series, nil
}

func (r *SeriesRepository) List(limit, skip int) ([]models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	// Only show published series; legacy docs without is_published are also shown
	publicFilter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
	}

	cursor, err := r.seriesCol.Find(ctx, publicFilter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var seriesList []models.Series
	if err := cursor.All(ctx, &seriesList); err != nil {
		return nil, err
	}

	// Ensure we return an empty slice instead of nil
	if seriesList == nil {
		seriesList = []models.Series{}
	}

	return seriesList, nil
}

func (r *SeriesRepository) Update(series *models.Series) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	series.UpdatedAt = time.Now()

	filter := bson.M{"_id": series.ID}
	update := bson.M{
		"$set": bson.M{
			"title":        series.Title,
			"description":  series.Description,
			"poster_url":   series.PosterURL,
			"backdrop_url": series.BackdropURL,
			"year":         series.Year,
			"genre":        series.Genre,
			"country":      series.Country,
			"is_premium":   series.IsPremium,
			"is_completed": series.IsCompleted,
			"updated_at":   series.UpdatedAt,
		},
	}

	_, err := r.seriesCol.UpdateOne(ctx, filter, update)
	return err
}

func (r *SeriesRepository) Delete(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Delete all episodes for all seasons of this series
	_, err := r.episodeCol.DeleteMany(ctx, bson.M{"series_id": id})
	if err != nil {
		return err
	}

	// Delete all seasons for this series
	_, err = r.seasonCol.DeleteMany(ctx, bson.M{"series_id": id})
	if err != nil {
		return err
	}

	// Delete the series
	_, err = r.seriesCol.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *SeriesRepository) IncrementViews(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{
		"$inc": bson.M{"views": 1},
	}
	_, err := r.seriesCol.UpdateOne(ctx, filter, update)
	return err
}

// Season CRUD

func (r *SeriesRepository) CreateSeason(season *models.Season) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	season.CreatedAt = time.Now()
	season.UpdatedAt = time.Now()

	result, err := r.seasonCol.InsertOne(ctx, season)
	if err != nil {
		return err
	}

	season.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *SeriesRepository) GetSeasonByID(id primitive.ObjectID) (*models.Season, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var season models.Season
	err := r.seasonCol.FindOne(ctx, bson.M{"_id": id}).Decode(&season)
	if err != nil {
		return nil, err
	}
	return &season, nil
}

func (r *SeriesRepository) GetSeasonsBySeriesID(seriesID primitive.ObjectID) ([]models.Season, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "season_number", Value: 1}})

	cursor, err := r.seasonCol.Find(ctx, bson.M{"series_id": seriesID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var seasons []models.Season
	if err := cursor.All(ctx, &seasons); err != nil {
		return nil, err
	}

	// Ensure we return an empty slice instead of nil
	if seasons == nil {
		seasons = []models.Season{}
	}

	return seasons, nil
}

func (r *SeriesRepository) UpdateSeason(season *models.Season) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	season.UpdatedAt = time.Now()

	filter := bson.M{"_id": season.ID}
	update := bson.M{
		"$set": bson.M{
			"title":        season.Title,
			"poster_url":   season.PosterURL,
			"description":  season.Description,
			"release_date": season.ReleaseDate,
			"updated_at":   season.UpdatedAt,
		},
	}

	_, err := r.seasonCol.UpdateOne(ctx, filter, update)
	return err
}

func (r *SeriesRepository) DeleteSeason(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Delete all episodes for this season
	_, err := r.episodeCol.DeleteMany(ctx, bson.M{"season_id": id})
	if err != nil {
		return err
	}

	// Delete the season
	_, err = r.seasonCol.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

// Episode CRUD

func (r *SeriesRepository) CreateEpisode(episode *models.Episode) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	episode.CreatedAt = time.Now()
	episode.UpdatedAt = time.Now()

	result, err := r.episodeCol.InsertOne(ctx, episode)
	if err != nil {
		return err
	}

	episode.ID = result.InsertedID.(primitive.ObjectID)
	return nil
}

func (r *SeriesRepository) GetEpisodeByID(id primitive.ObjectID) (*models.Episode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var episode models.Episode
	err := r.episodeCol.FindOne(ctx, bson.M{"_id": id}).Decode(&episode)
	if err != nil {
		return nil, err
	}
	return &episode, nil
}

func (r *SeriesRepository) GetEpisodesBySeasonID(seasonID primitive.ObjectID) ([]models.Episode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "episode_number", Value: 1}})

	cursor, err := r.episodeCol.Find(ctx, bson.M{"season_id": seasonID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var episodes []models.Episode
	if err := cursor.All(ctx, &episodes); err != nil {
		return nil, err
	}

	// Ensure we return an empty slice instead of nil
	if episodes == nil {
		episodes = []models.Episode{}
	}

	return episodes, nil
}

func (r *SeriesRepository) GetEpisodesBySeriesID(seriesID primitive.ObjectID) ([]models.Episode, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	opts := options.Find().SetSort(bson.D{{Key: "episode_number", Value: 1}})

	cursor, err := r.episodeCol.Find(ctx, bson.M{"series_id": seriesID}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var episodes []models.Episode
	if err := cursor.All(ctx, &episodes); err != nil {
		return nil, err
	}

	return episodes, nil
}

func (r *SeriesRepository) UpdateEpisode(episode *models.Episode) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	episode.UpdatedAt = time.Now()

	filter := bson.M{"_id": episode.ID}
	update := bson.M{
		"$set": bson.M{
			"season_id":      episode.SeasonID,
			"episode_number": episode.EpisodeNumber,
			"title":          episode.Title,
			"description":    episode.Description,
			"thumbnail_url":  episode.ThumbnailURL,
			"video_url":      episode.VideoURL,
			"embed_url":      episode.EmbedURL,
			"duration":       episode.Duration,
			"air_date":       episode.AirDate,
			"updated_at":     episode.UpdatedAt,
		},
	}

	_, err := r.episodeCol.UpdateOne(ctx, filter, update)
	return err
}

// UpdateEpisodeSeasonAndNumber updates only season_id and episode_number for an episode
func (r *SeriesRepository) UpdateEpisodeSeasonAndNumber(episodeID primitive.ObjectID, seasonID primitive.ObjectID, episodeNumber int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": episodeID}
	update := bson.M{
		"$set": bson.M{
			"season_id":      seasonID,
			"episode_number": episodeNumber,
			"updated_at":     time.Now(),
		},
	}

	_, err := r.episodeCol.UpdateOne(ctx, filter, update)
	return err
}

// ReorderEpisodesInSeason updates episode numbers for a batch of episodes in a season
func (r *SeriesRepository) ReorderEpisodesInSeason(seasonID primitive.ObjectID, episodeIDs []primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	session, err := r.episodeCol.Database().Client().StartSession()
	if err != nil {
		return err
	}
	defer session.EndSession(ctx)

	_, err = session.WithTransaction(ctx, func(sessCtx mongo.SessionContext) (interface{}, error) {
		for i, episodeID := range episodeIDs {
			filter := bson.M{"_id": episodeID, "season_id": seasonID}
			update := bson.M{
				"$set": bson.M{
					"episode_number": i + 1,
					"updated_at":     time.Now(),
				},
			}
			_, err := r.episodeCol.UpdateOne(sessCtx, filter, update)
			if err != nil {
				return nil, err
			}
		}
		return nil, nil
	})

	return err
}

// MoveEpisodeToSeason moves an episode to a different season and updates its episode number
func (r *SeriesRepository) MoveEpisodeToSeason(episodeID, newSeasonID primitive.ObjectID, newEpisodeNumber int) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": episodeID}
	update := bson.M{
		"$set": bson.M{
			"season_id":      newSeasonID,
			"episode_number": newEpisodeNumber,
			"updated_at":     time.Now(),
		},
	}

	_, err := r.episodeCol.UpdateOne(ctx, filter, update)
	return err
}

func (r *SeriesRepository) DeleteEpisode(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.episodeCol.DeleteOne(ctx, bson.M{"_id": id})
	return err
}

func (r *SeriesRepository) IncrementEpisodeViews(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{
		"$inc": bson.M{"views": 1},
	}
	_, err := r.episodeCol.UpdateOne(ctx, filter, update)
	return err
}

// Get series with all seasons and episodes
func (r *SeriesRepository) GetSeriesWithSeasons(seriesID primitive.ObjectID) (*models.SeriesWithSeasons, error) {
	series, err := r.GetByID(seriesID)
	if err != nil {
		return nil, err
	}

	seasons, err := r.GetSeasonsBySeriesID(seriesID)
	if err != nil {
		return nil, err
	}

	// Ensure seasons is never nil
	if seasons == nil {
		seasons = []models.Season{}
	}

	seasonsWithEpisodes := make([]models.SeasonWithEpisodes, len(seasons))
	for i, season := range seasons {
		episodes, err := r.GetEpisodesBySeasonID(season.ID)
		if err != nil {
			episodes = []models.Episode{}
		}
		// Ensure episodes is never nil
		if episodes == nil {
			episodes = []models.Episode{}
		}
		seasonsWithEpisodes[i] = models.SeasonWithEpisodes{
			Season:   season,
			Episodes: episodes,
		}
	}

	return &models.SeriesWithSeasons{
		Series:  *series,
		Seasons: seasonsWithEpisodes,
	}, nil
}

// ListAdmin returns ALL series (regardless of approval status) for the admin dashboard.
func (r *SeriesRepository) ListAdmin(limit, skip int) ([]models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 200
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit)).
		SetSkip(int64(skip))

	cursor, err := r.seriesCol.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var seriesList []models.Series
	if err := cursor.All(ctx, &seriesList); err != nil {
		return nil, err
	}
	if seriesList == nil {
		seriesList = []models.Series{}
	}
	return seriesList, nil
}

// SetApprovalStatus sets the approval status and publishes/unpublishes a series.
func (r *SeriesRepository) SetApprovalStatus(idHex, status, byUserID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return err
	}

	now := time.Now()
	result, err := r.seriesCol.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"approval_status": status,
			"is_published":    status == "approved",
			"approved_at":     now,
			"approved_by":     byUserID,
			"updated_at":      now,
		}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("series not found")
	}
	return nil
}
