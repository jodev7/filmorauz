package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
	"github.com/filmorauz/worker/services"
	"github.com/filmorauz/worker/storage"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// Font paths for premium branding
const (
	// LiberationSansBoldPath - semi-bold font for premium branding
	LiberationSansBoldPath = "/usr/share/fonts/truetype/liberation/LiberationSans-Bold.ttf"
	// Fallback font path
	UbuntuBoldPath = "/usr/share/fonts/truetype/ubuntu/Ubuntu-B.ttf"
)

// PosterGenerator handles poster generation and upload
type PosterGenerator struct {
	httpClient     *http.Client
	storage        storage.Storage
	tempDir        string
	aiEndpoint     string                 // Legacy AI image generation endpoint (optional)
	openAIClient   *services.OpenAIClient // OpenAI client for image generation
	storageMode    string                 // "dev" or "prod"
	baseUploadPath string                 // Base path for local uploads
	baseURL        string                 // Base URL for development mode (e.g., http://localhost:8080)
}

// NewPosterGenerator creates a new poster generator
func NewPosterGenerator(storageConfig storage.Config, tempDir string, aiEndpoint string, baseURL string) (*PosterGenerator, error) {
	return NewPosterGeneratorWithOpenAI(storageConfig, tempDir, aiEndpoint, baseURL, nil)
}

