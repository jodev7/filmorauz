package handlers

import (
	"log"
	"net/http"
	"strconv"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type SeriesHandler struct {
	seriesService   *services.SeriesService
	telegramService *services.TelegramService
}

func NewSeriesHandler(seriesService *services.SeriesService) *SeriesHandler {
	return &SeriesHandler{seriesService: seriesService}
}

// SetTelegramService wires the Telegram service after initialization.
func (h *SeriesHandler) SetTelegramService(svc *services.TelegramService) {
	h.telegramService = svc
}

// GET /api/series - List all series
func (h *SeriesHandler) ListSeries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	skip := (page - 1) * limit

	seriesList, err := h.seriesService.ListSeries(limit, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list series"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":  seriesList,
		"page":  page,
		"limit": limit,
	})
}

// GET /api/series/:slug - Get series by slug
func (h *SeriesHandler) GetSeriesBySlug(c *gin.Context) {
	slug := c.Param("slug")

	series, err := h.seriesService.GetSeriesWithSeasons(slug)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}

	// Block unpublished series on public routes.
	// Legacy series (no approval_status) have ApprovalStatus="" and IsPublished=false (struct default),
	// so we only block new content that has been explicitly set to pending/rejected.
	if !series.Series.IsPublished && series.Series.ApprovalStatus != "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}

	// Increment views
	h.seriesService.IncrementSeriesViews(series.Series.ID)

	c.JSON(http.StatusOK, series)
}

// GET /api/series/:id/seasons - Get seasons for a series
func (h *SeriesHandler) GetSeasons(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	seasons, err := h.seriesService.GetSeasonsBySeriesID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get seasons"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": seasons})
}

// GET /api/seasons/:id/episodes - Get episodes for a season
func (h *SeriesHandler) GetEpisodes(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	episodes, err := h.seriesService.GetEpisodesBySeasonID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get episodes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": episodes})
}

// GET /api/episodes/:id - Get episode by ID
func (h *SeriesHandler) GetEpisode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id"})
		return
	}

	episode, err := h.seriesService.GetEpisodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "episode not found"})
		return
	}

	// Increment views
	h.seriesService.IncrementEpisodeViews(id)

	// Get series to include slug
	series, err := h.seriesService.GetSeriesByID(episode.SeriesID)
	seriesSlug := ""
	if err == nil && series != nil {
		seriesSlug = series.Slug
	}

	// Return episode with series slug
	response := gin.H{
		"id":             episode.ID,
		"series_id":      episode.SeriesID,
		"season_id":      episode.SeasonID,
		"episode_number": episode.EpisodeNumber,
		"title":          episode.Title,
		"description":    episode.Description,
		"thumbnail_url":  episode.ThumbnailURL,
		"video_url":      episode.VideoURL,
		"embed_url":      episode.EmbedURL,
		"duration":       episode.Duration,
		"views":          episode.Views,
		"air_date":       episode.AirDate,
		"created_at":     episode.CreatedAt,
		"updated_at":     episode.UpdatedAt,
		"series_slug":    seriesSlug,
	}

	c.JSON(http.StatusOK, response)
}

// POST /api/admin/series - Create series (admin)
func (h *SeriesHandler) CreateSeries(c *gin.Context) {
	var input models.SeriesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	series, err := h.seriesService.CreateSeries(&input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create series"})
		return
	}

	c.JSON(http.StatusCreated, series)
}

// PUT /api/admin/series/:id - Update series (admin)
func (h *SeriesHandler) UpdateSeries(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	var input models.SeriesInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	series, err := h.seriesService.UpdateSeries(id, &input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update series"})
		return
	}

	c.JSON(http.StatusOK, series)
}

