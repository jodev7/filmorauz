package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/handlers"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/routes"
	"github.com/filmorauz/backend/services"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	cfg := config.Load()

	// Set Gin mode based on app mode
	if cfg.IsProd {
		gin.SetMode(gin.ReleaseMode)
	} else {
		gin.SetMode(gin.DebugMode)
	}

	// Connect to MongoDB
	db := connectMongo(cfg.MongoURI, cfg.DBName)

	// Initialize layers
	userRepo := repositories.NewUserRepository(db)
	authSessionRepo := repositories.NewAuthSessionRepository(db)
	movieRepo := repositories.NewMovieRepository(db)
	seriesRepo := repositories.NewSeriesRepository(db)
	counterRepo := repositories.NewCounterRepository(db)
	jobRepo := repositories.NewJobRepository(db)
	movieViewEventRepo := repositories.NewMovieViewEventRepository(db)

	// Watch history and favorites repositories
	watchHistoryRepo := repositories.NewWatchHistoryRepository(db)
	favoriteRepo := repositories.NewFavoriteRepository(db)

	// Rating repository
	ratingRepo := repositories.NewMovieRatingRepository(db)
	seriesRatingRepo := repositories.NewSeriesRatingRepository(db)
	episodeRatingRepo := repositories.NewEpisodeRatingRepository(db)

	// Share repository
	shareRepo := repositories.NewShareRepository(db)

	// Ban history repository
	banHistoryRepo := repositories.NewBanHistoryRepository(db)

	// Collection repository and service
	collectionRepo := repositories.NewCollectionRepository(db)
	collectionService := services.NewCollectionService(collectionRepo, movieRepo)

	// Ensure indexes
	if err := userRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure user indexes: %v", err)
	}
	if err := authSessionRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure auth session indexes: %v", err)
	}
	if err := watchHistoryRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure watch history indexes: %v", err)
	}
	if err := favoriteRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure favorite indexes: %v", err)
	}
	if err := ratingRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure rating indexes: %v", err)
	}
	if err := seriesRatingRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure series rating indexes: %v", err)
	}
	if err := episodeRatingRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure episode rating indexes: %v", err)
	}
	if err := shareRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure share indexes: %v", err)
	}

	// Initialize notification service
	botToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	notificationConfig := services.NotificationConfig{
		BotToken:        botToken,
		BotUsername:     cfg.TelegramBotUsername,
		ChannelUsername: cfg.TelegramChannelUsername,
	}
	notificationService, err := services.NewNotificationServiceWithConfig(notificationConfig)
	if err != nil {
		log.Printf("Warning: Failed to initialize notification service: %v", err)
	}

	// Initialize notification repository for in-app notifications
	notificationRepo := repositories.NewNotificationRepository(db)
	notificationService.SetRepositories(notificationRepo, userRepo)

	authService := services.NewAuthService(userRepo, authSessionRepo, cfg.JWTSecret)

	// Set bot username for deep links
	authService.SetBotUsername(cfg.TelegramBotUsername)
	movieService := services.NewMovieService(movieRepo, seriesRepo, counterRepo, notificationService, movieViewEventRepo)
	seriesService := services.NewSeriesService(seriesRepo, movieRepo)

	// Rating service
	ratingService := services.NewRatingService(ratingRepo, seriesRatingRepo, episodeRatingRepo, movieRepo, seriesRepo)

	// Share service
	shareService := services.NewShareService(shareRepo, movieRepo, userRepo, cfg.BaseSiteURL)

	// Seed admin user on first run
	seedAdmin(userRepo, cfg.AdminTelegramID)

	// Backfill movie codes for existing movies that don't have one
	movieService.BackfillMovieCodes()
	seriesService.BackfillSeriesCodes()

	// Setup Gin
	r := gin.Default()
	r.Use(func(c *gin.Context) {
		if strings.HasPrefix(c.Request.URL.Path, "/uploads/") || strings.HasPrefix(c.Request.URL.Path, "/stream/") {
			c.Header("Cache-Control", "public, max-age=31536000, immutable")
		}
		c.Next()
	})
	// Allow multipart forms up to 32 MB in memory (excess spills to disk).
	// This must be set before any route handler that calls c.Request.FormFile().
	r.MaxMultipartMemory = 32 << 20 // 32 MB

	// CORS config
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept", "X-Requested-With"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	if cfg.IsDev {
		// In dev, allow localhost dev origins
		corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000", "http://localhost:3001", "http://127.0.0.1:3001"}
	} else {
		// In prod, parse comma-separated ALLOWED_ORIGIN from .env
		if cfg.AllowedOrigin == "" {
			log.Fatal("ALLOWED_ORIGIN is required in PROD mode (e.g. https://filmorauz.net)")
		}
		var origins []string
		for _, origin := range strings.Split(cfg.AllowedOrigin, ",") {
			origin = strings.TrimSpace(origin)
			if origin != "" {
				origins = append(origins, origin)
			}
		}
		if len(origins) == 0 {
			log.Fatal("ALLOWED_ORIGIN is empty after parsing")
		}
		// Always allow the canonical prod origin so a misconfigured
		// ALLOWED_ORIGIN can't lock the frontend out of the API.
		hasCanonical := false
		for _, o := range origins {
			if o == "https://filmorauz.net" {
				hasCanonical = true
				break
			}
		}
		if !hasCanonical {
			origins = append(origins, "https://filmorauz.net")
		}
		corsConfig.AllowOrigins = origins
		log.Printf("CORS allowing origins: %v", origins)
	}

	r.Use(cors.New(corsConfig))

	// Get parser URL from config - supports both PARSER_SERVICE_URL and PARSER_URL
	parserURL := getParserURL()

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService, notificationService)
	movieHandler := handlers.NewMovieHandler(movieService, seriesService, userRepo)
	ingestionHandler := handlers.NewIngestionHandler(jobRepo, seriesRepo, seriesService, parserURL, cfg.WorkerUploadURL)
	uploadHandler := handlers.NewUploadHandler(userRepo, cfg)
	adminUserHandler := handlers.NewAdminUserHandler(userRepo, movieRepo, seriesRepo, banHistoryRepo, notificationService)
	userHandler := handlers.NewUserHandler(watchHistoryRepo, favoriteRepo, movieRepo, seriesRepo, userRepo)
	collectionHandler := handlers.NewCollectionHandler(collectionService)
	ratingHandler := handlers.NewRatingHandler(ratingService)

	// Comment repository, service, and handler
	commentRepo := repositories.NewCommentRepository(db)
	commentService := services.NewCommentService(commentRepo, userRepo)
	commentHandler := handlers.NewCommentHandler(commentService, notificationService, userRepo, movieRepo)

	// Share handler
	shareHandler := handlers.NewShareHandler(shareService)

	// Ban appeal repository and handler
	banAppealRepo := repositories.NewBanAppealRepository(db)
	banAppealHandler := handlers.NewBanAppealHandler(banAppealRepo, banHistoryRepo, userRepo, notificationService)

	// Ad repository and handler (Phase 2: telegramService wired after it's initialized below)
	adRepo := repositories.NewAdRepository(db)
	telegramPostRepo := repositories.NewTelegramPostRepository(db)
	if err := adRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure ad indexes: %v", err)
	}

	// One-shot migration: strip legacy "/media/" prefix from stored image/video URLs
	// so API responses match the canonical "/images/..." form the frontend expects.
	if migrated, err := adRepo.StripLegacyMediaPrefix(); err != nil {
		log.Printf("Warning: ad /media/ prefix migration failed: %v", err)
	} else if migrated > 0 {
		log.Printf("[MIGRATION] Stripped legacy /media/ prefix from %d ad media URL(s)", migrated)
	}
	if migrated, err := telegramPostRepo.StripLegacyMediaPrefix(); err != nil {
		log.Printf("Warning: telegram_posts /media/ prefix migration failed: %v", err)
	} else if migrated > 0 {
		log.Printf("[MIGRATION] Stripped legacy /media/ prefix from %d telegram post image_url(s)", migrated)
	}

	// Notification handler
	notificationHandler := handlers.NewNotificationHandler(notificationService)

	// Initialize Telegram service for movie notifications
	telegramService, err := services.NewTelegramService(services.TelegramConfig{
		BotToken:               botToken,
		BotUsername:            cfg.TelegramBotUsername,
		ChannelUsername:        cfg.TelegramChannelUsername,
		SerialsChannelUsername: cfg.TelegramSerialsChannel,
		AdminTelegramID:        cfg.AdminTelegramID,
		BaseSiteURL:            cfg.BaseSiteURL,
		ChannelsList:           cfg.TelegramChannels,
	})
	if err != nil {
		log.Printf("Warning: Failed to initialize Telegram service: %v", err)
	}
	telegramHandler := handlers.NewTelegramHandler(telegramService)

	// Ad handler (wired after telegramService is available)
	adHandler := handlers.NewAdHandler(adRepo, telegramService, cfg.TelegramChannels, userRepo)

	// Telegram post handler
	telegramPostHandler := handlers.NewTelegramPostHandler(telegramService, userRepo, telegramPostRepo)

	// Wire Telegram service into handlers that need it for approval auto-posts
	if telegramService != nil {
		movieHandler.SetTelegramService(telegramService)
	}

	// Series repository, service, and handler
	// seriesRepo already initialized above for rating service
	seriesHandler := handlers.NewSeriesHandler(seriesService)
	mediaHandler := handlers.NewMediaHandler(cfg, movieService, seriesService, userRepo)
	homepageHandler := handlers.NewHomepageHandler(movieService, collectionService, seriesService)
	if telegramService != nil {
		seriesHandler.SetTelegramService(telegramService)
	}

	// Clip repository and handler
	clipRepo := repositories.NewClipRepository(db)
	clipHandler := handlers.NewClipHandler(clipRepo, parserURL)

	// B2 cleanup service — nil in DEV when credentials are not set; DeleteMovie
	// then falls through to DB-only removal without aborting.
	b2Cleanup := services.NewB2CleanupService(services.B2CleanupConfig{
		KeyID:      cfg.B2KeyID,
		AppKey:     cfg.B2AppKey,
		BucketName: cfg.B2Bucket,
		CDNURL:     cfg.CDNURL,
	})

	// Instagram schedule repository and handler.
	// Initialized before storage wiring so DeleteMovie / DeleteSeries can
	// cascade into clip-linked schedules and publish jobs.
	igScheduleRepo := repositories.NewInstagramScheduleRepository(db)
	if err := igScheduleRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure instagram_schedules indexes: %v", err)
	}
	igScheduleHandler := handlers.NewInstagramScheduleHandler(igScheduleRepo, clipRepo)

	// Multi-platform publish job repository and handler
	publishJobRepo := repositories.NewPublishJobRepository(db)
	if err := publishJobRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure publish_jobs indexes: %v", err)
	}
	publishJobHandler := handlers.NewPublishJobHandler(publishJobRepo, clipRepo, parserURL)

	movieService.SetStorageDependencies(clipRepo, igScheduleRepo, publishJobRepo, b2Cleanup)
	seriesService.SetStorageDependencies(clipRepo, igScheduleRepo, publishJobRepo, b2Cleanup)
	movieService.SetJobRepository(jobRepo)
	seriesService.SetJobRepository(jobRepo)

	// Suggestion repository, service, and handler
	suggestionRepo := repositories.NewSuggestionRepository(db)
	suggestionService := services.NewSuggestionService(suggestionRepo, userRepo, notificationService)
	suggestionHandler := handlers.NewSuggestionHandlerWithConfig(suggestionService, cfg)

	// Premium (Telegram Stars) repository + handler
	premiumPaymentRepo := repositories.NewPremiumPaymentRepository(db)
	if err := premiumPaymentRepo.EnsureIndexes(); err != nil {
		log.Printf("Warning: Failed to ensure telegram_stars_payments indexes: %v", err)
	}
	premiumHandler := handlers.NewPremiumHandler(userRepo, premiumPaymentRepo)

	// Serve uploaded files in dev mode
	if cfg.IsDev {
		r.Static("/uploads", "./uploads")
		// Serve worker uploads directory for local HLS playback under /stream prefix
		// This avoids conflict with /uploads which is used for website assets
		if cfg.WorkerUploadsDir != "" {
			log.Printf("[DEV] Serving worker uploads from: %s at /stream", cfg.WorkerUploadsDir)
			r.Static("/stream", cfg.WorkerUploadsDir)
		}
	}

	// Register routes
	routes.Setup(r, authHandler, movieHandler, homepageHandler, ingestionHandler, uploadHandler, adminUserHandler, userHandler, collectionHandler, authService, ratingHandler, commentHandler, shareHandler, seriesHandler, mediaHandler, banAppealHandler, notificationHandler, telegramHandler, clipHandler, adHandler, telegramPostHandler, igScheduleHandler, publishJobHandler, suggestionHandler, premiumHandler)

	// Start premium cleanup background job (runs every 10 minutes)
	go startPremiumCleanupJob(userRepo, notificationService)

	// Start Instagram schedule executor (runs every 60 seconds)
	go startInstagramScheduler(igScheduleRepo, clipRepo, parserURL)

	// Start multi-platform publish job scheduler (runs every 60 seconds)
	go startPublishJobScheduler(publishJobRepo, clipRepo, parserURL)

	log.Printf("FilmoraUz API listening on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func connectMongo(uri, dbName string) *mongo.Database {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		log.Fatalf("MongoDB connection failed: %v", err)
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Fatalf("MongoDB ping failed: %v\nCheck that MongoDB is running and MONGO_URI is correct", err)
	}

	log.Printf("Connected to MongoDB at %s", dbName)
	return client.Database(dbName)
}

