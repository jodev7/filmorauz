package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/worker/storage"
)

// ProcessRequest represents a request to process a video
type ProcessRequest struct {
	Source       string  `json:"source"`
	SourceID     string  `json:"source_id,omitempty"`
	SourceURL    string  `json:"source_url,omitempty"`
	Title        string  `json:"title,omitempty"`
	InputFile    string  `json:"input_file"`              // Required: path to downloaded file
	OutputFile   string  `json:"output_file,omitempty"`   // Optional: output filename
	CutSeconds   int     `json:"cut_seconds,omitempty"`   // Optional: skip first N seconds
	LogoPath     string  `json:"logo_path,omitempty"`     // Optional: watermark overlay
	LogoPosition string  `json:"logo_position,omitempty"` // Optional: position like "W-w-10:10"
	LogoOpacity  float64 `json:"logo_opacity,omitempty"`  // Optional: 0.0-1.0
}

// ProcessResponse represents the response from processing
type ProcessResponse struct {
	Success       bool   `json:"success"`
	InputFile     string `json:"input_file"`
	OutputFile    string `json:"output_file,omitempty"`
	CutSeconds    int    `json:"cut_seconds"`
	LogoApplied   bool   `json:"logo_applied"`
	FFmpegCommand string `json:"ffmpeg_command,omitempty"`
	Error         string `json:"error,omitempty"`
	Duration      string `json:"duration,omitempty"`
}

// ProcessHandler handles video processing requests
type ProcessHandler struct {
	outputDir string
	tempDir   string
	b2Store   storage.Storage
}

// NewProcessHandler creates a new process handler
func NewProcessHandler(outputDir, tempDir string, b2Store storage.Storage) *ProcessHandler {
	// Ensure directories exist
	os.MkdirAll(outputDir, 0755)
	os.MkdirAll(tempDir, 0755)

	return &ProcessHandler{
		outputDir: outputDir,
		tempDir:   tempDir,
		b2Store:   b2Store,
	}
}

