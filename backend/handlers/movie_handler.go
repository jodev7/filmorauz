package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type MovieHandler struct {
	movieService    *services.MovieService
	userRepo        *repositories.UserRepository
	telegramService *services.TelegramService
}

func NewMovieHandler(movieService *services.MovieService, userRepo *repositories.UserRepository) *MovieHandler {
	return &MovieHandler{movieService: movieService, userRepo: userRepo}
}

// SetTelegramService wires the Telegram service after initialization.
func (h *MovieHandler) SetTelegramService(svc *services.TelegramService) {
	h.telegramService = svc
}

// --- Public Handlers ---

// ListMovies GET /api/movies?genre=Action&page=1&limit=20
func (h *MovieHandler) ListMovies(c *gin.Context) {
	genre := c.Query("genre")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	movies, total, err := h.movieService.ListMovies(genre, page, limit)
	if err != nil {
		log.Printf("[ERROR] ListMovies: failed to fetch movies: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch movies"})
		return
	}

	// Map source_type "ingestion" to valid public type for all movies in list
	mappedCount := 0
	for i := range movies {
		if movies[i].SourceType == "ingestion" {
			originalType := movies[i].SourceType
			if movies[i].VideoURL != "" {
				if strings.HasSuffix(movies[i].VideoURL, ".m3u8") || strings.Contains(movies[i].VideoURL, "manifest") {
					movies[i].SourceType = "direct_hls"
				} else if strings.HasSuffix(movies[i].VideoURL, ".mp4") {
					movies[i].SourceType = "direct_mp4"
				} else if movies[i].EmbedURL != "" {
					movies[i].SourceType = "iframe_embed"
				}
			}
			if movies[i].SourceType == "ingestion" {
				movies[i].SourceType = "direct_hls"
			}
			mappedCount++
			log.Printf("[ListMovies] Mapped movie[%d] source_type: %q -> %q", i, originalType, movies[i].SourceType)
		}
	}
	if mappedCount > 0 {
		log.Printf("[ListMovies] Total mapped: %d/%d movies", mappedCount, len(movies))
	}
	log.Printf("[MOVIE API] ListMovies genre_filter=%q response_genres=%v", genre, extractMovieGenres(movies))

	c.JSON(http.StatusOK, gin.H{
		"data":  movies,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// GetMovieBySlug GET /api/movies/slug/:slug
func (h *MovieHandler) GetMovieBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}

	log.Printf("[GetMovieBySlug] Looking up slug: %s", slug)
	movie, err := h.movieService.GetMovieBySlug(slug)
	if err != nil {
		log.Printf("[GetMovieBySlug] Movie not found for slug: %s, error: %v", slug, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}
	log.Printf("[GetMovieBySlug] Found movie: id=%v, slug=%s, title=%s, source_type=%s, genre=%v", movie.ID, movie.Slug, movie.Title, movie.SourceType, movie.Genre)

	// Block access to unpublished movies on public routes.
	// normalizeMovieFromBSON sets IsPublished=true for legacy documents, so this only
	// hides new content that has explicitly been set to pending/rejected.
	if !movie.IsPublished {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Map source_type "ingestion" to valid public type for frontend compatibility
	// Ingestion movies have video uploaded to CDN - detect type from URL extension
	if movie.SourceType == "ingestion" {
		originalType := movie.SourceType
		if movie.VideoURL != "" {
			if strings.HasSuffix(movie.VideoURL, ".m3u8") || strings.Contains(movie.VideoURL, "manifest") {
				movie.SourceType = "direct_hls"
			} else if strings.HasSuffix(movie.VideoURL, ".mp4") {
				movie.SourceType = "direct_mp4"
			} else if movie.EmbedURL != "" {
				movie.SourceType = "iframe_embed"
			}
		}
		log.Printf("[GetMovieBySlug] Mapped source_type: %q -> %q", originalType, movie.SourceType)
	}

	// Get current user if authenticated (for access check)
	var user *models.User
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		user, _ = h.userRepo.FindByHex(userIDStr)
	}

	// Get access info
	access := models.GetMovieAccessInfo(user, movie)
	log.Printf("[MOVIE API] GetMovieBySlug slug=%s db_genres=%v response_genres=%v", slug, movie.Genre, movie.Genre)

	c.JSON(http.StatusOK, gin.H{
		"data":   movie,
		"access": access,
	})
}

// GetMovieByID GET /api/movies/:id
func (h *MovieHandler) GetMovieByID(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	movie, err := h.movieService.GetMovieByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Block access to unpublished movies on public routes
	if !movie.IsPublished {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Map source_type "ingestion" to valid public type for frontend compatibility
	if movie.SourceType == "ingestion" {
		originalType := movie.SourceType
		if movie.VideoURL != "" {
			if strings.HasSuffix(movie.VideoURL, ".m3u8") || strings.Contains(movie.VideoURL, "manifest") {
				movie.SourceType = "direct_hls"
			} else if strings.HasSuffix(movie.VideoURL, ".mp4") {
				movie.SourceType = "direct_mp4"
			} else if movie.EmbedURL != "" {
				movie.SourceType = "iframe_embed"
			}
		}
		log.Printf("[GetMovieByID] Mapped source_type: %q -> %q", originalType, movie.SourceType)
	}

	// Get current user if authenticated (for access check)
	var user *models.User
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		user, _ = h.userRepo.FindByHex(userIDStr)
	}

	// Get access info
	access := models.GetMovieAccessInfo(user, movie)
	log.Printf("[MOVIE API] GetMovieByID id=%s db_genres=%v response_genres=%v", id, movie.Genre, movie.Genre)

	c.JSON(http.StatusOK, gin.H{
		"data":   movie,
		"access": access,
	})
}

// SearchMovies GET /api/search?q=inception
func (h *MovieHandler) SearchMovies(c *gin.Context) {
	query := c.Query("q")

	movies, err := h.movieService.SearchMovies(query)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": movies})
}

// MovieCodeResponse represents the response for movie code lookup
type MovieCodeResponse struct {
	Found bool       `json:"found"`
	Movie *MovieInfo `json:"movie,omitempty"`
}

// MovieInfo represents movie info in API response (for Telegram bot)
type MovieInfo struct {
	Title       string   `json:"title"`
	Code        string   `json:"code"`
	WebsiteURL  string   `json:"website_url"`
	PosterURL   string   `json:"poster_url"`
	BackdropURL string   `json:"backdrop_url"`
	Year        int      `json:"year"`
	Genre       []string `json:"genre"`
	Quality     string   `json:"quality"`
	Description string   `json:"description"`
	Duration    int      `json:"duration"`
}

// GetMovieByCode GET /api/public/movies/code/:code
func (h *MovieHandler) GetMovieByCode(c *gin.Context) {
	code := c.Param("code")
	if code == "" {
		c.JSON(http.StatusBadRequest, MovieCodeResponse{
			Found: false,
		})
		return
	}

	// Find by alphanumeric code
	movie, err := h.movieService.GetMovieByCode(code)
	if err != nil {
		c.JSON(http.StatusNotFound, MovieCodeResponse{
			Found: false,
		})
		return
	}

	c.JSON(http.StatusOK, MovieCodeResponse{
		Found: true,
		Movie: &MovieInfo{
			Title:       movie.Title,
			Code:        movie.Code,
			WebsiteURL:  movie.WebsiteURL,
			PosterURL:   movie.PosterURL,
			BackdropURL: movie.BackdropURL,
			Year:        movie.Year,
			Genre:       movie.Genre,
			Quality:     movie.Quality,
			Description: movie.Description,
			Duration:    movie.Duration,
		},
	})
}

// --- Admin Handlers ---

// CreateMovie POST /api/admin/movies
func (h *MovieHandler) CreateMovie(c *gin.Context) {
	var input models.MovieInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create flow stays strict: poster is required for new movies.
	if strings.TrimSpace(input.PosterURL) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "poster_url is required"})
		return
	}

	movie, err := h.movieService.CreateMovie(&input)
	if err != nil {
		if services.IsDuplicateMovieError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "movie already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": movie, "message": "Movie created"})
}

