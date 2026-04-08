package handlers

import (
	"net/http"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AdHandler struct {
	adRepo *repositories.AdRepository
}

func NewAdHandler(adRepo *repositories.AdRepository) *AdHandler {
	return &AdHandler{adRepo: adRepo}
}

// -- Admin endpoints --

// AdminListAds GET /api/admin/ads
func (h *AdHandler) AdminListAds(c *gin.Context) {
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
		Title        string   `json:"title" binding:"required"`
		Description  string   `json:"description"`
		ImageURL     string   `json:"image_url"`
		VideoURL     string   `json:"video_url"`
		TargetURL    string   `json:"target_url" binding:"required"`
		CallToAction string   `json:"call_to_action"`
		Placements   []string `json:"placements" binding:"required"`
		Status       string   `json:"status"`
		DurationDays int      `json:"duration_days"`
		Price        float64  `json:"price"`
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
		Title:        req.Title,
		Description:  req.Description,
		ImageURL:     req.ImageURL,
		VideoURL:     req.VideoURL,
		TargetURL:    req.TargetURL,
		CallToAction: req.CallToAction,
		Placements:   req.Placements,
		Status:       status,
		StartsAt:     startsAt,
		EndsAt:       endsAt,
		DurationDays: req.DurationDays,
		Price:        req.Price,
		CreatedBy:    createdBy,
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
		Title        string   `json:"title"`
		Description  string   `json:"description"`
		ImageURL     string   `json:"image_url"`
		VideoURL     string   `json:"video_url"`
		TargetURL    string   `json:"target_url"`
		CallToAction string   `json:"call_to_action"`
		Placements   []string `json:"placements"`
		Status       string   `json:"status"`
		DurationDays int      `json:"duration_days"`
		Price        float64  `json:"price"`
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
