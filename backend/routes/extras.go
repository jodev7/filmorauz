package routes

import (
	"github.com/filmorauz/backend/handlers"
	"github.com/filmorauz/backend/middleware"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// ExtraDeps carries the dependencies of routes registered by SetupExtras.
// New features register here instead of widening Setup's parameter list.
type ExtraDeps struct {
	AuthService  *services.AuthService
	AuditLogRepo *repositories.AuditLogRepository
	Library      *handlers.LibraryHandler
}

// SetupExtras registers user-library, community and growth routes.
// Gin lets several groups share the same prefix, so these live alongside the
// ones in Setup.
func SetupExtras(r *gin.Engine, d ExtraDeps) {
	api := r.Group("/api")

	user := api.Group("/user")
	user.Use(middleware.RequireAuth(d.AuthService))
	{
		// "Keyinroq ko'raman"
		user.GET("/watchlist", d.Library.GetWatchlist)
		user.POST("/watchlist/:type/:id", d.Library.AddToWatchlist)
		user.DELETE("/watchlist/:type/:id", d.Library.RemoveFromWatchlist)
		user.GET("/library/:type/:id", d.Library.LibraryStatus)

		// Series subscriptions (new-episode notifications)
		user.GET("/subscriptions", d.Library.GetSubscriptions)
		user.POST("/subscriptions/series/:id", d.Library.Subscribe)
		user.DELETE("/subscriptions/series/:id", d.Library.Unsubscribe)
	}

	admin := api.Group("/admin")
	admin.Use(middleware.RequireAdmin(d.AuthService))
	admin.Use(middleware.AuditLog(d.AuditLogRepo))
	{
		admin.POST("/suggestions/:id/link", d.Library.LinkSuggestion)
	}
}
