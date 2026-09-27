package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// CastMember is one actor on a movie/series, as fetched from TMDB.
type CastMember struct {
	Name       string `bson:"name" json:"name"`
	Character  string `bson:"character,omitempty" json:"character,omitempty"`
	ProfileURL string `bson:"profile_url,omitempty" json:"profile_url,omitempty"`
	TMDBID     int    `bson:"tmdb_id,omitempty" json:"tmdb_id,omitempty"`
}

// Person is an actor/director seen on the site (collection "people"),
// used for photos on the person page.
type Person struct {
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	TMDBID     int                `bson:"tmdb_id" json:"tmdb_id"`
	Name       string             `bson:"name" json:"name"`
	NameLower  string             `bson:"name_lower" json:"-"`
	ProfileURL string             `bson:"profile_url,omitempty" json:"profile_url,omitempty"`
	Department string             `bson:"department,omitempty" json:"department,omitempty"`
	UpdatedAt  time.Time          `bson:"updated_at" json:"updated_at"`
}
