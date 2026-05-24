package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type PublishJobHandler struct {
	jobRepo    *repositories.PublishJobRepository
	clipRepo   *repositories.ClipRepository
	seriesRepo *repositories.SeriesRepository
	parserURL  string
}

func NewPublishJobHandler(
	jobRepo *repositories.PublishJobRepository,
	clipRepo *repositories.ClipRepository,
	seriesRepo *repositories.SeriesRepository,
	parserURL string,
) *PublishJobHandler {
	return &PublishJobHandler{jobRepo: jobRepo, clipRepo: clipRepo, seriesRepo: seriesRepo, parserURL: parserURL}
}

// contentKindForClip returns the PublishJob.ContentKind value for a clip
// — "series" when the clip belongs to a serial episode, "movie"
// otherwise. Reuses the same heuristic as the caption builder so the
// classification stays consistent between job creation and publish.
func contentKindForClip(clip *models.Clip) string {
	if clip != nil && services.IsSeriesClip(clip) {
		return "series"
	}
	return "movie"
}

// ListAccounts GET /api/admin/publish/accounts
// Returns all configured account names grouped by platform.
func (h *PublishJobHandler) ListAccounts(c *gin.Context) {
	igAccounts := services.LoadInstagramAccounts()
	ytAccounts := services.LoadYouTubeAccounts()
	ttAccounts := services.LoadTikTokAccounts()

	igNames := make([]string, len(igAccounts))
	for i, a := range igAccounts {
		igNames[i] = a.Name
	}
	ytNames := make([]string, len(ytAccounts))
	for i, a := range ytAccounts {
		ytNames[i] = a.Name
	}
	ttNames := make([]string, len(ttAccounts))
	for i, a := range ttAccounts {
		ttNames[i] = a.Name
	}

	c.JSON(http.StatusOK, gin.H{
		"instagram": igNames,
		"youtube":   ytNames,
		"tiktok":    ttNames,
	})
}

