package services

import (
	"context"
	"fmt"
	"log"
	"os"
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
	seriesRepo     *repositories.SeriesRepository
	movieRepo      *repositories.MovieRepository
	clipRepo       *repositories.ClipRepository
	igScheduleRepo *repositories.InstagramScheduleRepository
	publishJobRepo *repositories.PublishJobRepository
	jobRepo        *repositories.JobRepository
	b2Cleanup      *B2CleanupService
}

// SetJobRepository wires the ingestion-jobs repository so DeleteSeries
// can cascade-purge job rows. Optional.
func (s *SeriesService) SetJobRepository(jobRepo *repositories.JobRepository) {
	s.jobRepo = jobRepo
}

func NewSeriesService(seriesRepo *repositories.SeriesRepository, movieRepo *repositories.MovieRepository) *SeriesService {
	return &SeriesService{
		seriesRepo: seriesRepo,
		movieRepo:  movieRepo,
	}
}

// SetStorageDependencies wires optional cleanup helpers so DeleteSeries
// can purge B2 assets, clip rows, and clip-linked publish/schedule jobs.
// Each argument is independently optional.
func (s *SeriesService) SetStorageDependencies(
	clipRepo *repositories.ClipRepository,
	igScheduleRepo *repositories.InstagramScheduleRepository,
	publishJobRepo *repositories.PublishJobRepository,
	b2 *B2CleanupService,
) {
	s.clipRepo = clipRepo
	s.igScheduleRepo = igScheduleRepo
	s.publishJobRepo = publishJobRepo
	s.b2Cleanup = b2
}

// SeriesDeleteResult is the structured outcome of a cascade series delete.
type SeriesDeleteResult struct {
	SeriesID             string           `json:"series_id"`
	Title                string           `json:"title"`
	SeasonsDeleted       int              `json:"seasons_deleted"`
	EpisodesDeleted      int              `json:"episodes_deleted"`
	ClipsDeleted         int              `json:"clips_deleted"`
	IngestionJobsDeleted int64            `json:"ingestion_jobs_deleted"`
	IGSchedulesDeleted   int64            `json:"instagram_schedules_deleted"`
	PublishJobsDeleted   int64            `json:"publish_jobs_deleted"`
	Partial              bool             `json:"partial"`
	B2                   *B2DeleteSummary `json:"b2"`
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

	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}

	err := s.seriesRepo.Create(series)
	if err != nil {
		return nil, err
	}

	return series, nil
}

// GetSeriesByID gets a series by ID
func (s *SeriesService) GetSeriesByID(id primitive.ObjectID) (*models.Series, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}
	return series, nil
}

// GetSeriesBySlug gets a series by slug
func (s *SeriesService) GetSeriesBySlug(slug string) (*models.Series, error) {
	series, err := s.seriesRepo.GetBySlug(slug)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}
	return series, nil
}

func (s *SeriesService) GetSeriesByCode(code string) (*models.Series, error) {
	series, err := s.seriesRepo.FindByCode(code)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}
	return series, nil
}

func (s *SeriesService) EnsureSeriesCodeByID(id primitive.ObjectID) (*models.Series, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}
	return series, nil
}

// GetSeriesWithSeasons gets series with all seasons and episodes
func (s *SeriesService) GetSeriesWithSeasons(slug string) (*models.SeriesWithSeasons, error) {
	series, err := s.GetSeriesBySlug(slug)
	if err != nil {
		return nil, err
	}

	return s.seriesRepo.GetSeriesWithSeasons(series.ID)
}

// ListSeries lists all series
func (s *SeriesService) ListSeries(limit, skip int, genre string) ([]models.Series, error) {
	seriesList, err := s.seriesRepo.List(limit, skip, genre)
	if err != nil {
		return nil, err
	}
	for i := range seriesList {
		if err := s.ensureSeriesCode(&seriesList[i]); err != nil {
			return nil, err
		}
	}
	return seriesList, nil
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
	if err := s.ensureSeriesCode(series); err != nil {
		return nil, err
	}

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
		current := series
		if err := s.ensureSeriesCode(&current); err != nil {
			log.Printf("Backfill: failed to generate code for series %s: %v", series.ID.Hex(), err)
			continue
		}
	}
	log.Printf("Backfill: series complete")
}

