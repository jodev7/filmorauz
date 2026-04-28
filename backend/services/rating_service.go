package services

import (
	"fmt"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type RatingService struct {
	ratingRepo        *repositories.MovieRatingRepository
	seriesRatingRepo  *repositories.SeriesRatingRepository
	episodeRatingRepo *repositories.EpisodeRatingRepository
	movieRepo         *repositories.MovieRepository
	seriesRepo        *repositories.SeriesRepository
}

func NewRatingService(ratingRepo *repositories.MovieRatingRepository, seriesRatingRepo *repositories.SeriesRatingRepository, episodeRatingRepo *repositories.EpisodeRatingRepository, movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository) *RatingService {
	return &RatingService{
		ratingRepo:        ratingRepo,
		seriesRatingRepo:  seriesRatingRepo,
		episodeRatingRepo: episodeRatingRepo,
		movieRepo:         movieRepo,
		seriesRepo:        seriesRepo,
	}
}

// SetRating creates or updates a user's rating for a movie
func (s *RatingService) SetRating(movieID, userID string, rating int) error {
	// Validate rating value
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	// Parse movie ID
	movieOID, err := primitive.ObjectIDFromHex(movieID)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}

	// Validate movie exists
	_, err = s.movieRepo.FindByID(movieOID)
	if err != nil {
		return fmt.Errorf("movie not found")
	}

	// Upsert the rating
	_, err = s.ratingRepo.UpsertRating(userOID, movieOID, rating)
	if err != nil {
		return fmt.Errorf("failed to save rating: %w", err)
	}

	// Update movie's rating aggregate
	err = s.ratingRepo.UpdateMovieRatingFields(movieOID)
	if err != nil {
		return fmt.Errorf("failed to update movie rating: %w", err)
	}

	return nil
}

// DeleteRating removes a user's rating for a movie
func (s *RatingService) DeleteRating(movieID, userID string) error {
	// Parse movie ID
	movieOID, err := primitive.ObjectIDFromHex(movieID)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}

	// Delete the rating
	err = s.ratingRepo.DeleteRating(userOID, movieOID)
	if err != nil {
		return fmt.Errorf("failed to delete rating: %w", err)
	}

	// Update movie's rating aggregate
	err = s.ratingRepo.UpdateMovieRatingFields(movieOID)
	if err != nil {
		return fmt.Errorf("failed to update movie rating: %w", err)
	}

	return nil
}

// GetRatingSummary returns the rating summary for a movie, optionally including user's rating
func (s *RatingService) GetRatingSummary(movieID string, userID *string) (*models.RatingSummary, error) {
	// Parse movie ID
	movieOID, err := primitive.ObjectIDFromHex(movieID)
	if err != nil {
		return nil, fmt.Errorf("invalid movie id")
	}

	// Get rating summary from repository
	avg, count, err := s.ratingRepo.GetRatingSummary(movieOID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rating summary: %w", err)
	}

	summary := &models.RatingSummary{
		RatingAvg:   avg,
		RatingCount: count,
	}

	// If user ID is provided, get user's rating
	if userID != nil && *userID != "" {
		userOID, err := primitive.ObjectIDFromHex(*userID)
		if err == nil {
			userRating, err := s.ratingRepo.GetUserRating(userOID, movieOID)
			if err == nil && userRating != nil {
				summary.UserRating = userRating
			}
		}
	}

	return summary, nil
}

// SetSeriesRating creates or updates a user's rating for a series
func (s *RatingService) SetSeriesRating(seriesID, userID string, rating int) error {
	// Validate rating value
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}

	// Parse series ID
	seriesOID, err := primitive.ObjectIDFromHex(seriesID)
	if err != nil {
		return fmt.Errorf("invalid series id")
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}

	// Validate series exists
	_, err = s.seriesRepo.GetByID(seriesOID)
	if err != nil {
		return fmt.Errorf("series not found")
	}

	// Upsert the rating
	_, err = s.seriesRatingRepo.UpsertRating(userOID, seriesOID, rating)
	if err != nil {
		return fmt.Errorf("failed to save rating: %w", err)
	}

	// Update series's rating aggregate
	err = s.seriesRatingRepo.UpdateSeriesRatingFields(seriesOID)
	if err != nil {
		return fmt.Errorf("failed to update series rating: %w", err)
	}

	return nil
}

