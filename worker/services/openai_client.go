package services

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/worker/models"
	openai "github.com/sashabaranov/go-openai"
)

// OpenAIClient handles OpenAI API calls for image generation and editing
type OpenAIClient struct {
	client      *openai.Client
	httpClient  *http.Client
	apiKey      string
	baseURL     string // Custom base URL for OpenAI-compatible APIs
	model       string // Model to use for image generation
	temperature float64
	timeout     time.Duration
	tempDir     string
}

// OpenAIConfig holds configuration for the OpenAI client
type OpenAIConfig struct {
	APIKey      string // OpenAI API key
	BaseURL     string // Custom base URL (for compatible APIs)
	Model       string // Model name (dall-e-3, dall-e-2, etc.)
	Temperature float64
	Timeout     time.Duration
	TempDir     string
}

// DefaultOpenAIConfig returns default configuration
func DefaultOpenAIConfig() *OpenAIConfig {
	return &OpenAIConfig{
		Model:       "dall-e-3",
		Temperature: 0.7,
		Timeout:     120 * time.Second,
		TempDir:     "./tmp",
	}
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(config *OpenAIConfig) (*OpenAIClient, error) {
	if config == nil {
		config = DefaultOpenAIConfig()
	}

	client := &OpenAIClient{
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
		apiKey:      config.APIKey,
		baseURL:     config.BaseURL,
		model:       config.Model,
		temperature: config.Temperature,
		timeout:     config.Timeout,
		tempDir:     config.TempDir,
	}

	// Initialize official OpenAI client if API key is provided
	if config.APIKey != "" {
		cfg := openai.DefaultConfig(config.APIKey)
		if config.BaseURL != "" {
			cfg.BaseURL = config.BaseURL
		}
		client.client = openai.NewClientWithConfig(cfg)
		log.Printf("[OPENAI] OpenAI client initialized with model: %s", config.Model)
	}

	// Create temp directory if it doesn't exist
	if err := os.MkdirAll(config.TempDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return client, nil
}

// IsConfigured returns true if the client is properly configured
func (c *OpenAIClient) IsConfigured() bool {
	return c.apiKey != "" && c.client != nil
}

// ImageGenerationRequest represents a request for image generation
type ImageGenerationRequest struct {
	Prompt        string // The main prompt for generation
	Model         string // Override default model
	Size          string // Image size (1024x1024, 1792x1024, etc.)
	Quality       string // Image quality (standard, hd)
	Style         string // Style (vivid, natural)
	OriginalImage []byte // Original image for editing (optional)
	MaskImage     []byte // Mask image for editing (optional)
}

// ImageGenerationResult represents the result of image generation
type ImageGenerationResult struct {
	ImagePath string // Local path to saved image
	ImageURL  string // URL of the generated image (if available)
	Base64    string // Base64 encoded image data
	Revised   string // Revised prompt (for DALL-E 3)
}

// GenerateBackdrop generates a localized movie backdrop (16:9) using OpenAI
func (c *OpenAIClient) GenerateBackdrop(ctx context.Context, metadata *models.EnrichedMetadata, originalBackdropURL string) (*ImageGenerationResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("OpenAI client not configured - API key required")
	}

	prompt := c.buildBackdropPrompt(metadata)

	log.Printf("[OPENAI] Generating backdrop for: %s", metadata.Title)
	log.Printf("[OPENAI] Backdrop prompt: %s", prompt)

	return c.generateImage(ctx, &ImageGenerationRequest{
		Prompt:  prompt,
		Model:   c.model,
		Size:    "1792x1024",
		Quality: "hd",
		Style:   "vivid",
	})
}

// buildBackdropPrompt builds a prompt for wide backdrop generation
func (c *OpenAIClient) buildBackdropPrompt(metadata *models.EnrichedMetadata) string {
	title := metadata.Title
	if metadata.TitleUz != "" {
		title = metadata.TitleUz
	} else if uzTitle := LocalizeTitle(metadata.OriginalTitle); uzTitle != "" {
		title = uzTitle
	}

	genreDesc := c.getGenreDescription(metadata.Genres)
	mood := c.getMoodAtmosphere(metadata)

	prompt := fmt.Sprintf(`
Create a premium cinematic movie backdrop for "%s" (%d).

Genre: %s
Mood: %s

Requirements:
- Wide cinematic 16:9 landscape composition
- Rich atmospheric background scene — no characters required
- High-end streaming platform quality (Netflix/Apple TV style)
- Dramatic lighting, deep colors, professional color grading
- No text or title overlays — clean image only
- Do NOT include any logos or watermarks

Visual Style:
- Panoramic establishing shot or key scene atmosphere
- Depth of field with bokeh background
- Professional cinematography framing
`, title, metadata.Year, genreDesc, mood)

	prompt = strings.Join(strings.Fields(prompt), " ")
	return prompt
}

// GeneratePoster generates a localized movie poster using OpenAI
// This creates a premium cinematic poster with Uzbek title and FilmoraUz branding
func (c *OpenAIClient) GeneratePoster(ctx context.Context, metadata *models.EnrichedMetadata, originalPosterURL string) (*ImageGenerationResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("OpenAI client not configured - API key required")
	}

	// Build the dynamic prompt based on movie metadata
	prompt := c.buildPosterPrompt(metadata)

	log.Printf("[OPENAI] Generating poster for: %s", metadata.Title)
	log.Printf("[OPENAI] Prompt: %s", prompt)

	// If we have an original poster, use image editing
	if originalPosterURL != "" && c.model != "dall-e-2" {
		return c.editPoster(ctx, prompt, originalPosterURL, metadata)
	}

	// Otherwise, generate from scratch
	return c.generateImage(ctx, &ImageGenerationRequest{
		Prompt:  prompt,
		Model:   c.model,
		Size:    "1024x1024",
		Quality: "hd",
		Style:   "vivid",
	})
}

