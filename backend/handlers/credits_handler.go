package handlers

import (
	"context"
	"errors"
	"log"
	"net/http"
	"time"

	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CreditsHandler: admin actions for TMDB cast/photos.
type CreditsHandler struct {
	svc *services.CreditsService
}

func NewCreditsHandler(svc *services.CreditsService) *CreditsHandler {
	return &CreditsHandler{svc: svc}
}

func (h *CreditsHandler) fetch(c *gin.Context, series bool) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	force := c.Query("force") == "1" || c.Query("force") == "true"
	ctx, cancel := context.WithTimeout(c.Request.Context(), 40*time.Second)
	defer cancel()
	var res *services.CreditsResult
	if series {
		res, err = h.svc.FetchSeries(ctx, id, force)
	} else {
		res, err = h.svc.FetchMovie(ctx, id, force)
	}
	switch {
	// Never 502/503/504 here: Cloudflare swaps those for its own error page
	// (without CORS headers), so the admin would only see "Failed to fetch".
	case errors.Is(err, services.ErrTMDBDisabled):
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
	case errors.Is(err, services.ErrCreditsTargetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "topilmadi"})
	case err != nil:
		log.Printf("[CREDITS] fetch %s: %v", id.Hex(), err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, res)
	}
}

// FetchMovie POST /api/admin/movies/:id/credits?force=1
func (h *CreditsHandler) FetchMovie(c *gin.Context) { h.fetch(c, false) }

// FetchSeries POST /api/admin/series/:id/credits?force=1
func (h *CreditsHandler) FetchSeries(c *gin.Context) { h.fetch(c, true) }

// Backfill POST /api/admin/credits/backfill — starts one batch now.
func (h *CreditsHandler) Backfill(c *gin.Context) {
	if !h.svc.Enabled() {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": services.ErrTMDBDisabled.Error()})
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		n, err := h.svc.Backfill(ctx, 100)
		log.Printf("[CREDITS] manual backfill: %d updated, err=%v", n, err)
	}()
	c.JSON(http.StatusAccepted, gin.H{"started": true})
}
