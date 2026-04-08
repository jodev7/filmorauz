package handlers

import (
	"net/http"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AdHandler struct {
	adRepo   *repositories.AdRepository
	telegram *services.TelegramService
}

func NewAdHandler(adRepo *repositories.AdRepository, telegram *services.TelegramService) *AdHandler {
	return &AdHandler{adRepo: adRepo, telegram: telegram}
}

// -- Admin endpoints --

// AdminListAds GET /api/admin/ads
func (h *AdHandler) AdminListAds(c *gin.Context) {
	// Persist expiry for any ended active ads before returning list
	_ = h.adRepo.ExpireEnded()

	ads, err := h.adRepo.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ads == nil {
		ads = []models.Ad{}
	}
	c.JSON(http.StatusOK, gin.H{"ads": ads})
}

// AdminGetStats GET /api/admin/ads/stats
func (h *AdHandler) AdminGetStats(c *gin.Context) {
	_ = h.adRepo.ExpireEnded()

	stats, err := h.adRepo.GetStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, stats)
}

// AdminCreateAd POST /api/admin/ads
func (h *AdHandler) AdminCreateAd(c *gin.Context) {
	var req struct {
		Title                  string   `json:"title" binding:"required"`
		Description            string   `json:"description"`
		ImageURL               string   `json:"image_url"`
		VideoURL               string   `json:"video_url"`
		TargetURL              string   `json:"target_url" binding:"required"`
		CallToAction           string   `json:"call_to_action"`
		Placements             []string `json:"placements" binding:"required"`
		Status                 string   `json:"status"`
		DurationDays           int      `json:"duration_days"`
		Price                  float64  `json:"price"`
		TelegramChannels       []string `json:"telegram_channels"`
		TelegramBotEnabled     bool     `json:"telegram_bot_enabled"`
		TelegramChannelEnabled bool     `json:"telegram_channel_enabled"`
		PlayerEnabled          bool     `json:"player_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	status := models.AdStatus(req.Status)
	if status == "" {
		status = models.AdStatusDraft
	}

	// Calculate schedule from duration_days
	now := time.Now()
	var startsAt, endsAt *time.Time
	if req.DurationDays > 0 {
		startsAt = &now
		end := now.AddDate(0, 0, req.DurationDays)
		endsAt = &end
	}

	// Get superadmin user ID from context
	var createdBy primitive.ObjectID
	if uid, ok := c.Get("user_id"); ok {
		if uidStr, ok := uid.(string); ok {
			if oid, err := primitive.ObjectIDFromHex(uidStr); err == nil {
				createdBy = oid
			}
		}
	}

	ad := &models.Ad{
		Title:                  req.Title,
		Description:            req.Description,
		ImageURL:               req.ImageURL,
		VideoURL:               req.VideoURL,
		TargetURL:              req.TargetURL,
		CallToAction:           req.CallToAction,
		Placements:             req.Placements,
		Status:                 status,
		StartsAt:               startsAt,
		EndsAt:                 endsAt,
		DurationDays:           req.DurationDays,
		Price:                  req.Price,
		TelegramChannels:       req.TelegramChannels,
		TelegramBotEnabled:     req.TelegramBotEnabled,
		TelegramChannelEnabled: req.TelegramChannelEnabled,
		PlayerEnabled:          req.PlayerEnabled,
		CreatedBy:              createdBy,
	}

	if err := h.adRepo.Create(ad); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, ad)
}

// AdminUpdateAd PUT /api/admin/ads/:id
func (h *AdHandler) AdminUpdateAd(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}

	var req struct {
		Title                  string   `json:"title"`
		Description            string   `json:"description"`
		ImageURL               string   `json:"image_url"`
		VideoURL               string   `json:"video_url"`
		TargetURL              string   `json:"target_url"`
		CallToAction           string   `json:"call_to_action"`
		Placements             []string `json:"placements"`
		Status                 string   `json:"status"`
		DurationDays           int      `json:"duration_days"`
		Price                  float64  `json:"price"`
		TelegramChannels       []string `json:"telegram_channels"`
		TelegramBotEnabled     *bool    `json:"telegram_bot_enabled"`
		TelegramChannelEnabled *bool    `json:"telegram_channel_enabled"`
		PlayerEnabled          *bool    `json:"player_enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	update := bson.M{}
	if req.Title != "" {
		update["title"] = req.Title
	}
	update["description"] = req.Description
	update["image_url"] = req.ImageURL
	update["video_url"] = req.VideoURL
	if req.TargetURL != "" {
		update["target_url"] = req.TargetURL
	}
	update["call_to_action"] = req.CallToAction
	if len(req.Placements) > 0 {
		update["placements"] = req.Placements
	}
	if req.Status != "" {
		update["status"] = req.Status
	}
	update["price"] = req.Price
	if req.TelegramChannels != nil {
		update["telegram_channels"] = req.TelegramChannels
	}
	if req.TelegramBotEnabled != nil {
		update["telegram_bot_enabled"] = *req.TelegramBotEnabled
	}
	if req.TelegramChannelEnabled != nil {
		update["telegram_channel_enabled"] = *req.TelegramChannelEnabled
	}
	if req.PlayerEnabled != nil {
		update["player_enabled"] = *req.PlayerEnabled
	}
	// Recalculate schedule if duration_days provided
	if req.DurationDays > 0 {
		now := time.Now()
		end := now.AddDate(0, 0, req.DurationDays)
		update["starts_at"] = now
		update["ends_at"] = end
		update["duration_days"] = req.DurationDays
	}

	if err := h.adRepo.Update(id, update); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ad, _ := h.adRepo.FindByID(id)
	c.JSON(http.StatusOK, ad)
}

// AdminDeleteAd DELETE /api/admin/ads/:id
func (h *AdHandler) AdminDeleteAd(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}

	if err := h.adRepo.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// -- Public endpoints --

// GetAdsByPlacement GET /api/ads?placement=...
func (h *AdHandler) GetAdsByPlacement(c *gin.Context) {
	placement := c.Query("placement")
	if placement == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "placement query param required"})
		return
	}

	ads, err := h.adRepo.FindByPlacement(placement)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if ads == nil {
		ads = []models.Ad{}
	}
	c.JSON(http.StatusOK, gin.H{"ads": ads})
}

