package services

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const movieCodeSequenceKey = "movie_code"

// Code limits for sequential numbering
const (
	codeLimit4Digits = 9999   // 0001-9999
	codeLimit5Digits = 99999  // 10000-99999
	codeLimit6Digits = 999999 // 100000-999999
	codeMaxLimit     = 999999 // Maximum allowed code
)

// Slug validation regex: lowercase letters, numbers, hyphens only
var slugRegex = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

func isValidSlug(slug string) bool {
	return slug != "" && slugRegex.MatchString(slug)
}

// normalizeMovieGenres normalizes genre array: trim, lowercase, dedupe
func normalizeMovieGenres(genres []string) []string {
	if len(genres) == 0 {
		return []string{}
	}
	seen := make(map[string]bool)
	var result []string
	for _, g := range genres {
		normalized := normalizeMovieGenreKey(g)
		if normalized == "" {
			continue
		}
		if !seen[normalized] {
			seen[normalized] = true
			result = append(result, normalized)
		}
	}
	log.Printf("[MOVIE] normalizeMovieGenres: in=%v out=%v", genres, result)
	return result
}

func normalizeMovieGenreKey(raw string) string {
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

// normalizeTitle normalizes a movie title: lowercase, trim, collapse spaces
func normalizeTitle(title string) string {
	trimmed := strings.TrimSpace(title)
	// Collapse multiple spaces into single space
	re := regexp.MustCompile(`\s+`)
	normalized := re.ReplaceAllString(trimmed, " ")
	return strings.ToLower(normalized)
}

// NormalizeTitleForDuplicate normalizes title for duplicate checking
func NormalizeTitleForDuplicate(title string, year int) string {
	return fmt.Sprintf("%s %d", normalizeTitle(title), year)
}

type DuplicateMovieError struct {
	Message string
	Rule    string
	MovieID primitive.ObjectID
}

func (e *DuplicateMovieError) Error() string {
	return e.Message
}

func IsDuplicateMovieError(err error) bool {
	var duplicateErr *DuplicateMovieError
	return errors.As(err, &duplicateErr)
}

type MovieService struct {
	repo            *repositories.MovieRepository
	seriesRepo      *repositories.SeriesRepository
	counterRepo     *repositories.CounterRepository
	notificationSvc *NotificationService
	viewEventRepo   *repositories.MovieViewEventRepository
	clipRepo        *repositories.ClipRepository // optional — enables clip cleanup on delete
	b2Cleanup       *B2CleanupService            // optional — nil means skip B2 cleanup
}

func NewMovieService(repo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository, counterRepo *repositories.CounterRepository, notificationSvc *NotificationService, viewEventRepo *repositories.MovieViewEventRepository) *MovieService {
	return &MovieService{repo: repo, seriesRepo: seriesRepo, counterRepo: counterRepo, notificationSvc: notificationSvc, viewEventRepo: viewEventRepo}
}

// SetStorageDependencies wires optional cleanup helpers so DeleteMovie can
// purge B2 assets and clip DB rows. Safe to call with nils (cleanup skipped).
func (s *MovieService) SetStorageDependencies(clipRepo *repositories.ClipRepository, b2 *B2CleanupService) {
	s.clipRepo = clipRepo
	s.b2Cleanup = b2
}

// GetTrendingMovies returns the most popular movies based on recent view events
func (s *MovieService) GetTrendingMovies(period string, limit int) ([]models.TrendingMovie, error) {
	// Validate and set defaults
	if limit < 1 || limit > 50 {
		limit = 12
	}

	// Calculate the time window
	var since time.Time
	switch period {
	case "7d":
		since = time.Now().AddDate(0, 0, -7)
	default: // "24h" or default
		since = time.Now().AddDate(0, 0, -1)
	}

	// Get trending view counts
	viewCounts, err := s.viewEventRepo.GetTrendingMovieViews(nil, since, limit*2) // Get more for filtering
	if err != nil {
		return nil, fmt.Errorf("failed to get trending views: %w", err)
	}

	if len(viewCounts) == 0 {
		return []models.TrendingMovie{}, nil
	}

	// Build movie IDs to fetch
	movieIDs := make([]string, len(viewCounts))
	for i, vc := range viewCounts {
		movieIDs[i] = vc.MovieID.Hex()
	}

	// Fetch movies and filter to only published ones with playable video
	movies, err := s.repo.FindByIDs(movieIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch movies: %w", err)
	}

	// Create a map for quick lookup
	movieMap := make(map[string]models.Movie)
	for _, m := range movies {
		movieMap[m.ID.Hex()] = m
	}

	// Build results
	var results []models.TrendingMovie
	for _, vc := range viewCounts {
		movie, ok := movieMap[vc.MovieID.Hex()]
		if !ok {
			continue
		}

		// Skip movies without playable video
		if movie.VideoURL == "" && movie.EmbedURL == "" {
			continue
		}

		// Skip unpublished movies — new pending content should not appear in trending.
		// Legacy movies (no approval_status field) are treated as approved.
		if !movie.IsPublished && movie.ApprovalStatus != "" {
			continue
		}

		results = append(results, models.TrendingMovie{
			Movie:         movie,
			ViewsInPeriod: vc.ViewCount,
		})

		if len(results) >= limit {
			break
		}
	}

	return results, nil
}

// GetRecommendations returns similar movies based on content similarity
func (s *MovieService) GetRecommendations(movieID string, limit int) ([]models.RecommendationMovie, error) {
	if limit < 1 || limit > 50 {
		limit = 12
	}

	// Get the source movie
	objID, err := primitive.ObjectIDFromHex(movieID)
	if err != nil {
		return nil, fmt.Errorf("invalid movie id")
	}

	sourceMovie, err := s.repo.FindByID(objID)
	if err != nil {
		return nil, fmt.Errorf("movie not found")
	}

	// Get candidate movies (same genre first for efficiency)
	candidates, err := s.repo.FindByGenre(sourceMovie.Genre, 100)
	if err != nil {
		return nil, fmt.Errorf("failed to find candidate movies: %w", err)
	}

	// Get trending for popularity boost
	trending, err := s.GetTrendingMovies("24h", 20)
	if err != nil {
		trending = nil // Don't fail if trending fails
	}
	trendingMap := make(map[string]int64)
	for _, t := range trending {
		trendingMap[t.Movie.ID.Hex()] = t.ViewsInPeriod
	}

	// Score each candidate
	type scoredMovie struct {
		movie models.Movie
		score int
	}
	var scoredCandidates []scoredMovie

	for _, candidate := range candidates {
		// Skip the source movie
		if candidate.ID == sourceMovie.ID {
			continue
		}

		// Skip movies without playable video
		if candidate.VideoURL == "" && candidate.EmbedURL == "" {
			continue
		}

		score := calculateSimilarityScore(*sourceMovie, candidate, trendingMap)
		scoredCandidates = append(scoredCandidates, scoredMovie{
			movie: candidate,
			score: score,
		})
	}

	// Sort by score descending
	for i := 0; i < len(scoredCandidates)-1; i++ {
		for j := i + 1; j < len(scoredCandidates); j++ {
			if scoredCandidates[j].score > scoredCandidates[i].score {
				scoredCandidates[i], scoredCandidates[j] = scoredCandidates[j], scoredCandidates[i]
			}
		}
	}

	// Take top results
	var results []models.RecommendationMovie
	for i := 0; i < len(scoredCandidates) && i < limit; i++ {
		m := scoredCandidates[i].movie
		results = append(results, models.RecommendationMovie{
			ID:        m.ID.Hex(),
			Title:     m.Title,
			Slug:      m.Slug,
			PosterURL: m.PosterURL,
			Year:      m.Year,
			Genres:    m.Genre,
			Score:     scoredCandidates[i].score,
		})
	}

	// Fallback: if not enough results, get trending movies
	if len(results) < limit {
		fallbackLimit := limit - len(results)
		trendingMovies, err := s.GetTrendingMovies("24h", fallbackLimit)
		if err == nil && len(trendingMovies) > 0 {
			for _, t := range trendingMovies {
				// Skip if already in results
				alreadyExists := false
				for _, r := range results {
					if r.ID == t.Movie.ID.Hex() {
						alreadyExists = true
						break
					}
				}
				if alreadyExists {
					continue
				}
				results = append(results, models.RecommendationMovie{
					ID:        t.Movie.ID.Hex(),
					Title:     t.Movie.Title,
					Slug:      t.Movie.Slug,
					PosterURL: t.Movie.PosterURL,
					Year:      t.Movie.Year,
					Genres:    t.Movie.Genre,
					Score:     0,
				})
				if len(results) >= limit {
					break
				}
			}
		}
	}

	return results, nil
}

// calculateSimilarityScore calculates a similarity score between two movies
func calculateSimilarityScore(source, candidate models.Movie, trending map[string]int64) int {
	score := 0

	// Primary genre match: +5
	if len(source.Genre) > 0 && len(candidate.Genre) > 0 {
		if source.Genre[0] == candidate.Genre[0] {
			score += 5
		}
	}

	// Shared genres: +3 per genre (max 6)
	sharedGenres := 0
	genreSet := make(map[string]bool)
	for _, g := range source.Genre {
		genreSet[g] = true
	}
	for _, g := range candidate.Genre {
		if genreSet[g] {
			sharedGenres++
		}
	}
	sharedGenresScore := sharedGenres * 3
	if sharedGenresScore > 6 {
		sharedGenresScore = 6
	}
	score += sharedGenresScore

	// Same country: +2
	if source.Country != "" && source.Country == candidate.Country {
		score += 2
	}

	// Year difference <= 3: +2
	yearDiff := abs(source.Year - candidate.Year)
	if yearDiff <= 3 {
		score += 2
	} else if yearDiff <= 7 {
		score += 1
	}

	// Popularity boost: +1-3
	if views, ok := trending[candidate.ID.Hex()]; ok {
		if views > 100 {
			score += 3
		} else if views > 50 {
			score += 2
		} else if views > 10 {
			score += 1
		}
	}

	return score
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (s *MovieService) ListMovies(genre string, page, limit int) ([]models.Movie, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	return s.repo.List(genre, page, limit)
}

func (s *MovieService) GetMovieBySlug(slug string) (*models.Movie, error) {
	return s.repo.FindBySlug(slug)
}

// GetMovieByID returns a movie by its ObjectID
func (s *MovieService) GetMovieByID(id string) (*models.Movie, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid movie id")
	}
	return s.repo.FindByID(objID)
}

// GetMovieByCode returns a movie by its alphanumeric code
func (s *MovieService) GetMovieByCode(code string) (*models.Movie, error) {
	movie, err := s.repo.FindByCode(code)
	if err != nil {
		return nil, err
	}
	return movie, nil
}

func (s *MovieService) SearchMovies(query string) ([]models.Movie, error) {
	if strings.TrimSpace(query) == "" {
		return []models.Movie{}, nil
	}
	return s.repo.Search(query)
}

func (s *MovieService) CreateMovie(input *models.MovieInput) (*models.Movie, error) {
	normalizedTitle := normalizeTitle(input.Title)
	desiredSlug := strings.TrimSpace(input.Slug)
	if desiredSlug == "" {
		desiredSlug = generateSlug(input.Title, input.Year)
	}

	existing, rule, err := s.findDuplicateMovie(input, desiredSlug, nil)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		log.Printf("[MOVIE DUPLICATE BLOCKED] create rule=%s existing_id=%s title=%q", rule, existing.ID.Hex(), existing.Title)
		return nil, &DuplicateMovieError{
			Message: "movie already exists",
			Rule:    rule,
			MovieID: existing.ID,
		}
	}

	slug, err := s.generateUniqueSlug(input.Title, input.Year)
	if err != nil {
		return nil, err
	}

	code, err := s.generateUniqueCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate movie code: %w", err)
	}

	websiteBaseURL := os.Getenv("BASE_SITE_URL")
	if websiteBaseURL == "" {
		websiteBaseURL = "https://filmorauz.net"
	}

	now := time.Now()
	movie := &models.Movie{
		ID:              primitive.NewObjectID(),
		Code:            code,
		Slug:            slug,
		WebsiteURL:      calculateWebsiteURL(slug, websiteBaseURL),
		Source:          sanitizeMovieSource(input.Source),
		Title:           input.Title,
		NormalizedTitle: normalizedTitle,
		Description:     input.Description,
		PosterURL:       input.PosterURL,
		BackdropURL:     input.BackdropURL,
		Year:            input.Year,
		Genre:           normalizeMovieGenres(input.Genre),
		Country:         input.Country,
		VideoURL:        input.VideoURL,
		EmbedURL:        input.EmbedURL,
		SourceType:      input.SourceType,
		Duration:        input.Duration,
		Quality:         input.Quality,
		RatingAvg:       0,
		RatingCount:     0,
		CreatedAt:       now,
		UpdatedAt:       now,
		TitleUz:         input.TitleUz,
		DescriptionUz:   input.DescriptionUz,
		GenresUz:        input.GenresUz,
		CountriesUz:     input.CountriesUz,
		OriginalTitle:   input.OriginalTitle,
		TMDBID:          input.TMDBID,
		MetadataSource:  input.MetadataSource,
		ApprovalStatus:  "pending",
		IsPublished:     false,
	}

	if err := s.repo.Create(movie); err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			log.Printf("[MOVIE DUPLICATE BLOCKED] create race slug=%s title=%q", slug, input.Title)
			return nil, &DuplicateMovieError{Message: "movie already exists", Rule: "slug"}
		}
		return nil, fmt.Errorf("failed to create movie: %w", err)
	}

	if s.notificationSvc != nil {
		s.notificationSvc.SendMovieNotificationAsync(movie)
	}

	return movie, nil
}

