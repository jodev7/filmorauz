package handlers

import (
	"log"
	"net/http"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// RatingHandler handles rating-related HTTP requests
type RatingHandler struct {
	ratingService *services.RatingService
}

func NewRatingHandler(ratingService *services.RatingService) *RatingHandler {
	return &RatingHandler{ratingService: ratingService}
}

// RatingRequest is the request body for setting a rating
type RatingRequest struct {
	Rating int `json:"rating" binding:"required,min=1,max=5"`
}

// GetRatingSummary godoc
// GET /api/v1/movies/:id/rating-summary
func (h *RatingHandler) GetRatingSummary(c *gin.Context) {
	movieID := c.Param("id")
	if movieID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	// Get user ID from context (may be nil if not authenticated)
	var userID *string
	userIDVal, exists := c.Get("user_id")
	if exists {
		id := userIDVal.(string)
		userID = &id
	}

	summary, err := h.ratingService.GetRatingSummary(movieID, userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetRatingSummary failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get rating summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// SetRating godoc
// POST /api/v1/movies/:id/rating
func (h *RatingHandler) SetRating(c *gin.Context) {
	// Get user from context (required for rating)
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	movieID := c.Param("id")
	if movieID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	var req RatingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating is required and must be between 1 and 5"})
		return
	}

	err := h.ratingService.SetRating(movieID, userID, req.Rating)
	if err != nil {
		log.Printf("[RATING HANDLER] SetRating failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated summary
	summary, err := h.ratingService.GetRatingSummary(movieID, &userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetRatingSummary after set failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"message": "rating saved"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rating saved",
		"data":    summary,
	})
}

// DeleteRating godoc
// DELETE /api/v1/movies/:id/rating
func (h *RatingHandler) DeleteRating(c *gin.Context) {
	// Get user from context (required for rating)
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	movieID := c.Param("id")
	if movieID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	err := h.ratingService.DeleteRating(movieID, userID)
	if err != nil {
		log.Printf("[RATING HANDLER] DeleteRating failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated summary
	summary, err := h.ratingService.GetRatingSummary(movieID, &userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetRatingSummary after delete failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"message": "rating removed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rating removed",
		"data":    summary,
	})
}

// RatingSummaryResponse is used for backward compatibility with frontend
type RatingSummaryResponse struct {
	RatingAvg   float64 `json:"ratingAvg"`
	RatingCount int64   `json:"ratingCount"`
	UserRating  *int    `json:"userRating,omitempty"`
}

// TransformToResponse transforms models.RatingSummary to response format
func TransformToResponse(summary *models.RatingSummary) *RatingSummaryResponse {
	if summary == nil {
		return &RatingSummaryResponse{
			RatingAvg:   0,
			RatingCount: 0,
		}
	}
	return &RatingSummaryResponse{
		RatingAvg:   summary.RatingAvg,
		RatingCount: summary.RatingCount,
		UserRating:  summary.UserRating,
	}
}

// GetSeriesRatingSummary godoc
// GET /api/v1/series/:id/rating-summary
func (h *RatingHandler) GetSeriesRatingSummary(c *gin.Context) {
	seriesID := c.Param("id")
	if seriesID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "series id is required"})
		return
	}

	// Get user ID from context (may be nil if not authenticated)
	var userID *string
	userIDVal, exists := c.Get("user_id")
	if exists {
		id := userIDVal.(string)
		userID = &id
	}

	summary, err := h.ratingService.GetSeriesRatingSummary(seriesID, userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetSeriesRatingSummary failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get rating summary"})
		return
	}

	c.JSON(http.StatusOK, summary)
}

// SetSeriesRating godoc
// POST /api/v1/series/:id/rating
func (h *RatingHandler) SetSeriesRating(c *gin.Context) {
	// Get user from context (required for rating)
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	seriesID := c.Param("id")
	if seriesID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "series id is required"})
		return
	}

	var req RatingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "rating is required and must be between 1 and 5"})
		return
	}

	err := h.ratingService.SetSeriesRating(seriesID, userID, req.Rating)
	if err != nil {
		log.Printf("[RATING HANDLER] SetSeriesRating failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated summary
	summary, err := h.ratingService.GetSeriesRatingSummary(seriesID, &userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetSeriesRatingSummary after set failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"message": "rating saved"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rating saved",
		"data":    summary,
	})
}

// DeleteSeriesRating godoc
// DELETE /api/v1/series/:id/rating
func (h *RatingHandler) DeleteSeriesRating(c *gin.Context) {
	// Get user from context (required for rating)
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	seriesID := c.Param("id")
	if seriesID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "series id is required"})
		return
	}

	err := h.ratingService.DeleteSeriesRating(seriesID, userID)
	if err != nil {
		log.Printf("[RATING HANDLER] DeleteSeriesRating failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Get updated summary
	summary, err := h.ratingService.GetSeriesRatingSummary(seriesID, &userID)
	if err != nil {
		log.Printf("[RATING HANDLER] GetSeriesRatingSummary after delete failed: %v", err)
		c.JSON(http.StatusOK, gin.H{"message": "rating removed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rating removed",
		"data":    summary,
	})
}
