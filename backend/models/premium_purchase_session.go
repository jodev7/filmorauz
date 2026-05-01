package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PremiumPurchaseSession struct {
	ID                      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Token                   string             `bson:"token" json:"token"`
	UserID                  primitive.ObjectID `bson:"user_id" json:"user_id"`
	TelegramID              int64              `bson:"telegram_id,omitempty" json:"telegram_id,omitempty"`
	Package                 string             `bson:"package" json:"package"`
	Status                  string             `bson:"status" json:"status"`
	ExpiresAt               time.Time          `bson:"expires_at" json:"expires_at"`
	CompletedAt             *time.Time         `bson:"completed_at,omitempty" json:"completed_at,omitempty"`
	TelegramPaymentChargeID string             `bson:"telegram_payment_charge_id,omitempty" json:"telegram_payment_charge_id,omitempty"`
	CreatedAt               time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt               time.Time          `bson:"updated_at" json:"updated_at"`
}

const (
	PremiumPurchaseSessionStatusPending   = "pending"
	PremiumPurchaseSessionStatusPaid      = "paid"
	PremiumPurchaseSessionStatusExpired   = "expired"
	PremiumPurchaseSessionStatusCancelled = "cancelled"
)
