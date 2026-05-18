package handlers

import (
	"bytes"
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
	Success        bool                  `json:"success"`
	Type           string                `json:"type"`
	Provider       string                `json:"provider"`
	Title          string                `json:"title"`
	Year           int                   `json:"year"`
	Poster         string                `json:"poster"`
	Backdrop       string                `json:"backdrop"`
	Description    string                `json:"description"`
	Episodes       []parserSerialEpisode `json:"episodes"`
	Seasons        []parserSerialSeason  `json:"seasons"`
	Warnings       []string              `json:"warnings"`
	MissingNumbers []int                 `json:"missing_numbers"`
	Error          string                `json:"error"`
}

type parserSerialAsyncStartResponse struct {
	OK          bool   `json:"ok"`
	ParserJobID string `json:"parser_job_id"`
	JobID       string `json:"job_id"`
	Status      string `json:"status"`
	Message     string `json:"message"`
	Error       string `json:"error"`
}

type parserSerialAsyncStatus struct {
	JobID           string                `json:"job_id"`
	Status          string                `json:"status"`
	Stage           string                `json:"stage"`
	Provider        string                `json:"provider"`
	Message         string                `json:"message"`
	Title           string                `json:"title"`
	Year            int                   `json:"year"`
	Poster          string                `json:"poster"`
	Backdrop        string                `json:"backdrop"`
	Description     string                `json:"description"`
	Episodes        []parserSerialEpisode `json:"episodes"`
	ExpectedTotal   int                   `json:"expected_total"`
	DiscoveredCount int                   `json:"discovered_count"`
	ResolvedCount   int                   `json:"resolved_count"`
	MissingNumbers  []int                 `json:"missing_numbers"`
	Warnings        []string              `json:"warnings"`
	Result          *parserSerialResponse `json:"result"`
	Error           string                `json:"error"`
}

func (s parserSerialAsyncStatus) asResponse() parserSerialResponse {
	if s.Result != nil {
		return *s.Result
	}
	return parserSerialResponse{
		Success:        len(s.Episodes) > 0,
		Type:           "serial",
		Provider:       s.Provider,
		Title:          s.Title,
		Year:           s.Year,
		Poster:         s.Poster,
		Backdrop:       s.Backdrop,
		Description:    s.Description,
		Episodes:       s.Episodes,
		Warnings:       s.Warnings,
		MissingNumbers: s.MissingNumbers,
	}
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

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	if err := h.markParentProcessing(ctx, parentID); err != nil {
		log.Printf("[series extractor] mark processing failed parent_job=%s err=%v", parentID, err)
	}

	parserJobID, err := h.startAsyncSerialParserJob(parserBaseURL, source, detailURL)
	if err != nil {
		h.failParent(ctx, parentID, err.Error())
		return
	}

	payload, createdCount, series, seasonsCount, err := h.pollSerialParserJob(ctx, source, sourceID, title, parserBaseURL, parserJobID, parentID)
	if err != nil {
		h.failParent(ctx, parentID, err.Error())
		return
	}
	if len(payload.Episodes) == 0 {
		h.failParent(ctx, parentID, "parser returned no episodes")
		return
	}
	// Deferred-DB mode: don't mark the parent completed yet. The extraction
	// phase only queued the child IngestionJobs. finalizeSerialParent() runs
	// once every child job is terminal — that's when we insert Series,
	// Seasons, Episodes and mark this parent completed.
	if err := h.markExtractionFinished(ctx, parentID, seasonsCount, len(payload.Episodes), createdCount, series.Slug, payload.MissingNumbers); err != nil {
		log.Printf("[series extractor] mark extraction finished failed parent_job=%s err=%v", parentID, err)
	}
}

// markExtractionFinished records that the parser has produced all episode
// jobs. The parent stays in "processing" — finalizeSerialParent moves it to
// "completed" once every child is terminal.
func (h *IngestionHandler) markExtractionFinished(
	ctx context.Context, id string, seasons, episodes, childJobs int,
	seriesSlug string, missing []int,
) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	message := fmt.Sprintf("Extraction finished: %d episodes queued. Waiting for downloads to complete.", childJobs)
	if len(missing) > 0 {
		message = fmt.Sprintf("%s Missing: %v", message, missing)
	}
	update := bson.M{
		"status":             models.IngestionStatusProcessing,
		"stage":              "waiting_for_episodes",
		"progress":           50,
		"seasons_count":      seasons,
		"episode_count":      episodes,
		"child_jobs_created": childJobs,
		"series_slug":        seriesSlug,
		"updated_at":         now,
		"message":            message,
	}
	if len(missing) > 0 {
		update["missing_episodes"] = missing
	}
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": update})
	return err
}

