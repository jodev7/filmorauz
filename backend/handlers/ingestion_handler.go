package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IngestionHandler handles ingestion API requests
type IngestionHandler struct {
	jobRepo    *repositories.JobRepository
	seriesRepo *repositories.SeriesRepository
	seriesSvc  *services.SeriesService
	parserURL  string
	workerURL  string
	httpClient *http.Client
}

func parserRawPrefix(body []byte, limit int) string {
	if limit <= 0 {
		limit = 300
	}
	raw := string(body)
	if len(raw) > limit {
		return raw[:limit]
	}
	return raw
}

func parserValueAsString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == float64(int(t)) {
			return strconv.Itoa(int(t))
		}
		return fmt.Sprintf("%v", t)
	case int:
		return strconv.Itoa(t)
	case int32:
		return strconv.Itoa(int(t))
	case int64:
		return strconv.FormatInt(t, 10)
	case json.Number:
		return t.String()
	default:
		if t == nil {
			return ""
		}
		return strings.TrimSpace(fmt.Sprintf("%v", t))
	}
}

func parserValueAsInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case float32:
		return int(t)
	case int:
		return t
	case int32:
		return int(t)
	case int64:
		return int(t)
	case json.Number:
		if i, err := t.Int64(); err == nil {
			return int(i)
		}
	case string:
		if t == "" {
			return 0
		}
		if i, err := strconv.Atoi(strings.TrimSpace(t)); err == nil {
			return i
		}
	}
	return 0
}

func parserValueAsStringSlice(v interface{}) []string {
	items, ok := v.([]interface{})
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(items))
	for _, item := range items {
		s := parserValueAsString(item)
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func parserMapToCatalogItem(raw map[string]interface{}) CatalogItem {
	itemType := parserValueAsString(raw["type"])
	if itemType == "" {
		itemType = parserValueAsString(raw["content_type"])
	}
	if itemType == "series" {
		itemType = "serial"
	}

	poster := parserValueAsString(raw["poster"])
	if poster == "" {
		poster = parserValueAsString(raw["poster_url"])
	}
	if poster == "" {
		poster = parserValueAsString(raw["img"])
	}

	return CatalogItem{
		SourceID:           parserValueAsString(raw["source_id"]),
		Title:              parserValueAsString(raw["title"]),
		Year:               parserValueAsInt(raw["year"]),
		Type:               itemType,
		Poster:             poster,
		Description:        parserValueAsString(raw["description"]),
		Genres:             parserValueAsStringSlice(raw["genres"]),
		DetailURL:          parserValueAsString(raw["detail_url"]),
		Confidence:         math.Max(0, math.Min(1, parserValueAsFloat(raw["confidence"]))),
		AvailableQualities: parserValueAsStringSlice(raw["available_qualities"]),
		SelectedQuality:    parserValueAsString(raw["selected_quality"]),
		SelectedVideoURL:   parserValueAsString(raw["selected_video_url"]),
	}
}

func parserValueAsFloat(v interface{}) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case json.Number:
		if f, err := t.Float64(); err == nil {
			return f
		}
	case string:
		if t == "" {
			return 0
		}
		if f, err := strconv.ParseFloat(strings.TrimSpace(t), 64); err == nil {
			return f
		}
	}
	return 0
}

func parseCatalogResponseFlexible(body []byte, page int, limit int) (CatalogResponse, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return CatalogResponse{}, err
	}

	result := CatalogResponse{
		Items:      []CatalogItem{},
		Page:       page,
		Limit:      limit,
		Total:      0,
		TotalPages: 0,
		HasMore:    false,
	}

	if v := parserValueAsInt(payload["page"]); v > 0 {
		result.Page = v
	}
	if v := parserValueAsInt(payload["limit"]); v > 0 {
		result.Limit = v
	}
	if v := parserValueAsInt(payload["total"]); v > 0 {
		result.Total = v
	}
	if v := parserValueAsInt(payload["total_pages"]); v > 0 {
		result.TotalPages = v
	}
	if hasMore, ok := payload["has_more"].(bool); ok {
		result.HasMore = hasMore
	}

	var rawItems []interface{}
	if items, ok := payload["items"].([]interface{}); ok {
		rawItems = items
	} else if results, ok := payload["results"].([]interface{}); ok {
		rawItems = results
	}

	for _, rawItem := range rawItems {
		itemMap, ok := rawItem.(map[string]interface{})
		if !ok {
			continue
		}
		item := parserMapToCatalogItem(itemMap)
		if item.SourceID == "" && item.DetailURL == "" && item.Title == "" {
			continue
		}
		result.Items = append(result.Items, item)
	}

	if result.Total == 0 {
		result.Total = len(result.Items)
	}

	return result, nil
}

func normalizeSearchResponse(body []byte) (map[string]interface{}, error) {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, err
	}
	if _, ok := payload["results"]; ok {
		return payload, nil
	}
	if items, ok := payload["items"]; ok {
		payload["results"] = items
	}
	return payload, nil
}

func resolveCompletedDownloadPath(jobID, rawPath string) string {
	canonicalize := func(path string) string {
		if !strings.HasSuffix(path, ".MUX.mp4") {
			return path
		}
		canonical := strings.TrimSuffix(path, ".MUX.mp4") + ".mp4"
		if _, err := os.Stat(path); err == nil {
			if _, dstErr := os.Stat(canonical); dstErr != nil {
				log.Printf("[DOWNLOAD RENAME] from=%s to=%s", path, canonical)
				if renameErr := os.Rename(path, canonical); renameErr == nil {
					return canonical
				}
			}
		}
		return path
	}

	candidates := []string{}
	if strings.TrimSpace(rawPath) != "" {
		candidates = append(candidates, rawPath)
	}

	downloadDir := os.Getenv("DOWNLOAD_DIR")
	if strings.TrimSpace(downloadDir) == "" {
		downloadDir = "/opt/filmorauz/parser/downloads"
	}
	base := strings.TrimSpace(jobID)
	if base != "" {
		candidates = append(candidates,
			filepath.Join(downloadDir, base+".mp4"),
			filepath.Join(downloadDir, base+".MUX.mp4"),
		)
		if matches, err := filepath.Glob(filepath.Join(downloadDir, base+"*")); err == nil {
			candidates = append(candidates, matches...)
		}
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		absPath, err := filepath.Abs(candidate)
		if err != nil {
			absPath = candidate
		}
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}
		if info, statErr := os.Stat(absPath); statErr == nil && !info.IsDir() && info.Size() > 0 {
			return canonicalize(absPath)
		}
	}
	return ""
}

// NewIngestionHandler creates a new ingestion handler
func NewIngestionHandler(jobRepo *repositories.JobRepository, seriesRepo *repositories.SeriesRepository, seriesSvc *services.SeriesService, parserURL string, workerURL string) *IngestionHandler {
	// Create HTTP client with explicit timeout
	httpClient := &http.Client{
		// Large series (e.g. Dexter ~90 episodes) take well over 30s to
		// scrape from the parser. Allow up to 3 minutes per call.
		Timeout: 180 * time.Second,
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
		},
	}

	log.Printf("[INGESTION] Handler initialized with parserURL: %s, workerURL: %s", parserURL, workerURL)

	return &IngestionHandler{
		jobRepo:    jobRepo,
		seriesRepo: seriesRepo,
		seriesSvc:  seriesSvc,
		parserURL:  parserURL,
		workerURL:  workerURL,
		httpClient: httpClient,
	}
}