// UpdateMovie PUT /api/admin/movies/:id
// Supports partial edits: empty poster_url / backdrop_url / video_url / embed_url
// are treated as "keep existing value", so admins can edit title/year/description
// without re-uploading media.
func (h *MovieHandler) UpdateMovie(c *gin.Context) {
	id := c.Param("id")

	var input models.MovieInput
	if err := c.ShouldBindJSON(&input); err != nil {
		log.Printf("[UpdateMovie] id=%s bind error: %v", id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[UpdateMovie] incoming id=%s title=%q poster=%q backdrop=%q video=%q embed=%q source_type=%q genres=%v",
		id, input.Title, input.PosterURL, input.BackdropURL, input.VideoURL, input.EmbedURL, input.SourceType, input.Genre)

	movie, err := h.movieService.UpdateMovie(id, &input)
	if err != nil {
		log.Printf("[UpdateMovie] id=%s service error: %v", id, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[UpdateMovie] id=%s saved genres=%v (len=%d)", id, movie.Genre, len(movie.Genre))

	log.Printf("[UpdateMovie] id=%s saved poster=%q backdrop=%q video=%q",
		id, movie.PosterURL, movie.BackdropURL, movie.VideoURL)

	c.JSON(http.StatusOK, gin.H{"data": movie, "message": "Movie updated"})
}

// DeleteMovie DELETE /api/admin/movies/:id
func (h *MovieHandler) DeleteMovie(c *gin.Context) {
	id := c.Param("id")

	if err := h.movieService.DeleteMovie(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Movie deleted"})
}

// GetTrendingMovies GET /api/v1/movies/trending?period=24h&limit=12
func (h *MovieHandler) GetTrendingMovies(c *gin.Context) {
	period := c.DefaultQuery("period", "24h")
	if period != "24h" && period != "7d" {
		period = "24h"
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if limit < 1 || limit > 50 {
		limit = 12
	}

	trending, err := h.movieService.GetTrendingMovies(period, limit)
	if err != nil {
		log.Printf("[ERROR] GetTrendingMovies: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get trending movies"})
		return
	}

	// Transform to API response format
	response := make([]gin.H, len(trending))
	for i, t := range trending {
		response[i] = gin.H{
			"id":              t.Movie.ID.Hex(),
			"title":           t.Movie.Title,
			"slug":            t.Movie.Slug,
			"poster_url":      t.Movie.PosterURL,
			"year":            t.Movie.Year,
			"genre":           t.Movie.Genre,
			"views_in_period": t.ViewsInPeriod,
		}
	}

	c.JSON(http.StatusOK, gin.H{"data": response})
}

// GetRecommendations GET /api/v1/movies/:id/recommendations?limit=12
func (h *MovieHandler) GetRecommendations(c *gin.Context) {
	movieID := c.Param("id")
	if movieID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "12"))
	if limit < 1 || limit > 50 {
		limit = 12
	}

	recommendations, err := h.movieService.GetRecommendationsAdvanced(movieID, "", limit)
	if err != nil {
		log.Printf("[ERROR] GetRecommendations: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get recommendations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": recommendations})
}

// GetMovieWatchSource GET /api/v1/movies/:id/watch
// Returns video source only if user has access (premium or non-premium movie)
func (h *MovieHandler) GetMovieWatchSource(c *gin.Context) {
	movieID := c.Param("id")
	if movieID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie id is required"})
		return
	}

	// Get user from context (set by auth middleware)
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Fetch movie
	movie, err := h.movieService.GetMovieByID(movieID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Fetch user to check premium status
	user, err := h.userRepo.FindByHex(userIDStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	// Check access - this is the main security gate
	if !models.CanAccessMovie(user, movie) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":      "premium_required",
			"message":    "Premium subscription required to watch this content",
			"is_premium": movie.IsPremium,
		})
		return
	}

	// User has access - return video source with prefetch support
	// Include HLS streaming data for optimized playback
	response := gin.H{
		"video_url":   movie.VideoURL,
		"embed_url":   movie.EmbedURL,
		"source_type": movie.SourceType,
	}

	// Add HLS streaming data if available
	if movie.SourceType == "direct_hls" && movie.MasterPlaylistURL != "" {
		response["hls"] = gin.H{
			"master_playlist_url": movie.MasterPlaylistURL,
			"generated_qualities": movie.GeneratedQualities,
			"default_quality":     getDefaultQuality(movie.DefaultQuality, movie.GeneratedQualities),
			"source_resolution":   movie.SourceResolution,
		}

		// Preload URLs for fast playback (first 2 segments of default quality)
		// These can be prefetched by the frontend
		defaultQ := getDefaultQuality(movie.DefaultQuality, movie.GeneratedQualities)
		if defaultQ != "" && movie.MasterPlaylistURL != "" {
			preloadURLs := generatePreloadURLs(movie.MasterPlaylistURL, defaultQ, 2)
			response["preload"] = gin.H{
				"master_playlist": movie.MasterPlaylistURL,
				"segments":        preloadURLs,
				"default_quality": defaultQ,
			}
			log.Printf("[PREFETCH] Generated preload URLs for movie %s: default=%s", movie.ID.Hex(), defaultQ)
		}
	}

	c.JSON(http.StatusOK, response)
}

