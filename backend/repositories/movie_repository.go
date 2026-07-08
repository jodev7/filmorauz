package repositories

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MovieRepository struct {
	col *mongo.Collection
}

type SitemapMovieRecord struct {
	ID          primitive.ObjectID `bson:"_id"`
	Slug        string             `bson:"slug"`
	Genre       []string           `bson:"genre"`
	UpdatedAt   time.Time          `bson:"updated_at"`
	Title       string             `bson:"title,omitempty"`
	Description string             `bson:"description,omitempty"`
	PosterURL   string             `bson:"poster_url,omitempty"`
	Duration    int                `bson:"duration,omitempty"`
	Year        int                `bson:"year,omitempty"`
	CreatedAt   time.Time          `bson:"created_at,omitempty"`
	// Stream URLs for the video sitemap's <video:content_loc>.
	MasterPlaylistURL string `bson:"master_playlist_url,omitempty"`
	VideoURL          string `bson:"video_url,omitempty"`
}

func normalizeGenreValues(genres []string) []string {
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
func (r *MovieRepository) Collection() *mongo.Collection {
	return r.col
}

// CountTotalViews returns total views across all movies
func (r *MovieRepository) CountTotalViews() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	pipeline := []bson.M{
		{"$group": bson.M{
			"_id":   nil,
			"total": bson.M{"$sum": "$views"},
		}},
	}

	cursor, err := r.col.Aggregate(ctx, pipeline)
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

func NewMovieRepository(db *mongo.Database) *MovieRepository {
	repo := &MovieRepository{col: db.Collection("movies")}
	repo.EnsureIndexes()
	return repo
}

func (r *MovieRepository) ListPublishedForSitemap() ([]SitemapMovieRecord, error) {
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
			"duration":    1,
			"year":        1,
			"created_at":  1,
			"master_playlist_url": 1,
			"video_url":           1,
		}).
		SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "_id", Value: 1}})

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var records []SitemapMovieRecord
	if err := cursor.All(ctx, &records); err != nil {
		return nil, err
	}
	if records == nil {
		records = []SitemapMovieRecord{}
	}
	for i := range records {
		records[i].Genre = normalizeGenreValues(records[i].Genre)
	}
	return records, nil
}

// EnsureIndexes creates required indexes on the movies collection
func (r *MovieRepository) EnsureIndexes() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Unique index on code (alphanumeric)
	codeIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "code", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	// Index on slug (already used for lookups)
	slugIndex := mongo.IndexModel{
		Keys:    bson.D{{Key: "slug", Value: 1}},
		Options: options.Index().SetUnique(true),
	}
	normalizedTitleYearIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "normalized_title", Value: 1},
			{Key: "year", Value: 1},
		},
	}
	sourceURLIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "source.provider", Value: 1},
			{Key: "source.source_url", Value: 1},
		},
	}
	sourceIDIndex := mongo.IndexModel{
		Keys: bson.D{
			{Key: "source.provider", Value: 1},
			{Key: "source.source_id", Value: 1},
		},
	}
	tmdbIndex := mongo.IndexModel{
		Keys: bson.D{{Key: "tmdb_id", Value: 1}},
	}

	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{
		codeIndex,
		slugIndex,
		normalizedTitleYearIndex,
		sourceURLIndex,
		sourceIDIndex,
		tmdbIndex,
	})
	if err != nil {
		return err
	}
	return nil
}

// normalizeFieldToString safely converts a mixed-type field to string
func normalizeFieldToString(value interface{}) string {
	if value == nil {
		return ""
	}
	switch v := value.(type) {
	case string:
		return v
	case int64:
		return strconv.FormatInt(v, 10)
	case int32:
		return strconv.FormatInt(int64(v), 10)
	case float64:
		// Handle float64 that might be from JSON number conversion
		if float64(int64(v)) == v {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	default:
		return fmt.Sprintf("%v", value)
	}
}

// normalizeFieldToInt safely converts a mixed-type field to int
func normalizeFieldToInt(value interface{}) int {
	if value == nil {
		return 0
	}
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case int32:
		return int(v)
	case float64:
		return int(v)
	case string:
		i, _ := strconv.Atoi(v)
		return i
	default:
		return 0
	}
}

