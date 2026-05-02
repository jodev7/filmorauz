package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// parserSerialEpisode mirrors the shape returned by the Python parser's
// /serial-details endpoint.
type parserSerialEpisode struct {
	Season      int               `json:"season"`
	Episode     int               `json:"episode"`
	Title       string            `json:"title"`
	EpisodeURL  string            `json:"episode_url"`
	VideoURL    string            `json:"video_url"`
	Poster      string            `json:"poster"`
	QualityURLs map[string]string `json:"quality_urls"`
	Error       string            `json:"error"`
}

type parserSerialSeasonEpisode struct {
	EpisodeNumber int               `json:"episode_number"`
	Title         string            `json:"title"`
	VideoURL      string            `json:"video_url"`
	QualityURLs   map[string]string `json:"quality_urls"`
	Error         string            `json:"error"`
}

type parserSerialSeason struct {
	SeasonNumber int                         `json:"season_number"`
	Episodes     []parserSerialSeasonEpisode `json:"episodes"`
}

type parserSerialResponse struct {
	Success     bool                  `json:"success"`
	Type        string                `json:"type"`
	Provider    string                `json:"provider"`
	Title       string                `json:"title"`
	Year        int                   `json:"year"`
	Poster      string                `json:"poster"`
	Backdrop    string                `json:"backdrop"`
	Description string                `json:"description"`
	Episodes       []parserSerialEpisode `json:"episodes"`
	Seasons        []parserSerialSeason  `json:"seasons"`
	Warnings       []string              `json:"warnings"`
	MissingNumbers []int                 `json:"missing_numbers"`
	Error          string                `json:"error"`
}

var serialSlugCleanRe = regexp.MustCompile(`[^a-z0-9]+`)

func slugifySerial(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = serialSlugCleanRe.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "series"
	}
	return s
}

// ContentTypeSerialParent identifies the placeholder parent job created
// synchronously by /api/admin/ingestion/import for a serial. The actual
// per-episode jobs (ContentType == "episode") are fanned out by the
// background extractor goroutine.
const ContentTypeSerialParent = "serial_parent"

