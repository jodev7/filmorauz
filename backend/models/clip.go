package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

type Clip struct {
	ID primitive.ObjectID `bson:"_id,omitempty" json:"id"`

	// ContentKind is "movie" (default, also assumed when absent for legacy
	// docs) or "series". It decides which set of linkage fields below is
	// populated.
	ContentKind string `bson:"content_kind,omitempty" json:"content_kind,omitempty"`

	// Movie linkage (kind == "movie")
	MovieID    primitive.ObjectID `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	MovieTitle string             `bson:"movie_title,omitempty" json:"movie_title,omitempty"`
	MovieSlug  string             `bson:"movie_slug,omitempty" json:"movie_slug,omitempty"`
	MovieCode  string             `bson:"movie_code,omitempty" json:"movie_code,omitempty"`

	// Series/episode linkage (kind == "series")
	SeriesID      primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeriesTitle   string             `bson:"series_title,omitempty" json:"series_title,omitempty"`
	SeriesSlug    string             `bson:"series_slug,omitempty" json:"series_slug,omitempty"`
	SeasonID      primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	SeasonNumber  int                `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumber int                `bson:"episode_number,omitempty" json:"episode_number,omitempty"`
	EpisodeID     primitive.ObjectID `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	Title         string             `bson:"title,omitempty" json:"title,omitempty"`
	ClipIndex     int                `bson:"clip_index,omitempty" json:"clip_index,omitempty"`
	SourceType    string             `bson:"source_type,omitempty" json:"source_type,omitempty"`

	Filename    string    `bson:"filename" json:"filename"`
	Path        string    `bson:"path" json:"path"`
	URL         string    `bson:"url" json:"url"`
	Duration    int       `bson:"duration" json:"duration"`
	Sequence    int       `bson:"sequence" json:"sequence"`
	StorageType string    `bson:"storage_type" json:"storage_type"`
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`

	// AI (Gemini) generated post text — populated only on the AI viral clip
	// path. Caption is the catchy hook; Hashtags each start with '#'. Used
	// directly as the Instagram caption (see BuildClipCaptionAI).
	Caption  string   `bson:"caption,omitempty" json:"caption,omitempty"`
	Hashtags []string `bson:"hashtags,omitempty" json:"hashtags,omitempty"`

	// Instagram upload tracking (shared by movie and series clips — the
	// existing per-clip upload flow works identically regardless of kind).
	UploadedToInstagram       bool       `bson:"uploaded_to_instagram" json:"uploaded_to_instagram"`
	InstagramUploadCount      int        `bson:"instagram_upload_count" json:"instagram_upload_count"`
	LastInstagramUploadAt     *time.Time `bson:"last_instagram_upload_at,omitempty" json:"last_instagram_upload_at,omitempty"`
	LastInstagramUploadStatus string     `bson:"last_instagram_upload_status" json:"last_instagram_upload_status"` // "success" | "failed" | ""
}

func (c *Clip) DisplayTitle() string {
	if c == nil {
		return ""
	}
	if c.MovieTitle != "" {
		return c.MovieTitle
	}
	if c.Title != "" {
		return c.Title
	}
	if c.SeriesTitle != "" {
		return c.SeriesTitle
	}
	return ""
}

func (c *Clip) DisplaySlug() string {
	if c == nil {
		return ""
	}
	if c.MovieSlug != "" {
		return c.MovieSlug
	}
	return c.SeriesSlug
}

type ClipResult struct {
	MovieID      interface{} `json:"movie_id"`
	Code         string      `json:"code"`
	Slug         string      `json:"slug"`
	DisplayTitle string      `json:"display_title"`
}