// NewPosterGeneratorWithOpenAI creates a new poster generator with optional OpenAI client
func NewPosterGeneratorWithOpenAI(storageConfig storage.Config, tempDir string, aiEndpoint string, baseURL string, openAIClient *services.OpenAIClient) (*PosterGenerator, error) {
	store, err := storage.NewStorage(storageConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	pg := &PosterGenerator{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		storage:        store,
		tempDir:        tempDir,
		aiEndpoint:     aiEndpoint,
		openAIClient:   openAIClient,
		storageMode:    storageConfig.Mode,
		baseUploadPath: storageConfig.LocalPath,
		baseURL:        baseURL,
	}

	// Log OpenAI client status
	if pg.openAIClient != nil && pg.openAIClient.IsConfigured() {
		log.Printf("[POSTER] OpenAI client is configured and ready for poster generation")
	} else {
		log.Printf("[POSTER] OpenAI client not configured - will use legacy AI endpoint if available")
	}

	return pg, nil
}

// PosterResult contains the result of poster generation
type PosterResult struct {
	OriginalPosterURL  string // URL of original poster found
	GeneratedPosterURL string // URL of AI-generated poster (if successful)
	PosterGenerated    bool   // Whether AI poster was generated
	// Multiple sizes for optimized loading
	PosterThumbURL    string // ~300px width poster thumbnail
	PosterMediumURL   string // ~600px width poster medium
	PosterOriginalURL string // Original size poster
}

// BackdropResult contains the result of backdrop generation
type BackdropResult struct {
	OriginalBackdropURL  string // URL of original backdrop found
	GeneratedBackdropURL string // URL of AI-generated backdrop (if successful)
	BackdropGenerated    bool   // Whether AI backdrop was generated
	// Multiple sizes for optimized loading
	BackdropThumbURL    string // ~640px width backdrop thumbnail
	BackdropMediumURL   string // ~1280px width backdrop medium
	BackdropOriginalURL string // Original size backdrop
}

// GeneratePoster generates an Uzbek-localized poster or finds original
// Returns the final poster URL to use
func (pg *PosterGenerator) GeneratePoster(ctx context.Context, enrichedMetadata *models.EnrichedMetadata, canonicalFolder string) (*PosterResult, error) {
	result := &PosterResult{}

	// Step 1: Try to find original poster from enriched metadata
	originalPosterURL := enrichedMetadata.PosterURL
	if originalPosterURL != "" {
		// CRITICAL: Check if poster is from watermarked source (uzmovi, etc.)
		if pg.isWatermarkedPoster(originalPosterURL) {
			log.Printf("[POSTER] WARNING: Poster from watermarked source detected, REJECTING: %s", originalPosterURL)
			log.Printf("[POSTER] Watermarked posters are NOT allowed - must use TMDB or AI-generated")
			originalPosterURL = ""
		} else {
			log.Printf("[POSTER] Found CLEAN original poster: %s", originalPosterURL)

			// Try to download and validate the original poster
			valid, err := pg.validatePosterURL(ctx, originalPosterURL)
			if err != nil || !valid {
				log.Printf("[POSTER] Original poster not accessible, will try AI generation")
				originalPosterURL = ""
			} else {
				result.OriginalPosterURL = originalPosterURL
				log.Printf("[POSTER] CLEAN poster validated successfully")
			}
		}
	}

	// Step 2: Use original URL — prefer the validated one, fall back to raw enriched URL.
	// Never return an error when we have any URL to work with; the logo overlay
	// in pipeline.go (step 6b) will download, brand, and save it as poster_branded.jpg.
	fallbackURL := result.OriginalPosterURL
	if fallbackURL == "" {
		fallbackURL = enrichedMetadata.PosterURL
		if fallbackURL != "" {
			log.Printf("[POSTER] Validation failed earlier but raw URL available — using as fallback: %s", fallbackURL)
		}
	}

	if fallbackURL != "" {
		result.GeneratedPosterURL = fallbackURL
		result.OriginalPosterURL = fallbackURL
		result.PosterGenerated = false // logo overlay in pipeline.go will set this true
		log.Printf("[POSTER] TMDB fallback poster URL returned for logo overlay: %s", fallbackURL)
		return result, nil
	}

	// Step 4: Truly nothing available
	log.Printf("[POSTER] WARNING: No poster URL available at all for: %s", enrichedMetadata.Title)
	return result, fmt.Errorf("no poster available")
}

// validatePosterURL checks if a poster URL is accessible
func (pg *PosterGenerator) validatePosterURL(ctx context.Context, posterURL string) (bool, error) {
	if posterURL == "" {
		return false, nil
	}

	// Skip validation for local/relative paths
	if !strings.HasPrefix(posterURL, "http") {
		return false, nil
	}

	req, err := http.NewRequestWithContext(ctx, "GET", posterURL, nil)
	if err != nil {
		return false, err
	}
	// Only fetch first 512 bytes to confirm reachability without downloading the full image
	req.Header.Set("Range", "bytes=0-511")

	resp, err := pg.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	return resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusPartialContent, nil
}

// isWatermarkedPoster checks if a poster URL is from a watermarked source
// CRITICAL: We NEVER use posters from uzmovi or other watermarked sources
func (pg *PosterGenerator) isWatermarkedPoster(posterURL string) bool {
	if posterURL == "" {
		return false
	}

	lowerURL := strings.ToLower(posterURL)

	// Check for known watermarked sources
	watermarkedSources := []string{
		"uzmovi",
		"uzmovi.com",
		"uzmovi.net",
		"uzmovi.org",
	}

	for _, source := range watermarkedSources {
		if strings.Contains(lowerURL, source) {
			log.Printf("[POSTER] Detected watermarked source '%s' in URL: %s", source, posterURL)
			return true
		}
	}

	return false
}

// generateAIPoster generates an Uzbek-localized poster using AI
// CRITICAL: canonicalFolder must be passed to ensure poster is saved in the SAME folder as HLS files
// Priority: 1. OpenAI client (if configured), 2. Legacy AI endpoint, 3. Error
func (pg *PosterGenerator) generateAIPoster(ctx context.Context, metadata *models.EnrichedMetadata, canonicalFolder string) (string, error) {
	if metadata == nil || metadata.Title == "" {
		return "", fmt.Errorf("metadata is empty")
	}

	log.Printf("[POSTER] generateAIPoster: using canonicalFolder=%s", canonicalFolder)

	// Priority 1: Use OpenAI client if configured
	if pg.openAIClient != nil && pg.openAIClient.IsConfigured() {
		log.Printf("[POSTER] Using OpenAI client for poster generation")
		return pg.generateAIPosterWithOpenAI(ctx, metadata, canonicalFolder)
	}

	// Priority 2: Use legacy AI endpoint if configured
	if pg.aiEndpoint != "" {
		log.Printf("[POSTER] Using legacy AI endpoint for poster generation")
		return pg.generateAIPosterLegacy(ctx, metadata, canonicalFolder)
	}

	// No AI generation available
	return "", fmt.Errorf("no AI generation configured - OpenAI client or AI_ENDPOINT required")
}

// generateAIPosterWithOpenAI generates poster using OpenAI SDK
func (pg *PosterGenerator) generateAIPosterWithOpenAI(ctx context.Context, metadata *models.EnrichedMetadata, canonicalFolder string) (string, error) {
	log.Printf("[POSTER] ===== OpenAI POSTER GENERATION START =====")
	log.Printf("[POSTER] title=%q original_title=%q year=%d", metadata.Title, metadata.OriginalTitle, metadata.Year)
	log.Printf("[POSTER] tmdb_poster_url=%q", metadata.PosterURL)
	log.Printf("[POSTER] canonicalFolder=%s storageMode=%s", canonicalFolder, pg.storageMode)

	originalPosterURL := metadata.PosterURL

	// Call OpenAI — errors must not be swallowed
	result, err := pg.openAIClient.GeneratePoster(ctx, metadata, originalPosterURL)
	if err != nil {
		log.Printf("[POSTER] ===== OpenAI POSTER GENERATION FAILED =====")
		log.Printf("[POSTER] Error: %v", err)
		return "", fmt.Errorf("OpenAI poster generation failed: %w", err)
	}

	if result == nil || result.ImagePath == "" {
		log.Printf("[POSTER] ERROR: OpenAI returned nil result or empty ImagePath")
		return "", fmt.Errorf("OpenAI returned empty result")
	}

	log.Printf("[POSTER] OpenAI returned image: path=%s url=%s", result.ImagePath, result.ImageURL)

	imageData, err := os.ReadFile(result.ImagePath)
	if err != nil {
		log.Printf("[POSTER] ERROR: failed to read generated image file %s: %v", result.ImagePath, err)
		return "", fmt.Errorf("failed to read generated image: %w", err)
	}
	log.Printf("[POSTER] Read generated image: %d bytes", len(imageData))
	os.Remove(result.ImagePath) // clean up temp file

	ext := ".png"
	if strings.Contains(result.ImagePath, ".jpg") || strings.Contains(result.ImagePath, ".jpeg") {
		ext = ".jpg"
	}

	if pg.storageMode == "dev" {
		savedURL, saveErr := pg.savePosterLocally(imageData, canonicalFolder, "image/jpeg")
		if saveErr != nil {
			log.Printf("[POSTER] ERROR: failed to save poster locally: %v", saveErr)
			return "", saveErr
		}
		log.Printf("[POSTER] Poster saved locally → URL: %s", savedURL)
		return savedURL, nil
	}

	// Production: Upload to B2
	filename := fmt.Sprintf("movies/%s/poster%s", canonicalFolder, ext)
	log.Printf("[POSTER] Uploading poster to B2: %s", filename)
	publicURL, err := pg.storage.UploadData(filename, imageData, "image/jpeg")
	if err != nil {
		log.Printf("[POSTER] ERROR: B2 upload failed: %v", err)
		return "", fmt.Errorf("failed to upload poster to B2: %w", err)
	}

	log.Printf("[POSTER] ===== OpenAI POSTER GENERATION COMPLETE ===== url=%s", publicURL)
	return publicURL, nil
}

// generateAIPosterLegacy generates poster using the legacy AI endpoint
func (pg *PosterGenerator) generateAIPosterLegacy(ctx context.Context, metadata *models.EnrichedMetadata, canonicalFolder string) (string, error) {
	// Determine the title to use for poster
	// Priority: title_uz (Uzbek) > original_title (if has mapping) > title (English)
	displayTitle := metadata.Title
	if metadata.TitleUz != "" {
		displayTitle = metadata.TitleUz
		log.Printf("[POSTER] Using Uzbek title for poster: %s", displayTitle)
	} else {
		// Try to find Uzbek title from mapping
		if uzTitle := LocalizeTitle(metadata.OriginalTitle); uzTitle != "" {
			displayTitle = uzTitle
			log.Printf("[POSTER] Found mapped Uzbek title for poster: %s", displayTitle)
		} else if metadata.OriginalTitle != "" && metadata.OriginalTitle != metadata.Title {
			displayTitle = metadata.OriginalTitle
			log.Printf("[POSTER] Using original title for poster: %s", displayTitle)
		}
	}

	// Prepare prompt for Uzbek localization with premium FilmoraUz branding
	// Branding: Filmora=white, Uz=orange (#FF7A00), .net=white
	prompt := fmt.Sprintf("Movie poster for '%s'. Professional movie poster style, high quality, cinematic composition. Preserve original composition, facial accuracy, and cinematic lighting. Add FilmoraUz branding at bottom center or bottom-right with clean spacing. Use 'Filmora' in white, 'Uz' in orange (#FF7A00), '.net' in white. Add subtle shadow for readability. Do not place branding over faces or key title area. Premium streaming platform quality.", displayTitle)

	// Create request for AI generation
	reqBody := map[string]interface{}{
		"prompt":     prompt,
		"width":      600,
		"height":     900,
		"num_images": 1,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pg.aiEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pg.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - assume it returns image URL directly
	var aiResp struct {
		ImageURL string `json:"image_url"`
		URL      string `json:"url"`
		Images   []struct {
			URL string `json:"url"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		// If not JSON, might be raw image - need to download and upload
		return "", fmt.Errorf("failed to parse AI response: %w", err)
	}

	imageURL := aiResp.ImageURL
	if imageURL == "" && len(aiResp.Images) > 0 {
		imageURL = aiResp.Images[0].URL
	}

	if imageURL == "" {
		return "", fmt.Errorf("no image URL in AI response")
	}

	// Download the generated image and upload to storage using CANONICAL folder
	return pg.uploadGeneratedPoster(ctx, imageURL, canonicalFolder)
}

// uploadGeneratedPoster downloads an image and uploads to storage
// CRITICAL: Uses canonicalFolder to ensure poster is saved in the SAME folder as HLS files
func (pg *PosterGenerator) uploadGeneratedPoster(ctx context.Context, imageURL, canonicalFolder string) (string, error) {
	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Determine file extension from content type
	contentType := resp.Header.Get("Content-Type")
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	log.Printf("[POSTER] uploadGeneratedPoster: using canonicalFolder=%s", canonicalFolder)

	// Check storage mode
	if pg.storageMode == "dev" {
		// DEVELOPMENT MODE: Save locally to uploads/movies/<canonicalFolder>/poster.jpg
		// CRITICAL: Use the SAME canonicalFolder that was used for HLS files
		return pg.savePosterLocally(imageData, canonicalFolder, contentType)
	}

	// PRODUCTION MODE: Upload to B2 using canonical folder structure
	// CRITICAL: Use canonicalFolder to match HLS file storage structure
	filename := fmt.Sprintf("movies/%s/poster%s", canonicalFolder, ext)
	publicURL, err := pg.storage.UploadData(filename, imageData, contentType)
	if err != nil {
		return "", fmt.Errorf("failed to upload poster to B2: %w", err)
	}

	log.Printf("[POSTER] Uploaded generated poster to B2: %s (canonicalFolder=%s)", publicURL, canonicalFolder)
	return publicURL, nil
}

// savePosterLocally saves poster to local filesystem
func (pg *PosterGenerator) savePosterLocally(imageData []byte, canonicalFolder, contentType string) (string, error) {
	// Determine extension from content type
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	// Get base directory
	baseDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create uploads/movies/<canonicalFolder> directory
	targetDir := filepath.Join(baseDir, "uploads", "movies", canonicalFolder)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create poster directory: %w", err)
	}

	// Write poster file
	posterPath := filepath.Join(targetDir, fmt.Sprintf("poster%s", ext))
	if err := os.WriteFile(posterPath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write poster file: %w", err)
	}

	// Return FULL URL with BASE_URL prefix (e.g., http://localhost:8080/stream/...)
	// This ensures database stores complete URLs, not relative paths
	relativePath := fmt.Sprintf("/stream/%s/poster%s", canonicalFolder, ext)
	fullURL := pg.baseURL + relativePath
	log.Printf("[POSTER] Saved poster locally: %s (full URL: %s)", relativePath, fullURL)
	return fullURL, nil
}

// FindOriginalPoster attempts to find a better poster URL
// Now uses TMDB as primary source for clean posters
func (pg *PosterGenerator) FindOriginalPoster(ctx context.Context, title string, year int) (string, error) {
	// This is now handled by the metadata enricher which uses TMDB
	// The enricher sets PosterURL to TMDB poster if available
	log.Printf("[POSTER] FindOriginalPoster called for title=%s, year=%d (handled by enricher)", title, year)
	return "", nil
}

// GenerateBackdrop generates an Uzbek-localized backdrop or finds original
// Returns the final backdrop URL to use
// CRITICAL: canonicalFolder must be passed to ensure backdrop is saved in the SAME folder as HLS files
func (pg *PosterGenerator) GenerateBackdrop(ctx context.Context, enrichedMetadata *models.EnrichedMetadata, canonicalFolder string) (*BackdropResult, error) {
	result := &BackdropResult{}

	log.Printf("[BACKDROP] GenerateBackdrop: using canonicalFolder=%s", canonicalFolder)

	// Step 1: Try to find original backdrop from enriched metadata
	originalBackdropURL := enrichedMetadata.BackdropURL
	if originalBackdropURL != "" {
		// CRITICAL: Check if backdrop is from watermarked source
		if pg.isWatermarkedPoster(originalBackdropURL) {
			log.Printf("[BACKDROP] WARNING: Backdrop from watermarked source detected, REJECTING: %s", originalBackdropURL)
			originalBackdropURL = ""
		} else {
			log.Printf("[BACKDROP] Found CLEAN original backdrop: %s", originalBackdropURL)

			// Try to download and validate the original backdrop
			valid, err := pg.validatePosterURL(ctx, originalBackdropURL)
			if err != nil || !valid {
				log.Printf("[BACKDROP] Original backdrop not accessible, will try AI generation")
				originalBackdropURL = ""
			} else {
				result.OriginalBackdropURL = originalBackdropURL
				log.Printf("[BACKDROP] CLEAN backdrop validated successfully")
			}
		}
	}

	// Step 2: Use original URL — prefer the validated one, fall back to raw enriched URL.
	fallbackURL := result.OriginalBackdropURL
	if fallbackURL == "" {
		fallbackURL = enrichedMetadata.BackdropURL
		if fallbackURL != "" {
			log.Printf("[BACKDROP] Validation failed earlier but raw URL available — using as fallback: %s", fallbackURL)
		}
	}

	if fallbackURL != "" {
		result.GeneratedBackdropURL = fallbackURL
		result.OriginalBackdropURL = fallbackURL
		result.BackdropGenerated = false // logo overlay in pipeline.go will set backdropGenerated via UploadBrandedBackdrop
		log.Printf("[BACKDROP] TMDB fallback backdrop URL returned for logo overlay: %s", fallbackURL)
		return result, nil
	}

	// Step 4: Truly nothing available
	log.Printf("[BACKDROP] WARNING: No backdrop URL available at all for: %s", enrichedMetadata.Title)
	return result, fmt.Errorf("no backdrop available")
}

// generateAIBackdropWithOpenAI generates backdrop using OpenAI SDK
func (pg *PosterGenerator) generateAIBackdropWithOpenAI(ctx context.Context, metadata *models.EnrichedMetadata, canonicalFolder string) (string, error) {
	log.Printf("[BACKDROP] ===== OpenAI BACKDROP GENERATION START =====")
	log.Printf("[BACKDROP] title=%q year=%d tmdb_backdrop_url=%q", metadata.Title, metadata.Year, metadata.BackdropURL)
	log.Printf("[BACKDROP] canonicalFolder=%s storageMode=%s", canonicalFolder, pg.storageMode)

	result, err := pg.openAIClient.GenerateBackdrop(ctx, metadata, metadata.BackdropURL)
	if err != nil {
		log.Printf("[BACKDROP] ===== OpenAI BACKDROP GENERATION FAILED =====")
		log.Printf("[BACKDROP] Error: %v", err)
		return "", fmt.Errorf("OpenAI backdrop generation failed: %w", err)
	}
	if result == nil || result.ImagePath == "" {
		log.Printf("[BACKDROP] ERROR: OpenAI returned nil result or empty ImagePath")
		return "", fmt.Errorf("OpenAI returned empty result for backdrop")
	}

	log.Printf("[BACKDROP] OpenAI returned image: path=%s url=%s", result.ImagePath, result.ImageURL)

	imageData, err := os.ReadFile(result.ImagePath)
	if err != nil {
		log.Printf("[BACKDROP] ERROR: failed to read generated image file %s: %v", result.ImagePath, err)
		return "", fmt.Errorf("failed to read generated backdrop image: %w", err)
	}
	log.Printf("[BACKDROP] Read generated image: %d bytes", len(imageData))
	os.Remove(result.ImagePath) // clean up temp file

	ext := ".png"
	if strings.Contains(result.ImagePath, ".jpg") || strings.Contains(result.ImagePath, ".jpeg") {
		ext = ".jpg"
	}

	if pg.storageMode == "dev" {
		savedURL, saveErr := pg.saveBackdropLocally(imageData, canonicalFolder, "image/jpeg")
		if saveErr != nil {
			log.Printf("[BACKDROP] ERROR: failed to save backdrop locally: %v", saveErr)
			return "", saveErr
		}
		log.Printf("[BACKDROP] Backdrop saved locally → URL: %s", savedURL)
		return savedURL, nil
	}

	filename := fmt.Sprintf("movies/%s/backdrop%s", canonicalFolder, ext)
	log.Printf("[BACKDROP] Uploading backdrop to B2: %s", filename)
	publicURL, err := pg.storage.UploadData(filename, imageData, "image/jpeg")
	if err != nil {
		log.Printf("[BACKDROP] ERROR: B2 upload failed: %v", err)
		return "", fmt.Errorf("failed to upload backdrop to B2: %w", err)
	}

	log.Printf("[BACKDROP] ===== OpenAI BACKDROP GENERATION COMPLETE ===== url=%s", publicURL)
	return publicURL, nil
}

// generateAIBackdrop generates an Uzbek-localized backdrop using AI
// CRITICAL: canonicalFolder must be passed to ensure backdrop is saved in the SAME folder as HLS files
func (pg *PosterGenerator) generateAIBackdrop(ctx context.Context, metadata *models.EnrichedMetadata, canonicalFolder string) (string, error) {
	if metadata == nil || metadata.Title == "" {
		return "", fmt.Errorf("metadata is empty")
	}

	log.Printf("[BACKDROP] generateAIBackdrop: using canonicalFolder=%s", canonicalFolder)

	// Priority 1: Use OpenAI client if configured
	if pg.openAIClient != nil && pg.openAIClient.IsConfigured() {
		log.Printf("[BACKDROP] Using OpenAI client for backdrop generation")
		return pg.generateAIBackdropWithOpenAI(ctx, metadata, canonicalFolder)
	}

	// Priority 2: Use legacy AI endpoint
	if pg.aiEndpoint == "" {
		return "", fmt.Errorf("no AI generation configured")
	}

	// Determine the title to use for backdrop
	// Priority: title_uz (Uzbek) > original_title (if has mapping) > title (English)
	displayTitle := metadata.Title
	if metadata.TitleUz != "" {
		displayTitle = metadata.TitleUz
		log.Printf("[BACKDROP] Using Uzbek title for backdrop: %s", displayTitle)
	} else {
		// Try to find Uzbek title from mapping
		if uzTitle := LocalizeTitle(metadata.OriginalTitle); uzTitle != "" {
			displayTitle = uzTitle
			log.Printf("[BACKDROP] Found mapped Uzbek title for backdrop: %s", displayTitle)
		} else if metadata.OriginalTitle != "" && metadata.OriginalTitle != metadata.Title {
			displayTitle = metadata.OriginalTitle
			log.Printf("[BACKDROP] Using original title for backdrop: %s", displayTitle)
		}
	}

	// Prepare prompt for Uzbek localization with premium FilmoraUz branding
	// Branding: Filmora=white, Uz=orange (#FF7A00), .net=white
	prompt := fmt.Sprintf("Movie backdrop for '%s'. Professional movie backdrop style, high quality, cinematic wide composition (16:9 aspect ratio). Preserve original color grading and wide cinematic composition. No cropping of key elements. Add FilmoraUz branding at bottom right with clean spacing. Use 'Filmora' in white, 'Uz' in orange (#FF7A00), '.net' in white. Add subtle shadow for readability. Keep backdrop clean - do not force unnecessary text. If no visible text on original, just apply clean branding. Premium streaming platform quality.", displayTitle)

	// Create request for AI generation
	reqBody := map[string]interface{}{
		"prompt":     prompt,
		"width":      1920,
		"height":     1080,
		"num_images": 1,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", pg.aiEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := pg.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("AI request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("AI returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - assume it returns image URL directly
	var aiResp struct {
		ImageURL string `json:"image_url"`
		URL      string `json:"url"`
		Images   []struct {
			URL string `json:"url"`
		} `json:"images"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&aiResp); err != nil {
		// If not JSON, might be raw image - need to download and upload
		return "", fmt.Errorf("failed to parse AI response: %w", err)
	}

	imageURL := aiResp.ImageURL
	if imageURL == "" && len(aiResp.Images) > 0 {
		imageURL = aiResp.Images[0].URL
	}

	if imageURL == "" {
		return "", fmt.Errorf("no image URL in AI response")
	}

	// Download the generated image and upload to storage using CANONICAL folder
	return pg.uploadGeneratedBackdrop(ctx, imageURL, canonicalFolder)
}

// uploadGeneratedBackdrop downloads an image and uploads to storage
// CRITICAL: Uses canonicalFolder to ensure backdrop is saved in the SAME folder as HLS files
func (pg *PosterGenerator) uploadGeneratedBackdrop(ctx context.Context, imageURL, canonicalFolder string) (string, error) {
	// Download the image
	resp, err := http.Get(imageURL)
	if err != nil {
		return "", fmt.Errorf("failed to download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	// Read image data
	imageData, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read image data: %w", err)
	}

	// Determine file extension from content type
	contentType := resp.Header.Get("Content-Type")
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	log.Printf("[BACKDROP] uploadGeneratedBackdrop: using canonicalFolder=%s", canonicalFolder)

	// Check storage mode
	if pg.storageMode == "dev" {
		// DEVELOPMENT MODE: Save locally to uploads/movies/<canonicalFolder>/backdrop.jpg
		// CRITICAL: Use the SAME canonicalFolder that was used for HLS files
		return pg.saveBackdropLocally(imageData, canonicalFolder, contentType)
	}

	// PRODUCTION MODE: Upload to B2 using canonical folder structure
	// CRITICAL: Use canonicalFolder to match HLS file storage structure
	filename := fmt.Sprintf("movies/%s/backdrop%s", canonicalFolder, ext)
	publicURL, err := pg.storage.UploadData(filename, imageData, contentType)
	if err != nil {
		return "", fmt.Errorf("failed to upload backdrop to B2: %w", err)
	}

	log.Printf("[BACKDROP] Uploaded generated backdrop to B2: %s (canonicalFolder=%s)", publicURL, canonicalFolder)
	return publicURL, nil
}

// saveBackdropLocally saves backdrop to local filesystem
func (pg *PosterGenerator) saveBackdropLocally(imageData []byte, slug, contentType string) (string, error) {
	// Determine extension from content type
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	// Get base directory
	baseDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %w", err)
	}

	// Create uploads/movies/<slug> directory
	targetDir := filepath.Join(baseDir, "uploads", "movies", slug)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create backdrop directory: %w", err)
	}

	// Write backdrop file
	backdropPath := filepath.Join(targetDir, fmt.Sprintf("backdrop%s", ext))
	if err := os.WriteFile(backdropPath, imageData, 0644); err != nil {
		return "", fmt.Errorf("failed to write backdrop file: %w", err)
	}

	// Return FULL URL with BASE_URL prefix (e.g., http://localhost:8080/stream/...)
	// This ensures database stores complete URLs, not relative paths
	relativePath := fmt.Sprintf("/stream/%s/backdrop%s", slug, ext)
	fullURL := pg.baseURL + relativePath
	log.Printf("[BACKDROP] Saved backdrop locally: %s (full URL: %s)", relativePath, fullURL)
	return fullURL, nil
}

// AddLogoOverlayForBackdrop adds FilmoraUz.net logo to a backdrop image
// Uses pure Go for image processing (NO ImageMagick dependency)
// Returns the path to the branded backdrop or original if logo fails
func (pg *PosterGenerator) AddLogoOverlayForBackdrop(ctx context.Context, backdropPath string, slug string) (string, error) {
	// Use pure Go implementation - no ImageMagick required
	return pg.addLogoOverlayGoForBackdrop(ctx, backdropPath, slug)
}

// addLogoOverlayGoForBackdrop adds premium FilmoraUz.net branding to backdrop using pure Go
// Creates a Netflix-level polished backdrop with:
// - Elegant FilmoraUz.net branding in bottom-right
// - Subtle shadow for depth
func (pg *PosterGenerator) addLogoOverlayGoForBackdrop(ctx context.Context, backdropPath string, slug string) (string, error) {
	log.Printf("[BACKDROP] Starting premium backdrop generation (Go-based, no ImageMagick)")
	log.Printf("[BACKDROP] Adding FilmoraUz branding to CLEAN backdrop (no watermarks)")

	// Read the input image
	inputFile, err := os.Open(backdropPath)
	if err != nil {
		log.Printf("[BACKDROP] Failed to open backdrop image: %v", err)
		return backdropPath, nil
	}
	defer inputFile.Close()

	// Decode the image
	img, format, err := image.Decode(inputFile)
	if err != nil {
		log.Printf("[BACKDROP] Failed to decode backdrop image: %v", err)
		return backdropPath, nil
	}

	imgBounds := img.Bounds()
	log.Printf("[BACKDROP] Decoded backdrop: format=%s, size=%dx%d", format, imgBounds.Dx(), imgBounds.Dy())

	// Convert to RGBA for manipulation
	rgba := image.NewRGBA(imgBounds)
	draw.Draw(rgba, rgba.Bounds(), img, imgBounds.Min, draw.Src)

	// Add elegant FilmoraUz.net branding in bottom-right
	log.Printf("[BACKDROP] Adding FilmoraUz.net branding...")
	if err := addPremiumBrandingForBackdrop(rgba); err != nil {
		log.Printf("[BACKDROP] Branding failed: %v, continuing without branding", err)
	} else {
		log.Printf("[BACKDROP] Premium branding applied successfully")
	}

	// Determine file extension and output path
	ext := ".jpg"
	if strings.ToLower(format) == "png" {
		ext = ".png"
	}

	// Create output path
	var outputPath string
	if pg.storageMode == "dev" {
		baseDir, _ := os.Getwd()
		targetDir := filepath.Join(baseDir, "uploads", "movies", slug)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Printf("[BACKDROP] Failed to create directory: %v", err)
			return backdropPath, nil
		}
		outputPath = filepath.Join(targetDir, fmt.Sprintf("backdrop_branded%s", ext))
	} else {
		outputPath = filepath.Join(pg.tempDir, fmt.Sprintf("%s_backdrop_branded%s", slug, ext))
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Printf("[BACKDROP] Failed to create output file: %v", err)
		return backdropPath, nil
	}
	defer outputFile.Close()

	// Encode and save
	if strings.ToLower(format) == "png" {
		err = png.Encode(outputFile, rgba)
	} else {
		err = jpeg.Encode(outputFile, rgba, &jpeg.Options{Quality: 92})
	}

	if err != nil {
		log.Printf("[BACKDROP] Failed to encode branded backdrop: %v", err)
		return backdropPath, nil
	}

	log.Printf("[BACKDROP] Premium backdrop saved: %s", outputPath)
	return outputPath, nil
}

// addPremiumBrandingForBackdrop adds elegant FilmoraUz.net branding to backdrop
// Implements advanced branding spec with color split, responsive sizing, shadow/glow
func addPremiumBrandingForBackdrop(img *image.RGBA) error {
	// Try to load semi-bold font first, fallback to regular
	fontParsed, err := loadPremiumFont()
	if err != nil {
		log.Printf("[BACKDROP] Failed to load premium font: %v, using regular", err)
		fontParsed, err = opentype.Parse(goregular.TTF)
		if err != nil {
			log.Printf("[BACKDROP] Failed to parse regular font: %v", err)
			return err
		}
		log.Printf("[BACKDROP] Using regular font for backdrop branding")
	} else {
		log.Printf("[BACKDROP] Using premium semi-bold font for backdrop branding")
	}

	// Get image dimensions
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Responsive sizing: 10-14% of backdrop width
	targetWidth := float64(width) * 0.12 // 12% of width (middle of 10-14%)

	// Calculate font size based on target width
	// Approximate: each character is about 0.55 * fontSize wide
	// "FilmoraUz.net" = 13 characters
	approxCharWidth := 0.55
	brandingText := "FilmoraUz.net"
	charCount := float64(len(brandingText))
	fontSize := targetWidth / (charCount * approxCharWidth)

	// Clamp font size to reasonable range
	if fontSize < 10 {
		fontSize = 10
	}
	if fontSize > 48 {
		fontSize = 48
	}

	log.Printf("[BACKDROP] Branding: width=%d, height=%d, targetWidth=%.1f, fontSize=%.1f", width, height, targetWidth, fontSize)

	face, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("[BACKDROP] Failed to create font face: %v", err)
		return err
	}
	defer face.Close()

	// Positioning: bottom right, 4-6% margin from right and bottom
	marginBottom := float64(height) * 0.05 // 5% from bottom
	if marginBottom < 15 {
		marginBottom = 15
	}
	if marginBottom > 60 {
		marginBottom = 60
	}

	marginRight := float64(width) * 0.05 // 5% from right
	if marginRight < 15 {
		marginRight = 15
	}
	if marginRight > 60 {
		marginRight = 60
	}

	// Calculate total text width for right-alignment
	textWidth := int(charCount * fontSize * approxCharWidth)
	x := width - textWidth - int(marginRight)
	y := height - int(marginBottom)

	// Analyze local brightness for contrast adaptation
	localBrightness := analyzeLocalBrightness(img, x, y, textWidth, int(fontSize*1.5))
	log.Printf("[BACKDROP] Local brightness at branding position: %.2f", localBrightness)

	// Apply shadow and glow
	shadowBlur := fontSize * 0.15 // 15% of font size
	if shadowBlur < 12 {
		shadowBlur = 12
	}
	if shadowBlur > 18 {
		shadowBlur = 18
	}

	glowBlur := fontSize * 0.10 // 10% of font size
	if glowBlur < 8 {
		glowBlur = 8
	}
	if glowBlur > 12 {
		glowBlur = 12
	}

	// Draw glow (very subtle)
	glowColor := color.RGBA{R: 255, G: 255, B: 255, A: 20} // rgba(255,255,255,0.08)
	drawTextWithBlur(img, brandingText, face, x, y, glowColor, int(glowBlur))

	// Draw shadow
	shadowOffset := int(fontSize * 0.03) // 3% of font size
	if shadowOffset < 2 {
		shadowOffset = 2
	}
	if shadowOffset > 4 {
		shadowOffset = 4
	}
	shadowColor := color.RGBA{R: 0, G: 0, B: 0, A: 153} // rgba(0,0,0,0.6)
	drawTextWithBlur(img, brandingText, face, x+shadowOffset, y+shadowOffset, shadowColor, int(shadowBlur))

	// If background is bright, add dark stroke for readability
	if localBrightness > 0.6 {
		log.Printf("[BACKDROP] Bright background detected, adding dark stroke for readability")
		strokeColor := color.RGBA{R: 0, G: 0, B: 0, A: 102} // rgba(0,0,0,0.4)
		strokeWidth := 1
		if fontSize > 25 {
			strokeWidth = 2
		}
		drawTextWithStroke(img, brandingText, face, x, y, strokeColor, strokeWidth)
	}

	// Draw color-split branding
	// "Filmora" = white (#FFFFFF)
	// "Uz" = orange (#FF7A00)
	// ".net" = dimmed white/gray (rgba(255,255,255,0.6))

	// Calculate positions for each part
	filmoraText := "Filmora"
	uzText := "Uz"
	netText := ".net"

	filmoraWidth := int(float64(len(filmoraText)) * fontSize * approxCharWidth)
	uzWidth := int(float64(len(uzText)) * fontSize * approxCharWidth)

	// Draw "Filmora" in white
	filmoraColor := color.RGBA{R: 255, G: 255, B: 255, A: 245} // #FFFFFF with 96% opacity
	drawTextWithFace(img, filmoraText, face, x, y, filmoraColor)

	// Draw "Uz" in orange
	uzColor := color.RGBA{R: 255, G: 122, B: 0, A: 245} // #FF7A00 with 96% opacity
	drawTextWithFace(img, uzText, face, x+filmoraWidth, y, uzColor)

	// Draw ".net" in dimmed white
	netColor := color.RGBA{R: 255, G: 255, B: 255, A: 153} // rgba(255,255,255,0.6)
	drawTextWithFace(img, netText, face, x+filmoraWidth+uzWidth, y, netColor)

	log.Printf("[BACKDROP] Premium branding applied: Filmora(white) + Uz(orange) + .net(dimmed)")
	return nil
}

