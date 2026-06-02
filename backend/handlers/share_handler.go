package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ShareHandler struct {
	shareService *services.ShareService
}

func NewShareHandler(shareService *services.ShareService) *ShareHandler {
	return &ShareHandler{shareService: shareService}
}

// CreateShare creates a new share for a movie
// POST /api/movies/share with body {"movie_id": "..."}
func (h *ShareHandler) CreateShare(c *gin.Context) {
	var req struct {
		MovieID string `json:"movie_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Convert movie ID from hex string
	movieID, err := primitive.ObjectIDFromHex(req.MovieID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	// Get user ID from context (if authenticated)
	var userID *primitive.ObjectID
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		uid, err := primitive.ObjectIDFromHex(userIDStr)
		if err == nil {
			userID = &uid
		}
	}

	// Determine source (web or telegram)
	source := "web"
	if c.GetHeader("X-Telegram-Auth") != "" {
		source = "telegram"
	}

	// Create share
	share, shareURL, err := h.shareService.CreateShare(movieID, userID, source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"share_code": share.Code,
		"share_url":  shareURL,
	})
}

// CreateSeriesShare creates a new tracked share for a series
// POST /api/series/share with body {"series_id": "..."}
func (h *ShareHandler) CreateSeriesShare(c *gin.Context) {
	var req struct {
		SeriesID string `json:"series_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	seriesID, err := primitive.ObjectIDFromHex(req.SeriesID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	var userID *primitive.ObjectID
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		if uid, err := primitive.ObjectIDFromHex(userIDStr); err == nil {
			userID = &uid
		}
	}

	source := "web"
	if c.GetHeader("X-Telegram-Auth") != "" {
		source = "telegram"
	}

	share, shareURL, err := h.shareService.CreateSeriesShare(seriesID, userID, source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"share_code": share.Code,
		"share_url":  shareURL,
	})
}

// RecordShareOpen records when a share is opened
func (h *ShareHandler) RecordShareOpen(c *gin.Context) {
	code := c.Param("code")

	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "share code is required"})
		return
	}

	// Record the open
	err := h.shareService.RecordShareOpen(code)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "share open recorded",
	})
}

// GetMovieShareStats returns share statistics for a movie
// GET /api/movies/share-stats?movie_id=...
func (h *ShareHandler) GetMovieShareStats(c *gin.Context) {
	movieIDStr := c.Query("movie_id")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie_id is required"})
		return
	}

	// Convert movie ID from hex string
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	stats, err := h.shareService.GetMovieShareStats(movieID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get share stats"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"shares_created_count": stats.SharesCreatedCount,
		"total_share_opens":    stats.TotalShareOpens,
	})
}

// GetUserShareStats returns share statistics for the current user
func (h *ShareHandler) GetUserShareStats(c *gin.Context) {
	// Get user ID from context
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	stats, err := h.shareService.GetUserShareStats(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get share stats"})
		return
	}

	// Format response
	response := gin.H{
		"total_shares_created": stats.TotalSharesCreated,
		"total_share_opens":    stats.TotalShareOpens,
		"total_movies_shared":  stats.TotalMoviesShared,
	}

	if stats.TopSharedMovie != nil {
		response["top_shared_movie"] = gin.H{
			"movie_id":    stats.TopSharedMovie.MovieID,
			"title":       stats.TopSharedMovie.Title,
			"share_count": stats.TopSharedMovie.ShareCount,
		}
	}

	c.JSON(http.StatusOK, response)
}

// GetAdminShareStats returns admin-level share statistics
func (h *ShareHandler) GetAdminShareStats(c *gin.Context) {
	// Check admin role (case-insensitive)
	role, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusForbidden, gin.H{"error": "access denied"})
		return
	}
	roleStr, ok := role.(string)
	if !ok {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}
	normalizedRole := strings.ToLower(strings.TrimSpace(roleStr))
	if normalizedRole != "admin" && normalizedRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
		return
	}

	stats, err := h.shareService.GetAdminShareStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get share stats"})
		return
	}

	// Format top movies
	topMovies := make([]gin.H, len(stats.TopSharedMovies))
	for i, m := range stats.TopSharedMovies {
		topMovies[i] = gin.H{
			"movie_id":             m.MovieID,
			"title":                m.Title,
			"shares_created_count": m.SharesCreatedCount,
			"total_share_opens":    m.TotalShareOpens,
		}
	}

	// Format top users
	topUsers := make([]gin.H, len(stats.TopUsersByShares))
	for i, u := range stats.TopUsersByShares {
		topUsers[i] = gin.H{
			"user_id":              u.UserID,
			"display_name":         u.DisplayName,
			"shares_created_count": u.SharesCreatedCount,
		}
	}

	// Format recent shares
	recentShares := make([]gin.H, len(stats.RecentShares))
	for i, s := range stats.RecentShares {
		recentShares[i] = gin.H{
			"id":         s.ID.Hex(),
			"code":       s.Code,
			"movie_id":   s.MovieID.Hex(),
			"clicks":     s.Clicks,
			"created_at": s.CreatedAt.Format("2006-01-02T15:04:05Z"),
			"source":     s.Source,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"total_shares_created": stats.TotalSharesCreated,
		"total_share_opens":    stats.TotalShareOpens,
		"top_shared_movies":    topMovies,
		"top_users_by_shares":  topUsers,
		"recent_shares":        recentShares,
	})
}

// Format error helper
func formatShareError(err error) string {
	return fmt.Sprintf("share error: %v", err)
}
