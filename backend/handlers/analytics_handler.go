package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AnalyticsHandler struct {
	repo *repositories.AnalyticsRepository
}

func NewAnalyticsHandler(repo *repositories.AnalyticsRepository) *AnalyticsHandler {
	return &AnalyticsHandler{repo: repo}
}

func analyticsDays(c *gin.Context, fallback int) int {
	days, _ := strconv.Atoi(c.DefaultQuery("days", strconv.Itoa(fallback)))
	if days <= 0 {
		return fallback
	}
	if days > 365 {
		return 365
	}
	return days
}

func analyticsLimit(c *gin.Context, fallback int) int {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", strconv.Itoa(fallback)))
	if limit <= 0 {
		return fallback
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func userObjectIDFromContext(c *gin.Context) *primitive.ObjectID {
	userID := c.GetString("user_id")
	if userID == "" {
		return nil
	}
	oid, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		return nil
	}
	return &oid
}

func (h *AnalyticsHandler) AdminSearchSummary(c *gin.Context) {
	top, zero, err := h.repo.SearchSummary(c.Request.Context(), analyticsDays(c, 30), analyticsLimit(c, 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get search analytics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"top": top, "zero_results": zero})
}

func (h *AnalyticsHandler) AdminTopContentByPeriod(c *gin.Context) {
	data, err := h.repo.TopContentByPeriod(c.Request.Context(), analyticsDays(c, 7), analyticsLimit(c, 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get trend analytics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *AnalyticsHandler) AdminCompletionSummary(c *gin.Context) {
	data, err := h.repo.CompletionSummary(c.Request.Context(), analyticsLimit(c, 10))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get completion analytics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": data})
}

func (h *AnalyticsHandler) AdminPremiumFunnel(c *gin.Context) {
	data, err := h.repo.PremiumFunnel(c.Request.Context(), analyticsDays(c, 30))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get premium funnel"})
		return
	}
	c.JSON(http.StatusOK, data)
}

func (h *AnalyticsHandler) AdminPlaybackReports(c *gin.Context) {
	reports, err := h.repo.ListPlaybackReports(c.Request.Context(), c.DefaultQuery("status", "new"), analyticsLimit(c, 20))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get playback reports"})
		return
	}
	counts, err := h.repo.PlaybackReportCounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get playback report counts"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reports": reports, "counts": counts})
}

func (h *AnalyticsHandler) CreatePlaybackReport(c *gin.Context) {
	var req struct {
		TargetType string `json:"target_type" binding:"required"`
		TargetID   string `json:"target_id" binding:"required"`
		Title      string `json:"title"`
		URL        string `json:"url"`
		Reason     string `json:"reason" binding:"required"`
		Note       string `json:"note"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report"})
		return
	}
	targetType := strings.ToLower(strings.TrimSpace(req.TargetType))
	if targetType != "movie" && targetType != "episode" && targetType != "series" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_type"})
		return
	}
	targetID, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.TargetID))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target_id"})
		return
	}
	note := strings.TrimSpace(req.Note)
	if len(note) > 500 {
		note = note[:500]
	}
	title := strings.TrimSpace(req.Title)
	if len(title) > 200 {
		title = title[:200]
	}
	url := strings.TrimSpace(req.URL)
	if len(url) > 300 {
		url = url[:300]
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 80 {
		reason = reason[:80]
	}
	if reason == "" {
		reason = "playback_issue"
	}
	now := time.Now()
	report := models.PlaybackReport{
		TargetType: targetType,
		TargetID:   targetID,
		UserID:     userObjectIDFromContext(c),
		Title:      title,
		URL:        url,
		Reason:     reason,
		Note:       note,
		Status:     "new",
		IP:         c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if err := h.repo.CreatePlaybackReport(c.Request.Context(), report); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create report"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *AnalyticsHandler) RecordPremiumEvent(c *gin.Context) {
	userID := userObjectIDFromContext(c)
	if userID == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	var req struct {
		EventType  string `json:"event_type" binding:"required"`
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
		Package    string `json:"package"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event"})
		return
	}
	eventType := strings.ToLower(strings.TrimSpace(req.EventType))
	if eventType != "lock_view" && eventType != "cta_click" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid event_type"})
		return
	}
	var targetID *primitive.ObjectID
	if strings.TrimSpace(req.TargetID) != "" {
		if oid, err := primitive.ObjectIDFromHex(strings.TrimSpace(req.TargetID)); err == nil {
			targetID = &oid
		}
	}
	event := models.PremiumFunnelEvent{
		EventType:  eventType,
		TargetType: strings.ToLower(strings.TrimSpace(req.TargetType)),
		TargetID:   targetID,
		UserID:     *userID,
		Package:    strings.TrimSpace(req.Package),
		IP:         c.ClientIP(),
		UserAgent:  c.GetHeader("User-Agent"),
		CreatedAt:  time.Now(),
	}
	if err := h.repo.RecordPremiumFunnelEvent(c.Request.Context(), event); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record event"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
