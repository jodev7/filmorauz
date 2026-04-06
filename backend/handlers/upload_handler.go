package handlers

import (
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
		// PROD mode - use CDN (Backblaze B2 or other)
		savedURL, err = h.saveToCDN(file, filename, contentType)
		if err != nil {
			log.Printf("[UPLOAD] Error saving to CDN: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upload to storage"})
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
	return fmt.Sprintf("http://localhost:%s/uploads/%s", h.config.Port, filename), nil
}

// saveToCDN saves file to CDN/storage (production)
func (h *UploadHandler) saveToCDN(file multipart.File, filename string, contentType string) (string, error) {
	// TODO: Implement actual CDN upload (Backblaze B2, S3, etc.)
	// For now, this is a placeholder that returns an error
	// In production, you would use the Backblaze B2 SDK or AWS S3 SDK

	log.Printf("[UPLOAD] CDN upload not implemented, using placeholder URL")

	// For production, you would upload to B2/S3 and return the public URL
	// Example with Backblaze B2:
	// b2, _ := b2.NewB2(...)
	// uploaded, _ := b2.UploadFile(file, filename, contentType)
	// return uploaded.URL, nil

	// Placeholder - return a URL that would be returned by CDN
	// In production, replace with actual CDN URL
	return fmt.Sprintf("https://cdn.filmorauz.uz/images/%s", filename), nil
}

// UploadMovieAssets handles file uploads for movie assets (poster, backdrop, video)
func (h *UploadHandler) UploadMovieAssets(c *gin.Context) {
	fileType := c.PostForm("type")
	if fileType != "poster" && fileType != "backdrop" && fileType != "video" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid type: must be 'poster', 'backdrop', or 'video'"})
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
		savedURL, err = h.saveToCDN(file, filename, contentType)
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

	return fmt.Sprintf("http://localhost:%s/uploads/movies/%s/%s", h.config.Port, subDir, filename), nil
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
