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

	_, err := r.col.Indexes().CreateMany(ctx, []mongo.IndexModel{codeIndex, slugIndex})
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

	// Handle genre - may be array or single value
	if genre, ok := doc["genre"]; ok && genre != nil {
		switch g := genre.(type) {
		case []interface{}:
			genres := make([]string, 0, len(g))
			for _, item := range g {
				if s, ok := item.(string); ok {
					genres = append(genres, s)
				}
			}
			movie.Genre = genres
		case []string:
			movie.Genre = g
		}
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

	// Handle title_uz
	if titleUz, ok := doc["title_uz"].(string); ok {
		movie.TitleUz = titleUz
	}

	// Handle description_uz
	if descUz, ok := doc["description_uz"].(string); ok {
		movie.DescriptionUz = descUz
	}

	// Handle genres_uz — stored as []string or []interface{}
	if gUz, ok := doc["genres_uz"]; ok && gUz != nil {
		switch g := gUz.(type) {
		case []interface{}:
			arr := make([]string, 0, len(g))
			for _, item := range g {
				if s, ok := item.(string); ok {
					arr = append(arr, s)
				}
			}
			movie.GenresUz = arr
		case []string:
			movie.GenresUz = g
		}
	}

	// Handle countries_uz — may be stored as string (legacy) or []string/[]interface{}
	if cUz, ok := doc["countries_uz"]; ok && cUz != nil {
		switch c := cUz.(type) {
		case string:
			if c != "" {
				movie.CountriesUz = strings.Split(c, ", ")
			}
		case []interface{}:
			arr := make([]string, 0, len(c))
			for _, item := range c {
				if s, ok := item.(string); ok {
					arr = append(arr, s)
				}
			}
			movie.CountriesUz = arr
		case []string:
			movie.CountriesUz = c
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

	return normalizeMovieFromBSON(doc)
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

// Create inserts a new movie
func (r *MovieRepository) Create(movie *models.Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	log.Printf("[MOVIE REPO] Creating movie: title=%s, code=%s", movie.Title, movie.Code)

	result, err := r.col.InsertOne(ctx, movie)
	if err != nil {
		log.Printf("[MOVIE REPO] Error creating movie: %v", err)
		return err
	}

	log.Printf("[MOVIE REPO] Movie created successfully: id=%s, title=%s, code=%s",
		result.InsertedID, movie.Title, movie.Code)
	return nil
}

// Update replaces movie fields by ID
func (r *MovieRepository) Update(id primitive.ObjectID, movie *models.Movie) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := r.col.UpdateOne(
		ctx,
		bson.M{"_id": id},
		bson.M{"$set": movie},
	)
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
