package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

type UserHandler struct {
	watchHistoryRepo *repositories.WatchHistoryRepository
	favoriteRepo     *repositories.FavoriteRepository
	movieRepo        *repositories.MovieRepository
	seriesRepo       *repositories.SeriesRepository
	userRepo         *repositories.UserRepository
}

func NewUserHandler(
	watchHistoryRepo *repositories.WatchHistoryRepository,
	favoriteRepo *repositories.FavoriteRepository,
	movieRepo *repositories.MovieRepository,
	seriesRepo *repositories.SeriesRepository,
	userRepo *repositories.UserRepository,
) *UserHandler {
	return &UserHandler{
		watchHistoryRepo: watchHistoryRepo,
		favoriteRepo:     favoriteRepo,
		movieRepo:        movieRepo,
		seriesRepo:       seriesRepo,
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
	MovieID    string `json:"movie_id"`
	TargetID   string `json:"target_id"`
	TargetType string `json:"target_type"`
}

// WatchProgressRequest is the request body for saving watch progress
type WatchProgressRequest struct {
	PositionSec int64  `json:"positionSec" binding:"required,min=0"`
	DurationSec int64  `json:"durationSec" binding:"required,min=1"`
	TargetType  string `json:"target_type"`
}

type GenericWatchProgressRequest struct {
	TargetType  string `json:"target_type" binding:"required"`
	TargetID    string `json:"target_id" binding:"required"`
	CurrentTime int64  `json:"current_time" binding:"required,min=0"`
	Duration    int64  `json:"duration" binding:"required,min=1"`
}

type resolvedUserTarget struct {
	TargetType string
	TargetID   primitive.ObjectID
	SeriesID   *primitive.ObjectID
	SeasonID   *primitive.ObjectID
	EpisodeID  *primitive.ObjectID
}

func (h *UserHandler) resolveFavoriteTarget(c *gin.Context, rawID string) (*resolvedUserTarget, error) {
	targetType := strings.TrimSpace(c.Query("target_type"))
	return h.resolveUserTarget(rawID, targetType)
}

func (h *UserHandler) resolveUserTarget(rawID string, rawTargetType string) (*resolvedUserTarget, error) {
	targetID, err := primitive.ObjectIDFromHex(strings.TrimSpace(rawID))
	if err != nil {
		return nil, fmt.Errorf("invalid target id")
	}

	targetType := strings.TrimSpace(strings.ToLower(rawTargetType))
	switch targetType {
	case "episode":
		episode, err := h.seriesRepo.GetEpisodeByID(targetID)
		if err != nil {
			return nil, fmt.Errorf("episode not found")
		}
		return &resolvedUserTarget{
			TargetType: "episode",
			TargetID:   targetID,
			SeriesID:   &episode.SeriesID,
			SeasonID:   &episode.SeasonID,
			EpisodeID:  &episode.ID,
		}, nil
	case "movie":
		if _, err := h.movieRepo.FindByID(targetID); err != nil {
			return nil, fmt.Errorf("movie not found")
		}
		return &resolvedUserTarget{TargetType: "movie", TargetID: targetID}, nil
	}

	if _, err := h.movieRepo.FindByID(targetID); err == nil {
		return &resolvedUserTarget{TargetType: "movie", TargetID: targetID}, nil
	} else if err != mongo.ErrNoDocuments {
		return nil, err
	}

	episode, err := h.seriesRepo.GetEpisodeByID(targetID)
	if err == nil {
		return &resolvedUserTarget{
			TargetType: "episode",
			TargetID:   targetID,
			SeriesID:   &episode.SeriesID,
			SeasonID:   &episode.SeasonID,
			EpisodeID:  &episode.ID,
		}, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	return nil, fmt.Errorf("content not found")
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
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_id is required"})
		return
	}

	targetRawID := strings.TrimSpace(req.TargetID)
	if targetRawID == "" {
		targetRawID = strings.TrimSpace(req.MovieID)
	}
	target, err := h.resolveUserTarget(targetRawID, req.TargetType)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Parse user ID
	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Record watch history
	err = h.watchHistoryRepo.UpsertWatchHistory(userOID, target.TargetID, target.TargetType, target.SeriesID, target.SeasonID, target.EpisodeID)
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

	targetIDStr := c.Param("movieId")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content id is required"})
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

	target, err := h.resolveUserTarget(targetIDStr, req.TargetType)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Save progress
	err = h.watchHistoryRepo.SaveProgress(userOID, target.TargetID, target.TargetType, target.SeriesID, target.SeasonID, target.EpisodeID, req.PositionSec, req.DurationSec)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to save progress: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "progress saved"})
}

