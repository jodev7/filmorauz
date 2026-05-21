package handlers

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const (
	maxContentVideoSize int64 = 250 * 1024 * 1024 // 250MB — comfortably above
	// what a CapCut export of a Reels-length clip will ever produce.
)

var allowedContentVideoTypes = map[string]bool{
	"video/mp4":       true,
	"video/quicktime": true,
	"video/webm":      true,
	"application/octet-stream": true, // some browsers send this for .mov
}

type ContentHandler struct {
	folderRepo *repositories.ContentFolderRepository
	clipRepo   *repositories.ContentClipRepository
	jobRepo    *repositories.PublishJobRepository
	uploader   *UploadHandler
	cfg        *config.Config
	parserURL  string
}

func NewContentHandler(
	folderRepo *repositories.ContentFolderRepository,
	clipRepo *repositories.ContentClipRepository,
	jobRepo *repositories.PublishJobRepository,
	uploader *UploadHandler,
	cfg *config.Config,
	parserURL string,
) *ContentHandler {
	return &ContentHandler{
		folderRepo: folderRepo,
		clipRepo:   clipRepo,
		jobRepo:    jobRepo,
		uploader:   uploader,
		cfg:        cfg,
		parserURL:  parserURL,
	}
}

// ── Folders ──────────────────────────────────────────────────────────────

var slugSanitizer = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugSanitizer.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = fmt.Sprintf("folder-%d", time.Now().UnixNano())
	}
	return s
}

func (h *ContentHandler) ListFolders(c *gin.Context) {
	folders, err := h.folderRepo.ListAll(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list folders"})
		return
	}
	if folders == nil {
		folders = []models.ContentFolder{}
	}
	c.JSON(http.StatusOK, gin.H{"data": folders})
}

type folderInput struct {
	Title       string `json:"title"`
	PosterURL   string `json:"poster_url"`
	Description string `json:"description"`
	SortOrder   int    `json:"sort_order"`
}

func (h *ContentHandler) CreateFolder(c *gin.Context) {
	var in folderInput
	if err := c.ShouldBindJSON(&in); err != nil || strings.TrimSpace(in.Title) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "title is required"})
		return
	}
	ctx := c.Request.Context()
	slug := slugify(in.Title)
	// Ensure unique by suffixing -2, -3, … if needed.
	base := slug
	for i := 2; i < 100; i++ {
		exists, err := h.folderRepo.SlugExists(ctx, slug, primitive.NilObjectID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "slug check failed"})
			return
		}
		if !exists {
			break
		}
		slug = fmt.Sprintf("%s-%d", base, i)
	}
	folder := &models.ContentFolder{
		Title:       strings.TrimSpace(in.Title),
		Slug:        slug,
		PosterURL:   strings.TrimSpace(in.PosterURL),
		Description: strings.TrimSpace(in.Description),
		SortOrder:   in.SortOrder,
	}
	if err := h.folderRepo.Create(ctx, folder); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": folder})
}

func (h *ContentHandler) GetFolder(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	ctx := c.Request.Context()
	folder, err := h.folderRepo.GetByID(ctx, id)
	if err != nil || folder == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": folder})
}

