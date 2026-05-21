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
	// ContentKind is "movie" (default) or "series". Captured at job
	// creation so the caption builder picks the correct lead text,
	// code label, and hashtag stack without a DB round-trip at publish
	// time. Legacy rows missing the field are treated as "movie".
	ContentKind  string             `bson:"content_kind,omitempty" json:"content_kind,omitempty"`
	Platform     string             `bson:"platform"              json:"platform"`     // instagram | youtube | tiktok
	AccountName  string             `bson:"account_name"          json:"account_name"`
	ScheduledFor time.Time          `bson:"scheduled_for"         json:"scheduled_for"`
	Status       string             `bson:"status"                json:"status"`
	CreatedBy    string             `bson:"created_by"            json:"created_by"`
	CreatedAt    time.Time          `bson:"created_at"            json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at"            json:"updated_at"`
	ExecutedAt   *time.Time         `bson:"executed_at,omitempty" json:"executed_at,omitempty"`
	Error        string             `bson:"error,omitempty"       json:"error,omitempty"`
	// Instagram-specific publish tracking. Populated when a successful
	// upload response is received OR when a sidecar success is recovered
	// after a proxy timeout. Once set, MarkFailed must NOT clear them.
	InstagramMediaID string     `bson:"instagram_media_id,omitempty" json:"instagram_media_id,omitempty"`
	InstagramPostURL string     `bson:"instagram_post_url,omitempty" json:"instagram_post_url,omitempty"`
	PublishedAt      *time.Time `bson:"published_at,omitempty"       json:"published_at,omitempty"`
	RetryCount       int        `bson:"retry_count,omitempty"        json:"retry_count,omitempty"`

	// SourceKind distinguishes movie/series-derived clips ("clip", default
	// for legacy rows) from admin-curated content folder clips ("content").
	// When SourceKind == "content", ContentClipID + ContentFolderID identify
	// the source and CaptionOverride is the literal caption to use — the
	// caption builder's movie/series template is skipped.
	SourceKind      string             `bson:"source_kind,omitempty"        json:"source_kind,omitempty"`
	ContentClipID   primitive.ObjectID `bson:"content_clip_id,omitempty"    json:"content_clip_id,omitempty"`
	ContentFolderID primitive.ObjectID `bson:"content_folder_id,omitempty"  json:"content_folder_id,omitempty"`
	CaptionOverride string             `bson:"caption_override,omitempty"   json:"caption_override,omitempty"`
}