// UploadBrandedBackdrop uploads the branded backdrop to storage
func (pg *PosterGenerator) UploadBrandedBackdrop(ctx context.Context, brandedPath, slug string) (string, error) {
	// Read the branded backdrop
	data, err := os.ReadFile(brandedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read branded backdrop: %w", err)
	}

	// Determine content type
	contentType := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(brandedPath), ".png") {
		contentType = "image/png"
	}

	// Determine extension
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	// Check storage mode
	if pg.storageMode == "dev" {
		// DEVELOPMENT: Return FULL URL with BASE_URL prefix
		// This ensures database stores complete URLs, not relative paths
		relativePath := fmt.Sprintf("/stream/%s/backdrop_branded%s", slug, ext)
		fullURL := pg.baseURL + relativePath
		log.Printf("[BACKDROP] Branded backdrop saved locally: %s (full URL: %s)", relativePath, fullURL)
		return fullURL, nil
	}

	// PRODUCTION: Upload to B2 using canonical folder structure
	// CRITICAL: Use canonical folder to match HLS file storage structure
	filename := fmt.Sprintf("movies/%s/backdrop_branded%s", slug, ext)
	publicURL, err := pg.storage.UploadData(filename, data, contentType)
	if err != nil {
		return "", fmt.Errorf("failed to upload branded backdrop to B2: %w", err)
	}

	log.Printf("[BACKDROP] Branded backdrop uploaded to B2: %s (canonicalFolder=%s)", publicURL, slug)
	return publicURL, nil
}

