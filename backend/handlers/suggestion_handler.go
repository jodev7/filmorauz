package handlers

import (
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SuggestionHandler struct {
	suggestionService *services.SuggestionService
	config            *config.Config
}

func NewSuggestionHandler(suggestionService *services.SuggestionService) *SuggestionHandler {
	return &SuggestionHandler{
		suggestionService: suggestionService,
	}
}

func NewSuggestionHandlerWithConfig(suggestionService *services.SuggestionService, cfg *config.Config) *SuggestionHandler {
	return &SuggestionHandler{
		suggestionService: suggestionService,
		config:            cfg,
	}
}

var suggestionAllowedImageTypes = map[string]bool{
	"image/jpeg": true,
	"image/jpg":  true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

func (h *SuggestionHandler) CreateSuggestion(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	userName, _ := c.Get("user_name")
	userEmail, _ := c.Get("user_email")

	isMultipart := strings.Contains(c.ContentType(), "multipart/form-data")

	var input models.SuggestionInput
	var imageURL, imageStorageKey, imageMimeType string
	var imageSize int64

	if isMultipart {
		if err := c.Request.ParseMultipartForm(10 * 1024 * 1024); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid form data"})
			return
		}

		input.Type = models.SuggestionType(c.Request.FormValue("type"))
		input.Title = c.Request.FormValue("title")
		input.Message = c.Request.FormValue("message")
		input.SourceURL = c.Request.FormValue("source_url")

		file, header, err := c.Request.FormFile("image")
		if err == nil {
			defer file.Close()

			contentType := header.Header.Get("Content-Type")
			if !suggestionAllowedImageTypes[contentType] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type. allowed: jpg, jpeg, png, webp, gif"})
				return
			}

			maxSize := maxFormImageSize
			if contentType == "image/gif" {
				maxSize = maxFormGIFSize
			}
			if header.Size > maxSize {
				c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": fmt.Sprintf("image too large (max %dMB)", maxSize/1024/1024)})
				return
			}

			imageMimeType = contentType
			imageSize = header.Size

			uploadedURL, storageKey, uploadErr := h.uploadImageToStorage(file, header.Filename, contentType)
			if uploadErr != nil {
				log.Printf("[SuggestionHandler] Error uploading image: %v", uploadErr)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "image upload failed"})
				return
			}

			imageURL = uploadedURL
			imageStorageKey = storageKey
		}
	} else {
		if err := c.ShouldBindJSON(&input); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "title, type va message maydonlari majburiy"})
			return
		}
	}

	if input.Title == "" || input.Message == "" || input.Type == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title, type va message maydonlari majburiy"})
		return
	}

	if input.Type != models.SuggestionTypeMovie && input.Type != models.SuggestionTypeSeries {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type: faqat 'movie' yoki 'series' mumkin"})
		return
	}

	userNameStr := ""
	if n, ok := userName.(string); ok {
		userNameStr = n
	}
	userEmailStr := ""
	if e, ok := userEmail.(string); ok {
		userEmailStr = e
	}

	suggestion, err := h.suggestionService.CreateSuggestion(
		c.Request.Context(),
		userID,
		userNameStr,
		userEmailStr,
		&input,
		imageURL,
		imageStorageKey,
		imageMimeType,
		imageSize,
	)
	if err != nil {
		log.Printf("[SuggestionHandler] Error creating suggestion: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "xatolik yuz berdi"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":    "Tavsiya muvaffaqiyatli yuborildi",
		"suggestion": suggestion,
	})
}

func (h *SuggestionHandler) uploadImageToStorage(file multipart.File, filename, contentType string) (string, string, error) {
	if h.config == nil {
		return "", "", fmt.Errorf("config not initialized")
	}

	cfg := h.config
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", "", fmt.Errorf("failed to process file: %w", err)
	}
	maxSize := maxFormImageSize
	if contentType == "image/gif" {
		maxSize = maxFormGIFSize
	}
	data, detectedType, err := readAndValidateUpload(file, maxSize, suggestionAllowedImageTypes)
	if err != nil {
		return "", "", err
	}

	objectKey := buildFolderObjectKey("suggestions", "suggestion", filename, detectedType, ".jpg")
	uploader := NewUploadHandler(nil, cfg)
	imageURL, err := uploader.storeUploadedFile(objectKey, data, detectedType)
	if err != nil {
		return "", "", err
	}

	log.Printf("[SuggestionHandler] Direct upload success: path=%s url=%s", objectKey, imageURL)
	return imageURL, objectKey, nil
}

func (h *SuggestionHandler) GetMySuggestions(c *gin.Context) {
	userIDStr, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr.(string))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	result, err := h.suggestionService.GetSuggestionsByUser(c.Request.Context(), userID, page, limit)
	if err != nil {
		log.Printf("[SuggestionHandler] Error getting suggestions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "xatolik yuz berdi"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *SuggestionHandler) AdminListSuggestions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	statusStr := c.Query("status")
	var status *models.SuggestionStatus
	if statusStr != "" && statusStr != "all" {
		s := models.SuggestionStatus(statusStr)
		status = &s
	}

	result, err := h.suggestionService.GetSuggestions(c.Request.Context(), page, limit, status)
	if err != nil {
		log.Printf("[SuggestionHandler] Error listing suggestions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "xatolik yuz berdi"})
		return
	}

	c.JSON(http.StatusOK, result)
}

func (h *SuggestionHandler) AdminGetSuggestion(c *gin.Context) {
	id := c.Param("id")

	suggestion, err := h.suggestionService.GetSuggestionByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tavsiya topilmadi"})
		return
	}

	c.JSON(http.StatusOK, suggestion)
}

func (h *SuggestionHandler) AdminUpdateSuggestion(c *gin.Context) {
	id := c.Param("id")

	adminUserIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	adminUserIDStr, ok := adminUserIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	var input models.SuggestionUpdateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status maydoni majburiy"})
		return
	}

	if input.Status != models.SuggestionStatusAccepted && input.Status != models.SuggestionStatusRejected {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status: faqat 'accepted' yoki 'rejected' mumkin"})
		return
	}

	suggestion, err := h.suggestionService.UpdateSuggestionStatus(
		c.Request.Context(),
		id,
		input.Status,
		input.AdminMessage,
		adminUserIDStr+"",
	)
	if err != nil {
		log.Printf("[SuggestionHandler] Error updating suggestion: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "xatolik yuz berdi"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "Tavsiya yangilandi",
		"suggestion": suggestion,
	})
}

func (h *SuggestionHandler) AdminGetStats(c *gin.Context) {
	stats, err := h.suggestionService.GetStats(c.Request.Context())
	if err != nil {
		log.Printf("[SuggestionHandler] Error getting stats: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "xatolik yuz berdi"})
		return
	}

	c.JSON(http.StatusOK, stats)
}
