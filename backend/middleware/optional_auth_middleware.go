package middleware

import (
	"strings"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// OptionalAuth extracts JWT token from Authorization header if present, but doesn't require it
func OptionalAuth(authService *services.AuthService) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			// No auth header - continue without user context (anonymous)
			c.Next()
			return
		}

		// Expect "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			// Invalid format - continue without user context (anonymous)
			c.Next()
			return
		}

		claims, err := authService.ValidateToken(parts[1])
		if err != nil {
			// Invalid token - continue without user context (anonymous)
			c.Next()
			return
		}

		// Store claims in context for handlers
		c.Set("user_id", claims["user_id"])
		c.Set("email", claims["email"])
		c.Set("role", claims["role"])
		c.Next()
	}
}
