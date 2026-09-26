package routes

import (
	"time"

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
	Movies       *handlers.MovieHandler
	Community    *handlers.CommunityHandler
}

// SetupExtras registers user-library, community and growth routes.
// Gin lets several groups share the same prefix, so these live alongside the
// ones in Setup.
func SetupExtras(r *gin.Engine, d ExtraDeps) {
	api := r.Group("/api")

	// Options for the advanced movie filter (countries, year range).
	api.GET("/movies/filters", middleware.CacheResponse(10*time.Minute), d.Movies.MovieFilterFacets)

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

		// "Siz uchun" personal recommendations
		user.GET("/for-you", d.Library.ForYou)
	}

	// Reviews ("qisqa taqriz") — public list, optional auth for "mine"/helpful.
	api.GET("/reviews/:type/:id", middleware.OptionalAuth(d.AuthService), d.Community.ListReviews)
	user.POST("/reviews/:type/:id", d.Community.UpsertReview)
	user.DELETE("/reviews/:type/:id", d.Community.DeleteMyReview)
	user.POST("/review-helpful/:id", d.Community.ToggleReviewHelpful)

	// Comment reports ("shikoyat")
	v1 := api.Group("/v1")
	v1.POST("/comments/:id/report", middleware.RequireAuth(d.AuthService), d.Community.ReportComment)

	v1Admin := v1.Group("/admin")
	v1Admin.Use(middleware.RequireAdmin(d.AuthService))
	v1Admin.Use(middleware.AuditLog(d.AuditLogRepo))
	{
		v1Admin.GET("/comments/reported", d.Community.ListReportedComments)
		v1Admin.POST("/comments/:id/dismiss-reports", d.Community.DismissCommentReports)
		v1Admin.DELETE("/reviews/:id", d.Community.AdminDeleteReview)
	}

	admin := api.Group("/admin")
	admin.Use(middleware.RequireAdmin(d.AuthService))
	admin.Use(middleware.AuditLog(d.AuditLogRepo))
	{
		admin.POST("/suggestions/:id/link", d.Library.LinkSuggestion)
	}
}
