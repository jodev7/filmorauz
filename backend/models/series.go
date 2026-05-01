package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Series represents a TV series
type Series struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code,omitempty" json:"code,omitempty"`
	Slug        string             `bson:"slug" json:"slug"` // URL-friendly slug
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	PosterURL   string             `bson:"poster_url" json:"poster_url"`
	BackdropURL string             `bson:"backdrop_url" json:"backdrop_url"`
	Year        int                `bson:"year" json:"year"`
	Genre       []string           `bson:"genre" json:"genre"`
	Country     string             `bson:"country" json:"country"`
	Views       int64              `bson:"views" json:"views"`               // View counter
	RatingAvg   float64            `bson:"rating_avg" json:"rating_avg"`     // Average rating (1-5)
	RatingCount int64              `bson:"rating_count" json:"rating_count"` // Number of ratings
	IsPremium   bool               `bson:"is_premium" json:"is_premium"`     // Premium content flag
	IsCompleted bool               `bson:"is_completed" json:"is_completed"` // Series completed flag
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`

	// Approval workflow — new series start as pending/unpublished
	ApprovalStatus string     `bson:"approval_status,omitempty" json:"approval_status,omitempty"` // "pending" | "approved" | "rejected"
	IsPublished    bool       `bson:"is_published" json:"is_published"`
	ApprovedAt     *time.Time `bson:"approved_at,omitempty" json:"approved_at,omitempty"`
	ApprovedBy     string     `bson:"approved_by,omitempty" json:"approved_by,omitempty"`
}

// Season represents a season in a series
type Season struct {
	ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SeriesID     primitive.ObjectID `bson:"series_id" json:"series_id"`
	SeasonNumber int                `bson:"season_number" json:"season_number"`
	Title        string             `bson:"title" json:"title"`           // e.g., "Season 1" or custom title
	PosterURL    string             `bson:"poster_url" json:"poster_url"` // Season-specific poster
	Description  string             `bson:"description" json:"description"`
	ReleaseDate  time.Time          `bson:"release_date" json:"release_date"`
	CreatedAt    time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt    time.Time          `bson:"updated_at" json:"updated_at"`
}

// Episode represents an episode in a season
type Episode struct {
	ID            primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	SeriesID      primitive.ObjectID `bson:"series_id" json:"series_id"`
	SeasonID      primitive.ObjectID `bson:"season_id" json:"season_id"`
	EpisodeNumber int                `bson:"episode_number" json:"episode_number"`
	Title         string             `bson:"title" json:"title"`
	Description   string             `bson:"description" json:"description"`
	ThumbnailURL  string             `bson:"thumbnail_url" json:"thumbnail_url"`
	VideoURL      string             `bson:"video_url" json:"video_url"`
	EmbedURL      string             `bson:"embed_url" json:"embed_url"`
	SourceType    VideoSourceType    `bson:"source_type,omitempty" json:"source_type,omitempty"`
	Duration      int                `bson:"duration" json:"duration"` // minutes
	Views         int64              `bson:"views" json:"views"`       // View counter
	AirDate       time.Time          `bson:"air_date" json:"air_date"`
	CreatedAt     time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt     time.Time          `bson:"updated_at" json:"updated_at"`

	// Hover-preview thumbnails — see Movie.ThumbnailsBaseURL.
	ThumbnailsBaseURL string `bson:"thumbnails_base_url,omitempty" json:"thumbnails_base_url,omitempty"`
	ThumbnailInterval int    `bson:"thumbnail_interval,omitempty" json:"thumbnail_interval,omitempty"`
}

// SeasonWithEpisodes combines a season with its episodes
type SeasonWithEpisodes struct {
	Season   Season    `json:"season"`
	Episodes []Episode `json:"episodes"`
}

// SeriesWithSeasons combines a series with its seasons
type SeriesWithSeasons struct {
	Series  Series               `json:"series"`
	Seasons []SeasonWithEpisodes `json:"seasons"`
}

// SeriesInput is used for create/update requests.
// Note: PosterURL is NOT marked required here so the same struct can be used
// for partial updates (edit flow preserves existing poster when unchanged).
// CreateSeries can enforce PosterURL at the handler level if needed.
type SeriesInput struct {
	Title       string   `json:"title" binding:"required"`
	Description string   `json:"description"`
	PosterURL   string   `json:"poster_url"`
	BackdropURL string   `json:"backdrop_url"`
	Year        int      `json:"year" binding:"required"`
	Genre       []string `json:"genre"`
	Country     string   `json:"country"`
	IsPremium   bool     `json:"is_premium"`
	IsCompleted bool     `json:"is_completed"`
	Slug        string   `json:"slug"`
}

// SeasonInput is used for create/update requests
type SeasonInput struct {
	Title       string    `json:"title" binding:"required"`
	PosterURL   string    `json:"poster_url"`
	Description string    `json:"description"`
	ReleaseDate time.Time `json:"release_date"`
}

// EpisodeInput is used for create/update requests
type EpisodeInput struct {
	Title        string          `json:"title" binding:"required"`
	Description  string          `json:"description"`
	ThumbnailURL string          `json:"thumbnail_url"`
	VideoURL     string          `json:"video_url"`
	EmbedURL     string          `json:"embed_url"`
	SourceType   VideoSourceType `json:"source_type"`
	Duration     int             `json:"duration"`
	AirDate      string          `json:"air_date"`
}