// safeHandler wraps a handler function with panic recovery
func safeHandler(fn func(c *gin.Context)) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				// Log the panic with stack trace
				log.Printf("[PANIC RECOVERY] Handler panicked: %v\n%s", r, debug.Stack())
				// Return a proper JSON error instead of crashing
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":   "Internal server error",
					"details": fmt.Sprintf("recovered from panic: %v", r),
				})
			}
		}()
		fn(c)
	}
}

// SearchSource searches for movies on a source
// GET /api/ingestion/search?source=uzmovi&q=interstellar
func (h *IngestionHandler) SearchSource(c *gin.Context) {
	// Panic recovery wrapper
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] SearchSource panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	source := c.Query("source")
	if strings.Contains(source, ".") {
		source = strings.Split(source, ".")[0]
	}
	query := c.Query("q")

	log.Printf("[INGESTION] SEARCH: request received - source=%s, query=%s", source, query)

	if source == "" || query == "" {
		log.Printf("[INGESTION] SEARCH: missing required parameters")
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and q parameters are required"})
		return
	}

	// Validate source
	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinolar":    true,
		"kinochilar": true,
		"uzmedia":    true,
		"manual":     true,
	}
	if !validSources[source] {
		log.Printf("[INGESTION] SEARCH: invalid source=%s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia, kinolar, kinochilar, uzmedia, manual"})
		return
	}

	// Build parser URL with proper URL encoding using net/url
	params := url.Values{}
	params.Set("source", source)
	params.Set("q", query)

	// Ensure parserURL is properly formatted
	parserBaseURL := h.parserURL
	if parserBaseURL == "" {
		log.Printf("[INGESTION] SEARCH: ERROR - parserURL is not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	// Remove trailing slash from parserURL if present
	parserBaseURL = fmt.Sprintf("%s", parserBaseURL)

	parserURL := fmt.Sprintf("%s/search?%s", parserBaseURL, params.Encode())

	// Log the final parser URL for debugging
	log.Printf("[INGESTION] SEARCH: parser_base_url=%s", h.parserURL)
	log.Printf("[INGESTION] SEARCH: full_url=%s", parserURL)

	// Make HTTP request with detailed error handling
	var resp *http.Response
	resp, err := h.httpClient.Get(parserURL)
	if err != nil {
		// Check if it's a connection error
		log.Printf("[INGESTION] SEARCH: connection error - %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "parser service unavailable",
			"details": fmt.Sprintf("failed to connect to parser service: %v", err),
		})
		return
	}

	// Safety check: ensure response is not nil
	if resp == nil {
		log.Printf("[INGESTION] SEARCH: ERROR - nil response from parser")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "parser service returned nil response",
		})
		return
	}

	// Log response status
	log.Printf("[INGESTION] SEARCH: response_status=%d", resp.StatusCode)

	// Read response body with size limit to prevent memory issues
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close() // Close immediately after reading

	if err != nil {
		log.Printf("[INGESTION] SEARCH: ERROR reading response body - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to read parser response",
			"details": fmt.Sprintf("error reading response: %v", err),
		})
		return
	}

	// Log response body length for debugging
	log.Printf("[INGESTION] SEARCH: response_body_length=%d", len(body))

	// Debug: print response body sample
	bodySample := string(body)
	if len(bodySample) > 500 {
		bodySample = bodySample[:500] + "..."
	}
	log.Printf("[INGESTION] SEARCH: response_body_sample=%s", bodySample)

	if source == "freekino" {
		log.Printf("[FREEKINO] parser status=%d raw_len=%d", resp.StatusCode, len(body))
	}

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		log.Printf("[INGESTION] SEARCH: ERROR - parser returned status=%d, body=%s", resp.StatusCode, bodyStr)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "parser service returned error",
			"status":  resp.StatusCode,
			"details": bodyStr,
		})
		return
	}

	// Decode JSON safely - handle empty or malformed JSON
	if len(body) == 0 {
		log.Printf("[INGESTION] SEARCH: ERROR - empty response body")
		c.JSON(http.StatusOK, gin.H{
			"results": []interface{}{},
			"message": "no results found",
		})
		return
	}

	result, err := normalizeSearchResponse(body)
	if err != nil {
		if source == "freekino" {
			log.Printf("[FREEKINO] parse error raw_prefix=%s", parserRawPrefix(body, 300))
		}
		log.Printf("[INGESTION] SEARCH: ERROR decoding JSON - %v, body: %s", err, parserRawPrefix(body, 200))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to parse parser response",
			"details": fmt.Sprintf("JSON decode error: %v", err),
			"status":  resp.StatusCode,
		})
		return
	}

	log.Printf("[INGESTION] SEARCH: success - returning results")
	c.JSON(http.StatusOK, result)
}

// GetMovieDetails gets detailed movie info from source
// GET /api/ingestion/details?source=uzmovi&id=interstellar
func (h *IngestionHandler) GetMovieDetails(c *gin.Context) {
	// Panic recovery wrapper
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] GetMovieDetails panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	source := c.Query("source")
	if strings.Contains(source, ".") {
		source = strings.Split(source, ".")[0]
	}
	sourceID := c.Query("id")
	detailURL := c.Query("url")

	log.Printf("[INGESTION] DETAILS: request received - source=%s, source_id=%s, url=%s", source, sourceID, detailURL)

	if source == "" || (sourceID == "" && detailURL == "") {
		log.Printf("[INGESTION] DETAILS: missing required parameters")
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and (id or url) parameters are required"})
		return
	}

	// Validate source
	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinolar":    true,
		"kinochilar": true,
		"uzmedia":    true,
		"manual":     true,
	}
	if !validSources[source] {
		log.Printf("[INGESTION] DETAILS: invalid source=%s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source"})
		return
	}

	// Ensure parserURL is configured
	if h.parserURL == "" {
		log.Printf("[INGESTION] DETAILS: ERROR - parserURL is not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	// Build parser URL with proper encoding
	parserURL := fmt.Sprintf("%s/details", h.parserURL)
	req, err := http.NewRequest("GET", parserURL, nil)
	if err != nil {
		log.Printf("[INGESTION] DETAILS: ERROR creating request - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create parser request"})
		return
	}

	// Add query parameters
	q := req.URL.Query()
	q.Add("source", source)
	if sourceID != "" {
		q.Add("id", sourceID)
	}
	if detailURL != "" {
		q.Add("url", detailURL)
	}
	req.URL.RawQuery = q.Encode()

	// Log request details
	log.Printf("[INGESTION] DETAILS: parser_base_url=%s", h.parserURL)
	log.Printf("[INGESTION] DETAILS: full_url=%s", req.URL.String())

	// Make request with timeout-safe client
	var resp *http.Response
	resp, err = h.httpClient.Do(req)
	if err != nil {
		log.Printf("[INGESTION] DETAILS: ERROR calling parser - %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "parser service unavailable",
			"details": fmt.Sprintf("failed to connect to parser service: %v", err),
		})
		return
	}

	// Safety check: ensure response is not nil
	if resp == nil {
		log.Printf("[INGESTION] DETAILS: ERROR - nil response from parser")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "parser service returned nil response",
		})
		return
	}

	// Log response status
	log.Printf("[INGESTION] DETAILS: response_status=%d", resp.StatusCode)

	// Read response body
	body, err := io.ReadAll(resp.Body)
	resp.Body.Close() // Close immediately after reading

	if err != nil {
		log.Printf("[INGESTION] DETAILS: ERROR reading response - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to read parser response",
			"details": fmt.Sprintf("error reading response: %v", err),
		})
		return
	}

	// Log response body length
	log.Printf("[INGESTION] DETAILS: response_body_length=%d", len(body))

	if resp.StatusCode != http.StatusOK {
		bodyStr := string(body)
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		log.Printf("[INGESTION] DETAILS: ERROR - parser returned status=%d, body=%s", resp.StatusCode, bodyStr)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "parser service returned error",
			"status":  resp.StatusCode,
			"details": bodyStr,
		})
		return
	}

	// Decode JSON safely - handle empty or malformed JSON
	if len(body) == 0 {
		log.Printf("[INGESTION] DETAILS: ERROR - empty response body")
		c.JSON(http.StatusOK, gin.H{
			"data":    nil,
			"message": "no details found",
		})
		return
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		log.Printf("[INGESTION] DETAILS: ERROR decoding JSON - %v, body: %s", err, bodyStr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to parse parser response",
			"details": fmt.Sprintf("JSON decode error: %v", err),
		})
		return
	}

	log.Printf("[INGESTION] DETAILS: success")
	c.JSON(http.StatusOK, result)
}