// RecordImpression POST /api/ads/:id/impression
func (h *AdHandler) RecordImpression(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}
	if err := h.adRepo.IncrementImpression(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RecordClick POST /api/ads/:id/click
func (h *AdHandler) RecordClick(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}
	if err := h.adRepo.IncrementClick(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// -- Phase 2 endpoints --

// SendTelegramAd POST /api/superadmin/ads/:id/send-telegram
// Manually triggers posting the ad to its configured Telegram channels/bot
func (h *AdHandler) SendTelegramAd(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}

	ad, err := h.adRepo.FindByID(id)
	if err != nil || ad == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "ad not found"})
		return
	}

	if h.telegram == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "telegram service not configured"})
		return
	}

	var results []map[string]interface{}

	// Post to Telegram channels
	if ad.TelegramChannelEnabled && len(ad.TelegramChannels) > 0 {
		for _, ch := range ad.TelegramChannels {
			target := ch
			if len(target) > 0 && target[0] != '@' {
				target = "@" + target
			}
			res := h.telegram.SendAdToChannel(target, ad.Title, ad.Description, ad.ImageURL, ad.TargetURL, ad.CallToAction)
			delivery := &models.AdDelivery{
				AdID:      id,
				Placement: "telegram_channel_post",
				Target:    ch,
				Status:    res.Status,
				MessageID: res.MessageID,
				SentAt:    time.Now(),
				Error:     res.Error,
			}
			_ = h.adRepo.LogDelivery(delivery)
			results = append(results, map[string]interface{}{
				"target":     ch,
				"placement":  "telegram_channel_post",
				"status":     res.Status,
				"message_id": res.MessageID,
				"error":      res.Error,
			})
		}
	}

	// Post to bot (admin chat as representative bot delivery)
	if ad.TelegramBotEnabled {
		res := h.telegram.SendAdToBot(0, ad.Title, ad.Description, ad.ImageURL, ad.TargetURL, ad.CallToAction)
		// Note: chatID=0 will fail gracefully; real bot delivery uses bot update handlers
		delivery := &models.AdDelivery{
			AdID:      id,
			Placement: "telegram_bot_message",
			Target:    "bot",
			Status:    res.Status,
			MessageID: res.MessageID,
			SentAt:    time.Now(),
			Error:     res.Error,
		}
		_ = h.adRepo.LogDelivery(delivery)
		results = append(results, map[string]interface{}{
			"target":    "bot",
			"placement": "telegram_bot_message",
			"status":    res.Status,
			"error":     res.Error,
		})
	}

	if len(results) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ad has no telegram channels or bot enabled"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"results": results})
}

// GetAdDelivery GET /api/superadmin/ads/:id/delivery
func (h *AdHandler) GetAdDelivery(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid ad id"})
		return
	}

	records, err := h.adRepo.GetDeliveryHistory(id, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if records == nil {
		records = []models.AdDelivery{}
	}
	c.JSON(http.StatusOK, gin.H{"deliveries": records})
}
