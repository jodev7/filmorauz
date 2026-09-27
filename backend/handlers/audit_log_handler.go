package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
)

type AuditLogHandler struct {
	repo *repositories.AuditLogRepository
}

func NewAuditLogHandler(repo *repositories.AuditLogRepository) *AuditLogHandler {
	return &AuditLogHandler{repo: repo}
}

// List GET /api/superadmin/audit-logs?page=&limit=&actor_id=&method=&q=&failed=1
func (h *AuditLogHandler) List(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	items, total, err := h.repo.List(ctx, repositories.AuditLogFilter{
		ActorID: c.Query("actor_id"),
		Method:  c.Query("method"),
		Query:   c.Query("q"),
		Failed:  c.Query("failed") == "1" || c.Query("failed") == "true",
	}, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load audit logs"})
		return
	}
	if limit < 1 || limit > 100 {
		limit = 50
	}
	totalPages := int((total + int64(limit) - 1) / int64(limit))
	c.JSON(http.StatusOK, gin.H{
		"data":        items,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}
