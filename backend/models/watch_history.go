package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WatchHistory represents a user's watch history entry
type WatchHistory struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"user_id" json:"user_id"`
	MovieID         primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	LastPositionSec int64              `bson:"last_position_sec" json:"last_position_sec"`
	DurationSec     int64              `bson:"duration_sec" json:"duration_sec"`
	ProgressPercent float64            `bson:"progress_percent" json:"progress_percent"`
	Completed       bool               `bson:"completed" json:"completed"`
	StartedAt       time.Time          `bson:"started_at" json:"started_at"`
	LastWatchedAt   time.Time          `bson:"last_watched_at" json:"last_watched_at"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}

// WatchProgressRequest represents the request body for saving watch progress
type WatchProgressRequest struct {
	PositionSec int64 `json:"positionSec" binding:"required,min=0"`
	DurationSec int64 `json:"durationSec" binding:"required,min=1"`
}

// ContinueWatchingItem represents a continue watching item for frontend
type ContinueWatchingItem struct {
	MovieID         string    `json:"movie_id"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	PosterURL       string    `json:"poster_url"`
	LastPositionSec int64     `json:"last_position_sec"`
	DurationSec     int64     `json:"duration_sec"`
	ProgressPercent float64   `json:"progress_percent"`
	LastWatchedAt   time.Time `json:"last_watched_at"`
}

// WatchHistoryWithMovie represents watch history with movie details for frontend
type WatchHistoryWithMovie struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	MovieID   primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	WatchedAt time.Time          `bson:"watched_at" json:"watched_at"`
	// Movie details (matching aggregation output)
	Title      string `bson:"title" json:"title"`
	PosterURL  string `bson:"poster_url" json:"poster_url"`
	Slug       string `bson:"slug" json:"slug"`
	Code       string `bson:"code" json:"code"`
	Year       int    `bson:"year" json:"year"`
	Quality    string `bson:"quality" json:"quality"`
	WebsiteURL string `bson:"website_url" json:"website_url"`
}