// decodeBSONStringArray extracts a []string from a bson.M field, handling the
// primitive.A type returned by the mongo driver alongside []interface{} and
// []string. Returns an empty (non-nil) slice when the field is of any other type.
func decodeBSONStringArray(value interface{}) []string {
	var items []interface{}
	switch v := value.(type) {
	case primitive.A:
		items = []interface{}(v)
	case []interface{}:
		items = v
	case []string:
		out := make([]string, 0, len(v))
		for _, s := range v {
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	default:
		return []string{}
	}

	out := make([]string, 0, len(items))
	for _, item := range items {
		if s, ok := item.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}

func normalizeGenreFieldValue(value interface{}) []string {
	// BSON arrays decoded into bson.M come back as primitive.A (a defined type
	// with underlying []interface{}). A Go type switch on []interface{} does
	// NOT match primitive.A, so we handle it explicitly — otherwise every
	// genre read falls to the default branch and silently returns []string{}.
	var items []interface{}
	switch g := value.(type) {
	case primitive.A:
		items = []interface{}(g)
	case []interface{}:
		items = g
	case []string:
		return normalizeGenreValues(g)
	case string:
		if strings.Contains(g, ",") {
			return normalizeGenreValues(strings.Split(g, ","))
		}
		return normalizeGenreValues([]string{g})
	default:
		return []string{}
	}

	genres := make([]string, 0, len(items))
	for _, item := range items {
		switch v := item.(type) {
		case string:
			genres = append(genres, v)
		case fmt.Stringer:
			genres = append(genres, v.String())
		}
	}
	return normalizeGenreValues(genres)
}

// normalizeMovieFromBSON converts a bson.M document to models.Movie, handling legacy types
func normalizeMovieFromBSON(doc bson.M) (*models.Movie, error) {
	movie := &models.Movie{}

	// Handle _id
	if id, ok := doc["_id"]; ok && id != nil {
		if oid, ok := id.(primitive.ObjectID); ok {
			movie.ID = oid
		} else if str, ok := id.(string); ok {
			if oid, err := primitive.ObjectIDFromHex(str); err == nil {
				movie.ID = oid
			}
		}
	}

	// Handle code - normalize to string (handles legacy numeric)
	movie.Code = normalizeFieldToString(doc["code"])

	// Handle slug
	if slug, ok := doc["slug"].(string); ok {
		movie.Slug = slug
	}
	if normalizedTitle, ok := doc["normalized_title"].(string); ok {
		movie.NormalizedTitle = normalizedTitle
	}
	if source, ok := doc["source"].(bson.M); ok {
		movie.Source = &models.MovieSource{
			Provider:  normalizeFieldToString(source["provider"]),
			SourceURL: normalizeFieldToString(source["source_url"]),
			SourceID:  normalizeFieldToString(source["source_id"]),
		}
	}

	// Handle website_url
	if url, ok := doc["website_url"].(string); ok {
		movie.WebsiteURL = url
	}

	// Handle title
	if title, ok := doc["title"].(string); ok {
		movie.Title = title
	}

	// Handle description
	if desc, ok := doc["description"].(string); ok {
		movie.Description = desc
	}

	// Handle poster_url
	if poster, ok := doc["poster_url"].(string); ok {
		movie.PosterURL = poster
	}

	// Handle backdrop_url
	if backdrop, ok := doc["backdrop_url"].(string); ok {
		movie.BackdropURL = backdrop
	}

	// Handle year - normalize to int (handles legacy numeric)
	movie.Year = normalizeFieldToInt(doc["year"])

	// Handle genre - may be array or single value OR stored as "genres"
	genreFound := false
	if genre, ok := doc["genre"]; ok && genre != nil {
		genreFound = true
		log.Printf("[DEBUG] normalizeMovieFromBSON: found 'genre' field, type=%T value=%v", genre, genre)
		movie.Genre = normalizeGenreFieldValue(genre)
		log.Printf("[DEBUG] normalizeMovieFromBSON: normalized 'genre' -> %v", movie.Genre)
	} else if genre, ok := doc["genres"]; ok && genre != nil {
		// Fallback: check for "genres" field (some legacy data might use this)
		genreFound = true
		log.Printf("[DEBUG] normalizeMovieFromBSON: found 'genres' field (fallback), type=%T value=%v", genre, genre)
		movie.Genre = normalizeGenreFieldValue(genre)
		log.Printf("[DEBUG] normalizeMovieFromBSON: normalized 'genres' -> %v", movie.Genre)
	} else if genre, ok := doc["movie_genre"]; ok && genre != nil {
		// Fallback: some legacy/imported docs may use "movie_genre"
		genreFound = true
		log.Printf("[DEBUG] normalizeMovieFromBSON: found 'movie_genre' field (fallback), type=%T value=%v", genre, genre)
		movie.Genre = normalizeGenreFieldValue(genre)
		log.Printf("[DEBUG] normalizeMovieFromBSON: normalized 'movie_genre' -> %v", movie.Genre)
	}

	if !genreFound {
		// Log available keys for debugging
		var keys []string
		for k := range doc {
			keys = append(keys, k)
		}
		log.Printf("[DEBUG] normalizeMovieFromBSON: neither 'genre' nor 'genres' found in doc, available keys: %v", keys)
	}
	// Always ensure Genre is non-nil empty slice, never null
	movie.Genre = normalizeGenreValues(movie.Genre)
	if movie.Genre == nil {
		movie.Genre = []string{}
		log.Printf("[DEBUG] normalizeMovieFromBSON: Genre set to empty slice (was nil)")
	}

	// Handle country
	if country, ok := doc["country"].(string); ok {
		movie.Country = country
	}

	// Handle video_url
	if videoURL, ok := doc["video_url"].(string); ok {
		movie.VideoURL = videoURL
	}

	// Handle embed_url
	if embedURL, ok := doc["embed_url"].(string); ok {
		movie.EmbedURL = embedURL
	}

	// Handle source_type
	if sourceType, ok := doc["source_type"].(string); ok {
		movie.SourceType = models.VideoSourceType(sourceType)
	}

	// Handle duration
	movie.Duration = normalizeFieldToInt(doc["duration"])

	// Handle quality
	if quality, ok := doc["quality"].(string); ok {
		movie.Quality = quality
	}

	// Handle views - bson.M decodes int32/int64/float64 depending on stored type
	if views, ok := doc["views"]; ok && views != nil {
		switch v := views.(type) {
		case int64:
			movie.Views = v
		case int32:
			movie.Views = int64(v)
		case int:
			movie.Views = int64(v)
		case float64:
			movie.Views = int64(v)
		}
	}

	// Handle rating_avg
	if ratingAvg, ok := doc["rating_avg"]; ok && ratingAvg != nil {
		switch v := ratingAvg.(type) {
		case float64:
			movie.RatingAvg = v
		case int32:
			movie.RatingAvg = float64(v)
		case int64:
			movie.RatingAvg = float64(v)
		case int:
			movie.RatingAvg = float64(v)
		}
	}

	// Handle rating_count
	if ratingCount, ok := doc["rating_count"]; ok && ratingCount != nil {
		switch v := ratingCount.(type) {
		case int64:
			movie.RatingCount = v
		case int32:
			movie.RatingCount = int64(v)
		case int:
			movie.RatingCount = int64(v)
		case float64:
			movie.RatingCount = int64(v)
		}
	}

	// Handle created_at — bson.M decodes BSON Date as primitive.DateTime, not time.Time
	if createdAt, ok := doc["created_at"]; ok && createdAt != nil {
		switch v := createdAt.(type) {
		case primitive.DateTime:
			movie.CreatedAt = v.Time()
		case time.Time:
			movie.CreatedAt = v
		}
	}

	// Handle updated_at — same as created_at
	if updatedAt, ok := doc["updated_at"]; ok && updatedAt != nil {
		switch v := updatedAt.(type) {
		case primitive.DateTime:
			movie.UpdatedAt = v.Time()
		case time.Time:
			movie.UpdatedAt = v
		}
	}

	// Handle is_premium
	if isPremium, ok := doc["is_premium"].(bool); ok {
		movie.IsPremium = isPremium
	}

	// Handle HLS streaming fields
	if masterPlaylistURL, ok := doc["master_playlist_url"].(string); ok {
		movie.MasterPlaylistURL = masterPlaylistURL
	}
	if aq, ok := doc["available_qualities"]; ok && aq != nil {
		movie.AvailableQualities = decodeBSONStringArray(aq)
	}
	if gq, ok := doc["generated_qualities"]; ok && gq != nil {
		movie.GeneratedQualities = decodeBSONStringArray(gq)
	}
	if len(movie.AvailableQualities) == 0 && len(movie.GeneratedQualities) > 0 {
		movie.AvailableQualities = append([]string(nil), movie.GeneratedQualities...)
	}
	if len(movie.GeneratedQualities) == 0 && len(movie.AvailableQualities) > 0 {
		movie.GeneratedQualities = append([]string(nil), movie.AvailableQualities...)
	}
	if defaultQuality, ok := doc["default_quality"].(string); ok {
		movie.DefaultQuality = defaultQuality
	}
	if sourceResolution, ok := doc["source_resolution"].(string); ok {
		movie.SourceResolution = sourceResolution
	}

	// Handle title_uz
	if titleUz, ok := doc["title_uz"].(string); ok {
		movie.TitleUz = titleUz
	}

	// Handle description_uz
	if descUz, ok := doc["description_uz"].(string); ok {
		movie.DescriptionUz = descUz
	}

	// Handle genres_uz — stored as []string or []interface{}/primitive.A
	if gUz, ok := doc["genres_uz"]; ok && gUz != nil {
		movie.GenresUz = decodeBSONStringArray(gUz)
	}

	// Handle countries_uz — may be stored as string (legacy) or []string/[]interface{}/primitive.A
	if cUz, ok := doc["countries_uz"]; ok && cUz != nil {
		if s, isString := cUz.(string); isString {
			if s != "" {
				movie.CountriesUz = strings.Split(s, ", ")
			}
		} else {
			movie.CountriesUz = decodeBSONStringArray(cUz)
		}
	}

	// Handle original_title
	if origTitle, ok := doc["original_title"].(string); ok {
		movie.OriginalTitle = origTitle
	}

	// Handle tmdb_id
	movie.TMDBID = normalizeFieldToInt(doc["tmdb_id"])

	// Handle metadata_source
	if ms, ok := doc["metadata_source"].(string); ok {
		movie.MetadataSource = ms
	}

	// Handle approval workflow fields.
	// Legacy documents (created before approval feature) have no is_published field —
	// treat them as already published/approved so existing content stays visible.
	if pub, ok := doc["is_published"]; ok {
		if b, ok := pub.(bool); ok {
			movie.IsPublished = b
		}
	} else {
		movie.IsPublished = true // legacy document — treat as approved
	}
	if status, ok := doc["approval_status"].(string); ok {
		movie.ApprovalStatus = status
	} else {
		movie.ApprovalStatus = "approved" // legacy document
	}
	if raw, ok := doc["approved_at"]; ok && raw != nil {
		switch v := raw.(type) {
		case primitive.DateTime:
			t := v.Time()
			movie.ApprovedAt = &t
		case time.Time:
			movie.ApprovedAt = &v
		}
	}
	if by, ok := doc["approved_by"].(string); ok {
		movie.ApprovedBy = by
	}

	return movie, nil
}

// List returns all movies, optionally filtered by genre
func (r *MovieRepository) List(genre string, page, limit int) ([]models.Movie, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("[ListMovies] === DEBUG START ===")
	log.Printf("[ListMovies] Request: genre=%q, page=%d, limit=%d", genre, page, limit)
	if normalized := normalizeGenreValues([]string{genre}); len(normalized) > 0 {
		genre = normalized[0]
	} else {
		genre = ""
	}

	// Only show published movies publicly; legacy docs without is_published are also shown
	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
	}
	if genre != "" {
		filter["genre"] = bson.M{"$in": []string{genre}}
	}
	log.Printf("[ListMovies] Query filter: %v", filter)

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		log.Printf("[ListMovies] ERROR counting: %v", err)
		log.Printf("[ListMovies] === DEBUG END ===")
		return nil, 0, fmt.Errorf("count documents: %w", err)
	}
	log.Printf("[ListMovies] Total matching movies: %d", total)

	// Sort by updated_at desc (most recently edited/added first); fall back to
	// created_at for documents that have never been edited.
	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		log.Printf("[ListMovies] ERROR finding: %v", err)
		log.Printf("[ListMovies] === DEBUG END ===")
		return nil, 0, fmt.Errorf("find movies: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode into bson.M first, then normalize
	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		log.Printf("[ListMovies] ERROR decoding: %v", err)
		log.Printf("[ListMovies] === DEBUG END ===")
		return nil, 0, fmt.Errorf("decode raw movies: %w", err)
	}
	log.Printf("[ListMovies] Raw docs fetched: %d", len(rawDocs))

	// Normalize each document
	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			log.Printf("[ListMovies] WARN: failed to normalize: %v", err)
			continue
		}
		log.Printf("[ListMovies] Movie: id=%v, slug=%q, source_type=%q", movie.ID, movie.Slug, movie.SourceType)
		movies = append(movies, *movie)
	}

	log.Printf("[ListMovies] RETURNING: %d movies", len(movies))
	log.Printf("[ListMovies] === DEBUG END ===")
	return movies, total, nil
}

