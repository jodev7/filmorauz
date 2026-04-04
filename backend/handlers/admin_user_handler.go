package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/middleware"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// AdminUserHandler handles admin user management
type AdminUserHandler struct {
	userRepo            *repositories.UserRepository
	movieRepo           *repositories.MovieRepository
	seriesRepo          *repositories.SeriesRepository
	banHistoryRepo      *repositories.BanHistoryRepository
	notificationService *services.NotificationService
}

// NewAdminUserHandler creates a new admin user handler
func NewAdminUserHandler(userRepo *repositories.UserRepository, movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository, banHistoryRepo *repositories.BanHistoryRepository, notificationService *services.NotificationService) *AdminUserHandler {
	return &AdminUserHandler{
		userRepo:            userRepo,
		movieRepo:           movieRepo,
		seriesRepo:          seriesRepo,
		banHistoryRepo:      banHistoryRepo,
		notificationService: notificationService,
	}
}

// timePtr returns a pointer to a time.Time value
func timePtr(t time.Time) *time.Time {
	return &t
}

// DashboardStats returns combined stats for admin dashboard
func (h *AdminUserHandler) DashboardStats(c *gin.Context) {
	// User stats
	totalUsers, err := h.userRepo.CountTotal()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get user stats"})
		return
	}

	todayUsers, err := h.userRepo.CountRegisteredToday()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get today user stats"})
		return
	}

	thisMonthUsers, err := h.userRepo.CountRegisteredThisMonth()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get month user stats"})
		return
	}

	// Recent users
	recentUsers, err := h.userRepo.FindRecentUsers(5)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get recent users"})
		return
	}

	// Map to response format
	type userResponse struct {
		ID               string  `json:"id"`
		DisplayName      string  `json:"display_name"`
		Username         string  `json:"username"`
		TelegramID       int64   `json:"telegram_id"`
		Role             string  `json:"role"`
		IsPremium        bool    `json:"is_premium"`
		IsPremiumActive  bool    `json:"is_premium_active"`
		PremiumExpiresAt *string `json:"premium_expires_at,omitempty"`
		CreatedAt        string  `json:"created_at"`
		LastLoginAt      string  `json:"last_login_at"`
	}

	recentUsersResp := make([]userResponse, len(recentUsers))
	for i, u := range recentUsers {
		var premiumExpiresAt *string
		if u.PremiumExpiresAt != nil {
			expiresAt := u.PremiumExpiresAt.Format(time.RFC3339)
			premiumExpiresAt = &expiresAt
		}
		recentUsersResp[i] = userResponse{
			ID:               u.ID.Hex(),
			DisplayName:      u.DisplayName,
			Username:         u.TelegramUser,
			TelegramID:       u.TelegramID,
			Role:             u.Role,
			IsPremium:        u.IsPremium,
			IsPremiumActive:  u.IsPremiumActive(),
			PremiumExpiresAt: premiumExpiresAt,
			CreatedAt:        u.CreatedAt.Format(time.RFC3339),
			LastLoginAt:      u.LastLoginAt.Format(time.RFC3339),
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"users": gin.H{
			"total":                 totalUsers,
			"registered_today":      todayUsers,
			"registered_this_month": thisMonthUsers,
			"recent":                recentUsersResp,
		},
	})
}

// TopContentDTO is a lightweight DTO for analytics responses
type TopContentDTO struct {
	ID         string `json:"id"`
	Title      string `json:"title"`
	Slug       string `json:"slug"`
	ViewsCount int64  `json:"views_count"`
	PosterURL  string `json:"poster_url,omitempty"`
}

