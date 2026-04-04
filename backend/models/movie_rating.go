package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// MovieRating represents a user's rating for a movie
type MovieRating struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MovieID   primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Rating    int                `bson:"rating" json:"rating"` // 1-5
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}

// RatingInput is used for creating/updating ratings
type RatingInput struct {
	Rating int `json:"rating" binding:"required,min=1,max=5"`
}

// RatingSummary represents the aggregate rating data for a movie
type RatingSummary struct {
	RatingAvg   float64 `json:"rating_avg"`
	RatingCount int64   `json:"rating_count"`
	UserRating  *int    `json:"user_rating,omitempty"` // nil if user hasn't rated
}
