package services

import (
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SeriesService struct {
	seriesRepo *repositories.SeriesRepository
}

func NewSeriesService(seriesRepo *repositories.SeriesRepository) *SeriesService {
	return &SeriesService{seriesRepo: seriesRepo}
}

// GenerateSlug creates a URL-friendly slug from title
func (s *SeriesService) GenerateSlug(title string) string {
	// Convert to lowercase and replace spaces with dashes
	slug := strings.ToLower(title)
	slug = strings.ReplaceAll(slug, " ", "-")
	// Remove special characters
	var result strings.Builder
	for _, c := range slug {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// CreateSeries creates a new series
func (s *SeriesService) CreateSeries(input *models.SeriesInput) (*models.Series, error) {
	series := &models.Series{
		Slug:        s.GenerateSlug(input.Title),
		Title:       input.Title,
		Description: input.Description,
		PosterURL:   input.PosterURL,
		BackdropURL: input.BackdropURL,
		Year:        input.Year,
		Genre:       input.Genre,
		Country:     input.Country,
		IsPremium:   input.IsPremium,
		IsCompleted: input.IsCompleted,

		// Approval workflow: new series start hidden pending admin review
		ApprovalStatus: "pending",
		IsPublished:    false,
	}

	// Check if slug already exists, append timestamp if needed
	existing, _ := s.seriesRepo.GetBySlug(series.Slug)
	if existing != nil {
		series.Slug = series.Slug + "-" + time.Now().Format("20060102")
	}

	err := s.seriesRepo.Create(series)
	if err != nil {
		return nil, err
	}

	return series, nil
}

// GetSeriesByID gets a series by ID
func (s *SeriesService) GetSeriesByID(id primitive.ObjectID) (*models.Series, error) {
	return s.seriesRepo.GetByID(id)
}

// GetSeriesBySlug gets a series by slug
func (s *SeriesService) GetSeriesBySlug(slug string) (*models.Series, error) {
	return s.seriesRepo.GetBySlug(slug)
}

// GetSeriesWithSeasons gets series with all seasons and episodes
func (s *SeriesService) GetSeriesWithSeasons(slug string) (*models.SeriesWithSeasons, error) {
	series, err := s.seriesRepo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}

	return s.seriesRepo.GetSeriesWithSeasons(series.ID)
}

// ListSeries lists all series
func (s *SeriesService) ListSeries(limit, skip int) ([]models.Series, error) {
	return s.seriesRepo.List(limit, skip)
}

// UpdateSeries updates a series
func (s *SeriesService) UpdateSeries(id primitive.ObjectID, input *models.SeriesInput) (*models.Series, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	series.Title = input.Title
	series.Description = input.Description
	series.PosterURL = input.PosterURL
	series.BackdropURL = input.BackdropURL
	series.Year = input.Year
	series.Genre = input.Genre
	series.Country = input.Country
	series.IsPremium = input.IsPremium
	series.IsCompleted = input.IsCompleted

	err = s.seriesRepo.Update(series)
	if err != nil {
		return nil, err
	}

	return series, nil
}

// DeleteSeries deletes a series
func (s *SeriesService) DeleteSeries(id primitive.ObjectID) error {
	return s.seriesRepo.Delete(id)
}

// CreateSeason creates a new season
func (s *SeriesService) CreateSeason(seriesID primitive.ObjectID, seasonNumber int, input *models.SeasonInput) (*models.Season, error) {
	season := &models.Season{
		SeriesID:     seriesID,
		SeasonNumber: seasonNumber,
		Title:        input.Title,
		PosterURL:    input.PosterURL,
		Description:  input.Description,
		ReleaseDate:  input.ReleaseDate,
	}

	err := s.seriesRepo.CreateSeason(season)
	if err != nil {
		return nil, err
	}

	return season, nil
}

// GetSeasonsBySeriesID gets all seasons for a series
func (s *SeriesService) GetSeasonsBySeriesID(seriesID primitive.ObjectID) ([]models.Season, error) {
	return s.seriesRepo.GetSeasonsBySeriesID(seriesID)
}

// CreateEpisode creates a new episode
func (s *SeriesService) CreateEpisode(seriesID, seasonID primitive.ObjectID, episodeNumber int, input *models.EpisodeInput) (*models.Episode, error) {
	airDate, _ := time.Parse("2006-01-02", input.AirDate)

	episode := &models.Episode{
		SeriesID:      seriesID,
		SeasonID:      seasonID,
		EpisodeNumber: episodeNumber,
		Title:         input.Title,
		Description:   input.Description,
		ThumbnailURL:  input.ThumbnailURL,
		VideoURL:      input.VideoURL,
		EmbedURL:      input.EmbedURL,
		Duration:      input.Duration,
		AirDate:       airDate,
	}

	err := s.seriesRepo.CreateEpisode(episode)
	if err != nil {
		return nil, err
	}

	return episode, nil
}

// GetEpisodeByID gets an episode by ID
func (s *SeriesService) GetEpisodeByID(id primitive.ObjectID) (*models.Episode, error) {
	return s.seriesRepo.GetEpisodeByID(id)
}

// GetEpisodesBySeasonID gets all episodes for a season
func (s *SeriesService) GetEpisodesBySeasonID(seasonID primitive.ObjectID) ([]models.Episode, error) {
	return s.seriesRepo.GetEpisodesBySeasonID(seasonID)
}

// IncrementViews increments series view count
func (s *SeriesService) IncrementSeriesViews(id primitive.ObjectID) error {
	return s.seriesRepo.IncrementViews(id)
}

// IncrementEpisodeViews increments episode view count
func (s *SeriesService) IncrementEpisodeViews(id primitive.ObjectID) error {
	return s.seriesRepo.IncrementEpisodeViews(id)
}

// DeleteEpisode deletes an episode by ID
func (s *SeriesService) DeleteEpisode(id primitive.ObjectID) error {
	return s.seriesRepo.DeleteEpisode(id)
}

// ListAllSeriesAdmin returns all series regardless of approval status for admin dashboard.
func (s *SeriesService) ListAllSeriesAdmin(limit, skip int) ([]models.Series, error) {
	return s.seriesRepo.ListAdmin(limit, skip)
}

// SetSeriesApprovalStatus approves or rejects a series.
func (s *SeriesService) SetSeriesApprovalStatus(id, status, byUserID string) error {
	return s.seriesRepo.SetApprovalStatus(id, status, byUserID)
}