// ServeHTTP handles HTTP requests
func (h *ProcessHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("[WORKER] %s %s", r.Method, r.URL.Path)

	// Get presigned upload URL for direct B2 upload
	if r.Method == http.MethodGet && r.URL.Path == "/get-upload-url" {
		h.handleGetUploadURL(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/upload-profile" {
		h.handleUploadProfile(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/upload-telegram-post" {
		h.handleUploadTelegramPost(w, r)
		return
	}

	if r.Method == http.MethodPost && r.URL.Path == "/upload-temp-movie" {
		h.handleUploadTempMovie(w, r)
		return
	}

	if r.Method == http.MethodPost && (r.URL.Path == "/upload-poster" || r.URL.Path == "/upload-backdrop") {
		h.handleUploadMovieImage(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.URL.Path != "/process" {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	h.handleProcess(w, r)
}

func (h *ProcessHandler) handleProcess(w http.ResponseWriter, r *http.Request) {
	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.sendError(w, fmt.Sprintf("Failed to read request body: %v", err), http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Parse request
	var req ProcessRequest
	if err := json.Unmarshal(body, &req); err != nil {
		h.sendError(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.InputFile == "" {
		h.sendError(w, "input_file is required", http.StatusBadRequest)
		return
	}

	log.Printf("[WORKER] Process request: input=%s, output=%s, cut=%d, logo=%s",
		req.InputFile, req.OutputFile, req.CutSeconds, req.LogoPath)

	// Process the video
	resp := h.processVideo(req)

	// Send response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *ProcessHandler) processVideo(req ProcessRequest) *ProcessResponse {
	resp := &ProcessResponse{
		Success:     false,
		InputFile:   req.InputFile,
		CutSeconds:  req.CutSeconds,
		LogoApplied: req.LogoPath != "" && req.LogoPath != "none",
	}

	// Validate input file exists
	if _, err := os.Stat(req.InputFile); os.IsNotExist(err) {
		resp.Error = fmt.Sprintf("Input file does not exist: %s", req.InputFile)
		return resp
	}

	// Determine output file
	outputFile := req.OutputFile
	if outputFile == "" {
		// Generate output filename from input
		baseName := filepath.Base(req.InputFile)
		ext := filepath.Ext(baseName)
		nameWithoutExt := strings.TrimSuffix(baseName, ext)
		outputFile = filepath.Join(h.outputDir, nameWithoutExt+"_processed.mp4")
	} else if !filepath.IsAbs(outputFile) {
		// Make relative paths absolute
		outputFile = filepath.Join(h.outputDir, outputFile)
	}

	// Ensure output directory exists
	outDir := filepath.Dir(outputFile)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		resp.Error = fmt.Sprintf("Failed to create output directory: %v", err)
		return resp
	}

	// Build FFmpeg command
	cmd := h.buildFFmpegCommand(req, outputFile)
	resp.FFmpegCommand = strings.Join(cmd.Args, " ")

	log.Printf("[WORKER] Running FFmpeg: %s", resp.FFmpegCommand)

	// Execute FFmpeg
	startTime := time.Now()
	output, err := cmd.CombinedOutput()
	elapsed := time.Since(startTime)

	if err != nil {
		resp.Error = fmt.Sprintf("FFmpeg failed: %v\nOutput: %s", err, string(output))
		log.Printf("[WORKER] FFmpeg failed: %v", err)
		return resp
	}

	// Verify output file exists
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		resp.Error = "FFmpeg completed but output file not found"
		return resp
	}

	resp.Success = true
	resp.OutputFile = outputFile
	resp.Duration = elapsed.Round(time.Second).String()

	log.Printf("[WORKER] Processing completed: %s -> %s (took %s)", req.InputFile, outputFile, resp.Duration)

	return resp
}

func (h *ProcessHandler) buildFFmpegCommand(req ProcessRequest, outputFile string) *exec.Cmd {
	// Base FFmpeg arguments
	args := []string{
		"-y", // Overwrite output
		"-i", req.InputFile,
		"-c:v", "libx264", // Video codec
		"-preset", "medium",
		"-crf", "23",
		"-c:a", "aac", // Audio codec
		"-b:a", "128k",
		"-movflags", "+faststart", // Enable fast start for web playback
	}

	// Add cut/trim if specified
	if req.CutSeconds > 0 {
		args = append(args, "-ss", fmt.Sprintf("%d", req.CutSeconds))
		log.Printf("[WORKER] Will skip first %d seconds", req.CutSeconds)
	}

	// Always resolve and apply logo from docs/logo.png.
	// Never rely on the caller passing logo_path — watermark must always be applied.
	resolvedLogo := resolveLogoPath()
	if resolvedLogo != "" {
		// Use the same filter as createBaseVideo: scale to 18% of video width,
		// overlay at bottom-right with 20px padding.
		videoChain := "scale=trunc(iw/2)*2:trunc(ih/2)*2"
		filterComplex := fmt.Sprintf(
			"[0:v]%s[vscaled];[1:v][vscaled]scale2ref=w=iw*0.18:h=-1[logo][vref];[vref][logo]overlay=W-w-20:H-h-20:shortest=1[out]",
			videoChain,
		)
		args = append(args,
			"-loop", "1",
			"-i", resolvedLogo,
			"-filter_complex", filterComplex,
			"-map", "[out]",
			"-map", "0:a?",
		)
		log.Printf("[WORKER] Applying logo overlay: %s", resolvedLogo)
	} else {
		log.Printf("[WORKER] WARNING: docs/logo.png not found — skipping watermark")
	}

	// Output file
	args = append(args, outputFile)

	return exec.Command("ffmpeg", args...)
}

func (h *ProcessHandler) sendError(w http.ResponseWriter, message string, status int) {
	log.Printf("[WORKER] Error: %s", message)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": false,
		"error":   message,
	})
}

// resolveLogoPath finds docs/logo.png relative to CWD first, then the
// executable's directory — matching the same lookup order as createBaseVideo.
func resolveLogoPath() string {
	if cwd, err := os.Getwd(); err == nil {
		if p := filepath.Join(cwd, "docs", "logo.png"); fileExistsHTTP(p) {
			return p
		}
	}
	if exe, err := os.Executable(); err == nil {
		if p := filepath.Join(filepath.Dir(exe), "docs", "logo.png"); fileExistsHTTP(p) {
			return p
		}
	}
	return ""
}

func fileExistsHTTP(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// sanitizeKeySegment keeps only lowercase letters, digits, '-', '_', '.'. Slash is
// not allowed here; apply this to filename segments, not full paths.
func sanitizeKeySegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + 32)
		case r == '-' || r == '_' || r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func safeExtension(originalFilename, contentType, fallback string) string {
	switch strings.ToLower(strings.TrimSpace(contentType)) {
	case "image/jpeg", "image/jpg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "video/webm":
		return ".webm"
	case "video/ogg":
		return ".ogg"
	case "video/quicktime":
		return ".mov"
	}

	ext := sanitizeKeySegment(strings.ToLower(filepath.Ext(originalFilename)))
	if ext == "" || ext == "." {
		return fallback
	}
	return ext
}

func safeUploadKey(prefix, label, originalFilename, contentType, fallbackExt string) string {
	ext := safeExtension(originalFilename, contentType, fallbackExt)
	base := fmt.Sprintf("%d", time.Now().UnixNano())
	if label != "" {
		base += "_" + sanitizeKeySegment(label)
	}
	return strings.Trim(prefix, "/") + "/" + base + ext
}

func (h *ProcessHandler) handleUploadProfile(w http.ResponseWriter, r *http.Request) {
	if h.b2Store == nil {
		h.sendError(w, "B2 storage not configured", http.StatusServiceUnavailable)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		h.sendError(w, fmt.Sprintf("no file provided: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.sendError(w, fmt.Sprintf("failed to read file: %v", err), http.StatusBadRequest)
		return
	}

	filename := safeUploadKey("images/profile", "profile", header.Filename, header.Header.Get("Content-Type"), ".jpg")

	log.Printf("[B2] profile final object key: %s", filename)
	log.Printf("[B2] Uploading profile image: bucket=filmorauznet, key=%s, size=%d", filename, len(data))

	publicURL, err := h.b2Store.UploadData(filename, data, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("[B2] Upload failed: %v", err)
		h.sendError(w, fmt.Sprintf("B2 upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[B2] Upload success: %s", publicURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": publicURL,
	})
}

func (h *ProcessHandler) handleUploadTelegramPost(w http.ResponseWriter, r *http.Request) {
	if h.b2Store == nil {
		h.sendError(w, "B2 storage not configured", http.StatusServiceUnavailable)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		h.sendError(w, fmt.Sprintf("no file provided: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.sendError(w, fmt.Sprintf("failed to read file: %v", err), http.StatusBadRequest)
		return
	}

	mediaKey := safeUploadKey("images/telegram-post", "telegram_post", header.Filename, header.Header.Get("Content-Type"), ".jpg")

	log.Printf("[B2] telegram-post final object key: %s", mediaKey)
	log.Printf("[B2] Uploading telegram post: bucket=filmorauznet, key=%s, size=%d", mediaKey, len(data))

	publicURL, err := h.b2Store.UploadData(mediaKey, data, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("[B2] Upload failed: %v", err)
		h.sendError(w, fmt.Sprintf("B2 upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[B2] Upload success: %s", publicURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": publicURL,
	})
}

// handleUploadTempMovie handles direct MP4 upload to B2 temp path
// This stores raw movie files in temp/movies/ for the ingestion pipeline
func (h *ProcessHandler) handleUploadTempMovie(w http.ResponseWriter, r *http.Request) {
	if h.b2Store == nil {
		h.sendError(w, "B2 storage not configured", http.StatusServiceUnavailable)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		h.sendError(w, fmt.Sprintf("no file provided: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.sendError(w, fmt.Sprintf("failed to read file: %v", err), http.StatusBadRequest)
		return
	}

	tempKey := safeUploadKey("temp/raw", "temp_movie", header.Filename, header.Header.Get("Content-Type"), ".mp4")

	log.Printf("[B2] temp-movie final object key: %s", tempKey)
	log.Printf("[B2] Uploading temp movie: key=%s, size=%d", tempKey, len(data))

	publicURL, err := h.b2Store.UploadData(tempKey, data, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("[B2] Temp movie upload failed: %v", err)
		h.sendError(w, fmt.Sprintf("B2 upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[B2] Temp movie upload success: %s", publicURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url":      publicURL,
		"temp_key": tempKey,
	})
}

// handleUploadMovieImage handles poster/backdrop image uploads for movies
// Uploads to temp/posters or temp/backdrops path in B2 (temp storage for ingestion)
func (h *ProcessHandler) handleUploadMovieImage(w http.ResponseWriter, r *http.Request) {
	if h.b2Store == nil {
		h.sendError(w, "B2 storage not configured", http.StatusServiceUnavailable)
		return
	}

	path := r.URL.Path
	var imageType string
	if path == "/upload-poster" {
		imageType = "posters"
	} else if path == "/upload-backdrop" {
		imageType = "backdrops"
	} else {
		h.sendError(w, "Invalid upload path", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("image")
	if err != nil {
		h.sendError(w, fmt.Sprintf("no file provided: %v", err), http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		h.sendError(w, fmt.Sprintf("failed to read file: %v", err), http.StatusBadRequest)
		return
	}

	mediaKey := safeUploadKey("temp/"+imageType, strings.TrimSuffix(imageType, "s"), header.Filename, header.Header.Get("Content-Type"), ".jpg")

	log.Printf("[B2] movie-%s final object key: %s", imageType, mediaKey)
	log.Printf("[B2] Uploading movie %s: key=%s, size=%d", imageType, mediaKey, len(data))

	publicURL, err := h.b2Store.UploadData(mediaKey, data, header.Header.Get("Content-Type"))
	if err != nil {
		log.Printf("[B2] Movie image upload failed: %v", err)
		h.sendError(w, fmt.Sprintf("B2 upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("[B2] Movie image upload success: %s", publicURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"url": publicURL,
	})
}

// handleGetUploadURL returns a presigned upload URL for direct B2 upload
// GET /get-upload-url?type=poster&filename=abc.jpg
func (h *ProcessHandler) handleGetUploadURL(w http.ResponseWriter, r *http.Request) {
	typeParam := r.URL.Query().Get("type")
	filename := r.URL.Query().Get("filename")

	if typeParam == "" || filename == "" {
		h.sendError(w, "type and filename are required", http.StatusBadRequest)
		return
	}

	// Determine path based on type
	var tempPath string
	switch typeParam {
	case "poster":
		tempPath = safeUploadKey("temp/posters", "poster", filename, "", ".jpg")
	case "backdrop":
		tempPath = safeUploadKey("temp/backdrops", "backdrop", filename, "", ".jpg")
	case "video":
		tempPath = safeUploadKey("temp/raw", "video", filename, "", ".mp4")
	default:
		h.sendError(w, "invalid type: must be poster, backdrop, or video", http.StatusBadRequest)
		return
	}

	log.Printf("[B2] presigned-url final object key: %s", tempPath)

	// Get presigned upload URL
	uploadURL, authToken, err := h.b2Store.GetPresignedUploadURL(tempPath, "")
	if err != nil {
		log.Printf("[B2] GetUploadURL failed: %v", err)
		h.sendError(w, fmt.Sprintf("failed to get upload URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Get CDN URL for the file
	cdnURL := h.b2Store.GetURL(tempPath)

	log.Printf("[B2] Got upload URL for: %s, cdn: %s", tempPath, cdnURL)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"upload_url": uploadURL,
		"auth_token": authToken,
		"file_key":   tempPath,
		"cdn_url":    cdnURL,
	})
}
