package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Favorite represents a user's favorite content target.
type Favorite struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID     primitive.ObjectID `bson:"user_id" json:"user_id"`
	TargetType string             `bson:"target_type,omitempty" json:"target_type,omitempty"`
	TargetID   primitive.ObjectID `bson:"target_id,omitempty" json:"target_id,omitempty"`
	MovieID    primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"` // Legacy fallback for movie favorites
	SeriesID   primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID   primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}

// FavoriteListItem represents a favorite enriched for frontend display.
type FavoriteListItem struct {
	ID            primitive.ObjectID `bson:"_id" json:"id"`
	RecordID      primitive.ObjectID `bson:"record_id" json:"record_id"`
	TargetType    string             `bson:"target_type" json:"target_type"`
	TargetID      primitive.ObjectID `bson:"target_id" json:"target_id"`
	MovieID       primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	SeriesID      primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID      primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	EpisodeID     primitive.ObjectID `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	Title         string             `bson:"title" json:"title"`
	PosterURL     string             `bson:"poster_url" json:"poster_url"`
	BackdropURL   string             `bson:"backdrop_url,omitempty" json:"backdrop_url,omitempty"`
	Slug          string             `bson:"slug" json:"slug"`
	Code          string             `bson:"code" json:"code"`
	Year          int                `bson:"year" json:"year"`
	Quality       string             `bson:"quality" json:"quality"`
	WebsiteURL    string             `bson:"website_url" json:"website_url"`
	Type          string             `bson:"type" json:"type"`
	SeriesTitle   string             `bson:"series_title,omitempty" json:"series_title,omitempty"`
	SeriesSlug    string             `bson:"series_slug,omitempty" json:"series_slug,omitempty"`
	SeasonNumber  int                `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumber int                `bson:"episode_number,omitempty" json:"episode_number,omitempty"`
	EpisodeTitle  string             `bson:"episode_title,omitempty" json:"episode_title,omitempty"`
}
