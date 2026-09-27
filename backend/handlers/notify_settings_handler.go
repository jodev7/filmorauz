package handlers

import (
	"net/http"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

// NotifySettingsHandler: per-category delivery settings and web push
// subscriptions.
type NotifySettingsHandler struct {
	notifications *services.NotificationService
	users         *repositories.UserRepository
}

func NewNotifySettingsHandler(n *services.NotificationService, users *repositories.UserRepository) *NotifySettingsHandler {
	return &NotifySettingsHandler{notifications: n, users: users}
}

// Get GET /api/user/notification-settings
func (h *NotifySettingsHandler) Get(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	repo := h.notifications.PrefsRepo()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not available"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	prefs, _ := repo.Get(ctx, userID)
	telegram := false
	if u, err := h.users.FindByID(userID.Hex()); err == nil && u != nil {
		telegram = u.TelegramChatID != 0 || u.TelegramID != 0
	}
	c.JSON(http.StatusOK, gin.H{
		"prefs":              prefs,
		"categories":         models.NotificationCategories,
		"push_public_key":    h.notifications.Pusher().PublicKey(),
		"push_devices":       repo.CountSubscriptions(ctx, userID),
		"telegram_connected": telegram,
	})
}

// Update PUT /api/user/notification-settings {prefs}
func (h *NotifySettingsHandler) Update(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	repo := h.notifications.PrefsRepo()
	if repo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "not available"})
		return
	}
	var req struct {
		Prefs repositories.NotifyPrefs `json:"prefs"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Prefs == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	saved, err := repo.Set(ctx, userID, req.Prefs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"prefs": saved})
}

type pushSubBody struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
}

// Subscribe POST /api/user/push-subscriptions {endpoint, keys:{p256dh, auth}}
func (h *NotifySettingsHandler) Subscribe(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	repo := h.notifications.PrefsRepo()
	if repo == nil || h.notifications.Pusher() == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Push bildirishnomalar serverda sozlanmagan"})
		return
	}
	var b pushSubBody
	if err := c.ShouldBindJSON(&b); err != nil || !strings.HasPrefix(b.Endpoint, "https://") || len(b.Endpoint) > 1000 ||
		b.Keys.P256dh == "" || b.Keys.Auth == "" || len(b.Keys.P256dh) > 200 || len(b.Keys.Auth) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid subscription"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := repo.AddSubscription(ctx, userID, b.Endpoint, b.Keys.P256dh, b.Keys.Auth, c.Request.UserAgent()); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Unsubscribe DELETE /api/user/push-subscriptions {endpoint}
func (h *NotifySettingsHandler) Unsubscribe(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	repo := h.notifications.PrefsRepo()
	var b struct {
		Endpoint string `json:"endpoint"`
	}
	if repo == nil || c.ShouldBindJSON(&b) != nil || b.Endpoint == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	_ = repo.RemoveSubscription(ctx, userID, b.Endpoint)
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Test POST /api/user/push-subscriptions/test
func (h *NotifySettingsHandler) Test(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	n := h.notifications.PushToUser(c.Request.Context(), userID, services.PushPayload{
		Title: "FilmoraUz",
		Body:  "Push bildirishnomalar ishlayapti ✅",
		URL:   "/notifications",
		Tag:   "test",
	})
	if n == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Hech qaysi qurilmaga yetib bormadi"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"sent": n})
}
