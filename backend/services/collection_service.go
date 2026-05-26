package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type CollectionService struct {
	collectionRepo *repositories.CollectionRepository
	movieRepo      *repositories.MovieRepository
	seriesRepo     *repositories.SeriesRepository
}

func NewCollectionService(collectionRepo *repositories.CollectionRepository, movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository) *CollectionService {
	return &CollectionService{
		collectionRepo: collectionRepo,
		movieRepo:      movieRepo,
		seriesRepo:     seriesRepo,
	}
}

// Create creates a new collection
func (s *CollectionService) Create(ctx context.Context, input models.CollectionInput) (*models.Collection, error) {
	// Validate input
	if strings.TrimSpace(input.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Slug) == "" {
		return nil, fmt.Errorf("slug is required")
	}

	// Check slug uniqueness
	exists, err := s.collectionRepo.CheckSlugExists(ctx, input.Slug, primitive.NilObjectID)
	if err != nil {
		return nil, fmt.Errorf("failed to check slug: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("slug '%s' already exists", input.Slug)
	}

	// Convert movie ID strings to ObjectIDs
	movieIDs, err := s.convertMovieIDs(input.MovieIDs)
	if err != nil {
		return nil, err
	}
	seriesIDs, err := s.convertObjectIDs(input.SeriesIDs, "series")
	if err != nil {
		return nil, err
	}

	collection := &models.Collection{
		Title:       input.Title,
		Slug:        input.Slug,
		Description: input.Description,
		PosterURL:   input.PosterURL,
		IsPublished: input.IsPublished,
		IsFeatured:  input.IsFeatured,
		SortOrder:   input.SortOrder,
		MovieIDs:    movieIDs,
		SeriesIDs:   seriesIDs,
	}

	if err := s.collectionRepo.Create(ctx, collection); err != nil {
		return nil, fmt.Errorf("failed to create collection: %w", err)
	}

	return collection, nil
}

// Update updates an existing collection
func (s *CollectionService) Update(ctx context.Context, idHex string, input models.CollectionInput) (*models.Collection, error) {
	// Get existing collection
	collection, err := s.collectionRepo.GetByIDHex(ctx, idHex)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}
	if collection == nil {
		return nil, fmt.Errorf("collection not found")
	}

	// Validate input
	if strings.TrimSpace(input.Title) == "" {
		return nil, fmt.Errorf("title is required")
	}
	if strings.TrimSpace(input.Slug) == "" {
		return nil, fmt.Errorf("slug is required")
	}

	// Check slug uniqueness (excluding current collection)
	exists, err := s.collectionRepo.CheckSlugExists(ctx, input.Slug, collection.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to check slug: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("slug '%s' already exists", input.Slug)
	}

	// Convert movie/series ID strings to ObjectIDs
	movieIDs, err := s.convertMovieIDs(input.MovieIDs)
	if err != nil {
		return nil, err
	}
	seriesIDs, err := s.convertObjectIDs(input.SeriesIDs, "series")
	if err != nil {
		return nil, err
	}

	// Update fields
	collection.Title = input.Title
	collection.Slug = input.Slug
	collection.Description = input.Description
	collection.PosterURL = input.PosterURL
	collection.IsPublished = input.IsPublished
	collection.IsFeatured = input.IsFeatured
	collection.SortOrder = input.SortOrder
	collection.MovieIDs = movieIDs
	collection.SeriesIDs = seriesIDs

	if err := s.collectionRepo.Update(ctx, collection.ID, collection); err != nil {
		return nil, fmt.Errorf("failed to update collection: %w", err)
	}

	return collection, nil
}

// Delete deletes a collection by ID
func (s *CollectionService) Delete(ctx context.Context, idHex string) error {
	collection, err := s.collectionRepo.GetByIDHex(ctx, idHex)
	if err != nil {
		return fmt.Errorf("failed to get collection: %w", err)
	}
	if collection == nil {
		return fmt.Errorf("collection not found")
	}

	return s.collectionRepo.Delete(ctx, collection.ID)
}

// GetByID gets a collection by ID (for admin)
func (s *CollectionService) GetByID(ctx context.Context, idHex string) (*models.Collection, error) {
	return s.collectionRepo.GetByIDHex(ctx, idHex)
}

// GetBySlug gets a published collection by slug (for public)
func (s *CollectionService) GetBySlug(ctx context.Context, slug string) (*models.CollectionWithMovies, error) {
	collection, err := s.collectionRepo.GetBySlug(ctx, slug)
	if err != nil {
		return nil, fmt.Errorf("failed to get collection: %w", err)
	}
	if collection == nil {
		return nil, nil
	}

	// Only return published collections publicly
	if !collection.IsPublished {
		return nil, nil
	}

	return s.populateCollectionWithMovies(ctx, collection)
}

