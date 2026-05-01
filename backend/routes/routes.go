package routes

import (
	"github.com/filmorauz/backend/handlers"
	"github.com/filmorauz/backend/middleware"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

func Setup(r *gin.Engine, authHandler *handlers.AuthHandler, movieHandler *handlers.MovieHandler, homepageHandler *handlers.HomepageHandler, ingestionHandler *handlers.IngestionHandler, uploadHandler *handlers.UploadHandler, adminUserHandler *handlers.AdminUserHandler, userHandler *handlers.UserHandler, collectionHandler *handlers.CollectionHandler, authService *services.AuthService, ratingHandler *handlers.RatingHandler, commentHandler *handlers.CommentHandler, shareHandler *handlers.ShareHandler, seriesHandler *handlers.SeriesHandler, mediaHandler *handlers.MediaHandler, banAppealHandler *handlers.BanAppealHandler, notificationHandler *handlers.NotificationHandler, telegramHandler *handlers.TelegramHandler, clipHandler *handlers.ClipHandler, adHandler *handlers.AdHandler, telegramPostHandler *handlers.TelegramPostHandler, igScheduleHandler *handlers.InstagramScheduleHandler, publishJobHandler *handlers.PublishJobHandler, suggestionHandler *handlers.SuggestionHandler, premiumHandler *handlers.PremiumHandler) {
	api := r.Group("/api")

	// Health check
	api.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Telegram Auth routes (public)
	api.POST("/auth/telegram/register", authHandler.RegisterBotUser)
	api.POST("/auth/telegram/start", authHandler.TelegramAuthStart)
	api.POST("/auth/telegram/complete", authHandler.TelegramAuthComplete)
	api.GET("/auth/telegram/status/:code", authHandler.TelegramAuthStatus)

	// Telegram notification route (for worker to call after movie creation)
	api.POST("/telegram/notify-movie", telegramHandler.NotifyMovie)
	api.GET("/telegram/health", telegramHandler.HealthCheck)

	// Protected auth routes
	api.GET("/auth/me", middleware.RequireAuthNoBan(authService), authHandler.CurrentUser)
	api.GET("/auth/bootstrap", middleware.RequireAuthNoBan(authService), authHandler.Bootstrap)
	api.PATCH("/auth/me", middleware.RequireAuth(authService), authHandler.UpdateProfile)
	api.PATCH("/auth/me/language", middleware.RequireAuth(authService), authHandler.UpdateLanguageCode)
	api.POST("/auth/upload/profile-image", middleware.RequireAuth(authService), uploadHandler.UploadProfileImage)
	api.PATCH("/auth/profile-style", middleware.RequireAuth(authService), authHandler.UpdateProfileStyle)
	api.PATCH("/auth/privacy", middleware.RequireAuth(authService), authHandler.UpdatePrivacy)
	api.POST("/auth/logout", middleware.RequireAuth(authService), authHandler.Logout)
	api.POST("/auth/refresh", middleware.RequireAuth(authService), authHandler.RefreshToken)

	// Ban appeal routes (for banned users)
	appeals := api.Group("/appeals")
	appeals.Use(middleware.RequireAuth(authService))
	{
		appeals.POST("", banAppealHandler.CreateAppeal)   // Submit appeal (banned users only)
		appeals.GET("/me", banAppealHandler.GetMyAppeals) // Get my appeals
	}

	// Notification routes (for all authenticated users)
	notifications := api.Group("/notifications")
	notifications.Use(middleware.RequireAuth(authService))
	{
		notifications.GET("", notificationHandler.GetNotifications)
		notifications.GET("/unread-count", notificationHandler.GetUnreadCount)
		notifications.PATCH("/:id/read", notificationHandler.MarkAsRead)
		notifications.PATCH("/read-all", notificationHandler.MarkAllAsRead)
	}

	// Suggestion routes (for authenticated users)
	suggestions := api.Group("/suggestions")
	suggestions.Use(middleware.RequireAuth(authService))
	{
		suggestions.POST("", suggestionHandler.CreateSuggestion)
		suggestions.GET("", suggestionHandler.GetMySuggestions)
	}

	// User-protected routes (history, favorites)
	user := api.Group("/user")
	user.Use(middleware.RequireAuth(authService))
	{
		// Watch history
		user.POST("/history/watch", userHandler.RecordWatchHistory)
		user.GET("/history", userHandler.GetWatchHistory)

		// Continue Watching
		user.GET("/continue-watching", userHandler.GetContinueWatching)

		// Favorites
		user.POST("/favorites/:movieId", userHandler.AddFavorite)
		user.DELETE("/favorites/:movieId", userHandler.RemoveFavorite)
		user.GET("/favorites", userHandler.GetFavorites)

		// Premium purchase via wallet
		user.POST("/premium/buy", userHandler.BuyPremium)
	}

	// Public premium package list (Telegram Stars)
	api.GET("/premium/packages", premiumHandler.GetPackages)
	api.POST("/premium/stars/session", middleware.RequireAuth(authService), premiumHandler.CreateStarsSession)

	// Internal: bot calls this after a Telegram Stars successful_payment.
	// Auth via shared X-Internal-Token header (BOT_INTERNAL_TOKEN env).
	api.POST("/internal/premium/telegram-stars/session/validate", premiumHandler.ValidateStarsSession)
	api.POST("/internal/premium/telegram-stars/session/cancel", premiumHandler.CancelStarsSession)
	api.POST("/internal/premium/telegram-stars/grant", premiumHandler.GrantTelegramStars)

	// Watch progress (auth required)
	watch := api.Group("/watch")
	watch.Use(middleware.RequireAuth(authService))
	{
		watch.GET("/progress", userHandler.GetWatchProgress)
		watch.POST("/progress", userHandler.SaveWatchProgressGeneric)
		watch.POST("/progress/reset", userHandler.ResetWatchProgress)
		watch.POST("/:movieId/progress", userHandler.SaveWatchProgress)
		watch.POST("/:movieId/complete", userHandler.MarkWatchComplete)
	}

	// Public view counter (no auth required) - must be before :slug routes
	api.POST("/movies/:id/view", userHandler.RecordView)

	// Public user profile (optional auth — needed to enforce privacy for owner/admin)
	api.GET("/users/:id", middleware.OptionalAuth(authService), userHandler.GetPublicProfile)
	api.GET("/media/access-token", middleware.OptionalAuth(authService), mediaHandler.GetMediaToken)

	// Public movie routes
	api.GET("/homepage", homepageHandler.GetHomepageData)
	api.GET("/movies", movieHandler.ListMovies)
	api.GET("/movies/trending", movieHandler.GetTrendingMovies)
	// Movie by slug (must come before :id routes to avoid slug being treated as id)
	api.GET("/movies/slug/:slug", movieHandler.GetMovieBySlug)
	// Movie by ID and recommendations
	api.GET("/movies/:id", movieHandler.GetMovieByID)
	api.GET("/movies/recommendations", movieHandler.GetRecommendations) // ?movie_id=xxx&limit=12
	api.GET("/movies/:id/recommendations", movieHandler.GetRecommendations)
	api.GET("/search", movieHandler.SearchMovies)

	// Protected movie watch endpoint (requires auth + premium check)
	v1Watch := api.Group("/v1")
	v1Movies := v1Watch.Group("/movies")
	v1Movies.Use(middleware.RequireAuth(authService))
	{
		v1Movies.GET("/:id/watch", movieHandler.GetMovieWatchSource)
	}

	// Movie rating routes (v1)
	v1 := api.Group("/v1")
	{
		// Rating summary (public - optional auth to get user rating)
		v1.GET("/movies/:id/rating-summary", middleware.OptionalAuth(authService), ratingHandler.GetRatingSummary)

		// Rating (auth required)
		rating := v1.Group("/movies/:id/rating")
		rating.Use(middleware.RequireAuth(authService))
		{
			rating.POST("", ratingHandler.SetRating)
			rating.DELETE("", ratingHandler.DeleteRating)
		}

		// Series rating summary (public - optional auth to get user rating)
		v1.GET("/series/:id/rating-summary", middleware.OptionalAuth(authService), ratingHandler.GetSeriesRatingSummary)

		// Episode rating
		v1.GET("/episodes/:id/rating-summary", middleware.OptionalAuth(authService), ratingHandler.GetEpisodeRatingSummary)
		episodeRating := v1.Group("/episodes/:id/rating")
		episodeRating.Use(middleware.RequireAuth(authService))
		{
			episodeRating.POST("", ratingHandler.SetEpisodeRating)
			episodeRating.DELETE("", ratingHandler.DeleteEpisodeRating)
		}

		// Series rating (auth required)
		seriesRating := v1.Group("/series/:id/rating")
		seriesRating.Use(middleware.RequireAuth(authService))
		{
			seriesRating.POST("", ratingHandler.SetSeriesRating)
			seriesRating.DELETE("", ratingHandler.DeleteSeriesRating)
		}
	}

	// Share routes (public)
	api.POST("/movies/share", shareHandler.CreateShare)
	api.POST("/shares/:code/open", shareHandler.RecordShareOpen)
	api.GET("/movies/share-stats", shareHandler.GetMovieShareStats)

	// User share stats (auth required)
	userShares := api.Group("/user")
	userShares.Use(middleware.RequireAuth(authService))
	{
		userShares.GET("/share-stats", shareHandler.GetUserShareStats)
	}

	// Public movie code lookup (for Telegram bot)
	api.GET("/public/movies/code/:code", movieHandler.GetMovieByCode)

	// Ingestion routes (public for admin UI - auth handled separately)
	api.GET("/ingestion/search", ingestionHandler.SearchSource)
	api.GET("/ingestion/details", ingestionHandler.GetMovieDetails)
	api.GET("/ingestion/catalog/categories", ingestionHandler.GetCatalogCategories)
	api.GET("/ingestion/catalog", ingestionHandler.ListCatalog)

	// Progress endpoint is public - called by parser without auth
	api.POST("/ingestion/jobs/:id/progress", ingestionHandler.UpdateJobProgress)

	// Process endpoint - triggers worker to process downloaded video
	api.POST("/ingestion/jobs/:id/process", ingestionHandler.ProcessIngestionJob)

	// Episode complete endpoint — worker posts here after HLS upload so the
	// backend can attach the final playback URL to the Episode record.
	api.POST("/ingestion/episodes/:id/complete", ingestionHandler.CompleteEpisode)

	// Worker claim endpoint - worker calls this to get a job for ffmpeg processing
	// Returns job where steps.download=true and steps.process=false
	api.GET("/ingestion/jobs/worker/claim", ingestionHandler.WorkerClaimJob)
	api.GET("/ingestion/jobs/parser/claim", ingestionHandler.ParserClaimJob)

	// Get presigned upload URL for direct B2 upload
	api.GET("/get-upload-url", ingestionHandler.GetUploadURL)
	api.GET("/upload/b2-url", middleware.RequireAdmin(authService), uploadHandler.GetB2UploadURL)
	api.POST("/upload/b2-complete", middleware.RequireAdmin(authService), uploadHandler.CompleteB2Upload)

	// Admin routes — protected by JWT and admin role check
	admin := api.Group("/admin")
	admin.Use(middleware.RequireAdmin(authService))
	{
		// Movie asset uploads
		admin.POST("/movies/upload", uploadHandler.UploadMovieAssets)
		admin.POST("/upload/movie-poster", uploadHandler.UploadMoviePoster)
		admin.POST("/upload/movie-backdrop", uploadHandler.UploadMovieBackdrop)
		admin.POST("/upload/series-poster", uploadHandler.UploadSeriesPoster)
		admin.POST("/upload/series-backdrop", uploadHandler.UploadSeriesBackdrop)
		admin.POST("/upload/collection-poster", uploadHandler.UploadCollectionPoster)
		admin.POST("/upload/ad-media", uploadHandler.UploadAdMedia)
		admin.POST("/upload/telegram-post-media", uploadHandler.UploadTelegramPostMedia)

		// Proxy upload to B2 (avoids CORS issue with direct browser upload)
		admin.POST("/upload-temp", uploadHandler.UploadTemp)

		// Movie management
		admin.GET("/movies", movieHandler.AdminListMovies)
		admin.POST("/movies", movieHandler.CreateMovie)
		admin.PUT("/movies/:id", movieHandler.UpdateMovie)
		admin.DELETE("/movies/:id", movieHandler.DeleteMovie)
		admin.PATCH("/movies/:id/approve", movieHandler.ApproveMovie)
		admin.PATCH("/movies/:id/reject", movieHandler.RejectMovie)

		// Ingestion job management
		admin.POST("/ingestion/jobs", ingestionHandler.CreateIngestionJob)
		admin.POST("/ingestion/direct-upload", ingestionHandler.CreateDirectUploadJob)
		admin.GET("/ingestion/jobs", ingestionHandler.ListIngestionJobs)
		admin.GET("/ingestion/jobs/:id", ingestionHandler.GetIngestionJob)
		admin.POST("/ingestion/jobs/:id/retry", ingestionHandler.RetryIngestionJob)
		admin.DELETE("/ingestion/jobs/:id", ingestionHandler.DeleteIngestionJob)
		// Manual import from direct video URL
		admin.POST("/ingestion/manual", ingestionHandler.CreateManualJob)
		// Import from catalog
		admin.POST("/ingestion/import", ingestionHandler.ImportFromCatalog)

		// User management
		admin.GET("/dashboard/stats", adminUserHandler.DashboardStats)
		admin.GET("/analytics/top-movies", adminUserHandler.GetTopMovies)
		admin.GET("/analytics/top-series", adminUserHandler.GetTopSeries)
		admin.GET("/analytics/users", adminUserHandler.GetUserMetrics)
		admin.GET("/users", adminUserHandler.ListUsers)
		admin.GET("/users/banned", adminUserHandler.GetBannedUsers)
		admin.GET("/users/ban-history", adminUserHandler.GetBanHistory)
		admin.PATCH("/users/:id/role/:role", adminUserHandler.UpdateUserRole)
		admin.PATCH("/v1/users/:id/role", adminUserHandler.UpdateUserRole)
		admin.PATCH("/users/:id/premium", adminUserHandler.UpdateUserPremium)
		admin.POST("/users/:id/ban", adminUserHandler.BanUser)
		admin.DELETE("/users/:id/ban", adminUserHandler.UnbanUser)

		// Ban appeals management
		admin.GET("/appeals", banAppealHandler.GetAppeals)
		admin.GET("/appeals/stats", banAppealHandler.GetAppealStats)
		admin.GET("/appeals/pending-count", banAppealHandler.GetPendingCount)
		admin.POST("/appeals/:id/review", banAppealHandler.ReviewAppeal)

		// Share analytics
		admin.GET("/share-stats", shareHandler.GetAdminShareStats)

		// Collection management
		admin.POST("/collections", collectionHandler.CreateCollection)
		admin.GET("/collections", collectionHandler.ListCollections)
		admin.GET("/collections/:id", collectionHandler.GetCollection)
		admin.PUT("/collections/:id", collectionHandler.UpdateCollection)
		admin.DELETE("/collections/:id", collectionHandler.DeleteCollection)

		// Clip management
		admin.GET("/clips/groups", clipHandler.ListClipGroups)
		admin.GET("/clips/groups/debug", clipHandler.GetClipGroupsDebug)
		admin.GET("/clips", clipHandler.ListClips)
		admin.GET("/clips/movie/:movieId", clipHandler.GetClipsByMovie)
		admin.GET("/clips/:id/download", clipHandler.DownloadClip)
		admin.POST("/clips", clipHandler.SaveClips)
		admin.DELETE("/clips/movie/:movieId", clipHandler.DeleteClipsByMovie)
		admin.POST("/clips/:id/instagram", clipHandler.UploadToInstagram)
		admin.POST("/clips/:id/instagram/schedule", igScheduleHandler.Create)
		admin.GET("/clips/:id/instagram/schedules", igScheduleHandler.ListForClip)
		admin.GET("/instagram/accounts", clipHandler.ListInstagramAccounts)
		admin.GET("/instagram/schedules", igScheduleHandler.ListAll)
		admin.PATCH("/instagram/schedules/:scheduleId", igScheduleHandler.UpdateTime)
		admin.DELETE("/instagram/schedules/:scheduleId", igScheduleHandler.Cancel)

		// Multi-platform publish jobs
		admin.GET("/publish/accounts", publishJobHandler.ListAccounts)
		admin.POST("/clips/:id/publish/now", publishJobHandler.UploadNow)
		admin.POST("/clips/:id/publish/schedule", publishJobHandler.Schedule)
		admin.GET("/clips/:id/publish/jobs", publishJobHandler.ListForClip)
		admin.GET("/publish/jobs", publishJobHandler.ListAll)
		admin.PATCH("/publish/jobs/:jobId", publishJobHandler.UpdateTime)
		admin.DELETE("/publish/jobs/:jobId", publishJobHandler.Cancel)

		// Suggestion management
		admin.GET("/suggestions", suggestionHandler.AdminListSuggestions)
		admin.GET("/suggestions/stats", suggestionHandler.AdminGetStats)
		admin.GET("/suggestions/:id", suggestionHandler.AdminGetSuggestion)
		admin.PATCH("/suggestions/:id", suggestionHandler.AdminUpdateSuggestion)
	}

	// Public collection routes
	collections := api.Group("/collections")
	{
		collections.GET("", collectionHandler.GetCollections)
		collections.GET("/featured", collectionHandler.GetFeaturedCollections)
		collections.GET("/slug/:slug", collectionHandler.GetCollectionBySlug)
	}

	// Comment routes (v1)
	v1Comments := api.Group("/v1")
	{
		// Public - get comments by target (movie or episode)
		v1Comments.GET("/comments", middleware.OptionalAuth(authService), commentHandler.GetCommentsByTarget)

		// Public - get movie comments (backward compatibility)
		v1Comments.GET("/movies/:id/comments", middleware.OptionalAuth(authService), commentHandler.GetComments)

		// Auth required - create comment, reply, edit, delete
		comments := v1Comments.Group("/movies/:id/comments")
		comments.Use(middleware.RequireAuth(authService))
		{
			comments.POST("", commentHandler.CreateComment)
		}

		// Reply to comment
		replies := v1Comments.Group("/comments")
		replies.Use(middleware.RequireAuth(authService))
		{
			replies.POST("/:id/replies", commentHandler.CreateReply)
			replies.POST("/:id/like", commentHandler.ToggleCommentLike)
			replies.PUT("/:id", commentHandler.UpdateComment)
			replies.DELETE("/:id", commentHandler.DeleteComment)
		}
	}

	// Admin comment moderation - under /api/v1/admin/ (requires admin role)
	v1Admin := v1.Group("/admin")
	v1Admin.Use(middleware.RequireAdmin(authService))
	{
		v1Admin.GET("/comments", commentHandler.AdminGetComments)
		v1Admin.PATCH("/comments/:id/status", commentHandler.AdminUpdateCommentStatus)
		v1Admin.DELETE("/comments/:id", commentHandler.AdminDeleteComment)
		v1Admin.GET("/comment-settings", commentHandler.AdminGetCommentSettings)
		v1Admin.PUT("/comment-settings", commentHandler.AdminUpdateCommentSettings)
	}

	// Series routes
	// NOTE: More specific routes (with more path segments) must come BEFORE less specific ones
	// to avoid wildcard conflicts in Gin

	// Season routes - must come before series/:slug
	seasons := api.Group("/seasons")
	{
		seasons.GET("/:id/episodes", seriesHandler.GetEpisodes)
	}

	// Episode routes
	episodes := api.Group("/episodes")
	{
		episodes.GET("/:id", seriesHandler.GetEpisode)
	}

	// Series by ID routes - must come before /series/:slug
	seriesByID := api.Group("/series-by-id")
	{
		seriesByID.GET("/:id/seasons", seriesHandler.GetSeasons)
	}

	// Series routes - least specific (just /series and /series/:slug)
	series := api.Group("/series")
	{
		series.GET("", seriesHandler.ListSeries)
		series.GET("/:slug", seriesHandler.GetSeriesBySlug)
	}

	// Admin series management
	adminSeries := admin.Group("/series")
	{
		adminSeries.GET("", seriesHandler.AdminListSeries)
		adminSeries.POST("", seriesHandler.CreateSeries)
		adminSeries.PUT("/:id", seriesHandler.UpdateSeries)
		adminSeries.DELETE("/:id", seriesHandler.DeleteSeries)
		adminSeries.PATCH("/:id/approve", seriesHandler.ApproveSeries)
		adminSeries.PATCH("/:id/reject", seriesHandler.RejectSeries)
		adminSeries.POST("/:id/seasons", seriesHandler.CreateSeason)
	}

	// Admin season/episode management
	adminSeasons := admin.Group("/seasons")
	{
		adminSeasons.PUT("/:id", seriesHandler.UpdateSeason)
		adminSeasons.DELETE("/:id", seriesHandler.DeleteSeason)
		adminSeasons.POST("/:id/episodes", seriesHandler.CreateEpisode)
		adminSeasons.POST("/:id/episodes/reorder", seriesHandler.ReorderEpisodes)
		adminSeasons.PUT("/:id/episodes", seriesHandler.UpdateSeasonEpisodes)
	}

	// Admin episode management
	adminEpisodes := admin.Group("/episodes")
	{
		adminEpisodes.PUT("/:id", seriesHandler.UpdateEpisode)
		adminEpisodes.DELETE("/:id", seriesHandler.DeleteEpisode)
		adminEpisodes.POST("/:id/move", seriesHandler.MoveEpisodeToSeason)
	}

	// Superadmin-only ad management
	superadmin := api.Group("/superadmin")
	superadmin.Use(middleware.RequireSuperAdmin(authService))
	{
		superadmin.GET("/ads", adHandler.AdminListAds)
		superadmin.GET("/ads/stats", adHandler.AdminGetStats)
		superadmin.POST("/ads", adHandler.AdminCreateAd)
		superadmin.PUT("/ads/:id", adHandler.AdminUpdateAd)
		superadmin.DELETE("/ads/:id", adHandler.AdminDeleteAd)
		superadmin.POST("/ads/upload", uploadHandler.UploadAdMedia)
		superadmin.POST("/ads/:id/send-telegram", adHandler.SendTelegramAd)
		superadmin.GET("/ads/:id/delivery", adHandler.GetAdDelivery)
	}

	// Superadmin-only wallet management
	superadmin.PATCH("/users/:id/wallet", adminUserHandler.UpdateUserWallet)

	// Superadmin-only telegram post
	superadmin.GET("/telegram-posts", telegramPostHandler.ListPosts)
	superadmin.POST("/telegram-post", telegramPostHandler.SendPost)
	superadmin.POST("/telegram-post/upload", uploadHandler.UploadTelegramPostMedia)

	// Public ad routes
	api.GET("/ads", adHandler.GetAdsByPlacement)
	api.GET("/ads/batch", adHandler.GetAdsBatch)
	api.POST("/ads/:id/impression", adHandler.RecordImpression)
	api.POST("/ads/:id/click", adHandler.RecordClick)
}
