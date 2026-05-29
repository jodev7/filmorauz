package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
)

// MetadataEnricher handles metadata enrichment from external sources
type MetadataEnricher struct {
	httpClient *http.Client
	parserURL  string
	tmdbClient *TMDBClient
}

// NewMetadataEnricher creates a new metadata enricher
func NewMetadataEnricher(parserURL string, tmdbAPIKey string) *MetadataEnricher {
	var tmdbClient *TMDBClient
	if tmdbAPIKey != "" {
		tmdbClient = NewTMDBClient(tmdbAPIKey)
		log.Printf("[METADATA] TMDB client initialized with API key")
	} else {
		log.Printf("[METADATA] WARNING: TMDB API key not provided, TMDB enrichment disabled")
	}

	return &MetadataEnricher{
		parserURL: parserURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		tmdbClient: tmdbClient,
	}
}

// EnrichRequest represents a metadata enrichment request
type EnrichRequest struct {
	Query  string `json:"q"`
	Source string `json:"source"`
}

// EnrichResult represents the result from search
type EnrichResult struct {
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Description   string   `json:"description"`
	Year          int      `json:"year"`
	Genres        []string `json:"genres"`
	Countries     []string `json:"countries"`
	PosterURL     string   `json:"poster_url"`
	BackdropURL   string   `json:"backdrop_url"`
	Duration      int      `json:"duration"`
	Quality       string   `json:"quality"`
	Translation   string   `json:"translation"`
}

// SearchResultsResponse represents the search response from parser
type SearchResultsResponse struct {
	Results []EnrichResult `json:"results"`
}

// EnrichMetadata enriches movie metadata using external sources
// Priority: TMDB (primary) -> Parser (fallback)
// Returns enriched metadata and the source (tmdb/parser/fallback)
func (e *MetadataEnricher) EnrichMetadata(ctx context.Context, parserMetadata *models.ParsedMovieMetadata, jobSource string) (*models.EnrichedMetadata, string, error) {
	if parserMetadata == nil {
		return nil, "parser", fmt.Errorf("parser metadata is nil")
	}

	title := parserMetadata.Title
	if title == "" {
		log.Printf("[METADATA] No title provided, using fallback")
		fallback := e.createFallbackMetadata(parserMetadata)
		e.populateLocalizedFields(fallback)
		return fallback, "parser", nil
	}

	log.Printf("[METADATA] Enriching metadata for: %s", title)

	// STEP 1: Try TMDB first (PRIMARY SOURCE)
	if e.tmdbClient != nil {
		// Normalize title for TMDB search (remove Uzbek/Russian words, quality indicators, etc.)
		normalizedTitle := NormalizeTitleForSearch(title)
		log.Printf("[METADATA] Searching TMDB for: '%s' (normalized: '%s', year=%d)", title, normalizedTitle, parserMetadata.Year)
		tmdbMetadata, tmdbErr := e.tmdbClient.SearchAndEnrichMovie(ctx, normalizedTitle, parserMetadata.Year)
		if tmdbErr != nil {
			log.Printf("[METADATA] TMDB search failed: %v, falling back to parser", tmdbErr)
		} else if tmdbMetadata != nil {
			log.Printf("[METADATA] TMDB match found: title=%s, originalTitle=%s, year=%d, poster=%s",
				tmdbMetadata.Title, tmdbMetadata.OriginalTitle, tmdbMetadata.Year, tmdbMetadata.PosterURL)

			// Merge with parser metadata for any missing fields
			merged := MergeMetadata(tmdbMetadata, parserMetadata)
			if merged != nil {
				// CRITICAL: Always use TMDB poster, never parser poster
				// Parser posters contain watermarks (uzmovi.com)
				if tmdbMetadata.PosterURL != "" {
					merged.PosterURL = tmdbMetadata.PosterURL
					log.Printf("[METADATA] Using CLEAN TMDB poster: %s", merged.PosterURL)
				} else {
					log.Printf("[METADATA] WARNING: TMDB returned no poster, will need fallback")
					merged.PosterURL = ""
				}
				// Always use TMDB backdrop if available
				if tmdbMetadata.BackdropURL != "" {
					merged.BackdropURL = tmdbMetadata.BackdropURL
				}
				merged.SourceProvider = "tmdb"

				// Populate Uzbek localized fields
				e.populateLocalizedFields(merged)

				log.Printf("[METADATA] FINAL enriched metadata: title=%s, title_uz=%s, description_uz=%s, genres_uz=%v, countries_uz=%v",
					merged.Title, merged.TitleUz, merged.DescriptionUz, merged.GenresUz, merged.CountriesUz)

				return merged, "tmdb", nil
			}
		} else {
			log.Printf("[METADATA] TMDB returned no results for: %s", title)
		}
	} else {
		log.Printf("[METADATA] TMDB client not available, using parser only")
	}

	// STEP 2: Fallback to parser service (but NEVER use parser posters)
	log.Printf("[METADATA] Falling back to parser enrichment for: %s (source=%s)", title, jobSource)
	enriched, err := e.searchAndEnrich(ctx, title, parserMetadata.Year, jobSource)
	if err != nil {
		log.Printf("[METADATA] Parser enrichment failed: %v, using fallback", err)
		fallback := e.createFallbackMetadataFromParser(parserMetadata)
		e.populateLocalizedFields(fallback)
		return fallback, "parser", nil
	}

	if enriched == nil {
		log.Printf("[METADATA] No parser enrichment results, using fallback")
		fallback := e.createFallbackMetadataFromParser(parserMetadata)
		e.populateLocalizedFields(fallback)
		return fallback, "parser", nil
	}

	// CRITICAL: Clear parser poster URL to prevent watermarked posters
	// If TMDB didn't provide a poster, we'll handle it in poster generation
	if enriched.PosterURL != "" && strings.Contains(enriched.PosterURL, "uzmovi") {
		log.Printf("[METADATA] WARNING: Parser returned uzmovi poster, CLEARING to prevent watermark")
		enriched.PosterURL = ""
	}

	// Populate Uzbek localized fields
	e.populateLocalizedFields(enriched)

	log.Printf("[METADATA] Parser enrichment successful: title=%s, year=%d, poster=%s",
		enriched.Title, enriched.Year, enriched.PosterURL)

	return enriched, "parser", nil
}