// DeleteSeries removes a series and every related asset:
//   - all seasons, all episodes, plus the series row
//   - per-episode HLS folders in B2 (each is independently safe-checked)
//   - the top-level series folder when it can be safely derived
//   - series poster/backdrop, season posters, episode thumbnails
//   - all clips linked to the series or to any of its episodes
//   - the publish/schedule jobs that reference those clips
//
// Per-asset failures are collected in result.B2 instead of aborting so
// one transient B2 hiccup cannot leave a series un-deleteable.
func (s *SeriesService) DeleteSeries(id primitive.ObjectID) (*SeriesDeleteResult, error) {
	series, err := s.seriesRepo.GetByID(id)
	if err != nil {
		return nil, err
	}

	result := &SeriesDeleteResult{
		SeriesID: id.Hex(),
		Title:    series.Title,
		B2:       NewB2DeleteSummary(),
	}

	s.cleanupSeriesStorage(series, result, nil)
	if len(result.B2.Errors) > 0 {
		result.Partial = true
	}

	if err := s.seriesRepo.Delete(id); err != nil {
		log.Printf("[SERIES DELETE] FAILED repo.Delete id=%s: %v", id.Hex(), err)
		return result, err
	}
	log.Printf("[SERIES DELETE] done — id=%s removed from DB (b2_files_deleted=%d skipped=%d errors=%d)",
		id.Hex(), result.B2.FilesDeleted, len(result.B2.Skipped), len(result.B2.Errors))
	return result, nil
}