// SaveWatchProgressGeneric godoc
// POST /api/watch/progress
func (h *UserHandler) SaveWatchProgressGeneric(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req GenericWatchProgressRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type, target_id, current_time and duration are required"})
		return
	}
	if req.CurrentTime > req.Duration+10 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "current_time cannot exceed duration"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	target, err := h.resolveUserTarget(req.TargetID, req.TargetType)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	if err := h.watchHistoryRepo.SaveProgress(userOID, target.TargetID, target.TargetType, target.SeriesID, target.SeasonID, target.EpisodeID, req.CurrentTime, req.Duration); err != nil {
		log.Printf("[USER HANDLER] Failed to save generic progress: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save progress"})
		return
	}

	progress, err := h.watchHistoryRepo.GetProgress(userOID, target.TargetID, target.TargetType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load saved progress"})
		return
	}
	c.JSON(http.StatusOK, progress)
}

// GetWatchProgress godoc
// GET /api/watch/progress?target_type=movie&target_id=...
func (h *UserHandler) GetWatchProgress(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	targetType := strings.TrimSpace(c.Query("target_type"))
	targetIDStr := strings.TrimSpace(c.Query("target_id"))
	if targetType == "" || targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type and target_id are required"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	target, err := h.resolveUserTarget(targetIDStr, targetType)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	progress, err := h.watchHistoryRepo.GetProgress(userOID, target.TargetID, target.TargetType)
	if err != nil {
		log.Printf("[USER HANDLER] Failed to get progress: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get progress"})
		return
	}
	if progress == nil {
		c.JSON(http.StatusOK, gin.H{
			"current_time":     0,
			"duration":         0,
			"progress_percent": 0,
			"completed":        false,
		})
		return
	}

	c.JSON(http.StatusOK, progress)
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

	targetIDStr := c.Param("movieId")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content id is required"})
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
		DurationSec int64  `json:"durationSec"`
		TargetType  string `json:"target_type"`
	}
	c.ShouldBindJSON(&req)

	target, err := h.resolveUserTarget(targetIDStr, req.TargetType)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Mark as complete
	err = h.watchHistoryRepo.MarkComplete(userOID, target.TargetID, target.TargetType, target.SeriesID, target.SeasonID, target.EpisodeID, req.DurationSec)
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
		history = []models.WatchHistoryListItem{}
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

	targetIDStr := c.Param("movieId")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content id is required"})
		return
	}

	target, err := h.resolveFavoriteTarget(c, targetIDStr)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	err = h.favoriteRepo.AddFavorite(userOID, target.TargetID, target.TargetType, target.SeriesID, target.SeasonID)
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

	targetIDStr := c.Param("movieId")
	if targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content id is required"})
		return
	}

	target, err := h.resolveFavoriteTarget(c, targetIDStr)
	if err != nil {
		status := http.StatusNotFound
		if strings.Contains(err.Error(), "invalid") {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	err = h.favoriteRepo.RemoveFavorite(userOID, target.TargetID, target.TargetType)
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
		favorites = []models.FavoriteListItem{}
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

	if _, err = h.movieRepo.FindByID(movieID); err == nil {
		err = h.movieRepo.IncrementViews(movieID)
	} else if err == mongo.ErrNoDocuments {
		_, epErr := h.seriesRepo.GetEpisodeByID(movieID)
		if epErr != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
			return
		}
		err = h.seriesRepo.IncrementEpisodeViews(movieID)
	}
	if err != nil {
		log.Printf("[USER HANDLER] Failed to increment views: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record view"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "view recorded"})
}

// premiumPlans maps plan IDs to price (UZS) and duration in days
var premiumPlans = map[string]struct {
	Price        float64
	DurationDays int
}{
	"1month":  {Price: 25000, DurationDays: 30},
	"3month":  {Price: 69000, DurationDays: 90},
	"6month":  {Price: 118000, DurationDays: 180},
	"12month": {Price: 234000, DurationDays: 365},
}

// BuyPremium godoc
// POST /api/user/premium/buy
// Deducts wallet balance and activates premium for the authenticated user.
func (h *UserHandler) BuyPremium(c *gin.Context) {
	userIDStr, _ := c.Get("user_id")

	var req struct {
		PlanID string `json:"plan_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plan, ok := premiumPlans[req.PlanID]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid plan_id"})
		return
	}

	err := h.userRepo.DeductWalletAndActivatePremium(userIDStr.(string), plan.Price, plan.DurationDays)
	if err != nil {
		if err.Error() == "insufficient_balance" {
			c.JSON(http.StatusPaymentRequired, gin.H{"error": fmt.Sprintf("Hisobda mablag' yetarli emas. Zarur: %.0f so'm", plan.Price)})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "message": "Premium muvaffaqiyatli faollashtirildi"})
}
