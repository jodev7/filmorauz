package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// SeriesRating represents a user's rating for a series
type SeriesRating struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SeriesID  primitive.ObjectID `bson:"series_id" json:"series_id"`
	UserID    primitive.ObjectID `bson:"user_id" json:"user_id"`
	Rating    int                `bson:"rating" json:"rating"` // 1-5
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
}