func (s *SeriesService) cleanupSeriesStorage(series *models.Series, result *SeriesDeleteResult, track ProgressTracker) {
	if series == nil {
		return
	}

	episodes, err := s.seriesRepo.GetEpisodesBySeriesID(series.ID)
	if err != nil {
		result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("load series episodes: %v", err))
		log.Printf("[SERIES DELETE] FAILED to load episodes: %v", err)
		return
	}
	seasons, err := s.seriesRepo.GetSeasonsBySeriesID(series.ID)
	if err != nil {
		result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("load series seasons: %v", err))
		log.Printf("[SERIES DELETE] FAILED to load seasons: %v", err)
		return
	}
	result.SeasonsDeleted = len(seasons)
	result.EpisodesDeleted = len(episodes)

	if s.b2Cleanup != nil {
		// 1) Per-episode HLS folders. Each episode's video lives at its own
		// `videos/serials/<slug>/season-N/episode-M/` folder, so deleting per-
		// episode keeps the blast radius scoped to that episode even when one
		// season has dozens of episodes.
		seenEpisodePrefixes := make(map[string]struct{})
		for _, episode := range episodes {
			folder := s.b2Cleanup.DeriveVideoFolderPrefix(episode.VideoURL)
			if folder == "" {
				continue
			}
			parts := strings.Split(strings.Trim(folder, "/"), "/")
			// We want at minimum videos/{serials|series}/<slug>/season-N/episode-M/
			// The DeriveVideoFolderPrefix trims at the slug level (3 parts);
			// if we got that, walk further into the episode folder using the URL key.
			if len(parts) == 3 {
				if key := s.b2Cleanup.NormalizeKey(episode.VideoURL); key != "" {
					keyParts := strings.Split(key, "/")
					if len(keyParts) >= 5 {
						folder = strings.Join(keyParts[:5], "/") + "/"
					}
				}
			}
			if _, dup := seenEpisodePrefixes[folder]; dup {
				continue
			}
			seenEpisodePrefixes[folder] = struct{}{}
			s.b2Cleanup.SafeDeletePrefix(folder, fmt.Sprintf("episode-hls-%s", episode.ID.Hex()), result.B2)
		}

		// 2) Top-level series folder, only if we can prove it belongs to this
		// series via slug or ID (validateB2DeletePrefixForSeries).
		seriesPrefix := s.deriveSeriesDeletePrefix(series, episodes)
		if seriesPrefix != "" {
			if err := validateB2DeletePrefixForSeries(seriesPrefix, *series); err != nil {
				result.B2.Skipped = append(result.B2.Skipped,
					fmt.Sprintf("series-folder: %q failed identity check: %v", seriesPrefix, err))
				log.Printf("[SERIES DELETE] series folder identity check failed for prefix=%q: %v — skipping", seriesPrefix, err)
			} else {
				filesCount, err := s.countFilesByPrefix(seriesPrefix)
				log.Printf("[B2_DELETE] series_id=%s title=%q prefix=%s files_count=%d", series.ID.Hex(), series.Title, seriesPrefix, filesCount)
				if err != nil {
					result.B2.Errors = append(result.B2.Errors,
						fmt.Sprintf("series-folder: list %q: %v", seriesPrefix, err))
				} else if filesCount > seriesB2FileCap {
					result.B2.Skipped = append(result.B2.Skipped,
						fmt.Sprintf("series-folder: %q has %d files (>%d) — refusing for safety", seriesPrefix, filesCount, seriesB2FileCap))
					log.Printf("[SERIES DELETE] refusing prefix=%q with %d files (>%d safety cap)", seriesPrefix, filesCount, seriesB2FileCap)
					result.Partial = true
				} else {
					s.b2Cleanup.SafeDeletePrefix(seriesPrefix, "series-folder", result.B2)
				}
			}

			// Legacy backward-compat: pre-fix uploads landed under
			// videos/movies/serials/<series-folder>/. If a row in this series
			// still references that path, walk and remove it. Identity is
			// proven by reusing the same series-folder token.
			if seriesPrefix != "" {
				legacyPrefix := strings.Replace(seriesPrefix, "videos/serials/", "videos/movies/serials/", 1)
				if legacyPrefix != seriesPrefix {
					s.b2Cleanup.SafeDeletePrefix(legacyPrefix, "series-folder-legacy", result.B2)
				}
			}
		}

		// 3) Imagery and stray episode files.
		s.b2Cleanup.SafeDeleteKey(series.PosterURL, "series-poster", result.B2)
		s.b2Cleanup.SafeDeleteKey(series.BackdropURL, "series-backdrop", result.B2)
		for _, season := range seasons {
			s.b2Cleanup.SafeDeleteKey(season.PosterURL, fmt.Sprintf("season-poster-%d", season.SeasonNumber), result.B2)
		}
		normalizedSeries := normalizeB2DeletePrefix(seriesPrefix)
		for _, episode := range episodes {
			s.b2Cleanup.SafeDeleteKey(episode.ThumbnailURL, fmt.Sprintf("episode-thumb-%s", episode.ID.Hex()), result.B2)
			if videoKey := s.b2Cleanup.NormalizeKey(episode.VideoURL); videoKey != "" {
				if normalizedSeries == "" || !strings.HasPrefix(videoKey, normalizedSeries) {
					s.b2Cleanup.SafeDeleteKey(videoKey, fmt.Sprintf("episode-video-%s", episode.ID.Hex()), result.B2)
				}
			}
		}
	}

	// 4) Clips linked to this series — we collect from BOTH series_id and
	// per-episode lookups so legacy clips (only episode_id set) are caught.
	if s.clipRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		clipsByID := make(map[primitive.ObjectID]models.Clip)

		seriesClips, err := s.clipRepo.FindBySeriesID(ctx, series.ID)
		if err != nil {
			result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("load series clips: %v", err))
			log.Printf("[SERIES DELETE] FAILED to load series clips: %v", err)
		}
		for _, clip := range seriesClips {
			clipsByID[clip.ID] = clip
		}

		for _, episode := range episodes {
			episodeClips, err := s.clipRepo.FindByEpisodeID(ctx, episode.ID)
			if err != nil {
				result.B2.Errors = append(result.B2.Errors,
					fmt.Sprintf("load episode %s clips: %v", episode.ID.Hex(), err))
				continue
			}
			for _, clip := range episodeClips {
				clipsByID[clip.ID] = clip
			}
		}

		clipIDs := make([]primitive.ObjectID, 0, len(clipsByID))
		for _, clip := range clipsByID {
			clipIDs = append(clipIDs, clip.ID)
			if s.b2Cleanup == nil {
				continue
			}
			key := s.b2Cleanup.NormalizeKey(clip.URL)
			if key == "" {
				key = s.b2Cleanup.NormalizeKey(clip.Path)
			}
			if key == "" {
				result.B2.Skipped = append(result.B2.Skipped,
					fmt.Sprintf("clip %s: no key derivable", clip.ID.Hex()))
				continue
			}
			s.b2Cleanup.SafeDeleteKey(key, fmt.Sprintf("clip-%s", clip.ID.Hex()), result.B2)
		}

		// Aggregate clips folder under videos/clips/<series-folder>/ —
		// a defensive sweep so any orphan files left behind by partial
		// runs are also removed. Identity is gated on series slug/id.
		if s.b2Cleanup != nil {
			if seriesFolder := seriesFolderTokenFromPrefix(s.deriveSeriesDeletePrefix(series, episodes)); seriesFolder != "" {
				clipsPrefix := "videos/clips/" + seriesFolder + "/"
				if validateClipsDeletePrefixForSeries(clipsPrefix, *series) == nil {
					filesCount, listErr := s.countFilesByPrefix(clipsPrefix)
					log.Printf("[B2_DELETE] series_id=%s clips_prefix=%s files_count=%d", series.ID.Hex(), clipsPrefix, filesCount)
					if listErr != nil {
						result.B2.Errors = append(result.B2.Errors,
							fmt.Sprintf("series-clips-folder: list %q: %v", clipsPrefix, listErr))
					} else if filesCount > clipsB2FileCap {
						result.B2.Skipped = append(result.B2.Skipped,
							fmt.Sprintf("series-clips-folder: %q has %d files (>%d) — refusing for safety", clipsPrefix, filesCount, clipsB2FileCap))
						result.Partial = true
					} else if filesCount > 0 {
						s.b2Cleanup.SafeDeletePrefix(clipsPrefix, "series-clips-folder", result.B2)
					}
				}
			}
		}

		// Delete clip DB rows. Use the union (series_id + episode_id list) so
		// no clip linked to this content survives.
		if err := s.clipRepo.DeleteBySeriesAndEpisodeIDs(ctx, series.ID, episodeIDsFromEpisodes(episodes)); err != nil {
			result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("delete series clip rows: %v", err))
			log.Printf("[SERIES DELETE] FAILED to delete clip rows: %v", err)
		} else {
			result.ClipsDeleted = len(clipIDs)
			log.Printf("[SERIES DELETE] removed %d clip row(s) from DB", len(clipIDs))
		}

		// 5) Instagram schedules + publish jobs.
		if len(clipIDs) > 0 {
			if s.igScheduleRepo != nil {
				deleted, err := s.igScheduleRepo.DeleteByClipIDs(ctx, clipIDs)
				if err != nil {
					result.B2.Errors = append(result.B2.Errors,
						fmt.Sprintf("delete instagram schedules: %v", err))
					log.Printf("[SERIES DELETE] FAILED instagram_schedules cleanup: %v", err)
				} else {
					result.IGSchedulesDeleted = deleted
				}
			}
			if s.publishJobRepo != nil {
				deleted, err := s.publishJobRepo.DeleteByClipIDs(ctx, clipIDs)
				if err != nil {
					result.B2.Errors = append(result.B2.Errors,
						fmt.Sprintf("delete publish jobs: %v", err))
					log.Printf("[SERIES DELETE] FAILED publish_jobs cleanup: %v", err)
				} else {
					result.PublishJobsDeleted = deleted
				}
			}
		}
	}

	// 6) Ingestion-jobs cascade. series_id is primary; episode_id list
	// catches legacy episode jobs without series_id linkage; (source,
	// source_id) catches any series-level metadata fetch job that ran
	// before linkage was stamped.
	if s.jobRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		var totalJobs int64
		if deleted, err := s.jobRepo.DeleteBySeriesID(ctx, series.ID); err != nil {
			result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("delete ingestion jobs by series_id: %v", err))
			log.Printf("[SERIES DELETE] FAILED ingestion_jobs by series_id: %v", err)
		} else {
			totalJobs += deleted
		}
		if epIDs := episodeIDsFromEpisodes(episodes); len(epIDs) > 0 {
			if deleted, err := s.jobRepo.DeleteByEpisodeIDs(ctx, epIDs); err != nil {
				result.B2.Errors = append(result.B2.Errors, fmt.Sprintf("delete ingestion jobs by episode_id: %v", err))
				log.Printf("[SERIES DELETE] FAILED ingestion_jobs by episode_id: %v", err)
			} else {
				totalJobs += deleted
			}
		}
		result.IngestionJobsDeleted = totalJobs
		log.Printf("[SERIES DELETE] removed %d ingestion_job row(s)", totalJobs)
	}
}