// AddLogoOverlay adds FilmoraUz.net logo to a poster image
// Uses pure Go for image processing (NO ImageMagick dependency)
// Returns the path to the branded poster or original if logo fails
func (pg *PosterGenerator) AddLogoOverlay(ctx context.Context, posterPath string, slug string) (string, error) {
	// Use pure Go implementation - no ImageMagick required
	return pg.addLogoOverlayGo(ctx, posterPath, slug)
}

// addLogoOverlayGo adds premium FilmoraUz.net branding using pure Go
// Creates a Netflix-level polished poster with:
// - Subtle cinematic bottom gradient
// - Elegant FilmoraUz.net branding
// - Optional soft vignette for depth
func (pg *PosterGenerator) addLogoOverlayGo(ctx context.Context, posterPath string, slug string) (string, error) {
	log.Printf("[POSTER] Starting premium poster generation (Go-based, no ImageMagick)")
	log.Printf("[POSTER] Adding FilmoraUz branding to CLEAN poster (no watermarks)")

	// Read the input image
	inputFile, err := os.Open(posterPath)
	if err != nil {
		log.Printf("[POSTER] Failed to open poster image: %v", err)
		return posterPath, nil
	}
	defer inputFile.Close()

	// Decode the image
	img, format, err := image.Decode(inputFile)
	if err != nil {
		log.Printf("[POSTER] Failed to decode poster image: %v", err)
		return posterPath, nil
	}

	imgBounds := img.Bounds()
	log.Printf("[POSTER] Decoded poster: format=%s, size=%dx%d", format, imgBounds.Dx(), imgBounds.Dy())

	// Convert to RGBA for manipulation
	rgba := image.NewRGBA(imgBounds)
	draw.Draw(rgba, rgba.Bounds(), img, imgBounds.Min, draw.Src)

	// Step 1: Apply subtle cinematic bottom gradient
	log.Printf("[POSTER] Applying cinematic bottom gradient...")
	applyBottomGradient(rgba)
	log.Printf("[POSTER] Bottom gradient applied successfully")

	// Step 2: Add elegant FilmoraUz.net branding
	log.Printf("[POSTER] Adding FilmoraUz.net branding...")
	if err := addPremiumBranding(rgba); err != nil {
		log.Printf("[POSTER] Branding failed: %v, continuing with gradient only", err)
	} else {
		log.Printf("[POSTER] Premium branding applied successfully")
	}

	// Determine file extension and output path
	ext := ".jpg"
	if strings.ToLower(format) == "png" {
		ext = ".png"
	}

	// Create output path
	var outputPath string
	if pg.storageMode == "dev" {
		baseDir, _ := os.Getwd()
		targetDir := filepath.Join(baseDir, "uploads", "movies", slug)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Printf("[POSTER] Failed to create directory: %v", err)
			return posterPath, nil
		}
		outputPath = filepath.Join(targetDir, fmt.Sprintf("poster_branded%s", ext))
	} else {
		outputPath = filepath.Join(pg.tempDir, fmt.Sprintf("%s_poster_branded%s", slug, ext))
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Printf("[POSTER] Failed to create output file: %v", err)
		return posterPath, nil
	}
	defer outputFile.Close()

	// Encode and save
	if strings.ToLower(format) == "png" {
		err = png.Encode(outputFile, rgba)
	} else {
		err = jpeg.Encode(outputFile, rgba, &jpeg.Options{Quality: 92})
	}

	if err != nil {
		log.Printf("[POSTER] Failed to encode branded poster: %v", err)
		return posterPath, nil
	}

	log.Printf("[POSTER] Premium poster saved: %s", outputPath)
	return outputPath, nil
}

