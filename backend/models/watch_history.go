package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// WatchHistory represents a user's watch history entry.
type WatchHistory struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID          primitive.ObjectID `bson:"user_id" json:"user_id"`
	TargetType      string             `bson:"target_type,omitempty" json:"target_type,omitempty"`
	TargetID        primitive.ObjectID `bson:"target_id,omitempty" json:"target_id,omitempty"`
	MovieID         primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"` // Legacy fallback for movies
	SeriesID        primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID        primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	EpisodeID       primitive.ObjectID `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	LastPositionSec int64              `bson:"last_position_sec" json:"last_position_sec"`
	DurationSec     int64              `bson:"duration_sec" json:"duration_sec"`
	ProgressPercent float64            `bson:"progress_percent" json:"progress_percent"`
	Completed       bool               `bson:"completed" json:"completed"`
	WatchedAt       time.Time          `bson:"watched_at" json:"watched_at"`
	StartedAt       time.Time          `bson:"started_at" json:"started_at"`
	LastWatchedAt   time.Time          `bson:"last_watched_at" json:"last_watched_at"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}

// WatchProgressRequest represents the request body for saving watch progress
type WatchProgressRequest struct {
	PositionSec int64  `json:"positionSec" binding:"min=0"`
	DurationSec int64  `json:"durationSec" binding:"min=1"`
	CurrentTime int64  `json:"current_time" binding:"min=0"`
	Duration    int64  `json:"duration" binding:"min=1"`
	TargetType  string `json:"target_type"`
	TargetID    string `json:"target_id"`
}

type WatchProgressResponse struct {
	CurrentTime     int64   `json:"current_time"`
	Duration        int64   `json:"duration"`
	ProgressPercent float64 `json:"progress_percent"`
	Completed       bool    `json:"completed"`
}

// ContinueWatchingItem represents a continue watching item for frontend
type ContinueWatchingItem struct {
	MovieID         string    `json:"movie_id,omitempty"`
	TargetType      string    `json:"target_type"`
	TargetID        string    `json:"target_id"`
	EpisodeID       string    `json:"episode_id,omitempty"`
	Title           string    `json:"title"`
	Slug            string    `json:"slug"`
	PosterURL       string    `json:"poster_url"`
	Type            string    `json:"type,omitempty"`
	SeriesTitle     string    `json:"series_title,omitempty"`
	SeriesSlug      string    `json:"series_slug,omitempty"`
	SeasonNumber    int       `json:"season_number,omitempty"`
	EpisodeNumber   int       `json:"episode_number,omitempty"`
	EpisodeTitle    string    `json:"episode_title,omitempty"`
	LastPositionSec int64     `json:"last_position_sec"`
	DurationSec     int64     `json:"duration_sec"`
	ProgressPercent float64   `json:"progress_percent"`
	LastWatchedAt   time.Time `json:"last_watched_at"`
}

// WatchHistoryListItem represents watch history enriched for frontend display.
type WatchHistoryListItem struct {
	ID              primitive.ObjectID `bson:"_id" json:"id"`
	RecordID        primitive.ObjectID `bson:"record_id" json:"record_id"`
	MovieID         primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	TargetType      string             `bson:"target_type" json:"target_type"`
	TargetID        primitive.ObjectID `bson:"target_id" json:"target_id"`
	SeriesID        primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID        primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	EpisodeID       primitive.ObjectID `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	WatchedAt       time.Time          `bson:"watched_at" json:"watched_at"`
	LastPositionSec int64              `bson:"last_position_sec" json:"last_position_sec"`
	DurationSec     int64              `bson:"duration_sec" json:"duration_sec"`
	ProgressPercent float64            `bson:"progress_percent" json:"progress_percent"`
	Completed       bool               `bson:"completed" json:"completed"`
	Title           string             `bson:"title" json:"title"`
	PosterURL       string             `bson:"poster_url" json:"poster_url"`
	BackdropURL     string             `bson:"backdrop_url,omitempty" json:"backdrop_url,omitempty"`
	Slug            string             `bson:"slug" json:"slug"`
	Code            string             `bson:"code" json:"code"`
	Year            int                `bson:"year" json:"year"`
	Quality         string             `bson:"quality" json:"quality"`
	WebsiteURL      string             `bson:"website_url" json:"website_url"`
	Type            string             `bson:"type" json:"type"`
	SeriesTitle     string             `bson:"series_title,omitempty" json:"series_title,omitempty"`
	SeriesSlug      string             `bson:"series_slug,omitempty" json:"series_slug,omitempty"`
	SeasonNumber    int                `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumber   int                `bson:"episode_number,omitempty" json:"episode_number,omitempty"`
	EpisodeTitle    string             `bson:"episode_title,omitempty" json:"episode_title,omitempty"`
}
