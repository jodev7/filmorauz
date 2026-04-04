package services

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CommentWithUserDTO is used for API responses
type CommentWithUserDTO struct {
	ID                  string               `json:"id"`
	MovieID             string               `json:"movie_id,omitempty"`
	TargetType          string               `json:"target_type,omitempty"`
	TargetID            string               `json:"target_id,omitempty"`
	UserID              string               `json:"user_id"`
	ParentID            *string              `json:"parent_id,omitempty"`
	Content             string               `json:"content"`
	Status              string               `json:"status"`
	HasBlockedWord      bool                 `json:"has_blocked_word"`
	HasLink             bool                 `json:"has_link"`
	CreatedAt           time.Time            `json:"created_at"`
	UpdatedAt           time.Time            `json:"updated_at"`
	UserDisplayName     string               `json:"user_display_name"`
	UserAvatarURL       string               `json:"user_avatar_url,omitempty"`
	UserRole            string               `json:"user_role"`
	UserIsPremium       bool                 `json:"user_is_premium"`
	UserIsPremiumActive bool                 `json:"user_is_premium_active"`
	RepliesCount        int                  `json:"replies_count,omitempty"`
	Replies             []CommentWithUserDTO `json:"replies,omitempty"`
}

type CommentService struct {
	commentRepo *repositories.CommentRepository
	userRepo    *repositories.UserRepository
}

func NewCommentService(commentRepo *repositories.CommentRepository, userRepo *repositories.UserRepository) *CommentService {
	return &CommentService{
		commentRepo: commentRepo,
		userRepo:    userRepo,
	}
}

// CreateComment creates a new comment with moderation checks
// For backward compatibility, if targetType is empty, it uses movie_id
func (s *CommentService) CreateComment(movieID, userID primitive.ObjectID, content string, parentID *primitive.ObjectID, targetType models.CommentTargetType, targetID primitive.ObjectID) (*models.MovieComment, error) {
	// Get moderation settings
	settings, err := s.commentRepo.GetModerationSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to get moderation settings")
	}

	// Check if comments are globally enabled
	if !settings.CommentsEnabled {
		return nil, fmt.Errorf("comments are currently disabled")
	}

	// Validate content
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	// Check max comment length
	maxLen := settings.MaxCommentLength
	if maxLen <= 0 {
		maxLen = 2000 // Default
	}
	if len(content) > maxLen {
		return nil, fmt.Errorf("content must not exceed %d characters", maxLen)
	}
	if len(content) < 1 {
		return nil, fmt.Errorf("content is too short")
	}

	// Check cooldown (rate limiting)
	if settings.CommentCooldownSeconds > 0 {
		lastComment, err := s.commentRepo.GetLastCommentByUser(userID)
		if err == nil && lastComment != nil {
			cooldownEnd := lastComment.CreatedAt.Add(time.Duration(settings.CommentCooldownSeconds) * time.Second)
			if time.Now().Before(cooldownEnd) {
				remaining := cooldownEnd.Sub(time.Now()).Seconds()
				return nil, fmt.Errorf("please wait %.0f seconds before commenting again", remaining)
			}
		}
	}

	// Check rate limiting (max comments per minute)
	if settings.MaxCommentsPerMinute > 0 {
		count, err := s.commentRepo.CountRecentCommentsByUser(userID, settings.MaxCommentsPerMinute)
		if err == nil && count >= int64(settings.MaxCommentsPerMinute) {
			return nil, fmt.Errorf("you've reached the maximum comment limit. please wait a minute")
		}
	}

	// Check if this is a reply
	if parentID != nil {
		// Check if replies are enabled
		if !settings.RepliesEnabled {
			return nil, fmt.Errorf("replies are currently disabled")
		}

		// Check max reply depth
		depth, err := s.commentRepo.GetCommentDepth(*parentID)
		if err == nil && depth >= settings.MaxReplyDepth {
			return nil, fmt.Errorf("maximum reply depth reached. you cannot reply to this comment")
		}
	}

	// Run moderation checks
	hasBlockedWord, blockedWord := s.checkBannedWords(content, settings.BannedWords)
	hasLink := s.checkLinks(content)

	// Determine status
	status := models.CommentStatusApproved
	if settings.RequireModeration || hasBlockedWord || (hasLink && settings.BlockLinks) {
		status = models.CommentStatusPending
	}

	// If auto-hide is enabled and banned word found, mark as hidden too
	if hasBlockedWord && settings.AutoHideBannedContent {
		status = models.CommentStatusPending
	}

	// Capture user's premium status at comment creation time (for priority sorting)
	isPremiumUser := false
	if s.userRepo != nil {
		_, isActive, err := s.userRepo.GetUserPremiumStatus(userID)
		if err == nil {
			isPremiumUser = isActive
		}
	}

	// Create comment
	comment := &models.MovieComment{
		MovieID:        movieID,
		TargetType:     targetType,
		TargetID:       targetID,
		UserID:         userID,
		ParentID:       parentID,
		Content:        content,
		Status:         status,
		HasBlockedWord: hasBlockedWord,
		HasLink:        hasLink,
		IsPremiumUser:  isPremiumUser,
	}

	if blockedWord != "" {
		_ = blockedWord // Could be logged or stored for admin review
	}

	err = s.commentRepo.Create(comment)
	if err != nil {
		return nil, fmt.Errorf("failed to create comment: %w", err)
	}

	return comment, nil
}