// seriesFolderTokenFromPrefix returns the series-folder segment of a
// videos/serials/<folder>/ prefix.
func seriesFolderTokenFromPrefix(prefix string) string {
	prefix = strings.Trim(prefix, "/")
	if prefix == "" {
		return ""
	}
	parts := strings.Split(prefix, "/")
	if len(parts) != 3 || parts[0] != "videos" || parts[1] != "serials" {
		return ""
	}
	return parts[2]
}

// validateClipsDeletePrefixForSeries is the series-side analogue of
// validateClipsDeletePrefix.
func validateClipsDeletePrefixForSeries(prefix string, series models.Series) error {
	prefix = normalizeB2DeletePrefix(prefix)
	if !strings.HasPrefix(prefix, "videos/clips/") {
		return fmt.Errorf("unsafe delete prefix")
	}
	parts := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(parts) != 3 {
		return fmt.Errorf("unsafe delete prefix")
	}
	folder := strings.ToLower(parts[2])
	tokens := []string{
		strings.ToLower(strings.TrimSpace(series.Slug)),
		strings.ToLower(series.ID.Hex()),
	}
	for _, token := range tokens {
		if token != "" && strings.Contains(folder, token) {
			return nil
		}
	}
	return fmt.Errorf("unsafe delete prefix")
}

