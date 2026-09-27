package handlers

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// CommunityHandler: comment reports and short reviews.
type CommunityHandler struct {
	db       *mongo.Database
	repo     *repositories.CommunityRepository
	comments *services.CommentService
	ratings  *services.RatingService
}

func NewCommunityHandler(db *mongo.Database, repo *repositories.CommunityRepository, comments *services.CommentService, ratings *services.RatingService) *CommunityHandler {
	return &CommunityHandler{db: db, repo: repo, comments: comments, ratings: ratings}
}

var reportReasons = map[string]bool{"spam": true, "haqorat": true, "spoyler": true, "boshqa": true}

// ReportComment POST /api/v1/comments/:id/report {reason}
func (h *CommunityHandler) ReportComment(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	commentID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = c.ShouldBindJSON(&req)
	reason := strings.ToLower(strings.TrimSpace(req.Reason))
	if !reportReasons[reason] {
		reason = "boshqa"
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	var owner struct {
		UserID primitive.ObjectID `bson:"user_id"`
	}
	if err := h.db.Collection("movie_comments").FindOne(ctx, bson.M{"_id": commentID}).Decode(&owner); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	if owner.UserID == userID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "o'z izohingizga shikoyat qilib bo'lmaydi"})
		return
	}
	count, hidden, err := h.repo.ReportComment(ctx, commentID, userID, reason)
	if errors.Is(err, repositories.ErrAlreadyReported) {
		c.JSON(http.StatusOK, gin.H{"reported": true, "already": true})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to report"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"reported": true, "reports_count": count, "hidden": hidden})
}

// ListReportedComments GET /api/v1/admin/comments/reported
func (h *CommunityHandler) ListReportedComments(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	rows, err := h.repo.ListReportedComments(ctx, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reported comments"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": rows, "total": len(rows)})
}

// DismissCommentReports POST /api/v1/admin/comments/:id/dismiss-reports
func (h *CommunityHandler) DismissCommentReports(c *gin.Context) {
	commentID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if err := h.repo.DismissReports(ctx, commentID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ─── Reviews ─────────────────────────────────────────────────────────────────

func parseReviewTarget(c *gin.Context) (string, primitive.ObjectID, bool) {
	t := c.Param("type")
	if !repositories.ValidLibraryTarget(t) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "type must be movie or series"})
		return "", primitive.NilObjectID, false
	}
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return "", primitive.NilObjectID, false
	}
	return t, id, true
}

// ListReviews GET /api/reviews/:type/:id?sort=helpful|new
func (h *CommunityHandler) ListReviews(c *gin.Context) {
	t, id, ok := parseReviewTarget(c)
	if !ok {
		return
	}
	var viewer *primitive.ObjectID
	if raw := c.GetString("user_id"); raw != "" {
		if oid, err := primitive.ObjectIDFromHex(raw); err == nil {
			viewer = &oid
		}
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	reviews, total, err := h.repo.ListReviews(ctx, t, id, c.Query("sort"), limit, viewer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load reviews"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": reviews, "total": total})
}

// UpsertReview POST /api/user/reviews/:type/:id {rating, text}
// Also sets the user's star rating so the title's average stays in sync.
func (h *CommunityHandler) UpsertReview(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := parseReviewTarget(c)
	if !ok {
		return
	}
	var req struct {
		Rating int    `json:"rating"`
		Text   string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if req.Rating < 1 || req.Rating > 5 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "baho 1 dan 5 gacha bo'lishi kerak"})
		return
	}
	text := strings.Join(strings.Fields(req.Text), " ")
	if n := utf8.RuneCountInString(text); n < repositories.ReviewMinLen || n > repositories.ReviewMaxLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": "taqriz 10–500 belgidan iborat bo'lishi kerak"})
		return
	}
	if h.comments != nil {
		if err := h.comments.CheckTextAllowed(text); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	col := "movies"
	if t == "series" {
		col = "series"
	}
	if n, _ := h.db.Collection(col).CountDocuments(ctx, bson.M{"_id": id}); n == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "content not found"})
		return
	}
	if err := h.repo.UpsertReview(ctx, userID, t, id, req.Rating, text); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save review"})
		return
	}
	if h.ratings != nil {
		if t == "series" {
			_ = h.ratings.SetSeriesRating(id.Hex(), userID.Hex(), req.Rating)
		} else {
			_ = h.ratings.SetRating(id.Hex(), userID.Hex(), req.Rating)
		}
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// DeleteMyReview DELETE /api/user/reviews/:type/:id
func (h *CommunityHandler) DeleteMyReview(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	t, id, ok := parseReviewTarget(c)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	if _, err := h.repo.DeleteReview(ctx, bson.M{"user_id": userID, "target_type": t, "target_id": id}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete review"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

// ToggleReviewHelpful POST /api/user/review-helpful/:id
func (h *CommunityHandler) ToggleReviewHelpful(c *gin.Context) {
	userID, ok := currentUserOID(c)
	if !ok {
		return
	}
	reviewID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	marked, count, err := h.repo.ToggleHelpful(ctx, reviewID, userID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"helpful_by_me": marked, "helpful_count": count})
}

// AdminDeleteReview DELETE /api/v1/admin/reviews/:id (admins + moderators)
func (h *CommunityHandler) AdminDeleteReview(c *gin.Context) {
	reviewID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid review id"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	deleted, err := h.repo.DeleteReview(ctx, bson.M{"_id": reviewID})
	if err != nil || !deleted {
		c.JSON(http.StatusNotFound, gin.H{"error": "review not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
