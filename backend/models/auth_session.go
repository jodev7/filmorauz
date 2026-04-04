package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type AuthSession struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code" json:"code"`     // 8-char unique code
	Status      string             `bson:"status" json:"status"` // "pending", "completed", "expired"
	UserID      primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	TelegramID  int64              `bson:"telegram_id,omitempty" json:"telegram_id,omitempty"`
	ExpiresAt   time.Time          `bson:"expires_at" json:"expires_at"`
	CompletedAt time.Time          `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// Auth session statuses
const (
	AuthSessionStatusPending   = "pending"
	AuthSessionStatusCompleted = "completed"
	AuthSessionStatusExpired   = "expired"
)

// AuthSessionRequest is the request body for completing an auth session from the bot
type AuthSessionRequest struct {
	Code       string `json:"code" binding:"required"`
	TelegramID int64  `json:"telegram_id" binding:"required"`
	Username   string `json:"username"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
	PhotoURL   string `json:"photo_url"`
}
