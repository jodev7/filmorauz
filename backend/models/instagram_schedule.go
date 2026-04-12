package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	InstagramScheduleStatusPending    = "pending"
	InstagramScheduleStatusProcessing = "processing"
	InstagramScheduleStatusSuccess    = "success"
	InstagramScheduleStatusFailed     = "failed"
)

// InstagramSchedule represents a scheduled Instagram Reel upload.
// ScheduledFor is stored in UTC; the Asia/Tashkent offset is applied at the API boundary.
type InstagramSchedule struct {
	ID           primitive.ObjectID `bson:"_id,omitempty"         json:"id"`
	ClipID       primitive.ObjectID `bson:"clip_id"               json:"clip_id"`
	ClipURL      string             `bson:"clip_url"              json:"clip_url"`
	MovieTitle   string             `bson:"movie_title"           json:"movie_title"`
	MovieSlug    string             `bson:"movie_slug"            json:"movie_slug"`
	MovieCode    string             `bson:"movie_code"            json:"movie_code"`
	AccountNames []string           `bson:"account_names"         json:"account_names"`
	ScheduledFor time.Time          `bson:"scheduled_for"         json:"scheduled_for"`
	Status       string             `bson:"status"                json:"status"`
	CreatedBy    string             `bson:"created_by"            json:"created_by"`
	CreatedAt    time.Time          `bson:"created_at"            json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"            json:"updated_at"`
	ExecutedAt   *time.Time         `bson:"executed_at,omitempty" json:"executed_at,omitempty"`
	Error        string             `bson:"error,omitempty"       json:"error,omitempty"`
}