// GetTopMovies returns top viewed movies for admin analytics
func (h *AdminUserHandler) GetTopMovies(c *gin.Context) {
	ctx := c.Request.Context()

	// Get top 10 movies by views - use simple filter
	opts := options.Find().
		SetSort(bson.D{{Key: "views", Value: -1}}).
		SetLimit(10)

	cursor, err := h.movieRepo.Collection().Find(ctx, bson.M{}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch top movies"})
		return
	}
	defer cursor.Close(ctx)

	// Decode into a slice of maps first, then convert to DTO
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode movies"})
		return
	}

	// Handle nil results
	if results == nil {
		results = []bson.M{}
	}

	var topMovies []TopContentDTO
	for _, r := range results {
		id := ""
		if oid, ok := r["_id"].(primitive.ObjectID); ok {
			id = oid.Hex()
		}

		title, _ := r["title"].(string)
		slug, _ := r["slug"].(string)
		posterURL, _ := r["poster_url"].(string)

		// Handle views - could be int, int32, int64, or missing
		var viewsCount int64 = 0
		switch v := r["views"].(type) {
		case int64:
			viewsCount = v
		case int32:
			viewsCount = int64(v)
		case int:
			viewsCount = int64(v)
		}

		topMovies = append(topMovies, TopContentDTO{
			ID:         id,
			Title:      title,
			Slug:       slug,
			ViewsCount: viewsCount,
			PosterURL:  posterURL,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": topMovies})
}

// GetTopSeries returns top viewed series for admin analytics
func (h *AdminUserHandler) GetTopSeries(c *gin.Context) {
	ctx := c.Request.Context()

	// Get top 10 series by views
	opts := options.Find().
		SetSort(bson.D{{Key: "views", Value: -1}}).
		SetLimit(10)

	cursor, err := h.seriesRepo.Collection().Find(ctx, bson.M{}, opts)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch top series"})
		return
	}
	defer cursor.Close(ctx)

	// Decode into a slice of maps first, then convert to DTO
	var results []bson.M
	if err := cursor.All(ctx, &results); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to decode series"})
		return
	}

	// Handle nil results
	if results == nil {
		results = []bson.M{}
	}

	var topSeries []TopContentDTO
	for _, r := range results {
		id := ""
		if oid, ok := r["_id"].(primitive.ObjectID); ok {
			id = oid.Hex()
		}

		title, _ := r["title"].(string)
		slug, _ := r["slug"].(string)
		posterURL, _ := r["poster_url"].(string)

		// Handle views - could be int, int32, int64, or missing
		var viewsCount int64 = 0
		switch v := r["views"].(type) {
		case int64:
			viewsCount = v
		case int32:
			viewsCount = int64(v)
		case int:
			viewsCount = int64(v)
		}

		topSeries = append(topSeries, TopContentDTO{
			ID:         id,
			Title:      title,
			Slug:       slug,
			ViewsCount: viewsCount,
			PosterURL:  posterURL,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": topSeries})
}

// GetUserMetrics returns user metrics for admin dashboard
func (h *AdminUserHandler) GetUserMetrics(c *gin.Context) {
	// Get total users
	totalUsers, err := h.userRepo.CountTotal()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get total users"})
		return
	}

	// Get premium users
	premiumUsers, err := h.userRepo.CountPremium()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get premium users"})
		return
	}

	// Calculate conversion rate
	var conversionRate float64
	if totalUsers > 0 {
		conversionRate = float64(premiumUsers) / float64(totalUsers) * 100
	}

	// Get total views from movies
	movieViews, err := h.movieRepo.CountTotalViews()
	if err != nil {
		movieViews = 0
	}

	// Get total views from series
	seriesViews, err := h.seriesRepo.CountTotalViews()
	if err != nil {
		seriesViews = 0
	}

	// Total views = movies + series
	totalViews := movieViews + seriesViews

	c.JSON(http.StatusOK, gin.H{
		"total_users":     totalUsers,
		"premium_users":   premiumUsers,
		"conversion_rate": conversionRate,
		"total_views":     totalViews,
		"movie_views":     movieViews,
		"series_views":    seriesViews,
	})
}

// ListUsers returns paginated users
func (h *AdminUserHandler) ListUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	search := c.Query("search")
	role := c.Query("role")

	// If role is "all" or empty, don't filter by role
	if role == "all" {
		role = ""
	}

	users, total, err := h.userRepo.FindUsers(page, limit, search, role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get users"})
		return
	}

	// Map to response format
	type userResponse struct {
		ID               string  `json:"id"`
		DisplayName      string  `json:"display_name"`
		Username         string  `json:"username"`
		TelegramID       int64   `json:"telegram_id"`
		Role             string  `json:"role"`
		IsPremium        bool    `json:"is_premium"`
		IsPremiumActive  bool    `json:"is_premium_active"`
		PremiumExpiresAt *string `json:"premium_expires_at,omitempty"`
		AuthProvider     string  `json:"auth_provider"`
		CreatedAt        string  `json:"created_at"`
		LastLoginAt      string  `json:"last_login_at"`
		IsBanned         bool    `json:"is_banned"`
		BanReason        string  `json:"ban_reason,omitempty"`
		BannedUntil      *string `json:"banned_until,omitempty"`
		BannedByUsername string  `json:"banned_by_username,omitempty"`
	}

	usersResp := make([]userResponse, len(users))
	for i, u := range users {
		var premiumExpiresAt *string
		if u.PremiumExpiresAt != nil {
			expiresAt := u.PremiumExpiresAt.Format(time.RFC3339)
			premiumExpiresAt = &expiresAt
		}
		// Format ban information
		var bannedUntilStr *string
		if u.Ban != nil && u.Ban.BannedUntil != nil {
			until := u.Ban.BannedUntil.Format(time.RFC3339)
			bannedUntilStr = &until
		}

		var banReason string
		var bannedByUsername string
		var isBanned bool
		if u.Ban != nil {
			isBanned = u.Ban.IsBanned
			banReason = u.Ban.Reason
			bannedByUsername = u.Ban.BannedByUsername
		}

		usersResp[i] = userResponse{
			ID:               u.ID.Hex(),
			DisplayName:      u.DisplayName,
			Username:         u.TelegramUser,
			TelegramID:       u.TelegramID,
			Role:             u.Role,
			IsPremium:        u.IsPremium,
			IsPremiumActive:  u.IsPremiumActive(),
			PremiumExpiresAt: premiumExpiresAt,
			AuthProvider:     u.AuthProvider,
			CreatedAt:        u.CreatedAt.Format(time.RFC3339),
			LastLoginAt:      u.LastLoginAt.Format(time.RFC3339),
			IsBanned:         isBanned,
			BanReason:        banReason,
			BannedUntil:      bannedUntilStr,
			BannedByUsername: bannedByUsername,
		}
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        usersResp,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// UpdateUserRole updates a user's role
func (h *AdminUserHandler) UpdateUserRole(c *gin.Context) {
	// Get current user role from context (set by auth middleware)
	currentUserRole, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication error"})
		return
	}

	// ONLY superadmin can change roles (case-insensitive)
	currentUserRoleStr, ok := currentUserRole.(string)
	if !ok || strings.ToLower(strings.TrimSpace(currentUserRoleStr)) != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "Only superadmin can change user roles",
		})
		return
	}

	userID := c.Param("id")

	// Check if target user is SuperAdmin - cannot change role of SuperAdmin
	targetUser, checkErr := h.userRepo.FindByID(userID)
	if checkErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if middleware.IsSuperAdmin(targetUser.Role) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "SuperAdmin rolni o'zgartirish mumkin emas",
		})
		return
	}

	// Support both JSON body and path param for backward compatibility
	var newRole string
	var req struct {
		Role string `json:"role"`
	}

	// Try to parse JSON body first
	if c.Request.Body != nil && c.Request.ContentLength > 0 {
		if parseErr := c.ShouldBindJSON(&req); parseErr == nil && req.Role != "" {
			newRole = req.Role
		}
	}

	// Fall back to path param if no body
	if newRole == "" {
		newRole = c.Param("role")
	}

	if newRole == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "role is required"})
		return
	}

	// Validate role
	validRoles := map[string]bool{
		"user":       true,
		"admin":      true,
		"superadmin": true,
	}
	if !validRoles[newRole] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}

	// Prevent superadmin from accidentally removing their own superadmin role
	// Get current user ID from context (set by auth middleware)
	currentUserID := c.GetString("user_id")
	if currentUserID != "" && userID == currentUserID && newRole != "superadmin" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "cannot change own role",
			"message": "You cannot change your own superadmin role",
		})
		return
	}

	err := h.userRepo.UpdateUserRoleByHex(userID, newRole)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update user role"})
		return
	}

	// Fetch updated user to return
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message": "role updated",
			"role":    newRole,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "role updated",
		"role":       user.Role,
		"is_premium": user.IsPremium,
	})
}

