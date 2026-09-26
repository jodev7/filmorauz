package handlers

import (
	"errors"
	"net/http"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// HistoryHandler lets users manage their own watch history.
type HistoryHandler struct {
	history *repositories.WatchHistoryRepository
}

func NewHistoryHandler(history *repositories.WatchHistoryRepository) *HistoryHandler {
	return &HistoryHandler{history: history}
}

func (h *HistoryHandler) target(c *gin.Context) (string, primitive.ObjectID, bool) {
	t := c.Param("type")
	if !repositories.ValidHistoryTarget(t) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be movie or episode"})
		return "", primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", primitive.NilObjectID, false
	}
	return t, id, true
}

// HideFromContinue DELETE /api/user/continue-watching/:type/:id
func (h *HistoryHandler) HideFromContinue(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := h.target(c)
	if !ok {
		return
	}
	if err := h.history.HideFromContinue(userID, t, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RestoreToContinue POST /api/user/continue-watching/:type/:id/restore
func (h *HistoryHandler) RestoreToContinue(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := h.target(c)
	if !ok {
		return
	}
	if err := h.history.RestoreToContinue(userID, t, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// MarkWatched POST /api/user/history/:type/:id/watched
func (h *HistoryHandler) MarkWatched(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := h.target(c)
	if !ok {
		return
	}
	if err := h.history.MarkWatched(userID, t, id); err != nil {
		if errors.Is(err, repositories.ErrHistoryTargetNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteEntry DELETE /api/user/history/:type/:id
func (h *HistoryHandler) DeleteEntry(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := h.target(c)
	if !ok {
		return
	}
	n, err := h.history.DeleteEntry(userID, t, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": n})
}

// ClearHistory DELETE /api/user/history
func (h *HistoryHandler) ClearHistory(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	n, err := h.history.ClearHistory(userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "deleted": n})
}

// SeriesProgress GET /api/user/series-progress/:id
func (h *HistoryHandler) SeriesProgress(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	seriesID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	p, err := h.history.GetSeriesProgress(userID, seriesID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed"})
		return
	}
	c.JSON(http.StatusOK, p)
}
