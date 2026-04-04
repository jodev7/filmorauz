package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Favorite represents a user's favorite movie
type Favorite struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	MovieID   primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// FavoriteWithMovie represents favorite with movie details for frontend
type FavoriteWithMovie struct {
	ID        primitive.ObjectID `bson:"_id" json:"id"`
	MovieID   primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	// Movie details (matching aggregation output)
	Title      string `bson:"title" json:"title"`
	PosterURL  string `bson:"poster_url" json:"poster_url"`
	Slug       string `bson:"slug" json:"slug"`
	Code       string `bson:"code" json:"code"`
	Year       int    `bson:"year" json:"year"`
	Quality    string `bson:"quality" json:"quality"`
	WebsiteURL string `bson:"website_url" json:"website_url"`
}