func (h *IngestionHandler) startAsyncSerialParserJob(parserBaseURL, source, detailURL string) (string, error) {
	if err := checkParserHealth(parserBaseURL); err != nil {
		return "", err
	}

	body, _ := json.Marshal(map[string]string{
		"source": source,
		"url":    detailURL,
	})
	endpoint := fmt.Sprintf("%s/serial/extract/start", strings.TrimRight(parserBaseURL, "/"))
	log.Printf("[serial import] parser_start_url=%s", endpoint)
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := serialDetailsClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to start parser serial extraction: %w", err)
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[serial import] parser_start_url=%s status=%d content_type=%s body_preview_first_300_chars=%q",
		endpoint, resp.StatusCode, resp.Header.Get("Content-Type"), parserRawPrefix(respBody, 300))
	if !looksLikeJSON(resp.Header.Get("Content-Type"), respBody) {
		return "", fmt.Errorf("parser async start returned non-json response (HTTP %d)", resp.StatusCode)
	}

	var start parserSerialAsyncStartResponse
	if err := json.Unmarshal(respBody, &start); err != nil {
		return "", fmt.Errorf("failed to decode parser async start response: %w", err)
	}
	parserJobID := strings.TrimSpace(start.ParserJobID)
	if parserJobID == "" {
		parserJobID = strings.TrimSpace(start.JobID)
	}
	if resp.StatusCode != http.StatusAccepted || parserJobID == "" {
		msg := strings.TrimSpace(start.Error)
		if msg == "" {
			msg = strings.TrimSpace(start.Message)
		}
		if msg == "" {
			msg = string(respBody)
		}
		return "", fmt.Errorf("parser async start failed (HTTP %d): %s", resp.StatusCode, msg)
	}
	return parserJobID, nil
}