func (s *MovieService) UpdateMovie(id string, input *models.MovieInput) (*models.Movie, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, fmt.Errorf("invalid movie id")
	}

	existing, err := s.repo.FindByID(objID)
	if err != nil {
		return nil, fmt.Errorf("movie not found")
	}

	// Validate and update slug if provided
	if input.Slug != "" {
		// Validate slug format: lowercase, alphanumeric, hyphen only
		if !isValidSlug(input.Slug) {
			return nil, fmt.Errorf("invalid slug: use only lowercase letters, numbers, and hyphens")
		}
		// Check uniqueness (exclude current movie)
		existingSlug, err := s.repo.FindBySlug(input.Slug)
		if err == nil && existingSlug != nil && existingSlug.ID != objID {
			return nil, fmt.Errorf("slug already in use by another movie")
		}
		existing.Slug = input.Slug
		// Recalculate WebsiteURL when slug changes
		websiteBaseURL := os.Getenv("BASE_SITE_URL")
		if websiteBaseURL == "" {
			websiteBaseURL = "https://filmorauz.net"
		}
		existing.WebsiteURL = calculateWebsiteURL(input.Slug, websiteBaseURL)
		log.Printf("[UpdateMovie] slug changed from=%q to=%q, recalculating website_url=%q", existing.Slug, input.Slug, existing.WebsiteURL)
	}

	// Update fields; keep the original code.
	// For media URLs, empty input means "keep existing" so admins can do
	// partial edits (e.g. change title) without re-uploading poster/backdrop/video.
	existing.Title = input.Title
	existing.NormalizedTitle = normalizeTitle(input.Title)
	existing.Description = input.Description
	if input.PosterURL != "" {
		existing.PosterURL = input.PosterURL
	}
	if input.BackdropURL != "" {
		existing.BackdropURL = input.BackdropURL
	}
	existing.Year = input.Year
	// Normalize genres: trim, lowercase, dedupe before saving
	existing.Genre = normalizeMovieGenres(input.Genre)
	existing.Country = input.Country
	if input.VideoURL != "" {
		existing.VideoURL = input.VideoURL
	}
	if input.EmbedURL != "" {
		existing.EmbedURL = input.EmbedURL
	}
	if input.SourceType != "" {
		existing.SourceType = input.SourceType
	}
	if input.Source != nil {
		existing.Source = sanitizeMovieSource(input.Source)
	}
	existing.Duration = input.Duration
	existing.Quality = input.Quality
	existing.IsPremium = input.IsPremium
	existing.UpdatedAt = time.Now()

	// LOCALIZATION: Update Uzbek display fields
	existing.TitleUz = input.TitleUz
	existing.DescriptionUz = input.DescriptionUz
	existing.GenresUz = input.GenresUz
	existing.CountriesUz = input.CountriesUz
	existing.OriginalTitle = input.OriginalTitle
	existing.TMDBID = input.TMDBID
	existing.MetadataSource = input.MetadataSource

	if err := s.repo.Update(objID, existing); err != nil {
		return nil, fmt.Errorf("failed to update movie: %w", err)
	}
	return existing, nil
}

