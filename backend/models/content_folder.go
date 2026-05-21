package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ContentFolder groups admin-edited clips (e.g. CapCut exports) for later
// scheduled publishing. Distinct from Clip — that one is auto-generated from
// movies/series; this one is human-curated and not tied to any movie/series.
type ContentFolder struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title       string             `bson:"title" json:"title"`
	Slug        string             `bson:"slug" json:"slug"`
	PosterURL   string             `bson:"poster_url,omitempty" json:"poster_url,omitempty"`
	Description string             `bson:"description,omitempty" json:"description,omitempty"`
	SortOrder   int                `bson:"sort_order" json:"sort_order"`
	ClipsCount  int                `bson:"clips_count" json:"clips_count"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
}

// ContentClip is a single video file uploaded into a ContentFolder, plus the
// admin-authored caption and aggregate upload status used by the dashboard.
type ContentClip struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	FolderID    primitive.ObjectID `bson:"folder_id" json:"folder_id"`
	Title       string             `bson:"title" json:"title"`
	Filename    string             `bson:"filename" json:"filename"`
	Path        string             `bson:"path" json:"path"`
	URL         string             `bson:"url" json:"url"`
	Caption     string             `bson:"caption,omitempty" json:"caption,omitempty"`
	StorageType string             `bson:"storage_type" json:"storage_type"`
	Size        int64              `bson:"size,omitempty" json:"size,omitempty"`
	Duration    int                `bson:"duration,omitempty" json:"duration,omitempty"`

	UploadCount      int        `bson:"upload_count" json:"upload_count"`
	LastUploadStatus string     `bson:"last_upload_status" json:"last_upload_status"` // "" | "success" | "failed"
	LastUploadAt     *time.Time `bson:"last_upload_at,omitempty" json:"last_upload_at,omitempty"`

	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}