// LocalizeAndBrandPoster downloads poster from TMDB, localizes it to Uzbek, adds premium branding, and saves to canonical folder
// This method handles the complete workflow:
// 1. Download poster from source URL
// 2. Add Uzbek title text overlay (localization)
// 3. Apply premium FilmoraUz.net branding
// 4. Save to the canonical movie folder
func (pg *PosterGenerator) LocalizeAndBrandPoster(ctx context.Context, posterPath string, canonicalFolder string, metadata *models.EnrichedMetadata) (string, error) {
	log.Printf("[POSTER] Starting localized poster generation with branding")
	log.Printf("[POSTER] Canonical folder: %s", canonicalFolder)
	log.Printf("[POSTER] Movie title: %s", metadata.Title)
	log.Printf("[POSTER] Original title: %s", metadata.OriginalTitle)

	// Read the input image
	inputFile, err := os.Open(posterPath)
	if err != nil {
		log.Printf("[POSTER] Failed to open poster image: %v", err)
		return posterPath, err
	}
	defer inputFile.Close()

	// Decode the image
	img, format, err := image.Decode(inputFile)
	if err != nil {
		log.Printf("[POSTER] Failed to decode poster image: %v", err)
		return posterPath, err
	}

	imgBounds := img.Bounds()
	log.Printf("[POSTER] Decoded poster: format=%s, size=%dx%d", format, imgBounds.Dx(), imgBounds.Dy())

	// Convert to RGBA for manipulation
	rgba := image.NewRGBA(imgBounds)
	draw.Draw(rgba, rgba.Bounds(), img, imgBounds.Min, draw.Src)

	// Step 1: Add Uzbek title localization overlay
	log.Printf("[POSTER] Adding Uzbek localization overlay...")
	if metadata != nil && metadata.Title != "" {
		if err := addUzbekTitleOverlay(rgba, metadata.Title, metadata.OriginalTitle); err != nil {
			log.Printf("[POSTER] Uzbek title overlay failed: %v (continuing with original poster)", err)
		} else {
			log.Printf("[POSTER] Uzbek title overlay applied successfully")
		}
	}

	// Step 2: Apply subtle cinematic bottom gradient
	log.Printf("[POSTER] Applying cinematic bottom gradient...")
	applyBottomGradient(rgba)

	// Step 3: Add premium FilmoraUz.net branding
	log.Printf("[POSTER] Adding premium FilmoraUz.net branding...")
	if err := addPremiumBrandingSemiBold(rgba); err != nil {
		log.Printf("[POSTER] Premium branding failed: %v, trying regular branding", err)
		// Fallback to regular branding
		if err := addPremiumBranding(rgba); err != nil {
			log.Printf("[POSTER] Regular branding also failed: %v", err)
		}
	} else {
		log.Printf("[POSTER] Premium semi-bold branding applied successfully")
	}

	// Step 4: Save to canonical folder
	ext := ".jpg"
	if strings.ToLower(format) == "png" {
		ext = ".png"
	}

	var outputPath string
	baseDir, _ := os.Getwd()

	if pg.storageMode == "dev" {
		// Save to canonical movie folder in uploads/movies/<canonicalFolder>/
		targetDir := filepath.Join(baseDir, "uploads", "movies", canonicalFolder)
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			log.Printf("[POSTER] Failed to create directory: %v", err)
			return posterPath, err
		}
		outputPath = filepath.Join(targetDir, fmt.Sprintf("poster_branded%s", ext))
	} else {
		// Save to temp dir for production upload
		outputPath = filepath.Join(pg.tempDir, fmt.Sprintf("%s_poster_branded%s", canonicalFolder, ext))
	}

	// Create output file
	outputFile, err := os.Create(outputPath)
	if err != nil {
		log.Printf("[POSTER] Failed to create output file: %v", err)
		return posterPath, err
	}
	defer outputFile.Close()

	// Encode and save
	if strings.ToLower(format) == "png" {
		err = png.Encode(outputFile, rgba)
	} else {
		err = jpeg.Encode(outputFile, rgba, &jpeg.Options{Quality: 92})
	}

	if err != nil {
		log.Printf("[POSTER] Failed to encode branded poster: %v", err)
		return posterPath, err
	}

	log.Printf("[POSTER] Localized and branded poster saved: %s", outputPath)
	return outputPath, nil
}