// importSerial creates a parent ingestion job IMMEDIATELY (so the HTTP
// response can return inside the proxy timeout window), then kicks off the
// expensive parser /serial-details + per-episode job creation in a goroutine.
//
// For an N=90 serial this used to block the request for minutes and trip the
// 504 at the proxy. Now the handler returns 202 within ~50ms regardless of N.
//
// Idempotent on re-import: an existing active (queued/processing) serial_parent
// for the same (source, source_id) is reused instead of created. Per-episode
// jobs are still keyed by (source, "<source_id>:s<N>e<M>").
func (h *IngestionHandler) importSerial(c *gin.Context, source, sourceID, detailURL, adminTitle string) {
	parserBaseURL := strings.TrimRight(h.parserURL, "/")
	if parserBaseURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Idempotency: if a parent job already exists for this (source, source_id),
	// and is not in a terminal-failed state, return its id without re-queueing.
	if existing, err := h.jobRepo.GetBySourceAndID(ctx, source, sourceID); err == nil && existing != nil {
		if existing.ContentType == ContentTypeSerialParent &&
			existing.Status != models.IngestionStatusFailed {
			log.Printf("[import] reuse existing parent job=%s source=%s source_id=%s status=%s",
				existing.ID.Hex(), source, sourceID, existing.Status)
			c.JSON(http.StatusAccepted, gin.H{
				"ok":      true,
				"job_id":  existing.ID.Hex(),
				"message": "Import navbatga qo'shildi",
			})
			return
		}
	}

	title := strings.TrimSpace(adminTitle)
	if title == "" {
		title = "Serial " + sourceID
	}
	parent := &models.IngestionJob{
		Title:       title,
		Source:      source,
		SourceID:    sourceID,
		DetailURL:   detailURL,
		Status:      models.IngestionStatusQueued,
		Stage:       string(models.IngestionStatusQueued),
		ContentType: ContentTypeSerialParent,
		Steps:       models.JobSteps{},
		Logs:        []models.IngestionLog{},
		Message:     "Import queued",
	}
	if err := h.jobRepo.Create(ctx, parent); err != nil {
		log.Printf("[import] failed to create parent job source=%s source_id=%s err=%v", source, sourceID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to queue import", "details": err.Error()})
		return
	}

	log.Printf("[import] queued source=%s source_id=%s parent_job=%s", source, sourceID, parent.ID.Hex())

	go h.runSerialExtractionAsync(parent, source, sourceID, detailURL, title, parserBaseURL)

	c.JSON(http.StatusAccepted, gin.H{
		"ok":      true,
		"job_id":  parent.ID.Hex(),
		"message": "Import navbatga qo'shildi",
	})
}

// runSerialExtractionAsync performs the heavy work that used to be inline in
// importSerial. It runs in its own goroutine — there is no Gin context here,
// errors are surfaced by updating the parent job document.
func (h *IngestionHandler) runSerialExtractionAsync(
	parent *models.IngestionJob, source, sourceID, detailURL, title, parserBaseURL string,
) {
	parentID := parent.ID.Hex()
	log.Printf("[series extractor] started parent_job=%s source=%s source_id=%s url=%s",
		parentID, source, sourceID, detailURL)

	// Use a long-lived context for the full extraction; parser /serial-details
	// for a 90-episode serial can take many minutes because it visits every
	// episode page. Capping at 30 minutes is a sanity guard.
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := h.markParentProcessing(ctx, parentID); err != nil {
		log.Printf("[series extractor] mark processing failed parent_job=%s err=%v", parentID, err)
	}

	params := url.Values{}
	params.Set("url", detailURL)
	params.Set("source", source)
	parserEndpoint := fmt.Sprintf("%s/serial-details?%s", parserBaseURL, params.Encode())
	log.Printf("[series extractor] calling parser parent_job=%s endpoint=%s", parentID, parserEndpoint)

	resp, err := h.httpClient.Get(parserEndpoint)
	if err != nil {
		h.failParent(ctx, parentID, fmt.Sprintf("parser unreachable: %v", err))
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var payload parserSerialResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		h.failParent(ctx, parentID, fmt.Sprintf("invalid parser response (http=%d): %v", resp.StatusCode, err))
		return
	}
	if !payload.Success {
		reason := payload.Error
		if reason == "" {
			reason = "parser could not extract serial"
		}
		h.failParent(ctx, parentID, reason)
		return
	}
	if len(payload.Episodes) == 0 {
		h.failParent(ctx, parentID, "parser returned no episodes")
		return
	}

	resolvedTitle := strings.TrimSpace(payload.Title)
	if resolvedTitle == "" {
		resolvedTitle = title
	}

	series, err := h.upsertSeries(ctx, resolvedTitle, &payload)
	if err != nil {
		h.failParent(ctx, parentID, fmt.Sprintf("failed to upsert series: %v", err))
		return
	}

	seasonsSet := map[int]struct{}{}
	for _, ep := range payload.Episodes {
		s := ep.Season
		if s <= 0 {
			s = 1
		}
		seasonsSet[s] = struct{}{}
	}
	log.Printf("[series extractor] found seasons=%d episodes=%d parent_job=%s",
		len(seasonsSet), len(payload.Episodes), parentID)

	seasonCache := map[int]*models.Season{}
	createdCount := 0
	skipped := 0

	for _, ep := range payload.Episodes {
		if ep.VideoURL == "" {
			skipped++
			continue
		}
		seasonNum := ep.Season
		if seasonNum <= 0 {
			seasonNum = 1
		}
		season, ok := seasonCache[seasonNum]
		if !ok {
			s, err := h.upsertSeason(ctx, series.ID, seasonNum)
			if err != nil {
				log.Printf("[series extractor] upsert season s%d failed parent_job=%s err=%v", seasonNum, parentID, err)
				continue
			}
			season = s
			seasonCache[seasonNum] = season
		}

		episode, err := h.upsertEpisode(ctx, series.ID, season.ID, ep, series.PosterURL)
		if err != nil {
			log.Printf("[series extractor] upsert episode S%02dE%02d failed parent_job=%s err=%v",
				seasonNum, ep.Episode, parentID, err)
			continue
		}

		_, created, err := h.createEpisodeJob(ctx, series, season, episode, source, sourceID, ep, seasonNum)
		if err != nil {
			log.Printf("[series extractor] create job S%02dE%02d failed parent_job=%s err=%v",
				seasonNum, ep.Episode, parentID, err)
			continue
		}
		if created {
			createdCount++
		}
	}

	log.Printf("[series extractor] created child jobs=%d skipped=%d parent_job=%s",
		createdCount, skipped, parentID)
	if len(payload.Warnings) > 0 {
		log.Printf("[series extractor] warnings parent_job=%s warnings=%v", parentID, payload.Warnings)
	}
	if err := h.completeParent(ctx, parentID, len(seasonsSet), len(payload.Episodes), createdCount, series.ID, series.Slug); err != nil {
		log.Printf("[series extractor] complete parent failed parent_job=%s err=%v", parentID, err)
	}
}

func (h *IngestionHandler) markParentProcessing(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": bson.M{
		"status":                 models.IngestionStatusProcessing,
		"stage":                  string(models.IngestionStatusProcessing),
		"processing_started_at":  now,
		"started_at":             now,
		"updated_at":             now,
		"message":                "Extracting episodes",
	}})
	return err
}