// DeleteMovie removes the movie's B2 assets (HLS folder, poster, backdrop,
// clips), its clip DB rows, and finally the movie document.
func (s *MovieService) DeleteMovie(id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}

	movie, findErr := s.repo.FindByID(objID)
	if findErr != nil || movie == nil {
		return fmt.Errorf("movie not found")
	}

	log.Printf("[MOVIE DELETE] start — id=%s title=%q code=%s", id, movie.Title, movie.Code)

	if err := s.cleanupMovieStorage(objID, movie); err != nil {
		log.Printf("[MOVIE DELETE] aborting DB delete id=%s: %v", id, err)
		return err
	}

	if delErr := s.repo.Delete(id); delErr != nil {
		log.Printf("[MOVIE DELETE] FAILED repo.Delete id=%s: %v", id, delErr)
		return delErr
	}
	log.Printf("[MOVIE DELETE] done — id=%s removed from DB", id)
	return nil
}

// cleanupMovieStorage deletes every B2 asset associated with a movie and
// wipes its clip rows. Any B2 or clip cleanup failure aborts the movie delete.
func (s *MovieService) cleanupMovieStorage(objID primitive.ObjectID, movie *models.Movie) error {
	if s.b2Cleanup == nil {
		log.Printf("[MOVIE DELETE] B2 cleanup disabled (no service configured) — skipping storage delete")
	} else {
		prefix := s.deriveMovieDeletePrefix(movie)
		if prefix != "" {
			if err := validateB2DeletePrefix(prefix, *movie); err != nil {
				return err
			}
			filesCount, err := s.countFilesByPrefix(prefix)
			log.Printf("[B2_DELETE] movie_id=%s", movie.ID.Hex())
			log.Printf("[B2_DELETE] title=%q", movie.Title)
			log.Printf("[B2_DELETE] prefix=%s", prefix)
			log.Printf("[B2_DELETE] files_count=%d", filesCount)
			if err != nil {
				return fmt.Errorf("list b2 prefix %q: %w", prefix, err)
			}
			if filesCount > 500 || isUnsafeB2DeletePrefix(prefix) {
				return fmt.Errorf("unsafe delete prefix")
			}
			if _, err := s.b2Cleanup.DeleteByPrefix(prefix); err != nil {
				return fmt.Errorf("delete b2 prefix %q: %w", prefix, err)
			}
		} else {
			log.Printf("[MOVIE DELETE] no safe videos/movies prefix derivable from master=%q video=%q — skipping folder purge",
				movie.MasterPlaylistURL, movie.VideoURL)
		}

		// 2) Individual video URL if it points outside the folder (e.g. a direct MP4).
		prefix = normalizeB2DeletePrefix(prefix)
		if videoKey := s.b2Cleanup.ExtractKeyFromURL(movie.VideoURL); videoKey != "" && !strings.HasPrefix(videoKey, prefix) {
			if _, err := s.b2Cleanup.DeleteByKey(videoKey); err != nil {
				return fmt.Errorf("delete movie video key %q: %w", videoKey, err)
			}
		}

		// 3) Poster + backdrop images.
		for label, u := range map[string]string{
			"poster":   movie.PosterURL,
			"backdrop": movie.BackdropURL,
		} {
			key := s.b2Cleanup.ExtractKeyFromURL(u)
			if key == "" {
				log.Printf("[MOVIE DELETE] %s: no key derivable from url=%q", label, u)
				continue
			}
			if _, err := s.b2Cleanup.DeleteByKey(key); err != nil {
				return fmt.Errorf("delete movie %s key %q: %w", label, key, err)
			}
		}
	}

	// 4) Clips — delete B2 files then DB rows.
	if s.clipRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		clips, cErr := s.clipRepo.FindByMovieID(ctx, objID)
		if cErr != nil {
			return fmt.Errorf("load movie clips: %w", cErr)
		} else {
			log.Printf("[MOVIE DELETE] %d clip(s) linked to movie — cleaning up", len(clips))
			if s.b2Cleanup != nil {
				for _, clip := range clips {
					key := s.b2Cleanup.ExtractKeyFromURL(clip.URL)
					if key == "" {
						key = strings.TrimPrefix(clip.Path, "/")
					}
					if key == "" {
						log.Printf("[MOVIE DELETE] clip id=%s: no key derivable, skipping B2 delete", clip.ID.Hex())
						continue
					}
					if _, err := s.b2Cleanup.DeleteByKey(key); err != nil {
						return fmt.Errorf("delete clip key %q: %w", key, err)
					}
				}
			}
			if err := s.clipRepo.DeleteByMovieID(ctx, objID); err != nil {
				return fmt.Errorf("delete clip rows: %w", err)
			}
			log.Printf("[MOVIE DELETE] removed %d clip row(s) from DB", len(clips))
		}
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func (s *MovieService) countFilesByPrefix(prefix string) (int, error) {
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

func (s *MovieService) deriveMovieDeletePrefix(movie *models.Movie) string {
	if movie == nil || s.b2Cleanup == nil {
		return ""
	}
	for _, raw := range []string{movie.MasterPlaylistURL, movie.VideoURL} {
		key := strings.TrimSpace(s.b2Cleanup.ExtractKeyFromURL(raw))
		if key == "" {
			continue
		}
		if prefix := movieDeletePrefixFromKey(key); prefix != "" {
			return prefix
		}
	}
	return ""
}

func movieDeletePrefixFromKey(key string) string {
	key = strings.Trim(strings.TrimSpace(key), "/")
	if key == "" {
		return ""
	}
	parts := strings.Split(key, "/")
	if len(parts) < 4 {
		return ""
	}
	if parts[0] != "videos" || parts[1] != "movies" || strings.TrimSpace(parts[2]) == "" {
		return ""
	}
	return strings.Join(parts[:3], "/") + "/"
}

func validateB2DeletePrefix(prefix string, movie models.Movie) error {
	prefix = normalizeB2DeletePrefix(prefix)
	if isUnsafeB2DeletePrefix(prefix) {
		return fmt.Errorf("unsafe delete prefix")
	}
	if !strings.HasPrefix(prefix, "videos/movies/") {
		return fmt.Errorf("unsafe delete prefix")
	}
	parts := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(parts) != 3 {
		return fmt.Errorf("unsafe delete prefix")
	}
	folder := parts[2]
	if folder == "" {
		return fmt.Errorf("unsafe delete prefix")
	}

	var tokens []string
	tokens = append(tokens, strings.ToLower(strings.TrimSpace(movie.Slug)))
	tokens = append(tokens, strings.ToLower(movie.ID.Hex()))
	if movie.Source != nil {
		tokens = append(tokens, strings.ToLower(strings.TrimSpace(movie.Source.SourceID)))
	}
	folderLower := strings.ToLower(folder)
	for _, token := range tokens {
		if token != "" && strings.Contains(folderLower, token) {
			return nil
		}
	}
	return fmt.Errorf("unsafe delete prefix")
}

func validateB2DeletePrefixForSeries(prefix string, series models.Series) error {
	prefix = normalizeB2DeletePrefix(prefix)
	if prefix == "" || strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("unsafe delete prefix")
	}
	if !strings.HasPrefix(prefix, "videos/serials/") {
		return fmt.Errorf("unsafe delete prefix")
	}
	parts := strings.Split(strings.Trim(prefix, "/"), "/")
	if len(parts) != 3 {
		return fmt.Errorf("unsafe delete prefix")
	}
	folder := strings.ToLower(parts[2])
	allowed := []string{
		strings.ToLower(strings.TrimSpace(series.Slug)),
		strings.ToLower(series.ID.Hex()),
	}
	for _, token := range allowed {
		if token != "" && strings.Contains(folder, token) {
			return nil
		}
	}
	return fmt.Errorf("unsafe delete prefix")
}

func firstNonEmptyTrimmed(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}

func dedupeKeys(keys []string) []string {
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out
}

// MarkTelegramPostedOnApproval flips telegram_posted_on_approval=true so the
// next approve click does not re-post to Telegram.
func (s *MovieService) MarkTelegramPostedOnApproval(id string) error {
	return s.repo.MarkTelegramPostedOnApproval(id)
}

// BackfillMovieCodes assigns codes to existing movies that don't have one.
// This is safe to call on startup — it's idempotent.
func (s *MovieService) BackfillMovieCodes() {
	movies, err := s.repo.FindMoviesWithoutCode()
	if err != nil {
		log.Printf("Backfill: failed to find movies without code: %v", err)
		return
	}
	if len(movies) == 0 {
		return
	}

	log.Printf("Backfill: assigning codes to %d existing movies...", len(movies))
	for _, movie := range movies {
		code, err := s.generateUniqueCode()
		if err != nil {
			log.Printf("Backfill: failed to generate code for movie %s: %v", movie.ID.Hex(), err)
			continue
		}

		// Get website base URL
		websiteBaseURL := os.Getenv("BASE_SITE_URL")
		if websiteBaseURL == "" {
			websiteBaseURL = "https://filmorauz.net"
		}

		// Generate slug if missing
		slug := movie.Slug
		if slug == "" {
			slug = generateSlug(movie.Title, movie.Year)
		}

		websiteURL := calculateWebsiteURL(slug, websiteBaseURL)

		if err := s.repo.SetMovieCodeAndURL(movie.ID, code, slug, websiteURL); err != nil {
			log.Printf("Backfill: failed to set code %s for movie %s: %v", code, movie.ID.Hex(), err)
			continue
		}
		log.Printf("Backfill: assigned code %s to movie '%s'", code, movie.Title)
	}
	log.Printf("Backfill: complete")
}

// generateUniqueCode generates a unique sequential numeric code
// Finds the highest existing numeric code from movies collection and returns code+1
// Uses: 0001-9999 (4 digits), 10000-99999 (5 digits), 100000-999999 (6 digits)
func (s *MovieService) generateUniqueCode() (string, error) {
	code, err := getNextContentCode(s.repo, s.seriesRepo)
	if err != nil {
		return "", fmt.Errorf("failed to generate next content code: %w", err)
	}

	log.Printf("Generated sequential content code for movie: %s", code)
	return code, nil
}

// formatMovieCode formats a sequence number as a zero-padded numeric code
// Rules:
//   - seq <= 9999    → 4-digit zero-padded (e.g., 1 -> "0001")
//   - seq <= 99999   → 5-digit zero-padded (e.g., 10000 -> "10000")
//   - seq <= 999999  → 6-digit zero-padded (e.g., 100000 -> "100000")
func formatMovieCode(seq int64) string {
	return formatContentCode(seq)
}

// generateUniqueSlug creates a URL-safe slug from title and year
func (s *MovieService) generateUniqueSlug(title string, year int) (string, error) {
	base := generateSlug(title, year)
	slug := base
	counter := 1

	for {
		exists, err := s.repo.SlugExists(slug)
		if err != nil {
			return "", err
		}
		if !exists {
			return slug, nil
		}
		slug = fmt.Sprintf("%s-%d", base, counter)
		counter++
	}
}

// generateSlug converts a string to a URL-safe slug
func generateSlug(title string, year int) string {
	slug := strings.ToLower(title)
	// Replace non-alphanumeric chars with dash
	re := regexp.MustCompile(`[^a-z0-9]+`)
	slug = re.ReplaceAllString(slug, "-")
	// Trim leading/trailing dashes
	slug = strings.Trim(slug, "-")

	// Add year if provided
	if year > 0 {
		slug = fmt.Sprintf("%s-%d", slug, year)
	}

	return slug
}

// calculateWebsiteURL generates the website URL for a movie based on its slug
func calculateWebsiteURL(slug string, baseURL string) string {
	if baseURL == "" {
		baseURL = "https://filmorauz.net"
	}
	return fmt.Sprintf("%s/movies/%s", baseURL, slug)
}

// ListAllMoviesAdmin returns all movies regardless of approval status for admin dashboard.
func (s *MovieService) ListAllMoviesAdmin(page, limit int) ([]models.Movie, int64, error) {
	return s.repo.ListAdmin(page, limit)
}

// SetMovieApprovalStatus approves or rejects a movie.
func (s *MovieService) SetMovieApprovalStatus(id, status, byUserID string) error {
	if status == "approved" {
		current, err := s.GetMovieByID(id)
		if err != nil {
			return err
		}
		existing, rule, err := s.findDuplicateMovie(&models.MovieInput{
			Source:     current.Source,
			Title:      current.Title,
			Year:       current.Year,
			Slug:       current.Slug,
			TMDBID:     current.TMDBID,
			SourceType: current.SourceType,
		}, current.Slug, &current.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			log.Printf("[MOVIE DUPLICATE BLOCKED] approve movie_id=%s rule=%s existing_id=%s", current.ID.Hex(), rule, existing.ID.Hex())
			return &DuplicateMovieError{
				Message: "movie already exists",
				Rule:    rule,
				MovieID: existing.ID,
			}
		}
	}
	return s.repo.SetApprovalStatus(id, status, byUserID)
}

func sanitizeMovieSource(source *models.MovieSource) *models.MovieSource {
	if source == nil {
		return nil
	}
	provider := strings.TrimSpace(source.Provider)
	sourceURL := strings.TrimSpace(source.SourceURL)
	sourceID := strings.TrimSpace(source.SourceID)
	if provider == "" && sourceURL == "" && sourceID == "" {
		return nil
	}
	return &models.MovieSource{
		Provider:  provider,
		SourceURL: sourceURL,
		SourceID:  sourceID,
	}
}

func matchesDuplicateCandidate(movie *models.Movie, normalizedTitle string, year int) bool {
	if movie == nil || year <= 0 || movie.Year != year {
		return false
	}
	for _, candidate := range []string{movie.Title, movie.TitleUz, movie.OriginalTitle} {
		if candidate != "" && normalizeTitle(candidate) == normalizedTitle {
			return true
		}
	}
	return false
}

func (s *MovieService) findDuplicateMovie(input *models.MovieInput, desiredSlug string, excludeID *primitive.ObjectID) (*models.Movie, string, error) {
	if input == nil {
		return nil, "", nil
	}

	sameMovie := func(movie *models.Movie) bool {
		return movie != nil && excludeID != nil && movie.ID == *excludeID
	}

	source := sanitizeMovieSource(input.Source)
	if source != nil && source.Provider != "" && source.SourceURL != "" {
		movie, err := s.repo.FindBySourceURL(source.Provider, source.SourceURL)
		if err != nil {
			return nil, "", fmt.Errorf("failed duplicate check by source+source_url: %w", err)
		}
		if movie != nil && !sameMovie(movie) {
			return movie, "source+source_url", nil
		}
	}

	if source != nil && source.Provider != "" && source.SourceID != "" {
		movie, err := s.repo.FindBySourceID(source.Provider, source.SourceID)
		if err != nil {
			return nil, "", fmt.Errorf("failed duplicate check by source+external_id: %w", err)
		}
		if movie != nil && !sameMovie(movie) {
			return movie, "external_id", nil
		}
	}

	if desiredSlug != "" {
		movie, err := s.repo.FindBySlug(desiredSlug)
		if err == nil && movie != nil && !sameMovie(movie) {
			return movie, "slug", nil
		}
		if err != nil && !errors.Is(err, mongo.ErrNoDocuments) {
			return nil, "", fmt.Errorf("failed duplicate check by slug: %w", err)
		}
	}

	normalizedTitle := normalizeTitle(input.Title)
	if normalizedTitle != "" && input.Year > 0 {
		movie, err := s.repo.FindByNormalizedTitleYear(normalizedTitle, input.Year)
		if err != nil {
			return nil, "", fmt.Errorf("failed duplicate check by normalized title+year: %w", err)
		}
		if movie != nil && !sameMovie(movie) {
			return movie, "normalized_title+year", nil
		}
	}

	if input.TMDBID > 0 {
		movie, err := s.repo.FindByTMDBID(input.TMDBID)
		if err != nil {
			return nil, "", fmt.Errorf("failed duplicate check by external id: %w", err)
		}
		if movie != nil && !sameMovie(movie) {
			return movie, "external_id", nil
		}
	}

	if normalizedTitle != "" && input.Year > 0 {
		candidates, err := s.repo.FindByYear(input.Year)
		if err != nil {
			return nil, "", fmt.Errorf("failed duplicate fallback check: %w", err)
		}
		for i := range candidates {
			candidate := candidates[i]
			if excludeID != nil && candidate.ID == *excludeID {
				continue
			}
			if matchesDuplicateCandidate(&candidate, normalizedTitle, input.Year) {
				return &candidate, "normalized_title+year", nil
			}
		}
	}

	return nil, "", nil
}

// GetRecommendationsAdvanced returns recommendations using hybrid scoring (content + popularity + user personalization)
func (s *MovieService) GetRecommendationsAdvanced(movieID string, userID string, limit int) ([]models.Movie, error) {
	if limit < 1 || limit > 20 {
		limit = 12
	}
	return s.repo.GetRecommendations(movieID, userID, limit)
}