// GetAll gets all collections (for admin)
func (s *CollectionService) GetAll(ctx context.Context) ([]models.Collection, error) {
	return s.collectionRepo.GetAll(ctx)
}

// GetPublished gets all published collections (for public)
func (s *CollectionService) GetPublished(ctx context.Context) ([]models.CollectionWithMovies, error) {
	collections, err := s.collectionRepo.GetPublished(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get published collections: %w", err)
	}

	return s.populateCollectionsWithMovies(ctx, collections)
}

// GetFeatured gets featured and published collections (for public)
func (s *CollectionService) GetFeatured(ctx context.Context) ([]models.CollectionWithMovies, error) {
	collections, err := s.collectionRepo.GetFeatured(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get featured collections: %w", err)
	}

	return s.populateCollectionsWithMovies(ctx, collections)
}

// convertMovieIDs converts string movie IDs to ObjectIDs
func (s *CollectionService) convertMovieIDs(movieIDStrings []string) ([]primitive.ObjectID, error) {
	return s.convertObjectIDs(movieIDStrings, "movie")
}

// convertObjectIDs converts string IDs to ObjectIDs, tagging errors with the
// resource label ("movie" or "series") for clearer messages.
func (s *CollectionService) convertObjectIDs(idStrings []string, label string) ([]primitive.ObjectID, error) {
	ids := make([]primitive.ObjectID, 0, len(idStrings))
	for _, idStr := range idStrings {
		idStr = strings.TrimSpace(idStr)
		if idStr == "" {
			continue
		}
		objID, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			return nil, fmt.Errorf("invalid %s id '%s': %w", label, idStr, err)
		}
		ids = append(ids, objID)
	}
	return ids, nil
}

// populateCollectionWithMovies populates movie + series data for a single collection
func (s *CollectionService) populateCollectionWithMovies(ctx context.Context, collection *models.Collection) (*models.CollectionWithMovies, error) {
	movies, err := s.movieRepo.GetMoviesByIDs(ctx, collection.MovieIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to get movies: %w", err)
	}

	movieBasics := make([]models.MovieBasic, 0, len(movies))
	for _, m := range movies {
		movieBasics = append(movieBasics, models.MovieBasic{
			ID:          m.ID.Hex(),
			Code:        m.Code,
			Title:       m.Title,
			Slug:        m.Slug,
			PosterURL:   m.PosterURL,
			Year:        m.Year,
			Genre:       m.Genre,
			Genres:      m.Genre,
			Duration:    m.Duration,
			Quality:     m.Quality,
			RatingAvg:   m.RatingAvg,
			RatingCount: m.RatingCount,
			IsPremium:   m.IsPremium,
			CreatedAt:   m.CreatedAt.Format(time.RFC3339),
		})
	}

	seriesBasics := []models.SeriesBasic{}
	if len(collection.SeriesIDs) > 0 && s.seriesRepo != nil {
		seriesList, err := s.seriesRepo.GetSeriesByIDs(ctx, collection.SeriesIDs)
		if err != nil {
			return nil, fmt.Errorf("failed to get series: %w", err)
		}
		for _, sr := range seriesList {
			seriesBasics = append(seriesBasics, models.SeriesBasic{
				ID:          sr.ID.Hex(),
				Code:        sr.Code,
				Title:       sr.Title,
				Slug:        sr.Slug,
				PosterURL:   sr.PosterURL,
				Year:        sr.Year,
				Genre:       sr.Genre,
				Genres:      sr.Genre,
				Quality:     sr.Quality,
				RatingAvg:   sr.RatingAvg,
				RatingCount: sr.RatingCount,
				IsPremium:   sr.IsPremium,
				CreatedAt:   sr.CreatedAt.Format(time.RFC3339),
			})
		}
	}

	return &models.CollectionWithMovies{
		ID:          collection.ID.Hex(),
		Title:       collection.Title,
		Slug:        collection.Slug,
		Description: collection.Description,
		PosterURL:   collection.PosterURL,
		SortOrder:   collection.SortOrder,
		Movies:      movieBasics,
		Series:      seriesBasics,
	}, nil
}

// populateCollectionsWithMovies populates movie data for multiple collections
func (s *CollectionService) populateCollectionsWithMovies(ctx context.Context, collections []models.Collection) ([]models.CollectionWithMovies, error) {
	result := make([]models.CollectionWithMovies, 0, len(collections))
	for _, c := range collections {
		withMovies, err := s.populateCollectionWithMovies(ctx, &c)
		if err != nil {
			// Skip collections with invalid movie references
			continue
		}
		result = append(result, *withMovies)
	}
	return result, nil
}