// ListTopRated returns published movies sorted by rating, highest first.
// Only movies with at least one rating are considered so the row reflects
// genuine audience scores rather than unrated catalogue padding.
func (r *MovieRepository) ListTopRated(limit int) ([]models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit < 1 || limit > 50 {
		limit = 12
	}

	filter := bson.M{
		"$and": []bson.M{
			{"$or": []bson.M{
				{"is_published": true},
				{"is_published": bson.M{"$exists": false}},
			}},
			{"rating_count": bson.M{"$gt": 0}},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "rating_avg", Value: -1}, {Key: "rating_count", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find top rated movies: %w", err)
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, fmt.Errorf("decode top rated movies: %w", err)
	}

	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}
	return movies, nil
}

// ListMostViewed returns published movies sorted by view count, highest first.
// Used to build the homepage "Weekly Top 10" widget.
func (r *MovieRepository) ListMostViewed(limit int) ([]models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit < 1 || limit > 50 {
		limit = 12
	}

	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "views", Value: -1}, {Key: "_id", Value: 1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find most viewed movies: %w", err)
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, fmt.Errorf("decode most viewed movies: %w", err)
	}

	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}
	return movies, nil
}

// ListByGenre returns published movies for a single genre, newest first.
func (r *MovieRepository) ListByGenre(genre string, limit int) ([]models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if normalized := normalizeGenreValues([]string{genre}); len(normalized) > 0 {
		genre = normalized[0]
	} else {
		return []models.Movie{}, nil
	}
	if limit < 1 || limit > 50 {
		limit = 12
	}

	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
		"genre": bson.M{"$in": []string{genre}},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "updated_at", Value: -1}, {Key: "created_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find movies by genre: %w", err)
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, fmt.Errorf("decode movies by genre: %w", err)
	}

	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}
	return movies, nil
}

