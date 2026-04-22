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
		trimmed := strings.TrimSpace(g)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if !seen[lower] {
			seen[lower] = true
			result = append(result, lower)
		}
	}
	log.Printf("[MOVIE] normalizeMovieGenres: in=%v out=%v", genres, result)
	return result
}

type MovieService struct {
	repo            *repositories.MovieRepository
	counterRepo     *repositories.CounterRepository
	notificationSvc *NotificationService
	viewEventRepo   *repositories.MovieViewEventRepository
	clipRepo        *repositories.ClipRepository // optional — enables clip cleanup on delete
	b2Cleanup       *B2CleanupService            // optional — nil means skip B2 cleanup
}

func NewMovieService(repo *repositories.MovieRepository, counterRepo *repositories.CounterRepository, notificationSvc *NotificationService, viewEventRepo *repositories.MovieViewEventRepository) *MovieService {
	return &MovieService{repo: repo, counterRepo: counterRepo, notificationSvc: notificationSvc, viewEventRepo: viewEventRepo}
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
	slug, err := s.generateUniqueSlug(input.Title, input.Year)
	if err != nil {
		return nil, err
	}

	// Generate unique alphanumeric code
	code, err := s.generateUniqueCode()
	if err != nil {
		return nil, fmt.Errorf("failed to generate movie code: %w", err)
	}

	// Get website base URL from environment
	websiteBaseURL := os.Getenv("BASE_SITE_URL")
	if websiteBaseURL == "" {
		websiteBaseURL = "https://filmorauz.net"
	}

	movie := &models.Movie{
		ID:          primitive.NewObjectID(),
		Code:        code,
		Title:       input.Title,
		Description: input.Description,
		PosterURL:   input.PosterURL,
		BackdropURL: input.BackdropURL,
		Year:        input.Year,
		Genre:       normalizeMovieGenres(input.Genre),
		Country:     input.Country,
		VideoURL:    input.VideoURL,
		EmbedURL:    input.EmbedURL,
		SourceType:  input.SourceType,
		Duration:    input.Duration,
		Quality:     input.Quality,
		Slug:        slug,
		WebsiteURL:  calculateWebsiteURL(slug, websiteBaseURL),
		RatingAvg:   0,
		RatingCount: 0,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),

		// LOCALIZATION: Uzbek display fields
		TitleUz:        input.TitleUz,
		DescriptionUz:  input.DescriptionUz,
		GenresUz:       input.GenresUz,
		CountriesUz:    input.CountriesUz,
		OriginalTitle:  input.OriginalTitle,
		TMDBID:         input.TMDBID,
		MetadataSource: input.MetadataSource,

		// Approval workflow: new content starts hidden pending admin review
		ApprovalStatus: "pending",
		IsPublished:    false,
	}

	if err := s.repo.Create(movie); err != nil {
		return nil, fmt.Errorf("failed to create movie: %w", err)
	}

	// Send notification asynchronously (non-blocking)
	// This won't affect the movie creation success
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
// clips), its clip DB rows, and the movie document. B2/clip cleanup runs
// best-effort: a missing file or transient delete error is logged but does
// not abort the flow — the DB record is always removed at the end so the
// admin UI never gets stuck on a half-deleted movie.
func (s *MovieService) DeleteMovie(id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return fmt.Errorf("invalid movie id")
	}

	movie, findErr := s.repo.FindByID(objID)
	if findErr != nil || movie == nil {
		// Movie already gone — still try the final DeleteOne so the caller
		// sees a consistent response.
		log.Printf("[MOVIE DELETE] id=%s not found in DB (err=%v), attempting repo.Delete anyway", id, findErr)
		return s.repo.Delete(id)
	}

	log.Printf("[MOVIE DELETE] start — id=%s title=%q code=%s", id, movie.Title, movie.Code)

	s.cleanupMovieStorage(objID, movie)

	if delErr := s.repo.Delete(id); delErr != nil {
		log.Printf("[MOVIE DELETE] FAILED repo.Delete id=%s: %v", id, delErr)
		return delErr
	}
	log.Printf("[MOVIE DELETE] done — id=%s removed from DB", id)
	return nil
}