// buildPosterPrompt builds a rich, dynamic prompt for poster generation
func (c *OpenAIClient) buildPosterPrompt(metadata *models.EnrichedMetadata) string {
	// Determine the title to use
	title := metadata.Title
	if metadata.TitleUz != "" {
		title = metadata.TitleUz
	} else if uzTitle := LocalizeTitle(metadata.OriginalTitle); uzTitle != "" {
		title = uzTitle
	}

	// Build genre description
	genreDesc := c.getGenreDescription(metadata.Genres)

	// Build mood/atmosphere based on genres and description
	mood := c.getMoodAtmosphere(metadata)

	// Build composition guidance
	composition := c.getCompositionGuidance(metadata)

	// Build branding instruction
	branding := c.getBrandingInstruction()

	// Construct the full prompt
	prompt := fmt.Sprintf(`
Create a premium cinematic movie poster for "%s" (%d).

Genre: %s
Mood: %s

Requirements:
%s

Visual Style:
- High-end streaming platform quality poster
- Dramatic cinematic composition with professional lighting
- Strong contrast and rich colors
- Clean, professional typography with large readable title
- Premium blockbuster aesthetic suitable for theatrical release

%s

Important:
- The title must be prominently displayed in Uzbek language
- Maintain facial accuracy and proper proportions
- Do not place branding over faces or key visual elements
- Output must look like an official localized theatrical poster
`, title, metadata.Year, genreDesc, mood, composition, branding)

	// Clean up whitespace
	prompt = strings.Join(strings.Fields(prompt), " ")
	prompt = strings.ReplaceAll(prompt, " , ", ", ")
	prompt = strings.ReplaceAll(prompt, " . ", ". ")

	return prompt
}

// getGenreDescription returns a description based on genres
func (c *OpenAIClient) getGenreDescription(genres []string) string {
	if len(genres) == 0 {
		return "action blockbuster"
	}

	var descParts []string
	seen := make(map[string]bool)

	for _, g := range genres {
		g = strings.ToLower(g)
		if seen[g] {
			continue
		}
		seen[g] = true

		switch g {
		case "action", "боевой", "jaction":
			descParts = append(descParts, "high-octane action")
		case "thriller", "триллер":
			descParts = append(descParts, "edge-of-seat thriller")
		case "horror", "ужасы":
			descParts = append(descParts, "terrifying horror")
		case "comedy", "комедия":
			descParts = append(descParts, "comedy")
		case "drama", "драма":
			descParts = append(descParts, "emotional drama")
		case "science fiction", "sci-fi", "фантастика", "fantastika":
			descParts = append(descParts, "epic science fiction")
		case "fantasy", "фэнтези":
			descParts = append(descParts, "magical fantasy")
		case "romance", "романтика", "romantika":
			descParts = append(descParts, "romantic")
		case "animation", "анимация", "animatsiya":
			descParts = append(descParts, "animated")
		case "adventure", "приключения", "sarguzasht":
			descParts = append(descParts, "adventure")
		case "war", "военный", "urush":
			descParts = append(descParts, "war epic")
		case "crime", "криминал", "jinoyat":
			descParts = append(descParts, "crime drama")
		default:
			descParts = append(descParts, g)
		}
	}

	if len(descParts) == 0 {
		return "entertainment"
	}
	return strings.Join(descParts, ", ")
}