func checkParserHealth(parserBaseURL string) error {
	endpoint := fmt.Sprintf("%s/health", strings.TrimRight(parserBaseURL, "/"))
	resp, err := serialDetailsClient.Get(endpoint)
	if err != nil {
		return fmt.Errorf("parser health check failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[serial import] parser_health_url=%s status=%d content_type=%s body_preview_first_300_chars=%q",
		endpoint, resp.StatusCode, resp.Header.Get("Content-Type"), parserRawPrefix(body, 300))
	if !looksLikeJSON(resp.Header.Get("Content-Type"), body) {
		return fmt.Errorf("parser health returned non-json response (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("parser health check failed (HTTP %d)", resp.StatusCode)
	}
	return nil
}

func (h *IngestionHandler) pollSerialParserJob(
	ctx context.Context,
	source, sourceID, fallbackTitle, parserBaseURL, parserJobID, parentID string,
) (parserSerialResponse, int, *models.Series, int, error) {
	endpoint := fmt.Sprintf("%s/serial/extract/status/%s", strings.TrimRight(parserBaseURL, "/"), url.PathEscape(parserJobID))
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var (
		createdCount int
		lastLoggedAt int
		series       *models.Series
	)
	seasonCache := map[int]*models.Season{}
	seenSeasons := map[int]struct{}{}

	for {
		status, err := fetchSerialAsyncStatus(endpoint)
		if err != nil {
			return parserSerialResponse{}, createdCount, nil, 0, err
		}
		payload := status.asResponse()

		resolvedTitle := strings.TrimSpace(payload.Title)
		if resolvedTitle == "" {
			resolvedTitle = fallbackTitle
		}
		if series == nil && resolvedTitle != "" {
			series = h.buildSeriesStub(resolvedTitle, &payload)
		}
		parentObjID, _ := primitive.ObjectIDFromHex(parentID)
		if series != nil {
			createdCount, err = h.createIncrementalEpisodeJobs(ctx, series, seasonCache, seenSeasons, source, sourceID, parentObjID, payload.Episodes, createdCount)
			if err != nil {
				return parserSerialResponse{}, createdCount, series, len(seenSeasons), err
			}
		}

		if _, err := h.updateParentExtractionProgress(ctx, parentID, payload, createdCount, buildParentProgressMessage(status, createdCount)); err != nil {
			log.Printf("[series extractor] progress update failed parent_job=%s err=%v", parentID, err)
		}
		resolvedCount := maxInt(status.ResolvedCount, len(status.Episodes))
		if resolvedCount >= lastLoggedAt+5 {
			lastLoggedAt = resolvedCount - (resolvedCount % 5)
			expected := status.ExpectedTotal
			if expected <= 0 {
				expected = resolvedCount
			}
			_ = h.appendParentLog(ctx, parentID, fmt.Sprintf("Extracting episodes %d/%d. Created child jobs: %d", resolvedCount, expected, createdCount), "info")
		}

		switch strings.ToLower(status.Status) {
		case "queued", "processing":
			select {
			case <-ctx.Done():
				return parserSerialResponse{}, createdCount, series, len(seenSeasons), fmt.Errorf("serial extraction timed out")
			case <-ticker.C:
			}
		case "completed":
			if len(payload.Episodes) == 0 {
				return payload, createdCount, series, len(seenSeasons), fmt.Errorf("parser returned no episodes")
			}
			if series == nil {
				series = h.buildSeriesStub(resolvedTitle, &payload)
			}
			return payload, createdCount, series, len(seenSeasons), nil
		case "failed":
			msg := strings.TrimSpace(status.Error)
			if msg == "" {
				msg = "parser serial extraction failed"
			}
			return payload, createdCount, series, len(seenSeasons), fmt.Errorf(msg)
		default:
			select {
			case <-ctx.Done():
				return parserSerialResponse{}, createdCount, series, len(seenSeasons), fmt.Errorf("serial extraction timed out")
			case <-ticker.C:
			}
		}
	}
}

func fetchSerialAsyncStatus(endpoint string) (parserSerialAsyncStatus, error) {
	resp, err := serialDetailsClient.Get(endpoint)
	if err != nil {
		return parserSerialAsyncStatus{}, fmt.Errorf("failed to poll parser serial extraction: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if !looksLikeJSON(resp.Header.Get("Content-Type"), body) {
		return parserSerialAsyncStatus{}, fmt.Errorf("parser async status returned non-json response (HTTP %d)", resp.StatusCode)
	}
	var status parserSerialAsyncStatus
	if err := json.Unmarshal(body, &status); err != nil {
		return parserSerialAsyncStatus{}, fmt.Errorf("failed to decode parser async status: %w", err)
	}
	if resp.StatusCode >= 400 {
		msg := strings.TrimSpace(status.Error)
		if msg == "" {
			msg = strings.TrimSpace(status.Message)
		}
		if msg == "" {
			msg = fmt.Sprintf("parser async status failed (HTTP %d)", resp.StatusCode)
		}
		return parserSerialAsyncStatus{}, fmt.Errorf(msg)
	}
	return status, nil
}

func (h *IngestionHandler) createIncrementalEpisodeJobs(
	ctx context.Context,
	series *models.Series,
	seasonCache map[int]*models.Season,
	seenSeasons map[int]struct{},
	source, sourceID string,
	parentID primitive.ObjectID,
	episodes []parserSerialEpisode,
	createdCount int,
) (int, error) {
	// Deferred-DB mode: do NOT create Series/Season/Episode rows yet. Only
	// queue episode IngestionJobs with all the metadata the finalization step
	// will need. Series/Seasons/Episodes are inserted in one batch by
	// finalizeSerialParent() once every child job is terminal.
	for _, ep := range episodes {
		if ep.VideoURL == "" {
			continue
		}
		seasonNum := ep.Season
		if seasonNum <= 0 {
			seasonNum = 1
		}
		seenSeasons[seasonNum] = struct{}{}

		_, created, err := h.createEpisodeJob(ctx, series, source, sourceID, parentID, ep, seasonNum)
		if err != nil {
			return createdCount, fmt.Errorf("failed to create episode job S%02dE%02d: %w", seasonNum, ep.Episode, err)
		}
		if created {
			createdCount++
		}
	}
	return createdCount, nil
}

func buildParentProgressMessage(status parserSerialAsyncStatus, createdCount int) string {
	expected := status.ExpectedTotal
	if expected <= 0 {
		expected = maxInt(status.ResolvedCount, len(status.Episodes))
	}
	if expected <= 0 {
		expected = len(status.Episodes)
	}
	message := strings.TrimSpace(status.Message)
	if message == "" {
		message = fmt.Sprintf("Extracting episodes %d/%d...", maxInt(status.ResolvedCount, len(status.Episodes)), expected)
	}
	if len(status.MissingNumbers) > 0 {
		message = fmt.Sprintf("%s Created child jobs: %d. Missing: %v", message, createdCount, status.MissingNumbers)
	} else {
		message = fmt.Sprintf("%s Created child jobs: %d", message, createdCount)
	}
	return message
}

func (h *IngestionHandler) updateParentExtractionProgress(ctx context.Context, id string, payload parserSerialResponse, createdCount int, message string) (*mongo.UpdateResult, error) {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}
	expected := len(payload.Episodes)
	if expected <= 0 {
		expected = createdCount
	}
	progress := 0
	if expected > 0 {
		progress = int(float64(createdCount) / float64(expected) * 100)
	}
	update := bson.M{
		"updated_at":         time.Now(),
		"message":            message,
		"progress":           progress,
		"episode_count":      len(payload.Episodes),
		"child_jobs_created": createdCount,
		"missing_episodes":   payload.MissingNumbers,
	}
	seasons := map[int]struct{}{}
	for _, ep := range payload.Episodes {
		season := ep.Season
		if season <= 0 {
			season = 1
		}
		seasons[season] = struct{}{}
	}
	update["seasons_count"] = len(seasons)
	return h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": update})
}

func (h *IngestionHandler) appendParentLog(ctx context.Context, id, message, level string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{
		"$push": bson.M{
			"logs": models.IngestionLog{
				Timestamp: time.Now(),
				Message:   message,
				Level:     level,
			},
		},
		"$set": bson.M{
			"updated_at": time.Now(),
		},
	})
	return err
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func (h *IngestionHandler) markParentProcessing(ctx context.Context, id string) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": bson.M{
		"status":                models.IngestionStatusProcessing,
		"stage":                 string(models.IngestionStatusProcessing),
		"progress":              0,
		"processing_started_at": now,
		"started_at":            now,
		"updated_at":            now,
		"message":               "Extracting episodes",
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
		"message":      reason,
		"completed_at": now,
		"updated_at":   now,
	}})
}