func seedAdmin(repo *repositories.UserRepository, adminTelegramID int64) {
	if adminTelegramID == 0 {
		log.Println("No ADMIN_TELEGRAM_ID configured, skipping admin seeding")
		return
	}

	// Check if admin user already exists
	existing, err := repo.FindByTelegramID(adminTelegramID)
	if err == nil && existing != nil {
		// Admin already exists
		return
	}
	if err != nil {
		log.Printf("Failed to check admin existence: %v", err)
		return
	}

	// Create admin user with Telegram ID
	_, err = repo.Create(
		adminTelegramID,
		"Admin",
		"",      // lastName
		"admin", // username
		"",      // photoURL
		"",      // languageCode
	)
	if err != nil {
		log.Printf("Failed to seed admin: %v", err)
		return
	}

	log.Printf("Admin user created with Telegram ID: %d", adminTelegramID)
}

// getParserURL returns the parser service URL from environment variables
// Supports both PARSER_SERVICE_URL (preferred) and PARSER_URL for compatibility
func getParserURL() string {
	// First check PARSER_SERVICE_URL (task requirement)
	if url := os.Getenv("PARSER_SERVICE_URL"); url != "" {
		log.Printf("Using PARSER_SERVICE_URL: %s", url)
		return url
	}
	// Fall back to PARSER_URL for backward compatibility
	if url := os.Getenv("PARSER_URL"); url != "" {
		log.Printf("Using PARSER_URL: %s", url)
		return url
	}
	// Default to localhost:8082 for local development
	log.Printf("Using default parser URL: http://127.0.0.1:8082")
	return "http://127.0.0.1:8082"
}

