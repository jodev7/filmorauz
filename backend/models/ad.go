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
	ImageURL     string `bson:"image_url,omitempty" json:"image_url,omitempty"`
	VideoURL     string `bson:"video_url,omitempty" json:"video_url,omitempty"`
	TargetURL    string `bson:"target_url" json:"target_url"`
	CallToAction string `bson:"call_to_action,omitempty" json:"call_to_action,omitempty"`

	// Website creatives (structured media)
	BannerMediaURL         string `bson:"banner_media_url,omitempty" json:"banner_media_url,omitempty"`
	BannerMediaType        string `bson:"banner_media_type,omitempty" json:"banner_media_type,omitempty"`
	InlineMediaURL         string `bson:"inline_media_url,omitempty" json:"inline_media_url,omitempty"`
	InlineMediaType        string `bson:"inline_media_type,omitempty" json:"inline_media_type,omitempty"`
	FixedBottomMediaURL    string `bson:"fixed_bottom_media_url,omitempty" json:"fixed_bottom_media_url,omitempty"`
	FixedBottomMediaType   string `bson:"fixed_bottom_media_type,omitempty" json:"fixed_bottom_media_type,omitempty"`
	PopupMediaURL          string `bson:"popup_media_url,omitempty" json:"popup_media_url,omitempty"`
	PopupMediaType         string `bson:"popup_media_type,omitempty" json:"popup_media_type,omitempty"`
	PlayerOverlayMediaURL  string `bson:"player_overlay_media_url,omitempty" json:"player_overlay_media_url,omitempty"`
	PlayerOverlayMediaType string `bson:"player_overlay_media_type,omitempty" json:"player_overlay_media_type,omitempty"`

	// Telegram shared media
	TelegramMediaURL  string `bson:"telegram_media_url,omitempty" json:"telegram_media_url,omitempty"`
	TelegramMediaType string `bson:"telegram_media_type,omitempty" json:"telegram_media_type,omitempty"`

	// Placement — which page slots this ad appears on
	Placements []string `bson:"placements" json:"placements"`

	// Status
	Status AdStatus `bson:"status" json:"status"`

	// Scheduling
	StartsAt     *time.Time `bson:"starts_at,omitempty" json:"starts_at,omitempty"`
	EndsAt       *time.Time `bson:"ends_at,omitempty" json:"ends_at,omitempty"`
	DurationDays int        `bson:"duration_days,omitempty" json:"duration_days,omitempty"`

	// Revenue tracking (manual price set by superadmin)
	Price    float64 `bson:"price" json:"price"`       // in USD
	Priority int     `bson:"priority" json:"priority"` // higher = shown first in rotation

	// Stats (atomic increments)
	Impressions int64 `bson:"impressions" json:"impressions"`
	Clicks      int64 `bson:"clicks" json:"clicks"`

	// Phase 2: Telegram targeting
	TelegramChannels       []string   `bson:"telegram_channels,omitempty" json:"telegram_channels,omitempty"`
	TelegramBotEnabled     bool       `bson:"telegram_bot_enabled" json:"telegram_bot_enabled"`
	TelegramBotChatIDs     []int64    `bson:"telegram_bot_chat_ids,omitempty" json:"telegram_bot_chat_ids,omitempty"` // explicit target chat IDs for bot delivery
	TelegramChannelEnabled bool       `bson:"telegram_channel_enabled" json:"telegram_channel_enabled"`
	TelegramDeliveries     int64      `bson:"telegram_deliveries" json:"telegram_deliveries"`
	TelegramLastSentAt     *time.Time `bson:"telegram_last_sent_at,omitempty" json:"telegram_last_sent_at,omitempty"`

	// Phase 2: Player targeting
	PlayerEnabled bool `bson:"player_enabled" json:"player_enabled"`

	// Metadata
	CreatedBy primitive.ObjectID `bson:"created_by" json:"created_by"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// AdDelivery is a lightweight delivery log record (stored in ad_deliveries collection)
type AdDelivery struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	AdID      primitive.ObjectID `bson:"ad_id" json:"ad_id"`
	Placement string             `bson:"placement" json:"placement"` // telegram_channel_post, telegram_bot_message, player_overlay_banner, etc.
	Target    string             `bson:"target" json:"target"`       // channel username, "bot", or "player"
	Status    string             `bson:"status" json:"status"`       // "success" | "failed"
	MessageID int                `bson:"message_id,omitempty" json:"message_id,omitempty"`
	SentAt    time.Time          `bson:"sent_at" json:"sent_at"`
	Error     string             `bson:"error,omitempty" json:"error,omitempty"`
}

// AdRotationState persists the last-shown index per placement for sequential round-robin rotation.
type AdRotationState struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Placement string             `bson:"placement" json:"placement"`
	LastIndex int                `bson:"last_index" json:"last_index"`
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
	// Phase 2
	TelegramDeliveries int64 `json:"telegram_deliveries"`
	TelegramFailed     int64 `json:"telegram_failed"`
}
