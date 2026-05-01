package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	CommentStatusApproved = "approved"
	CommentStatusPending  = "pending"
	CommentStatusRejected = "rejected"
)

// CommentTargetType represents the type of content a comment belongs to
type CommentTargetType string

const (
	CommentTargetMovie   CommentTargetType = "movie"
	CommentTargetEpisode CommentTargetType = "episode"
)

// MovieComment represents a comment on a movie or episode
type MovieComment struct {
	ID             primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	MovieID        primitive.ObjectID  `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	TargetType     CommentTargetType   `bson:"target_type,omitempty" json:"target_type,omitempty"`
	TargetID       primitive.ObjectID  `bson:"target_id,omitempty" json:"target_id,omitempty"`
	UserID         primitive.ObjectID  `bson:"user_id" json:"user_id"`
	ParentID       *primitive.ObjectID `bson:"parent_id,omitempty" json:"parent_id,omitempty"`
	Content        string              `bson:"content" json:"content"`
	Status         string              `bson:"status" json:"status"`
	HasBlockedWord bool                `bson:"has_blocked_word" json:"has_blocked_word"`
	HasLink        bool                `bson:"has_link" json:"has_link"`
	CreatedAt      time.Time           `bson:"created_at" json:"created_at"`
	UpdatedAt      time.Time           `bson:"updated_at" json:"updated_at"`
	// Snapshot of author premium status at comment creation time (for priority sorting)
	IsPremiumUser bool                 `bson:"is_premium_user" json:"is_premium_user"`
	LikedBy       []primitive.ObjectID `bson:"liked_by,omitempty" json:"liked_by,omitempty"`
	LikesCount    int                  `bson:"likes_count" json:"likes_count"`
}

// CommentWithUser combines comment with user info for API responses
type CommentWithUser struct {
	ID             primitive.ObjectID  `json:"id"`
	MovieID        primitive.ObjectID  `json:"movie_id,omitempty"`
	TargetType     CommentTargetType   `json:"target_type,omitempty"`
	TargetID       primitive.ObjectID  `json:"target_id,omitempty"`
	UserID         primitive.ObjectID  `json:"user_id"`
	ParentID       *primitive.ObjectID `json:"parent_id,omitempty"`
	Content        string              `json:"content"`
	Status         string              `json:"status"`
	HasBlockedWord bool                `json:"has_blocked_word"`
	HasLink        bool                `json:"has_link"`
	CreatedAt      time.Time           `json:"created_at"`
	UpdatedAt      time.Time           `json:"updated_at"`
	// Snapshot of author premium status at comment creation time (for priority sorting)
	IsPremiumUser bool `json:"is_premium_user"`
	// User info
	UserDisplayName     string `json:"user_display_name"`
	UserUsername        string `json:"user_username,omitempty"`
	UserAvatarURL       string `json:"user_avatar_url,omitempty"`
	UserRole            string `json:"user_role"`
	UserIsPremium       bool   `json:"user_is_premium"`
	UserIsPremiumActive bool   `json:"user_is_premium_active"`
	// Target info for admin
	TargetTitle string `json:"target_title,omitempty"`
	// Replies count (for top-level comments)
	RepliesCount int `json:"replies_count,omitempty"`
	// Like data
	LikesCount int                  `json:"likes_count"`
	LikedByMe  bool                 `json:"liked_by_me"`
	LikedBy    []primitive.ObjectID `json:"-"`
}

// CommentModerationSettings holds global moderation settings
type CommentModerationSettings struct {
	ID                     string             `bson:"_id" json:"id"`
	CommentsEnabled        bool               `bson:"comments_enabled" json:"comments_enabled"`
	RepliesEnabled         bool               `bson:"replies_enabled" json:"replies_enabled"`
	BlockLinks             bool               `bson:"block_links" json:"block_links"`
	MaxCommentLength       int                `bson:"max_comment_length" json:"max_comment_length"`
	MaxReplyDepth          int                `bson:"max_reply_depth" json:"max_reply_depth"`
	RequireModeration      bool               `bson:"require_moderation" json:"require_moderation"`
	BannedWords            []string           `bson:"banned_words" json:"banned_words"`
	AutoHideBannedContent  bool               `bson:"auto_hide_banned_content" json:"auto_hide_banned_content"`
	CommentCooldownSeconds int                `bson:"comment_cooldown_seconds" json:"comment_cooldown_seconds"`
	MaxCommentsPerMinute   int                `bson:"max_comments_per_minute" json:"max_comments_per_minute"`
	DefaultSort            string             `bson:"default_sort" json:"default_sort"`
	UpdatedAt              time.Time          `bson:"updated_at" json:"updated_at"`
	UpdatedBy              primitive.ObjectID `bson:"updated_by,omitempty" json:"updated_by,omitempty"`
}

// GetDefaultModerationSettings returns the default settings
func GetDefaultModerationSettings() *CommentModerationSettings {
	return &CommentModerationSettings{
		ID:                     "global",
		CommentsEnabled:        true,
		RepliesEnabled:         true,
		BlockLinks:             true,
		MaxCommentLength:       2000,
		MaxReplyDepth:          3,
		RequireModeration:      false,
		BannedWords:            []string{"spam", "casino", "xxx", "porn", "hentai", "milfporn"},
		AutoHideBannedContent:  true,
		CommentCooldownSeconds: 30,
		MaxCommentsPerMinute:   5,
		DefaultSort:            "newest",
		UpdatedAt:              time.Now(),
	}
}
