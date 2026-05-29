package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Expense is a manually recorded project cost entered by a superadmin
// (e.g. a VPS invoice, a domain renewal, a one-off purchase). It lives
// alongside the automatically tracked clip_ai_usage spend; the expenses
// dashboard sums both to show total project cost.
type Expense struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	// Category groups spend on the dashboard. Free-form, but the UI offers
	// a few presets: "vps", "domain", "ai", "marketing", "other".
	Category string `bson:"category" json:"category"`

	// Title is a short human label ("Hetzner VPS — May", "Domain renewal").
	Title string `bson:"title" json:"title"`

	// AmountUSD is the cost in USD.
	AmountUSD float64 `bson:"amount_usd" json:"amount_usd"`

	// Recurring marks ongoing costs (monthly VPS) vs one-off purchases.
	Recurring bool `bson:"recurring" json:"recurring"`

	Note string `bson:"note,omitempty" json:"note,omitempty"`

	// IncurredAt is when the cost was actually incurred (admin-supplied);
	// CreatedAt is when the row was entered.
	IncurredAt time.Time `bson:"incurred_at" json:"incurred_at"`
	CreatedBy  string    `bson:"created_by,omitempty" json:"created_by,omitempty"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
}
