package handlers

import (
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

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
	genre := c.Query("genre")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	skip := (page - 1) * limit

	seriesList, err := h.seriesService.ListSeries(limit, skip, genre)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list series"})
		return
	}
	log.Printf("[SERIES API] ListSeries genre_filter=%q response_genres=%v", genre, extractSeriesGenres(seriesList))

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
	log.Printf("[SERIES API] GetSeriesBySlug slug=%s db_genres=%v response_genres=%v", slug, series.Series.Genre, series.Series.Genre)

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
		"source_type":    resolveEpisodeSourceType(episode),
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
		log.Printf("[UpdateSeries] id=%s bind error: %v", idStr, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[UpdateSeries] incoming id=%s title=%q poster=%q backdrop=%q year=%d country=%q genres=%v",
		idStr, input.Title, input.PosterURL, input.BackdropURL, input.Year, input.Country, input.Genre)

	series, err := h.seriesService.UpdateSeries(id, &input)
	if err != nil {
		log.Printf("[UpdateSeries] id=%s service error: %v", idStr, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[UpdateSeries] id=%s saved title=%q genres=%v", idStr, series.Title, series.Genre)

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

// PUT /api/admin/seasons/:id - Update season (admin)
func (h *SeriesHandler) UpdateSeason(c *gin.Context) {
	idStr := c.Param("id")
	seasonID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	var input struct {
		Title       string `json:"title"`
		PosterURL   string `json:"poster_url"`
		Description string `json:"description"`
		ReleaseDate string `json:"release_date"`
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
	if input.ReleaseDate != "" {
		releaseDate, _ := time.Parse("2006-01-02", input.ReleaseDate)
		seasonInput.ReleaseDate = releaseDate
	}

	season, err := h.seriesService.UpdateSeason(seasonID, seasonInput)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, season)
}

// DELETE /api/admin/seasons/:id - Delete season (admin)
func (h *SeriesHandler) DeleteSeason(c *gin.Context) {
	idStr := c.Param("id")
	seasonID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	if err := h.seriesService.DeleteSeason(seasonID); err != nil {
		if strings.Contains(err.Error(), "cannot delete season with episodes") {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "season deleted"})
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
		SourceType    string `json:"source_type"`
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
		SourceType:   models.VideoSourceType(input.SourceType),
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

// PUT /api/admin/episodes/:id - Update episode (admin)
func (h *SeriesHandler) UpdateEpisode(c *gin.Context) {
	idStr := c.Param("id")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id"})
		return
	}

	var input struct {
		SeasonID      string `json:"season_id"`
		EpisodeNumber int    `json:"episode_number"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		ThumbnailURL  string `json:"thumbnail_url"`
		VideoURL      string `json:"video_url"`
		EmbedURL      string `json:"embed_url"`
		SourceType    string `json:"source_type"`
		Duration      int    `json:"duration"`
		AirDate       string `json:"air_date"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	episode, err := h.seriesService.GetEpisodeByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "episode not found"})
		return
	}

	if input.Title != "" {
		episode.Title = input.Title
	}
	if input.Description != "" {
		episode.Description = input.Description
	}
	if input.ThumbnailURL != "" {
		episode.ThumbnailURL = input.ThumbnailURL
	}
	if input.VideoURL != "" {
		episode.VideoURL = input.VideoURL
	}
	if input.EmbedURL != "" {
		episode.EmbedURL = input.EmbedURL
	}
	if input.SourceType != "" {
		episode.SourceType = models.VideoSourceType(input.SourceType)
	}
	if input.Duration > 0 {
		episode.Duration = input.Duration
	}
	if input.AirDate != "" {
		airDate, _ := time.Parse("2006-01-02", input.AirDate)
		if !airDate.IsZero() {
			episode.AirDate = airDate
		}
	}
	if input.SeasonID != "" {
		seasonID, err := primitive.ObjectIDFromHex(input.SeasonID)
		if err == nil {
			episode.SeasonID = seasonID
		}
	}
	if input.EpisodeNumber > 0 {
		episode.EpisodeNumber = input.EpisodeNumber
	}

	err = h.seriesService.UpdateEpisode(episode)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update episode"})
		return
	}

	c.JSON(http.StatusOK, episode)
}

