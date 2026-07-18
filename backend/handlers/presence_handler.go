package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
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
	h.presence.Touch(req.SessionID, userID, c.ClientIP(), c.GetHeader("User-Agent"))

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

// OnlineSessionsList returns a paginated list of every live session (authed +
// anonymous) with its IP, device and — for logged-in users — a clickable name.
// Powers the "Jonli faollik" detailed list on the admin dashboard.
func (h *PresenceHandler) OnlineSessionsList(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if limit < 1 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	all := h.presence.OnlineSessions()
	total := len(all)

	start := (page - 1) * limit
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	pageItems := all[start:end]

	// Batch-fetch the authed users on this page for names + avatars.
	userMap := map[string]*models.User{}
	oids := make([]primitive.ObjectID, 0, len(pageItems))
	for _, s := range pageItems {
		if s.UserID == "" {
			continue
		}
		if oid, err := primitive.ObjectIDFromHex(s.UserID); err == nil {
			oids = append(oids, oid)
		}
	}
	if len(oids) > 0 {
		if users, err := h.userRepo.FindByIDs(oids); err == nil {
			for i := range users {
				userMap[users[i].ID.Hex()] = &users[i]
			}
		}
	}

	sessions := make([]gin.H, 0, len(pageItems))
	for _, s := range pageItems {
		item := gin.H{
			"session_id": s.SessionID,
			"ip":         s.IP,
			"device":     parseDevice(s.UserAgent),
			"user_agent": s.UserAgent,
			"first_seen": s.FirstSeen,
			"last_seen":  s.LastSeen,
			"type":       "anonymous",
		}
		if s.UserID != "" {
			item["type"] = "authenticated"
			item["user_id"] = s.UserID
			if u := userMap[s.UserID]; u != nil {
				item["name"] = resolveDisplayName(u)
				item["avatar_url"] = resolveAvatarURL(u)
				item["role"] = u.Role
			} else {
				item["name"] = "Foydalanuvchi"
			}
		}
		sessions = append(sessions, item)
	}

	totalPages := (total + limit - 1) / limit

	c.JSON(http.StatusOK, gin.H{
		"sessions":    sessions,
		"page":        page,
		"limit":       limit,
		"total":       total,
		"total_pages": totalPages,
	})
}

// parseDevice turns a raw User-Agent into a short "Browser · OS" label for the
// admin list. It uses simple substring heuristics — good enough for a glanceable
// device column without pulling in a UA-parsing dependency.
func parseDevice(ua string) string {
	if strings.TrimSpace(ua) == "" {
		return "Noma'lum"
	}

	browser := ""
	switch {
	case strings.Contains(ua, "Edg/") || strings.Contains(ua, "Edge"):
		browser = "Edge"
	case strings.Contains(ua, "OPR/") || strings.Contains(ua, "Opera"):
		browser = "Opera"
	case strings.Contains(ua, "SamsungBrowser"):
		browser = "Samsung Internet"
	case strings.Contains(ua, "YaBrowser"):
		browser = "Yandex"
	case strings.Contains(ua, "Firefox"):
		browser = "Firefox"
	case strings.Contains(ua, "Chrome") || strings.Contains(ua, "CriOS"):
		browser = "Chrome"
	case strings.Contains(ua, "Safari"):
		browser = "Safari"
	}

	os := ""
	switch {
	case strings.Contains(ua, "Android"):
		os = "Android"
	case strings.Contains(ua, "iPhone"):
		os = "iPhone"
	case strings.Contains(ua, "iPad"):
		os = "iPad"
	case strings.Contains(ua, "Windows"):
		os = "Windows"
	case strings.Contains(ua, "Mac OS X") || strings.Contains(ua, "Macintosh"):
		os = "macOS"
	case strings.Contains(ua, "Linux"):
		os = "Linux"
	}

	switch {
	case browser != "" && os != "":
		return browser + " · " + os
	case browser != "":
		return browser
	case os != "":
		return os
	default:
		return "Noma'lum"
	}
}