// Make sure models are used
var _ = models.IngestionStatusPending

// startInstagramScheduler runs a background goroutine that executes due scheduled uploads.
// It polls every 60 seconds. Atomically claims one job at a time to prevent double-execution.
func startInstagramScheduler(scheduleRepo *repositories.InstagramScheduleRepository, clipRepo *repositories.ClipRepository, parserURL string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	// Run once immediately so uploads scheduled "now" don't wait 60s
	runDueInstagramSchedules(scheduleRepo, clipRepo, parserURL)
	for range ticker.C {
		runDueInstagramSchedules(scheduleRepo, clipRepo, parserURL)
	}
}

func runDueInstagramSchedules(scheduleRepo *repositories.InstagramScheduleRepository, clipRepo *repositories.ClipRepository, parserURL string) {
	for {
		schedule, err := scheduleRepo.ClaimDue()
		if err != nil {
			log.Printf("[IGScheduler] claim error: %v", err)
			return
		}
		if schedule == nil {
			return // nothing due right now
		}
		log.Printf("[IGScheduler] executing schedule=%s clip=%s accounts=%v",
			schedule.ID.Hex(), schedule.ClipID.Hex(), schedule.AccountNames)
		go executeInstagramSchedule(scheduleRepo, clipRepo, schedule, parserURL)
	}
}