func (h *IngestionHandler) completeParent(
	ctx context.Context, id string, seasons, episodes, childJobs int,
	seriesID primitive.ObjectID, seriesSlug string, missing []int,
) error {
	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return err
	}
	now := time.Now()
	message := fmt.Sprintf("Extracting finished: %d episodes discovered, %d child jobs created", episodes, childJobs)
	if len(missing) > 0 {
		message = fmt.Sprintf("%s. Missing: %v", message, missing)
	}
	update := bson.M{
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
		"message":            message,
	}
	if len(missing) > 0 {
		update["missing_episodes"] = missing
	}
	_, err = h.jobRepo.GetCollection().UpdateByID(ctx, objID, bson.M{"$set": update})
	return err
}

func determineHighestQuality(episodes []parserSerialEpisode) string {
	highest := 0
	quality := ""
	for _, ep := range episodes {
		for q := range ep.QualityURLs {
			var val int
			if _, err := fmt.Sscanf(q, "%dp", &val); err == nil {
				if val > highest {
					highest = val
					quality = q
				}
			} else if strings.EqualFold(q, "1080p ultra") || strings.EqualFold(q, "4k") {
				// Special cases
				val = 2160
				if strings.Contains(strings.ToLower(q), "1080p") {
					val = 1081 // Slightly higher than 1080
				}
				if val > highest {
					highest = val
					quality = q
				}
			}
		}
	}
	return quality
}

// buildSeriesStub returns an in-memory Series with slug/title/etc populated
// for use during extraction, WITHOUT persisting it. The real DB insert
// happens in finalizeSerialParent() once every child job is terminal.
func (h *IngestionHandler) buildSeriesStub(title string, payload *parserSerialResponse) *models.Series {
	return &models.Series{
		Slug:        slugifySerial(title),
		Title:       title,
		Description: payload.Description,
		PosterURL:   payload.Poster,
		BackdropURL: payload.Backdrop,
		Year:        payload.Year,
		Quality:     determineHighestQuality(payload.Episodes),
	}
}

