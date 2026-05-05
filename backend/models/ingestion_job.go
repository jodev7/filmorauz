package models

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// IngestionStatus represents the status of an ingestion job
type IngestionStatus string

const (
	IngestionStatusQueued         IngestionStatus = "queued"
	IngestionStatusPending        IngestionStatus = IngestionStatusQueued
	IngestionStatusDownloading    IngestionStatus = "downloading"
	IngestionStatusDownloaded     IngestionStatus = "downloaded"
	IngestionStatusReadyToProcess IngestionStatus = "ready_to_process"
	IngestionStatusProcessing     IngestionStatus = "processing"
	IngestionStatusUploading      IngestionStatus = "uploading"
	IngestionStatusCompleted      IngestionStatus = "completed"
	IngestionStatusFailed         IngestionStatus = "failed"

	// Legacy statuses for pipeline compatibility (deprecated but still used)
	IngestionStatusParsing     IngestionStatus = "parsing"
	IngestionStatusNeedsManual IngestionStatus = "needs_manual"

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
	ID                 primitive.ObjectID   `bson:"_id,omitempty" json:"id"`
	MovieID            primitive.ObjectID   `bson:"movie_id,omitempty" json:"movie_id,omitempty"`
	Title              string               `bson:"title" json:"title"` // Movie title
	Source             string               `bson:"source" json:"source"`
	SourceID           string               `bson:"source_id" json:"source_id"`
	DetailURL          string               `bson:"detail_url" json:"detail_url"`
	Status             IngestionStatus      `bson:"status" json:"status"`
	Progress           int                  `bson:"progress" json:"progress"`
	Steps              JobSteps             `bson:"steps" json:"steps"`
	Logs               []IngestionLog       `bson:"logs" json:"logs"`
	Metadata           *ParsedMovieMetadata `bson:"metadata,omitempty" json:"metadata,omitempty"`
	Error              string               `bson:"error,omitempty" json:"error,omitempty"`
	LocalPath          string               `bson:"local_path,omitempty" json:"local_path,omitempty"`                     // Original source file path (parser/downloads)
	FilePath           string               `bson:"file_path,omitempty" json:"file_path,omitempty"`                       // Legacy alias for local_path
	DownloadedFilePath string               `bson:"downloaded_file_path,omitempty" json:"downloaded_file_path,omitempty"` // Legacy alias for local_path
	OutputPath         string               `bson:"output_path,omitempty" json:"output_path,omitempty"`                   // Processed output path (worker/uploads/... or B2)
	PlaylistPath       string               `bson:"playlist_path,omitempty" json:"playlist_path,omitempty"`               // HLS playlist path
	OutputMode         string               `bson:"output_mode,omitempty" json:"output_mode,omitempty"`                   // "development" or "production"
	SourceFileDeleted  bool                 `bson:"source_file_deleted,omitempty" json:"source_file_deleted,omitempty"`   // True if source file was deleted after processing
	VideoURL           string               `bson:"video_url,omitempty" json:"video_url,omitempty"`
	RetryCount         int                  `bson:"retry_count" json:"retry_count"`
	CreatedAt          time.Time            `bson:"created_at" json:"created_at"`
	UpdatedAt          time.Time            `bson:"updated_at" json:"updated_at"`
	// StartedAt is set when a worker first claims the job (status -> processing).
	// Elapsed time should be measured from this, NOT created_at, so jobs sitting
	// in the pending queue do not show a growing timer.
	StartedAt   *time.Time `bson:"started_at,omitempty" json:"started_at,omitempty"`
	CompletedAt *time.Time `bson:"completed_at,omitempty" json:"completed_at,omitempty"`

	// Phase-specific timestamps for accurate elapsed-time reporting in the UI.
	// download_started_at / download_finished_at bracket the download stage.
	// queued_for_processing_at marks when the job entered the ready_to_process
	// queue. processing_started_at / processing_finished_at bracket the ffmpeg
	// stage so jobs that wait hours for a process slot do not show inflated
	// "processing elapsed" numbers.
	DownloadStartedAt     *time.Time `bson:"download_started_at,omitempty" json:"download_started_at,omitempty"`
	DownloadFinishedAt    *time.Time `bson:"download_finished_at,omitempty" json:"download_finished_at,omitempty"`
	QueuedForProcessingAt *time.Time `bson:"queued_for_processing_at,omitempty" json:"queued_for_processing_at,omitempty"`
	ProcessingStartedAt   *time.Time `bson:"processing_started_at,omitempty" json:"processing_started_at,omitempty"`
	ProcessingFinishedAt  *time.Time `bson:"processing_finished_at,omitempty" json:"processing_finished_at,omitempty"`

	// Real-time download progress fields
	Stage           string     `bson:"stage,omitempty" json:"stage,omitempty"`
	Message         string     `bson:"message,omitempty" json:"message,omitempty"` // Human-readable message
	DownloadedBytes int64      `bson:"downloaded_bytes,omitempty" json:"downloaded_bytes,omitempty"`
	TotalBytes      int64      `bson:"total_bytes,omitempty" json:"total_bytes,omitempty"`
	SpeedMBps       float64    `bson:"speed_mbps,omitempty" json:"speed_mbps,omitempty"`
	EtaSeconds      int        `bson:"eta_seconds,omitempty" json:"eta_seconds,omitempty"`
	LastProgressAt  *time.Time `bson:"last_progress_at,omitempty" json:"last_progress_at,omitempty"`
	WorkerID        string     `bson:"worker_id,omitempty" json:"worker_id,omitempty"`
	LockedUntil     *time.Time `bson:"locked_until,omitempty" json:"locked_until,omitempty"`

	// Direct upload specific fields
	TempFileURL string `bson:"temp_file_url,omitempty" json:"temp_file_url,omitempty"` // Temp file URL (B2 path) for direct upload
	TempFileKey string `bson:"temp_file_key,omitempty" json:"temp_file_key,omitempty"` // Temp file B2 key for cleanup
	Quality     string `bson:"quality,omitempty" json:"quality,omitempty"`             // Video quality (480p, 720p, etc.)
	IsPremium   bool   `bson:"is_premium,omitempty" json:"is_premium,omitempty"`       // Premium content flag

	// Quality info from source (Asilmedia etc.)
	SourceQuality      string   `bson:"source_quality,omitempty" json:"source_quality,omitempty"`
	AvailableQualities []string `bson:"available_qualities,omitempty" json:"available_qualities,omitempty"`
	SourceResolution   string   `bson:"source_resolution,omitempty" json:"source_resolution,omitempty"`
	GeneratedQualities []string `bson:"generated_qualities,omitempty" json:"generated_qualities,omitempty"`
	MasterPlaylistURL  string   `bson:"master_playlist_url,omitempty" json:"master_playlist_url,omitempty"`

	// Serial ingestion linkage. Set only when this job processes a single
	// episode (ContentType == "episode"); otherwise left empty for movies.
	ContentType   string             `bson:"content_type,omitempty" json:"content_type,omitempty"` // "" | "movie" | "episode"
	SeriesID      primitive.ObjectID `bson:"series_id,omitempty" json:"series_id,omitempty"`
	SeasonID      primitive.ObjectID `bson:"season_id,omitempty" json:"season_id,omitempty"`
	EpisodeID     primitive.ObjectID `bson:"episode_id,omitempty" json:"episode_id,omitempty"`
	SeriesSlug    string             `bson:"series_slug,omitempty" json:"series_slug,omitempty"`
	SeasonNumber  int                `bson:"season_number,omitempty" json:"season_number,omitempty"`
	EpisodeNumber int                `bson:"episode_number,omitempty" json:"episode_number,omitempty"`

	// Serial-parent specific summary fields. ContentType == "serial_parent" rows
	// are created immediately by the import endpoint, then populated by the
	// background extractor as it discovers seasons/episodes and fans out child
	// jobs. UI shows these as a progress indicator while extraction runs.
	SeasonsCount     int   `bson:"seasons_count,omitempty"      json:"seasons_count,omitempty"`
	EpisodeCount     int   `bson:"episode_count,omitempty"      json:"episode_count,omitempty"`
	ChildJobsCreated int   `bson:"child_jobs_created,omitempty" json:"child_jobs_created,omitempty"`
	MissingEpisodes  []int `bson:"missing_episodes,omitempty"   json:"missing_episodes,omitempty"`
}

// JobSteps tracks the completion status of each pipeline step
type JobSteps struct {
	Download bool `bson:"download" json:"download"`
	Process  bool `bson:"process" json:"process"`
	Upload   bool `bson:"upload" json:"upload"`
	Metadata bool `bson:"metadata" json:"metadata"`
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
}

// VideoSource represents a video source
type VideoSource struct {
	Quality string `bson:"quality" json:"quality"`
	URL     string `bson:"url" json:"url"`
	Type    string `bson:"type" json:"type"` // iframe, mp4, hls, download
}
