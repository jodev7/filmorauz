package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IngestionStatus represents the status of an ingestion job
type IngestionStatus string

const (
	IngestionStatusPending    IngestionStatus = "pending"
	IngestionStatusProcessing IngestionStatus = "processing"
	IngestionStatusCompleted  IngestionStatus = "completed"
	IngestionStatusFailed     IngestionStatus = "failed"

	// Legacy statuses for pipeline compatibility (deprecated but still used)
	IngestionStatusParsing     IngestionStatus = "parsing"
	IngestionStatusDownloading IngestionStatus = "downloading"
	IngestionStatusUploading   IngestionStatus = "uploading"

	// NEW: Detailed pipeline statuses
	IngestionStatusDownloadFailed      IngestionStatus = "download_failed"
	IngestionStatusParsingComplete     IngestionStatus = "parsing_complete"
	IngestionStatusEnrichingMetadata   IngestionStatus = "enriching_metadata"
	IngestionStatusFindingPoster       IngestionStatus = "finding_poster"
	IngestionStatusGeneratingPoster    IngestionStatus = "generating_poster"
	IngestionStatusUploadingPoster     IngestionStatus = "uploading_poster"
	IngestionStatusGeneratingBackdrop  IngestionStatus = "generating_backdrop"
	IngestionStatusCreatingMovie       IngestionStatus = "creating_movie"
	IngestionStatusSendingNotification IngestionStatus = "sending_notification"

	// NEW: Video processing detailed statuses
	IngestionStatusCuttingVideo      IngestionStatus = "cutting_video"
	IngestionStatusRemovingWatermark IngestionStatus = "removing_watermark"
	IngestionStatusAddingLogo        IngestionStatus = "adding_logo"
	IngestionStatusHLSProcessing     IngestionStatus = "hls_processing"
	IngestionStatusFinalizingStorage IngestionStatus = "finalizing_storage"
)

