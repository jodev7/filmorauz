package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type MovieHandler struct {
	movieService *services.MovieService
	userRepo     *repositories.UserRepository
}

func NewMovieHandler(movieService *services.MovieService, userRepo *repositories.UserRepository) *MovieHandler {
	return &MovieHandler{movieService: movieService, userRepo: userRepo}
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

	movie, err := h.movieService.GetMovieBySlug(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}

	// Get current user if authenticated (for access check)
	var user *models.User
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		user, _ = h.userRepo.FindByHex(userIDStr)
	}

	// Get access info
	access := models.GetMovieAccessInfo(user, movie)

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

	// Get current user if authenticated (for access check)
	var user *models.User
	userIDStr := c.GetString("user_id")
	if userIDStr != "" {
		user, _ = h.userRepo.FindByHex(userIDStr)
	}

	// Get access info
	access := models.GetMovieAccessInfo(user, movie)

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

	movie, err := h.movieService.CreateMovie(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": movie, "message": "Movie created"})
}

// UpdateMovie PUT /api/admin/movies/:id
func (h *MovieHandler) UpdateMovie(c *gin.Context) {
	id := c.Param("id")

	var input models.MovieInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	movie, err := h.movieService.UpdateMovie(id, &input)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

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
			"genres":          t.Movie.Genre,
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

	recommendations, err := h.movieService.GetRecommendations(movieID, limit)
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

	// User has access - return video source
	c.JSON(http.StatusOK, gin.H{
		"video_url":   movie.VideoURL,
		"embed_url":   movie.EmbedURL,
		"source_type": movie.SourceType,
	})
}