func (h *ContentHandler) UpdateFolder(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	var in folderInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	patch := bson.M{}
	if strings.TrimSpace(in.Title) != "" {
		patch["title"] = strings.TrimSpace(in.Title)
	}
	if in.PosterURL != "" {
		patch["poster_url"] = strings.TrimSpace(in.PosterURL)
	}
	if in.Description != "" {
		patch["description"] = strings.TrimSpace(in.Description)
	}
	patch["sort_order"] = in.SortOrder
	if err := h.folderRepo.Update(c.Request.Context(), id, patch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *ContentHandler) DeleteFolder(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	ctx := c.Request.Context()
	if _, err := h.clipRepo.DeleteByFolder(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to remove folder clips"})
		return
	}
	if err := h.folderRepo.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete folder"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// UploadFolderPoster — image upload reusing the existing direct-upload pipeline.
func (h *ContentHandler) UploadFolderPoster(c *gin.Context) {
	h.uploader.handleDirectUpload(c, uploadSpec{
		FieldName:    "file",
		Folder:       "images/content",
		Label:        "content_poster",
		MaxSizeBytes: maxFormImageSize,
		AllowedTypes: allowedFormImageTypes,
		FallbackExt:  ".jpg",
		ReturnPath:   "",
	})
}

// ── Clips ────────────────────────────────────────────────────────────────

func (h *ContentHandler) ListClips(c *gin.Context) {
	folderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	clips, err := h.clipRepo.ListByFolder(c.Request.Context(), folderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list clips"})
		return
	}
	if clips == nil {
		clips = []models.ContentClip{}
	}
	c.JSON(http.StatusOK, gin.H{"data": clips})
}

// UploadClip — multipart: video (file) + title + caption.
func (h *ContentHandler) UploadClip(c *gin.Context) {
	folderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	ctx := c.Request.Context()
	folder, err := h.folderRepo.GetByID(ctx, folderID)
	if err != nil || folder == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "folder not found"})
		return
	}

	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "video file is required"})
		return
	}
	defer file.Close()

	data, contentType, err := readAndValidateUpload(file, maxContentVideoSize, allowedContentVideoTypes)
	if err != nil {
		status := http.StatusBadRequest
		if strings.Contains(err.Error(), "too large") {
			status = http.StatusRequestEntityTooLarge
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// .mov sometimes detected as octet-stream — accept by extension as a safety net.
	if contentType == "application/octet-stream" {
		ext := strings.ToLower(filepath.Ext(header.Filename))
		if ext != ".mov" && ext != ".mp4" && ext != ".webm" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video type"})
			return
		}
		contentType = "video/quicktime"
	}

	ext := safeUploadExt(header.Filename, contentType, ".mp4")
	objectKey := fmt.Sprintf("content/%s/%d%s", folderID.Hex(), time.Now().UnixNano(), ext)

	url, err := h.uploader.storeUploadedFile(objectKey, data, contentType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store video"})
		return
	}

	title := strings.TrimSpace(c.PostForm("title"))
	if title == "" {
		title = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}
	caption := strings.TrimSpace(c.PostForm("caption"))

	storageType := "local"
	if !h.cfg.IsDev {
		storageType = "b2"
	}

	clip := &models.ContentClip{
		FolderID:    folderID,
		Title:       title,
		Filename:    header.Filename,
		Path:        objectKey,
		URL:         url,
		Caption:     caption,
		StorageType: storageType,
		Size:        int64(len(data)),
	}
	if err := h.clipRepo.Create(ctx, clip); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record clip"})
		return
	}
	if err := h.folderRepo.IncrementClipsCount(ctx, folderID, 1); err != nil {
		// non-fatal; count drift is recoverable
		_ = err
	}
	c.JSON(http.StatusOK, gin.H{"data": clip})
}

type clipPatch struct {
	Title   string `json:"title"`
	Caption string `json:"caption"`
}