// cleanupMovieStorage deletes every B2 asset associated with a movie and
// wipes its clip rows. Every step is best-effort + logged.
func (s *MovieService) cleanupMovieStorage(objID primitive.ObjectID, movie *models.Movie) {
	if s.b2Cleanup == nil {
		log.Printf("[MOVIE DELETE] B2 cleanup disabled (no service configured) — skipping storage delete")
	} else {
		// 1) HLS folder: wipe videos/<folder>/ recursively so master.m3u8,
		//    quality subfolders, and all .ts segments are removed in one pass.
		prefix := firstNonEmpty(
			s.b2Cleanup.DeriveVideoFolderPrefix(movie.MasterPlaylistURL),
			s.b2Cleanup.DeriveVideoFolderPrefix(movie.VideoURL),
		)
		if prefix != "" {
			log.Printf("[MOVIE DELETE] HLS prefix to purge: %s", prefix)
			if _, err := s.b2Cleanup.DeleteByPrefix(prefix); err != nil {
				log.Printf("[MOVIE DELETE] HLS prefix delete failed (continuing): %v", err)
			}
		} else {
			log.Printf("[MOVIE DELETE] no videos/ prefix derivable from master=%q video=%q — skipping HLS folder purge",
				movie.MasterPlaylistURL, movie.VideoURL)
		}

		// 2) Individual video URL if it points outside the HLS folder (e.g.
		//    an .mp4 upload that lives in temp/videos/…). Safe if already covered.
		if videoKey := s.b2Cleanup.ExtractKeyFromURL(movie.VideoURL); videoKey != "" && !strings.HasPrefix(videoKey, prefix) {
			if _, err := s.b2Cleanup.DeleteByKey(videoKey); err != nil {
				log.Printf("[MOVIE DELETE] video key delete failed key=%s err=%v", videoKey, err)
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
				log.Printf("[MOVIE DELETE] %s delete failed key=%s err=%v", label, key, err)
			}
		}
	}

	// 4) Clips — delete B2 files then DB rows.
	if s.clipRepo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		clips, cErr := s.clipRepo.FindByMovieID(ctx, objID)
		if cErr != nil {
			log.Printf("[MOVIE DELETE] FindByMovieID failed (continuing): %v", cErr)
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
						log.Printf("[MOVIE DELETE] clip delete failed key=%s err=%v", key, err)
					}
				}
			}
			if err := s.clipRepo.DeleteByMovieID(ctx, objID); err != nil {
				log.Printf("[MOVIE DELETE] clipRepo.DeleteByMovieID failed (continuing): %v", err)
			} else {
				log.Printf("[MOVIE DELETE] removed %d clip row(s) from DB", len(clips))
			}
		}
	}
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
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
	// Find highest existing code from movies collection directly
	highestSeq, err := s.repo.FindHighestCode()
	if err != nil {
		return "", fmt.Errorf("failed to find highest code: %w", err)
	}

	// Generate next code
	nextSeq := highestSeq + 1

	// Check if we've exceeded the maximum
	if nextSeq > codeMaxLimit {
		return "", fmt.Errorf("movie code limit exceeded: %d > %d", nextSeq, codeMaxLimit)
	}

	// Format the code based on the sequence range
	code := formatMovieCode(nextSeq)

	log.Printf("Generated sequential movie code: %s (highest_existing: %d, next: %d)", code, highestSeq, nextSeq)
	return code, nil
}

// formatMovieCode formats a sequence number as a zero-padded numeric code
// Rules:
//   - seq <= 9999    → 4-digit zero-padded (e.g., 1 -> "0001")
//   - seq <= 99999   → 5-digit zero-padded (e.g., 10000 -> "10000")
//   - seq <= 999999  → 6-digit zero-padded (e.g., 100000 -> "100000")
func formatMovieCode(seq int64) string {
	switch {
	case seq <= 9999:
		return fmt.Sprintf("%04d", seq)
	case seq <= 99999:
		return fmt.Sprintf("%05d", seq)
	default:
		return fmt.Sprintf("%06d", seq)
	}
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
	return s.repo.SetApprovalStatus(id, status, byUserID)
}

// GetRecommendationsAdvanced returns recommendations using hybrid scoring (content + popularity + user personalization)
func (s *MovieService) GetRecommendationsAdvanced(movieID string, userID string, limit int) ([]models.Movie, error) {
	if limit < 1 || limit > 20 {
		limit = 12
	}
	return s.repo.GetRecommendations(movieID, userID, limit)
}
