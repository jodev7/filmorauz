package handlers

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ErrorHandler struct {
	repo *repositories.ErrorRepository

	mu      sync.Mutex
	buckets map[string]*ipBucket
}

type ipBucket struct {
	windowStart time.Time
	count       int
}

func NewErrorHandler(repo *repositories.ErrorRepository) *ErrorHandler {
	return &ErrorHandler{repo: repo, buckets: map[string]*ipBucket{}}
}

// allow: at most 20 client error reports per IP per minute.
func (h *ErrorHandler) allow(ip string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	now := time.Now()
	b, ok := h.buckets[ip]
	if !ok || now.Sub(b.windowStart) > time.Minute {
		if len(h.buckets) > 10000 { // crude memory bound
			h.buckets = map[string]*ipBucket{}
		}
		h.buckets[ip] = &ipBucket{windowStart: now, count: 1}
		return true
	}
	b.count++
	return b.count <= 20
}

// ReportClientError POST /api/client-errors — browser error beacon.
func (h *ErrorHandler) ReportClientError(c *gin.Context) {
	if !h.allow(c.ClientIP()) {
		c.Status(http.StatusTooManyRequests)
		return
	}
	var req struct {
		Kind    string `json:"kind"`
		Message string `json:"message"`
		Stack   string `json:"stack"`
		URL     string `json:"url"`
		Release string `json:"release"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || strings.TrimSpace(req.Message) == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	kind := req.Kind
	if kind != "window" && kind != "promise" && kind != "react" {
		kind = "window"
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	_ = h.repo.Record(ctx, repositories.ErrorEvent{
		Source:    "client",
		Kind:      kind,
		Message:   req.Message,
		Stack:     req.Stack,
		URL:       req.URL,
		Release:   req.Release,
		UserID:    c.GetString("user_id"),
		UserAgent: c.Request.UserAgent(),
	})
	c.Status(http.StatusNoContent)
}

// ListErrors GET /api/admin/errors?source=client|server&resolved=1
func (h *ErrorHandler) ListErrors(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	groups, err := h.repo.List(ctx, c.Query("source"), c.Query("resolved") == "1", 150)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load errors"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": groups, "total": len(groups)})
}

// ResolveError POST /api/admin/errors/:id/resolve
func (h *ErrorHandler) ResolveError(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.repo.Resolve(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
