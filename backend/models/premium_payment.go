package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// TelegramStarsPayment records a premium subscription payment made via Telegram Stars.
// telegram_payment_charge_id is unique to provide idempotency against duplicate webhooks.
type TelegramStarsPayment struct {
	ID                      primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID                  primitive.ObjectID `bson:"user_id" json:"user_id"`
	TelegramID              int64              `bson:"telegram_id" json:"telegram_id"`
	SessionToken            string             `bson:"session_token,omitempty" json:"session_token,omitempty"`
	Package                 string             `bson:"package" json:"package"`
	StarsAmount             int                `bson:"stars_amount" json:"stars_amount"`
	DurationMonths          int                `bson:"duration_months" json:"duration_months"`
	TelegramPaymentChargeID string             `bson:"telegram_payment_charge_id" json:"telegram_payment_charge_id"`
	ProviderPaymentChargeID string             `bson:"provider_payment_charge_id" json:"provider_payment_charge_id"`
	Status                  string             `bson:"status" json:"status"`
	CreatedAt               time.Time          `bson:"created_at" json:"created_at"`
}