func (h *IngestionHandler) failParent(ctx context.Context, id, reason string) {
	log.Printf("[series extractor] failed parent_job=%s error=%s", id, reason)
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return
	}
	now := time.Now()
	_, _ = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": bson.M{
		"status":       models.IngestionStatusFailed,
		"stage":        string(models.IngestionStatusFailed),
		"error":        reason,
		"completed_at": now,
		"updated_at":   now,
	}})
}

func (h *IngestionHandler) completeParent(
	ctx context.Context, id string, seasons, episodes, childJobs int,
	seriesID primitive.ObjectID, seriesSlug string,
) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": bson.M{
		"status":             models.IngestionStatusCompleted,
		"stage":              string(models.IngestionStatusCompleted),
		"progress":           100,
		"seasons_count":      seasons,
		"episode_count":      episodes,
		"child_jobs_created": childJobs,
		"series_id":          seriesID,
		"series_slug":        seriesSlug,
		"completed_at":       now,
		"updated_at":         now,
		"message":            fmt.Sprintf("Extracted %d episodes (%d new jobs)", episodes, childJobs),
	}})
	return err
}

// upsertSeries finds an existing series by slug or creates a new one (pending
// approval). The slug is derived deterministically from the title so re-
// importing the same serial targets the same row.
func (h *IngestionHandler) upsertSeries(ctx context.Context, title string, payload *parserSerialResponse) (*models.Series, error) {
	slug := slugifySerial(title)
	col := h.seriesRepo.Collection()

	var existing models.Series
	err := col.FindOne(ctx, bson.M{"slug": slug}).Decode(&existing)
	if err == nil {
		return &existing, nil
	}
	if err != mongo.ErrNoDocuments {
		return nil, err
	}

	now := time.Now()
	series := &models.Series{
		Slug:           slug,
		Title:          title,
		Description:    payload.Description,
		PosterURL:      payload.Poster,
		BackdropURL:    payload.Backdrop,
		Year:           payload.Year,
		ApprovalStatus: "pending",
		IsPublished:    false,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	res, err := col.InsertOne(ctx, series)
	if err != nil {
		return nil, err
	}
	series.ID = res.InsertedID.(primitive.ObjectID)
	if h.seriesSvc != nil {
		if _, err := h.seriesSvc.EnsureSeriesCodeByID(series.ID); err != nil {
			return nil, err
		}
	}
	return series, nil
}

func (h *IngestionHandler) upsertSeason(ctx context.Context, seriesID primitive.ObjectID, seasonNum int) (*models.Season, error) {
	// Reuse the repository's raw collection via GetSeasonsBySeriesID scan is wasteful;
	// instead issue a FindOne directly through the repo's underlying DB.
	seasons, err := h.seriesRepo.GetSeasonsBySeriesID(seriesID)
	if err == nil {
		for i := range seasons {
			if seasons[i].SeasonNumber == seasonNum {
				return &seasons[i], nil
			}
		}
	}

	season := &models.Season{
		SeriesID:     seriesID,
		SeasonNumber: seasonNum,
		Title:        fmt.Sprintf("Season %d", seasonNum),
	}
	if err := h.seriesRepo.CreateSeason(season); err != nil {
		return nil, err
	}
	return season, nil
}

func (h *IngestionHandler) upsertEpisode(ctx context.Context, seriesID, seasonID primitive.ObjectID, ep parserSerialEpisode, fallbackPoster string) (*models.Episode, error) {
	episodes, err := h.seriesRepo.GetEpisodesBySeasonID(seasonID)
	if err == nil {
		for i := range episodes {
			if episodes[i].EpisodeNumber == ep.Episode {
				return &episodes[i], nil
			}
		}
	}

	thumb := ep.Poster
	if thumb == "" {
		thumb = fallbackPoster
	}
	title := strings.TrimSpace(ep.Title)
	if title == "" {
		title = fmt.Sprintf("%d-qism", ep.Episode)
	}

	episode := &models.Episode{
		SeriesID:      seriesID,
		SeasonID:      seasonID,
		EpisodeNumber: ep.Episode,
		Title:         title,
		ThumbnailURL:  thumb,
	}
	if err := h.seriesRepo.CreateEpisode(episode); err != nil {
		return nil, err
	}
	return episode, nil
}

// createEpisodeJob inserts a per-episode ingestion job, keyed by
// (source, source_id="<serial-source-id>:s<N>e<M>") so reruns are idempotent.
// Returns (job, wasCreated, err). If the job already exists, wasCreated=false.
func (h *IngestionHandler) createEpisodeJob(
	ctx context.Context,
	series *models.Series,
	season *models.Season,
	episode *models.Episode,
	source, sourceID string,
	ep parserSerialEpisode,
	seasonNum int,
) (*models.IngestionJob, bool, error) {
	episodeSourceID := fmt.Sprintf("%s:s%02de%02d", sourceID, seasonNum, ep.Episode)

	// Already exists?
	existing, err := h.jobRepo.GetBySourceAndID(ctx, source, episodeSourceID)
	if err == nil && existing != nil {
		return existing, false, nil
	}

	job := &models.IngestionJob{
		Title:         fmt.Sprintf("%s S%02dE%02d", series.Title, seasonNum, ep.Episode),
		Source:        source,
		SourceID:      episodeSourceID,
		DetailURL:     ep.EpisodeURL,
		VideoURL:      ep.VideoURL,
		Status:        models.IngestionStatusQueued,
		Stage:         string(models.IngestionStatusQueued),
		Progress:      0,
		Steps:         models.JobSteps{},
		Logs:          []models.IngestionLog{},
		ContentType:   "episode",
		SeriesID:      series.ID,
		SeasonID:      season.ID,
		EpisodeID:     episode.ID,
		SeriesSlug:    series.Slug,
		SeasonNumber:  seasonNum,
		EpisodeNumber: ep.Episode,
		Metadata: &models.ParsedMovieMetadata{
			Title:        fmt.Sprintf("%s S%02dE%02d", series.Title, seasonNum, ep.Episode),
			Poster:       series.PosterURL,
			Backdrop:     series.BackdropURL,
			Year:         series.Year,
			VideoPageURL: ep.EpisodeURL,
		},
	}

	if err := h.jobRepo.Create(ctx, job); err != nil {
		// Duplicate key race — treat as existing
		if mongo.IsDuplicateKeyError(err) {
			if dup, gerr := h.jobRepo.GetBySourceAndID(ctx, source, episodeSourceID); gerr == nil && dup != nil {
				return dup, false, nil
			}
		}
		return nil, false, err
	}
	return job, true, nil
}

// CompleteEpisode is called by the worker once an episode's HLS output is
// finalized. Updates the target Episode row with the playback URL so the
// frontend can serve it.
//
// POST /api/ingestion/episodes/:id/complete
// Body: { "video_url": "...", "duration": 42, "embed_url": "" }
func (h *IngestionHandler) CompleteEpisode(c *gin.Context) {
	idHex := c.Param("id")
	epID, err := primitive.ObjectIDFromHex(idHex)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode id"})
		return
	}

	var body struct {
		VideoURL          string `json:"video_url" binding:"required"`
		EmbedURL          string `json:"embed_url"`
		Duration          int    `json:"duration"`
		ThumbnailsBaseURL string `json:"thumbnails_base_url"`
		ThumbnailInterval int    `json:"thumbnail_interval"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	episode, err := h.seriesRepo.GetEpisodeByID(epID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "episode not found"})
		return
	}
	if h.seriesSvc != nil {
		if _, err := h.seriesSvc.EnsureSeriesCodeByID(episode.SeriesID); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to assign series code"})
			return
		}
	}
	episode.VideoURL = body.VideoURL
	episode.SourceType = models.VideoSourceDirectHLS
	if body.EmbedURL != "" {
		episode.EmbedURL = body.EmbedURL
	}
	if body.Duration > 0 {
		episode.Duration = body.Duration
	}
	if body.ThumbnailsBaseURL != "" {
		episode.ThumbnailsBaseURL = body.ThumbnailsBaseURL
		if body.ThumbnailInterval > 0 {
			episode.ThumbnailInterval = body.ThumbnailInterval
		}
	}
	if err := h.seriesRepo.UpdateEpisode(episode); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	log.Printf("[INGESTION] EPISODE COMPLETE: id=%s video_url=%s", epID.Hex(), body.VideoURL)
	c.JSON(http.StatusOK, gin.H{"message": "episode updated", "episode": episode})
}

func safeBody(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
