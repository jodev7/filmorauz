package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Collection represents a curated collection/playlist of movies
type Collection struct {
	ID          primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	Title       string               `bson:"title" json:"title"`
	Slug        string               `bson:"slug" json:"slug"`
	Description string               `bson:"description" json:"description"`
	PosterURL   string               `bson:"poster_url" json:"poster_url"`
	IsPublished bool                 `bson:"is_published" json:"is_published"`
	IsFeatured  bool                 `bson:"is_featured" json:"is_featured"`
	SortOrder   int                  `bson:"sort_order" json:"sort_order"`
	MovieIDs    []primitive.ObjectID `bson:"movie_ids" json:"movie_ids"`
	SeriesIDs   []primitive.ObjectID `bson:"series_ids,omitempty" json:"series_ids,omitempty"`
	CreatedAt   time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time            `bson:"updated_at" json:"updated_at"`
}

// CollectionInput is used for create/update requests
type CollectionInput struct {
	Title       string   `json:"title" binding:"required"`
	Slug        string   `json:"slug" binding:"required"`
	Description string   `json:"description"`
	PosterURL   string   `json:"poster_url"`
	IsPublished bool     `json:"is_published"`
	IsFeatured  bool     `json:"is_featured"`
	SortOrder   int      `json:"sort_order"`
	MovieIDs    []string `json:"movie_ids"`  // Movie IDs as strings for JSON input
	SeriesIDs   []string `json:"series_ids"` // Series IDs as strings for JSON input
}

// CollectionWithMovies represents a collection with populated movie data
type CollectionWithMovies struct {
	ID          string       `json:"id"`
	Title       string       `json:"title"`
	Slug        string       `json:"slug"`
	Description string       `json:"description"`
	PosterURL   string       `json:"poster_url"`
	SortOrder   int           `json:"sort_order"`
	Movies      []MovieBasic  `json:"movies"`
	Series      []SeriesBasic `json:"series"`
}

// SeriesBasic is a minimal series representation for collection responses
type SeriesBasic struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	PosterURL   string   `json:"poster_url"`
	Year        int      `json:"year"`
	Genre       []string `json:"genre"`
	Genres      []string `json:"genres"`
	Quality     string   `json:"quality"`
	RatingAvg   float64  `json:"rating_avg"`
	RatingCount int64    `json:"rating_count"`
	IsPremium   bool     `json:"is_premium"`
	CreatedAt   string   `json:"created_at"`
}

// MovieBasic is a minimal movie representation for collection responses
type MovieBasic struct {
	ID          string   `json:"id"`
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	PosterURL   string   `json:"poster_url"`
	Year        int      `json:"year"`
	Genre       []string `json:"genre"`
	Genres      []string `json:"genres"`
	Duration    int      `json:"duration"`
	Quality     string   `json:"quality"`
	RatingAvg   float64  `json:"rating_avg"`
	RatingCount int64    `json:"rating_count"`
	IsPremium   bool     `json:"is_premium"`
	CreatedAt   string   `json:"created_at"`
}