// FindBySlug returns a single movie by slug
func (r *MovieRepository) FindBySlug(slug string) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[FindBySlug] === DEBUG START ===")
	log.Printf("[FindBySlug] Requested slug: %q", slug)
	log.Printf("[FindBySlug] Query filter: {\"slug\": %q}", slug)

	// Decode into bson.M first to handle type mismatches (e.g. countries_uz stored as string vs []string)
	var raw bson.M
	err := r.col.FindOne(ctx, bson.M{"slug": slug}).Decode(&raw)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Printf("[FindBySlug] RESULT: No document found for slug: %q", slug)
			log.Printf("[FindBySlug] === DEBUG END (not found) ===")
			return nil, err
		}
		log.Printf("[FindBySlug] ERROR finding slug %q: %v", slug, err)
		log.Printf("[FindBySlug] === DEBUG END (error) ===")
		return nil, fmt.Errorf("find by slug: %w", err)
	}

	movie, err := normalizeMovieFromBSON(raw)
	if err != nil {
		log.Printf("[FindBySlug] ERROR normalizing document for slug %q: %v", slug, err)
		log.Printf("[FindBySlug] === DEBUG END (normalize error) ===")
		return nil, fmt.Errorf("normalize movie: %w", err)
	}

	log.Printf("[FindBySlug] RESULT: Found movie id=%v, slug=%q, title=%s", movie.ID, movie.Slug, movie.Title)
	log.Printf("[FindBySlug] RESULT: genre=%v (len=%d)", movie.Genre, len(movie.Genre))
	log.Printf("[FindBySlug] RESULT: source_type=%q, video_url=%q, embed_url=%q", movie.SourceType, movie.VideoURL, movie.EmbedURL)
	log.Printf("[FindBySlug] RESULT: views=%d, is_premium=%v", movie.Views, movie.IsPremium)
	log.Printf("[FindBySlug] === DEBUG END (found) ===")
	return movie, nil
}