// BanUser bans a user with duration and reason
func (h *AdminUserHandler) BanUser(c *gin.Context) {
	// Get current user role from context (set by auth middleware)
	currentUserRole, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication error"})
		return
	}

	// Only admin/superadmin can ban users
	currentUserRoleStr, ok := currentUserRole.(string)
	if !ok || (strings.ToLower(strings.TrimSpace(currentUserRoleStr)) != "admin" && strings.ToLower(strings.TrimSpace(currentUserRoleStr)) != "superadmin") {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "Only admins can ban users",
		})
		return
	}

	userID := c.Param("id")

	// Check if target user is SuperAdmin - cannot ban SuperAdmin
	targetUser, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if middleware.IsSuperAdmin(targetUser.Role) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "SuperAdmin hisobini ban qilish mumkin emas",
		})
		return
	}

	// Parse request body
	var req struct {
		DurationDays int    `json:"duration_days"` // 0 for permanent
		Reason       string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate reason
	if req.Reason == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "reason is required"})
		return
	}

	// Get current admin user info
	currentUserID := c.GetString("user_id")
	currentUser, err := h.userRepo.FindByID(currentUserID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get admin user info"})
		return
	}

	// Calculate ban expiry
	var bannedUntil *time.Time
	isPermanent := req.DurationDays == 0
	if req.DurationDays > 0 {
		expiry := time.Now().AddDate(0, 0, req.DurationDays)
		bannedUntil = &expiry
	}

	// Create ban info
	bannedByUsername := currentUser.TelegramUser
	if currentUser.DisplayName != "" {
		bannedByUsername = currentUser.DisplayName
	}

	banInfo := &models.BanInfo{
		IsBanned:         true,
		Reason:           req.Reason,
		BannedAt:         timePtr(time.Now()),
		BannedUntil:      bannedUntil,
		BannedByUserID:   currentUserID,
		BannedByUsername: bannedByUsername,
	}

	// Update user ban status
	err = h.userRepo.UpdateUserBan(userID, banInfo)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to ban user"})
		return
	}

	// Create ban history record
	userObjectID, _ := primitive.ObjectIDFromHex(userID)
	banHistory := &models.BanHistory{
		UserID:           userObjectID,
		Reason:           req.Reason,
		BannedAt:         time.Now(),
		BannedUntil:      bannedUntil,
		IsPermanent:      isPermanent,
		BannedByUserID:   currentUser.ID,
		BannedByUsername: bannedByUsername,
	}
	h.banHistoryRepo.Create(banHistory)

	// Send notification to the banned user
	go func() {
		if h.notificationService != nil {
			// Build duration string
			var durationStr string
			if isPermanent {
				durationStr = "permanent"
			} else if req.DurationDays > 0 {
				durationStr = fmt.Sprintf("%d kun", req.DurationDays)
			} else {
				durationStr = "muajjam"
			}

			err := h.notificationService.NotifyBanApplied(
				c.Request.Context(),
				userObjectID,
				req.Reason,
				durationStr,
				bannedByUsername,
			)
			if err != nil {
				log.Printf("[Notification] Failed to send ban applied notification: %v", err)
			} else {
				log.Printf("[Notification] Ban applied notification sent to user %s", userID)
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "user banned successfully",
		"ban":     banInfo,
	})
}

