package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BanAppealStatus represents the status of a ban appeal
type BanAppealStatus string

const (
	BanAppealStatusPending  BanAppealStatus = "pending"
	BanAppealStatusApproved BanAppealStatus = "approved"
	BanAppealStatusRejected BanAppealStatus = "rejected"
)

// BanAppeal represents a user's appeal against a ban
type BanAppeal struct {
	ID                 primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID             primitive.ObjectID `bson:"user_id" json:"user_id"`
	BanHistoryID       primitive.ObjectID `bson:"ban_history_id,omitempty" json:"ban_history_id,omitempty"`
	Username           string             `bson:"username" json:"username"`
	TelegramID         string             `bson:"telegram_id" json:"telegram_id"`
	Message            string             `bson:"message" json:"message"`
	Status             BanAppealStatus    `bson:"status" json:"status"`
	AdminNote          string             `bson:"admin_note,omitempty" json:"admin_note,omitempty"`
	CreatedAt          time.Time          `bson:"created_at" json:"created_at"`
	ReviewedAt         *time.Time         `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	ReviewedByUserID   primitive.ObjectID `bson:"reviewed_by_user_id,omitempty" json:"reviewed_by_user_id,omitempty"`
	ReviewedByUsername string             `bson:"reviewed_by_username,omitempty" json:"reviewed_by_username,omitempty"`
	// Link to ban info for display
	BanReason       string `bson:"ban_reason,omitempty" json:"ban_reason,omitempty"`
	BanBannedAt     string `bson:"ban_banned_at,omitempty" json:"ban_banned_at,omitempty"`
	BanBannedUntil  string `bson:"ban_banned_until,omitempty" json:"ban_banned_until,omitempty"`
	BanBannedByName string `bson:"ban_banned_by_name,omitempty" json:"ban_banned_by_name,omitempty"`
}

// IsPending checks if the appeal is still pending
func (ba *BanAppeal) IsPending() bool {
	return ba.Status == BanAppealStatusPending
}

// IsProcessed checks if the appeal has been reviewed
func (ba *BanAppeal) IsProcessed() bool {
	return ba.Status == BanAppealStatusApproved || ba.Status == BanAppealStatusRejected
}

// CreateBanAppealRequest represents the request to create a ban appeal
type CreateBanAppealRequest struct {
	Message string `json:"message" binding:"required,min=10,max=2000"`
}

// ReviewBanAppealRequest represents the request to review a ban appeal
type ReviewBanAppealRequest struct {
	Action    string `json:"action" binding:"required,oneof=approve reject"`
	AdminNote string `json:"admin_note"`
	UnbanUser bool   `json:"unban_user"`
}

// GetBanAppealsResponse represents the response for getting ban appeals
type GetBanAppealsResponse struct {
	Appeals    []BanAppeal `json:"appeals"`
	Total      int64       `json:"total"`
	Page       int         `json:"page"`
	PerPage    int         `json:"per_page"`
	TotalPages int         `json:"total_pages"`
}

// GetMyAppealsResponse represents the response for getting a user's appeals
type GetMyAppealsResponse struct {
	Appeals []BanAppeal `json:"appeals"`
	Total   int64       `json:"total"`
}
