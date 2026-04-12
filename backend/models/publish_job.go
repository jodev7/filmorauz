package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	PublishPlatformInstagram = "instagram"
	PublishPlatformYouTube   = "youtube"
	PublishPlatformTikTok    = "tiktok"

	PublishJobStatusPending    = "pending"
	PublishJobStatusProcessing = "processing"
	PublishJobStatusSuccess    = "success"
	PublishJobStatusFailed     = "failed"
)

// PublishJob represents a single scheduled social-media publish task.
// One job = one platform + one account. Multiple jobs are created when admin
// picks several platforms/accounts at once.
type PublishJob struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"         json:"id"`
	ClipID       primitive.ObjectID `bson:"clip_id"               json:"clip_id"`
	ClipURL      string             `bson:"clip_url"              json:"clip_url"`
	MovieTitle   string             `bson:"movie_title"           json:"movie_title"`
	MovieSlug    string             `bson:"movie_slug"            json:"movie_slug"`
	MovieCode    string             `bson:"movie_code"            json:"movie_code"`
	Platform     string             `bson:"platform"              json:"platform"`     // instagram | youtube | tiktok
	AccountName  string             `bson:"account_name"          json:"account_name"`
	ScheduledFor time.Time          `bson:"scheduled_for"         json:"scheduled_for"`
	Status       string             `bson:"status"                json:"status"`
	CreatedBy    string             `bson:"created_by"            json:"created_by"`
	CreatedAt    time.Time          `bson:"created_at"            json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"            json:"updated_at"`
	ExecutedAt   *time.Time         `bson:"executed_at,omitempty" json:"executed_at,omitempty"`
	Error        string             `bson:"error,omitempty"       json:"error,omitempty"`
}
