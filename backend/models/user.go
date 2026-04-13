package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type User struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Role        string             `bson:"role" json:"role"` // "user", "admin", "superadmin"
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
	LastLoginAt time.Time          `bson:"last_login_at" json:"last_login_at"`

	// User-editable profile fields
	DisplayName     string `bson:"display_name,omitempty" json:"display_name,omitempty"`
	ProfileImageURL string `bson:"profile_image_url,omitempty" json:"profile_image_url,omitempty"`

	// Telegram-specific fields (Telegram-only auth)
	TelegramID     int64  `bson:"telegram_id,omitempty" json:"telegram_id,omitempty"`
	TelegramChatID int64  `bson:"telegram_chat_id,omitempty" json:"telegram_chat_id,omitempty"` // chat.id from /start
	TelegramUser   string `bson:"telegram_user,omitempty" json:"telegram_user,omitempty"`       // username without @
	FirstName    string `bson:"first_name,omitempty" json:"first_name,omitempty"`
	LastName     string `bson:"last_name,omitempty" json:"last_name,omitempty"`
	PhotoURL     string `bson:"photo_url,omitempty" json:"photo_url,omitempty"`
	LanguageCode string `bson:"language_code,omitempty" json:"language_code,omitempty"`
	AuthProvider string `bson:"auth_provider" json:"auth_provider"` // "telegram" only
	IsActive     bool   `bson:"is_active" json:"is_active"`

	// Premium subscription fields
	IsPremium        bool       `bson:"is_premium" json:"is_premium"`
	PremiumStartedAt *time.Time `bson:"premium_started_at,omitempty" json:"premium_started_at,omitempty"`
	PremiumExpiresAt *time.Time `bson:"premium_expires_at,omitempty" json:"premium_expires_at,omitempty"`

	// Wallet balance in UZS (so'm)
	WalletBalance float64 `bson:"wallet_balance" json:"wallet_balance"`

	// Profile customization (premium only)
	ProfileStyle *ProfileStyle `bson:"profile_style,omitempty" json:"profile_style,omitempty"`

	// Privacy settings (premium only)
	Privacy *PrivacySettings `bson:"privacy,omitempty" json:"privacy,omitempty"`

	// Ban information
	Ban *BanInfo `bson:"ban,omitempty" json:"ban,omitempty"`
}

// ProfileStyle defines premium profile customization options
type ProfileStyle struct {
	Frame    string `bson:"frame" json:"frame"`       // "none", "gold"
	Theme    string `bson:"theme" json:"theme"`       // "default", "gold_dark"
	Gradient string `bson:"gradient" json:"gradient"` // "none", "gold", "purple"
}

// Valid profile style values
var ValidProfileStyles = map[string][]string{
	"frame":    {"none", "gold"},
	"theme":    {"default", "gold_dark"},
	"gradient": {"none", "gold", "purple"},
}

// PrivacySettings defines profile privacy options
type PrivacySettings struct {
	IsPrivate bool `bson:"is_private" json:"is_private"`
}

// BanInfo contains information about a user ban
type BanInfo struct {
	IsBanned         bool       `bson:"is_banned" json:"is_banned"`
	Reason           string     `bson:"reason,omitempty" json:"reason,omitempty"`
	BannedAt         *time.Time `bson:"banned_at,omitempty" json:"banned_at,omitempty"`
	BannedUntil      *time.Time `bson:"banned_until,omitempty" json:"banned_until,omitempty"`
	BannedByUserID   string     `bson:"banned_by_user_id,omitempty" json:"banned_by_user_id,omitempty"`
	BannedByUsername string     `bson:"banned_by_username,omitempty" json:"banned_by_username,omitempty"`
}

// IsBannedActive returns true if user is currently banned
func (u *User) IsBannedActive() bool {
	if u.Ban == nil || !u.Ban.IsBanned {
		return false
	}
	// If banned_until is nil, it's a permanent ban
	if u.Ban.BannedUntil == nil {
		return true
	}
	// Check if ban has expired
	return time.Now().Before(*u.Ban.BannedUntil)
}

// IsPremiumActive returns true if user has active premium subscription.
// A user is considered active premium if:
// - isPremium == true AND
// - premiumExpiresAt is nil (unlimited) OR premiumExpiresAt > now
func (u *User) IsPremiumActive() bool {
	if !u.IsPremium {
		return false
	}
	// nil means unlimited premium
	if u.PremiumExpiresAt == nil {
		return true
	}
	return u.PremiumExpiresAt.After(time.Now())
}

// User roles
const (
	RoleUser       = "user"
	RoleAdmin      = "admin"
	RoleSuperAdmin = "superadmin"
)

// Auth providers (Telegram-only now)
const AuthProviderTelegram = "telegram"

// CanAccessMovie checks if user can access a movie based on premium status.
// Returns true if movie is not premium OR user has active premium.
func CanAccessMovie(user *User, movie *Movie) bool {
	// Non-premium movies are accessible to everyone
	if !movie.IsPremium {
		return true
	}
	// Premium movies require active premium subscription
	if user == nil {
		return false
	}
	return user.IsPremiumActive()
}

// GetMovieAccessInfo returns access information for a movie.
func GetMovieAccessInfo(user *User, movie *Movie) map[string]interface{} {
	hasAccess := CanAccessMovie(user, movie)
	return map[string]interface{}{
		"is_premium": movie.IsPremium,
		"has_access": hasAccess,
	}
}
