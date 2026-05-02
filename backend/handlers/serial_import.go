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

// importSerial calls the parser's /serial-details, upserts series/season/
// episode records, then creates one ingestion job per episode.
//
// Idempotent on re-import: series/season/episode records are reused by slug /
// (series,season) / (season,episode), and per-episode jobs are unique-keyed by
// (source, source_id="<serial-source-id>:s<N>e<M>") so retried imports do not
// duplicate rows.
func (h *IngestionHandler) importSerial(c *gin.Context, source, sourceID, detailURL, adminTitle string) {
	parserBaseURL := strings.TrimRight(h.parserURL, "/")
	if parserBaseURL == "" {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "parser service URL is not configured"})
		return
	}

	params := url.Values{}
	params.Set("url", detailURL)
	params.Set("source", source)
	parserEndpoint := fmt.Sprintf("%s/serial-details?%s", parserBaseURL, params.Encode())
	log.Printf("[INGESTION] SERIAL: calling parser %s", parserEndpoint)

	resp, err := h.httpClient.Get(parserEndpoint)
	if err != nil {
		log.Printf("[INGESTION] SERIAL: parser call failed - %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "parser service unreachable", "details": err.Error()})
		return
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	var payload parserSerialResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		log.Printf("[INGESTION] SERIAL: decode failed status=%d body=%s", resp.StatusCode, safeBody(body))
		c.JSON(http.StatusBadGateway, gin.H{"error": "invalid parser response", "details": err.Error()})
		return
	}
	if !payload.Success {
		log.Printf("[INGESTION] SERIAL: parser reported failure status=%d error=%s", resp.StatusCode, payload.Error)
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "parser could not extract serial", "details": payload.Error})
		return
	}
	if len(payload.Episodes) == 0 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "parser returned no episodes"})
		return
	}

	title := strings.TrimSpace(payload.Title)
	if title == "" {
		title = strings.TrimSpace(adminTitle)
	}
	if title == "" {
		title = "Serial " + sourceID
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	series, err := h.upsertSeries(ctx, title, &payload)
	if err != nil {
		log.Printf("[INGESTION] SERIAL: upsert series failed - %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert series", "details": err.Error()})
		return
	}
	log.Printf("[INGESTION] SERIAL: series=%s slug=%s episodes=%d", series.ID.Hex(), series.Slug, len(payload.Episodes))

	// Group episodes by season number.
	seasonCache := map[int]*models.Season{}
	var createdJobs []*models.IngestionJob
	var skipped []string
	episodesResolved := 0

	for _, ep := range payload.Episodes {
		if ep.VideoURL == "" {
			reason := "no video_url"
			if strings.TrimSpace(ep.Error) != "" {
				reason = ep.Error
			}
			skipped = append(skipped, fmt.Sprintf("S%02dE%02d (%s)", ep.Season, ep.Episode, reason))
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
				log.Printf("[INGESTION] SERIAL: upsert season s%d failed - %v", seasonNum, err)
				continue
			}
			season = s
			seasonCache[seasonNum] = season
		}

		episode, err := h.upsertEpisode(ctx, series.ID, season.ID, ep, series.PosterURL)
		if err != nil {
			log.Printf("[INGESTION] SERIAL: upsert episode S%02dE%02d failed - %v", seasonNum, ep.Episode, err)
			continue
		}
		episodesResolved++

		job, created, err := h.createEpisodeJob(ctx, series, season, episode, source, sourceID, ep, seasonNum)
		if err != nil {
			log.Printf("[INGESTION] SERIAL: create job S%02dE%02d failed - %v", seasonNum, ep.Episode, err)
			continue
		}
		if created {
			createdJobs = append(createdJobs, job)
		} else {
			skipped = append(skipped, fmt.Sprintf("S%02dE%02d (job exists)", seasonNum, ep.Episode))
		}
	}

	log.Printf("[INGESTION] SERIAL: done series=%s created_jobs=%d skipped=%d", series.ID.Hex(), len(createdJobs), len(skipped))

	if len(payload.Warnings) > 0 {
		log.Printf("[INGESTION] SERIAL: parser warnings=%v", payload.Warnings)
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":           "Serial ingestion started",
		"series_id":         series.ID.Hex(),
		"series_slug":       series.Slug,
		"episodes_found":    len(payload.Episodes),
		"episodes_resolved": episodesResolved,
		"jobs_created":      len(createdJobs),
		"jobs":              createdJobs,
		"skipped":           skipped,
		"warnings":          payload.Warnings,
		"missing_numbers":   payload.MissingNumbers,
	})
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
