package middleware

import (
	"net/http"
	"strings"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// isAdminRole checks if the given role is admin or superadmin (case-insensitive)
func isAdminRole(role string) bool {
	normalizedRole := strings.ToLower(strings.TrimSpace(role))
	return normalizedRole == "admin" || normalizedRole == "superadmin"
}

// RequireAdmin validates JWT token and checks for admin/superadmin role
func RequireAdmin(authService *services.AuthService) gin.HandlerFunc {
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

		// Store claims in context for handlers
		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", claims["role"])

		// SECURITY: Check if user has admin/superadmin role (case-insensitive)
		role, ok := claims["role"].(string)
		if !ok || !isAdminRole(role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "admin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