// CreateIngestionJob creates a new ingestion job
// POST /api/ingestion/jobs
func (h *IngestionHandler) CreateIngestionJob(c *gin.Context) {
	// Panic recovery wrapper
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] CreateIngestionJob panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	var input struct {
		Title     string `json:"title"`
		Source    string `json:"source" binding:"required"`
		SourceID  string `json:"source_id" binding:"required"`
		DetailURL string `json:"detail_url"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[INGESTION] JOB: invalid request - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] JOB: creating job - title=%s, source=%s, source_id=%s",
		input.Title, input.Source, input.SourceID)

	// Validate source
	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinolar":    true,
		"kinochilar": true,
		"uzmedia":    true,
		"manual":     true,
	}
	if !validSources[input.Source] {
		log.Printf("[INGESTION] JOB: invalid source=%s", input.Source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source"})
		return
	}

	// Create job with all fields
	job := &models.IngestionJob{
		Title:     input.Title,
		Source:    input.Source,
		SourceID:  input.SourceID,
		DetailURL: input.DetailURL,
		Status:    models.IngestionStatusQueued,
		Stage:     string(models.IngestionStatusQueued),
		Progress:  0,
		Steps:     models.JobSteps{},
		Logs:      []models.IngestionLog{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.jobRepo.Create(ctx, job); err != nil {
		// Check for duplicate
		if ctx.Err() == nil {
			// Check if it's a duplicate key error
			c.JSON(http.StatusConflict, gin.H{"error": "job already exists for this source and source_id"})
			return
		}
		log.Printf("[INGESTION] JOB: failed to create job - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job"})
		return
	}

	log.Printf("[INGESTION] JOB: created job %s", job.ID.Hex())
	c.JSON(http.StatusCreated, gin.H{"data": job, "message": "Ingestion job created"})
}

// CreateDirectUploadJob creates a new ingestion job for direct MP4 upload
// POST /api/admin/ingestion/direct-upload
type DirectUploadInput struct {
	Title       string   `json:"title" binding:"required"`
	TempFileURL string   `json:"temp_file_url" binding:"required"`
	TempFileKey string   `json:"temp_file_key"` // B2 temp key for cleanup tracking
	PosterURL   string   `json:"poster_url"`
	BackdropURL string   `json:"backdrop_url"`
	Year        int      `json:"year"`
	Genres      []string `json:"genres"`
	Country     string   `json:"country"`
	Duration    int      `json:"duration"`
	Quality     string   `json:"quality"`
	IsPremium   bool     `json:"is_premium"`
}

func (h *IngestionHandler) CreateDirectUploadJob(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] CreateDirectUploadJob panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	var input DirectUploadInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[INGESTION] DIRECT_UPLOAD: invalid request - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] DIRECT_UPLOAD: creating job - title=%s, temp_file_url=%s",
		input.Title, input.TempFileURL)

	// Validate temp_file_url
	if input.TempFileURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "temp_file_url is required"})
		return
	}

	// Build metadata from input
	metadata := &models.ParsedMovieMetadata{
		Title:        input.Title,
		Poster:       input.PosterURL,
		Backdrop:     input.BackdropURL,
		Year:         input.Year,
		Genres:       input.Genres,
		Country:      input.Country,
		Duration:     input.Duration,
		VideoPageURL: input.TempFileURL,
	}

	sourceID := input.TempFileKey
	if sourceID == "" {
		sourceID = fmt.Sprintf("manual/direct_mp4:%d", time.Now().UnixNano())
	}

	// Create job with direct_upload source. The worker uses this source value to
	// run the direct MP4 pipeline; source_id identifies this manual upload.
	job := &models.IngestionJob{
		Title:       input.Title,
		Source:      "direct_upload",
		SourceID:    sourceID,
		DetailURL:   input.TempFileURL, // Store temp URL in DetailURL
		TempFileURL: input.TempFileURL,
		TempFileKey: input.TempFileKey, // Store B2 temp key for cleanup tracking
		Status:      models.IngestionStatusReadyToProcess,
		Stage:       string(models.IngestionStatusReadyToProcess),
		Progress:    0,
		Steps:       models.JobSteps{Download: true},
		Logs:        []models.IngestionLog{},
		Metadata:    metadata,
		Quality:     input.Quality,
		IsPremium:   input.IsPremium,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.jobRepo.Create(ctx, job); err != nil {
		log.Printf("[INGESTION] DIRECT_UPLOAD: failed to create job - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job"})
		return
	}

	log.Printf("[INGESTION] DIRECT_UPLOAD: created job %s", job.ID.Hex())
	c.JSON(http.StatusCreated, gin.H{"data": job, "message": "Direct upload ingestion job created"})
}

// GetUploadURL returns a presigned upload URL for direct B2 upload
// GET /api/get-upload-url?type=poster|backdrop|video&filename=abc.jpg
func (h *IngestionHandler) GetUploadURL(c *gin.Context) {
	fileType := c.Query("type")
	filename := c.Query("filename")

	if fileType == "" || filename == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type and filename are required"})
		return
	}

	if fileType != "poster" && fileType != "backdrop" && fileType != "video" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be poster, backdrop, or video"})
		return
	}

	// Call worker to get presigned URL
	if h.workerURL == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "worker service not configured"})
		return
	}

	query := url.Values{}
	query.Set("type", fileType)
	query.Set("filename", filename)
	workerResp, err := h.httpClient.Get(h.workerURL + "/get-upload-url?" + query.Encode())
	if err != nil {
		log.Printf("[UPLOAD_URL] Failed to call worker: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get upload URL"})
		return
	}
	defer workerResp.Body.Close()

	if workerResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(workerResp.Body)
		log.Printf("[UPLOAD_URL] Worker returned: %d %s", workerResp.StatusCode, body)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "worker error"})
		return
	}

	var result map[string]string
	if err := json.NewDecoder(workerResp.Body).Decode(&result); err != nil {
		log.Printf("[UPLOAD_URL] Failed to decode response: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid response"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"upload_url": result["upload_url"],
		"auth_token": result["auth_token"],
		"file_key":   result["file_key"],
		"cdn_url":    result["cdn_url"],
	})
}

// GetIngestionJob gets a job by ID
// GET /api/ingestion/jobs/:id
func (h *IngestionHandler) GetIngestionJob(c *gin.Context) {
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	job, err := h.jobRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": job})
}

// buildJobStatusFilter maps a status keyword to a Mongo filter for jobs.
// Empty/"all" returns an empty filter.
func buildJobStatusFilter(status string) bson.M {
	filter := bson.M{}
	switch status {
	case "", "all":
	case "active":
		filter["status"] = bson.M{"$nin": bson.A{
			models.IngestionStatusCompleted,
			models.IngestionStatusFailed,
			models.IngestionStatusDownloadFailed,
		}}
	case "pending", "queued":
		filter["$or"] = bson.A{
			bson.M{"status": models.IngestionStatusQueued},
			bson.M{"stage": "queued"},
		}
	case "downloading":
		filter["status"] = models.IngestionStatusDownloading
	case "ready_to_process":
		filter["status"] = models.IngestionStatusReadyToProcess
	case "processing":
		filter["$or"] = bson.A{
			bson.M{"status": bson.M{"$in": bson.A{
				models.IngestionStatusProcessing,
				models.IngestionStatusUploading,
				models.IngestionStatusHLSProcessing,
			}}},
			bson.M{"stage": bson.M{"$in": bson.A{
				"processing",
				"uploading",
				"hls_processing",
			}}},
		}
	case "failed":
		filter["status"] = bson.M{"$in": bson.A{
			models.IngestionStatusFailed,
			models.IngestionStatusDownloadFailed,
		}}
	case "completed":
		filter["status"] = models.IngestionStatusCompleted
	case "stuck":
		filter["status"] = bson.M{"$nin": bson.A{
			models.IngestionStatusCompleted,
			models.IngestionStatusFailed,
			models.IngestionStatusDownloadFailed,
		}}
		filter["updated_at"] = bson.M{"$lt": time.Now().Add(-30 * time.Minute)}
	default:
		filter["status"] = status
	}
	return filter
}

// ListIngestionJobs lists all jobs with filters
// GET /api/ingestion/jobs?status=pending&page=1&limit=30&skip=0&light=true
func (h *IngestionHandler) ListIngestionJobs(c *gin.Context) {
	startTime := time.Now()
	defer func() {
		duration := time.Since(startTime)
		if duration > 1*time.Second {
			log.Printf("[API] ListIngestionJobs slow response: duration=%v", duration)
		}
	}()

	status := c.Query("status")
	source := c.Query("source")
	if strings.Contains(source, ".") {
		source = strings.Split(source, ".")[0]
	}
	limit := 30
	page := 1
	skip := 0
	light := c.Query("light") == "true"

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if s := c.Query("skip"); s != "" {
		fmt.Sscanf(s, "%d", &skip)
	}
	if limit <= 0 {
		limit = 30
	}
	if limit > 100 {
		limit = 100
	}
	if page <= 0 {
		page = 1
	}
	if c.Query("page") != "" {
		skip = (page - 1) * limit
	}
	if skip < 0 {
		skip = 0
	}
	if c.Query("page") == "" {
		page = (skip / limit) + 1
	}

	filter := buildJobStatusFilter(status)
	if source != "" {
		filter["source"] = source
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var jobs []*models.IngestionJob
	var total int64
	var err error

	total, err = h.jobRepo.CountTopLevelGroups(ctx, filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count jobs"})
		return
	}

	jobs, err = h.jobRepo.ListByTopLevelGroups(ctx, filter, limit, skip, light)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}

	// Ensure jobs is never nil - return empty array instead
	if jobs == nil {
		jobs = []*models.IngestionJob{}
	}

	totalPages := 0
	if total > 0 {
		totalPages = int(math.Ceil(float64(total) / float64(limit)))
	}

	statusCounts := map[string]int64{}
	countKeys := []string{"all", "active", "pending", "processing", "failed", "stuck", "completed"}
	for _, key := range countKeys {
		f := buildJobStatusFilter(key)
		if source != "" {
			f["source"] = source
		}
		count, cerr := h.jobRepo.CountTopLevelGroups(ctx, f)
		if cerr != nil {
			log.Printf("[API] ListIngestionJobs status_counts %s error: %v", key, cerr)
			continue
		}
		statusCounts[key] = count
	}

	c.JSON(http.StatusOK, gin.H{
		"data":          jobs,
		"page":          page,
		"total":         total,
		"totalPages":    totalPages,
		"total_pages":   totalPages,
		"limit":         limit,
		"skip":          skip,
		"status_counts": statusCounts,
	})
}

// RetryIngestionJob retries a failed job and starts the download
// POST /api/ingestion/jobs/:id/retry
func (h *IngestionHandler) RetryIngestionJob(c *gin.Context) {
	id := c.Param("id")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	job, err := h.jobRepo.GetByID(ctx, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	// Allow retry from ANY state (except already processing)
	// This enables retry from: pending, failed, completed, processing (if stuck)
	if job.Status == models.IngestionStatusProcessing || job.Status == models.IngestionStatusUploading || job.Status == models.IngestionStatusDownloading {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is already being processed"})
		return
	}

	// Determine the appropriate stage based on existing job state
	// If local_path exists and steps.download is true, go to processing stage
	// Otherwise, start from download stage
	newStatus := models.IngestionStatusQueued

	if job.LocalPath != "" && job.Steps.Download {
		newStatus = models.IngestionStatusReadyToProcess
		log.Printf("[INGESTION] RETRY: Job %s has local_path=%s, resetting to ready_to_process", id, job.LocalPath)
	} else if job.Steps.Download && job.LocalPath == "" {
		newStatus = models.IngestionStatusQueued
		log.Printf("[INGESTION] RETRY: Job %s has steps.download but no local_path, restarting download", id)
	} else {
		log.Printf("[INGESTION] RETRY: Job %s restarting from queued download stage", id)
	}

	progressValue := 0
	if newStatus == models.IngestionStatusReadyToProcess {
		progressValue = 100
	}
	if err := h.jobRepo.UpdateStatus(ctx, id, newStatus, progressValue); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update job status"})
		return
	}

	if err := h.jobRepo.ClearError(ctx, id); err != nil {
		log.Printf("[INGESTION] WARNING: failed to clear error: %v", err)
	}

	log.Printf("[INGESTION] RETRY: Job %s restarted - status=%s", id, newStatus)

	c.JSON(http.StatusOK, gin.H{"message": "job retry initiated", "status": newStatus})
}

// UpdateJobProgress updates the download progress for a job
// This is called by the parser to report real-time download progress
// POST /api/ingestion/jobs/:id/progress
func (h *IngestionHandler) UpdateJobProgress(c *gin.Context) {
	id := c.Param("id")

	log.Printf("[INGESTION] PROGRESS: ========== START ==========")
	log.Printf("[INGESTION] PROGRESS: job_id from URL: %q", id)

	// Read raw body first
	bodyBytes, _ := io.ReadAll(c.Request.Body)
	log.Printf("[INGESTION] PROGRESS: raw_body = %s", string(bodyBytes))
	c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))

	if id == "" {
		log.Printf("[INGESTION] PROGRESS: ERROR - empty job id")
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id is required"})
		return
	}

	var progress struct {
		Stage              string  `json:"stage"`
		Status             string  `json:"status"`
		ProgressPercent    int     `json:"progress_percent"`
		Progress           int     `json:"progress"` // Alternative field name
		DownloadedBytes    int64   `json:"downloaded_bytes"`
		TotalBytes         int64   `json:"total_bytes"`
		SpeedMBps          float64 `json:"speed_mbps"`
		EtaSeconds         int     `json:"eta_seconds"`
		Message            string  `json:"message"`
		StepsDownload      bool    `json:"steps_download"`
		FilePath           string  `json:"file_path"` // Local file path after download completes
		LocalPath          string  `json:"local_path"`
		DownloadedFilePath string  `json:"downloaded_file_path"`
	}

	if err := c.ShouldBindJSON(&progress); err != nil {
		log.Printf("[INGESTION] PROGRESS: ERROR - invalid JSON: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] PROGRESS: RAW payload: %+v", progress)
	log.Printf("[INGESTION] PROGRESS: parsed progress: stage=%q, status=%q, progress=%d%%, steps_download=%v",
		progress.Stage, progress.Status, progress.ProgressPercent, progress.StepsDownload)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use ProgressPercent if set, otherwise fall back to Progress
	progressValue := progress.ProgressPercent
	if progressValue == 0 && progress.Progress > 0 {
		progressValue = progress.Progress
	}

	progressUpdate := &repositories.ProgressUpdate{
		Stage:              progress.Stage,
		Status:             progress.Status,
		Progress:           progressValue,
		DownloadedBytes:    progress.DownloadedBytes,
		TotalBytes:         progress.TotalBytes,
		SpeedMBps:          progress.SpeedMBps,
		EtaSeconds:         progress.EtaSeconds,
		Message:            progress.Message,
		StepsDownload:      progress.StepsDownload,
		FilePath:           progress.FilePath,
		LocalPath:          progress.LocalPath,
		DownloadedFilePath: progress.DownloadedFilePath,
	}

	if progress.StepsDownload || progressValue >= 100 {
		pathForLog := progress.LocalPath
		if pathForLog == "" {
			pathForLog = progress.FilePath
		}
		if pathForLog == "" {
			pathForLog = progress.DownloadedFilePath
		}
		log.Printf("[INGESTION] complete download job=%s local_path=%s", id, pathForLog)

		resolvedPath := resolveCompletedDownloadPath(id, pathForLog)
		if resolvedPath != "" {
			objID, objErr := primitive.ObjectIDFromHex(id)
			if objErr != nil {
				log.Printf("[INGESTION] COMPLETE: invalid job id %s: %v", id, objErr)
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
				return
			}

			now := time.Now()
			// Aggregation pipeline lets us only stamp download_finished_at /
			// queued_for_processing_at once (first real download->ready
			// transition). Status sync polling that re-hits this endpoint
			// should not reset them.
			update := bson.A{
				bson.M{
					"$set": bson.M{
						"local_path":               resolvedPath,
						"file_path":                resolvedPath,
						"downloaded_file_path":     resolvedPath,
						"status":                   models.IngestionStatusReadyToProcess,
						"stage":                    "ready_to_process",
						"progress":                 progressValue,
						"updated_at":               now,
						"error":                    "",
						"steps.download":           true,
						"steps.process":            false,
						"download_finished_at":     bson.M{"$ifNull": bson.A{"$download_finished_at", now}},
						"queued_for_processing_at": bson.M{"$ifNull": bson.A{"$queued_for_processing_at", now}},
					},
				},
				bson.M{"$unset": bson.A{"completed_at"}},
			}

			result, updErr := h.jobRepo.GetCollection().UpdateByID(ctx, objID, update)
			if updErr != nil {
				log.Printf("[INGESTION] COMPLETE: failed to persist local_path for job=%s path=%s err=%v", id, resolvedPath, updErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save completed download"})
				return
			}

			log.Printf("[INGESTION] COMPLETE: job=%s saved local_path=%s matched=%d modified=%d", id, resolvedPath, result.MatchedCount, result.ModifiedCount)
			c.JSON(http.StatusOK, gin.H{"message": "Download completion saved", "local_path": resolvedPath, "status": "ready_to_process"})
			return
		}

		log.Printf("[INGESTION] COMPLETE: job=%s progress=100 but file path not resolved yet; keeping non-failed state", id)
		if objID, objErr := primitive.ObjectIDFromHex(id); objErr == nil {
			_, _ = h.jobRepo.GetCollection().UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
				"$set": bson.M{
					"progress":   100,
					"updated_at": time.Now(),
					"message":    "Download completed, waiting for file path verification",
				},
			})
		}
		c.JSON(http.StatusOK, gin.H{"message": "Download completion pending path verification"})
		return
	}

	log.Printf("[INGESTION] PROGRESS: calling repository.UpdateProgress with id=%q, progress=%d", id, progressUpdate.Progress)

	err := h.jobRepo.UpdateProgress(ctx, id, progressUpdate)
	if err != nil {
		log.Printf("[INGESTION] PROGRESS: ERROR from repository: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update progress"})
		return
	}

	log.Printf("[INGESTION] PROGRESS: SUCCESS - returning 200")
	c.JSON(http.StatusOK, gin.H{"message": "Progress updated"})
}

// ProcessIngestionJob triggers the worker to process a downloaded video
// POST /api/ingestion/jobs/:id/process
// NOTE: This endpoint should NOT change job status - the worker polls for pending jobs
// Changing status here causes the job to become invisible to the worker
func (h *IngestionHandler) ProcessIngestionJob(c *gin.Context) {
	id := c.Param("id")

	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job id is required"})
		return
	}

	log.Printf("[INGESTION] PROCESS: Worker trigger requested for job %s", id)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Get current job to verify it's ready for processing
	job, err := h.jobRepo.GetByID(ctx, id)
	if err != nil {
		log.Printf("[INGESTION] PROCESS: failed to get job - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get job"})
		return
	}

	// Verify download is complete (steps.download should be true)
	if !job.Steps.Download {
		log.Printf("[INGESTION] PROCESS: job %s not ready - steps.download=%v", id, job.Steps.Download)
		c.JSON(http.StatusBadRequest, gin.H{"error": "download not complete, cannot process"})
		return
	}

	// Ensure local_path is set
	if job.LocalPath == "" {
		log.Printf("[INGESTION] PROCESS: job %s has no local_path", id)
		c.JSON(http.StatusBadRequest, gin.H{"error": "no local file path, cannot process"})
		return
	}

	// DO NOT change status here - worker polls for pending jobs
	// Changing to "processing" makes the job invisible to ClaimNextJob
	// The job stays as "pending" and worker will pick it up
	log.Printf("[INGESTION] PROCESS: Job %s is ready for worker (status unchanged: %s, local_path: %s)",
		id, job.Status, job.LocalPath)

	c.JSON(http.StatusOK, gin.H{"message": "Worker triggered", "job_id": id, "local_path": job.LocalPath})
}

// WorkerClaimJob allows worker to claim a job for ffmpeg processing
// GET /api/ingestion/jobs/worker/claim
// Returns a job where steps.download=true and steps.process=false
func (h *IngestionHandler) WorkerClaimJob(c *gin.Context) {
	log.Printf("[INGESTION] WORKER: Claim request for processing job")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// First try to claim a job ready for processing (steps.download=true)
	job, err := h.jobRepo.ClaimNextProcessingJob(ctx)
	if err != nil {
		log.Printf("[INGESTION] WORKER: Error claiming processing job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim job"})
		return
	}

	if job != nil {
		log.Printf("[INGESTION] WORKER: Claimed job %s for processing (local_path: %s)", job.ID.Hex(), job.LocalPath)
		c.JSON(http.StatusOK, gin.H{
			"job_id":     job.ID.Hex(),
			"local_path": job.LocalPath,
			"stage":      "process",
			"steps":      job.Steps,
			"source":     job.Source,
			"source_id":  job.SourceID,
		})
		return
	}

	// No jobs ready for processing
	log.Printf("[INGESTION] WORKER: No jobs ready for processing")
	c.JSON(http.StatusNotFound, gin.H{"error": "no jobs ready for processing"})
}

// ParserClaimJob allows the parser service to atomically claim the next queued download job.
// GET /api/ingestion/jobs/parser/claim
func (h *IngestionHandler) ParserClaimJob(c *gin.Context) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	job, err := h.jobRepo.ClaimNextJob(ctx)
	if err != nil {
		log.Printf("[INGESTION] PARSER: Error claiming download job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim job"})
		return
	}
	if job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no queued jobs"})
		return
	}

	log.Printf("[INGESTION] PARSER: Claimed job %s for download", job.ID.Hex())
	c.JSON(http.StatusOK, gin.H{
		"job_id":              job.ID.Hex(),
		"title":               job.Title,
		"source":              job.Source,
		"source_id":           job.SourceID,
		"detail_url":          job.DetailURL,
		"video_url":           job.VideoURL,
		"local_path":          job.LocalPath,
		"status":              job.Status,
		"contentType":         job.ContentType,
		"metadata":            job.Metadata,
		"source_quality":      job.SourceQuality,
		"available_qualities": job.AvailableQualities,
	})
}

// DeleteIngestionSeries removes ALL ingestion jobs that share a series_slug.
// Used by the admin UI to bulk-delete every episode of a serial in one click.
// DELETE /api/admin/ingestion/series/:slug
func (h *IngestionHandler) DeleteIngestionSeries(c *gin.Context) {
	slug := strings.TrimSpace(c.Param("slug"))
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "missing series slug"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	res, err := h.jobRepo.GetCollection().DeleteMany(ctx, bson.M{"series_slug": slug})
	if err != nil {
		log.Printf("[INGESTION] DELETE SERIES: mongo error slug=%s err=%v", slug, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete series jobs"})
		return
	}
	if res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "no jobs found for series", "slug": slug})
		return
	}

	log.Printf("[INGESTION] DELETE SERIES: removed %d job(s) slug=%s", res.DeletedCount, slug)
	c.JSON(http.StatusOK, gin.H{"message": "series jobs deleted", "slug": slug, "deleted": res.DeletedCount})
}

// DeleteIngestionJob removes an ingestion job from MongoDB.
// DELETE /api/admin/ingestion/jobs/:id
func (h *IngestionHandler) DeleteIngestionJob(c *gin.Context) {
	id := c.Param("id")

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, err := h.jobRepo.GetCollection().DeleteOne(ctx, bson.M{"_id": objID})
	if err != nil {
		log.Printf("[INGESTION] DELETE: mongo error id=%s err=%v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete job"})
		return
	}
	if res.DeletedCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
		return
	}

	log.Printf("[INGESTION] DELETE: removed job id=%s", id)
	c.JSON(http.StatusOK, gin.H{"message": "job deleted", "id": id})
}

// Ensure types are used
var (
	_ primitive.ObjectID
	_ time.Time
)

// CatalogItem represents an item from the source catalog
type CatalogItem struct {
	SourceID           string   `json:"source_id"`
	Title              string   `json:"title"`
	Year               int      `json:"year"`
	Type               string   `json:"type"` // "movie" or "serial"
	Poster             string   `json:"poster"`
	Description        string   `json:"description"`
	Genres             []string `json:"genres"`
	DetailURL          string   `json:"detail_url"`
	Confidence         float64  `json:"confidence,omitempty"`
	AvailableQualities []string `json:"available_qualities,omitempty"`
	SelectedQuality    string   `json:"selected_quality,omitempty"`
	SelectedVideoURL   string   `json:"selected_video_url,omitempty"`
}

// CatalogResponse represents the paginated catalog response
type CatalogResponse struct {
	Items      []CatalogItem `json:"items"`
	Page       int           `json:"page"`
	Limit      int           `json:"limit"`
	Total      int           `json:"total"`
	TotalPages int           `json:"total_pages"`
	HasMore    bool          `json:"has_more"`
}

// ListCatalog lists items from a source catalog with pagination
// GET /api/ingestion/catalog?source=uzmovi&page=1&limit=20&type=movies
func (h *IngestionHandler) ListCatalog(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] ListCatalog panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	source := c.Query("source")
	if strings.Contains(source, ".") {
		source = strings.Split(source, ".")[0]
	}
	page := 1
	limit := 20
	typeFilter := c.Query("type") // "movies", "serials", or empty for both

	if p := c.Query("page"); p != "" {
		fmt.Sscanf(p, "%d", &page)
	}
	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}

	log.Printf("[INGESTION] CATALOG: request - source=%s, page=%d, limit=%d, type=%s", source, page, limit, typeFilter)

	// Validate source (manual doesn't have catalog)
	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinochilar": true,
		"uzmedia":    true,
		"kinolar":    true,
	}
	if !validSources[source] {
		log.Printf("[INGESTION] CATALOG: invalid source=%s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source for catalog. valid: uzmovi, freekino, asilmedia, kinochilar, uzmedia, kinolar"})
		return
	}

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 50 {
		limit = 20
	}

	// Build parser URL
	parserBaseURL := h.parserURL
	if parserBaseURL == "" {
		log.Printf("[INGESTION] CATALOG: ERROR - parserURL is not configured")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	categoryURL := c.Query("category_url")

	params := url.Values{}
	params.Set("source", source)
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("limit", fmt.Sprintf("%d", limit))
	if typeFilter != "" {
		params.Set("type", typeFilter)
	}
	if categoryURL != "" {
		params.Set("category_url", categoryURL)
	}

	parserURL := fmt.Sprintf("%s/catalog?%s", parserBaseURL, params.Encode())
	log.Printf("[INGESTION] CATALOG: forwarding to parser source=%s page=%d limit=%d type=%q category_url=%q", source, page, limit, typeFilter, categoryURL)
	log.Printf("[INGESTION] CATALOG: calling parser: %s", parserURL)

	resp, err := h.httpClient.Get(parserURL)
	if err != nil {
		log.Printf("[INGESTION] CATALOG: connection error - %v", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error":   "parser service unavailable",
			"details": fmt.Sprintf("failed to connect to parser service: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		bodyStr := string(body)
		if len(bodyStr) > 500 {
			bodyStr = bodyStr[:500] + "..."
		}
		log.Printf("[INGESTION] CATALOG: parser returned status=%d, body=%s", resp.StatusCode, bodyStr)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "parser service returned error",
			"status":  resp.StatusCode,
			"details": bodyStr,
		})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[INGESTION] CATALOG: ERROR reading response - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read parser response"})
		return
	}

	if source == "freekino" {
		log.Printf("[FREEKINO] parser status=%d raw_len=%d", resp.StatusCode, len(body))
	}

	if len(body) == 0 {
		c.JSON(http.StatusOK, CatalogResponse{
			Items:      []CatalogItem{},
			Page:       page,
			Limit:      limit,
			Total:      0,
			TotalPages: 0,
			HasMore:    false,
		})
		return
	}

	result, err := parseCatalogResponseFlexible(body, page, limit)
	if err != nil {
		if source == "freekino" {
			log.Printf("[FREEKINO] parse error raw_prefix=%s", parserRawPrefix(body, 300))
		}
		log.Printf("[INGESTION] CATALOG: ERROR decoding JSON - %v", err)
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "failed to parse parser response",
			"details": fmt.Sprintf("JSON decode error: %v", err),
			"status":  resp.StatusCode,
			"raw":     parserRawPrefix(body, 300),
		})
		return
	}

	// Ensure items is never nil
	if result.Items == nil {
		result.Items = []CatalogItem{}
	}

	// Calculate pagination
	if result.TotalPages == 0 && result.Total > 0 {
		result.TotalPages = (result.Total + limit - 1) / limit
	}
	result.HasMore = page < result.TotalPages

	log.Printf("[INGESTION] CATALOG: success - %d items, page %d/%d", len(result.Items), page, result.TotalPages)
	c.JSON(http.StatusOK, result)
}

// CatalogCategory represents a single genre/category from a source site
type CatalogCategory struct {
	ID   string `json:"id"` // same as slug, for frontend keying
	Name string `json:"name"`
	URL  string `json:"url"`
	Slug string `json:"slug"`
}

// GetCatalogCategories returns genre/category links for a source
// GET /api/ingestion/catalog/categories?source=uzmovi
func (h *IngestionHandler) GetCatalogCategories(c *gin.Context) {
	source := c.Query("source")
	if strings.Contains(source, ".") {
		source = strings.Split(source, ".")[0]
	}

	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinochilar": true,
		"uzmedia":    true,
		"kinolar":    true,
	}
	if !validSources[source] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia, kinochilar, uzmedia, kinolar"})
		return
	}

	parserBaseURL := h.parserURL
	if parserBaseURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	parserURL := fmt.Sprintf("%s/categories?source=%s", parserBaseURL, source)
	log.Printf("[INGESTION] CATEGORIES: calling parser: %s", parserURL)

	resp, err := h.httpClient.Get(parserURL)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "parser service unavailable", "details": err.Error()})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		c.JSON(http.StatusBadGateway, gin.H{"error": "parser returned error", "details": string(body)})
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read parser response"})
		return
	}

	var result struct {
		Source     string            `json:"source"`
		Categories []CatalogCategory `json:"categories"`
	}
	if err := json.Unmarshal(body, &result); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse parser response"})
		return
	}

	if result.Categories == nil {
		result.Categories = []CatalogCategory{}
	}

	// Populate ID from slug for frontend compatibility
	for i := range result.Categories {
		if result.Categories[i].ID == "" {
			result.Categories[i].ID = result.Categories[i].Slug
		}
	}

	log.Printf("[INGESTION] CATEGORIES: source=%s count=%d", source, len(result.Categories))
	c.JSON(http.StatusOK, result)
}

// CreateManualJob creates an ingestion job from a direct video URL
// POST /api/ingestion/manual
func (h *IngestionHandler) CreateManualJob(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] CreateManualJob panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	var input struct {
		VideoURL string `json:"video_url" binding:"required"`
		Title    string `json:"title"`
		Year     int    `json:"year"`
		Poster   string `json:"poster"`
		Backdrop string `json:"backdrop"`
		Type     string `json:"type"` // "movie" or "serial", default "movie"
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[INGESTION] MANUAL: invalid request - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] MANUAL: creating manual job - url=%s, title=%s", input.VideoURL, input.Title)

	// Validate video URL
	if input.VideoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "video_url is required"})
		return
	}

	// Generate a unique source_id for manual imports
	sourceID := fmt.Sprintf("manual-%d", time.Now().UnixNano())

	// Create job
	job := &models.IngestionJob{
		Title:    input.Title,
		Source:   "manual",
		SourceID: sourceID,
		// For manual imports, we store the video URL in DetailURL
		DetailURL: input.VideoURL,
		Status:    models.IngestionStatusQueued,
		Stage:     string(models.IngestionStatusQueued),
		Progress:  0,
		Steps:     models.JobSteps{},
		Logs:      []models.IngestionLog{},
		Metadata: &models.ParsedMovieMetadata{
			Title:        input.Title,
			Year:         input.Year,
			Poster:       input.Poster,
			Backdrop:     input.Backdrop,
			VideoPageURL: input.VideoURL,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.jobRepo.Create(ctx, job); err != nil {
		log.Printf("[INGESTION] MANUAL: failed to create job - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create job"})
		return
	}

	log.Printf("[INGESTION] MANUAL: created job %s", job.ID.Hex())
	c.JSON(http.StatusCreated, gin.H{"data": job, "message": "Manual ingestion job created"})
}

// ImportFromCatalog imports an item from the catalog
// POST /api/ingestion/import
func (h *IngestionHandler) ImportFromCatalog(c *gin.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[PANIC RECOVERY] ImportFromCatalog panicked: %v\n%s", r, debug.Stack())
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "Internal server error",
				"details": fmt.Sprintf("recovered from panic: %v", r),
			})
		}
	}()

	var input struct {
		Source         string      `json:"source" binding:"required"`
		SourceID       string      `json:"source_id"`
		DetailURL      string      `json:"detail_url"`
		Title          string      `json:"title"`
		Type           string      `json:"type"` // "movie" or "serial"
		Year           interface{} `json:"year"`
		Poster         string      `json:"poster"`
		Quality        string      `json:"quality"`
		ForceConfirmed bool        `json:"force_confirmed"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[INGESTION] IMPORT: invalid request - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Normalize year to int
	finalYear := parserValueAsInt(input.Year)

	// Always re-fetch /details before import and verify that the selected card
	// still matches the parser detail page. This blocks wrong-movie imports when
	// search cards are ambiguous or stale.
	if input.DetailURL != "" {
		detailsURL := fmt.Sprintf("%s/details?source=%s&url=%s", h.parserURL, input.Source, url.QueryEscape(input.DetailURL))
		log.Printf("[DIRECT_IMPORT] fetching details url=%s source=%s", input.DetailURL, input.Source)
		resp, err := h.httpClient.Get(detailsURL)
		if err == nil && resp != nil {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			var details map[string]interface{}
			if len(body) > 0 {
				json.Unmarshal(body, &details)
			}
			// Treat 200 and 422 alike for metadata pickup — 422 still contains
			// title/source_id/type when the parser couldn't resolve a video URL.
			if resp.StatusCode == 200 || resp.StatusCode == 422 {
				selectedIdentity := importIdentitySnapshot{
					Source:    input.Source,
					SourceID:  input.SourceID,
					DetailURL: input.DetailURL,
					Title:     input.Title,
					Year:      finalYear,
					Type:      input.Type,
					Poster:    input.Poster,
				}
				fetchedIdentity := importIdentitySnapshot{
					Source:    input.Source,
					SourceID:  parserValueAsString(details["source_id"]),
					DetailURL: parserValueAsString(details["detail_url"]),
					Title:     parserValueAsString(details["title"]),
					Year:      parserValueAsInt(details["year"]),
					Type:      parserValueAsString(details["type"]),
					Poster:    parserValueAsString(details["poster"]),
				}
				if fetchedIdentity.DetailURL == "" {
					fetchedIdentity.DetailURL = input.DetailURL
				}
				if fetchedIdentity.SourceID == "" {
					fetchedIdentity.SourceID = input.SourceID
				}
				confidence := identityConfidence(selectedIdentity, fetchedIdentity)
				log.Printf("[identity] selected=%s", identityLogString(selectedIdentity))
				log.Printf("[identity] fetched=%s", identityLogString(fetchedIdentity))
				log.Printf("[identity] confidence=%.3f", confidence)
				if !input.ForceConfirmed && (normalizeIdentityType(fetchedIdentity.Type) == "unknown" || confidence < 0.85) {
					c.JSON(http.StatusConflict, gin.H{
						"error":                 "admin confirmation required",
						"reason":                "selected result did not confidently match fetched detail page",
						"confidence":            confidence,
						"selected":              selectedIdentity,
						"fetched":               fetchedIdentity,
						"requires_confirmation": true,
					})
					return
				}
				if input.ForceConfirmed {
					log.Printf("[identity] force_confirmed=true accepted confidence=%.3f", confidence)
				}
				if sid, ok := details["source_id"].(string); ok && sid != "" && input.SourceID == "" {
					input.SourceID = sid
				}
				if input.Title == "" {
					if t, ok := details["title"].(string); ok {
						input.Title = t
					}
				}
				if input.Type == "" {
					if t, ok := details["type"].(string); ok {
						input.Type = t
					}
				}
				if input.Year == 0 {
					input.Year = parserValueAsInt(details["year"])
				}
				if input.Poster == "" {
					input.Poster = parserValueAsString(details["poster"])
				}
				if input.Title == "" {
					input.Title = fetchedIdentity.Title
				}
				if input.SourceID == "" {
					input.SourceID = fetchedIdentity.SourceID
				}
				if input.DetailURL == "" {
					input.DetailURL = fetchedIdentity.DetailURL
				}
			} else {
				log.Printf("[DIRECT_IMPORT] parser /details returned status=%d", resp.StatusCode)
			}
		} else if err != nil {
			log.Printf("[DIRECT_IMPORT] parser /details request failed: %v", err)
			if resp != nil {
				resp.Body.Close()
			}
		}
	}

	// Reject when content_type is unknown — never silently default to "movie".
	normalizedType := strings.ToLower(strings.TrimSpace(input.Type))
	if normalizedType == "series" {
		normalizedType = "serial"
	}
	if normalizedType != "" && normalizedType != "movie" && normalizedType != "serial" {
		log.Printf("[DIRECT_IMPORT] unsupported content_type=%q url=%s", input.Type, input.DetailURL)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":      "Content type could not be detected",
			"reason":     fmt.Sprintf("parser returned unsupported type: %q", input.Type),
			"detail_url": input.DetailURL,
			"source":     input.Source,
		})
		return
	}
	if normalizedType == "" {
		log.Printf("[DIRECT_IMPORT] missing content_type for url=%s source=%s", input.DetailURL, input.Source)
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":      "Content type could not be detected",
			"reason":     "parser did not return a movie/serial classification",
			"detail_url": input.DetailURL,
			"source":     input.Source,
		})
		return
	}
	input.Type = normalizedType
	log.Printf("[DIRECT_IMPORT] url=%s source=%s content_type=%s", input.DetailURL, input.Source, input.Type)

	// If still no source_id, generate from detail_url
	if input.SourceID == "" && input.DetailURL != "" {
		parts := strings.Split(input.DetailURL, "/")
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			last = strings.TrimSuffix(last, ".html")
			last = strings.TrimSuffix(last, ".htm")
			input.SourceID = last
		}
	}

	log.Printf("[INGESTION] IMPORT: importing - source=%s, source_id=%s, type=%s, title=%s", input.Source, input.SourceID, input.Type, input.Title)

	// Validate source
	validSources := map[string]bool{
		"uzmovi":     true,
		"freekino":   true,
		"asilmedia":  true,
		"kinolar":    true,
		"kinochilar": true,
		"uzmedia":    true,
		"manual":     true,
	}
	if !validSources[input.Source] {
		log.Printf("[INGESTION] IMPORT: invalid source=%s", input.Source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia, kinolar, kinochilar, uzmedia, manual"})
		return
	}

	// Serial branch — call parser /serial-details and fan out one job per episode.
	if input.Type == "serial" {
		if input.DetailURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "detail_url is required for serial import"})
			return
		}
		h.importSerial(c, input.Source, input.SourceID, input.DetailURL, input.Title)
		return
	}

	// Create job — content_type comes from detection above (movie at this point;
	// the serial branch returned earlier).
	job := &models.IngestionJob{
		Title:       input.Title,
		Source:      input.Source,
		SourceID:    input.SourceID,
		DetailURL:   input.DetailURL,
		Status:      models.IngestionStatusQueued,
		Stage:       string(models.IngestionStatusQueued),
		Progress:    0,
		Steps:       models.JobSteps{},
		Logs:        []models.IngestionLog{},
		ContentType: input.Type,
		Quality:     input.Quality,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := h.jobRepo.Create(ctx, job); err != nil {
		// Check for duplicate
		log.Printf("[INGESTION] IMPORT: failed to create job - %v", err)
		c.JSON(http.StatusConflict, gin.H{"error": "job already exists for this source and source_id"})
		return
	}

	log.Printf("[INGESTION] IMPORT: created job %s", job.ID.Hex())
	c.JSON(http.StatusCreated, gin.H{"data": job, "message": "Ingestion job created from catalog"})
}