// DeleteSeriesRating removes a user's rating for a series
func (s *RatingService) DeleteSeriesRating(seriesID, userID string) error {
	// Parse series ID
	seriesOID, err := primitive.ObjectIDFromHex(seriesID)
	if err != nil {
		return fmt.Errorf("invalid series id")
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}

	// Delete the rating
	err = s.seriesRatingRepo.DeleteRating(userOID, seriesOID)
	if err != nil {
		return fmt.Errorf("failed to delete rating: %w", err)
	}

	// Update series's rating aggregate
	err = s.seriesRatingRepo.UpdateSeriesRatingFields(seriesOID)
	if err != nil {
		return fmt.Errorf("failed to update series rating: %w", err)
	}

	return nil
}

// GetSeriesRatingSummary returns the aggregate rating for a series. The aggregate
// is computed from all per-episode ratings rather than the legacy series_ratings
// collection — series no longer rate as a single unit; the score reflects the
// average of every rating across every episode in the series.
func (s *RatingService) GetSeriesRatingSummary(seriesID string, userID *string) (*models.RatingSummary, error) {
	// Parse series ID
	seriesOID, err := primitive.ObjectIDFromHex(seriesID)
	if err != nil {
		return nil, fmt.Errorf("invalid series id")
	}

	avg, count, err := s.episodeRatingRepo.GetSeriesRatingFromEpisodes(seriesOID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rating summary: %w", err)
	}

	summary := &models.RatingSummary{
		RatingAvg:   avg,
		RatingCount: count,
	}

	_ = userID // Series no longer carry per-user ratings; episode-level rating provides that.

	return summary, nil
}

// SetEpisodeRating creates or updates a user's rating for an episode.
func (s *RatingService) SetEpisodeRating(episodeID, userID string, rating int) error {
	if rating < 1 || rating > 5 {
		return fmt.Errorf("rating must be between 1 and 5")
	}
	episodeOID, err := primitive.ObjectIDFromHex(episodeID)
	if err != nil {
		return fmt.Errorf("invalid episode id")
	}
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}
	return s.episodeRatingRepo.UpsertRating(userOID, episodeOID, rating)
}

// DeleteEpisodeRating removes a user's rating for an episode.
func (s *RatingService) DeleteEpisodeRating(episodeID, userID string) error {
	episodeOID, err := primitive.ObjectIDFromHex(episodeID)
	if err != nil {
		return fmt.Errorf("invalid episode id")
	}
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return fmt.Errorf("invalid user id")
	}
	return s.episodeRatingRepo.DeleteRating(userOID, episodeOID)
}

// GetEpisodeRatingSummary returns aggregate rating for an episode plus the viewer's own rating.
func (s *RatingService) GetEpisodeRatingSummary(episodeID string, userID *string) (*models.RatingSummary, error) {
	episodeOID, err := primitive.ObjectIDFromHex(episodeID)
	if err != nil {
		return nil, fmt.Errorf("invalid episode id")
	}
	avg, count, err := s.episodeRatingRepo.GetRatingSummary(episodeOID)
	if err != nil {
		return nil, fmt.Errorf("failed to get rating summary: %w", err)
	}
	summary := &models.RatingSummary{RatingAvg: avg, RatingCount: count}
	if userID != nil && *userID != "" {
		if userOID, err := primitive.ObjectIDFromHex(*userID); err == nil {
			if r, err := s.episodeRatingRepo.GetUserRating(userOID, episodeOID); err == nil && r != nil {
				summary.UserRating = r
			}
		}
	}
	return summary, nil
}
