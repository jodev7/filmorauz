package handlers

import (
	"log"
	"net/http"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// TelegramHandler handles Telegram notification API requests
type TelegramHandler struct {
	telegramService *services.TelegramService
}

// NewTelegramHandler creates a new Telegram handler
func NewTelegramHandler(telegramService *services.TelegramService) *TelegramHandler {
	return &TelegramHandler{
		telegramService: telegramService,
	}
}

// NotifyMovieRequest is the request body for movie notification
type NotifyMovieRequest struct {
	Title       string   `json:"title" binding:"required"`
	Year        int      `json:"year"`
	Genres      []string `json:"genres"`
	GenresUz    []string `json:"genres_uz"`
	Code        string   `json:"code"`
	PosterURL   string   `json:"poster_url"`
	Quality     string   `json:"quality"`
	Duration    int      `json:"duration"`
	Description string   `json:"description"`
	MovieURL    string   `json:"movie_url"`
	Slug        string   `json:"slug"`
}

// NotifyMovie godoc
// POST /api/telegram/notify-movie
// Sends a movie notification to Telegram channels
func (h *TelegramHandler) NotifyMovie(c *gin.Context) {
	var req NotifyMovieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[TELEGRAM HANDLER] Invalid request: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	log.Printf("[TELEGRAM HANDLER] Received notification request for movie: %s (%d)", req.Title, req.Year)

	// Convert to service type
	movieData := &services.TelegramMovieData{
		Title:       req.Title,
		Year:        req.Year,
		Genres:      req.Genres,
		GenresUz:    req.GenresUz,
		Code:        req.Code,
		PosterURL:   req.PosterURL,
		Quality:     req.Quality,
		Duration:    req.Duration,
		Description: req.Description,
		MovieURL:    req.MovieURL,
		Slug:        req.Slug,
	}

	// Send notification
	result := h.telegramService.NotifyMovieCreated(movieData)

	if result.Success {
		log.Printf("[TELEGRAM HANDLER] Notification sent successfully: channels=%v, bot=%v",
			result.ChannelPosted, result.BotNotified)
		c.JSON(http.StatusOK, result)
	} else {
		// Partial success or failure - still return 200 with result details
		log.Printf("[TELEGRAM HANDLER] Notification completed with issues: channels=%v, failed=%v, error=%s",
			result.ChannelPosted, result.ChannelFailed, result.ErrorMessage)
		c.JSON(http.StatusOK, result)
	}
}

// HealthCheck godoc
// GET /api/telegram/health
// Returns Telegram service health status
func (h *TelegramHandler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "ok",
		"service": "telegram",
	})
}
