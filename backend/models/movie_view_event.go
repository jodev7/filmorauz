package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MovieViewEvent represents a view event for trending calculations
// This is written only after ~15 seconds of watching
type MovieViewEvent struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MovieID   primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	UserID    primitive.ObjectID `bson:"user_id,omitempty" json:"user_id,omitempty"`
	SessionID string             `bson:"session_id" json:"session_id"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// TrendingMovie represents a movie with its view count in a specific period
type TrendingMovie struct {
	Movie         Movie `json:"movie"`
	ViewsInPeriod int64 `json:"views_in_period"`
}

// RecommendationMovie represents a movie in the recommendations response
type RecommendationMovie struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Slug      string   `json:"slug"`
	PosterURL string   `json:"poster_url"`
	Year      int      `json:"year"`
	Genres    []string `json:"genres"`
	Score     int      `json:"score,omitempty"`
}
