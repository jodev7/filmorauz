package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

const (
	maxFileSize = 5 * 1024 * 1024 // 5MB
)

var allowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
}

// UploadHandler handles file uploads for profile images
type UploadHandler struct {
	userRepo *repositories.UserRepository
	config   *config.Config
}

// NewUploadHandler creates a new upload handler
func NewUploadHandler(userRepo *repositories.UserRepository, cfg *config.Config) *UploadHandler {
	return &UploadHandler{
		userRepo: userRepo,
		config:   cfg,
	}
}

// UploadProfileImage handles profile image upload
// Supports both file upload and URL input
func (h *UploadHandler) UploadProfileImage(c *gin.Context) {
	// Get user from context (set by middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check if this is a URL update (form field "image_url")
	imageURL := c.PostForm("image_url")
	if imageURL != "" {
		// URL mode - validate and save
		if !isValidURL(imageURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid URL format"})
			return
		}

		// Update with URL
		err := h.userRepo.UpdateProfileImage(userID.(string), "", imageURL)
		if err != nil {
			log.Printf("[UPLOAD] Error updating profile image URL: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":       "Profile image updated",
			"profile_image": imageURL,
		})
		return
	}

	// File upload mode
	file, header, err := c.Request.FormFile("image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Validate file size
	if header.Size > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 5MB)"})
		return
	}

	// Validate file type
	contentType := header.Header.Get("Content-Type")
	if !allowedImageTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type. allowed: jpg, jpeg, png, webp"})
		return
	}

	// Generate unique filename using timestamp
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}
	// Use timestamp for unique filename
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d%s", timestamp, ext)

	// Process based on environment
	var savedURL string

	if h.config.IsDev {
		// DEV mode - save locally
		savedURL, err = h.saveLocal(file, filename)
		if err != nil {
			log.Printf("[UPLOAD] Error saving locally: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}
	} else {
		// PROD mode - use worker B2 upload
		workerURL := h.config.WorkerUploadURL
		if workerURL == "" {
			log.Printf("[UPLOAD] Error: WORKER_UPLOAD_URL not configured")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload service not configured"})
			return
		}

		// Seek to beginning of file
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			log.Printf("[UPLOAD] Error seeking file: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}

		// Create multipart form
		var b bytes.Buffer
		wr := multipart.NewWriter(&b)
		part, err := wr.CreateFormFile("image", filename)
		if err != nil {
			log.Printf("[UPLOAD] Error creating form: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload form"})
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			log.Printf("[UPLOAD] Error copying to form: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}
		wr.Close()

		// Upload to worker
		resp, err := http.Post(workerURL+"/upload-profile", wr.FormDataContentType(), &b)
		if err != nil {
			log.Printf("[UPLOAD] Error uploading to worker: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[UPLOAD] Worker returned status %d: %s", resp.StatusCode, body)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
			return
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			log.Printf("[UPLOAD] Error decoding response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid upload response"})
			return
		}

		savedURL = result["url"]
		if savedURL == "" {
			log.Printf("[UPLOAD] Worker returned empty URL")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed - no URL returned"})
			return
		}
	}

	// Update user profile with new image URL
	err = h.userRepo.UpdateProfileImage(userID.(string), "", savedURL)
	if err != nil {
		log.Printf("[UPLOAD] Error updating profile image: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update profile"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "Profile image uploaded",
		"profile_image_url": savedURL,
	})
}

// saveLocal saves file to local storage (development)
func (h *UploadHandler) saveLocal(file multipart.File, filename string) (string, error) {
	// Create uploads directory if it doesn't exist
	storageDir := "./uploads"
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create uploads directory: %w", err)
	}

	// Create file path
	filePath := filepath.Join(storageDir, filename)

	// Create the file on disk
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	// Copy file content
	_, err = io.Copy(dst, file)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	// Return full URL with backend host and port
	if h.config.IsDev {
		return h.config.GetBaseURL() + "/" + filename, nil
	}
	return h.config.GetCDNFileURL("images/" + filename), nil
}

// saveToCDN saves file to CDN/storage (production)
func (h *UploadHandler) saveToCDN(file multipart.File, filename string, subDir string) (string, error) {
	baseURL := h.config.GetCDNFileURL(subDir + "/" + filename)
	if baseURL == "" {
		return "", fmt.Errorf("CDN_URL not configured for production mode")
	}
	log.Printf("[UPLOAD] Using CDN URL: %s", baseURL)
	return baseURL, nil
}

// saveToCDNWithKey saves file to CDN using the exact B2 object key
func (h *UploadHandler) saveToCDNWithKey(file multipart.File, mediaKey string) (string, error) {
	cdnURL := h.config.GetCDNFileURL(mediaKey)
	if cdnURL == "" {
		return "", fmt.Errorf("CDN_URL not configured for production mode")
	}
	log.Printf("[UPLOAD] Using CDN URL (key=%s): %s", mediaKey, cdnURL)
	return cdnURL, nil
}

// UploadMovieAssets handles file uploads for movie assets (poster, backdrop, video, temp_movie)
func (h *UploadHandler) UploadMovieAssets(c *gin.Context) {
	fileType := c.PostForm("type")
	if fileType != "poster" && fileType != "backdrop" && fileType != "video" && fileType != "temp_movie" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be 'poster', 'backdrop', 'video', or 'temp_movie'"})
		return
	}

	// Handle temp_movie (raw MP4 for ingestion pipeline)
	if fileType == "temp_movie" {
		h.uploadTempMovie(c)
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	var maxSize int64 = 10 * 1024 * 1024 // 10MB for images, override for video
	var allowedTypes map[string]bool

	switch fileType {
	case "poster", "backdrop":
		maxSize = 10 * 1024 * 1024 // 10MB
		allowedTypes = allowedImageTypes
	case "video":
		maxSize = 5 * 1024 * 1024 * 1024 // 5GB for videos
		allowedTypes = map[string]bool{
			"video/mp4":             true,
			"video/webm":            true,
			"video/ogg":             true,
			"application/x-mpegURL": true,
		}
	}

	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		if fileType == "video" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type. allowed: mp4, webm, ogg, m3u8"})
		} else {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type. allowed: jpg, jpeg, png, webp"})
		}
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" && fileType == "video" {
		ext = ".mp4"
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), fileType, ext)

	var savedURL string

	if h.config.IsDev {
		savedURL, err = h.saveMovieAssetLocal(file, filename, fileType)
		if err != nil {
			log.Printf("[UPLOAD] Error saving movie asset locally: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}
	} else {
		savedURL, err = h.saveToCDN(file, filename, "movies/"+fileType+"s")
		if err != nil {
			log.Printf("[UPLOAD] Error saving movie asset to CDN: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message":  fmt.Sprintf("%s uploaded successfully", fileType),
		"url":      savedURL,
		"type":     fileType,
		"filename": filename,
	})
}

// saveMovieAssetLocal saves movie asset to local storage organized by type
func (h *UploadHandler) saveMovieAssetLocal(file multipart.File, filename string, fileType string) (string, error) {
	subDir := fileType + "s" // posters, backdrops, videos
	storageDir := filepath.Join(h.config.UploadsDir, "movies", subDir)

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	filePath := filepath.Join(storageDir, filename)

	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if h.config.IsDev {
		return h.config.GetBaseURL() + "/movies/" + subDir + "/" + filename, nil
	}
	return h.config.GetCDNFileURL("movies/" + subDir + "/" + filename), nil
}

// uploadTempMovie handles direct MP4 upload to B2 temp path for ingestion pipeline
// This endpoint receives raw movie files and uploads to temp storage (not final movies path)
func (h *UploadHandler) uploadTempMovie(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Validate it's a video file
	allowedTypes := map[string]bool{
		"video/mp4":       true,
		"video/webm":      true,
		"video/ogg":       true,
		"video/quicktime": true,
	}
	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type. allowed: mp4, webm, ogg, mov"})
		return
	}

	// Max file size: 10GB for raw movie files
	maxSize := int64(10 * 1024 * 1024 * 1024)
	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 10GB)"})
		return
	}

	// Generate temp filename with timestamp
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".mp4"
	}
	timestamp := time.Now().UnixNano()
	filename := fmt.Sprintf("%d%s", timestamp, ext)
	tempKey := "temp/movies/" + filename

	log.Printf("[TEMP_UPLOAD] Uploading temp movie: %s (size: %d)", tempKey, header.Size)

	var savedURL string
	if h.config.IsDev {
		// DEV mode - save locally
		savedURL, err = h.saveTempMovieLocal(file, filename)
		if err != nil {
			log.Printf("[TEMP_UPLOAD] Error saving temp movie locally: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save temp file"})
			return
		}
	} else {
		// PROD mode - upload to B2 temp path via worker
		workerURL := h.config.WorkerUploadURL
		if workerURL == "" {
			log.Printf("[TEMP_UPLOAD] Error: WORKER_UPLOAD_URL not configured")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload service not configured"})
			return
		}

		// Seek to beginning
		if _, err := file.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}

		// Create multipart form
		var b bytes.Buffer
		wr := multipart.NewWriter(&b)
		part, err := wr.CreateFormFile("file", filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload form"})
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}
		wr.Close()

		// Upload to worker for temp storage
		resp, err := http.Post(workerURL+"/upload-temp-movie", wr.FormDataContentType(), &b)
		if err != nil {
			log.Printf("[TEMP_UPLOAD] Error uploading to worker: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[TEMP_UPLOAD] Worker returned status %d: %s", resp.StatusCode, body)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
			return
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			log.Printf("[TEMP_UPLOAD] Error decoding response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid upload response"})
			return
		}

		savedURL = result["url"]
		if savedURL == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed - no URL returned"})
			return
		}
	}

	log.Printf("[TEMP_UPLOAD] Uploaded temp movie: url=%s", savedURL)

	c.JSON(http.StatusOK, gin.H{
		"message":  "Temp movie uploaded successfully",
		"url":      savedURL,
		"temp_key": tempKey,
		"filename": filename,
	})
}

