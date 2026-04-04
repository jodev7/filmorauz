package middleware

import (
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// IsSuperAdmin checks if the given role is superadmin (case-insensitive)
func IsSuperAdmin(role string) bool {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	return normalizedRole == "superadmin"
}

// IsAdmin checks if the given role is admin or superadmin (case-insensitive)
func IsAdmin(role string) bool {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	return normalizedRole == "admin" || normalizedRole == "superadmin"
}

// bannedFlowAllowedPaths contains routes that banned users are allowed to access
// These are the routes needed for the banned flow: viewing /banned page, submitting appeals, etc.
var bannedFlowAllowedPaths = map[string]bool{
	// Appeal endpoints
	"/api/appeals":    true, // POST - create appeal
	"/api/appeals/me": true, // GET - get my appeals
	// Notifications (for banned page to show notifications)
	"/api/notifications":              true, // GET - get notifications
	"/api/notifications/unread-count": true, // GET - unread count
	"/api/notifications/read-all":     true, // PATCH - mark all as read
	"/api/notifications/:id/read":     true, // PATCH - mark single as read
	// Auth endpoints needed for banned page
	"/api/auth/me":     true, // GET - get current user info
	"/api/auth/logout": true, // POST - logout
}

// isBannedFlowAllowed checks if the current path is allowed for banned users
func isBannedFlowAllowed(fullPath string) bool {
	// Direct match
	if bannedFlowAllowedPaths[fullPath] {
		return true
	}

	// Pattern match for /api/notifications/:id/read
	if strings.HasPrefix(fullPath, "/api/notifications/") && strings.HasSuffix(fullPath, "/read") {
		return true
	}

	return false
}

// RequireAuth validates JWT token from Authorization header
func RequireAuth(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Check if user is banned
		userID, ok := claims["user_id"].(string)
		if !ok || userID == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
			c.Abort()
			return
		}

		// Get user from database to check ban status
		user, err := authService.GetCurrentUser(userID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "user not found"})
			c.Abort()
			return
		}

		// Check if user is banned
		if user.Ban != nil && user.Ban.IsBanned {
			// Check if ban has expired
			isActiveBan := user.Ban.BannedUntil == nil || user.Ban.BannedUntil.After(time.Now())

			if isActiveBan {
				// Check if this endpoint is allowed for banned users (banned flow)
				if isBannedFlowAllowed(c.FullPath()) {
					// Allow banned flow endpoints
					c.Set("user_id", claims["user_id"])
					c.Set("email", claims["email"])
					c.Set("role", claims["role"])
					c.Set("banned_user", user)
					c.Next()
					return
				}

				c.JSON(http.StatusForbidden, gin.H{
					"error":              "account_banned",
					"message":            "Siz ban olgansiz",
					"reason":             user.Ban.Reason,
					"banned_at":          user.Ban.BannedAt,
					"banned_until":       user.Ban.BannedUntil,
					"banned_by_username": user.Ban.BannedByUsername,
				})
				c.Abort()
				return
			}
		}

		// Store claims in context for handlers
		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", claims["role"])
		c.Set("banned_user", user)
		c.Next()
	}
}

// RequireAuthNoBan validates JWT token but does NOT check ban status
// Use this for endpoints that need to return user data including ban info
func RequireAuthNoBan(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
			c.Abort()
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired token"})
			c.Abort()
			return
		}

		// Get user from database to include ban info
		userID, _ := claims["user_id"].(string)
		if userID != "" {
			user, err := authService.GetCurrentUser(userID)
			if err == nil {
				c.Set("banned_user", user)
			}
		}

		// Store claims in context for handlers
		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", claims["role"])
		c.Next()
	}
}
