package repositories

import (
	"context"
	"fmt"
	"log"
	"strings"
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

type SitemapSeriesRecord struct {
	ID          primitive.ObjectID `bson:"_id"`
	Slug        string             `bson:"slug"`
	Genre       []string           `bson:"genre"`
	UpdatedAt   time.Time          `bson:"updated_at"`
	Title       string             `bson:"title,omitempty"`
	Description string             `bson:"description,omitempty"`
	PosterURL   string             `bson:"poster_url,omitempty"`
	Year        int                `bson:"year,omitempty"`
	CreatedAt   time.Time          `bson:"created_at,omitempty"`
}

type SitemapSeasonRecord struct {
	ID           primitive.ObjectID `bson:"_id"`
	SeriesID     primitive.ObjectID `bson:"series_id"`
	SeasonNumber int                `bson:"season_number"`
	UpdatedAt    time.Time          `bson:"updated_at"`
}

type SitemapEpisodeRecord struct {
	ID            primitive.ObjectID `bson:"_id"`
	SeriesID      primitive.ObjectID `bson:"series_id"`
	SeasonID      primitive.ObjectID `bson:"season_id"`
	EpisodeNumber int                `bson:"episode_number"`
	UpdatedAt     time.Time          `bson:"updated_at"`
	Title         string             `bson:"title,omitempty"`
	ThumbnailURL  string             `bson:"thumbnail_url,omitempty"`
	Duration      int                `bson:"duration,omitempty"`
	CreatedAt     time.Time          `bson:"created_at,omitempty"`
}

func normalizeSeriesGenres(genres []string) []string {
	if len(genres) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(genres))
	out := make([]string, 0, len(genres))
	for _, g := range genres {
		g = strings.ToLower(strings.TrimSpace(g))
		if g == "" {
			continue
		}
		g = strings.ReplaceAll(g, "_", "-")
		g = strings.Join(strings.FieldsFunc(g, func(r rune) bool {
			return r == ' ' || r == '-'
		}), "-")
		switch g {
		case "science-fiction", "sciencefiction", "scifi":
			g = "sci-fi"
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return out
}

// Collection returns the underlying mongo collection for admin operations
// GetSeriesByIDs retrieves multiple series by their ObjectIDs, preserving the
// order of the input IDs. Used by collection population.
func (r *SeriesRepository) GetSeriesByIDs(ctx context.Context, ids []primitive.ObjectID) ([]models.Series, error) {
	if len(ids) == 0 {
		return []models.Series{}, nil
	}

	cursor, err := r.seriesCol.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("find series by ids: %w", err)
	}
	defer cursor.Close(ctx)

	var series []models.Series
	if err := cursor.All(ctx, &series); err != nil {
		return nil, fmt.Errorf("decode series: %w", err)
	}

	seriesMap := make(map[primitive.ObjectID]models.Series, len(series))
	for _, s := range series {
		seriesMap[s.ID] = s
	}

	ordered := make([]models.Series, 0, len(ids))
	for _, id := range ids {
		if s, ok := seriesMap[id]; ok {
			ordered = append(ordered, s)
		}
	}
	return ordered, nil
}

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

func (r *SeriesRepository) ListPublishedForSitemap() ([]SitemapSeriesRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().
		SetProjection(bson.M{
			"_id":         1,
			"slug":        1,
			"genre":       1,
			"updated_at":  1,
			"title":       1,
			"description": 1,
			"poster_url":  1,
			"year":        1,
			"created_at":  1,
		}).
		SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "_id", Value: 1}})

	cursor, err := r.seriesCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []SitemapSeriesRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []SitemapSeriesRecord{}
	}
	for i := range records {
		records[i].Genre = normalizeSeriesGenres(records[i].Genre)
	}
	return records, nil
}