// FindByID returns a single movie by ObjectID
func (r *MovieRepository) FindByID(id primitive.ObjectID) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{"_id": id})
	if err != nil {
		return nil, fmt.Errorf("find by id: %w", err)
	}
	defer cursor.Close(ctx)

	if !cursor.Next(ctx) {
		return nil, mongo.ErrNoDocuments
	}

	var doc bson.M
	if err := cursor.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode movie: %w", err)
	}

	movie, err := normalizeMovieFromBSON(doc)
	log.Printf("[FindByID] RESULT: movie id=%v, genre=%v (len=%d)", movie.ID, movie.Genre, len(movie.Genre))
	return movie, err
}

// IncrementViews atomically increments the view count for a movie
func (r *MovieRepository) IncrementViews(id primitive.ObjectID) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{"_id": id}
	update := bson.M{
		"$inc": bson.M{"views": 1},
	}

	_, err := r.col.UpdateOne(ctx, filter, update)
	return err
}

// FindByIDHex is a convenience method to find movie by hex string ID
func (r *MovieRepository) FindByIDHex(idHex string) (*models.Movie, error) {
	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return nil, err
	}
	return r.FindByID(id)
}

// FindByCode returns a single movie by alphanumeric code
func (r *MovieRepository) FindByCode(code string) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to find by string code first
	cursor, err := r.col.Find(ctx, bson.M{"code": code})
	if err != nil {
		return nil, fmt.Errorf("find by code: %w", err)
	}
	defer cursor.Close(ctx)

	if cursor.Next(ctx) {
		var doc bson.M
		if err := cursor.Decode(&doc); err != nil {
			return nil, fmt.Errorf("decode movie: %w", err)
		}
		return normalizeMovieFromBSON(doc)
	}

	// If not found by string, try numeric code (legacy)
	codeNum, err := strconv.Atoi(code)
	if err != nil {
		return nil, mongo.ErrNoDocuments
	}

	cursor2, err := r.col.Find(ctx, bson.M{"code": codeNum})
	if err != nil {
		return nil, fmt.Errorf("find by numeric code: %w", err)
	}
	defer cursor2.Close(ctx)

	if !cursor2.Next(ctx) {
		return nil, mongo.ErrNoDocuments
	}

	var doc bson.M
	if err := cursor2.Decode(&doc); err != nil {
		return nil, fmt.Errorf("decode movie: %w", err)
	}

	return normalizeMovieFromBSON(doc)
}

// FindByIDs returns multiple movies by their ObjectID hex strings
func (r *MovieRepository) FindByIDs(ids []string) ([]models.Movie, error) {
	if len(ids) == 0 {
		return []models.Movie{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Convert hex strings to ObjectIDs
	objectIDs := make([]primitive.ObjectID, 0, len(ids))
	for _, id := range ids {
		objID, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			continue
		}
		objectIDs = append(objectIDs, objID)
	}

	if len(objectIDs) == 0 {
		return []models.Movie{}, nil
	}

	cursor, err := r.col.Find(ctx, bson.M{"_id": bson.M{"$in": objectIDs}})
	if err != nil {
		return nil, fmt.Errorf("find by ids: %w", err)
	}
	defer cursor.Close(ctx)

	var movies []models.Movie
	if err := cursor.All(ctx, &movies); err != nil {
		return nil, fmt.Errorf("decode movies: %w", err)
	}

	if movies == nil {
		return []models.Movie{}, nil
	}

	return movies, nil
}

// GetMoviesByIDs retrieves multiple movies by their ObjectIDs, preserving order
func (r *MovieRepository) GetMoviesByIDs(ctx context.Context, ids []primitive.ObjectID) ([]models.Movie, error) {
	if len(ids) == 0 {
		return []models.Movie{}, nil
	}

	cursor, err := r.col.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("find by ids: %w", err)
	}
	defer cursor.Close(ctx)

	var movies []models.Movie
	if err := cursor.All(ctx, &movies); err != nil {
		return nil, fmt.Errorf("decode movies: %w", err)
	}

	if movies == nil {
		return []models.Movie{}, nil
	}

	// Preserve original order
	movieMap := make(map[primitive.ObjectID]models.Movie)
	for _, m := range movies {
		movieMap[m.ID] = m
	}

	orderedMovies := make([]models.Movie, 0, len(ids))
	for _, id := range ids {
		if m, ok := movieMap[id]; ok {
			orderedMovies = append(orderedMovies, m)
		}
	}

	return orderedMovies, nil
}