// addUzbekTitleOverlay adds Uzbek title text at the top of the poster for localization
func addUzbekTitleOverlay(img *image.RGBA, uzbekTitle, originalTitle string) error {
	log.Printf("[POSTER] Preparing Uzbek title overlay for: %s", uzbekTitle)

	// Try to load semi-bold font
	fontParsed, err := loadPremiumFont()
	if err != nil {
		log.Printf("[POSTER] Failed to load premium font: %v, using fallback", err)
		fontParsed, err = opentype.Parse(goregular.TTF)
		if err != nil {
			return fmt.Errorf("failed to parse fallback font: %w", err)
		}
	}

	// Get image dimensions
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Title positioning: top center, below the top edge
	// Use about 8-12% of poster height for title area
	marginTop := float64(height) * 0.05
	if marginTop < 30 {
		marginTop = 30
	}
	if marginTop > 80 {
		marginTop = 80
	}

	// Calculate font size (title should be readable but not dominate)
	// Target: about 5-8% of poster width for title text
	targetWidth := float64(width) * 0.08
	approxCharWidth := 0.55
	titleText := uzbekTitle
	if titleText == "" {
		titleText = originalTitle
	}
	charCount := float64(len(titleText))
	fontSize := targetWidth / (charCount * approxCharWidth)

	// Clamp font size
	if fontSize < 14 {
		fontSize = 14
	}
	if fontSize > 48 {
		fontSize = 48
	}

	face, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return fmt.Errorf("failed to create font face: %w", err)
	}
	defer face.Close()

	// Center horizontally
	textWidth := int(charCount * fontSize * approxCharWidth)
	x := (width - textWidth) / 2
	y := int(marginTop) + int(fontSize)

	// Create a gradient background for the title area for better readability
	titleBgHeight := int(fontSize * 1.8)
	titleBgTop := int(marginTop) - 10
	if titleBgTop < 0 {
		titleBgTop = 0
	}

	// Add subtle dark gradient behind text for better readability
	for py := titleBgTop; py < titleBgTop+titleBgHeight && py < height; py++ {
		progress := float64(py-titleBgTop) / float64(titleBgHeight)
		// Fade from dark at top to transparent at bottom
		alpha := uint8((1.0 - progress) * 140)
		for px := 0; px < width; px++ {
			if px >= x && px <= x+textWidth {
				oldPixel := img.RGBAAt(px, py)
				newR := blendChannel(0, oldPixel.R, alpha)
				newG := blendChannel(0, oldPixel.G, alpha)
				newB := blendChannel(0, oldPixel.B, alpha)
				newA := oldPixel.A
				img.SetRGBA(px, py, color.RGBA{R: newR, G: newG, B: newB, A: newA})
			}
		}
	}

	// Add subtle drop shadow for text
	shadowOffset := 2
	shadowColor := color.RGBA{R: 0, G: 0, B: 0, A: 128}
	drawTextWithStrokeShadow(img, titleText, face, x+shadowOffset, y+shadowOffset, shadowColor, 2)

	// Draw the Uzbek title in white with slight shadow
	textColor := color.RGBA{R: 255, G: 255, B: 255, A: 245}
	drawTextWithFace(img, titleText, face, x, y, textColor)

	log.Printf("[POSTER] Uzbek title overlay added: '%s' at (%d, %d) with fontSize=%.1f", titleText, x, y, fontSize)
	return nil
}

// loadPremiumFont loads a semi-bold font for premium branding
func loadPremiumFont() (*opentype.Font, error) {
	// Try Liberation Sans Bold first
	fontData, err := os.ReadFile(LiberationSansBoldPath)
	if err == nil {
		log.Printf("[POSTER] Using Liberation Sans Bold for premium branding")
		return opentype.Parse(fontData)
	}
	log.Printf("[POSTER] LiberationSansBold not found: %v", err)

	// Try Ubuntu Bold as fallback
	fontData, err = os.ReadFile(UbuntuBoldPath)
	if err == nil {
		log.Printf("[POSTER] Using Ubuntu Bold for premium branding")
		return opentype.Parse(fontData)
	}
	log.Printf("[POSTER] UbuntuBold not found: %v", err)

	return nil, fmt.Errorf("no premium fonts available")
}

