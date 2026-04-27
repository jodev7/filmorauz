package services

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Slug validation regex: lowercase letters, numbers, hyphens only
var seriesSlugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func isValidSeriesSlug(slug string) bool {
	return slug != "" && seriesSlugRegex.MatchString(slug)
}

type SeriesService struct {
	seriesRepo *repositories.SeriesRepository
	movieRepo  *repositories.MovieRepository
	clipRepo   *repositories.ClipRepository
	b2Cleanup  *B2CleanupService
}

func NewSeriesService(seriesRepo *repositories.SeriesRepository, movieRepo *repositories.MovieRepository) *SeriesService {
	return &SeriesService{
		seriesRepo: seriesRepo,
		movieRepo:  movieRepo,
	}
}

func (s *SeriesService) SetStorageDependencies(clipRepo *repositories.ClipRepository, b2 *B2CleanupService) {
	s.clipRepo = clipRepo
	s.b2Cleanup = b2
}

// normalizeGenres trims, lowercases, and dedupes genre strings.
// Backend + DB store genres as lowercase English (e.g. "drama", "comedy").
func normalizeGenres(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, g := range in {
		g = normalizeSeriesGenreKey(g)
		if g == "" {
			continue
		}
		if _, ok := seen[g]; ok {
			continue
		}
		seen[g] = struct{}{}
		out = append(out, g)
	}
	return out
}

