package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// NotificationType represents the type of notification
type NotificationType string

const (
	NotificationPremiumActivated    NotificationType = "PREMIUM_ACTIVATED"
	NotificationPremiumExpiringSoon NotificationType = "PREMIUM_EXPIRING_SOON"
	NotificationPremiumExpired      NotificationType = "PREMIUM_EXPIRED"
	NotificationBanApplied          NotificationType = "BAN_APPLIED"
	NotificationBanRemoved          NotificationType = "BAN_REMOVED"
	NotificationAppealSubmitted     NotificationType = "APPEAL_SUBMITTED"
	NotificationAppealApproved      NotificationType = "APPEAL_APPROVED"
	NotificationAppealRejected      NotificationType = "APPEAL_REJECTED"
	NotificationCommentReply        NotificationType = "COMMENT_REPLY"
	NotificationCommentLike         NotificationType = "COMMENT_LIKE"
)

// Notification represents a user notification
type Notification struct {
	ID        primitive.ObjectID     `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID     `bson:"user_id" json:"user_id"`
	Type      NotificationType       `bson:"type" json:"type"`
	Title     string                 `bson:"title" json:"title"`
	Message   string                 `bson:"message" json:"message"`
	IsRead    bool                   `bson:"is_read" json:"is_read"`
	Data      map[string]interface{} `bson:"data,omitempty" json:"data,omitempty"`
	ActionURL string                 `bson:"action_url,omitempty" json:"action_url,omitempty"`
	CreatedAt time.Time              `bson:"created_at" json:"created_at"`
	ReadAt    *time.Time             `bson:"read_at,omitempty" json:"read_at,omitempty"`
}

// NotificationData contains structured data for specific notification types
type NotificationData struct {
	// For PREMIUM_ACTIVATED
	PremiumExpiresAt *time.Time `json:"premium_expires_at,omitempty"`

	// For PREMIUM_EXPIRING_SOON
	DaysUntilExpiry int `json:"days_until_expiry,omitempty"`

	// For COMMENT_REPLY
	MovieID     string `json:"movie_id,omitempty"`
	MovieSlug   string `json:"movie_slug,omitempty"`
	CommentID   string `json:"comment_id,omitempty"`
	ReplyID     string `json:"reply_id,omitempty"`
	ReplierID   string `json:"replier_id,omitempty"`
	ReplierName string `json:"replier_name,omitempty"`

	// For BAN_APPLIED/BAN_REMOVED
	BanReason    string `json:"ban_reason,omitempty"`
	BanDuration  string `json:"ban_duration,omitempty"`
	BannedByName string `json:"banned_by_name,omitempty"`

	// For APPEAL_SUBMITTED/APPEAL_APPROVED/REJECTED
	AppealID  string `json:"appeal_id,omitempty"`
	AdminNote string `json:"admin_note,omitempty"`
}

// NotificationResponse represents the API response for notifications
type NotificationResponse struct {
	Notifications []Notification `json:"notifications"`
	Total         int64          `json:"total"`
	UnreadCount   int64          `json:"unread_count"`
}

// NotificationCreateRequest is used internally to create notifications
type NotificationCreateRequest struct {
	UserID    primitive.ObjectID
	Type      NotificationType
	Title     string
	Message   string
	ActionURL string
	Data      map[string]interface{}
}

// MarkReadRequest represents marking a notification as read
type MarkReadRequest struct {
	IDs []string `json:"ids"`
}
