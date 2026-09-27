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
	Referral     *handlers.ReferralHandler
	Errors       *handlers.ErrorHandler
	DailyReport  *handlers.DailyReportHandler
	History      *handlers.HistoryHandler
	Lists        *handlers.UserListHandler
	NotifyPrefs  *handlers.NotifySettingsHandler
	Credits      *handlers.CreditsHandler
	Avatars      *handlers.TelegramAvatarImporter
	LLMS         *handlers.LLMSHandler
}

// SetupExtras registers user-library, community and growth routes.
// Gin lets several groups share the same prefix, so these live alongside the
// ones in Setup.
func SetupExtras(r *gin.Engine, d ExtraDeps) {
	// llms.txt for AI assistants (proxied to the apex domain by Next).
	r.GET("/llms.txt", d.LLMS.GetIndex)
	r.GET("/llms-full.txt", d.LLMS.GetFull)
	r.GET("/llms/movies/:file", d.LLMS.GetMovie)
	r.GET("/llms/series/:file", d.LLMS.GetSeries)

	api := r.Group("/api")

	// Browser error beacon (rate-limited per IP; optional auth for user id).
	api.POST("/client-errors", middleware.OptionalAuth(d.AuthService), d.Errors.ReportClientError)

	// Options for the advanced movie filter (countries, year range).
	api.GET("/movies/filters", middleware.CacheResponse(10*time.Minute), d.Movies.MovieFilterFacets)

	// Discovery: actor/director pages and "random movie".
	api.GET("/movies/random", d.Movies.RandomMovie)
	api.GET("/movies/random-list", d.Movies.RandomMovies)
	api.GET("/people/:name", middleware.CacheResponse(5*time.Minute), d.Movies.PersonCredits)

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

		// Watch history management
		user.DELETE("/continue-watching/:type/:id", d.History.HideFromContinue)
		user.POST("/continue-watching/:type/:id/restore", d.History.RestoreToContinue)
		user.POST("/history/:type/:id/watched", d.History.MarkWatched)
		user.DELETE("/history/:type/:id", d.History.DeleteEntry)
		user.DELETE("/history", d.History.ClearHistory)
		user.GET("/series-progress/:id", d.History.SeriesProgress)
		user.GET("/year-review", d.History.YearReview)

		// Notification settings + web push
		user.GET("/notification-settings", d.NotifyPrefs.Get)
		user.PUT("/notification-settings", d.NotifyPrefs.Update)
		user.POST("/push-subscriptions", d.NotifyPrefs.Subscribe)
		user.DELETE("/push-subscriptions", d.NotifyPrefs.Unsubscribe)
		user.POST("/push-subscriptions/test", d.NotifyPrefs.Test)

		// Personal lists
		user.GET("/lists", d.Lists.Mine)
		user.POST("/lists", d.Lists.Create)
		user.PATCH("/lists/:id", d.Lists.Update)
		user.DELETE("/lists/:id", d.Lists.Delete)
		user.POST("/lists/:id/items/:type/:targetId", d.Lists.AddItem)
		user.DELETE("/lists/:id/items/:type/:targetId", d.Lists.RemoveItem)
		user.GET("/lists-containing/:type/:id", d.Lists.Containing)

		// Referral program
		user.GET("/referral", d.Referral.GetMyReferral)
		user.POST("/referral/claim", d.Referral.ClaimReferral)
	}

	// Shared personal list page
	api.GET("/lists/:slug", middleware.OptionalAuth(d.AuthService), d.Lists.BySlug)

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

		// Single movie for the edit page (any approval status) and
		// scheduled publishing.
		admin.GET("/movies/:id", d.Movies.AdminGetMovie)
		admin.POST("/movies/:id/schedule", d.Movies.ScheduleMovie)
		admin.DELETE("/movies/:id/schedule", d.Movies.CancelMovieSchedule)

		// TMDB cast + photos (also filled automatically in the background).
		admin.POST("/movies/:id/credits", d.Credits.FetchMovie)
		admin.POST("/series/:id/credits", d.Credits.FetchSeries)
		admin.POST("/credits/backfill", d.Credits.Backfill)
		admin.GET("/tmdb/search", d.Credits.Search)

		// llms.txt status / rebuild now (SEO page)
		admin.GET("/seo/llms", d.LLMS.Status)
		admin.POST("/seo/llms", d.LLMS.Status)

		// Error tracking (client + server)
		admin.GET("/errors", d.Errors.ListErrors)
		admin.POST("/errors/:id/resolve", d.Errors.ResolveError)
	}

	// Daily Telegram report (preview / send now).
	superadmin := api.Group("/superadmin")
	superadmin.Use(middleware.RequireSuperAdmin(d.AuthService))
	superadmin.Use(middleware.AuditLog(d.AuditLogRepo))
	{
		superadmin.GET("/daily-report/preview", d.DailyReport.Preview)
		superadmin.POST("/daily-report/send", d.DailyReport.SendNow)
		superadmin.POST("/users/telegram-avatars/backfill", d.Avatars.BackfillNow)
	}
}
