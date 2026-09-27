package handlers

import (
	"errors"
	"net/http"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// UserListHandler serves personal, shareable lists.
type UserListHandler struct {
	lists *repositories.UserListRepository
}

func NewUserListHandler(lists *repositories.UserListRepository) *UserListHandler {
	return &UserListHandler{lists: lists}
}

func listError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repositories.ErrListNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Ro'yxat topilmadi"})
	case errors.Is(err, repositories.ErrListLimit):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ko'pi bilan 30 ta ro'yxat yaratish mumkin"})
	case errors.Is(err, repositories.ErrListFull):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Ro'yxatga ko'pi bilan 200 ta kino qo'shish mumkin"})
	case errors.Is(err, repositories.ErrListTitle):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Nom 1–60 belgidan iborat bo'lsin"})
	case errors.Is(err, repositories.ErrListDesc):
		c.JSON(http.StatusBadRequest, gin.H{"error": "Tavsif juda uzun (300 belgigacha)"})
	case errors.Is(err, repositories.ErrTargetNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "Kino topilmadi"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Xatolik yuz berdi"})
	}
}

func listIDParam(c *gin.Context) (primitive.ObjectID, bool) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return primitive.NilObjectID, false
	}
	return id, true
}

// Mine GET /api/user/lists
func (h *UserListHandler) Mine(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	lists, err := h.lists.ListMine(ctx, userID)
	if err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": lists})
}

// Create POST /api/user/lists {title, description, is_public}
func (h *UserListHandler) Create(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	var req struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		IsPublic    *bool  `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	public := true
	if req.IsPublic != nil {
		public = *req.IsPublic
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	l, err := h.lists.Create(ctx, userID, req.Title, req.Description, public)
	if err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": gin.H{
		"id": l.ID.Hex(), "title": l.Title, "description": l.Description, "is_public": l.IsPublic,
		"share_slug": l.ShareSlug, "count": 0, "covers": []string{}, "updated_at": l.UpdatedAt,
	}})
}

// Update PATCH /api/user/lists/:id
func (h *UserListHandler) Update(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	listID, ok := listIDParam(c)
	if !ok {
		return
	}
	var req struct {
		Title       *string `json:"title"`
		Description *string `json:"description"`
		IsPublic    *bool   `json:"is_public"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.lists.Update(ctx, userID, listID, req.Title, req.Description, req.IsPublic); err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Delete DELETE /api/user/lists/:id
func (h *UserListHandler) Delete(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	listID, ok := listIDParam(c)
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.lists.Delete(ctx, userID, listID); err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func libraryTarget(c *gin.Context, typeKey, idKey string) (string, primitive.ObjectID, bool) {
	t := c.Param(typeKey)
	if !repositories.ValidLibraryTarget(t) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be movie or series"})
		return "", primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(c.Param(idKey))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", primitive.NilObjectID, false
	}
	return t, id, true
}

// AddItem POST /api/user/lists/:id/items/:type/:targetId
func (h *UserListHandler) AddItem(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	listID, ok := listIDParam(c)
	if !ok {
		return
	}
	t, target, ok := libraryTarget(c, "type", "targetId")
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.lists.AddItem(ctx, userID, listID, t, target); err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// RemoveItem DELETE /api/user/lists/:id/items/:type/:targetId
func (h *UserListHandler) RemoveItem(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	listID, ok := listIDParam(c)
	if !ok {
		return
	}
	t, target, ok := libraryTarget(c, "type", "targetId")
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.lists.RemoveItem(ctx, userID, listID, t, target); err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// Containing GET /api/user/lists-containing/:type/:id
func (h *UserListHandler) Containing(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, target, ok := libraryTarget(c, "type", "id")
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	ids, err := h.lists.ListsContaining(ctx, userID, t, target)
	if err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": ids})
}

// BySlug GET /api/lists/:slug — share page (optional auth for owner view).
func (h *UserListHandler) BySlug(c *gin.Context) {
	viewer := primitive.NilObjectID
	if raw := c.GetString("user_id"); raw != "" {
		if id, err := primitive.ObjectIDFromHex(raw); err == nil {
			viewer = id
		}
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	d, err := h.lists.GetBySlug(ctx, c.Param("slug"), viewer)
	if err != nil {
		listError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": d})
}