func executeInstagramSchedule(
	scheduleRepo *repositories.InstagramScheduleRepository,
	clipRepo *repositories.ClipRepository,
	schedule *models.InstagramSchedule,
	parserURL string,
) {
	caption := fmt.Sprintf("%s\n\nKinoni profildagi bot orqali toping!\nKino Kodi: %s", schedule.MovieTitle, schedule.MovieCode)
	var uploadErrors []string

	for _, accountName := range schedule.AccountNames {
		account := services.GetInstagramAccount(accountName)
		if account == nil {
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: account not configured", accountName))
			continue
		}
		if err := services.UploadReelToInstagram(parserURL, schedule.ClipURL, caption, account); err != nil {
			uploadErrors = append(uploadErrors, fmt.Sprintf("%s: %v", accountName, err))
		}
	}

	ctx := context.Background()
	if len(uploadErrors) > 0 {
		errMsg := strings.Join(uploadErrors, "; ")
		if err := scheduleRepo.MarkFailed(schedule.ID, errMsg); err != nil {
			log.Printf("[IGScheduler] MarkFailed error schedule=%s: %v", schedule.ID.Hex(), err)
		}
		// Persist upload failure on the clip document so the clips list reflects it
		if err := clipRepo.RecordInstagramUpload(ctx, schedule.ClipID, "failed"); err != nil {
			log.Printf("[IGScheduler] RecordInstagramUpload(failed) error clip=%s: %v", schedule.ClipID.Hex(), err)
		}
		log.Printf("[IGScheduler] schedule=%s failed: %s", schedule.ID.Hex(), errMsg)
	} else {
		if err := scheduleRepo.MarkSuccess(schedule.ID); err != nil {
			log.Printf("[IGScheduler] MarkSuccess error schedule=%s: %v", schedule.ID.Hex(), err)
		}
		// Persist upload success on the clip document so the clips list reflects it after refresh
		if err := clipRepo.RecordInstagramUpload(ctx, schedule.ClipID, "success"); err != nil {
			log.Printf("[IGScheduler] RecordInstagramUpload(success) error clip=%s: %v", schedule.ClipID.Hex(), err)
		}
		log.Printf("[IGScheduler] schedule=%s success", schedule.ID.Hex())
	}
}