// getDefaultQuality returns the default quality for playback
func getDefaultQuality(custom string, qualities []string) string {
	if custom != "" {
		return custom
	}
	// Default to 480p for fast initial playback
	if len(qualities) > 0 {
		for _, q := range qualities {
			if q == "480p" {
				return "480p"
			}
		}
		// If no 480p, return first available (lowest quality)
		return qualities[len(qualities)-1]
	}
	return "480p"
}

// generatePreloadURLs generates URLs for first N segments of given quality
func generatePreloadURLs(masterURL, quality string, segmentCount int) []string {
	// Extract base path from master playlist URL
	// e.g., https://cdn.example.com/videos/movie-slug/master.m3u8
	// -> https://cdn.example.com/videos/movie-slug/480p/

	// Find last slash position before master.m3u8
	lastSlash := lastIndex(masterURL, "/")
	if lastSlash == -1 {
		return nil
	}

	basePath := masterURL[:lastSlash+1] + quality + "/"

	urls := make([]string, segmentCount)
	for i := 0; i < segmentCount; i++ {
		urls[i] = basePath + fmt.Sprintf("segment_%03d.ts", i+1)
	}

	return urls
}

// lastIndex is a simple string last index helper
func lastIndex(s, substr string) int {
	for i := len(s) - len(substr); i >= 0; i-- {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// AdminListMovies GET /api/admin/movies
// Returns ALL movies (pending, approved, rejected) for the admin dashboard.
func (h *MovieHandler) AdminListMovies(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}

	movies, total, err := h.movieService.ListAllMoviesAdmin(page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch movies"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  movies,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// ApproveMovie PATCH /api/admin/movies/:id/approve
func (h *MovieHandler) ApproveMovie(c *gin.Context) {
	id := c.Param("id")
	byUserID := c.GetString("user_id")

	// Snapshot whether Telegram was already posted before this approve click —
	// a re-approval (status was already "approved") must not re-broadcast.
	prior, priorErr := h.movieService.GetMovieByID(id)
	alreadyPosted := priorErr == nil && prior != nil && prior.TelegramPostedOnApproval

	if err := h.movieService.SetMovieApprovalStatus(id, "approved", byUserID); err != nil {
		if err.Error() == "movie not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
			return
		}
		if services.IsDuplicateMovieError(err) {
			c.JSON(http.StatusConflict, gin.H{"error": "movie already exists"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Async Telegram post — non-blocking
	if h.telegramService != nil && !alreadyPosted {
		go func() {
			log.Printf("[TELEGRAM APPROVE] triggered for movie id=%s by user=%s", id, byUserID)
			movie, err := h.movieService.GetMovieByID(id)
			if err != nil {
				log.Printf("[TELEGRAM APPROVE] could not fetch movie %s: %v", id, err)
				return
			}
			if movie.TelegramPostedOnApproval {
				log.Printf("[TELEGRAM APPROVE] movie id=%s already posted — skipping duplicate", id)
				return
			}
			watchURL := h.telegramService.GetBaseSiteURL() + "/movies/" + movie.Slug
			data := &services.TelegramMovieData{
				Title:       movie.Title,
				Year:        movie.Year,
				Genres:      movie.Genre,
				GenresUz:    movie.GenresUz,
				Country:     movie.Country,
				CountriesUz: movie.CountriesUz,
				Code:        movie.Code,
				PosterURL:   movie.PosterURL,
				Quality:     movie.Quality,
				Description: movie.Description,
				Slug:        movie.Slug,
				MovieURL:    watchURL,
			}
			log.Printf("[TELEGRAM] movie=%s genres from DB: %v (len=%d)", movie.Title, movie.Genre, len(movie.Genre))
			posted := h.telegramService.PostContentApproval(data, false)
			log.Printf("[TELEGRAM APPROVE] movie id=%s result: posted_to=%v", id, posted)
			if len(posted) == 0 {
				log.Printf("[TELEGRAM APPROVE] movie id=%s no channels received the post — not marking as posted", id)
				return
			}
			if err := h.movieService.MarkTelegramPostedOnApproval(id); err != nil {
				log.Printf("[TELEGRAM APPROVE] failed to mark movie id=%s as posted: %v", id, err)
			}
		}()
	} else if alreadyPosted {
		log.Printf("[TELEGRAM APPROVE] movie id=%s already posted on a previous approval — skipping", id)
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "approval_status": "approved"})
}

// RejectMovie PATCH /api/admin/movies/:id/reject
func (h *MovieHandler) RejectMovie(c *gin.Context) {
	id := c.Param("id")
	byUserID := c.GetString("user_id")

	if err := h.movieService.SetMovieApprovalStatus(id, "rejected", byUserID); err != nil {
		if err.Error() == "movie not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "approval_status": "rejected"})
}

func extractMovieGenres(movies []models.Movie) [][]string {
	out := make([][]string, 0, len(movies))
	for _, m := range movies {
		out = append(out, m.Genre)
	}
	return out
}
