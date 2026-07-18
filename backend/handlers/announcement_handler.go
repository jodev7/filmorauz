package handlers

import (
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AnnouncementHandler struct {
	repo     *repositories.AnnouncementRepository
	userRepo *repositories.UserRepository
}

func NewAnnouncementHandler(repo *repositories.AnnouncementRepository, userRepo *repositories.UserRepository) *AnnouncementHandler {
	return &AnnouncementHandler{repo: repo, userRepo: userRepo}
}

// enrichCreators populates CreatedByName for each item via a single batched
// users lookup so the admin list shows "kim yaratdi" without N+1 queries.
func (h *AnnouncementHandler) enrichCreators(items []models.Announcement) {
	if h.userRepo == nil || len(items) == 0 {
		return
	}
	idSet := map[primitive.ObjectID]bool{}
	ids := make([]primitive.ObjectID, 0, len(items))
	for _, it := range items {
		if it.CreatedBy.IsZero() || idSet[it.CreatedBy] {
			continue
		}
		idSet[it.CreatedBy] = true
		ids = append(ids, it.CreatedBy)
	}
	if len(ids) == 0 {
		return
	}
	users, err := h.userRepo.FindByIDs(ids)
	if err != nil {
		return
	}
	nameByID := map[primitive.ObjectID]string{}
	for _, u := range users {
		name := strings.TrimSpace(u.DisplayName)
		if name == "" {
			name = strings.TrimSpace(u.FirstName)
		}
		if name == "" {
			name = "—"
		}
		nameByID[u.ID] = name
	}
	for i := range items {
		if n, ok := nameByID[items[i].CreatedBy]; ok {
			items[i].CreatedByName = n
		}
	}
}

// GetActive returns active in-window announcements. Used by the global modal.
// GET /api/announcements/active
func (h *AnnouncementHandler) GetActive(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := h.repo.ListActive(ctx, time.Now())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AdminList returns every announcement for the admin UI.
// GET /api/admin/announcements
func (h *AnnouncementHandler) AdminList(c *gin.Context) {
	items, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	h.enrichCreators(items)
	c.JSON(http.StatusOK, gin.H{"items": items})
}

// AdminGet returns a single announcement.
// GET /api/admin/announcements/:id
func (h *AnnouncementHandler) AdminGet(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	a, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if a == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, a)
}

// AdminCreate creates a new announcement.
// POST /api/admin/announcements
func (h *AnnouncementHandler) AdminCreate(c *gin.Context) {
	var input models.AnnouncementInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !input.EndsAt.After(input.StartsAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ends_at must be after starts_at"})
		return
	}
	a := &models.Announcement{
		Type:        models.NormalizeAnnouncementType(input.Type),
		Title:       strings.TrimSpace(input.Title),
		Body:        input.Body,
		LinkURL:     strings.TrimSpace(input.LinkURL),
		LinkLabel:   strings.TrimSpace(input.LinkLabel),
		StartsAt:    input.StartsAt,
		EndsAt:      input.EndsAt,
		Dismissible: input.Dismissible,
		IsActive:    input.IsActive,
		Priority:    input.Priority,
	}
	if uid, ok := c.Get("user_id"); ok {
		if s, ok := uid.(string); ok {
			if oid, err := primitive.ObjectIDFromHex(s); err == nil {
				a.CreatedBy = oid
			}
		}
	}
	if err := h.repo.Create(c.Request.Context(), a); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, a)
}

// AdminUpdate updates an existing announcement.
// PATCH /api/admin/announcements/:id
func (h *AnnouncementHandler) AdminUpdate(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var input models.AnnouncementInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !input.EndsAt.After(input.StartsAt) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ends_at must be after starts_at"})
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	input.LinkURL = strings.TrimSpace(input.LinkURL)
	input.LinkLabel = strings.TrimSpace(input.LinkLabel)
	updated, err := h.repo.Update(c.Request.Context(), id, &input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if updated == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.JSON(http.StatusOK, updated)
}

// AdminDelete deletes an announcement.
// DELETE /api/admin/announcements/:id
func (h *AnnouncementHandler) AdminDelete(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.repo.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}