func (r *SeriesRepository) GetSeasonsBySeriesIDs(seriesIDs []primitive.ObjectID) ([]SitemapSeasonRecord, error) {
	if len(seriesIDs) == 0 {
		return []SitemapSeasonRecord{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opts := options.Find().
		SetProjection(bson.M{
			"_id":           1,
			"series_id":     1,
			"season_number": 1,
			"updated_at":    1,
		}).
		SetSort(bson.D{
			{Key: "series_id", Value: 1},
			{Key: "season_number", Value: 1},
			{Key: "_id", Value: 1},
		})

	cursor, err := r.seasonCol.Find(ctx, bson.M{"series_id": bson.M{"$in": seriesIDs}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []SitemapSeasonRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []SitemapSeasonRecord{}
	}
	return records, nil
}

func (r *SeriesRepository) GetEpisodesBySeriesIDs(seriesIDs []primitive.ObjectID) ([]SitemapEpisodeRecord, error) {
	if len(seriesIDs) == 0 {
		return []SitemapEpisodeRecord{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	opts := options.Find().
		SetProjection(bson.M{
			"_id":            1,
			"series_id":      1,
			"season_id":      1,
			"episode_number": 1,
			"updated_at":     1,
			"title":          1,
			"thumbnail_url":  1,
			"duration":       1,
			"created_at":     1,
		}).
		SetSort(bson.D{
			{Key: "series_id", Value: 1},
			{Key: "season_id", Value: 1},
			{Key: "episode_number", Value: 1},
			{Key: "_id", Value: 1},
		})

	cursor, err := r.episodeCol.Find(ctx, bson.M{"series_id": bson.M{"$in": seriesIDs}}, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []SitemapEpisodeRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []SitemapEpisodeRecord{}
	}
	return records, nil
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
	series.Genre = normalizeSeriesGenres(series.Genre)
	if err := r.applyEpisodeRatingSummary(ctx, &series); err != nil {
		return nil, err
	}
	return &series, nil
}

func (r *SeriesRepository) applyEpisodeRatingSummary(ctx context.Context, series *models.Series) error {
	if series == nil {
		return nil
	}
	list := []models.Series{*series}
	if err := r.applyEpisodeRatingSummaries(ctx, list); err != nil {
		return err
	}
	*series = list[0]
	return nil
}

func (r *SeriesRepository) applyEpisodeRatingSummaries(ctx context.Context, seriesList []models.Series) error {
	if len(seriesList) == 0 {
		return nil
	}

	seriesIDs := make([]primitive.ObjectID, 0, len(seriesList))
	indexByID := make(map[primitive.ObjectID]int, len(seriesList))
	for i := range seriesList {
		seriesIDs = append(seriesIDs, seriesList[i].ID)
		indexByID[seriesList[i].ID] = i
		seriesList[i].RatingAvg = 0
		seriesList[i].RatingCount = 0
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: bson.M{"series_id": bson.M{"$in": seriesIDs}}}},
		{{Key: "$lookup", Value: bson.M{
			"from":         "episode_ratings",
			"localField":   "_id",
			"foreignField": "episode_id",
			"as":           "ratings",
		}}},
		{{Key: "$unwind", Value: bson.M{"path": "$ratings", "preserveNullAndEmptyArrays": false}}},
		{{Key: "$group", Value: bson.M{
			"_id":   "$series_id",
			"avg":   bson.M{"$avg": "$ratings.rating"},
			"count": bson.M{"$sum": 1},
		}}},
	}

	cursor, err := r.episodeCol.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	defer cursor.Close(ctx)

	for cursor.Next(ctx) {
		var row struct {
			ID    primitive.ObjectID `bson:"_id"`
			Avg   float64            `bson:"avg"`
			Count int64              `bson:"count"`
		}
		if err := cursor.Decode(&row); err != nil {
			return err
		}
		if idx, ok := indexByID[row.ID]; ok {
			seriesList[idx].RatingAvg = row.Avg
			seriesList[idx].RatingCount = row.Count
		}
	}
	return cursor.Err()
}

func (r *SeriesRepository) GetBySlug(slug string) (*models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var series models.Series
	err := r.seriesCol.FindOne(ctx, bson.M{"slug": slug}).Decode(&series)
	if err != nil {
		return nil, err
	}
	series.Genre = normalizeSeriesGenres(series.Genre)
	if err := r.applyEpisodeRatingSummary(ctx, &series); err != nil {
		return nil, err
	}
	return &series, nil
}

func (r *SeriesRepository) FindByCode(code string) (*models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var series models.Series
	if err := r.seriesCol.FindOne(ctx, bson.M{"code": code}).Decode(&series); err != nil {
		return nil, err
	}
	series.Genre = normalizeSeriesGenres(series.Genre)
	if err := r.applyEpisodeRatingSummary(ctx, &series); err != nil {
		return nil, err
	}
	return &series, nil
}

func (r *SeriesRepository) List(limit, skip int, genre string) ([]models.Series, error) {
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
	if normalized := normalizeSeriesGenres([]string{genre}); len(normalized) > 0 {
		publicFilter["genre"] = bson.M{"$in": []string{normalized[0]}}
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
	for i := range seriesList {
		seriesList[i].Genre = normalizeSeriesGenres(seriesList[i].Genre)
	}
	if err := r.applyEpisodeRatingSummaries(ctx, seriesList); err != nil {
		return nil, err
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
			"slug":         series.Slug,
			"code":         series.Code,
			"title":        series.Title,
			"description":  series.Description,
			"poster_url":   series.PosterURL,
			"backdrop_url": series.BackdropURL,
			"year":         series.Year,
			"genre":        series.Genre,
			"country":      series.Country,
			"is_premium":   series.IsPremium,
			"is_completed": series.IsCompleted,
			"quality":      series.Quality,
			"available_qualities": series.AvailableQualities,
			"generated_qualities": series.GeneratedQualities,
			"title_uz":       series.TitleUz,
			"description_uz": series.DescriptionUz,
			"genres_uz":      series.GenresUz,
			"countries_uz":   series.CountriesUz,
			"updated_at":   series.UpdatedAt,
		},
	}

	_, err := r.seriesCol.UpdateOne(ctx, filter, update)
	return err
}

// FindHighestCode finds the highest numeric code in the series collection.
func (r *SeriesRepository) FindHighestCode() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := r.seriesCol.Find(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to query series: %w", err)
	}
	defer cursor.Close(ctx)

	var highestSeq int64
	for cursor.Next(ctx) {
		var doc struct {
			Code string `bson:"code"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		if doc.Code == "" {
			continue
		}

		var seq int64
		if _, err := fmt.Sscanf(doc.Code, "%d", &seq); err == nil && seq > highestSeq {
			highestSeq = seq
		}
	}

	return highestSeq, nil
}

func (r *SeriesRepository) FindSeriesWithoutCode() ([]models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	filter := bson.M{
		"$or": []bson.M{
			{"code": bson.M{"$exists": false}},
			{"code": ""},
			{"code": nil},
		},
	}
	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := r.seriesCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find series without code: %w", err)
	}
	defer cursor.Close(ctx)

	var seriesList []models.Series
	if err := cursor.All(ctx, &seriesList); err != nil {
		return nil, fmt.Errorf("decode series without code: %w", err)
	}
	for i := range seriesList {
		seriesList[i].Genre = normalizeSeriesGenres(seriesList[i].Genre)
	}
	if seriesList == nil {
		return []models.Series{}, nil
	}
	return seriesList, nil
}

func (r *SeriesRepository) SetSeriesCode(id primitive.ObjectID, code string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := r.seriesCol.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"code":       code,
			"updated_at": time.Now(),
		}},
	)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		log.Printf("series code backfill skipped missing series: %s", id.Hex())
	}
	return nil
}

func (r *SeriesRepository) CodeExists(code string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := r.seriesCol.CountDocuments(ctx, bson.M{"code": code})
	return count > 0, err
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
			"season_id":           episode.SeasonID,
			"episode_number":      episode.EpisodeNumber,
			"title":               episode.Title,
			"description":         episode.Description,
			"thumbnail_url":       episode.ThumbnailURL,
			"video_url":           episode.VideoURL,
			"embed_url":           episode.EmbedURL,
			"source_type":         episode.SourceType,
			"duration":            episode.Duration,
			"quality":             episode.Quality,
			"air_date":            episode.AirDate,
			"master_playlist_url": episode.MasterPlaylistURL,
			"available_qualities": episode.AvailableQualities,
			"generated_qualities": episode.GeneratedQualities,
			"updated_at":          episode.UpdatedAt,
			"thumbnails_base_url": episode.ThumbnailsBaseURL,
			"thumbnail_interval":  episode.ThumbnailInterval,
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
	for i := range seriesList {
		seriesList[i].Genre = normalizeSeriesGenres(seriesList[i].Genre)
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

// Search does a basic text search on title and description for series
func (r *SeriesRepository) Search(query string) ([]models.Series, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Case-insensitive regex search on title; only published content
	filter := bson.M{
		"$and": []bson.M{
			{
				"$or": []bson.M{
					{"is_published": true},
					{"is_published": bson.M{"$exists": false}},
				},
			},
			{
				"$or": []bson.M{
					{"title": bson.M{"$regex": query, "$options": "i"}},
					{"description": bson.M{"$regex": query, "$options": "i"}},
				},
			},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(20)

	cursor, err := r.seriesCol.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search series: %w", err)
	}
	defer cursor.Close(ctx)

	var seriesList []models.Series
	if err := cursor.All(ctx, &seriesList); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}

	if seriesList == nil {
		seriesList = []models.Series{}
	}
	for i := range seriesList {
		seriesList[i].Genre = normalizeSeriesGenres(seriesList[i].Genre)
	}
	if err := r.applyEpisodeRatingSummaries(ctx, seriesList); err != nil {
		return nil, err
	}

	return seriesList, nil
}
