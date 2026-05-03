package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type InstagramScheduleHandler struct {
	scheduleRepo *repositories.InstagramScheduleRepository
	clipRepo     *repositories.ClipRepository
	seriesRepo   *repositories.SeriesRepository
}

func NewInstagramScheduleHandler(
	scheduleRepo *repositories.InstagramScheduleRepository,
	clipRepo *repositories.ClipRepository,
	seriesRepo *repositories.SeriesRepository,
) *InstagramScheduleHandler {
	return &InstagramScheduleHandler{scheduleRepo: scheduleRepo, clipRepo: clipRepo, seriesRepo: seriesRepo}
}

// POST /api/admin/clips/:id/instagram/schedule
// Body: {"account_names": ["main"], "scheduled_for": "2024-01-15T14:30:00+05:00"}
func (h *InstagramScheduleHandler) Create(c *gin.Context) {
	clipIDStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(clipIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	var req struct {
		AccountNames []string `json:"account_names"`
		ScheduledFor string   `json:"scheduled_for"` // RFC3339 with +05:00 offset
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AccountNames) == 0 || req.ScheduledFor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_names and scheduled_for are required"})
		return
	}

	scheduledFor, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be RFC3339 (e.g. 2024-01-15T14:30:00+05:00)"})
		return
	}
	if scheduledFor.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be in the future"})
		return
	}

	ctx := context.Background()
	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	userID, _ := c.Get("user_id")
	createdBy := ""
	if userID != nil {
		createdBy, _ = userID.(string)
	}

	s := &models.InstagramSchedule{
		ClipID:       clipID,
		ClipURL:      clip.URL,
		MovieTitle:   clip.MovieTitle,
		MovieSlug:    clip.MovieSlug,
		MovieCode:    services.ResolveInstagramClipCode(ctx, clip, h.seriesRepo),
		AccountNames: req.AccountNames,
		ScheduledFor: scheduledFor.UTC(),
		CreatedBy:    createdBy,
	}
	if err := h.scheduleRepo.Create(s); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create schedule"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": s})
}

// GET /api/admin/instagram/schedules
func (h *InstagramScheduleHandler) ListAll(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	schedules, total, err := h.scheduleRepo.ListAll(limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list schedules"})
		return
	}
	if schedules == nil {
		schedules = []models.InstagramSchedule{}
	}
	c.JSON(http.StatusOK, gin.H{"data": schedules, "total": total})
}

// GET /api/admin/clips/:id/instagram/schedules
func (h *InstagramScheduleHandler) ListForClip(c *gin.Context) {
	clipIDStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(clipIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	schedules, err := h.scheduleRepo.FindByClipID(clipID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list schedules"})
		return
	}
	if schedules == nil {
		schedules = []models.InstagramSchedule{}
	}
	c.JSON(http.StatusOK, gin.H{"data": schedules})
}

// PATCH /api/admin/instagram/schedules/:scheduleId
// Body: {"scheduled_for": "2024-01-15T16:00:00+05:00"}
func (h *InstagramScheduleHandler) UpdateTime(c *gin.Context) {
	idStr := c.Param("scheduleId")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}

	var req struct {
		ScheduledFor string `json:"scheduled_for"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ScheduledFor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for is required"})
		return
	}

	newTime, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be RFC3339"})
		return
	}
	if newTime.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be in the future"})
		return
	}

	updated, err := h.scheduleRepo.UpdateScheduledTime(id, newTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update schedule"})
		return
	}
	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "schedule not found or no longer pending"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "schedule updated"})
}

// DELETE /api/admin/instagram/schedules/:scheduleId
func (h *InstagramScheduleHandler) Cancel(c *gin.Context) {
	idStr := c.Param("scheduleId")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid schedule id"})
		return
	}
	cancelled, err := h.scheduleRepo.Cancel(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel schedule"})
		return
	}
	if !cancelled {
		c.JSON(http.StatusNotFound, gin.H{"error": "schedule not found or already executing/completed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "schedule cancelled"})
}
