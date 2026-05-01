package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CommentHandler handles comment-related HTTP requests
type CommentHandler struct {
	commentService      *services.CommentService
	notificationService *services.NotificationService
	userRepo            *repositories.UserRepository
	movieRepo           *repositories.MovieRepository
}

func NewCommentHandler(commentService *services.CommentService, notificationService *services.NotificationService, userRepo *repositories.UserRepository, movieRepo *repositories.MovieRepository) *CommentHandler {
	return &CommentHandler{
		commentService:      commentService,
		notificationService: notificationService,
		userRepo:            userRepo,
		movieRepo:           movieRepo,
	}
}

// CommentRequest is the request body for creating a comment
type CommentRequest struct {
	Content    string `json:"content" binding:"required"`
	TargetType string `json:"target_type"` // "movie" or "episode"
	TargetID   string `json:"target_id"`   // ID of movie or episode
}

// EpisodeCommentRequest is the request body for creating an episode comment
type EpisodeCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

// ReplyRequest is the request body for replying to a comment
type ReplyRequest struct {
	Content string `json:"content" binding:"required"`
}

// UpdateCommentRequest is the request body for updating a comment
type UpdateCommentRequest struct {
	Content string `json:"content" binding:"required"`
}

// CommentResponse represents a comment response with replies
type CommentResponse struct {
	Comment *services.CommentWithUserDTO  `json:"comment"`
	Replies []services.CommentWithUserDTO `json:"replies,omitempty"`
}

// GetComments godoc
// GET /api/v1/movies/:id/comments
func (h *CommentHandler) GetComments(c *gin.Context) {
	movieIDStr := c.Param("id")
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	skip := (page - 1) * limit

	// Get all approved comments for this movie (including nested replies)
	allComments, err := h.commentService.GetAllApprovedComments(movieID, limit*5, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get comments"})
		return
	}

	applyLikedByMe(allComments, extractViewerID(c))

	// Build tree structure and convert to response
	tree := buildCommentTree(allComments)

	// Convert tree to response format
	result := make([]CommentResponse, 0, len(tree))
	for _, node := range tree {
		commentDTO := commentToDTO(node.Comment)
		repliesDTO := convertRepliesToTree(node.Children, allComments)
		result = append(result, CommentResponse{
			Comment: &commentDTO,
			Replies: repliesDTO,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result, "page": page, "limit": limit})
}

// commentToDTO converts a CommentWithUser to CommentWithUserDTO
func commentToDTO(c models.CommentWithUser) services.CommentWithUserDTO {
	dto := services.CommentWithUserDTO{
		ID:                  c.ID.Hex(),
		UserID:              c.UserID.Hex(),
		ParentID:            pointerToString(c.ParentID),
		Content:             c.Content,
		Status:              c.Status,
		HasBlockedWord:      c.HasBlockedWord,
		HasLink:             c.HasLink,
		CreatedAt:           c.CreatedAt,
		UpdatedAt:           c.UpdatedAt,
		UserDisplayName:     c.UserDisplayName,
		UserAvatarURL:       c.UserAvatarURL,
		UserRole:            c.UserRole,
		UserIsPremium:       c.UserIsPremium,
		UserIsPremiumActive: c.UserIsPremiumActive,
		RepliesCount:        c.RepliesCount,
		LikesCount:          c.LikesCount,
		LikedByMe:           c.LikedByMe,
	}
	if !c.MovieID.IsZero() {
		dto.MovieID = c.MovieID.Hex()
	}
	if c.TargetType != "" {
		dto.TargetType = string(c.TargetType)
	}
	if !c.TargetID.IsZero() {
		dto.TargetID = c.TargetID.Hex()
	}
	return dto
}

// applyLikedByMe stamps liked_by_me on comments based on viewer id
func applyLikedByMe(comments []models.CommentWithUser, viewerID *primitive.ObjectID) {
	if viewerID == nil || viewerID.IsZero() {
		return
	}
	for i := range comments {
		for _, id := range comments[i].LikedBy {
			if id == *viewerID {
				comments[i].LikedByMe = true
				break
			}
		}
	}
}

// extractViewerID returns the authenticated user's ObjectID if present.
func extractViewerID(c *gin.Context) *primitive.ObjectID {
	v, ok := c.Get("user_id")
	if !ok {
		return nil
	}
	s, ok := v.(string)
	if !ok || s == "" {
		return nil
	}
	oid, err := primitive.ObjectIDFromHex(s)
	if err != nil {
		return nil
	}
	return &oid
}

// convertRepliesToTree converts children to DTOs with their nested replies
func convertRepliesToTree(children []models.CommentWithUser, allComments []models.CommentWithUser) []services.CommentWithUserDTO {
	result := make([]services.CommentWithUserDTO, 0)
	for _, child := range children {
		childDTO := commentToDTO(child)
		// Find children of this reply
		childChildren := getChildrenOfComment(child.ID, allComments)
		if len(childChildren) > 0 {
			// Recursively get nested replies
			nestedReplies := convertRepliesToTree(childChildren, allComments)
			// Add nested replies to this reply's replies
			childDTO.Replies = nestedReplies
		}
		result = append(result, childDTO)
	}
	return result
}

// getChildrenOfComment finds all comments that have the given parent ID
func getChildrenOfComment(parentID primitive.ObjectID, allComments []models.CommentWithUser) []models.CommentWithUser {
	var children []models.CommentWithUser
	for _, c := range allComments {
		if c.ParentID != nil && *c.ParentID == parentID {
			children = append(children, c)
		}
	}
	return children
}

// CommentTreeNode represents a comment and its children
type CommentTreeNode struct {
	Comment  models.CommentWithUser
	Children []models.CommentWithUser
}

// GetCommentsByTarget godoc
// GET /api/v1/comments - Get comments by target (movie or episode)
func (h *CommentHandler) GetCommentsByTarget(c *gin.Context) {
	targetType := c.Query("target_type") // "movie" or "episode"
	targetIDStr := c.Query("target_id")

	if targetType == "" || targetIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_type and target_id are required"})
		return
	}

	targetID, err := primitive.ObjectIDFromHex(targetIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target id"})
		return
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}
	skip := (page - 1) * limit

	// Get all approved comments for this target (including nested replies)
	allComments, err := h.commentService.GetAllApprovedCommentsByTarget(models.CommentTargetType(targetType), targetID, limit*5, skip)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get comments"})
		return
	}

	applyLikedByMe(allComments, extractViewerID(c))

	// Build tree structure and convert to response
	tree := buildCommentTree(allComments)

	// Convert tree to response format
	result := make([]CommentResponse, 0, len(tree))
	for _, node := range tree {
		commentDTO := commentToDTO(node.Comment)
		repliesDTO := convertRepliesToTree(node.Children, allComments)
		result = append(result, CommentResponse{
			Comment: &commentDTO,
			Replies: repliesDTO,
		})
	}

	c.JSON(http.StatusOK, gin.H{"data": result, "page": page, "limit": limit})
}