func normalizeSeriesGenreKey(raw string) string {
	g := strings.ToLower(strings.TrimSpace(raw))
	if g == "" {
		return ""
	}
	g = strings.ReplaceAll(g, "_", "-")
	g = strings.Join(strings.FieldsFunc(g, func(r rune) bool {
		return r == ' ' || r == '-'
	}), "-")
	switch g {
	case "science-fiction", "sciencefiction", "scifi":
		return "sci-fi"
	default:
		return g
	}
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
	slug := strings.TrimSpace(input.Slug)
	if slug == "" {
		slug = s.GenerateSlug(input.Title)
	} else {
		if !isValidSeriesSlug(slug) {
			return nil, fmt.Errorf("invalid slug: use only lowercase letters, numbers, and hyphens")
		}
	}

	series := &models.Series{
		Code:        "",
		Slug:        slug,
		Title:       input.Title,
		Description: input.Description,
		PosterURL:   input.PosterURL,
		BackdropURL: input.BackdropURL,
		Year:        input.Year,
		Genre:       normalizeGenres(input.Genre),
		Country:     input.Country,
		IsPremium:   input.IsPremium,
		IsCompleted: input.IsCompleted,

		// Approval workflow: new series start hidden pending admin review
		ApprovalStatus: "pending",
		IsPublished:    false,
	}

	// Check if slug already exists, append timestamp if it was auto-generated.
	existing, _ := s.seriesRepo.GetBySlug(series.Slug)
	if existing != nil {
		if strings.TrimSpace(input.Slug) != "" {
			return nil, fmt.Errorf("slug already in use by another series")
		}
		series.Slug = series.Slug + "-" + time.Now().Format("20060102")
	}

	if strings.TrimSpace(series.Code) == "" {
		code, err := getNextContentCode(s.movieRepo, s.seriesRepo)
		if err != nil {
			return nil, err
		}
		series.Code = code
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
func (s *SeriesService) ListSeries(limit, skip int, genre string) ([]models.Series, error) {
	return s.seriesRepo.List(limit, skip, genre)
}

// UpdateSeries updates a series
func (s *SeriesService) UpdateSeries(id primitive.ObjectID, input *models.SeriesInput) (*models.Series, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	// Validate and update slug if provided
	if input.Slug != "" {
		if !isValidSeriesSlug(input.Slug) {
			return nil, fmt.Errorf("invalid slug: use only lowercase letters, numbers, and hyphens")
		}
		existingSlug, err := s.seriesRepo.GetBySlug(input.Slug)
		if err == nil && existingSlug != nil && existingSlug.ID != id {
			return nil, fmt.Errorf("slug already in use by another series")
		}
		series.Slug = input.Slug
	}

	series.Title = input.Title
	series.Description = input.Description
	if input.PosterURL != "" {
		series.PosterURL = input.PosterURL
	}
	if input.BackdropURL != "" {
		series.BackdropURL = input.BackdropURL
	}
	series.Year = input.Year
	series.Genre = normalizeGenres(input.Genre)
	series.Country = input.Country
	series.IsPremium = input.IsPremium
	series.IsCompleted = input.IsCompleted

	err = s.seriesRepo.Update(series)
	if err != nil {
		return nil, err
	}

	return series, nil
}

// BackfillSeriesCodes assigns codes to existing series that don't have one.
// It is safe to call on startup.
func (s *SeriesService) BackfillSeriesCodes() {
	seriesList, err := s.seriesRepo.FindSeriesWithoutCode()
	if err != nil {
		log.Printf("Backfill: failed to find series without code: %v", err)
		return
	}
	if len(seriesList) == 0 {
		return
	}

	log.Printf("Backfill: assigning codes to %d existing series...", len(seriesList))
	for _, series := range seriesList {
		code, err := getNextContentCode(s.movieRepo, s.seriesRepo)
		if err != nil {
			log.Printf("Backfill: failed to generate code for series %s: %v", series.ID.Hex(), err)
			continue
		}
		if err := s.seriesRepo.SetSeriesCode(series.ID, code); err != nil {
			log.Printf("Backfill: failed to set code %s for series %s: %v", code, series.ID.Hex(), err)
			continue
		}
		log.Printf("Backfill: assigned code %s to series '%s'", code, series.Title)
	}
	log.Printf("Backfill: series complete")
}

// DeleteSeries deletes a series and its B2-backed assets.
func (s *SeriesService) DeleteSeries(id primitive.ObjectID) error {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return err
	}
	if err := s.cleanupSeriesStorage(series); err != nil {
		return err
	}
	return s.seriesRepo.Delete(id)
}

func (s *SeriesService) cleanupSeriesStorage(series *models.Series) error {
	if series == nil {
		return nil
	}

	if s.b2Cleanup != nil {
		episodes, err := s.seriesRepo.GetEpisodesBySeriesID(series.ID)
		if err != nil {
			return fmt.Errorf("load series episodes: %w", err)
		}
		seasons, err := s.seriesRepo.GetSeasonsBySeriesID(series.ID)
		if err != nil {
			return fmt.Errorf("load series seasons: %w", err)
		}

		prefix := s.deriveSeriesDeletePrefix(series, episodes)
		if prefix != "" {
			if err := validateB2DeletePrefixForSeries(prefix, *series); err != nil {
				return err
			}
			filesCount, err := s.countFilesByPrefix(prefix)
			log.Printf("[B2_DELETE] movie_id=%s", series.ID.Hex())
			log.Printf("[B2_DELETE] title=%q", series.Title)
			log.Printf("[B2_DELETE] prefix=%s", prefix)
			log.Printf("[B2_DELETE] files_count=%d", filesCount)
			if err != nil {
				return fmt.Errorf("list b2 prefix %q: %w", prefix, err)
			}
			if filesCount > 500 || isUnsafeB2DeletePrefix(prefix) {
				return fmt.Errorf("unsafe delete prefix")
			}
			if _, err := s.b2Cleanup.DeleteByPrefix(prefix); err != nil {
				return fmt.Errorf("delete series b2 prefix %q: %w", prefix, err)
			}
		}

		keys := []string{
			s.b2Cleanup.ExtractKeyFromURL(series.PosterURL),
			s.b2Cleanup.ExtractKeyFromURL(series.BackdropURL),
		}
		for _, season := range seasons {
			keys = append(keys, s.b2Cleanup.ExtractKeyFromURL(season.PosterURL))
		}
		for _, episode := range episodes {
			videoKey := s.b2Cleanup.ExtractKeyFromURL(episode.VideoURL)
			if prefix == "" || !strings.HasPrefix(videoKey, normalizeB2DeletePrefix(prefix)) {
				keys = append(keys, videoKey)
			}
			keys = append(keys, s.b2Cleanup.ExtractKeyFromURL(episode.ThumbnailURL))
		}
		for _, key := range dedupeKeys(keys) {
			if _, err := s.b2Cleanup.DeleteByKey(key); err != nil {
				return fmt.Errorf("delete series asset key %q: %w", key, err)
			}
		}
	}

	if s.clipRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		clips, err := s.clipRepo.FindBySeriesID(ctx, series.ID)
		if err != nil {
			return fmt.Errorf("load series clips: %w", err)
		}
		if s.b2Cleanup != nil {
			for _, clip := range clips {
				key := firstNonEmptyTrimmed(s.b2Cleanup.ExtractKeyFromURL(clip.URL), strings.TrimPrefix(clip.Path, "/"))
				if key == "" {
					continue
				}
				if _, err := s.b2Cleanup.DeleteByKey(key); err != nil {
					return fmt.Errorf("delete series clip key %q: %w", key, err)
				}
			}
		}
		if err := s.clipRepo.DeleteBySeriesID(ctx, series.ID); err != nil {
			return fmt.Errorf("delete series clip rows: %w", err)
		}
	}
	return nil
}

func (s *SeriesService) deriveSeriesDeletePrefix(series *models.Series, episodes []models.Episode) string {
	if s.b2Cleanup == nil || series == nil {
		return ""
	}
	for _, episode := range episodes {
		key := strings.TrimSpace(s.b2Cleanup.ExtractKeyFromURL(episode.VideoURL))
		if key == "" {
			continue
		}
		parts := strings.Split(strings.Trim(key, "/"), "/")
		if len(parts) < 4 || parts[0] != "videos" || parts[1] != "serials" || parts[2] == "" {
			continue
		}
		prefix := strings.Join(parts[:3], "/") + "/"
		if validateB2DeletePrefixForSeries(prefix, *series) == nil {
			return prefix
		}
	}
	return ""
}