// UnbanUser removes a ban from a user
func (h *AdminUserHandler) UnbanUser(c *gin.Context) {
	// Get current user role from context (set by auth middleware)
	currentUserRole, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication error"})
		return
	}

	// Only admin/superadmin can unban users
	currentUserRoleStr, ok := currentUserRole.(string)
	if !ok || (strings.ToLower(strings.TrimSpace(currentUserRoleStr)) != "admin" && strings.ToLower(strings.TrimSpace(currentUserRoleStr)) != "superadmin") {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "Only admins can unban users",
		})
		return
	}

	userID := c.Param("id")

	// Check if target user is SuperAdmin - cannot unban (protection)
	targetUser, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if middleware.IsSuperAdmin(targetUser.Role) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "SuperAdmin hisobini o'zgartirish mumkin emas",
		})
		return
	}

	// Get current admin user info
	currentAdminID := c.GetString("user_id")
	currentAdmin, err := h.userRepo.FindByID(currentAdminID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get admin user info"})
		return
	}

	// Get unbanning admin username
	unbannedByUsername := currentAdmin.TelegramUser
	if currentAdmin.DisplayName != "" {
		unbannedByUsername = currentAdmin.DisplayName
	}

	// Find and update the active ban in history
	activeBan, err := h.banHistoryRepo.FindActiveBanByUserID(userID)
	if err == nil && activeBan != nil {
		h.banHistoryRepo.UpdateUnban(activeBan.ID.Hex(), currentAdmin.ID, unbannedByUsername)
	}

	// Remove ban from user
	err = h.userRepo.RemoveUserBan(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unban user"})
		return
	}

	// Send notification to the unbanned user
	go func() {
		if h.notificationService != nil {
			userOID, err := primitive.ObjectIDFromHex(userID)
			if err != nil {
				log.Printf("[Notification] Failed to parse user ID for ban removal notification: %v", err)
				return
			}
			err = h.notificationService.NotifyBanRemoved(c.Request.Context(), userOID)
			if err != nil {
				log.Printf("[Notification] Failed to send ban removed notification: %v", err)
			} else {
				log.Printf("[Notification] Ban removed notification sent to user %s", userID)
			}
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "user unbanned successfully",
	})
}

