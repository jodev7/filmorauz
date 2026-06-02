package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Share represents a movie share link
type Share struct {
	ID               primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code             string             `bson:"code" json:"code"` // Short unique code
	// ContentKind discriminates "movie" (default/legacy when empty) vs "series".
	ContentKind      string             `bson:"content_kind,omitempty" json:"content_kind,omitempty"`
	MovieID          primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	SeriesID         primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	CreatedByUserID  primitive.ObjectID `bson:"created_by_user_id,omitempty" json:"created_by_user_id,omitempty"`
	CreatedByUserHex string             `bson:"created_by_user_hex,omitempty" json:"created_by_user_hex,omitempty"`
	Source           string             `bson:"source" json:"source"` // "web" or "telegram"
	Clicks           int64              `bson:"clicks" json:"clicks"`
	CreatedAt        time.Time          `bson:"created_at" json:"created_at"`
	LastClickedAt    *time.Time         `bson:"last_clicked_at,omitempty" json:"last_clicked_at,omitempty"`
}

// ShareSource constants
const (
	ShareSourceWeb      = "web"
	ShareSourceTelegram = "telegram"
)

// ShareStats represents share statistics
type ShareStats struct {
	SharesCreatedCount int64 `json:"shares_created_count"`
	TotalShareOpens    int64 `json:"total_share_opens"`
}

// UserShareStats represents user share statistics
type UserShareStats struct {
	TotalSharesCreated int64           `json:"total_shares_created"`
	TotalShareOpens    int64           `json:"total_share_opens"`
	TotalMoviesShared  int64           `json:"total_movies_shared"`
	TopSharedMovie     *TopSharedMovie `json:"top_shared_movie,omitempty"`
}

// TopSharedMovie represents the most shared movie by a user
type TopSharedMovie struct {
	MovieID    string `json:"movie_id"`
	Title      string `json:"title"`
	ShareCount int64  `json:"share_count"`
}

// AdminShareStats represents admin-level share statistics
type AdminShareStats struct {
	TotalSharesCreated int64                `json:"total_shares_created"`
	TotalShareOpens    int64                `json:"total_share_opens"`
	TopSharedMovies    []TopSharedMovieStat `json:"top_shared_movies"`
	TopUsersByShares   []TopUserShareStat   `json:"top_users_by_shares"`
	RecentShares       []Share              `json:"recent_shares,omitempty"`
}

// TopSharedMovieStat represents share stats for a movie
type TopSharedMovieStat struct {
	MovieID            string `json:"movie_id"`
	Title              string `json:"title"`
	SharesCreatedCount int64  `json:"shares_created_count"`
	TotalShareOpens    int64  `json:"total_share_opens"`
}

// TopUserShareStat represents share stats for a user
type TopUserShareStat struct {
	UserID             string `json:"user_id"`
	DisplayName        string `json:"display_name"`
	SharesCreatedCount int64  `json:"shares_created_count"`
}