// FindByGenre returns movies matching the given genre
func (r *MovieRepository) FindByGenre(genres []string, limit int) ([]models.Movie, error) {
	if len(genres) == 0 {
		return []models.Movie{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if limit <= 0 {
		limit = 20
	}

	// Match any of the genres; only published content
	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
		"genre": bson.M{"$in": genres},
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find by genre: %w", err)
	}
	defer cursor.Close(ctx)

	var movies []models.Movie
	if err := cursor.All(ctx, &movies); err != nil {
		return nil, fmt.Errorf("decode movies: %w", err)
	}

	if movies == nil {
		return []models.Movie{}, nil
	}

	return movies, nil
}

// Search does a basic text search on title and description
func (r *MovieRepository) Search(query string) ([]models.Movie, error) {
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

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search movies: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode into bson.M first
	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, fmt.Errorf("decode search results: %w", err)
	}

	// Normalize each document
	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			log.Printf("Warning: failed to normalize movie in search: %v", err)
			continue
		}
		movies = append(movies, *movie)
	}

	return movies, nil
}

// FindByNormalizedTitleYear retrieves a movie by normalized title and year.
func (r *MovieRepository) FindByNormalizedTitleYear(normalizedTitle string, year int) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var movie models.Movie
	filter := bson.M{
		"normalized_title": normalizedTitle,
		"year":             year,
	}
	if err := r.col.FindOne(ctx, filter).Decode(&movie); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &movie, nil
}

// FindBySourceURL retrieves a movie by source provider and source URL.
func (r *MovieRepository) FindBySourceURL(provider, sourceURL string) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var movie models.Movie
	filter := bson.M{
		"source.provider":   provider,
		"source.source_url": sourceURL,
	}
	if err := r.col.FindOne(ctx, filter).Decode(&movie); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &movie, nil
}

// FindBySourceID retrieves a movie by source type and source ID
func (r *MovieRepository) FindBySourceID(provider, sourceID string) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var movie models.Movie
	filter := bson.M{
		"source.provider":  provider,
		"source.source_id": sourceID,
	}
	if err := r.col.FindOne(ctx, filter).Decode(&movie); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &movie, nil
}

// FindByTMDBID retrieves a movie by TMDB ID
func (r *MovieRepository) FindByTMDBID(tmdbID int) (*models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var movie models.Movie
	filter := bson.M{
		"tmdb_id": tmdbID,
	}
	if err := r.col.FindOne(ctx, filter).Decode(&movie); err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, err
	}
	return &movie, nil
}

// FindByYear retrieves all movies for a specific year.
func (r *MovieRepository) FindByYear(year int) ([]models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{"year": year})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, err
	}

	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}

	return movies, nil
}

// Create inserts a new movie
func (r *MovieRepository) Create(movie *models.Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	movie.Genre = normalizeGenreValues(movie.Genre)
	if movie.Genre == nil {
		movie.Genre = []string{}
	}
	log.Printf("[MOVIE REPO] Creating movie: title=%s, code=%s, genre=%v", movie.Title, movie.Code, movie.Genre)

	result, err := r.col.InsertOne(ctx, movie)
	if err != nil {
		log.Printf("[MOVIE REPO] Error creating movie: %v", err)
		return err
	}

	log.Printf("[MOVIE REPO] Movie created successfully: id=%s, title=%s, code=%s, genre=%v",
		result.InsertedID, movie.Title, movie.Code, movie.Genre)
	return nil
}

// Update replaces movie fields by ID
func (r *MovieRepository) Update(id primitive.ObjectID, movie *models.Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	movie.Genre = normalizeGenreValues(movie.Genre)
	if movie.Genre == nil {
		movie.Genre = []string{}
	}
	log.Printf("[MOVIE REPO] Updating movie id=%v, genre=%v", id, movie.Genre)

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{
			"$set": bson.M{
				"code":                        movie.Code,
				"slug":                        movie.Slug,
				"website_url":                 movie.WebsiteURL,
				"source":                      movie.Source,
				"title":                       movie.Title,
				"normalized_title":            movie.NormalizedTitle,
				"description":                 movie.Description,
				"poster_url":                  movie.PosterURL,
				"backdrop_url":                movie.BackdropURL,
				"year":                        movie.Year,
				"genre":                       movie.Genre,
				"country":                     movie.Country,
				"video_url":                   movie.VideoURL,
				"embed_url":                   movie.EmbedURL,
				"source_type":                 movie.SourceType,
				"duration":                    movie.Duration,
				"quality":                     movie.Quality,
				"views":                       movie.Views,
				"rating_avg":                  movie.RatingAvg,
				"rating_count":                movie.RatingCount,
				"is_premium":                  movie.IsPremium,
				"created_at":                  movie.CreatedAt,
				"updated_at":                  movie.UpdatedAt,
				"master_playlist_url":         movie.MasterPlaylistURL,
				"available_qualities":         movie.AvailableQualities,
				"generated_qualities":         movie.GeneratedQualities,
				"default_quality":             movie.DefaultQuality,
				"source_resolution":           movie.SourceResolution,
				"title_uz":                    movie.TitleUz,
				"description_uz":              movie.DescriptionUz,
				"genres_uz":                   movie.GenresUz,
				"countries_uz":                movie.CountriesUz,
				"original_title":              movie.OriginalTitle,
				"tmdb_id":                     movie.TMDBID,
				"metadata_source":             movie.MetadataSource,
				"approval_status":             movie.ApprovalStatus,
				"is_published":                movie.IsPublished,
				"approved_at":                 movie.ApprovedAt,
				"approved_by":                 movie.ApprovedBy,
				"telegram_posted_on_approval": movie.TelegramPostedOnApproval,
			},
			"$unset": bson.M{
				"genres":      "",
				"movie_genre": "",
			},
		},
	)
	if err != nil {
		log.Printf("[MOVIE REPO] Error updating movie: %v", err)
	} else {
		log.Printf("[MOVIE REPO] Movie updated successfully id=%v", id)
	}
	return err
}