func (s *SeriesService) countFilesByPrefix(prefix string) (int, error) {
	if s.b2Cleanup == nil {
		return 0, nil
	}
	prefix = normalizeB2DeletePrefix(prefix)
	if prefix == "" {
		return 0, nil
	}
	pages, err := s.b2Cleanup.listByPrefix(prefix)
	if err != nil {
		return 0, err
	}
	total := 0
	for _, page := range pages {
		for _, file := range page.Files {
			if file.Action == "folder" {
				continue
			}
			total++
		}
	}
	return total, nil
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

// GetSeasonByID gets a season by ID
func (s *SeriesService) GetSeasonByID(id primitive.ObjectID) (*models.Season, error) {
	return s.seriesRepo.GetSeasonByID(id)
}

// UpdateSeason updates season metadata such as title
func (s *SeriesService) UpdateSeason(id primitive.ObjectID, input *models.SeasonInput) (*models.Season, error) {
	season, err := s.seriesRepo.GetSeasonByID(id)
	if err != nil {
		return nil, err
	}

	if input.Title != "" {
		season.Title = input.Title
	}
	if input.PosterURL != "" {
		season.PosterURL = input.PosterURL
	}
	if input.Description != "" {
		season.Description = input.Description
	}
	if !input.ReleaseDate.IsZero() {
		season.ReleaseDate = input.ReleaseDate
	}

	if err := s.seriesRepo.UpdateSeason(season); err != nil {
		return nil, err
	}

	return season, nil
}

// DeleteSeason deletes a season only if it has no episodes
func (s *SeriesService) DeleteSeason(id primitive.ObjectID) error {
	episodes, err := s.seriesRepo.GetEpisodesBySeasonID(id)
	if err != nil {
		return err
	}
	if len(episodes) > 0 {
		return fmt.Errorf("cannot delete season with episodes; move or delete episodes first")
	}
	return s.seriesRepo.DeleteSeason(id)
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
		SourceType:    inferEpisodeSourceType(input.SourceType, input.VideoURL, input.EmbedURL),
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

// UpdateEpisode updates an episode's details including season and episode number
func (s *SeriesService) UpdateEpisode(episode *models.Episode) error {
	episode.SourceType = inferEpisodeSourceType(episode.SourceType, episode.VideoURL, episode.EmbedURL)
	return s.seriesRepo.UpdateEpisode(episode)
}

func inferEpisodeSourceType(sourceType models.VideoSourceType, videoURL, embedURL string) models.VideoSourceType {
	if sourceType != "" {
		return sourceType
	}
	if embedURL != "" {
		return models.VideoSourceIframeEmbed
	}
	videoURL = strings.ToLower(videoURL)
	switch {
	case strings.Contains(videoURL, ".m3u8"), strings.Contains(videoURL, "/master.m3u8"):
		return models.VideoSourceDirectHLS
	case strings.Contains(videoURL, ".mp4"):
		return models.VideoSourceDirectMP4
	case videoURL != "":
		return models.VideoSourceDirectMP4
	default:
		return models.VideoSourceIframeEmbed
	}
}

// ReorderEpisodesInSeason reorders episodes within a season
func (s *SeriesService) ReorderEpisodesInSeason(seasonID primitive.ObjectID, episodeIDs []primitive.ObjectID) error {
	return s.seriesRepo.ReorderEpisodesInSeason(seasonID, episodeIDs)
}

// MoveEpisodeToSeason moves an episode to a different season with a new episode number
func (s *SeriesService) MoveEpisodeToSeason(episodeID, newSeasonID primitive.ObjectID, newEpisodeNumber int) error {
	season, err := s.seriesRepo.GetSeasonByID(newSeasonID)
	if err != nil {
		return err
	}
	if season == nil {
		return fmt.Errorf("season not found")
	}
	return s.seriesRepo.MoveEpisodeToSeason(episodeID, newSeasonID, newEpisodeNumber)
}

// UpdateSeasonEpisodes updates the full episode structure for a season (reorder + move)
func (s *SeriesService) UpdateSeasonEpisodes(seasonID primitive.ObjectID, episodes []models.Episode) error {
	for i, ep := range episodes {
		ep.EpisodeNumber = i + 1
		ep.SeasonID = seasonID
		if err := s.seriesRepo.UpdateEpisode(&ep); err != nil {
			return err
		}
	}
	return nil
}

// ListAllSeriesAdmin returns all series regardless of approval status for admin dashboard.
func (s *SeriesService) ListAllSeriesAdmin(limit, skip int) ([]models.Series, error) {
	return s.seriesRepo.ListAdmin(limit, skip)
}

// SetSeriesApprovalStatus approves or rejects a series.
func (s *SeriesService) SetSeriesApprovalStatus(id, status, byUserID string) error {
	return s.seriesRepo.SetApprovalStatus(id, status, byUserID)
}
