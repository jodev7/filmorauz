package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Clip struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	MovieID     primitive.ObjectID `bson:"movie_id" json:"movie_id"`
	MovieTitle  string             `bson:"movie_title" json:"movie_title"`
	MovieSlug   string             `bson:"movie_slug" json:"movie_slug"`
	MovieCode   string             `bson:"movie_code" json:"movie_code"`
	Filename    string             `bson:"filename" json:"filename"`
	Path        string             `bson:"path" json:"path"`
	URL         string             `bson:"url" json:"url"`
	Duration    int                `bson:"duration" json:"duration"`
	Sequence    int                `bson:"sequence" json:"sequence"`
	StorageType string             `bson:"storage_type" json:"storage_type"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`

	// Instagram upload tracking
	UploadedToInstagram       bool       `bson:"uploaded_to_instagram" json:"uploaded_to_instagram"`
	InstagramUploadCount      int        `bson:"instagram_upload_count" json:"instagram_upload_count"`
	LastInstagramUploadAt     *time.Time `bson:"last_instagram_upload_at,omitempty" json:"last_instagram_upload_at,omitempty"`
	LastInstagramUploadStatus string     `bson:"last_instagram_upload_status" json:"last_instagram_upload_status"` // "success" | "failed" | ""
}

type ClipResult struct {
	MovieID      interface{} `json:"movie_id"`
	Code         string      `json:"code"`
	Slug         string      `json:"slug"`
	DisplayTitle string      `json:"display_title"`
}
