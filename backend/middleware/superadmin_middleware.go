package middleware

import (
	"net/http"
	"strings"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// RequireSuperAdmin validates JWT and requires superadmin role specifically.
func RequireSuperAdmin(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authorization header required"})
			c.Abort()
			return
		}

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

		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", claims["role"])

		role, ok := claims["role"].(string)
		if !ok || strings.ToLower(strings.TrimSpace(role)) != "superadmin" {
			c.JSON(http.StatusForbidden, gin.H{"error": "superadmin access required"})
			c.Abort()
			return
		}

		c.Next()
	}
}
