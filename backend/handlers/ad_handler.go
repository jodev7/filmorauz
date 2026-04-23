package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// telegramMediaArgs maps the ad's TelegramMediaURL/Type to (imageURL, videoURL)
// arguments expected by SendAdToChannel / SendAdToBot.
func telegramMediaArgs(mediaURL, mediaType string) (imageURL, videoURL string) {
	if mediaType == "video" {
		return "", mediaURL
	}
	return mediaURL, ""
}

type AdHandler struct {
	adRepo          *repositories.AdRepository
	telegram        *services.TelegramService
	defaultChannels []string // loaded from TELEGRAM_CHANNELS env
	userRepo        *repositories.UserRepository
}

func NewAdHandler(adRepo *repositories.AdRepository, telegram *services.TelegramService, defaultChannels []string, userRepo *repositories.UserRepository) *AdHandler {
	return &AdHandler{adRepo: adRepo, telegram: telegram, defaultChannels: defaultChannels, userRepo: userRepo}
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
		Priority               int      `json:"priority"`
		BannerMediaURL         string   `json:"banner_media_url"`
		BannerMediaType        string   `json:"banner_media_type"`
		InlineMediaURL         string   `json:"inline_media_url"`
		InlineMediaType        string   `json:"inline_media_type"`
		FixedBottomMediaURL    string   `json:"fixed_bottom_media_url"`
		FixedBottomMediaType   string   `json:"fixed_bottom_media_type"`
		PopupMediaURL          string   `json:"popup_media_url"`
		PopupMediaType         string   `json:"popup_media_type"`
		PlayerOverlayMediaURL  string   `json:"player_overlay_media_url"`
		PlayerOverlayMediaType string   `json:"player_overlay_media_type"`
		TelegramMediaURL       string   `json:"telegram_media_url"`
		TelegramMediaType      string   `json:"telegram_media_type"`
		TelegramChannels       []string `json:"telegram_channels"`
		TelegramBotEnabled     bool     `json:"telegram_bot_enabled"`
		TelegramBotChatIDs     []int64  `json:"telegram_bot_chat_ids"`
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
		Priority:               req.Priority,
		BannerMediaURL:         req.BannerMediaURL,
		BannerMediaType:        req.BannerMediaType,
		InlineMediaURL:         req.InlineMediaURL,
		InlineMediaType:        req.InlineMediaType,
		FixedBottomMediaURL:    req.FixedBottomMediaURL,
		FixedBottomMediaType:   req.FixedBottomMediaType,
		PopupMediaURL:          req.PopupMediaURL,
		PopupMediaType:         req.PopupMediaType,
		PlayerOverlayMediaURL:  req.PlayerOverlayMediaURL,
		PlayerOverlayMediaType: req.PlayerOverlayMediaType,
		TelegramMediaURL:       req.TelegramMediaURL,
		TelegramMediaType:      req.TelegramMediaType,
		TelegramChannels:       req.TelegramChannels,
		TelegramBotEnabled:     req.TelegramBotEnabled,
		TelegramBotChatIDs:     req.TelegramBotChatIDs,
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
		Priority               *int     `json:"priority"`
		BannerMediaURL         string   `json:"banner_media_url"`
		BannerMediaType        string   `json:"banner_media_type"`
		InlineMediaURL         string   `json:"inline_media_url"`
		InlineMediaType        string   `json:"inline_media_type"`
		FixedBottomMediaURL    string   `json:"fixed_bottom_media_url"`
		FixedBottomMediaType   string   `json:"fixed_bottom_media_type"`
		PopupMediaURL          string   `json:"popup_media_url"`
		PopupMediaType         string   `json:"popup_media_type"`
		PlayerOverlayMediaURL  string   `json:"player_overlay_media_url"`
		PlayerOverlayMediaType string   `json:"player_overlay_media_type"`
		TelegramMediaURL       string   `json:"telegram_media_url"`
		TelegramMediaType      string   `json:"telegram_media_type"`
		TelegramChannels       []string `json:"telegram_channels"`
		TelegramBotEnabled     *bool    `json:"telegram_bot_enabled"`
		TelegramBotChatIDs     []int64  `json:"telegram_bot_chat_ids"`
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
	if req.Price != 0 {
		update["price"] = req.Price
	}
	if req.Priority != nil {
		update["priority"] = *req.Priority
	}
	update["banner_media_url"] = req.BannerMediaURL
	update["banner_media_type"] = req.BannerMediaType
	update["inline_media_url"] = req.InlineMediaURL
	update["inline_media_type"] = req.InlineMediaType
	update["fixed_bottom_media_url"] = req.FixedBottomMediaURL
	update["fixed_bottom_media_type"] = req.FixedBottomMediaType
	update["popup_media_url"] = req.PopupMediaURL
	update["popup_media_type"] = req.PopupMediaType
	update["player_overlay_media_url"] = req.PlayerOverlayMediaURL
	update["player_overlay_media_type"] = req.PlayerOverlayMediaType
	update["telegram_media_url"] = req.TelegramMediaURL
	update["telegram_media_type"] = req.TelegramMediaType
	if req.TelegramChannels != nil {
		update["telegram_channels"] = req.TelegramChannels
	}
	if req.TelegramBotEnabled != nil {
		update["telegram_bot_enabled"] = *req.TelegramBotEnabled
	}
	if req.TelegramBotChatIDs != nil {
		update["telegram_bot_chat_ids"] = req.TelegramBotChatIDs
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
// Returns all active ads for the placement, ordered by priority DESC then created_at ASC.
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

// GetAdsBatch GET /api/ads/batch?placements=a,b,c
// Returns ads grouped by placement so the frontend can batch homepage ad loads.
func (h *AdHandler) GetAdsBatch(c *gin.Context) {
	rawPlacements := strings.TrimSpace(c.Query("placements"))
	if rawPlacements == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "placements query param required"})
		return
	}

	seen := make(map[string]struct{})
	placements := make([]string, 0, 8)
	for _, placement := range strings.Split(rawPlacements, ",") {
		placement = strings.TrimSpace(placement)
		if placement == "" {
			continue
		}
		if _, exists := seen[placement]; exists {
			continue
		}
		seen[placement] = struct{}{}
		placements = append(placements, placement)
	}

	if len(placements) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "at least one placement is required"})
		return
	}

	adsByPlacement, err := h.adRepo.FindByPlacements(placements)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"placements": adsByPlacement})
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
	if ad.TelegramChannelEnabled {
		// Use env channels by default; fall back to ad's explicit list
		targetChannels := h.defaultChannels
		if len(targetChannels) == 0 {
			targetChannels = ad.TelegramChannels
		}
		log.Printf("[AD-TELEGRAM] channel delivery: %d targets (env=%d, ad=%d)",
			len(targetChannels), len(h.defaultChannels), len(ad.TelegramChannels))

		if len(targetChannels) == 0 {
			log.Printf("[AD-TELEGRAM] channel delivery skipped for ad %s: no channels configured", id.Hex())
			results = append(results, map[string]interface{}{
				"target":    "channels",
				"placement": "telegram_channel_post",
				"status":    "failed",
				"error":     "no channels configured — set TELEGRAM_CHANNELS in .env",
			})
		} else {
			for _, ch := range targetChannels {
				target := ch
				if len(target) > 0 && target[0] != '@' {
					target = "@" + target
				}
				log.Printf("[AD-TELEGRAM] sending channel ad to %s", target)
				imgURL, vidURL := telegramMediaArgs(ad.TelegramMediaURL, ad.TelegramMediaType)
				res := h.telegram.SendAdToChannel(target, ad.Title, ad.Description, imgURL, vidURL, ad.TargetURL, ad.CallToAction)
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
	}

	// Post to bot — fetch user chat_ids from MongoDB
	if ad.TelegramBotEnabled {
		chatIDs, err := h.userRepo.FindChatIDs(500)
		if err != nil {
			log.Printf("[AD-TELEGRAM] failed to load user chat_ids for ad %s: %v", id.Hex(), err)
			chatIDs = nil
		}
		log.Printf("[AD-TELEGRAM] bot delivery: %d valid user chat_ids found", len(chatIDs))

		if len(chatIDs) == 0 {
			log.Printf("[AD-TELEGRAM] bot delivery skipped for ad %s: no valid user chat_ids", id.Hex())
			_ = h.adRepo.LogDelivery(&models.AdDelivery{
				AdID:      id,
				Placement: "telegram_bot_message",
				Target:    "bot",
				Status:    "failed",
				SentAt:    time.Now(),
				Error:     "no_valid_chat_ids",
			})
			results = append(results, map[string]interface{}{
				"target":    "bot",
				"placement": "telegram_bot_message",
				"status":    "failed",
				"error":     "no valid user chat_ids — users must send /start to the bot first",
			})
		} else {
			for _, chatID := range chatIDs {
				log.Printf("[AD-TELEGRAM] sending bot ad to chat_id=%d", chatID)
				imgURL, vidURL := telegramMediaArgs(ad.TelegramMediaURL, ad.TelegramMediaType)
				res := h.telegram.SendAdToBot(chatID, ad.Title, ad.Description, imgURL, vidURL, ad.TargetURL, ad.CallToAction)
				target := fmt.Sprintf("bot:%d", chatID)
				_ = h.adRepo.LogDelivery(&models.AdDelivery{
					AdID:      id,
					Placement: "telegram_bot_message",
					Target:    target,
					Status:    res.Status,
					MessageID: res.MessageID,
					SentAt:    time.Now(),
					Error:     res.Error,
				})
				results = append(results, map[string]interface{}{
					"target":     target,
					"placement":  "telegram_bot_message",
					"status":     res.Status,
					"message_id": res.MessageID,
					"error":      res.Error,
				})
			}
		}
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
