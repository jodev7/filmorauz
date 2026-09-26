package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// adminMovieQueryFromRequest reads the admin list filters from the query
// string. Unknown values fall back to "no constraint".
func adminMovieQueryFromRequest(c *gin.Context, defaultLimit int) repositories.AdminMovieQuery {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(defaultLimit)))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 500 {
		limit = defaultLimit
	}
	pick := func(v string, allowed ...string) string {
		for _, a := range allowed {
			if v == a {
				return v
			}
		}
		return ""
	}
	q := strings.TrimSpace(c.Query("q"))
	if len(q) > 100 {
		q = q[:100]
	}
	return repositories.AdminMovieQuery{
		Page:    page,
		Limit:   limit,
		Status:  pick(c.Query("status"), "pending", "approved", "rejected", "scheduled"),
		Search:  q,
		Premium: pick(c.Query("premium"), "premium", "free"),
		Media:   pick(c.Query("media"), "missing"),
		Sort:    pick(c.Query("sort"), "newest", "oldest", "title", "views", "rating", "schedule"),
	}
}

// AdminGetMovie GET /api/admin/movies/:id — any approval status.
func (h *MovieHandler) AdminGetMovie(c *gin.Context) {
	if _, err := primitive.ObjectIDFromHex(c.Param("id")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	movie, err := h.movieService.GetMovieByID(c.Param("id"))
	if errors.Is(err, mongo.ErrNoDocuments) || (err == nil && movie == nil) {
		c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch movie"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": movie})
}

// ScheduleMovie POST /api/admin/movies/:id/schedule {publish_at: RFC3339}
func (h *MovieHandler) ScheduleMovie(c *gin.Context) {
	var req struct {
		PublishAt string `json:"publish_at"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "publish_at kerak"})
		return
	}
	at, err := time.Parse(time.RFC3339, strings.TrimSpace(req.PublishAt))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "publish_at noto'g'ri formatda"})
		return
	}
	now := time.Now()
	if at.Before(now.Add(time.Minute)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vaqt kamida 1 daqiqa keyin bo'lishi kerak"})
		return
	}
	if at.After(now.AddDate(1, 0, 0)) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Vaqt 1 yildan uzoq bo'lmasin"})
		return
	}
	if err := h.movieService.ScheduleMovie(c.Param("id"), at, c.GetString("user_id")); err != nil {
		switch {
		case errors.Is(err, repositories.ErrMovieAlreadyPublished):
			c.JSON(http.StatusConflict, gin.H{"error": "Kino allaqachon e'lon qilingan"})
		case err.Error() == "movie not found":
			c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		case err.Error() == "invalid movie id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to schedule"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "scheduled_publish_at": at.UTC()})
}

// CancelMovieSchedule DELETE /api/admin/movies/:id/schedule
func (h *MovieHandler) CancelMovieSchedule(c *gin.Context) {
	if err := h.movieService.CancelMovieSchedule(c.Param("id")); err != nil {
		switch err.Error() {
		case "movie not found":
			c.JSON(http.StatusNotFound, gin.H{"error": "movie not found"})
		case "invalid movie id":
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel schedule"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