// DELETE /api/admin/series/:id - Delete series (admin)
func (h *SeriesHandler) DeleteSeries(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	err = h.seriesService.DeleteSeries(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete series"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "series deleted"})
}

// POST /api/admin/series/:id/seasons - Create season (admin)
func (h *SeriesHandler) CreateSeason(c *gin.Context) {
	idStr := c.Param("id")
	seriesID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	var input struct {
		SeasonNumber int    `json:"season_number" binding:"required"`
		Title        string `json:"title" binding:"required"`
		PosterURL    string `json:"poster_url"`
		Description  string `json:"description"`
		ReleaseDate  string `json:"release_date"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	seasonInput := &models.SeasonInput{
		Title:       input.Title,
		PosterURL:   input.PosterURL,
		Description: input.Description,
	}

	season, err := h.seriesService.CreateSeason(seriesID, input.SeasonNumber, seasonInput)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create season"})
		return
	}

	c.JSON(http.StatusCreated, season)
}

// POST /api/admin/seasons/:id/episodes - Create episode (admin)
func (h *SeriesHandler) CreateEpisode(c *gin.Context) {
	idStr := c.Param("id")
	seasonID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	// Get season to find series ID
	season, err := h.seriesService.GetSeasonsBySeriesID(seasonID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "season not found"})
		return
	}

	var input struct {
		EpisodeNumber int    `json:"episode_number" binding:"required"`
		Title         string `json:"title" binding:"required"`
		Description   string `json:"description"`
		ThumbnailURL  string `json:"thumbnail_url"`
		VideoURL      string `json:"video_url"`
		EmbedURL      string `json:"embed_url"`
		Duration      int    `json:"duration"`
		AirDate       string `json:"air_date"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	episodeInput := &models.EpisodeInput{
		Title:        input.Title,
		Description:  input.Description,
		ThumbnailURL: input.ThumbnailURL,
		VideoURL:     input.VideoURL,
		EmbedURL:     input.EmbedURL,
		Duration:     input.Duration,
		AirDate:      input.AirDate,
	}

	// Find series ID from season
	seriesID := primitive.ObjectID{}
	for _, s := range season {
		if s.ID == seasonID {
			seriesID = s.SeriesID
			break
		}
	}

	episode, err := h.seriesService.CreateEpisode(seriesID, seasonID, input.EpisodeNumber, episodeInput)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create episode"})
		return
	}

	c.JSON(http.StatusCreated, episode)
}

// DELETE /api/admin/episodes/:id - Delete episode (admin)
func (h *SeriesHandler) DeleteEpisode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id"})
		return
	}

	err = h.seriesService.DeleteEpisode(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete episode"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "episode deleted"})
}

// AdminListSeries GET /api/admin/series
// Returns ALL series (pending, approved, rejected) for the admin dashboard.
func (h *SeriesHandler) AdminListSeries(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "200"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = 200
	}
	skip := (page - 1) * limit

	seriesList, err := h.seriesService.ListAllSeriesAdmin(limit, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch series"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": seriesList, "page": page, "limit": limit})
}

// ApproveSeries PATCH /api/admin/series/:id/approve
func (h *SeriesHandler) ApproveSeries(c *gin.Context) {
	id := c.Param("id")
	byUserID := c.GetString("user_id")

	if err := h.seriesService.SetSeriesApprovalStatus(id, "approved", byUserID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "series not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Async Telegram post — non-blocking
	if h.telegramService != nil {
		go func() {
			oid, err := primitive.ObjectIDFromHex(id)
			if err != nil {
				return
			}
			series, err := h.seriesService.GetSeriesByID(oid)
			if err != nil {
				log.Printf("[TELEGRAM APPROVE] could not fetch series %s: %v", id, err)
				return
			}
			watchURL := h.telegramService.GetBaseSiteURL() + "/series/" + series.Slug
			data := &services.TelegramMovieData{
				Title:       series.Title,
				Year:        series.Year,
				Genres:      series.Genre,
				PosterURL:   series.PosterURL,
				Description: series.Description,
				Slug:        series.Slug,
				MovieURL:    watchURL,
			}
			h.telegramService.PostContentApproval(data, true)
		}()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "approval_status": "approved"})
}

// RejectSeries PATCH /api/admin/series/:id/reject
func (h *SeriesHandler) RejectSeries(c *gin.Context) {
	id := c.Param("id")
	byUserID := c.GetString("user_id")

	if err := h.seriesService.SetSeriesApprovalStatus(id, "rejected", byUserID); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "series not found" {
			status = http.StatusNotFound
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "approval_status": "rejected"})
}