// getMoodAtmosphere returns mood/atmosphere guidance based on genres and description
func (c *OpenAIClient) getMoodAtmosphere(metadata *models.EnrichedMetadata) string {
	desc := strings.ToLower(metadata.Description)

	// Check description for mood clues
	if strings.Contains(desc, "dark") || strings.Contains(desc, "qorong'i") || strings.Contains(desc, "qora") {
		return "dark, moody atmosphere with dramatic shadows"
	}
	if strings.Contains(desc, "light") || strings.Contains(desc, "comedy") || strings.Contains(desc, "funny") {
		return "bright, engaging atmosphere"
	}
	if strings.Contains(desc, "romantic") || strings.Contains(desc, "love") || strings.Contains(desc, "sevgi") {
		return "warm, romantic atmosphere with soft lighting"
	}
	if strings.Contains(desc, "scary") || strings.Contains(desc, "terrifying") || strings.Contains(desc, "qo'rquv") {
		return "dark, suspenseful atmosphere with ominous lighting"
	}
	if strings.Contains(desc, "epic") || strings.Contains(desc, "grand") || strings.Contains(desc, "epik") {
		return "epic scale with grand visual composition"
	}

	// Fallback based on genres
	for _, g := range metadata.Genres {
		g = strings.ToLower(g)
		switch g {
		case "action", "боевой":
			return "high-energy atmosphere with dynamic composition"
		case "thriller", "триллер":
			return "tense, suspenseful atmosphere"
		case "horror", "ужасы":
			return "dark, unsettling atmosphere"
		case "science fiction", "sci-fi", "фантастика":
			return "futuristic, high-tech atmosphere"
		case "fantasy", "фэнтези":
			return "magical, otherworldly atmosphere"
		case "war", "военный":
			return "dramatic, intense battlefield atmosphere"
		}
	}

	return "cinematic atmosphere with professional movie-poster styling"
}

// getCompositionGuidance returns composition guidance based on available metadata
func (c *OpenAIClient) getCompositionGuidance(metadata *models.EnrichedMetadata) string {
	var guidance []string

	guidance = append(guidance, "- Central or off-center subject composition")
	guidance = append(guidance, "- Character(s) prominently featured in upper portion")
	guidance = append(guidance, "- Dramatic lighting with spotlight or rim lighting effects")

	// Add background/atmosphere based on genre
	for _, g := range metadata.Genres {
		g = strings.ToLower(g)
		switch g {
		case "action", "боевой", "j Action":
			guidance = append(guidance, "- Action elements or vehicles in foreground")
		case "horror", "ужасы":
			guidance = append(guidance, "- Dark, atmospheric background with fog or mist")
		case "science fiction", "sci-fi", "фантастика":
			guidance = append(guidance, "- Futuristic cityscape or space background")
		case "fantasy", "фэнтези":
			guidance = append(guidance, "- Magical or fantastical background elements")
		case "romance", "романтика":
			guidance = append(guidance, "- Soft, dreamy background with bokeh effects")
		}
	}

	return strings.Join(guidance, "\n")
}

// getBrandingInstruction returns the branding instruction for FilmoraUz
func (c *OpenAIClient) getBrandingInstruction() string {
	return `
Branding Requirements:
- Add "FilmoraUz.net" branding subtly in the bottom-right corner
- Use clean spacing between branding elements
- Branding should be visible but not intrusive
- Style: 'Filmora' in white, 'Uz' in orange (#FF7A00), '.net' in white
- Add subtle drop shadow for readability against any background
- Do not place branding over faces, text, or key visual elements
- Branding should appear professional, not like a watermark
`
}