// addPremiumBrandingSemiBold adds FilmoraUz.net branding with semi-bold font
func addPremiumBrandingSemiBold(img *image.RGBA) error {
	fontParsed, err := loadPremiumFont()
	if err != nil {
		log.Printf("[POSTER] Failed to load premium semi-bold font: %v", err)
		return err
	}

	// Get image dimensions
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Responsive sizing: 18-22% of poster width
	targetWidth := float64(width) * 0.20
	approxCharWidth := 0.55
	brandingText := "FilmoraUz.net"
	charCount := float64(len(brandingText))
	fontSize := targetWidth / (charCount * approxCharWidth)

	// Clamp font size
	if fontSize < 12 {
		fontSize = 12
	}
	if fontSize > 72 {
		fontSize = 72
	}

	log.Printf("[POSTER] Premium branding: width=%d, height=%d, fontSize=%.1f", width, height, fontSize)

	face, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		return err
	}
	defer face.Close()

	// Positioning: bottom center, 6% margin from bottom
	marginBottom := float64(height) * 0.06
	if marginBottom < 20 {
		marginBottom = 20
	}
	if marginBottom > 80 {
		marginBottom = 80
	}

	// Calculate text width for centering
	textWidth := int(charCount * fontSize * approxCharWidth)
	x := (width - textWidth) / 2
	y := height - int(marginBottom)

	// Analyze local brightness
	localBrightness := analyzeLocalBrightness(img, x, y, textWidth, int(fontSize*1.5))

	// Shadow settings
	shadowBlur := fontSize * 0.15
	if shadowBlur < 12 {
		shadowBlur = 12
	}
	if shadowBlur > 18 {
		shadowBlur = 18
	}

	shadowOffset := int(fontSize * 0.03)
	if shadowOffset < 2 {
		shadowOffset = 2
	}
	if shadowOffset > 4 {
		shadowOffset = 4
	}

	// Draw shadow
	shadowColor := color.RGBA{R: 0, G: 0, B: 0, A: 153}
	drawTextWithBlur(img, brandingText, face, x+shadowOffset, y+shadowOffset, shadowColor, int(shadowBlur))

	// Add stroke for bright backgrounds
	if localBrightness > 0.6 {
		strokeColor := color.RGBA{R: 0, G: 0, B: 0, A: 102}
		strokeWidth := 1
		if fontSize > 30 {
			strokeWidth = 2
		}
		drawTextWithStroke(img, brandingText, face, x, y, strokeColor, strokeWidth)
	}

	// Draw color-split branding
	filmoraText := "Filmora"
	uzText := "Uz"
	netText := ".net"

	filmoraWidth := int(float64(len(filmoraText)) * fontSize * approxCharWidth)
	uzWidth := int(float64(len(uzText)) * fontSize * approxCharWidth)

	// Draw "Filmora" in white
	filmoraColor := color.RGBA{R: 255, G: 255, B: 255, A: 245}
	drawTextWithFace(img, filmoraText, face, x, y, filmoraColor)

	// Draw "Uz" in orange (#FF7A00)
	uzColor := color.RGBA{R: 255, G: 122, B: 0, A: 245}
	drawTextWithFace(img, uzText, face, x+filmoraWidth, y, uzColor)

	// Draw ".net" in dimmed white
	netColor := color.RGBA{R: 255, G: 255, B: 255, A: 153}
	drawTextWithFace(img, netText, face, x+filmoraWidth+uzWidth, y, netColor)

	log.Printf("[POSTER] Premium semi-bold branding: Filmora(white) + Uz(#FF7A00 orange) + .net(dimmed)")
	return nil
}

// drawTextWithStrokeShadow draws text with both stroke and shadow
func drawTextWithStrokeShadow(img *image.RGBA, text string, face font.Face, x, y int, col color.RGBA, strokeWidth int) {
	// Draw stroke
	for i := -strokeWidth; i <= strokeWidth; i++ {
		for j := -strokeWidth; j <= strokeWidth; j++ {
			if i != 0 || j != 0 {
				drawTextWithFace(img, text, face, x+i, y+j, col)
			}
		}
	}
}

// applyBottomGradient applies a subtle cinematic gradient at the bottom of the poster
// This creates depth and improves text readability when branding is added
func applyBottomGradient(img *image.RGBA) {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	// Gradient settings
	gradientHeight := height / 4 // 25% of poster height
	if gradientHeight < 100 {
		gradientHeight = 100
	}
	if gradientHeight > 300 {
		gradientHeight = 300
	}

	// Start gradient from bottom
	startY := height - gradientHeight

	// Create gradient (black fading from bottom)
	for y := startY; y < height; y++ {
		// Calculate alpha based on position (0 at startY, 180 at bottom)
		progress := float64(y-startY) / float64(gradientHeight)
		alpha := uint8(progress * 180)

		// Apply gradient line
		for x := 0; x < width; x++ {
			oldPixel := img.RGBAAt(x, y)
			// Blend with black using gradient alpha
			newR := blendChannel(oldPixel.R, 0, alpha)
			newG := blendChannel(oldPixel.G, 0, alpha)
			newB := blendChannel(oldPixel.B, 0, alpha)
			newA := blendChannel(oldPixel.A, 255, alpha)

			img.SetRGBA(x, y, color.RGBA{
				R: newR, G: newG, B: newB, A: newA,
			})
		}
	}
}

// blendChannel blends two channel values with given alpha (0-255)
func blendChannel(top, bottom, alpha uint8) uint8 {
	return uint8((int(top)*(255-int(alpha)) + int(bottom)*int(alpha)) / 255)
}

// addPremiumBranding adds elegant FilmoraUz.net branding to the poster
// Implements advanced branding spec with color split, responsive sizing, shadow/glow
func addPremiumBranding(img *image.RGBA) error {
	// Try to load semi-bold font first, fallback to regular
	fontParsed, err := loadPremiumFont()
	if err != nil {
		log.Printf("[POSTER] Failed to load premium font: %v, using regular", err)
		fontParsed, err = opentype.Parse(goregular.TTF)
		if err != nil {
			log.Printf("[POSTER] Failed to parse regular font: %v", err)
			return err
		}
		log.Printf("[POSTER] Using regular font for poster branding")
	} else {
		log.Printf("[POSTER] Using premium semi-bold font for poster branding")
	}

	// Get image dimensions
	width := img.Bounds().Dx()
	height := img.Bounds().Dy()

	// Responsive sizing: 18-22% of poster width
	targetWidth := float64(width) * 0.20 // 20% of width (middle of 18-22%)

	// Calculate font size based on target width
	// Approximate: each character is about 0.55 * fontSize wide
	// "FilmoraUz.net" = 13 characters
	approxCharWidth := 0.55
	brandingText := "FilmoraUz.net"
	charCount := float64(len(brandingText))
	fontSize := targetWidth / (charCount * approxCharWidth)

	// Clamp font size to reasonable range
	if fontSize < 12 {
		fontSize = 12
	}
	if fontSize > 72 {
		fontSize = 72
	}

	log.Printf("[POSTER] Branding: width=%d, height=%d, targetWidth=%.1f, fontSize=%.1f", width, height, targetWidth, fontSize)

	face, err := opentype.NewFace(fontParsed, &opentype.FaceOptions{
		Size:    fontSize,
		DPI:     72,
		Hinting: font.HintingFull,
	})
	if err != nil {
		log.Printf("[POSTER] Failed to create font face: %v", err)
		return err
	}
	defer face.Close()

	// Positioning: bottom center, 5-7% margin from bottom
	marginBottom := float64(height) * 0.06 // 6% from bottom
	if marginBottom < 20 {
		marginBottom = 20
	}
	if marginBottom > 80 {
		marginBottom = 80
	}

	// Calculate total text width for centering
	textWidth := int(charCount * fontSize * approxCharWidth)
	x := (width - textWidth) / 2
	y := height - int(marginBottom)

	// Analyze local brightness for contrast adaptation
	localBrightness := analyzeLocalBrightness(img, x, y, textWidth, int(fontSize*1.5))
	log.Printf("[POSTER] Local brightness at branding position: %.2f", localBrightness)

	// Apply shadow and glow
	shadowBlur := fontSize * 0.15 // 15% of font size
	if shadowBlur < 12 {
		shadowBlur = 12
	}
	if shadowBlur > 18 {
		shadowBlur = 18
	}

	glowBlur := fontSize * 0.10 // 10% of font size
	if glowBlur < 8 {
		glowBlur = 8
	}
	if glowBlur > 12 {
		glowBlur = 12
	}

	// Draw glow (very subtle)
	glowColor := color.RGBA{R: 255, G: 255, B: 255, A: 20} // rgba(255,255,255,0.08)
	drawTextWithBlur(img, brandingText, face, x, y, glowColor, int(glowBlur))

	// Draw shadow
	shadowOffset := int(fontSize * 0.03) // 3% of font size
	if shadowOffset < 2 {
		shadowOffset = 2
	}
	if shadowOffset > 4 {
		shadowOffset = 4
	}
	shadowColor := color.RGBA{R: 0, G: 0, B: 0, A: 153} // rgba(0,0,0,0.6)
	drawTextWithBlur(img, brandingText, face, x+shadowOffset, y+shadowOffset, shadowColor, int(shadowBlur))

	// If background is bright, add dark stroke for readability
	if localBrightness > 0.6 {
		log.Printf("[POSTER] Bright background detected, adding dark stroke for readability")
		strokeColor := color.RGBA{R: 0, G: 0, B: 0, A: 102} // rgba(0,0,0,0.4)
		strokeWidth := 1
		if fontSize > 30 {
			strokeWidth = 2
		}
		drawTextWithStroke(img, brandingText, face, x, y, strokeColor, strokeWidth)
	}

	// Draw color-split branding
	// "Filmora" = white (#FFFFFF)
	// "Uz" = orange (#FF7A00)
	// ".net" = dimmed white/gray (rgba(255,255,255,0.6))

	// Calculate positions for each part
	filmoraText := "Filmora"
	uzText := "Uz"
	netText := ".net"

	filmoraWidth := int(float64(len(filmoraText)) * fontSize * approxCharWidth)
	uzWidth := int(float64(len(uzText)) * fontSize * approxCharWidth)

	// Draw "Filmora" in white
	filmoraColor := color.RGBA{R: 255, G: 255, B: 255, A: 245} // #FFFFFF with 96% opacity
	drawTextWithFace(img, filmoraText, face, x, y, filmoraColor)

	// Draw "Uz" in orange
	uzColor := color.RGBA{R: 255, G: 122, B: 0, A: 245} // #FF7A00 with 96% opacity
	drawTextWithFace(img, uzText, face, x+filmoraWidth, y, uzColor)

	// Draw ".net" in dimmed white
	netColor := color.RGBA{R: 255, G: 255, B: 255, A: 153} // rgba(255,255,255,0.6)
	drawTextWithFace(img, netText, face, x+filmoraWidth+uzWidth, y, netColor)

	log.Printf("[POSTER] Premium branding applied: Filmora(white) + Uz(orange) + .net(dimmed)")
	return nil
}