// Delete removes a movie by ID
func (r *MovieRepository) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}

	_, err = r.col.DeleteOne(ctx, bson.M{"_id": objID})
	return err
}

// SlugExists checks if a slug is already taken
func (r *MovieRepository) SlugExists(slug string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := r.col.CountDocuments(ctx, bson.M{"slug": slug})
	return count > 0, err
}

// CodeExists checks if a code already exists
func (r *MovieRepository) CodeExists(code string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	count, err := r.col.CountDocuments(ctx, bson.M{"code": code})
	return count > 0, err
}

// FindHighestCode finds the highest numeric code in the movies collection
// Returns the numeric value (e.g., 8 for "0008", 100 for "0100")
func (r *MovieRepository) FindHighestCode() (int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cursor, err := r.col.Find(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("failed to query movies: %w", err)
	}
	defer cursor.Close(ctx)

	var highestSeq int64 = 0
	for cursor.Next(ctx) {
		var doc struct {
			Code string `bson:"code"`
		}
		if err := cursor.Decode(&doc); err != nil {
			continue
		}

		code := doc.Code
		if code == "" {
			continue
		}

		// Parse code as integer safely
		var seq int64
		_, err := fmt.Sscanf(code, "%d", &seq)
		if err == nil && seq > highestSeq {
			highestSeq = seq
		}
	}

	return highestSeq, nil
}

// FindMoviesWithoutCode returns movies that have no code assigned
func (r *MovieRepository) FindMoviesWithoutCode() ([]models.Movie, error) {
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

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("find movies without code: %w", err)
	}
	defer cursor.Close(ctx)

	// Decode into bson.M first
	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, fmt.Errorf("decode results: %w", err)
	}

	// Normalize each document
	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			log.Printf("Warning: failed to normalize movie: %v", err)
			continue
		}
		movies = append(movies, *movie)
	}

	return movies, nil
}

// ListAdmin returns ALL movies (regardless of approval status) for the admin dashboard.
func (r *MovieRepository) ListAdmin(page, limit int) ([]models.Movie, int64, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 100
	}

	filter := bson.M{}

	total, err := r.col.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count admin movies: %w", err)
	}

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * limit)).
		SetLimit(int64(limit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("find admin movies: %w", err)
	}
	defer cursor.Close(ctx)

	var rawDocs []bson.M
	if err := cursor.All(ctx, &rawDocs); err != nil {
		return nil, 0, fmt.Errorf("decode admin movies: %w", err)
	}

	movies := make([]models.Movie, 0, len(rawDocs))
	for _, doc := range rawDocs {
		movie, err := normalizeMovieFromBSON(doc)
		if err != nil {
			continue
		}
		movies = append(movies, *movie)
	}

	return movies, total, nil
}

// MarkTelegramPostedOnApproval sets telegram_posted_on_approval=true so a
// subsequent approval click doesn't re-post to Telegram.
func (r *MovieRepository) MarkTelegramPostedOnApproval(idHex string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return err
	}
	_, err = r.col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{"telegram_posted_on_approval": true}},
	)
	return err
}

// SetApprovalStatus sets the approval status and publishes/unpublishes a movie.
func (r *MovieRepository) SetApprovalStatus(idHex, status, byUserID string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	id, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		return err
	}

	now := time.Now()
	result, err := r.col.UpdateOne(
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
		return fmt.Errorf("movie not found")
	}
	return nil
}

// SetMovieCodeAndURL updates code, slug, and website_url for a movie
func (r *MovieRepository) SetMovieCodeAndURL(id primitive.ObjectID, code, slug, websiteURL string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"code":        code,
			"slug":        slug,
			"website_url": websiteURL,
		}},
	)
	return err
}

// RecommendationScore holds a movie with its computed recommendation score
type RecommendationScore struct {
	Movie  models.Movie
	Reason string
}

