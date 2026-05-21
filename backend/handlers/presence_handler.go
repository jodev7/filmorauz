package handlers

import (
	"net/http"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PresenceHandler exposes endpoints for tracking online users and reading
// aggregated activity stats (online now + DAU/WAU/MAU).
type PresenceHandler struct {
	presence *services.PresenceService
	userRepo *repositories.UserRepository
}

func NewPresenceHandler(presence *services.PresenceService, userRepo *repositories.UserRepository) *PresenceHandler {
	return &PresenceHandler{presence: presence, userRepo: userRepo}
}

type heartbeatRequest struct {
	SessionID string `json:"session_id"`
}

// Heartbeat is called periodically by every browser tab (authed or anonymous).
// Auth is optional — if a valid JWT is present the session is counted as
// authenticated and last_active_at is bumped on the user document.
func (h *PresenceHandler) Heartbeat(c *gin.Context) {
	var req heartbeatRequest
	_ = c.ShouldBindJSON(&req)
	if req.SessionID == "" {
		req.SessionID = c.GetHeader("X-Session-Id")
	}
	if req.SessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id required"})
		return
	}

	userID := ""
	if v, ok := c.Get("user_id"); ok {
		if s, ok := v.(string); ok {
			userID = s
		}
	}
	h.presence.Touch(req.SessionID, userID)

	// Bump last_active_at for authed users (fire-and-forget; ignore errors).
	if userID != "" {
		if oid, err := primitive.ObjectIDFromHex(userID); err == nil {
			_ = h.userRepo.UpdateLastActive(oid)
		}
	}

	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// OnlineStats returns the admin dashboard activity block.
func (h *PresenceHandler) OnlineStats(c *gin.Context) {
	authed, anon, total := h.presence.OnlineCounts()

	now := time.Now()
	dau, _ := h.userRepo.CountActiveSince(now.Add(-24 * time.Hour))
	wau, _ := h.userRepo.CountActiveSince(now.Add(-7 * 24 * time.Hour))
	mau, _ := h.userRepo.CountActiveSince(now.Add(-30 * 24 * time.Hour))

	c.JSON(http.StatusOK, gin.H{
		"online": gin.H{
			"authenticated": authed,
			"anonymous":     anon,
			"total":         total,
		},
		"active": gin.H{
			"dau": dau,
			"wau": wau,
			"mau": mau,
		},
	})
}