// UpdateComment updates an existing comment
func (s *CommentService) UpdateComment(commentID, userID primitive.ObjectID, content string) (*models.MovieComment, error) {
	// Get existing comment
	comment, err := s.commentRepo.GetByID(commentID)
	if err != nil {
		return nil, fmt.Errorf("comment not found")
	}

	// Check ownership
	if comment.UserID != userID {
		return nil, fmt.Errorf("not authorized to edit this comment")
	}

	// Validate content
	content = strings.TrimSpace(content)
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}
	if len(content) > 2000 {
		return nil, fmt.Errorf("content must not exceed 2000 characters")
	}

	// Get moderation settings
	settings, err := s.commentRepo.GetModerationSettings()
	if err != nil {
		return nil, fmt.Errorf("failed to get moderation settings")
	}

	// Re-run moderation checks
	hasBlockedWord, _ := s.checkBannedWords(content, settings.BannedWords)
	hasLink := s.checkLinks(content)

	// Update comment
	comment.Content = content
	comment.HasBlockedWord = hasBlockedWord
	comment.HasLink = hasLink

	// If was pending, keep pending; if was approved, re-evaluate
	if comment.Status == models.CommentStatusApproved {
		if hasBlockedWord || (hasLink && settings.BlockLinks) {
			comment.Status = models.CommentStatusPending
		}
	}

	err = s.commentRepo.Update(comment)
	if err != nil {
		return nil, fmt.Errorf("failed to update comment: %w", err)
	}

	return comment, nil
}

// DeleteComment deletes a comment
func (s *CommentService) DeleteComment(commentID, userID primitive.ObjectID) error {
	comment, err := s.commentRepo.GetByID(commentID)
	if err != nil {
		return fmt.Errorf("comment not found")
	}

	// Check ownership
	if comment.UserID != userID {
		return fmt.Errorf("not authorized to delete this comment")
	}

	return s.commentRepo.Delete(commentID)
}

// GetCommentsByMovie returns approved comments for a movie
func (s *CommentService) GetCommentsByMovie(movieID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	// Validate movieID
	if movieID.IsZero() {
		return nil, fmt.Errorf("invalid movie id")
	}

	comments, err := s.commentRepo.GetByMovieID(movieID, limit, skip)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	// Get replies for each comment
	for i := range comments {
		replies, err := s.commentRepo.GetReplies(comments[i].ID)
		if err == nil {
			// Append replies to a special field - handled in handler
			// For now, we just get the count
			_ = replies
		}
	}

	return comments, nil
}

// GetCommentsByTarget returns approved comments for a target (movie or episode)
func (s *CommentService) GetCommentsByTarget(targetType models.CommentTargetType, targetID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	if targetID.IsZero() {
		return nil, fmt.Errorf("invalid target id")
	}

	if targetType == "" {
		targetType = models.CommentTargetMovie
	}

	comments, err := s.commentRepo.GetByTarget(targetType, targetID, limit, skip)
	if err != nil {
		return nil, fmt.Errorf("failed to get comments: %w", err)
	}

	return comments, nil
}