// populateLocalizedFields adds Uzbek-localized display fields to metadata
func (e *MetadataEnricher) populateLocalizedFields(metadata *models.EnrichedMetadata) {
	if metadata == nil {
		return
	}

	log.Printf("[LOCALIZATION] Populating Uzbek display fields for: %s", metadata.Title)

	// Title: use TMDB Uzbek if available, otherwise try our title mapping
	if metadata.TitleUz != "" {
		log.Printf("[LOCALIZATION] Using TMDB Uzbek title: %s", metadata.TitleUz)
	} else {
		// Try our manual title mapping
		uzTitle := LocalizeTitle(metadata.OriginalTitle)
		if uzTitle != "" {
			metadata.TitleUz = uzTitle
			log.Printf("[LOCALIZATION] Found Uzbek title from mapping: %s", metadata.TitleUz)
		} else {
			log.Printf("[LOCALIZATION] No TMDB Uzbek title or mapping found, leaving title_uz empty")
		}
	}

	// Description: use TMDB Uzbek if available, otherwise leave empty
	// Description localization should be done by AI in the future
	if metadata.DescriptionUz != "" {
		log.Printf("[LOCALIZATION] Using TMDB Uzbek description (length: %d)", len(metadata.DescriptionUz))
	} else {
		log.Printf("[LOCALIZATION] No TMDB Uzbek description available, leaving description_uz empty")
	}

	// Genres: localize to Uzbek using mapping
	metadata.GenresUz = LocalizeGenres(metadata.Genres)
	log.Printf("[LOCALIZATION] Uzbek genres: %v", metadata.GenresUz)

	// Countries: localize to Uzbek using mapping
	metadata.CountriesUz = LocalizeCountries(metadata.Countries)
	log.Printf("[LOCALIZATION] Uzbek countries: %v", metadata.CountriesUz)

	// Log final state for debugging
	log.Printf("[LOCALIZATION] FINAL state: title_uz='%s', description_uz='%s', genres_uz=%v, countries_uz=%v",
		metadata.TitleUz, truncateForLog(metadata.DescriptionUz, 100), metadata.GenresUz, metadata.CountriesUz)
}

// truncateForLog truncates a string for logging purposes
func truncateForLog(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// searchAndEnrich searches for the movie and returns enriched metadata
func (e *MetadataEnricher) searchAndEnrich(ctx context.Context, title string, year int, jobSource string) (*models.EnrichedMetadata, error) {
	// Build search URL
	query := title
	if year > 0 {
		query = fmt.Sprintf("%s %d", title, year)
	}

	if jobSource == "" {
		jobSource = "uzmovi"
	}
	searchURL := fmt.Sprintf("%s/search?source=%s&q=%s", e.parserURL, jobSource, strings.ReplaceAll(query, " ", "+"))

	log.Printf("[METADATA] Searching: %s", searchURL)

	resp, err := e.httpClient.Get(searchURL)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("search returned status %d: %s", resp.StatusCode, string(body))
	}

	var result SearchResultsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	if len(result.Results) == 0 {
		log.Printf("[METADATA] No search results for: %s", title)
		return nil, nil
	}

	// The parser /search endpoint is a fuzzy site search: its first result is
	// frequently an unrelated movie (e.g. searching "Dastur ..." returned
	// "Otamning bezovta ruhi ..."). The authoritative title already came from
	// /details, so we must only accept a search result whose title actually
	// matches — otherwise we'd overwrite the movie identity with a different
	// film. Pick the first result that matches by title (and year, when known).
	matchIdx := -1
	for i := range result.Results {
		r := result.Results[i]
		if e.isSimilarTitle(title, r.Title) && (year == 0 || r.Year == 0 || r.Year == year) {
			matchIdx = i
			break
		}
	}

	if matchIdx < 0 {
		// No confident match. Do NOT hijack the title with an unrelated hit —
		// return nil so the caller falls back to parser /details metadata,
		// which preserves the correct title.
		log.Printf("[METADATA] No search result matched title=%q year=%d (first candidate=%q year=%d); discarding fuzzy search result to preserve original title",
			title, year, result.Results[0].Title, result.Results[0].Year)
		return nil, nil
	}

	first := result.Results[matchIdx]

	return &models.EnrichedMetadata{
		Title:          first.Title,
		OriginalTitle:  first.OriginalTitle,
		Description:    first.Description,
		Year:           first.Year,
		Genres:         first.Genres,
		Countries:      first.Countries,
		Duration:       first.Duration,
		Quality:        first.Quality,
		Translation:    first.Translation,
		PosterURL:      first.PosterURL,
		BackdropURL:    first.BackdropURL,
		SourceProvider: jobSource,
		SourceURL:      "",
	}, nil
}