// buildCommentTree builds a tree structure from flat comments
func buildCommentTree(comments []models.CommentWithUser) []CommentTreeNode {
	// Find root comments (no parent)
	var rootNodes []CommentTreeNode
	for _, c := range comments {
		if c.ParentID == nil {
			rootNodes = append(rootNodes, CommentTreeNode{
				Comment:  c,
				Children: getChildrenOfComment(c.ID, comments),
			})
		}
	}
	return rootNodes
}

// CreateComment godoc
// POST /api/v1/movies/:id/comments
func (h *CommentHandler) CreateComment(c *gin.Context) {
	// Get user from context
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	// Get movie ID (for backward compatibility) and new target fields
	movieIDStr := c.Param("id")
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	var req CommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	// Determine target type and ID
	var targetType models.CommentTargetType
	var targetID primitive.ObjectID

	if req.TargetType != "" && req.TargetID != "" {
		// New format: use target_type and target_id
		targetType = models.CommentTargetType(req.TargetType)
		targetID, err = primitive.ObjectIDFromHex(req.TargetID)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid target id"})
			return
		}
	} else {
		// Backward compatibility: use movie_id from URL
		targetType = models.CommentTargetMovie
		targetID = movieID
	}

	comment, err := h.commentService.CreateComment(movieID, userOID, req.Content, nil, targetType, targetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Return appropriate response based on status
	if comment.Status == "pending" {
		c.JSON(http.StatusCreated, gin.H{
			"message": "comment submitted for moderation",
			"status":  "pending",
			"data": gin.H{
				"id":               comment.ID.Hex(),
				"has_blocked_word": comment.HasBlockedWord,
				"has_link":         comment.HasLink,
			},
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": comment})
}

// CreateReply godoc
// POST /api/v1/comments/:id/replies
func (h *CommentHandler) CreateReply(c *gin.Context) {
	// Get user from context
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	parentIDStr := c.Param("id")
	parentID, err := primitive.ObjectIDFromHex(parentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	// Get parent comment to find movie ID and target info
	parentComment, err := h.commentService.GetCommentByID(parentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "parent comment not found"})
		return
	}

	// Check if parent comment is approved - don't allow replies to deleted/pending/rejected comments
	if parentComment.Status != models.CommentStatusApproved {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot reply to this comment"})
		return
	}

	// Don't allow self-replies (replying to your own comment)
	if parentComment.UserID == userOID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot reply to your own comment"})
		return
	}

	var req ReplyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	// Validate content is not just whitespace
	trimmedContent := strings.TrimSpace(req.Content)
	if trimmedContent == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content cannot be empty or whitespace only"})
		return
	}

	// Use target from parent comment
	targetType := parentComment.TargetType
	targetID := parentComment.TargetID

	// Fallback to movie_id for backward compatibility
	if targetID.IsZero() && !parentComment.MovieID.IsZero() {
		targetType = models.CommentTargetMovie
		targetID = parentComment.MovieID
	}

	comment, err := h.commentService.CreateComment(parentComment.MovieID, userOID, req.Content, &parentID, targetType, targetID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Send notification to the parent comment owner about the reply
	// Only notify if reply was approved and replier is not the same as parent comment owner
	if comment.Status == "approved" && parentComment.UserID != userOID {
		// Get replier's info
		replierID := userOID
		replierName := "Foydalanuvchi"
		if h.userRepo != nil {
			replier, err := h.userRepo.FindByID(userID)
			if err == nil && replier != nil {
				if replier.DisplayName != "" {
					replierName = replier.DisplayName
				} else if replier.FirstName != "" {
					replierName = replier.FirstName
				} else if replier.TelegramUser != "" {
					replierName = replier.TelegramUser
				}
			}
		}

		// Get action URL based on target type
		actionURL := ""
		movieSlug := ""
		movieIDStr := targetID.Hex()

		if h.movieRepo != nil && !targetID.IsZero() {
			if targetType == models.CommentTargetMovie {
				movie, err := h.movieRepo.FindByIDHex(targetID.Hex())
				if err == nil && movie != nil {
					movieSlug = movie.Slug
					actionURL = fmt.Sprintf("/movies/%s#comment-%s", movieSlug, parentIDStr)
				}
			} else if targetType == models.CommentTargetEpisode {
				// For episodes, try to get series/movie info for the URL
				actionURL = fmt.Sprintf("/episode/%s#comment-%s", targetID.Hex(), parentIDStr)
			}
		}

		// If we couldn't build proper URL, use fallback
		if actionURL == "" {
			actionURL = fmt.Sprintf("/#comment-%s", parentIDStr)
		}

		// Send notification asynchronously
		go func() {
			if h.notificationService != nil {
				err := h.notificationService.NotifyCommentReply(
					c.Request.Context(),
					parentComment.UserID,
					movieIDStr,
					movieSlug,
					replierID,
					parentIDStr,
					comment.ID.Hex(),
					replierName,
					actionURL,
				)
				if err != nil {
					log.Printf("[Notification] Failed to send comment reply notification: %v", err)
				} else {
					log.Printf("[Notification] Comment reply notification sent to user %s", parentComment.UserID.Hex())
				}
			}
		}()
	}

	if comment.Status == "pending" {
		c.JSON(http.StatusCreated, gin.H{
			"message": "reply submitted for moderation",
			"status":  "pending",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": comment})
}

// UpdateComment godoc
// PUT /api/v1/comments/:id
func (h *CommentHandler) UpdateComment(c *gin.Context) {
	// Get user from context
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	var req UpdateCommentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content is required"})
		return
	}

	comment, err := h.commentService.UpdateComment(commentID, userOID, req.Content)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": comment})
}

// DeleteComment godoc
// DELETE /api/v1/comments/:id
func (h *CommentHandler) DeleteComment(c *gin.Context) {
	// Get user from context
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}

	userID, ok := userIDVal.(string)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	err = h.commentService.DeleteComment(commentID, userOID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "comment deleted"})
}

// Helper function to convert *primitive.ObjectID to *string
func pointerToString(ptr *primitive.ObjectID) *string {
	if ptr == nil {
		return nil
	}
	s := ptr.Hex()
	return &s
}

// AdminGetCommentSettings godoc
// GET /api/v1/admin/comment-settings
func (h *CommentHandler) AdminGetCommentSettings(c *gin.Context) {
	settings, err := h.commentService.GetModerationSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get settings"})
		return
	}
	c.JSON(http.StatusOK, settings)
}

