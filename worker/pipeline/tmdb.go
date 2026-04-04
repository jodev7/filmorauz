package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
)

const (
	tmdbBaseURL      = "https://api.themoviedb.org/3"
	tmdbImageBaseURL = "https://image.tmdb.org/t/p"
)

// TMDBClient handles interactions with The Movie Database API
type TMDBClient struct {
	apiKey     string
	httpClient *http.Client
}

// NewTMDBClient creates a new TMDB client
func NewTMDBClient(apiKey string) *TMDBClient {
	return &TMDBClient{
		apiKey: apiKey,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// TMDBSearchResponse represents the TMDB search/movie response
type TMDBSearchResponse struct {
	Page         int         `json:"page"`
	Results      []TMDBMovie `json:"results"`
	TotalPages   int         `json:"total_pages"`
	TotalResults int         `json:"total_results"`
}

// TMDBMovie represents a movie from TMDB search results
type TMDBMovie struct {
	ID               int     `json:"id"`
	Title            string  `json:"title"`
	OriginalTitle    string  `json:"original_title"`
	Overview         string  `json:"overview"`
	ReleaseDate      string  `json:"release_date"`
	GenreIDs         []int   `json:"genre_ids"`
	PosterPath       string  `json:"poster_path"`
	BackdropPath     string  `json:"backdrop_path"`
	OriginalLanguage string  `json:"original_language"`
	Popularity       float64 `json:"popularity"`
	VoteAverage      float64 `json:"vote_average"`
	VoteCount        int     `json:"vote_count"`
}

// TMDBMovieDetails represents full movie details from TMDB
type TMDBMovieDetails struct {
	ID                  int           `json:"id"`
	Title               string        `json:"title"`
	OriginalTitle       string        `json:"original_title"`
	Overview            string        `json:"overview"`
	ReleaseDate         string        `json:"release_date"`
	Genres              []TMDBGenre   `json:"genres"`
	PosterPath          string        `json:"poster_path"`
	BackdropPath        string        `json:"backdrop_path"`
	Runtime             int           `json:"runtime"`
	ProductionCountries []TMDBCountry `json:"production_countries"`
	OriginalLanguage    string        `json:"original_language"`
	Popularity          float64       `json:"popularity"`
	VoteAverage         float64       `json:"vote_average"`
	VoteCount           int           `json:"vote_count"`
}

// TMDBGenre represents a genre from TMDB
type TMDBGenre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// TMDBCountry represents a production country from TMDB
type TMDBCountry struct {
	ISO3166_1 string `json:"iso_3166_1"`
	Name      string `json:"name"`
}

// TMDBGenreList represents the genre list response
type TMDBGenreList struct {
	Genres []TMDBGenre `json:"genres"`
}

// SearchMovie searches for a movie by title on TMDB
// Returns the best matching movie or nil if not found
func (c *TMDBClient) SearchMovie(ctx context.Context, title string, year int) (*TMDBMovie, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	if title == "" {
		return nil, fmt.Errorf("title is empty")
	}

	log.Printf("[TMDB] Searching for movie: title=%s, year=%d", title, year)

	// Build search query
	query := title
	if year > 0 {
		query = fmt.Sprintf("%s %d", title, year)
	}

	// URL encode the query
	encodedQuery := url.QueryEscape(query)

	// Build search URL
	searchURL := fmt.Sprintf("%s/search/movie?query=%s&api_key=%s&language=en-US&page=1&include_adult=false",
		tmdbBaseURL, encodedQuery, c.apiKey)

	log.Printf("[TMDB] Search URL: %s", strings.Replace(searchURL, c.apiKey, "***", 1))

	// Make request
	req, err := http.NewRequestWithContext(ctx, "GET", searchURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TMDB search returned status %d: %s", resp.StatusCode, string(body))
	}

	var searchResp TMDBSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB search response: %w", err)
	}

	if len(searchResp.Results) == 0 {
		log.Printf("[TMDB] No results found for: %s", title)
		return nil, nil
	}

	// Find best match
	bestMatch := c.findBestMatch(searchResp.Results, title, year)
	if bestMatch == nil {
		log.Printf("[TMDB] No suitable match found for: %s", title)
		return nil, nil
	}

	log.Printf("[TMDB] Found match: id=%d, title=%s, original_title=%s, release_date=%s, poster_path=%s",
		bestMatch.ID, bestMatch.Title, bestMatch.OriginalTitle, bestMatch.ReleaseDate, bestMatch.PosterPath)

	return bestMatch, nil
}

// GetMovieDetails fetches full movie details from TMDB
func (c *TMDBClient) GetMovieDetails(ctx context.Context, movieID int) (*TMDBMovieDetails, error) {
	return c.GetMovieDetailsWithLanguage(ctx, movieID, "en-US")
}

// GetMovieDetailsWithLanguage fetches full movie details from TMDB with specified language
func (c *TMDBClient) GetMovieDetailsWithLanguage(ctx context.Context, movieID int, language string) (*TMDBMovieDetails, error) {
	if c.apiKey == "" {
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	log.Printf("[TMDB] Fetching movie details for ID: %d, language: %s", movieID, language)

	detailsURL := fmt.Sprintf("%s/movie/%d?api_key=%s&language=%s",
		tmdbBaseURL, movieID, c.apiKey, language)

	req, err := http.NewRequestWithContext(ctx, "GET", detailsURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("TMDB details request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("TMDB details returned status %d: %s", resp.StatusCode, string(body))
	}

	var details TMDBMovieDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to decode TMDB details response: %w", err)
	}

	log.Printf("[TMDB] Movie details (%s): title=%s, runtime=%d, genres=%d, countries=%d",
		language, details.Title, details.Runtime, len(details.Genres), len(details.ProductionCountries))

	return &details, nil
}

// GetUzbekMovieDetails fetches movie details from TMDB with Uzbek translations if available
func (c *TMDBClient) GetUzbekMovieDetails(ctx context.Context, movieID int) (*TMDBMovieDetails, *TMDBMovieDetails, error) {
	log.Printf("[TMDB] Fetching Uzbek-localized movie details for ID: %d", movieID)

	// Fetch both English and Uzbek versions in parallel would be ideal,
	// but for simplicity, we'll fetch English first then Uzbek
	englishDetails, err := c.GetMovieDetails(ctx, movieID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get English details: %w", err)
	}

	// Try to get Uzbek translation
	uzbekDetails, err := c.GetMovieDetailsWithLanguage(ctx, movieID, "uz")
	if err != nil {
		log.Printf("[TMDB] WARNING: Failed to get Uzbek details: %v, falling back to English only", err)
		return englishDetails, nil, nil
	}

	// Check if Uzbek translation actually exists (TMDB returns original if no translation)
	if uzbekDetails.Title == englishDetails.Title && 
		uzbekDetails.Overview == englishDetails.Overview {
		log.Printf("[TMDB] No Uzbek translation available for movie ID: %d", movieID)
		return englishDetails, nil, nil
	}

	log.Printf("[TMDB] Uzbek translation found for movie ID: %d, title: %s", movieID, uzbekDetails.Title)
	return englishDetails, uzbekDetails, nil
}

// TMDBLocalizedMetadata holds both English and Uzbek metadata from TMDB
type TMDBLocalizedMetadata struct {
	English *TMDBMovieDetails
	Uzbek   *TMDBMovieDetails
}

// BuildPosterURL constructs the full poster URL from TMDB poster_path
func (c *TMDBClient) BuildPosterURL(posterPath string) string {
	if posterPath == "" {
		return ""
	}
	// Use original quality for best results
	return fmt.Sprintf("%s/original%s", tmdbImageBaseURL, posterPath)
}

// BuildBackdropURL constructs the full backdrop URL from TMDB backdrop_path
func (c *TMDBClient) BuildBackdropURL(backdropPath string) string {
	if backdropPath == "" {
		return ""
	}
	return fmt.Sprintf("%s/original%s", tmdbImageBaseURL, backdropPath)
}

// findBestMatch finds the best matching movie from search results
func (c *TMDBClient) findBestMatch(results []TMDBMovie, title string, year int) *TMDBMovie {
	if len(results) == 0 {
		return nil
	}

	normalizedTitle := normalizeTitle(title)

	// First pass: exact title match with year
	if year > 0 {
		for _, movie := range results {
			if normalizeTitle(movie.Title) == normalizedTitle && extractYear(movie.ReleaseDate) == year {
				return &movie
			}
			if normalizeTitle(movie.OriginalTitle) == normalizedTitle && extractYear(movie.ReleaseDate) == year {
				return &movie
			}
		}
	}

	// Second pass: title match (any year)
	for _, movie := range results {
		if normalizeTitle(movie.Title) == normalizedTitle {
			return &movie
		}
		if normalizeTitle(movie.OriginalTitle) == normalizedTitle {
			return &movie
		}
	}

	// Third pass: partial title match
	for _, movie := range results {
		if isPartialMatch(normalizedTitle, normalizeTitle(movie.Title)) ||
			isPartialMatch(normalizedTitle, normalizeTitle(movie.OriginalTitle)) {
			return &movie
		}
	}

	// Fallback: return first result (most popular)
	return &results[0]
}

// normalizeTitle normalizes a title for comparison
func normalizeTitle(title string) string {
	title = strings.ToLower(title)
	title = strings.TrimSpace(title)
	// Remove common punctuation
	replacer := strings.NewReplacer(
		":", " ",
		"-", " ",
		"'", "",
		"\"", "",
		"!", "",
		"?", "",
		".", "",
		",", "",
	)
	title = replacer.Replace(title)
	// Collapse multiple spaces
	for strings.Contains(title, "  ") {
		title = strings.ReplaceAll(title, "  ", " ")
	}
	return strings.TrimSpace(title)
}

// NormalizeTitleForSearch cleans a title for TMDB search
// Removes Uzbek/Russian words and other noise that would prevent TMDB matching
func NormalizeTitleForSearch(title string) string {
	if title == "" {
		return ""
	}

	log.Printf("[TMDB] Normalizing title for search: '%s'", title)

	// Convert to lowercase for processing
	normalized := strings.ToLower(title)

	// Remove common Uzbek/Russian suffixes and words
	wordsToRemove := []string{
		// Uzbek words
		"uzbek tilida",
		"uzbekcha",
		"uzbek",
		"premyera",
		"tarjima",
		"tarjimasi",
		"o'zbek tilida",
		"o'zbekcha",
		// Russian words
		"на узбекском",
		"узбекский",
		"премьера",
		"перевод",
		"смотреть",
		"онлайн",
		"бесплатно",
		// Quality/format indicators
		"hd",
		"full hd",
		"1080p",
		"720p",
		"480p",
		"4k",
		"web-dl",
		"webrip",
		"hdtv",
		"bdrip",
		"dvdrip",
		"ts",
		"cam",
		// Common noise words
		"online",
		"watch",
		"free",
		"movie",
		"film",
		"serial",
		"series",
		"episode",
		"season",
		"qism",
		"bob",
		"fason",
		"fasl",
	}

	// Remove each word/phrase
	for _, word := range wordsToRemove {
		normalized = strings.ReplaceAll(normalized, word, " ")
	}

	// Remove extra whitespace
	for strings.Contains(normalized, "  ") {
		normalized = strings.ReplaceAll(normalized, "  ", " ")
	}

	normalized = strings.TrimSpace(normalized)

	// If normalization resulted in empty string, return original
	if normalized == "" {
		log.Printf("[TMDB] WARNING: Title normalization resulted in empty string, using original")
		return strings.TrimSpace(title)
	}

	log.Printf("[TMDB] Normalized title: '%s' -> '%s'", title, normalized)
	return normalized
}

// extractYear extracts year from a date string (YYYY-MM-DD format)
func extractYear(dateStr string) int {
	if dateStr == "" || len(dateStr) < 4 {
		return 0
	}
	year := 0
	fmt.Sscanf(dateStr[:4], "%d", &year)
	return year
}

// isPartialMatch checks if two titles have significant overlap
func isPartialMatch(a, b string) bool {
	if a == b {
		return true
	}
	// Check if one contains the other
	if strings.Contains(a, b) || strings.Contains(b, a) {
		return true
	}
	// Check word overlap
	wordsA := strings.Fields(a)
	wordsB := strings.Fields(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return false
	}
	matchCount := 0
	for _, wa := range wordsA {
		for _, wb := range wordsB {
			if wa == wb {
				matchCount++
				break
			}
		}
	}
	// At least 50% of words should match
	minWords := len(wordsA)
	if len(wordsB) < minWords {
		minWords = len(wordsB)
	}
	return float64(matchCount) >= float64(minWords)*0.5
}

// ConvertToEnrichedMetadata converts TMDB data to EnrichedMetadata
func (c *TMDBClient) ConvertToEnrichedMetadata(movie *TMDBMovie, details *TMDBMovieDetails) *models.EnrichedMetadata {
	if movie == nil {
		return nil
	}

	metadata := &models.EnrichedMetadata{
		Title:          movie.Title,
		OriginalTitle:  movie.OriginalTitle,
		Description:    movie.Overview,
		Year:           extractYear(movie.ReleaseDate),
		PosterURL:      c.BuildPosterURL(movie.PosterPath),
		BackdropURL:    c.BuildBackdropURL(movie.BackdropPath),
		SourceProvider: "tmdb",
		SourceURL:      fmt.Sprintf("https://www.themoviedb.org/movie/%d", movie.ID),
	}

	// If we have full details, use them for richer metadata
	if details != nil {
		metadata.Title = details.Title
		metadata.OriginalTitle = details.OriginalTitle
		metadata.Description = details.Overview
		metadata.Year = extractYear(details.ReleaseDate)
		metadata.Duration = details.Runtime
		metadata.PosterURL = c.BuildPosterURL(details.PosterPath)
		metadata.BackdropURL = c.BuildBackdropURL(details.BackdropPath)

		// Extract genre names
		genres := make([]string, 0, len(details.Genres))
		for _, g := range details.Genres {
			genres = append(genres, g.Name)
		}
		metadata.Genres = genres

		// Extract country names
		countries := make([]string, 0, len(details.ProductionCountries))
		for _, c := range details.ProductionCountries {
			countries = append(countries, c.Name)
		}
		metadata.Countries = countries
	} else {
		// Use genre IDs from search result (less accurate but available)
		// Note: We'd need a genre map to convert IDs to names
		// For now, leave genres empty if we don't have details
		metadata.Genres = []string{}
		metadata.Countries = []string{}
	}

	return metadata
}

// SearchAndEnrichMovie searches TMDB and returns enriched metadata
// This is the main entry point for TMDB integration
func (c *TMDBClient) SearchAndEnrichMovie(ctx context.Context, title string, year int) (*models.EnrichedMetadata, error) {
	if c.apiKey == "" {
		log.Printf("[TMDB] API key not configured, skipping TMDB search")
		return nil, fmt.Errorf("TMDB API key not configured")
	}

	// Step 1: Search for movie
	movie, err := c.SearchMovie(ctx, title, year)
	if err != nil {
		return nil, fmt.Errorf("TMDB search failed: %w", err)
	}

	if movie == nil {
		log.Printf("[TMDB] No movie found for: %s", title)
		return nil, nil
	}

	// Step 2: Get full details (English)
	var details *TMDBMovieDetails
	details, err = c.GetMovieDetails(ctx, movie.ID)
	if err != nil {
		log.Printf("[TMDB] Failed to get details, using search result: %v", err)
		// Continue with search result only
		details = nil
	}

	// Step 3: Convert to enriched metadata
	metadata := c.ConvertToEnrichedMetadata(movie, details)
	if metadata == nil {
		return nil, fmt.Errorf("failed to convert TMDB data to metadata")
	}

	// Step 4: Fetch Uzbek translations from TMDB
	log.Printf("[TMDB] Fetching Uzbek translations for movie ID: %d", movie.ID)
	uzbekDetails, err := c.GetMovieDetailsWithLanguage(ctx, movie.ID, "uz")
	if err != nil {
		log.Printf("[TMDB] WARNING: Failed to get Uzbek details: %v", err)
	} else if uzbekDetails != nil {
		// Check if Uzbek translation actually exists (TMDB returns original if no translation)
		if details != nil && uzbekDetails.Title != details.Title {
			log.Printf("[TMDB] Uzbek translation found: title=%s", uzbekDetails.Title)
			metadata.TitleUz = uzbekDetails.Title
			metadata.DescriptionUz = uzbekDetails.Overview
			metadata.HasTMDBUzTranslation = true
		} else if details == nil && uzbekDetails.Title != movie.Title {
			log.Printf("[TMDB] Uzbek translation found: title=%s", uzbekDetails.Title)
			metadata.TitleUz = uzbekDetails.Title
			metadata.DescriptionUz = uzbekDetails.Overview
			metadata.HasTMDBUzTranslation = true
		} else {
			log.Printf("[TMDB] No Uzbek translation available for movie ID: %d", movie.ID)
		}
	}

	log.Printf("[TMDB] Enriched metadata: title=%s, title_uz=%s, year=%d, poster=%s, genres=%v",
		metadata.Title, metadata.TitleUz, metadata.Year, metadata.PosterURL, metadata.Genres)

	return metadata, nil
}