// generateImage generates an image using OpenAI's image generation API
func (c *OpenAIClient) generateImage(ctx context.Context, req *ImageGenerationRequest) (*ImageGenerationResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("OpenAI client not configured")
	}

	// Use DALL-E 3 for high quality (string model names)
	model := "dall-e-3"
	if req.Model == "dall-e-2" {
		model = "dall-e-2"
	}

	// Set response format
	respFormat := openai.CreateImageResponseFormatURL
	if req.Prompt != "" && strings.Contains(req.Prompt, "base64") {
		respFormat = openai.CreateImageResponseFormatB64JSON
	}

	// Build image request
	imgReq := openai.ImageRequest{
		Model:          model,
		Prompt:         req.Prompt,
		Size:           openai.CreateImageSize1024x1024,
		Quality:        "hd",
		ResponseFormat: respFormat,
		N:              1,
	}

	// Set size if provided
	if req.Size != "" {
		switch req.Size {
		case "256x256":
			imgReq.Size = openai.CreateImageSize256x256
		case "512x512":
			imgReq.Size = openai.CreateImageSize512x512
		case "1024x1024":
			imgReq.Size = openai.CreateImageSize1024x1024
		case "1792x1024":
			imgReq.Size = openai.CreateImageSize1792x1024
		case "1024x1792":
			imgReq.Size = openai.CreateImageSize1024x1792
		}
	}

	// Set quality if provided
	if req.Quality == "standard" {
		imgReq.Quality = "standard"
	}

	// Set style if provided
	if req.Style == "natural" {
		imgReq.Style = "natural"
	} else {
		imgReq.Style = "vivid"
	}

	log.Printf("[OPENAI] ===== IMAGE GENERATION REQUEST =====")
	log.Printf("[OPENAI] model=%s size=%s quality=%s style=%s", imgReq.Model, imgReq.Size, imgReq.Quality, imgReq.Style)
	log.Printf("[OPENAI] prompt (first 200 chars): %.200s", imgReq.Prompt)

	// Generate image
	resp, err := c.client.CreateImage(ctx, imgReq)
	if err != nil {
		log.Printf("[OPENAI] ===== IMAGE GENERATION FAILED =====")
		log.Printf("[OPENAI] Error: %v", err)
		return nil, fmt.Errorf("image generation failed: %w", err)
	}

	if len(resp.Data) == 0 {
		log.Printf("[OPENAI] ERROR: response contained 0 images")
		return nil, fmt.Errorf("no image data returned")
	}

	log.Printf("[OPENAI] ===== IMAGE GENERATION SUCCESS =====")
	log.Printf("[OPENAI] revised_prompt (first 200 chars): %.200s", resp.Data[0].RevisedPrompt)
	log.Printf("[OPENAI] response_url=%q has_b64=%v", resp.Data[0].URL, resp.Data[0].B64JSON != "")

	result := &ImageGenerationResult{
		Revised: resp.Data[0].RevisedPrompt,
	}

	// Handle different response formats
	if resp.Data[0].URL != "" {
		log.Printf("[OPENAI] Downloading generated image from: %s", resp.Data[0].URL)
		localPath, err := c.downloadAndSaveImage(ctx, resp.Data[0].URL)
		if err != nil {
			log.Printf("[OPENAI] ERROR: failed to download generated image: %v", err)
			return nil, fmt.Errorf("failed to save generated image: %w", err)
		}
		result.ImagePath = localPath
		result.ImageURL = resp.Data[0].URL
		log.Printf("[OPENAI] Image saved to: %s", localPath)
	} else if resp.Data[0].B64JSON != "" {
		imgData, err := base64.StdEncoding.DecodeString(resp.Data[0].B64JSON)
		if err != nil {
			log.Printf("[OPENAI] ERROR: failed to decode base64 image: %v", err)
			return nil, fmt.Errorf("failed to decode base64 image: %w", err)
		}
		localPath, err := c.saveImageData(imgData, "png")
		if err != nil {
			log.Printf("[OPENAI] ERROR: failed to save b64 image: %v", err)
			return nil, fmt.Errorf("failed to save image data: %w", err)
		}
		result.ImagePath = localPath
		result.Base64 = resp.Data[0].B64JSON
		log.Printf("[OPENAI] B64 image saved to: %s", localPath)
	} else {
		log.Printf("[OPENAI] ERROR: response has neither URL nor B64JSON")
		return nil, fmt.Errorf("response contained no image URL or base64 data")
	}

	log.Printf("[OPENAI] Image generation complete: %s", result.ImagePath)
	return result, nil
}