// startPublishJobScheduler runs a background goroutine that executes due multi-platform publish jobs.
// It polls every 60 seconds. Atomically claims one job at a time to prevent double-execution.
func startPublishJobScheduler(jobRepo *repositories.PublishJobRepository, clipRepo *repositories.ClipRepository, parserURL string) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	runDuePublishJobs(jobRepo, clipRepo, parserURL)
	for range ticker.C {
		runDuePublishJobs(jobRepo, clipRepo, parserURL)
	}
}

func runDuePublishJobs(jobRepo *repositories.PublishJobRepository, clipRepo *repositories.ClipRepository, parserURL string) {
	for {
		job, err := jobRepo.ClaimDue()
		if err != nil {
			log.Printf("[PublishScheduler] claim error: %v", err)
			return
		}
		if job == nil {
			return
		}
		log.Printf("[PublishScheduler] executing job=%s platform=%s account=%s clip=%s",
			job.ID.Hex(), job.Platform, job.AccountName, job.ClipID.Hex())
		go executePublishJob(jobRepo, clipRepo, job, parserURL)
	}
}

func executePublishJob(
	jobRepo *repositories.PublishJobRepository,
	clipRepo *repositories.ClipRepository,
	job *models.PublishJob,
	parserURL string,
) {
	uploadErr := services.ExecutePlatformUpload(parserURL, job)

	ctx := context.Background()
	if uploadErr != nil {
		errMsg := fmt.Sprintf("%v", uploadErr)
		if err := jobRepo.MarkFailed(job.ID, errMsg); err != nil {
			log.Printf("[PublishScheduler] MarkFailed error job=%s: %v", job.ID.Hex(), err)
		}
		if job.Platform == models.PublishPlatformInstagram {
			if err := clipRepo.RecordInstagramUpload(ctx, job.ClipID, "failed"); err != nil {
				log.Printf("[PublishScheduler] RecordInstagramUpload(failed) clip=%s: %v", job.ClipID.Hex(), err)
			}
		}
		log.Printf("[PublishScheduler] job=%s failed: %s", job.ID.Hex(), errMsg)
	} else {
		if err := jobRepo.MarkSuccess(job.ID); err != nil {
			log.Printf("[PublishScheduler] MarkSuccess error job=%s: %v", job.ID.Hex(), err)
		}
		if job.Platform == models.PublishPlatformInstagram {
			if err := clipRepo.RecordInstagramUpload(ctx, job.ClipID, "success"); err != nil {
				log.Printf("[PublishScheduler] RecordInstagramUpload(success) clip=%s: %v", job.ClipID.Hex(), err)
			}
		}
		log.Printf("[PublishScheduler] job=%s success", job.ID.Hex())
	}
}