// UploadNow POST /api/admin/clips/:id/publish/now
// Synchronously uploads to each platform/account combo and returns per-job results.
// Body: {"jobs": [{"platform":"instagram","account_name":"main"}, ...]}
func (h *PublishJobHandler) UploadNow(c *gin.Context) {
	clipIDStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(clipIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	var req struct {
		Jobs []struct {
			Platform    string `json:"platform"`
			AccountName string `json:"account_name"`
		} `json:"jobs"`
		Caption string `json:"caption"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Jobs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jobs array is required"})
		return
	}

	ctx := context.Background()
	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	type jobResult struct {
		Platform    string `json:"platform"`
		AccountName string `json:"account_name"`
		Status      string `json:"status"`
		Error       string `json:"error,omitempty"`
	}

	results := make([]jobResult, 0, len(req.Jobs))
	overallStatus := "success"

	userID, _ := c.Get("user_id")
	createdBy := ""
	if userID != nil {
		createdBy, _ = userID.(string)
	}

	for _, j := range req.Jobs {
		clipCode := services.ResolveInstagramClipCode(ctx, clip, h.seriesRepo)
		caption := strings.TrimSpace(req.Caption)
		if caption == "" {
			caption = services.BuildClipCaptionAI(clip.Caption, clip.Hashtags, clipCode, services.IsSeriesClip(clip))
		}
		job := &models.PublishJob{
			ClipID:          clipID,
			ClipURL:         clip.URL,
			MovieTitle:      clip.DisplayTitle(),
			MovieSlug:       clip.DisplaySlug(),
			MovieCode:       clipCode,
			ContentKind:     contentKindForClip(clip),
			CaptionOverride: caption,
			Platform:        j.Platform,
			AccountName:     j.AccountName,
			CreatedBy:       createdBy,
		}
		log.Printf("[PublishNow] clip=%s platform=%s account=%s", clipID.Hex(), j.Platform, j.AccountName)
		// Allocate the job ID up-front so ExecutePlatformUpload can derive a
		// stable publish_key for the sidecar.
		job.ID = primitive.NewObjectID()
		mediaID, postURL, uploadErr := services.ExecutePlatformUpload(h.parserURL, job)
		if uploadErr != nil {
			log.Printf("[PublishNow] failed platform=%s account=%s: %v", j.Platform, j.AccountName, uploadErr)
			job.Status = models.PublishJobStatusFailed
			job.Error = uploadErr.Error()
			results = append(results, jobResult{
				Platform:    j.Platform,
				AccountName: j.AccountName,
				Status:      "failed",
				Error:       uploadErr.Error(),
			})
			overallStatus = "failed"
			// Record failure for Instagram on the clip document
			if j.Platform == models.PublishPlatformInstagram {
				_ = h.clipRepo.RecordInstagramUpload(ctx, clipID, "failed")
			}
		} else {
			log.Printf("[PublishNow] success platform=%s account=%s", j.Platform, j.AccountName)
			job.Status = models.PublishJobStatusSuccess
			job.InstagramMediaID = mediaID
			job.InstagramPostURL = postURL
			now := time.Now().UTC()
			job.PublishedAt = &now
			results = append(results, jobResult{
				Platform:    j.Platform,
				AccountName: j.AccountName,
				Status:      "success",
			})
			// Record Instagram upload on the clip document (legacy field used by dashboard)
			if j.Platform == models.PublishPlatformInstagram {
				if err := h.clipRepo.RecordInstagramUpload(ctx, clipID, "success"); err != nil {
					log.Printf("[PublishNow] RecordInstagramUpload error clip=%s: %v", clipID.Hex(), err)
				}
			}
		}
		// Persist every upload attempt (all platforms) as a completed PublishJob record
		if err := h.jobRepo.RecordCompleted(job); err != nil {
			log.Printf("[PublishNow] RecordCompleted error platform=%s account=%s: %v", j.Platform, j.AccountName, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "overall_status": overallStatus})
}

// Schedule POST /api/admin/clips/:id/publish/schedule
// Creates one pending PublishJob per platform/account combo.
// Body: {"jobs": [{"platform":"instagram","account_name":"main"},...], "scheduled_for": "RFC3339"}
func (h *PublishJobHandler) Schedule(c *gin.Context) {
	clipIDStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(clipIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	var req struct {
		Jobs []struct {
			Platform    string `json:"platform"`
			AccountName string `json:"account_name"`
		} `json:"jobs"`
		ScheduledFor string `json:"scheduled_for"` // RFC3339
		Caption      string `json:"caption"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Jobs) == 0 || req.ScheduledFor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jobs and scheduled_for are required"})
		return
	}

	scheduledFor, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be RFC3339 (e.g. 2024-01-15T14:30:00+05:00)"})
		return
	}
	if scheduledFor.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be in the future"})
		return
	}

	ctx := context.Background()
	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	userID, _ := c.Get("user_id")
	createdBy := ""
	if userID != nil {
		createdBy, _ = userID.(string)
	}

	jobs := make([]*models.PublishJob, 0, len(req.Jobs))
	scheduleCode := services.ResolveInstagramClipCode(ctx, clip, h.seriesRepo)
	scheduleCaption := strings.TrimSpace(req.Caption)
	if scheduleCaption == "" {
		scheduleCaption = services.BuildClipCaptionAI(clip.Caption, clip.Hashtags, scheduleCode, services.IsSeriesClip(clip))
	}
	for _, j := range req.Jobs {
		jobs = append(jobs, &models.PublishJob{
			ClipID:          clipID,
			ClipURL:         clip.URL,
			MovieTitle:      clip.DisplayTitle(),
			MovieSlug:       clip.DisplaySlug(),
			MovieCode:       scheduleCode,
			ContentKind:     contentKindForClip(clip),
			CaptionOverride: scheduleCaption,
			Platform:        j.Platform,
			AccountName:     j.AccountName,
			ScheduledFor:    scheduledFor.UTC(),
			CreatedBy:       createdBy,
		})
	}

	if err := h.jobRepo.CreateMany(jobs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create jobs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": jobs, "count": len(jobs)})
}

// ListForClip GET /api/admin/clips/:id/publish/jobs
func (h *PublishJobHandler) ListForClip(c *gin.Context) {
	clipIDStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(clipIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	jobs, err := h.jobRepo.FindByClipID(clipID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}
	if jobs == nil {
		jobs = []models.PublishJob{}
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs})
}

// ListAll GET /api/admin/publish/jobs
func (h *PublishJobHandler) ListAll(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)
	platform := c.Query("platform")

	jobs, total, err := h.jobRepo.ListAll(limit, offset, platform)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}
	if jobs == nil {
		jobs = []models.PublishJob{}
	}
	c.JSON(http.StatusOK, gin.H{
		"data":   jobs,
		"items":  jobs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// UpdateTime PATCH /api/admin/publish/jobs/:jobId
// Body: {"scheduled_for": "RFC3339"}
func (h *PublishJobHandler) UpdateTime(c *gin.Context) {
	idStr := c.Param("jobId")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}

	var req struct {
		ScheduledFor string `json:"scheduled_for"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.ScheduledFor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for is required"})
		return
	}

	newTime, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be RFC3339"})
		return
	}
	if newTime.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be in the future"})
		return
	}

	updated, err := h.jobRepo.UpdateScheduledTime(id, newTime)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update job"})
		return
	}
	if !updated {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found or no longer pending"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job updated"})
}

// Cancel DELETE /api/admin/publish/jobs/:jobId
func (h *PublishJobHandler) Cancel(c *gin.Context) {
	idStr := c.Param("jobId")
	id, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid job id"})
		return
	}
	cancelled, err := h.jobRepo.Cancel(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel job"})
		return
	}
	if !cancelled {
		c.JSON(http.StatusNotFound, gin.H{"error": "job not found or already executing/completed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "job cancelled"})
}