// GetBannedUsers returns all users with ban records
func (h *AdminUserHandler) GetBannedUsers(c *gin.Context) {
	search := c.Query("search")
	status := c.Query("status") // "all", "active", "expired", "permanent"

	users, err := h.userRepo.FindBannedUsers(search, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get banned users"})
		return
	}

	// Map to response format
	type bannedUserResponse struct {
		ID           string `json:"id"`
		DisplayName  string `json:"display_name"`
		Username     string `json:"username"`
		TelegramID   int64  `json:"telegram_id"`
		Role         string `json:"role"`
		AuthProvider string `json:"auth_provider"`
		IsPremium    bool   `json:"is_premium"`
		CreatedAt    string `json:"created_at"`
		// Ban info
		IsBanned         bool    `json:"is_banned"`
		BanReason        string  `json:"ban_reason"`
		BannedAt         string  `json:"banned_at"`
		BannedUntil      *string `json:"banned_until,omitempty"`
		BannedByUsername string  `json:"banned_by_username"`
		BanStatus        string  `json:"ban_status"` // "active", "expired", "permanent"
	}

	usersResp := make([]bannedUserResponse, len(users))
	for i, u := range users {
		var bannedUntilStr *string
		var banStatus string
		isPermanent := u.Ban != nil && u.Ban.BannedUntil == nil && u.Ban.IsBanned
		isExpired := u.Ban != nil && u.Ban.BannedUntil != nil && !u.Ban.BannedUntil.After(time.Now()) && u.Ban.IsBanned
		isActive := u.Ban != nil && u.Ban.IsBanned && (u.Ban.BannedUntil == nil || u.Ban.BannedUntil.After(time.Now()))

		if isPermanent {
			banStatus = "permanent"
		} else if isExpired {
			banStatus = "expired"
		} else if isActive {
			banStatus = "active"
		} else {
			banStatus = "unknown"
		}

		if u.Ban != nil && u.Ban.BannedUntil != nil {
			until := u.Ban.BannedUntil.Format(time.RFC3339)
			bannedUntilStr = &until
		}

		var banReason, bannedByUsername, bannedAt string
		if u.Ban != nil {
			banReason = u.Ban.Reason
			bannedByUsername = u.Ban.BannedByUsername
			if u.Ban.BannedAt != nil {
				bannedAt = u.Ban.BannedAt.Format(time.RFC3339)
			}
		}

		usersResp[i] = bannedUserResponse{
			ID:               u.ID.Hex(),
			DisplayName:      u.DisplayName,
			Username:         u.TelegramUser,
			TelegramID:       u.TelegramID,
			Role:             u.Role,
			AuthProvider:     u.AuthProvider,
			IsPremium:        u.IsPremium,
			CreatedAt:        u.CreatedAt.Format(time.RFC3339),
			IsBanned:         isActive || isPermanent,
			BanReason:        banReason,
			BannedAt:         bannedAt,
			BannedUntil:      bannedUntilStr,
			BannedByUsername: bannedByUsername,
			BanStatus:        banStatus,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  usersResp,
		"total": len(usersResp),
	})
}