// startPremiumCleanupJob runs a background goroutine that cleans up expired premium subscriptions.
// It runs every 10 minutes.
func startPremiumCleanupJob(userRepo *repositories.UserRepository, notificationService *services.NotificationService) {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()

	// Run once immediately on startup
	cleanupOnce(userRepo, notificationService)

	// Then run periodically
	for range ticker.C {
		cleanupOnce(userRepo, notificationService)
	}
}

func cleanupOnce(userRepo *repositories.UserRepository, notificationService *services.NotificationService) {
	updated, err := userRepo.CleanupExpiredPremium()
	if err != nil {
		log.Printf("[PremiumCleanup] Error cleaning up expired premium: %v", err)
		return
	}
	if updated > 0 {
		log.Printf("[PremiumCleanup] Cleaned up %d expired premium subscriptions", updated)
	}

	// Send expiry notifications for users whose premium is about to expire (within 3 days)
	// This runs every cleanup cycle
	ctx := context.Background()
	usersExpiring, err := userRepo.GetUsersWithExpiringPremium(3) // 3 days
	if err == nil && len(usersExpiring) > 0 {
		for _, user := range usersExpiring {
			if notificationService != nil {
				err := notificationService.NotifyPremiumExpiringSoon(ctx, user.ID, 3)
				if err != nil {
					log.Printf("[PremiumCleanup] Failed to send expiry notification to user %s: %v", user.ID.Hex(), err)
				}
			}
		}
		log.Printf("[PremiumCleanup] Sent %d expiry notifications", len(usersExpiring))
	}

	// Send expired notifications
	usersExpired, err := userRepo.GetUsersWithExpiredPremium()
	if err == nil && len(usersExpired) > 0 {
		for _, user := range usersExpired {
			if notificationService != nil {
				err := notificationService.NotifyPremiumExpired(ctx, user.ID)
				if err != nil {
					log.Printf("[PremiumCleanup] Failed to send expired notification to user %s: %v", user.ID.Hex(), err)
				}
			}
		}
		log.Printf("[PremiumCleanup] Sent %d expired notifications", len(usersExpired))
	}
}
