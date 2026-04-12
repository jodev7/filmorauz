package handlers

import (
	"log"
	"net/http"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type UserHandler struct {
	watchHistoryRepo *repositories.WatchHistoryRepository
	favoriteRepo     *repositories.FavoriteRepository
	movieRepo        *repositories.MovieRepository
	userRepo         *repositories.UserRepository
}

func NewUserHandler(
	watchHistoryRepo *repositories.WatchHistoryRepository,
	favoriteRepo *repositories.FavoriteRepository,
	movieRepo *repositories.MovieRepository,
	userRepo *repositories.UserRepository,
) *UserHandler {
	return &UserHandler{
		watchHistoryRepo: watchHistoryRepo,
		favoriteRepo:     favoriteRepo,
		movieRepo:        movieRepo,
		userRepo:         userRepo,
	}
}

// GetPublicProfile godoc
// GET /api/users/:id
func (h *UserHandler) GetPublicProfile(c *gin.Context) {
	userIDStr := c.Param("id")
	userID, err := primitive.ObjectIDFromHex(userIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user id"})
		return
	}

	profile, err := h.userRepo.GetPublicProfile(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user profile"})
		return
	}

	if profile == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	// Enforce profile privacy at the API level.
	// OptionalAuth middleware populates user_id and role if a valid token is present.
	if profile.IsPrivate {
		requesterID, _ := c.Get("user_id")
		requesterRole, _ := c.Get("role")
		isOwner := requesterID != nil && requesterID.(string) == profile.ID
		isAdmin := requesterRole != nil && (requesterRole.(string) == "admin" || requesterRole.(string) == "superadmin")
		if !isOwner && !isAdmin {
			c.JSON(http.StatusForbidden, gin.H{"error": "Bu foydalanuvchi profili yashirilgan"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": profile})
}

// WatchHistoryRequest is the request body for recording watch history
type WatchHistoryRequest struct {
	MovieID string `json:"movie_id" binding:"required"`
}

// WatchProgressRequest is the request body for saving watch progress
type WatchProgressRequest struct {
	PositionSec int64 `json:"positionSec" binding:"required,min=0"`
	DurationSec int64 `json:"durationSec" binding:"required,min=1"`
}

// RecordWatchHistory godoc
// POST /user/history/watch
func (h *UserHandler) RecordWatchHistory(c *gin.Context) {
	// Get user from context (set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req WatchHistoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie_id is required"})
		return
	}

	// Parse movie ID
	movieID, err := primitive.ObjectIDFromHex(req.MovieID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	// Validate movie exists
	_, err = h.movieRepo.FindByID(movieID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Record watch history
	err = h.watchHistoryRepo.UpsertWatchHistory(userOID, movieID)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to record watch history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record watch history"})
		return
	}

	// Note: View count is now only incremented via POST /movies/:id/view
	// to avoid duplicate increments

	c.JSON(http.StatusOK, gin.H{"message": "watch history recorded"})
}

// SaveWatchProgress godoc
// POST /api/v1/watch/:movieId/progress
func (h *UserHandler) SaveWatchProgress(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	movieIDStr := c.Param("movieId")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	// Parse movie ID
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Bind request body
	var req WatchProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positionSec and durationSec are required"})
		return
	}

	// Validate: position should not exceed duration (with small tolerance)
	if req.PositionSec > req.DurationSec+10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "positionSec cannot exceed durationSec"})
		return
	}

	// Validate movie exists
	_, err = h.movieRepo.FindByID(movieID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Save progress
	err = h.watchHistoryRepo.SaveProgress(userOID, movieID, req.PositionSec, req.DurationSec)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to save progress: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "progress saved"})
}

// MarkWatchComplete godoc
// POST /api/v1/watch/:movieId/complete
func (h *UserHandler) MarkWatchComplete(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	movieIDStr := c.Param("movieId")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	// Parse movie ID
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Bind request body for duration (optional)
	var req struct {
		DurationSec int64 `json:"durationSec"`
	}
	c.ShouldBindJSON(&req)

	// Mark as complete
	err = h.watchHistoryRepo.MarkComplete(userOID, movieID, req.DurationSec)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to mark complete: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark complete"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "marked as complete"})
}

// GetContinueWatching godoc
// GET /api/v1/users/me/continue-watching
func (h *UserHandler) GetContinueWatching(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Get continue watching items
	items, err := h.watchHistoryRepo.GetContinueWatching(userOID, 10)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to get continue watching: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get continue watching"})
		return
	}

	if items == nil {
		items = []models.ContinueWatchingItem{}
	}

	c.JSON(http.StatusOK, gin.H{"data": items})
}

// GetWatchHistory godoc
// GET /user/history
func (h *UserHandler) GetWatchHistory(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	history, err := h.watchHistoryRepo.GetUserWatchHistory(userOID, 50)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to get watch history: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get watch history"})
		return
	}

	if history == nil {
		history = []models.WatchHistoryWithMovie{}
	}

	c.JSON(http.StatusOK, gin.H{"data": history})
}

// AddFavorite godoc
// POST /user/favorites/:movieId
func (h *UserHandler) AddFavorite(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	movieIDStr := c.Param("movieId")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movieId is required"})
		return
	}

	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	// Validate movie exists
	_, err = h.movieRepo.FindByID(movieID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	err = h.favoriteRepo.AddFavorite(userOID, movieID)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to add favorite: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add favorite"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "favorite added"})
}

// RemoveFavorite godoc
// DELETE /user/favorites/:movieId
func (h *UserHandler) RemoveFavorite(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	movieIDStr := c.Param("movieId")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movieId is required"})
		return
	}

	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	err = h.favoriteRepo.RemoveFavorite(userOID, movieID)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to remove favorite: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove favorite"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "favorite removed"})
}

// GetFavorites godoc
// GET /user/favorites
func (h *UserHandler) GetFavorites(c *gin.Context) {
	// Get user from context
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	favorites, err := h.favoriteRepo.GetUserFavorites(userOID)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to get favorites: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get favorites"})
		return
	}

	if favorites == nil {
		favorites = []models.FavoriteWithMovie{}
	}

	c.JSON(http.StatusOK, gin.H{"data": favorites})
}

// RecordView godoc
// POST /movies/:id/view
// Records a view for a movie (can be called by authenticated or anonymous users)
func (h *UserHandler) RecordView(c *gin.Context) {
	movieIDStr := c.Param("id")
	if movieIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
		return
	}

	// Increment view count (no auth required)
	err = h.movieRepo.IncrementViews(movieID)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to increment views: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record view"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "view recorded"})
}