// IngestionJob represents a movie ingestion job
type IngestionJob struct {
	ID                primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	MovieID           primitive.ObjectID   `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	Title             string               `bson:"title" json:"title"` // Movie title
	Source            string               `bson:"source" json:"source"`
	SourceID          string               `bson:"source_id" json:"source_id"`
	DetailURL         string               `bson:"detail_url" json:"detail_url"`
	Status            IngestionStatus      `bson:"status" json:"status"`
	Progress          int                  `bson:"progress" json:"progress"`
	Steps             JobSteps             `bson:"steps" json:"steps"`
	Logs              []IngestionLog       `bson:"logs" json:"logs"`
	Metadata          *ParsedMovieMetadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	Error             string               `bson:"error,omitempty" json:"error,omitempty"`
	LocalPath         string               `bson:"local_path,omitempty" json:"local_path,omitempty"`
	OutputPath        string               `bson:"output_path,omitempty" json:"output_path,omitempty"`
	PlaylistPath      string               `bson:"playlist_path,omitempty" json:"playlist_path,omitempty"`
	OutputMode        string               `bson:"output_mode,omitempty" json:"output_mode,omitempty"` // "development" or "production"
	SourceFileDeleted bool                 `bson:"source_file_deleted,omitempty" json:"source_file_deleted,omitempty"`
	VideoURL          string               `bson:"video_url,omitempty" json:"video_url,omitempty"`
	RetryCount        int                  `bson:"retry_count" json:"retry_count"`
	CreatedAt         time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt         time.Time            `bson:"updated_at" json:"updated_at"`
	CompletedAt       *time.Time           `bson:"completed_at,omitempty" json:"completed_at,omitempty"`

	// NEW: Enriched metadata fields
	EnrichedMetadata   *EnrichedMetadata `bson:"enriched_metadata,omitempty" json:"enriched_metadata,omitempty"`
	OriginalPosterURL  string            `bson:"original_poster_url,omitempty" json:"original_poster_url,omitempty"`
	GeneratedPosterURL string            `bson:"generated_poster_url,omitempty" json:"generated_poster_url,omitempty"`
	PosterGenerated    bool              `bson:"poster_generated" json:"poster_generated"`
	MetadataSource     string            `bson:"metadata_source" json:"metadata_source"` // "parser", "enriched", "merged"

	// Telegram notification fields
	TelegramNotified       bool       `bson:"telegram_notified" json:"telegram_notified"`
	TelegramChannelsPosted []string   `bson:"telegram_channels_posted,omitempty" json:"telegram_channels_posted,omitempty"`
	TelegramBotNotified    bool       `bson:"telegram_bot_notified" json:"telegram_bot_notified"`
	TelegramError          string     `bson:"telegram_error,omitempty" json:"telegram_error,omitempty"`
	TelegramNotifiedAt     *time.Time `bson:"telegram_notified_at,omitempty" json:"telegram_notified_at,omitempty"`

	// Movie data for Telegram (cached after creation for idempotency)
	MovieCode string `bson:"movie_code,omitempty" json:"movie_code,omitempty"`
	MovieSlug string `bson:"movie_slug,omitempty" json:"movie_slug,omitempty"`
}

// JobSteps tracks the completion status of each pipeline step
type JobSteps struct {
	Download    bool `bson:"download" json:"download"`
	Process     bool `bson:"process" json:"process"`
	Upload      bool `bson:"upload" json:"upload"`
	Metadata    bool `bson:"metadata" json:"metadata"`
	Enrich      bool `bson:"enrich" json:"enrich"`
	Poster      bool `bson:"poster" json:"poster"`
	CreateMovie bool `bson:"create_movie" json:"create_movie"`
}

// IngestionLog represents a log entry for an ingestion job
type IngestionLog struct {
	Timestamp time.Time `bson:"timestamp" json:"timestamp"`
	Message   string    `bson:"message" json:"message"`
	Level     string    `bson:"level" json:"level"` // info, warn, error
}

// ParsedMovieMetadata represents metadata extracted from the parser
type ParsedMovieMetadata struct {
	Title        string        `bson:"title" json:"title"`
	Description  string        `bson:"description" json:"description"`
	Poster       string        `bson:"poster" json:"poster"`
	Backdrop     string        `bson:"backdrop" json:"backdrop"`
	Year         int           `bson:"year" json:"year"`
	Genres       []string      `bson:"genres" json:"genres"`
	Country      string        `bson:"country" json:"country"`
	Duration     int           `bson:"duration" json:"duration"`
	VideoPageURL string        `bson:"video_page_url" json:"video_page_url"`
	VideoURLs    []VideoSource `bson:"video_urls" json:"video_urls"`
	Quality      string        `bson:"quality" json:"quality"`
	Translation  string        `bson:"translation" json:"translation"`
}

// VideoSource represents a video source
type VideoSource struct {
	Quality string `bson:"quality" json:"quality"`
	URL     string `bson:"url" json:"url"`
	Type    string `bson:"type" json:"type"` // iframe, mp4, hls, download
}

// ProcessingResult represents the result of processing
type ProcessingResult struct {
	HLSIndexURL  string `bson:"hls_index_url" json:"hls_index_url"`
	StreamingURL string `bson:"streaming_url" json:"streaming_url"`
	Quality      string `bson:"quality" json:"quality"`
	FileSize     int64  `bson:"file_size" json:"file_size"`
}

// NEW: EnrichedMetadata represents metadata from external sources
type EnrichedMetadata struct {
	Title          string   `bson:"title" json:"title"`
	OriginalTitle  string   `bson:"original_title" json:"original_title"`
	Description    string   `bson:"description" json:"description"`
	Year           int      `bson:"year" json:"year"`
	Genres         []string `bson:"genres" json:"genres"`
	Countries      []string `bson:"countries" json:"countries"`
	Duration       int      `bson:"duration" json:"duration"`
	Quality        string   `bson:"quality" json:"quality"`
	Translation    string   `bson:"translation" json:"translation"`
	PosterURL      string   `bson:"poster_url" json:"poster_url"`
	BackdropURL    string   `bson:"backdrop_url" json:"backdrop_url"`
	SourceProvider string   `bson:"source_provider" json:"source_provider"`
	SourceURL      string   `bson:"source_url" json:"source_url"`

	// LOCALIZATION: Uzbek display fields (preferred for /uz pages)
	TitleUz       string   `bson:"title_uz,omitempty" json:"title_uz,omitempty"`
	DescriptionUz string   `bson:"description_uz,omitempty" json:"description_uz,omitempty"`
	GenresUz      []string `bson:"genres_uz,omitempty" json:"genres_uz,omitempty"`
	CountriesUz   []string `bson:"countries_uz,omitempty" json:"countries_uz,omitempty"`

	// Metadata source tracking
	HasTMDBUzTranslation bool `bson:"has_tmdb_uz_translation" json:"has_tmdb_uz_translation"`
}
