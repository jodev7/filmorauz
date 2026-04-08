package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// AdStatus represents the lifecycle state of an ad
type AdStatus string

const (
	AdStatusDraft   AdStatus = "draft"
	AdStatusActive  AdStatus = "active"
	AdStatusPaused  AdStatus = "paused"
	AdStatusExpired AdStatus = "expired"
)

// Ad represents an advertisement in the system
type Ad struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`

	// Display content
	ImageURL    string `bson:"image_url,omitempty" json:"image_url,omitempty"`
	VideoURL    string `bson:"video_url,omitempty" json:"video_url,omitempty"`
	TargetURL   string `bson:"target_url" json:"target_url"`
	CallToAction string `bson:"call_to_action,omitempty" json:"call_to_action,omitempty"`

	// Placement — which page slots this ad appears on
	Placements []string `bson:"placements" json:"placements"`

	// Status
	Status AdStatus `bson:"status" json:"status"`

	// Scheduling
	StartsAt     *time.Time `bson:"starts_at,omitempty" json:"starts_at,omitempty"`
	EndsAt       *time.Time `bson:"ends_at,omitempty" json:"ends_at,omitempty"`
	DurationDays int        `bson:"duration_days,omitempty" json:"duration_days,omitempty"`

	// Revenue tracking (manual price set by superadmin)
	Price float64 `bson:"price" json:"price"` // in USD

	// Stats (atomic increments)
	Impressions int64 `bson:"impressions" json:"impressions"`
	Clicks      int64 `bson:"clicks" json:"clicks"`

	// Metadata
	CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// AdStats is the aggregated stats response for the ads dashboard
type AdStats struct {
	TotalAds    int64   `json:"total_ads"`
	ActiveAds   int64   `json:"active_ads"`
	ExpiredAds  int64   `json:"expired_ads"`
	Impressions int64   `json:"impressions"`
	Clicks      int64   `json:"clicks"`
	Revenue     float64 `json:"revenue"`
}