func (h *ContentHandler) UpdateClip(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("clipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	var in clipPatch
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	patch := bson.M{}
	if strings.TrimSpace(in.Title) != "" {
		patch["title"] = strings.TrimSpace(in.Title)
	}
	// Allow clearing caption explicitly.
	patch["caption"] = strings.TrimSpace(in.Caption)
	if err := h.clipRepo.Update(c.Request.Context(), id, patch); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update clip"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "ok"})
}

func (h *ContentHandler) DeleteClip(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("clipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	ctx := c.Request.Context()
	clip, err := h.clipRepo.GetByID(ctx, id)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}
	if err := h.clipRepo.Delete(ctx, id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete clip"})
		return
	}
	if err := h.folderRepo.IncrementClipsCount(ctx, clip.FolderID, -1); err != nil {
		_ = err
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

// ── Publish (now + schedule) ─────────────────────────────────────────────

type contentJobReq struct {
	Platform    string `json:"platform"`
	AccountName string `json:"account_name"`
}

type contentPublishNowReq struct {
	Jobs []contentJobReq `json:"jobs"`
}

type contentScheduleReq struct {
	Jobs         []contentJobReq `json:"jobs"`
	ScheduledFor string          `json:"scheduled_for"` // RFC3339
}

// PublishNow synchronously uploads to each selected platform/account.
func (h *ContentHandler) PublishNow(c *gin.Context) {
	clipID, err := primitive.ObjectIDFromHex(c.Param("clipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	var req contentPublishNowReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Jobs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jobs array is required"})
		return
	}
	ctx := c.Request.Context()
	clip, err := h.clipRepo.GetByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	userID, _ := c.Get("user_id")
	createdBy, _ := userID.(string)

	type jobResult struct {
		Platform    string `json:"platform"`
		AccountName string `json:"account_name"`
		Status      string `json:"status"`
		Error       string `json:"error,omitempty"`
	}
	results := make([]jobResult, 0, len(req.Jobs))
	overall := "success"

	for _, j := range req.Jobs {
		job := &models.PublishJob{
			ID:              primitive.NewObjectID(),
			SourceKind:      "content",
			ContentClipID:   clipID,
			ContentFolderID: clip.FolderID,
			ClipURL:         clip.URL,
			MovieTitle:      clip.Title,
			CaptionOverride: clip.Caption,
			Platform:        j.Platform,
			AccountName:     j.AccountName,
			CreatedBy:       createdBy,
		}
		mediaID, postURL, uploadErr := services.ExecutePlatformUpload(h.parserURL, job)
		if uploadErr != nil {
			job.Status = models.PublishJobStatusFailed
			job.Error = uploadErr.Error()
			results = append(results, jobResult{Platform: j.Platform, AccountName: j.AccountName, Status: "failed", Error: uploadErr.Error()})
			overall = "failed"
			_ = h.clipRepo.RecordUpload(ctx, clipID, "failed")
		} else {
			job.Status = models.PublishJobStatusSuccess
			job.InstagramMediaID = mediaID
			job.InstagramPostURL = postURL
			now := time.Now().UTC()
			job.PublishedAt = &now
			results = append(results, jobResult{Platform: j.Platform, AccountName: j.AccountName, Status: "success"})
			_ = h.clipRepo.RecordUpload(ctx, clipID, "success")
		}
		_ = h.jobRepo.RecordCompleted(job)
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "overall_status": overall})
}

func (h *ContentHandler) Schedule(c *gin.Context) {
	clipID, err := primitive.ObjectIDFromHex(c.Param("clipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	var req contentScheduleReq
	if err := c.ShouldBindJSON(&req); err != nil || len(req.Jobs) == 0 || req.ScheduledFor == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "jobs and scheduled_for are required"})
		return
	}
	scheduledFor, err := time.Parse(time.RFC3339, req.ScheduledFor)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be RFC3339"})
		return
	}
	if scheduledFor.Before(time.Now()) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scheduled_for must be in the future"})
		return
	}
	ctx := c.Request.Context()
	clip, err := h.clipRepo.GetByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}
	userID, _ := c.Get("user_id")
	createdBy, _ := userID.(string)

	jobs := make([]*models.PublishJob, 0, len(req.Jobs))
	for _, j := range req.Jobs {
		jobs = append(jobs, &models.PublishJob{
			SourceKind:      "content",
			ContentClipID:   clipID,
			ContentFolderID: clip.FolderID,
			ClipURL:         clip.URL,
			MovieTitle:      clip.Title,
			CaptionOverride: clip.Caption,
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

// ListJobs returns PublishJob rows linked to a content clip, used by the UI
// to render scheduled / success / failed badges.
func (h *ContentHandler) ListJobs(c *gin.Context) {
	clipID, err := primitive.ObjectIDFromHex(c.Param("clipId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}
	jobs, err := h.jobRepo.FindByContentClipID(clipID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}
	if jobs == nil {
		jobs = []models.PublishJob{}
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs})
}

// ListAllJobs — folder-wide jobs view (used by the folder page header).
func (h *ContentHandler) ListAllFolderJobs(c *gin.Context) {
	folderID, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid folder id"})
		return
	}
	jobs, err := h.jobRepo.FindByContentFolderID(folderID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list jobs"})
		return
	}
	if jobs == nil {
		jobs = []models.PublishJob{}
	}
	c.JSON(http.StatusOK, gin.H{"data": jobs})
}

// Compile-time check: contentHandler context lookup helpers (silence linter).
var _ = context.Background