// upsertSeries finds an existing series by slug or creates a new one (pending
// approval). The slug is derived deterministically from the title so re-
// importing the same serial targets the same row.
func (h *IngestionHandler) upsertSeries(ctx context.Context, title string, payload *parserSerialResponse) (*models.Series, error) {
	slug := slugifySerial(title)
	col := h.seriesRepo.Collection()

	quality := determineHighestQuality(payload.Episodes)

	var existing models.Series
	err := col.FindOne(ctx, bson.M{"slug": slug}).Decode(&existing)
	if err == nil {
		// Update quality if it's better or was empty
		if existing.Quality == "" || (quality != "" && quality != existing.Quality) {
			_, _ = col.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{"$set": bson.M{"quality": quality}})
			existing.Quality = quality
		}
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
		Quality:        quality,
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
		Quality:       determineHighestQuality([]parserSerialEpisode{ep}),
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
	source, sourceID string,
	parentID primitive.ObjectID,
	ep parserSerialEpisode,
	seasonNum int,
) (*models.IngestionJob, bool, error) {
	episodeSourceID := fmt.Sprintf("%s:s%02de%02d", sourceID, seasonNum, ep.Episode)

	// Already exists?
	existing, err := h.jobRepo.GetBySourceAndID(ctx, source, episodeSourceID)
	if err == nil && existing != nil {
		return existing, false, nil
	}

	episodePoster := ep.Poster
	if episodePoster == "" {
		episodePoster = series.PosterURL
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
		// Deferred-DB mode: SeriesID/SeasonID/EpisodeID are left zero. They
		// will be filled in by finalizeSerialParent() when it creates the
		// actual Series/Seasons/Episodes after every child job is terminal.
		SeriesSlug:       series.Slug,
		SeasonNumber:     seasonNum,
		EpisodeNumber:    ep.Episode,
		ParentJobID:      parentID,
		EpisodeTitle:     strings.TrimSpace(ep.Title),
		EpisodePosterURL: episodePoster,
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

// serialDetailsClient is a long-timeout HTTP client used only for the parser
// /serial-details endpoint. Series with 90+ episodes can legitimately take
// many minutes to scrape end-to-end; the shared 180s client would 504 here.
var serialDetailsClient = &http.Client{
	Timeout: 25 * time.Minute,
}

// fetchSerialDetailsWithRetry GETs the parser /serial-details endpoint, with
// up to 3 attempts and exponential backoff on transient gateway errors
// (502/503/504). Returns body, status, content-type, error.
func fetchSerialDetailsWithRetry(endpoint, parentID string) ([]byte, int, string, error) {
	var lastErr error
	var lastStatus int
	var lastCT string
	var lastBody []byte
	backoff := 5 * time.Second
	for attempt := 1; attempt <= 3; attempt++ {
		resp, err := serialDetailsClient.Get(endpoint)
		if err != nil {
			lastErr = fmt.Errorf("parser unreachable: %v", err)
			log.Printf("[series extractor] attempt=%d parent_job=%s err=%v", attempt, parentID, err)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode
		lastCT = resp.Header.Get("Content-Type")
		lastBody = body
		if resp.StatusCode == http.StatusBadGateway ||
			resp.StatusCode == http.StatusServiceUnavailable ||
			resp.StatusCode == http.StatusGatewayTimeout {
			log.Printf("[series extractor] gateway error attempt=%d parent_job=%s status=%d", attempt, parentID, resp.StatusCode)
			time.Sleep(backoff)
			backoff *= 2
			continue
		}
		return body, resp.StatusCode, lastCT, nil
	}
	if lastErr != nil {
		return lastBody, lastStatus, lastCT, lastErr
	}
	return lastBody, lastStatus, lastCT, nil
}

// looksLikeJSON returns true when the response is plausibly a JSON payload —
// either the Content-Type advertises it, or the body's first non-whitespace
// byte is `{` or `[`. HTML error pages from proxies (`<html>`) are rejected.
func looksLikeJSON(contentType string, body []byte) bool {
	if strings.Contains(strings.ToLower(contentType), "application/json") {
		return true
	}
	for _, b := range body {
		switch b {
		case ' ', '\t', '\n', '\r':
			continue
		case '{', '[':
			return true
		default:
			return false
		}
	}
	return false
}

func safeBody(b []byte) string {
	s := string(b)
	if len(s) > 300 {
		return s[:300] + "..."
	}
	return s
}
