package handlers

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// LibraryHandler serves a user's personal library: the "Keyinroq ko'raman"
// (watch later) list and series subscriptions, plus the admin action that
// links a user suggestion to the content that was added for it.
type LibraryHandler struct {
	db            *mongo.Database
	library       *repositories.LibraryRepository
	notifications *services.NotificationService
}

func NewLibraryHandler(db *mongo.Database, library *repositories.LibraryRepository, notifications *services.NotificationService) *LibraryHandler {
	return &LibraryHandler{db: db, library: library, notifications: notifications}
}

func currentUserOID(c *gin.Context) (primitive.ObjectID, bool) {
	raw, _ := c.Get("user_id")
	s, _ := raw.(string)
	id, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return primitive.NilObjectID, false
	}
	return id, true
}

func (h *LibraryHandler) parseTarget(c *gin.Context) (string, primitive.ObjectID, bool) {
	targetType := c.Param("type")
	if !repositories.ValidLibraryTarget(targetType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be movie or series"})
		return "", primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", primitive.NilObjectID, false
	}
	return targetType, id, true
}

func ctx5(c *gin.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(c.Request.Context(), 5*time.Second)
}

// ─── Watchlist ───────────────────────────────────────────────────────────────

// GetWatchlist GET /api/user/watchlist
func (h *LibraryHandler) GetWatchlist(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	items, err := h.library.ListWatchlist(ctx, userID, 200)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load watchlist"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": len(items)})
}

// AddToWatchlist POST /api/user/watchlist/:type/:id
func (h *LibraryHandler) AddToWatchlist(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	targetType, id, ok := h.parseTarget(c)
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if exists, err := h.library.TargetExists(ctx, targetType, id); err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	if err := h.library.AddToWatchlist(ctx, userID, targetType, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to add"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"in_watchlist": true})
}

// RemoveFromWatchlist DELETE /api/user/watchlist/:type/:id
func (h *LibraryHandler) RemoveFromWatchlist(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	targetType, id, ok := h.parseTarget(c)
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.library.RemoveFromWatchlist(ctx, userID, targetType, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"in_watchlist": false})
}

// LibraryStatus GET /api/user/library/:type/:id — watchlist + subscription
// state for one title, used by the buttons on movie/series pages.
func (h *LibraryHandler) LibraryStatus(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	targetType, id, ok := h.parseTarget(c)
	if !ok {
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	inList, _ := h.library.InWatchlist(ctx, userID, targetType, id)
	resp := gin.H{"in_watchlist": inList}
	if targetType == "series" {
		subscribed, _ := h.library.IsSubscribed(ctx, userID, id)
		resp["subscribed"] = subscribed
	}
	c.JSON(http.StatusOK, resp)
}

// ─── Series subscriptions ────────────────────────────────────────────────────

// GetSubscriptions GET /api/user/subscriptions
func (h *LibraryHandler) GetSubscriptions(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	items, err := h.library.SubscribedSeries(ctx, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load subscriptions"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items, "total": len(items)})
}

// Subscribe POST /api/user/subscriptions/series/:id
func (h *LibraryHandler) Subscribe(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	seriesID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if exists, err := h.library.TargetExists(ctx, "series", seriesID); err != nil || !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "series not found"})
		return
	}
	if err := h.library.Subscribe(ctx, userID, seriesID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to subscribe"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscribed": true})
}

// Unsubscribe DELETE /api/user/subscriptions/series/:id
func (h *LibraryHandler) Unsubscribe(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	seriesID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	ctx, cancel := ctx5(c)
	defer cancel()
	if err := h.library.Unsubscribe(ctx, userID, seriesID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unsubscribe"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"subscribed": false})
}

// ─── Suggestion → added content ──────────────────────────────────────────────

// LinkSuggestion POST /api/admin/suggestions/:id/link {target_type, target_id}
//
// Marks a suggestion as accepted AND fulfilled by a specific movie/series,
// then tells the user ("Tavsiyangiz qo'shildi") on the site and in Telegram
// with a direct link to watch it.
func (h *LibraryHandler) LinkSuggestion(c *gin.Context) {
	suggestionID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid suggestion id"})
		return
	}
	var req struct {
		TargetType string `json:"target_type"`
		TargetID   string `json:"target_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || !repositories.ValidLibraryTarget(req.TargetType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type (movie|series) va target_id kerak"})
		return
	}
	targetID, err := primitive.ObjectIDFromHex(req.TargetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target id"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()

	col := "movies"
	if req.TargetType == "series" {
		col = "series"
	}
	var content struct {
		Title   string `bson:"title"`
		TitleUz string `bson:"title_uz"`
		Slug    string `bson:"slug"`
	}
	if err := h.db.Collection(col).FindOne(ctx, bson.M{"_id": targetID},
		options.FindOne().SetProjection(bson.M{"title": 1, "title_uz": 1, "slug": 1})).Decode(&content); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "kontent topilmadi"})
		return
	}
	title := content.TitleUz
	if title == "" {
		title = content.Title
	}

	reviewer, _ := c.Get("user_id")
	now := time.Now()
	var before models.Suggestion
	err = h.db.Collection("suggestions").FindOneAndUpdate(ctx, bson.M{"_id": suggestionID}, bson.M{"$set": bson.M{
		"status":       models.SuggestionStatusAccepted,
		"linked_type":  req.TargetType,
		"linked_id":    targetID.Hex(),
		"linked_slug":  content.Slug,
		"linked_title": title,
		"linked_at":    now,
		"reviewed_by":  fmt.Sprint(reviewer),
		"reviewed_at":  now,
		"updated_at":   now,
	}}).Decode(&before) // returns the document BEFORE the update
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "tavsiya topilmadi"})
		return
	}

	// Only notify the first time a suggestion is linked to this content.
	alreadyLinked := before.LinkedID == targetID.Hex()
	if !alreadyLinked && h.notifications != nil {
		path := "/movies/" + content.Slug
		if req.TargetType == "series" {
			path = "/series/" + content.Slug
		}
		go h.notifications.NotifyUserBoth(context.Background(), before.UserID, models.NotificationSuggestionAdded,
			"Tavsiyangiz qo'shildi 🎉",
			fmt.Sprintf("Siz so'ragan «%s» saytga qo'shildi. Marhamat, tomosha qiling!", title),
			path,
			map[string]interface{}{"suggestion_id": suggestionID.Hex(), "target_type": req.TargetType, "target_id": targetID.Hex()},
			fmt.Sprintf("🎉 Siz tavsiya qilgan <b>%s</b> FilmoraUz'ga qo'shildi!", html.EscapeString(strings.TrimSpace(title))),
			"▶️ Tomosha qilish",
		)
	}

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"notified":     !alreadyLinked,
		"linked_title": title,
		"linked_slug":  content.Slug,
	})
}
