package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// BanHistory represents a ban history record
type BanHistory struct {
	ID                 primitive.ObjectID  `bson:"_id,omitempty" json:"id"`
	UserID             primitive.ObjectID  `bson:"user_id" json:"user_id"`
	Reason             string              `bson:"reason" json:"reason"`
	BannedAt           time.Time           `bson:"banned_at" json:"banned_at"`
	BannedUntil        *time.Time          `bson:"banned_until,omitempty" json:"banned_until,omitempty"`
	IsPermanent        bool                `bson:"is_permanent" json:"is_permanent"`
	BannedByUserID     primitive.ObjectID  `bson:"banned_by_user_id" json:"banned_by_user_id"`
	BannedByUsername   string              `bson:"banned_by_username" json:"banned_by_username"`
	UnbannedAt         *time.Time          `bson:"unbanned_at,omitempty" json:"unbanned_at,omitempty"`
	UnbannedByUserID   *primitive.ObjectID `bson:"unbanned_by_user_id,omitempty" json:"unbanned_by_user_id,omitempty"`
	UnbannedByUsername *string             `bson:"unbanned_by_username,omitempty" json:"unbanned_by_username,omitempty"`
	CreatedAt          time.Time           `bson:"created_at" json:"created_at"`
}

// IsActive returns true if the ban is currently active
func (bh *BanHistory) IsActive() bool {
	// Already unbanned
	if bh.UnbannedAt != nil {
		return false
	}
	// Permanent ban is always active until unbanned
	if bh.IsPermanent {
		return true
	}
	// Check if temporary ban has expired
	if bh.BannedUntil == nil {
		return false
	}
	return time.Now().Before(*bh.BannedUntil)
}

// IsExpired returns true if the ban has expired naturally
func (bh *BanHistory) IsExpired() bool {
	// Already unbanned is not expired
	if bh.UnbannedAt != nil {
		return false
	}
	// Permanent bans never expire
	if bh.IsPermanent {
		return false
	}
	// Check if temporary ban has expired
	if bh.BannedUntil == nil {
		return false
	}
	return time.Now().After(*bh.BannedUntil)
}

// GetStatus returns the current status of the ban
func (bh *BanHistory) GetStatus() string {
	if bh.UnbannedAt != nil {
		return "unbanned"
	}
	if bh.IsExpired() {
		return "expired"
	}
	return "active"
}