func episodeIDsFromEpisodes(episodes []models.Episode) []primitive.ObjectID {
	ids := make([]primitive.ObjectID, 0, len(episodes))
	for _, e := range episodes {
		ids = append(ids, e.ID)
	}
	return ids
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
	seriesList, err := s.seriesRepo.ListAdmin(limit, skip)
	if err != nil {
		return nil, err
	}
	for i := range seriesList {
		if err := s.ensureSeriesCode(&seriesList[i]); err != nil {
			return nil, err
		}
	}
	return seriesList, nil
}

// SetSeriesApprovalStatus approves or rejects a series.
func (s *SeriesService) SetSeriesApprovalStatus(id, status, byUserID string) error {
	if status == "approved" {
		oid, err := primitive.ObjectIDFromHex(id)
		if err != nil {
			return err
		}
		series, err := s.seriesRepo.GetByID(oid)
		if err != nil {
			return err
		}
		if err := s.ensureSeriesCode(series); err != nil {
			return err
		}
	}
	return s.seriesRepo.SetApprovalStatus(id, status, byUserID)
}

func (s *SeriesService) ensureSeriesCode(series *models.Series) error {
	if series == nil {
		return nil
	}
	if strings.TrimSpace(series.Code) != "" {
		return nil
	}

	code, err := getNextContentCode(s.movieRepo, s.seriesRepo)
	if err != nil {
		return fmt.Errorf("generate series code: %w", err)
	}

	if series.ID.IsZero() {
		series.Code = code
		log.Printf("[CODE] assigned series code=%s", code)
		return nil
	}

	if err := s.seriesRepo.SetSeriesCode(series.ID, code); err != nil {
		return fmt.Errorf("set series code: %w", err)
	}
	series.Code = code
	log.Printf("[CODE] assigned series code=%s", code)
	return nil
}

func (s *SeriesService) BuildSeriesWebsiteURL(slug string) string {
	baseURL := strings.TrimSpace(os.Getenv("BASE_SITE_URL"))
	if baseURL == "" {
		baseURL = "https://filmorauz.net"
	}
	return strings.TrimRight(baseURL, "/") + "/series/" + strings.TrimLeft(slug, "/")
}