// saveTempMovieLocal saves temp movie to local storage
func (h *UploadHandler) saveTempMovieLocal(file multipart.File, filename string) (string, error) {
	storageDir := filepath.Join(h.config.UploadsDir, "temp", "movies")
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create storage directory: %w", err)
	}

	filePath := filepath.Join(storageDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	_, err = io.Copy(dst, file)
	if err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return h.config.GetBaseURL() + "/temp/movies/" + filename, nil
}

// adMediaAllowedMIME maps canonical MIME types to their media category (image/video).
// Extension aliases (like "image/jpg") and generic types are handled separately below.
var adMediaAllowedMIME = map[string]string{
	"image/jpeg":      "image",
	"image/png":       "image",
	"image/webp":      "image",
	"image/gif":       "image",
	"video/mp4":       "video",
	"video/webm":      "video",
	"video/quicktime": "video", // .mov
	"video/x-msvideo": "video", // .avi
}

// extToMIME maps common extensions to their canonical MIME type for fallback sniffing.
var extToMIME = map[string]string{
	".jpg":  "image/jpeg",
	".jpeg": "image/jpeg",
	".png":  "image/png",
	".webp": "image/webp",
	".gif":  "image/gif",
	".mp4":  "video/mp4",
	".webm": "video/webm",
	".mov":  "video/quicktime",
}