// editPoster edits an existing poster using OpenAI's image editing API
func (c *OpenAIClient) editPoster(ctx context.Context, prompt string, originalImageURL string, metadata *models.EnrichedMetadata) (*ImageGenerationResult, error) {
	if !c.IsConfigured() {
		return nil, fmt.Errorf("OpenAI client not configured")
	}

	log.Printf("[OPENAI] ===== POSTER EDIT (DALL-E 3 inspired generation) =====")
	log.Printf("[OPENAI] Fetching original poster for reference: %s", originalImageURL)

	// Download the original image (used as visual reference description in prompt)
	imgData, contentType, err := c.downloadImage(ctx, originalImageURL)
	if err != nil {
		log.Printf("[OPENAI] WARNING: failed to download original poster (%v) — will generate from scratch", err)
		// Fall through to scratch generation with text-only prompt
		return c.generateImage(ctx, &ImageGenerationRequest{
			Prompt:  prompt,
			Model:   c.model,
			Size:    "1024x1024",
			Quality: "hd",
			Style:   "vivid",
		})
	}
	log.Printf("[OPENAI] Original poster downloaded: %d bytes, type=%s", len(imgData), contentType)

	// Save locally for reference (not sent to DALL-E — API doesn't accept image URLs)
	originalPath, err := c.saveImageData(imgData, contentType)
	if err != nil {
		return nil, fmt.Errorf("failed to save original image: %w", err)
	}
	defer os.Remove(originalPath) // clean up temp file
	log.Printf("[OPENAI] Original poster cached locally: %s", originalPath)

	// For DALL-E 3, we can't do direct editing, so we use the image as inspiration
	// by including it in the prompt description
	inspiredPrompt := fmt.Sprintf(`
Create a premium cinematic movie poster inspired by the style and composition of: %s

Requirements: %s

Important:
- Keep similar composition and mood to the original
- Transform the title to Uzbek language
- Add FilmoraUz.net branding in bottom-right
- Maintain high quality cinematic look
`, originalImageURL, prompt)

	// Generate new poster inspired by the original
	imgReq := openai.ImageRequest{
		Model:          "dall-e-3",
		Prompt:         inspiredPrompt,
		Size:           openai.CreateImageSize1024x1024,
		Quality:        "hd",
		Style:          "vivid",
		ResponseFormat: openai.CreateImageResponseFormatURL,
		N:              1,
	}

	log.Printf("[OPENAI] ===== DALL-E 3 EDIT REQUEST =====")
	log.Printf("[OPENAI] model=dall-e-3 size=1024x1024 quality=hd style=vivid")
	log.Printf("[OPENAI] inspired_prompt (first 200 chars): %.200s", inspiredPrompt)

	resp, err := c.client.CreateImage(ctx, imgReq)
	if err != nil {
		log.Printf("[OPENAI] ===== DALL-E 3 EDIT FAILED =====")
		log.Printf("[OPENAI] Error: %v", err)
		return nil, fmt.Errorf("image editing failed: %w", err)
	}

	if len(resp.Data) == 0 {
		log.Printf("[OPENAI] ERROR: edit response contained 0 images")
		return nil, fmt.Errorf("no image data returned")
	}

	log.Printf("[OPENAI] Edit succeeded — response_url=%q revised=%.100s",
		resp.Data[0].URL, resp.Data[0].RevisedPrompt)

	result := &ImageGenerationResult{
		Revised: resp.Data[0].RevisedPrompt,
	}

	// Download the generated image
	if resp.Data[0].URL != "" {
		log.Printf("[OPENAI] Downloading edited poster from: %s", resp.Data[0].URL)
		localPath, err := c.downloadAndSaveImage(ctx, resp.Data[0].URL)
		if err != nil {
			log.Printf("[OPENAI] ERROR: failed to download edited poster: %v", err)
			return nil, fmt.Errorf("failed to save edited image: %w", err)
		}
		result.ImagePath = localPath
		result.ImageURL = resp.Data[0].URL
		log.Printf("[OPENAI] Edited poster saved: %s", localPath)
	} else {
		log.Printf("[OPENAI] ERROR: edit response has no URL")
		return nil, fmt.Errorf("edit response contained no image URL")
	}

	log.Printf("[OPENAI] Poster edit complete: %s", result.ImagePath)
	return result, nil
}

// downloadImage downloads an image from URL
func (c *OpenAIClient) downloadImage(ctx context.Context, url string) ([]byte, string, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, "", err
	}

	// Add common headers
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; FilmoraUzWorker/1.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("download failed with status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}

	// Detect content type
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}

	return data, contentType, nil
}

