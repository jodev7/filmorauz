package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// VideoSourceType defines the type of video source
type VideoSourceType string

const (
	VideoSourceIframeEmbed        VideoSourceType = "iframe_embed"
	VideoSourceDirectMP4          VideoSourceType = "direct_mp4"
	VideoSourceDirectHLS          VideoSourceType = "direct_hls"
	VideoSourceExternalRestricted VideoSourceType = "external_restricted"
)

type Movie struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Code        string             `bson:"code" json:"code"`               // Unique alphanumeric code (6-8 chars), converted from legacy numeric
	Slug        string             `bson:"slug" json:"slug"`               // URL-friendly slug
	WebsiteURL  string             `bson:"website_url" json:"website_url"` // Full website link
	Title       string             `bson:"title" json:"title"`
	Description string             `bson:"description" json:"description"`
	PosterURL   string             `bson:"poster_url" json:"poster_url"`
	BackdropURL string             `bson:"backdrop_url" json:"backdrop_url"`
	Year        int                `bson:"year" json:"year"`
	Genre       []string           `bson:"genre" json:"genre"`
	Country     string             `bson:"country" json:"country"`
	VideoURL    string             `bson:"video_url" json:"video_url"`
	EmbedURL    string             `bson:"embed_url" json:"embed_url"`
	SourceType  VideoSourceType    `bson:"source_type" json:"source_type"`
	Duration    int                `bson:"duration" json:"duration"`         // minutes
	Quality     string             `bson:"quality" json:"quality"`           // "720p", "1080p", etc.
	Views       int64              `bson:"views" json:"views"`               // View counter
	RatingAvg   float64            `bson:"rating_avg" json:"rating_avg"`     // Average rating (1-5)
	RatingCount int64              `bson:"rating_count" json:"rating_count"` // Number of ratings
	IsPremium   bool               `bson:"is_premium" json:"is_premium"`     // Premium content flag
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`

	// HLS Streaming Support
	MasterPlaylistURL  string   `bson:"master_playlist_url,omitempty" json:"master_playlist_url,omitempty"` // CDN URL to master.m3u8
	GeneratedQualities []string `bson:"generated_qualities,omitempty" json:"generated_qualities,omitempty"` // Available qualities ["480p", "720p", "1080p"]
	DefaultQuality     string   `bson:"default_quality,omitempty" json:"default_quality,omitempty"`         // Quality to start with (e.g., "480p")
	SourceResolution   string   `bson:"source_resolution,omitempty" json:"source_resolution,omitempty"`     // Original source resolution (e.g., "1920x1080")

	// LOCALIZATION: Uzbek display fields (preferred for /uz pages)
	TitleUz       string   `bson:"title_uz,omitempty" json:"title_uz,omitempty"`
	DescriptionUz string   `bson:"description_uz,omitempty" json:"description_uz,omitempty"`
	GenresUz      []string `bson:"genres_uz,omitempty" json:"genres_uz,omitempty"`
	CountriesUz   []string `bson:"countries_uz,omitempty" json:"countries_uz,omitempty"`

	// Original TMDB metadata for reference
	OriginalTitle  string `bson:"original_title,omitempty" json:"original_title,omitempty"`
	TMDBID         int    `bson:"tmdb_id,omitempty" json:"tmdb_id,omitempty"`
	MetadataSource string `bson:"metadata_source,omitempty" json:"metadata_source,omitempty"`

	// Approval workflow — new imports start as pending/unpublished
	ApprovalStatus string     `bson:"approval_status,omitempty" json:"approval_status,omitempty"` // "pending" | "approved" | "rejected"
	IsPublished    bool       `bson:"is_published" json:"is_published"`
	ApprovedAt     *time.Time `bson:"approved_at,omitempty" json:"approved_at,omitempty"`
	ApprovedBy     string     `bson:"approved_by,omitempty" json:"approved_by,omitempty"`
}

// MovieInput is used for create/update requests.
// Note: PosterURL is NOT marked required here so the same struct can be used
// for partial updates (edit flow preserves existing poster when unchanged).
// CreateMovie enforces PosterURL explicitly at the handler level.
type MovieInput struct {
	Title       string          `json:"title" binding:"required"`
	Description string          `json:"description" binding:"required"`
	PosterURL   string          `json:"poster_url"`
	BackdropURL string          `json:"backdrop_url"`
	Year        int             `json:"year" binding:"required"`
	Genre       []string        `json:"genre"`
	Country     string          `json:"country"`
	VideoURL    string          `json:"video_url"`
	EmbedURL    string          `json:"embed_url"`
	SourceType  VideoSourceType `json:"source_type"`
	Duration    int             `json:"duration"`
	Quality     string          `json:"quality"`
	IsPremium   bool            `json:"is_premium"`

	// HLS Streaming Support
	MasterPlaylistURL  string   `json:"master_playlist_url"`
	GeneratedQualities []string `json:"generated_qualities"`
	DefaultQuality     string   `json:"default_quality"`
	SourceResolution   string   `json:"source_resolution"`

	// LOCALIZATION: Uzbek display fields
	TitleUz        string   `json:"title_uz"`
	DescriptionUz  string   `json:"description_uz"`
	GenresUz       []string `json:"genres_uz"`
	CountriesUz    []string `json:"countries_uz"`
	OriginalTitle  string   `json:"original_title"`
	TMDBID         int      `json:"tmdb_id"`
	MetadataSource string   `json:"metadata_source"`
}