// resolveAdMIME returns the canonical MIME type for an uploaded ad file.
// Resolution order: part Content-Type header → file extension → http.DetectContentType (first 512 bytes).
// Returns ("", "") if the type cannot be resolved.
func resolveAdMIME(partCT string, filename string, file io.ReadSeeker) (mimeType string, category string) {
	// 1. Trust the part header if it's not generic.
	ct := strings.ToLower(strings.TrimSpace(partCT))
	if ct != "" && ct != "application/octet-stream" {
		// Strip parameters like "; charset=utf-8"
		if idx := strings.Index(ct, ";"); idx != -1 {
			ct = strings.TrimSpace(ct[:idx])
		}
		if cat, ok := adMediaAllowedMIME[ct]; ok {
			return ct, cat
		}
		// Header present but unrecognised → reject early with the exact type
		return ct, ""
	}

	// 2. Fall back to extension.
	ext := strings.ToLower(filepath.Ext(filename))
	if mime, ok := extToMIME[ext]; ok {
		if cat, ok2 := adMediaAllowedMIME[mime]; ok2 {
			return mime, cat
		}
	}

	// 3. Sniff the first 512 bytes.
	buf := make([]byte, 512)
	n, _ := file.Read(buf)
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", ""
	}
	sniffed := http.DetectContentType(buf[:n])
	sniffed = strings.ToLower(strings.TrimSpace(sniffed))
	if idx := strings.Index(sniffed, ";"); idx != -1 {
		sniffed = strings.TrimSpace(sniffed[:idx])
	}
	if cat, ok := adMediaAllowedMIME[sniffed]; ok {
		return sniffed, cat
	}
	return sniffed, ""
}