func resolveEpisodeSourceType(episode *models.Episode) models.VideoSourceType {
	if episode == nil {
		return models.VideoSourceIframeEmbed
	}
	if episode.SourceType != "" {
		return episode.SourceType
	}
	videoURL := strings.ToLower(episode.VideoURL)
	switch {
	case episode.EmbedURL != "":
		return models.VideoSourceIframeEmbed
	case strings.Contains(videoURL, ".m3u8"), strings.Contains(videoURL, "/master.m3u8"):
		return models.VideoSourceDirectHLS
	case strings.Contains(videoURL, ".mp4"):
		return models.VideoSourceDirectMP4
	case videoURL != "":
		return models.VideoSourceDirectMP4
	default:
		return models.VideoSourceIframeEmbed
	}
}

// POST /api/admin/seasons/:id/episodes/reorder - Reorder episodes in a season (admin)
func (h *SeriesHandler) ReorderEpisodes(c *gin.Context) {
	seasonIDStr := c.Param("id")
	seasonID, err := primitive.ObjectIDFromHex(seasonIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	var input struct {
		EpisodeIDs []string `json:"episode_ids" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body, episode_ids required"})
		return
	}

	episodeIDs := make([]primitive.ObjectID, len(input.EpisodeIDs))
	for i, idStr := range input.EpisodeIDs {
		id, err := primitive.ObjectIDFromHex(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id in list"})
			return
		}
		episodeIDs[i] = id
	}

	err = h.seriesService.ReorderEpisodesInSeason(seasonID, episodeIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reorder episodes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "episodes reordered"})
}

// POST /api/admin/episodes/:id/move - Move episode to another season (admin)
func (h *SeriesHandler) MoveEpisodeToSeason(c *gin.Context) {
	episodeIDStr := c.Param("id")
	episodeID, err := primitive.ObjectIDFromHex(episodeIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id"})
		return
	}

	var input struct {
		SeasonID      string `json:"season_id" binding:"required"`
		EpisodeNumber int    `json:"episode_number" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	seasonID, err := primitive.ObjectIDFromHex(input.SeasonID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	err = h.seriesService.MoveEpisodeToSeason(episodeID, seasonID, input.EpisodeNumber)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to move episode"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "episode moved"})
}

// PUT /api/admin/seasons/:id/episodes - Update all episodes in a season (admin)
func (h *SeriesHandler) UpdateSeasonEpisodes(c *gin.Context) {
	seasonIDStr := c.Param("id")
	seasonID, err := primitive.ObjectIDFromHex(seasonIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid season id"})
		return
	}

	var input struct {
		Episodes []struct {
			ID            string `json:"id" binding:"required"`
			Title         string `json:"title"`
			Description   string `json:"description"`
			ThumbnailURL  string `json:"thumbnail_url"`
			VideoURL      string `json:"video_url"`
			EpisodeNumber int    `json:"episode_number"`
		} `json:"episodes" binding:"required"`
	}

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	episodes := make([]models.Episode, len(input.Episodes))
	for i, ep := range input.Episodes {
		id, err := primitive.ObjectIDFromHex(ep.ID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id in list"})
			return
		}
		episodes[i] = models.Episode{
			ID:            id,
			SeasonID:      seasonID,
			EpisodeNumber: i + 1,
			Title:         ep.Title,
			ThumbnailURL:  ep.ThumbnailURL,
			VideoURL:      ep.VideoURL,
		}
	}

	err = h.seriesService.UpdateSeasonEpisodes(seasonID, episodes)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update episodes"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "season episodes updated"})
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
			log.Printf("[TELEGRAM] series=%s genres from DB: %v (len=%d)", series.Title, series.Genre, len(series.Genre))
			watchURL := h.telegramService.GetBaseSiteURL() + "/series/" + series.Slug
			data := &services.TelegramMovieData{
				Title:       series.Title,
				Year:        series.Year,
				Genres:      series.Genre,
				Country:     series.Country,
				PosterURL:   series.PosterURL,
				Description: series.Description,
				Slug:        series.Slug,
				MovieURL:    watchURL,
			}
			posted := h.telegramService.PostContentApproval(data, true)
			log.Printf("[TELEGRAM APPROVE] series id=%s result: posted_to=%v", id, posted)
		}()
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "approval_status": "approved"})
}

func extractSeriesGenres(seriesList []models.Series) [][]string {
	out := make([][]string, 0, len(seriesList))
	for _, s := range seriesList {
		out = append(out, s.Genre)
	}
	return out
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