// GetRecommendations returns recommended movies based on content similarity, popularity, and optionally user history
// It uses a hybrid scoring strategy with genre matching, popularity, recency, and optional personalization
func (r *MovieRepository) GetRecommendations(currentMovieID string, userID string, limit int) ([]models.Movie, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current movie for comparison
	var currentMovie models.Movie
	var currentObjID primitive.ObjectID
	if currentMovieID != "" {
		objID, err := primitive.ObjectIDFromHex(currentMovieID)
		if err == nil {
			currentObjID = objID
			r.col.FindOne(ctx, bson.M{"_id": objID}).Decode(&currentMovie)
		}
	}

	// Build filter: only published approved movies
	filter := bson.M{
		"$or": []bson.M{
			{"is_published": true},
			{"is_published": bson.M{"$exists": false}},
		},
	}
	if currentMovieID != "" && currentObjID.IsZero() == false {
		filter["_id"] = bson.M{"$ne": currentObjID}
	}

	// Fetch candidates (more than needed for scoring)
	candidatesLimit := limit * 5
	if candidatesLimit < 50 {
		candidatesLimit = 50
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "views", Value: -1}, {Key: "updated_at", Value: -1}}).
		SetLimit(int64(candidatesLimit))

	cursor, err := r.col.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var candidates []models.Movie
	if err := cursor.All(ctx, &candidates); err != nil {
		return nil, err
	}

	// Collect user preferences if userID provided (simplified: could be extended to query watch history)
	var userPreferredGenres []string
	if userID != "" {
		// TODO: Extend to query watch_history collection for personalized recommendations
		// For now, falls back to content+popularity only
	}

	// Score each candidate
	type scoredMovie struct {
		movie models.Movie
		score int
	}
	var scoredMovies []scoredMovie

	for _, m := range candidates {
		score := 0
		reason := ""

		// Content similarity: genre match (+5 per matched genre)
		for _, cg := range m.Genre {
			for _, mg := range currentMovie.Genre {
				if strings.EqualFold(cg, mg) {
					score += 5
					reason += "genre+"
				}
			}
		}

		// Country match (+2)
		if m.Country != "" && currentMovie.Country != "" && strings.EqualFold(m.Country, currentMovie.Country) {
			score += 2
			reason += "country+"
		}

		// Year proximity
		yearDiff := m.Year - currentMovie.Year
		if yearDiff < 0 {
			yearDiff = -yearDiff
		}
		if yearDiff <= 1 {
			score += 2
			reason += "year+"
		} else if yearDiff <= 3 {
			score += 1
			reason += "year~"
		}

		// Quality match (+1)
		if m.Quality != "" && currentMovie.Quality != "" && m.Quality == currentMovie.Quality {
			score += 1
			reason += "quality+"
		}

		// Premium status match (+1)
		if m.IsPremium == currentMovie.IsPremium {
			score += 1
			reason += "premium+"
		}

		// User personalization boost (+3 per user-preferred genre, only if we have preferences)
		for _, upg := range userPreferredGenres {
			for _, mg := range m.Genre {
				if strings.EqualFold(upg, mg) {
					score += 3
					reason += "user+"
				}
			}
		}

		// Popularity boost (views as tiebreaker) - normalized to 0-2 range
		if m.Views > 10000 {
			score += 2
		} else if m.Views > 1000 {
			score += 1
		}
		if m.Views > 1000 {
			reason += "popular+"
		}

		// Recency: movies added/updated recently get a small boost
		if m.UpdatedAt.After(time.Now().AddDate(0, -1, 0)) { // within last month
			score += 1
			reason += "recent+"
		}

		scoredMovies = append(scoredMovies, scoredMovie{movie: m, score: score})
	}

	// Sort by score descending, then by views
	for i := 0; i < len(scoredMovies)-1; i++ {
		for j := i + 1; j < len(scoredMovies); j++ {
			if scoredMovies[j].score > scoredMovies[i].score ||
				(scoredMovies[j].score == scoredMovies[i].score && scoredMovies[j].movie.Views > scoredMovies[i].movie.Views) {
				scoredMovies[i], scoredMovies[j] = scoredMovies[j], scoredMovies[i]
			}
		}
	}

	// Build diverse result (avoid clustering same genre at top)
	var result []models.Movie
	var seenGenres = make(map[string]int)
	maxPerGenre := 3

	for _, sm := range scoredMovies {
		if len(result) >= limit {
			break
		}
		// Limit same genre appearances for diversity
		for _, g := range sm.movie.Genre {
			if seenGenres[strings.ToLower(g)] >= maxPerGenre {
				continue
			}
		}
		result = append(result, sm.movie)
		// Increment genre counts
		for _, g := range sm.movie.Genre {
			seenGenres[strings.ToLower(g)]++
		}
	}

	// If we still don't have enough results, fill with fallback (latest approved movies)
	if len(result) < limit {
		opts := options.Find().
			SetSort(bson.D{{Key: "approved_at", Value: -1}}).
			SetSkip(0).
			SetLimit(int64(limit - len(result)))
		fallbackFilter := bson.M{
			"$or": []bson.M{
				{"is_published": true},
				{"is_published": bson.M{"$exists": false}},
			},
		}
		if currentMovieID != "" && currentObjID.IsZero() == false {
			fallbackFilter["_id"] = bson.M{"$ne": currentObjID}
		}
		// Exclude already included
		if len(result) > 0 {
			var includedIDs []primitive.ObjectID
			for _, m := range result {
				includedIDs = append(includedIDs, m.ID)
			}
			fallbackFilter["_id"] = bson.M{"$nin": includedIDs}
		}
		fallbackCursor, err := r.col.Find(ctx, fallbackFilter, opts)
		if err == nil {
			defer fallbackCursor.Close(ctx)
			var fallbackMovies []models.Movie
			if fallbackCursor.All(ctx, &fallbackMovies) == nil {
				for _, m := range fallbackMovies {
					if len(result) >= limit {
						break
					}
					result = append(result, m)
				}
			}
		}
	}

	return result, nil
}