// isSimilarTitle checks if two titles are similar enough
func (e *MetadataEnricher) isSimilarTitle(a, b string) bool {
	// Normalize both strings
	aNorm := strings.ToLower(strings.ReplaceAll(a, " ", ""))
	bNorm := strings.ToLower(strings.ReplaceAll(b, " ", ""))

	// Direct match
	if aNorm == bNorm {
		return true
	}

	// Check if one contains the other (for partial matches)
	if len(aNorm) > 3 && len(bNorm) > 3 {
		if strings.Contains(aNorm, bNorm) || strings.Contains(bNorm, aNorm) {
			return true
		}
	}

	// Check Levenshtein distance for small differences
	if len(aNorm) > 3 && len(bNorm) > 3 {
		dist := levenshteinDistance(aNorm, bNorm)
		maxLen := max(len(aNorm), len(bNorm))
		// Allow up to 30% difference
		return float64(dist)/float64(maxLen) < 0.3
	}

	return false
}

// createFallbackMetadata creates fallback metadata when no enrichment is available
func (e *MetadataEnricher) createFallbackMetadata(parserMetadata *models.ParsedMovieMetadata) *models.EnrichedMetadata {
	return e.createFallbackMetadataFromParser(parserMetadata)
}

// createFallbackMetadataFromParser converts parser metadata to enriched format
func (e *MetadataEnricher) createFallbackMetadataFromParser(pm *models.ParsedMovieMetadata) *models.EnrichedMetadata {
	if pm == nil {
		return &models.EnrichedMetadata{}
	}

	return &models.EnrichedMetadata{
		Title:          pm.Title,
		OriginalTitle:  "",
		Description:    pm.Description,
		Year:           pm.Year,
		Genres:         pm.Genres,
		Countries:      []string{pm.Country},
		Duration:       pm.Duration,
		Quality:        pm.Quality,
		Translation:    pm.Translation,
		PosterURL:      pm.Poster,
		BackdropURL:    pm.Backdrop,
		SourceProvider: "parser",
		SourceURL:      pm.VideoPageURL,
	}
}

// MergeMetadata merges enriched metadata with parser metadata
// Enriched takes priority, but falls back to parser values
func MergeMetadata(enriched *models.EnrichedMetadata, parser *models.ParsedMovieMetadata) *models.EnrichedMetadata {
	if enriched == nil {
		return nil
	}

	merged := *enriched

	// If enriched field is empty, use parser value
	if merged.Title == "" && parser != nil {
		merged.Title = parser.Title
	}
	if merged.Description == "" && parser != nil {
		merged.Description = parser.Description
	}
	if merged.Year == 0 && parser != nil && parser.Year > 0 {
		merged.Year = parser.Year
	}
	if len(merged.Genres) == 0 && parser != nil && len(parser.Genres) > 0 {
		merged.Genres = parser.Genres
	}
	if len(merged.Countries) == 0 && parser != nil && parser.Country != "" {
		merged.Countries = []string{parser.Country}
	}
	if merged.Duration == 0 && parser != nil && parser.Duration > 0 {
		merged.Duration = parser.Duration
	}
	if merged.PosterURL == "" && parser != nil && parser.Poster != "" {
		merged.PosterURL = parser.Poster
	}
	if merged.BackdropURL == "" && parser != nil && parser.Backdrop != "" {
		merged.BackdropURL = parser.Backdrop
	}

	return &merged
}

// levenshteinDistance calculates the Levenshtein distance between two strings
func levenshteinDistance(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}

	// Create matrix
	matrix := make([][]int, len(a)+1)
	for i := range matrix {
		matrix[i] = make([]int, len(b)+1)
	}

	// Initialize first column
	for i := range a {
		matrix[i+1][0] = i + 1
	}

	// Initialize first row
	for j := range b {
		matrix[0][j+1] = j + 1
	}

	// Fill in the rest
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}

			matrix[i][j] = matrix[i-1][j] + 1
			if matrix[i][j-1]+1 < matrix[i][j] {
				matrix[i][j] = matrix[i][j-1] + 1
			}
			if matrix[i-1][j-1]+cost < matrix[i][j] {
				matrix[i][j] = matrix[i-1][j-1] + cost
			}
		}
	}

	return matrix[len(a)][len(b)]
}