// drawTextWithFace draws text using a specific font face at position
func drawTextWithFace(img *image.RGBA, text string, face font.Face, x, y int, col color.RGBA) {
	point := fixed.Point26_6{
		X: fixed.Int26_6(x * 64),
		Y: fixed.Int26_6(y * 64),
	}

	// Create drawer
	drawer := &font.Drawer{
		Dst:  img,
		Src:  &image.Uniform{C: col},
		Face: face,
		Dot:  point,
	}

	drawer.DrawString(text)
}

// analyzeLocalBrightness analyzes the average brightness in a region of the image
// Returns value between 0.0 (dark) and 1.0 (bright)
func analyzeLocalBrightness(img *image.RGBA, x, y, width, height int) float64 {
	// Clamp region to image bounds
	bounds := img.Bounds()
	if x < 0 {
		x = 0
	}
	if y < 0 {
		y = 0
	}
	if x+width > bounds.Dx() {
		width = bounds.Dx() - x
	}
	if y+height > bounds.Dy() {
		height = bounds.Dy() - y
	}

	if width <= 0 || height <= 0 {
		return 0.5 // Default to medium brightness
	}

	var totalBrightness float64
	pixelCount := 0

	for py := y; py < y+height; py++ {
		for px := x; px < x+width; px++ {
			r, g, b, _ := img.At(px, py).RGBA()
			// Calculate perceived brightness (ITU-R BT.709)
			brightness := (0.2126*float64(r) + 0.7152*float64(g) + 0.0722*float64(b)) / 65535.0
			totalBrightness += brightness
			pixelCount++
		}
	}

	if pixelCount == 0 {
		return 0.5
	}

	return totalBrightness / float64(pixelCount)
}

// drawTextWithBlur draws text with a blur effect (simulated with multiple offset draws)
func drawTextWithBlur(img *image.RGBA, text string, face font.Face, x, y int, col color.RGBA, blurRadius int) {
	// Simulate blur by drawing multiple times with offsets
	for i := -blurRadius; i <= blurRadius; i += 2 {
		for j := -blurRadius; j <= blurRadius; j += 2 {
			// Reduce alpha for blur effect
			alpha := uint8(float64(col.A) * (1.0 - float64(i*i+j*j)/float64(blurRadius*blurRadius*2)))
			if alpha > 0 {
				blurColor := color.RGBA{R: col.R, G: col.G, B: col.B, A: alpha}
				drawTextWithFace(img, text, face, x+i, y+j, blurColor)
			}
		}
	}
}

// drawTextWithStroke draws text with a stroke outline
func drawTextWithStroke(img *image.RGBA, text string, face font.Face, x, y int, col color.RGBA, strokeWidth int) {
	// Draw stroke by drawing text at multiple offsets
	for i := -strokeWidth; i <= strokeWidth; i++ {
		for j := -strokeWidth; j <= strokeWidth; j++ {
			if i != 0 || j != 0 {
				drawTextWithFace(img, text, face, x+i, y+j, col)
			}
		}
	}
}

// UploadBrandedPoster uploads the branded poster to storage
func (pg *PosterGenerator) UploadBrandedPoster(ctx context.Context, brandedPath, slug string) (string, error) {
	// Read the branded poster
	data, err := os.ReadFile(brandedPath)
	if err != nil {
		return "", fmt.Errorf("failed to read branded poster: %w", err)
	}

	// Determine content type
	contentType := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(brandedPath), ".png") {
		contentType = "image/png"
	}

	// Determine extension
	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}

	// Check storage mode
	if pg.storageMode == "dev" {
		// DEVELOPMENT: Return FULL URL with BASE_URL prefix
		// This ensures database stores complete URLs, not relative paths
		relativePath := fmt.Sprintf("/stream/%s/poster_branded%s", slug, ext)
		fullURL := pg.baseURL + relativePath
		log.Printf("[POSTER] Branded poster saved locally: %s (full URL: %s)", relativePath, fullURL)
		return fullURL, nil
	}

	// PRODUCTION: Upload to B2 using canonical folder structure
	// CRITICAL: Use canonical folder to match HLS file storage structure
	filename := fmt.Sprintf("movies/%s/poster_branded%s", slug, ext)
	publicURL, err := pg.storage.UploadData(filename, data, contentType)
	if err != nil {
		return "", fmt.Errorf("failed to upload branded poster to B2: %w", err)
	}

	log.Printf("[POSTER] Branded poster uploaded to B2: %s (canonicalFolder=%s)", publicURL, slug)
	return publicURL, nil
}

// copyFile copies a file from src to dst
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}

// GeneratePosterWithAIForSlug creates a poster for a given slug title using local tools
// This is a fallback when no AI endpoint is configured
func (pg *PosterGenerator) GeneratePosterWithAIForSlug(title string) (string, error) {
	if title == "" {
		return "", fmt.Errorf("title is empty")
	}

	// Create a simple text-based poster using ImageMagick if available
	// This is a fallback implementation
	slug := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
	var cleanSlug strings.Builder
	for _, c := range slug {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			cleanSlug.WriteRune(c)
		}
	}

	// Check if ImageMagick is available
	_, err := exec.LookPath("convert")
	if err != nil {
		log.Printf("[POSTER] ImageMagick not available, skipping local generation")
		return "", fmt.Errorf("ImageMagick not available")
	}

	// Create a simple poster
	filename := filepath.Join(pg.tempDir, fmt.Sprintf("%s_poster.png", cleanSlug.String()))

	// Create a simple gradient image with title text
	cmd := exec.Command("convert",
		"-size", "600x900",
		"-gravity", "center",
		"-pointsize", "48",
		"-fill", "white",
		"-background", "#1a1a2e",
		"caption:"+title,
		filename,
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Printf("[POSTER] Failed to create poster: %s", string(output))
		return "", fmt.Errorf("failed to create poster: %w", err)
	}

	// Read and upload
	data, err := os.ReadFile(filename)
	if err != nil {
		return "", fmt.Errorf("failed to read generated poster: %w", err)
	}

	publicURL, err := pg.storage.UploadData(filepath.Base(filename), data, "image/png")
	if err != nil {
		return "", fmt.Errorf("failed to upload poster: %w", err)
	}

	// Cleanup
	os.Remove(filename)

	return publicURL, nil
}