// downloadAndSaveImage downloads an image and saves it to disk
func (c *OpenAIClient) downloadAndSaveImage(ctx context.Context, url string) (string, error) {
	imgData, contentType, err := c.downloadImage(ctx, url)
	if err != nil {
		return "", err
	}

	ext := "png"
	if strings.Contains(contentType, "jpeg") || strings.Contains(contentType, "jpg") {
		ext = "jpg"
	}

	return c.saveImageData(imgData, ext)
}

// saveImageData saves image data to disk
func (c *OpenAIClient) saveImageData(data []byte, format string) (string, error) {
	// Generate unique filename
	timestamp := time.Now().UnixNano()
	filename := filepath.Join(c.tempDir, fmt.Sprintf("openai_gen_%d.%s", timestamp, format))

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return filename, nil
}

// UploadImage uploads a local image to storage and returns the URL
// This is a placeholder - actual upload should be handled by the pipeline's storage
func (c *OpenAIClient) UploadImage(ctx context.Context, localPath string) (string, error) {
	if localPath == "" {
		return "", fmt.Errorf("local path is empty")
	}

	// Verify file exists
	if _, err := os.Stat(localPath); err != nil {
		return "", fmt.Errorf("file does not exist: %w", err)
	}

	// For now, just return the local path
	// The pipeline will handle actual storage
	return localPath, nil
}

// Cleanup removes temporary files
func (c *OpenAIClient) Cleanup(paths []string) {
	for _, p := range paths {
		if p != "" && strings.HasPrefix(p, c.tempDir) {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				log.Printf("[OPENAI] Failed to cleanup file %s: %v", p, err)
			}
		}
	}
}

// LocalizeTitle is a helper function to get Uzbek title from English
// This is duplicated from pipeline/localization.go to avoid import cycles
func LocalizeTitle(englishTitle string) string {
	// Fast & Furious series
	titleMap := map[string]string{
		"The Fast and the Furious":              "Forsaj",
		"2 Fast 2 Furious":                      "Forsaj 2",
		"The Fast and the Furious: Tokyo Drift": "Forsaj: Tokio Drift",
		"Fast & Furious":                        "Forsaj 4",
		"Fast Five":                             "Forsaj 5",
		"Furious 7":                             "Forsaj 7",
		"The Fate of the Furious":               "Forsaj 8",
		"F9: The Fast Saga":                     "Forsaj 9",
		"Fast X":                                "Forsaj 10",
		// Marvel/C superhero movies
		"Iron Man":                "Temir Odam",
		"Thor":                    "Tor",
		"Captain America":         "Kapitan Amerika",
		"Spider-Man":              "Odam-oyana",
		"Avengers":                "Qasoskorlar",
		"Guardians of the Galaxy": "Galaktika Qo'riqchilari",
		// Other popular titles
		"Taken":                    "Olib qo'yilgan",
		"John Wick":                "Jon Vik",
		"Die Hard":                 "Qattiq O'lim",
		"Rambo":                    "Rambo",
		"Rocky":                    "Rokki",
		"Terminator":               "Terminator",
		"Transformers":             "Transformers",
		"Avatar":                   "Avatar",
		"Jurassic Park":            "Yurassic Park",
		"Pirates of the Caribbean": "Karib Dengizlari Qaroqchilari",
		"Harry Potter":             "Gari Potter",
		"Star Wars":                "Yulduzlar Urushi",
		"James Bond":               "Jeyms Bond",
		"Mission: Impossible":      "Mumkin Emas Missiya",
		"The Shawshank Redemption": "Shoushenk Amali",
		"The Godfather":            "Ota",
		"The Dark Knight":          "Qorong'i Ritsar",
		"Forrest Gump":             "Forrest Gump",
		"The Matrix":               "Matritsa",
		"Inception":                "Boshlash",
		"Interstellar":             "Yulduzlararo",
		"The Lord of the Rings":    "Uzuklar Egasi",
	}

	if localized, ok := titleMap[englishTitle]; ok {
		return localized
	}
	return ""
}

// Helper function to decode PNG
func decodePNG(data []byte) (image.Image, error) {
	return png.Decode(bytes.NewReader(data))
}

// Helper function to decode JPEG
func decodeJPEG(data []byte) (image.Image, error) {
	return jpeg.Decode(bytes.NewReader(data))
}