// GetBanHistory returns full ban history
func (h *AdminUserHandler) GetBanHistory(c *gin.Context) {
	search := c.Query("search")
	status := c.Query("status") // "all", "active", "unbanned", "expired"

	params := &repositories.BanHistoryQueryParams{
		Search: search,
		Status: status,
	}

	history, err := h.banHistoryRepo.FindAll(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get ban history"})
		return
	}

	// Map to response format with user info
	type historyWithUser struct {
		ID                 string  `json:"id"`
		UserID             string  `json:"user_id"`
		UserDisplayName    string  `json:"user_display_name"`
		UserUsername       string  `json:"user_username"`
		UserTelegramID     int64   `json:"user_telegram_id"`
		Reason             string  `json:"reason"`
		BannedAt           string  `json:"banned_at"`
		BannedUntil        *string `json:"banned_until,omitempty"`
		IsPermanent        bool    `json:"is_permanent"`
		BannedByUsername   string  `json:"banned_by_username"`
		UnbannedAt         *string `json:"unbanned_at,omitempty"`
		UnbannedByUsername *string `json:"unbanned_by_username,omitempty"`
		Status             string  `json:"status"` // "active", "unbanned", "expired"
	}

	historyResp := make([]historyWithUser, 0, len(history))
	for _, bh := range history {
		// Get user info
		user, err := h.userRepo.FindByID(bh.UserID.Hex())
		displayName := ""
		username := ""
		var telegramID int64

		if err == nil && user != nil {
			displayName = user.DisplayName
			username = user.TelegramUser
			telegramID = user.TelegramID
		}

		var bannedUntilStr, unbannedAtStr, unbannedByStr *string
		if bh.BannedUntil != nil {
			s := bh.BannedUntil.Format(time.RFC3339)
			bannedUntilStr = &s
		}
		if bh.UnbannedAt != nil {
			s := bh.UnbannedAt.Format(time.RFC3339)
			unbannedAtStr = &s
		}
		if bh.UnbannedByUsername != nil && *bh.UnbannedByUsername != "" {
			unbannedByStr = bh.UnbannedByUsername
		}

		historyResp = append(historyResp, historyWithUser{
			ID:                 bh.ID.Hex(),
			UserID:             bh.UserID.Hex(),
			UserDisplayName:    displayName,
			UserUsername:       username,
			UserTelegramID:     telegramID,
			Reason:             bh.Reason,
			BannedAt:           bh.BannedAt.Format(time.RFC3339),
			BannedUntil:        bannedUntilStr,
			IsPermanent:        bh.IsPermanent,
			BannedByUsername:   bh.BannedByUsername,
			UnbannedAt:         unbannedAtStr,
			UnbannedByUsername: unbannedByStr,
			Status:             bh.GetStatus(),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  historyResp,
		"total": len(historyResp),
	})
}

// UpdateUserPremium updates a user's premium status
func (h *AdminUserHandler) UpdateUserPremium(c *gin.Context) {
	// Get current user role from context (set by auth middleware)
	currentUserRole, exists := c.Get("role")
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "authentication error"})
		return
	}

	// Only superadmin can change premium status
	if currentUserRole != "superadmin" {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "Only superadmin can change premium status",
		})
		return
	}

	userID := c.Param("id")

	// Check if target user is SuperAdmin - cannot change premium of SuperAdmin
	targetUser, checkErr := h.userRepo.FindByID(userID)
	if checkErr != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if middleware.IsSuperAdmin(targetUser.Role) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "forbidden",
			"message": "SuperAdmin premium holatini o'zgartirish mumkin emas",
		})
		return
	}

	// Parse request body - support both direct date and duration-based assignment
	var req struct {
		IsPremium        bool    `json:"is_premium"`
		PremiumExpiresAt *string `json:"premium_expires_at,omitempty"`
		DurationDays     *int    `json:"duration_days,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Get current user to check existing premium status
	currentUser, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var expiresAt *time.Time

	// Handle removing premium
	if !req.IsPremium {
		err := h.userRepo.UpdateUserPremium(userID, false, nil)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove premium"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":            "premium removed",
			"id":                 userID,
			"is_premium":         false,
			"premium_started_at": nil,
			"premium_expires_at": nil,
		})
		return
	}

	// Handle duration-based premium assignment
	if req.DurationDays != nil && *req.DurationDays > 0 {
		now := time.Now()
		var startTime time.Time

		// If user already has active premium, extend from current expiry
		// Otherwise, start from now
		if currentUser.IsPremium && currentUser.PremiumExpiresAt != nil && currentUser.PremiumExpiresAt.After(now) {
			startTime = *currentUser.PremiumExpiresAt
		} else {
			startTime = now
		}

		duration := time.Duration(*req.DurationDays) * 24 * time.Hour
		newExpiry := startTime.Add(duration)
		expiresAt = &newExpiry
	} else if req.PremiumExpiresAt != nil {
		t, err := time.Parse(time.RFC3339, *req.PremiumExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date format, use RFC3339"})
			return
		}
		expiresAt = &t
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "either duration_days or premium_expires_at required"})
		return
	}

	err = h.userRepo.UpdateUserPremium(userID, true, expiresAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update premium status"})
		return
	}

	// Send notification to user about premium activation
	go func() {
		if h.notificationService != nil {
			userOID, err := primitive.ObjectIDFromHex(userID)
			if err != nil {
				log.Printf("[Notification] Failed to parse user ID for premium activation notification: %v", err)
				return
			}
			err = h.notificationService.NotifyPremiumActivated(c.Request.Context(), userOID, expiresAt)
			if err != nil {
				log.Printf("[Notification] Failed to send premium activated notification: %v", err)
			} else {
				log.Printf("[Notification] Premium activated notification sent to user %s", userID)
			}
		}
	}()

	// Fetch updated user
	user, err := h.userRepo.FindByID(userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"message":            "premium status updated",
			"id":                 userID,
			"is_premium":         true,
			"premium_expires_at": expiresAt.Format(time.RFC3339),
		})
		return
	}

	// Format dates for response
	var startedAtStr, expiresAtStr *string
	if user.PremiumStartedAt != nil {
		started := user.PremiumStartedAt.Format(time.RFC3339)
		startedAtStr = &started
	}
	if user.PremiumExpiresAt != nil {
		expires := user.PremiumExpiresAt.Format(time.RFC3339)
		expiresAtStr = &expires
	}

	c.JSON(http.StatusOK, gin.H{
		"message":            "premium status updated",
		"id":                 user.ID.Hex(),
		"role":               user.Role,
		"is_premium":         user.IsPremium,
		"is_premium_active":  user.IsPremiumActive(),
		"premium_started_at": startedAtStr,
		"premium_expires_at": expiresAtStr,
	})
}
