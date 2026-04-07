package main

import (
	"context"
	"log"
	"os"
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
	movieService := services.NewMovieService(movieRepo, counterRepo, notificationService, movieViewEventRepo)

	// Rating service
	ratingService := services.NewRatingService(ratingRepo, seriesRatingRepo, movieRepo, seriesRepo)

	// Share service
	shareService := services.NewShareService(shareRepo, movieRepo, userRepo, cfg.BaseSiteURL)

	// Seed admin user on first run
	seedAdmin(userRepo, cfg.AdminTelegramID)

	// Backfill movie codes for existing movies that don't have one
	movieService.BackfillMovieCodes()

	// Setup Gin
	r := gin.Default()

	// CORS config
	corsConfig := cors.Config{
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}

	if cfg.IsDev {
		// In dev, explicitly allow localhost:3000 (frontend)
		// Note: Cannot use AllowAllOrigins with AllowCredentials
		corsConfig.AllowOrigins = []string{"http://localhost:3000", "http://127.0.0.1:3000"}
	} else {
		// In prod, only allow your real domain — set ALLOWED_ORIGIN in .env
		if cfg.AllowedOrigin == "" {
			log.Fatal("ALLOWED_ORIGIN is required in PROD mode (e.g. https://filmorauz.uz)")
		}
		corsConfig.AllowOrigins = []string{cfg.AllowedOrigin}
	}

	r.Use(cors.New(corsConfig))

	// Get parser URL from config - supports both PARSER_SERVICE_URL and PARSER_URL
	parserURL := getParserURL()

	// Initialize handlers
	authHandler := handlers.NewAuthHandler(authService)
	movieHandler := handlers.NewMovieHandler(movieService, userRepo)
	ingestionHandler := handlers.NewIngestionHandler(jobRepo, parserURL)
	uploadHandler := handlers.NewUploadHandler(userRepo, cfg)
	adminUserHandler := handlers.NewAdminUserHandler(userRepo, movieRepo, seriesRepo, banHistoryRepo, notificationService)
	userHandler := handlers.NewUserHandler(watchHistoryRepo, favoriteRepo, movieRepo, userRepo)
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

	// Notification handler
	notificationHandler := handlers.NewNotificationHandler(notificationService)

	// Initialize Telegram service for movie notifications
	telegramService, err := services.NewTelegramService(services.TelegramConfig{
		BotToken:        botToken,
		BotUsername:     cfg.TelegramBotUsername,
		ChannelUsername: cfg.TelegramChannelUsername,
		AdminTelegramID: cfg.AdminTelegramID,
		BaseSiteURL:     cfg.BaseSiteURL,
	})
	if err != nil {
		log.Printf("Warning: Failed to initialize Telegram service: %v", err)
	}
	telegramHandler := handlers.NewTelegramHandler(telegramService)

	// Series repository, service, and handler
	// seriesRepo already initialized above for rating service
	seriesService := services.NewSeriesService(seriesRepo)
	seriesHandler := handlers.NewSeriesHandler(seriesService)

	// Clip repository and handler
	clipRepo := repositories.NewClipRepository(db)
	clipHandler := handlers.NewClipHandler(clipRepo, parserURL)

	// Serve uploaded files in dev mode
	if cfg.IsDev {
		r.Static("/uploads", "./uploads")
		// Serve worker uploads directory for local HLS playback under /stream prefix
		// This avoids conflict with /uploads which is used for website assets
		if cfg.WorkerUploadsDir != "" {
			log.Printf("[DEV] Serving worker uploads from: %s at /stream", cfg.WorkerUploadsDir)
			r.Static("/stream", cfg.WorkerUploadsDir+"/movies")
		}
	}

	// Register routes
	routes.Setup(r, authHandler, movieHandler, ingestionHandler, uploadHandler, adminUserHandler, userHandler, collectionHandler, authService, ratingHandler, commentHandler, shareHandler, seriesHandler, banAppealHandler, notificationHandler, telegramHandler, clipHandler)

	// Start premium cleanup background job (runs every 10 minutes)
	go startPremiumCleanupJob(userRepo, notificationService)

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
	_, err := repo.FindByTelegramID(adminTelegramID)
	if err == nil {
		// Admin already exists
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
