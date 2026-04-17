package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"runtime/debug"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IngestionHandler handles ingestion API requests
type IngestionHandler struct {
	jobRepo    *repositories.JobRepository
	parserURL  string
	workerURL  string
	httpClient *http.Client
}

// NewIngestionHandler creates a new ingestion handler
func NewIngestionHandler(jobRepo *repositories.JobRepository, parserURL string, workerURL string) *IngestionHandler {
	// Create HTTP client with explicit timeout
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		// Transport with connection settings
		Transport: &http.Transport{
			MaxIdleConns:        10,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     30 * time.Second,
		},
	}

	log.Printf("[INGESTION] Handler initialized with parserURL: %s, workerURL: %s", parserURL, workerURL)

	return &IngestionHandler{
		jobRepo:    jobRepo,
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
	query := c.Query("q")

	log.Printf("[INGESTION] SEARCH: request received - source=%s, query=%s", source, query)

	if source == "" || query == "" {
		log.Printf("[INGESTION] SEARCH: missing required parameters")
		c.JSON(http.StatusBadRequest, gin.H{"error": "source and q parameters are required"})
		return
	}

	// Validate source
	validSources := map[string]bool{
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
		"kinolar":   true,
		"manual":    true,
	}
	if !validSources[source] {
		log.Printf("[INGESTION] SEARCH: invalid source=%s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia, kinolar, manual"})
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

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		bodyStr := string(body)
		if len(bodyStr) > 200 {
			bodyStr = bodyStr[:200] + "..."
		}
		log.Printf("[INGESTION] SEARCH: ERROR decoding JSON - %v, body: %s", err, bodyStr)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "failed to parse parser response",
			"details": fmt.Sprintf("JSON decode error: %v", err),
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
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
		"kinolar":   true,
		"manual":    true,
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
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
		"kinolar":   true,
		"manual":    true,
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
		Status:    models.IngestionStatusPending,
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
		Status:      models.IngestionStatusPending,
		Progress:    0,
		Steps:       models.JobSteps{},
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

// ListIngestionJobs lists all jobs with filters
// GET /api/ingestion/jobs?status=pending&limit=20&skip=0
func (h *IngestionHandler) ListIngestionJobs(c *gin.Context) {
	status := c.Query("status")
	limit := 20
	skip := 0

	if l := c.Query("limit"); l != "" {
		fmt.Sscanf(l, "%d", &limit)
	}
	if s := c.Query("skip"); s != "" {
		fmt.Sscanf(s, "%d", &skip)
	}

	filter := bson.M{}
	if status != "" {
		filter["status"] = status
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	jobs, err := h.jobRepo.List(ctx, filter, limit, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}

	// Ensure jobs is never nil - return empty array instead
	if jobs == nil {
		jobs = []*models.IngestionJob{}
	}

	total, _ := h.jobRepo.Count(ctx, filter)

	c.JSON(http.StatusOK, gin.H{
		"data":  jobs,
		"total": total,
		"limit": limit,
		"skip":  skip,
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
	if job.Status == models.IngestionStatusProcessing {
		c.JSON(http.StatusBadRequest, gin.H{"error": "job is already being processed"})
		return
	}

	// Determine the appropriate stage based on existing job state
	// If local_path exists and steps.download is true, go to processing stage
	// Otherwise, start from download stage
	newStage := "download"

	if job.LocalPath != "" && job.Steps.Download {
		// File already downloaded, go to processing
		newStage = "process"
		log.Printf("[INGESTION] RETRY: Job %s has local_path=%s, starting from process stage", id, job.LocalPath)
	} else if job.Steps.Download && job.LocalPath == "" {
		// Download marked complete but no local_path - restart from download
		newStage = "download"
		log.Printf("[INGESTION] RETRY: Job %s has steps.download but no local_path, restarting download", id)
	} else {
		log.Printf("[INGESTION] RETRY: Job %s starting from download stage", id)
	}

	// Update status to pending (worker will pick it up)
	if err := h.jobRepo.UpdateStatus(ctx, id, models.IngestionStatusPending, 0); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update job status"})
		return
	}

	// Update stage and status using UpdateProgress
	if err := h.jobRepo.UpdateProgress(ctx, id, &repositories.ProgressUpdate{
		Stage:    newStage,
		Status:   "pending",
		Progress: 0,
	}); err != nil {
		log.Printf("[INGESTION] WARNING: failed to update stage: %v", err)
	}

	// Clear error message using SetError with empty string
	if err := h.jobRepo.SetError(ctx, id, ""); err != nil {
		log.Printf("[INGESTION] WARNING: failed to clear error: %v", err)
	}

	log.Printf("[INGESTION] RETRY: Job %s restarted - status=pending, stage=%s", id, newStage)

	c.JSON(http.StatusOK, gin.H{"message": "job retry initiated", "stage": newStage})
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
		Stage           string  `json:"stage"`
		Status          string  `json:"status"`
		ProgressPercent int     `json:"progress_percent"`
		Progress        int     `json:"progress"` // Alternative field name
		DownloadedBytes int64   `json:"downloaded_bytes"`
		TotalBytes      int64   `json:"total_bytes"`
		SpeedMBps       float64 `json:"speed_mbps"`
		EtaSeconds      int     `json:"eta_seconds"`
		Message         string  `json:"message"`
		StepsDownload   bool    `json:"steps_download"`
		FilePath        string  `json:"file_path"` // Local file path after download completes
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
		Stage:           progress.Stage,
		Status:          progress.Status,
		Progress:        progressValue,
		DownloadedBytes: progress.DownloadedBytes,
		TotalBytes:      progress.TotalBytes,
		SpeedMBps:       progress.SpeedMBps,
		EtaSeconds:      progress.EtaSeconds,
		Message:         progress.Message,
		StepsDownload:   progress.StepsDownload,
		FilePath:        progress.FilePath,
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

// DeleteIngestionJob deletes a job
// DELETE /api/ingestion/jobs/:id
func (h *IngestionHandler) DeleteIngestionJob(c *gin.Context) {
	_ = c.Param("id")

	// For now, we don't actually delete - just mark as cancelled
	// In production, you'd implement actual deletion
	c.JSON(http.StatusOK, gin.H{"message": "Job deletion not implemented"})
}

// Ensure types are used
var (
	_ primitive.ObjectID
	_ time.Time
)

// CatalogItem represents an item from the source catalog
type CatalogItem struct {
	SourceID    string   `json:"source_id"`
	Title       string   `json:"title"`
	Year        int      `json:"year"`
	Type        string   `json:"type"` // "movie" or "serial"
	Poster      string   `json:"poster"`
	Description string   `json:"description"`
	Genres      []string `json:"genres"`
	DetailURL   string   `json:"detail_url"`
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
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
	}
	if !validSources[source] {
		log.Printf("[INGESTION] CATALOG: invalid source=%s", source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source for catalog. valid: uzmovi, freekino, asilmedia"})
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

	var result CatalogResponse
	if err := json.Unmarshal(body, &result); err != nil {
		log.Printf("[INGESTION] CATALOG: ERROR decoding JSON - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse parser response"})
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

	validSources := map[string]bool{
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
	}
	if !validSources[source] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia"})
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
		Status:    models.IngestionStatusPending,
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
		Source    string `json:"source" binding:"required"`
		SourceID  string `json:"source_id" binding:"required"`
		DetailURL string `json:"detail_url"`
		Title     string `json:"title"`
		Type      string `json:"type"` // "movie" or "serial"
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[INGESTION] IMPORT: invalid request - %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] IMPORT: importing - source=%s, source_id=%s, title=%s", input.Source, input.SourceID, input.Title)

	// Validate source
	validSources := map[string]bool{
		"uzmovi":    true,
		"freekino":  true,
		"asilmedia": true,
		"manual":    true,
	}
	if !validSources[input.Source] {
		log.Printf("[INGESTION] IMPORT: invalid source=%s", input.Source)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid source. valid: uzmovi, freekino, asilmedia, manual"})
		return
	}

	// Create job
	job := &models.IngestionJob{
		Title:     input.Title,
		Source:    input.Source,
		SourceID:  input.SourceID,
		DetailURL: input.DetailURL,
		Status:    models.IngestionStatusPending,
		Progress:  0,
		Steps:     models.JobSteps{},
		Logs:      []models.IngestionLog{},
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