// UploadAdMedia handles image/video uploads for ads
// POST /api/superadmin/ads/upload  (superadmin only)
func (h *UploadHandler) UploadAdMedia(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	// Resolve actual MIME type (handles empty/octet-stream part headers gracefully)
	partCT := header.Header.Get("Content-Type")
	contentType, detectedCategory := resolveAdMIME(partCT, header.Filename, file)
	if detectedCategory == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("unsupported file type: %q — accepted: JPG, PNG, WEBP, GIF, MP4, WEBM, MOV", contentType),
		})
		return
	}

	// Prefer explicit media_type from form field; otherwise use detected category.
	mediaType := c.PostForm("media_type")
	if mediaType != "image" && mediaType != "video" {
		mediaType = detectedCategory
	}

	// Validate that the file content matches the declared media_type.
	if mediaType != detectedCategory {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("file content is %s but media_type=%q was declared", detectedCategory, mediaType),
		})
		return
	}

	// Enforce size limits per category.
	var maxSize int64
	if mediaType == "image" {
		maxSize = 10 * 1024 * 1024 // 10 MB
	} else {
		maxSize = 500 * 1024 * 1024 // 500 MB
	}
	if header.Size > maxSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("file too large (max %dMB)", maxSize/1024/1024)})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext == "" {
		if mediaType == "image" {
			ext = ".jpg"
		} else {
			ext = ".mp4"
		}
	}

	filename := fmt.Sprintf("%d_ad_%s%s", time.Now().UnixNano(), mediaType, ext)

	var savedURL string
	if h.config.IsDev {
		savedURL, err = h.saveAdMediaLocal(file, filename, mediaType)
	} else {
		savedURL, err = h.saveToCDN(file, filename, "ads/"+mediaType+"s")
	}
	if err != nil {
		log.Printf("[UPLOAD] Ad media upload error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": savedURL, "media_type": mediaType})
}

// saveAdMediaLocal saves ad media to local storage under uploads/ads/{images|videos}/
func (h *UploadHandler) saveAdMediaLocal(file multipart.File, filename string, mediaType string) (string, error) {
	subDir := mediaType + "s" // images or videos
	storageDir := filepath.Join(h.config.UploadsDir, "ads", subDir)

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(storageDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	if h.config.IsDev {
		return h.config.GetBaseURL() + "/ads/" + subDir + "/" + filename, nil
	}
	return h.config.GetCDNFileURL("ads/" + subDir + "/" + filename), nil
}

// isValidURL validates a basic URL
func isValidURL(url string) bool {
	if url == "" {
		return false
	}
	// Basic URL validation
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return false
	}
	// Check for valid length
	if len(url) < 10 || len(url) > 500 {
		return false
	}
	return true
}

// GetStoragePath returns the local storage path for static file serving
func GetStoragePath() string {
	return "./storage/images"
}

// EnsureStorageDir creates the storage directory if it doesn't exist
func EnsureStorageDir() error {
	storageDir := "./storage/images"
	if _, err := os.Stat(storageDir); os.IsNotExist(err) {
		return os.MkdirAll(storageDir, 0755)
	}
	return nil
}

// InitUpload initializes upload-related configurations
func InitUpload() {
	// Ensure storage directory exists on startup
	if err := EnsureStorageDir(); err != nil {
		log.Printf("[UPLOAD] Warning: could not create storage directory: %v", err)
	}
	// Set storage directory modification time to prevent unused warnings
	_ = time.Now()
}

// UploadTelegramPostMedia handles image uploads for Telegram posts
// POST /api/superadmin/telegram-post/upload (superadmin only)
func (h *UploadHandler) UploadTelegramPostMedia(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	contentType := header.Header.Get("Content-Type")
	allowedTypes := map[string]bool{
		"image/jpeg": true,
		"image/jpg":  true,
		"image/png":  true,
		"image/webp": true,
		"image/gif":  true,
	}

	if header.Size > 10*1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large (max 10MB)"})
		return
	}

	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type"})
		return
	}

	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".jpg"
	}

	filename := fmt.Sprintf("%d_telegram_post%s", time.Now().UnixNano(), ext)

	var savedURL string
	if h.config.IsDev {
		savedURL, err = h.saveTelegramPostLocal(file, filename)
	} else {
		// Call worker for B2 upload
		workerURL := h.config.WorkerUploadURL
		if workerURL == "" {
			log.Printf("[UPLOAD] Error: WORKER_UPLOAD_URL not configured")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload service not configured"})
			return
		}

		if _, err := file.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}

		var b bytes.Buffer
		wr := multipart.NewWriter(&b)
		part, err := wr.CreateFormFile("image", filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload form"})
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}
		wr.Close()

		resp, err := http.Post(workerURL+"/upload-telegram-post", wr.FormDataContentType(), &b)
		if err != nil {
			log.Printf("[UPLOAD] Error uploading to worker: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[UPLOAD] Worker returned status %d: %s", resp.StatusCode, body)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
			return
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid upload response"})
			return
		}

		savedURL = result["url"]
		if savedURL == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed - no URL returned"})
			return
		}
	}
	if err != nil {
		log.Printf("[UPLOAD] Telegram post media upload error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": savedURL})
}