// GetAllApprovedCommentsByTarget returns all approved comments for a target (including nested replies)
func (s *CommentService) GetAllApprovedCommentsByTarget(targetType models.CommentTargetType, targetID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	if targetID.IsZero() {
		return nil, fmt.Errorf("invalid target id")
	}

	if targetType == "" {
		targetType = models.CommentTargetMovie
	}

	return s.commentRepo.GetAllApprovedByTarget(targetType, targetID, limit, skip)
}

// GetReplies returns replies for a parent comment
func (s *CommentService) GetReplies(parentID primitive.ObjectID) ([]models.CommentWithUser, error) {
	if parentID.IsZero() {
		return nil, fmt.Errorf("invalid parent comment id")
	}

	return s.commentRepo.GetReplies(parentID)
}

// GetAllApprovedComments returns all approved comments for a movie (no parent filter)
func (s *CommentService) GetAllApprovedComments(movieID primitive.ObjectID, limit, skip int) ([]models.CommentWithUser, error) {
	if movieID.IsZero() {
		return nil, fmt.Errorf("invalid movie id")
	}

	return s.commentRepo.GetAllApprovedByMovieID(movieID, limit, skip)
}

// GetCommentByID returns a comment by ID (used when creating replies)
func (s *CommentService) GetCommentByID(commentID primitive.ObjectID) (*models.MovieComment, error) {
	if commentID.IsZero() {
		return nil, fmt.Errorf("invalid comment id")
	}

	return s.commentRepo.GetByID(commentID)
}

// Admin: GetCommentsByStatus returns comments by status
func (s *CommentService) GetCommentsByStatus(status string, page, limit int) ([]models.CommentWithUser, int64, error) {
	if page < 1 {
		page = 1
	}
	if limit < 1 || limit > 100 {
		limit = 20
	}

	return s.commentRepo.GetByStatus(status, page, limit)
}

// Admin: UpdateCommentStatus updates comment status
func (s *CommentService) UpdateCommentStatus(commentID primitive.ObjectID, status string) error {
	if status != models.CommentStatusApproved && status != models.CommentStatusPending && status != models.CommentStatusRejected {
		return fmt.Errorf("invalid status")
	}

	return s.commentRepo.UpdateStatus(commentID, status)
}

// Admin: DeleteCommentByID deletes a comment by ID (admin can delete any)
func (s *CommentService) DeleteCommentByID(commentID primitive.ObjectID) error {
	return s.commentRepo.Delete(commentID)
}

// Admin: GetModerationSettings returns moderation settings
func (s *CommentService) GetModerationSettings() (*models.CommentModerationSettings, error) {
	return s.commentRepo.GetModerationSettings()
}

// Admin: UpdateModerationSettings updates moderation settings
func (s *CommentService) UpdateModerationSettings(settings *models.CommentModerationSettings) error {
	if settings.BlockLinks && settings.BannedWords == nil {
		settings.BannedWords = []string{}
	}
	// Ensure banned words are lowercase for case-insensitive matching
	for i, w := range settings.BannedWords {
		settings.BannedWords[i] = strings.ToLower(w)
	}

	return s.commentRepo.UpdateModerationSettings(settings)
}

// checkBannedWords checks if content contains banned words
func (s *CommentService) checkBannedWords(content string, bannedWords []string) (bool, string) {
	if len(bannedWords) == 0 {
		return false, ""
	}

	contentLower := strings.ToLower(content)

	for _, word := range bannedWords {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" {
			continue
		}

		// Simple word matching - check if word appears as standalone or part of word
		// For more strict matching, use word boundaries
		if strings.Contains(contentLower, word) {
			return true, word
		}
	}

	return false, ""
}

// checkLinks checks if content contains URLs
func (s *CommentService) checkLinks(content string) bool {
	// Common URL patterns
	linkPatterns := []string{
		`https?://`,    // http:// or https://
		`www\.`,        // www.
		`\.[a-z]{2,}/`, // .com/, .net/, etc
		`t\.me/`,       // Telegram.me
		`telegram\.`,   // telegram.com
		`instagram\.`,  // instagram.com
		`facebook\.`,   // facebook.com
		`twitter\.`,    // twitter.com
		`vk\.com`,      // vk.com
	}

	for _, pattern := range linkPatterns {
		matched, err := regexp.MatchString(pattern, content)
		if err == nil && matched {
			return true
		}
	}

	return false
}