// AdminUpdateCommentSettings godoc
// PUT /api/v1/admin/comment-settings
func (h *CommentHandler) AdminUpdateCommentSettings(c *gin.Context) {
	var req struct {
		CommentsEnabled        bool     `json:"comments_enabled"`
		RepliesEnabled         bool     `json:"replies_enabled"`
		BlockLinks             bool     `json:"block_links"`
		MaxCommentLength       int      `json:"max_comment_length"`
		MaxReplyDepth          int      `json:"max_reply_depth"`
		RequireModeration      bool     `json:"require_moderation"`
		BannedWords            []string `json:"banned_words"`
		AutoHideBannedContent  bool     `json:"auto_hide_banned_content"`
		CommentCooldownSeconds int      `json:"comment_cooldown_seconds"`
		MaxCommentsPerMinute   int      `json:"max_comments_per_minute"`
		DefaultSort            string   `json:"default_sort"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Get existing settings to preserve ID and UpdatedBy
	existing, err := h.commentService.GetModerationSettings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get existing settings"})
		return
	}

	// Update settings
	if req.MaxCommentLength > 0 {
		existing.MaxCommentLength = req.MaxCommentLength
	}
	if req.MaxReplyDepth > 0 {
		existing.MaxReplyDepth = req.MaxReplyDepth
	}
	if req.CommentCooldownSeconds >= 0 {
		existing.CommentCooldownSeconds = req.CommentCooldownSeconds
	}
	if req.MaxCommentsPerMinute > 0 {
		existing.MaxCommentsPerMinute = req.MaxCommentsPerMinute
	}

	existing.CommentsEnabled = req.CommentsEnabled
	existing.RepliesEnabled = req.RepliesEnabled
	existing.BlockLinks = req.BlockLinks
	existing.RequireModeration = req.RequireModeration
	existing.BannedWords = req.BannedWords
	existing.AutoHideBannedContent = req.AutoHideBannedContent
	existing.DefaultSort = req.DefaultSort

	err = h.commentService.UpdateModerationSettings(existing)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update settings"})
		return
	}

	c.JSON(http.StatusOK, existing)
}

// ToggleCommentLike godoc
// POST /api/v1/comments/:id/like — toggles like and returns updated count.
func (h *CommentHandler) ToggleCommentLike(c *gin.Context) {
	userIDVal, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
		return
	}
	userID, _ := userIDVal.(string)
	userOID, err := primitive.ObjectIDFromHex(userID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user id"})
		return
	}

	commentID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	comment, err := h.commentService.GetCommentByID(commentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "comment not found"})
		return
	}

	liked, count, ownerID, err := h.commentService.ToggleLike(commentID, userOID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to toggle like"})
		return
	}

	// Notify on new like (not unlike) and not on own comment
	if liked && ownerID != userOID && h.notificationService != nil {
		// Resolve liker name and action URL
		likerName := "Foydalanuvchi"
		if h.userRepo != nil {
			if u, err := h.userRepo.FindByID(userID); err == nil && u != nil {
				if u.DisplayName != "" {
					likerName = u.DisplayName
				} else if u.FirstName != "" {
					likerName = u.FirstName
				} else if u.TelegramUser != "" {
					likerName = u.TelegramUser
				}
			}
		}

		actionURL := ""
		movieIDStr := ""
		movieSlug := ""
		if comment.TargetType == models.CommentTargetEpisode && !comment.TargetID.IsZero() {
			actionURL = fmt.Sprintf("/episode/%s#comment-%s", comment.TargetID.Hex(), commentID.Hex())
		} else if h.movieRepo != nil {
			targetMovieID := comment.TargetID
			if targetMovieID.IsZero() {
				targetMovieID = comment.MovieID
			}
			if !targetMovieID.IsZero() {
				movieIDStr = targetMovieID.Hex()
				if movie, err := h.movieRepo.FindByIDHex(targetMovieID.Hex()); err == nil && movie != nil {
					movieSlug = movie.Slug
					actionURL = fmt.Sprintf("/movies/%s#comment-%s", movieSlug, commentID.Hex())
				}
			}
		}
		if actionURL == "" {
			actionURL = fmt.Sprintf("/#comment-%s", commentID.Hex())
		}

		go func() {
			_ = h.notificationService.NotifyCommentLike(
				context.Background(),
				ownerID,
				userOID,
				likerName,
				movieIDStr,
				movieSlug,
				commentID.Hex(),
				actionURL,
			)
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"liked":       liked,
		"likes_count": count,
	})
}

// AdminGetComments godoc
// GET /api/v1/admin/comments
func (h *CommentHandler) AdminGetComments(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	status := c.Query("status")
	// Note: search functionality not yet implemented

	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	// For now, we don't have search functionality in the repository
	// So we'll just get all comments with optional status filter
	comments, total, err := h.commentService.GetCommentsByStatus(status, page, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get comments"})
		return
	}

	totalPages := int(total) / limit
	if int(total)%limit > 0 {
		totalPages++
	}

	// Convert to response format with user info
	result := make([]gin.H, 0, len(comments))
	for _, c := range comments {
		targetType := string(c.TargetType)
		if targetType == "" {
			targetType = string(models.CommentTargetMovie)
		}
		targetID := ""
		if !c.TargetID.IsZero() {
			targetID = c.TargetID.Hex()
		} else if !c.MovieID.IsZero() {
			targetID = c.MovieID.Hex()
		}
		targetURL := ""
		targetSlug := ""
		if targetType == string(models.CommentTargetEpisode) && targetID != "" {
			targetURL = fmt.Sprintf("/episode/%s", targetID)
		} else if h.movieRepo != nil && !c.MovieID.IsZero() {
			if movie, err := h.movieRepo.FindByIDHex(c.MovieID.Hex()); err == nil && movie != nil {
				targetSlug = movie.Slug
				if targetSlug != "" {
					targetURL = fmt.Sprintf("/movies/%s", targetSlug)
				}
			}
		}

		userDisplayName := c.UserDisplayName
		if userDisplayName == "" {
			userDisplayName = "Noma'lum"
		}
		result = append(result, gin.H{
			"id":                     c.ID.Hex(),
			"movie_id":               c.MovieID.Hex(),
			"target_type":            targetType,
			"target_id":              targetID,
			"target_title":           c.TargetTitle,
			"target_slug":            targetSlug,
			"target_url":             targetURL,
			"user_id":                c.UserID.Hex(),
			"parent_id":              pointerToString(c.ParentID),
			"content":                c.Content,
			"status":                 c.Status,
			"has_blocked_word":       c.HasBlockedWord,
			"has_link":               c.HasLink,
			"created_at":             c.CreatedAt,
			"updated_at":             c.UpdatedAt,
			"user_display_name":      userDisplayName,
			"user_avatar_url":        c.UserAvatarURL,
			"user_role":              c.UserRole,
			"user_is_premium":        c.UserIsPremium,
			"user_is_premium_active": c.UserIsPremiumActive,
			"user": gin.H{
				"id":                c.UserID.Hex(),
				"display_name":      userDisplayName,
				"username":          c.UserUsername,
				"avatar_url":        c.UserAvatarURL,
				"role":              c.UserRole,
				"is_premium":        c.UserIsPremium,
				"is_premium_active": c.UserIsPremiumActive,
			},
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"data":        result,
		"total":       total,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
	})
}

// AdminUpdateCommentStatus godoc
// PATCH /api/v1/admin/comments/:id/status
func (h *CommentHandler) AdminUpdateCommentStatus(c *gin.Context) {
	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	var req struct {
		Status string `json:"status"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status is required"})
		return
	}

	if req.Status != "approved" && req.Status != "pending" && req.Status != "rejected" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	err = h.commentService.UpdateCommentStatus(commentID, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update comment status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "status updated"})
}

// AdminDeleteComment godoc
// DELETE /api/v1/admin/comments/:id
func (h *CommentHandler) AdminDeleteComment(c *gin.Context) {
	commentIDStr := c.Param("id")
	commentID, err := primitive.ObjectIDFromHex(commentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid comment id"})
		return
	}

	err = h.commentService.DeleteCommentByID(commentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete comment"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "comment deleted"})
}