// saveTelegramPostLocal saves telegram post images to local storage
func (h *UploadHandler) saveTelegramPostLocal(file multipart.File, filename string) (string, error) {
	storageDir := filepath.Join(h.config.UploadsDir, "telegram-post")

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	filePath := filepath.Join(storageDir, filename)
	dst, err := os.Create(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to create file: %w", err)
	}
	defer dst.Close()

	if _, err = io.Copy(dst, file); err != nil {
		return "", fmt.Errorf("failed to write file: %w", err)
	}

	return h.config.GetBaseURL() + "/telegram-post/" + filename, nil
}

// UploadTemp handles proxy upload to B2 via worker (avoids CORS issue with direct browser upload)
func (h *UploadHandler) UploadTemp(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no file provided"})
		return
	}
	defer file.Close()

	fileType := c.PostForm("type")
	if fileType == "" {
		fileType = "video"
	}

	var allowedTypes map[string]bool
	var maxSize int64
	var ext string

	switch fileType {
	case "poster", "backdrop":
		maxSize = 20 * 1024 * 1024 // 20 MB for images
		allowedTypes = allowedImageTypes
	case "video", "temp_movie":
		maxSize = 5 * 1024 * 1024 * 1024
		allowedTypes = map[string]bool{
			"video/mp4":       true,
			"video/webm":      true,
			"video/ogg":       true,
			"video/quicktime": true,
		}
	default:
		maxSize = 20 * 1024 * 1024 // 20 MB for images
		allowedTypes = allowedImageTypes
	}

	if header.Size > maxSize {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"success": false, "error": "file too large"})
		return
	}

	contentType := header.Header.Get("Content-Type")
	if !allowedTypes[contentType] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid file type"})
		return
	}

	ext = filepath.Ext(header.Filename)
	if ext == "" {
		if fileType == "video" || fileType == "temp_movie" {
			ext = ".mp4"
		} else {
			ext = ".jpg"
		}
	}

	filename := fmt.Sprintf("%d_%s%s", time.Now().UnixNano(), fileType, ext)
	subDir := fileType + "s"
	if fileType == "temp_movie" {
		subDir = "temp/movies"
	}

	var savedURL string
	var fileKey string

	if h.config.IsDev {
		savedURL, err = h.saveMovieAssetLocal(file, filename, fileType)
		if err != nil {
			log.Printf("[UPLOAD_TEMP] Error saving locally: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save file"})
			return
		}
		fileKey = subDir + "/" + filename
	} else {
		workerURL := h.config.WorkerUploadURL
		if workerURL == "" {
			log.Printf("[UPLOAD_TEMP] Error: WORKER_UPLOAD_URL not configured")
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload service not configured"})
			return
		}

		if _, err := file.Seek(0, io.SeekStart); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}

		var b bytes.Buffer
		wr := multipart.NewWriter(&b)
		part, err := wr.CreateFormFile("file", filename)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create upload form"})
			return
		}
		if _, err := io.Copy(part, file); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to process file"})
			return
		}
		wr.Close()

		var endpoint string
		if fileType == "temp_movie" {
			endpoint = "/upload-temp-movie"
		} else if fileType == "poster" {
			endpoint = "/upload-poster"
		} else if fileType == "backdrop" {
			endpoint = "/upload-backdrop"
		} else {
			endpoint = "/upload-video"
		}

		resp, err := http.Post(workerURL+endpoint, wr.FormDataContentType(), &b)
		if err != nil {
			log.Printf("[UPLOAD_TEMP] Error uploading to worker: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Printf("[UPLOAD_TEMP] Worker returned status %d: %s", resp.StatusCode, body)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed"})
			return
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			log.Printf("[UPLOAD_TEMP] Error decoding response: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "invalid upload response"})
			return
		}

		savedURL = result["url"]
		fileKey = result["temp_key"]
		if fileKey == "" {
			fileKey = result["file_key"]
		}
		if fileKey == "" {
			fileKey = subDir + "/" + filename
		}
		if savedURL == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "upload failed - no URL returned"})
			return
		}
	}

	log.Printf("[UPLOAD_TEMP] Uploaded: type=%s, key=%s, url=%s", fileType, fileKey, savedURL)

	c.JSON(http.StatusOK, gin.H{
		"url":      savedURL,
		"file_key": fileKey,
	})
}
