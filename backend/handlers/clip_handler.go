package handlers

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
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

type ClipHandler struct {
	clipRepo  *repositories.ClipRepository
	parserURL string
}

func NewClipHandler(clipRepo *repositories.ClipRepository, parserURL string) *ClipHandler {
	return &ClipHandler{clipRepo: clipRepo, parserURL: parserURL}
}

func (h *ClipHandler) ListClips(c *gin.Context) {
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "50"), 10, 64)
	offset, _ := strconv.ParseInt(c.DefaultQuery("offset", "0"), 10, 64)

	movieIDStr := strings.TrimSpace(c.Query("movie_id"))
	seriesIDStr := strings.TrimSpace(c.Query("series_id"))
	episodeIDStr := strings.TrimSpace(c.Query("episode_id"))

	ctx := context.Background()

	// Scoped lookup (lazy-load a single content group's clips).
	if movieIDStr != "" || seriesIDStr != "" || episodeIDStr != "" {
		filter := bson.M{}
		if movieIDStr != "" {
			id, err := primitive.ObjectIDFromHex(movieIDStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie_id"})
				return
			}
			filter["movie_id"] = id
		}
		if seriesIDStr != "" {
			id, err := primitive.ObjectIDFromHex(seriesIDStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series_id"})
				return
			}
			filter["series_id"] = id
		}
		if episodeIDStr != "" {
			id, err := primitive.ObjectIDFromHex(episodeIDStr)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid episode_id"})
				return
			}
			filter["episode_id"] = id
		}
		clips, total, err := h.clipRepo.ListFiltered(ctx, filter, limit, offset)
		if err != nil {
			log.Printf("[ClipHandler] ListClips(scoped): failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clips"})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"data":   clips,
			"total":  total,
			"limit":  limit,
			"offset": offset,
		})
		return
	}

	clips, total, err := h.clipRepo.List(ctx, limit, offset)
	if err != nil {
		log.Printf("[ClipHandler] ListClips: failed to fetch clips: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clips"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data":   clips,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// ListClipGroups GET /api/admin/clips/groups
// Returns a (movies, series→season→episode) summary tree built via aggregation.
// Counts only — clip docs are loaded lazily via ListClips with filters.
func (h *ClipHandler) ListClipGroups(c *gin.Context) {
	ctx := context.Background()
	movies, series, total, err := h.clipRepo.GroupClips(ctx)
	if err != nil {
		log.Printf("[ClipHandler] ListClipGroups: failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to group clips"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"movies":          movies,
		"series":          series,
		"total_clips":     total,
		"total_contents":  len(movies) + len(series),
	})
}

func (h *ClipHandler) GetClipsByMovie(c *gin.Context) {
	movieIDStr := c.Param("movieId")
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	ctx := context.Background()
	clips, err := h.clipRepo.FindByMovieID(ctx, movieID)
	if err != nil {
		log.Printf("[ClipHandler] GetClipsByMovie: failed to fetch clips: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clips"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": clips,
	})
}

func resolveClipDownloadURL(clip *models.Clip) string {
	if clip == nil {
		return ""
	}

	raw := strings.TrimSpace(clip.URL)
	cfg := config.Current()

	if strings.HasPrefix(raw, "http://") || strings.HasPrefix(raw, "https://") {
		if strings.Contains(raw, "/media/") && cfg != nil && cfg.CDNBaseURL != "" {
			if idx := strings.Index(raw, "/media/"); idx != -1 {
				mediaPath := raw[idx+len("/media"):]
				if strings.HasPrefix(mediaPath, "/videos/") {
					return strings.TrimSuffix(cfg.CDNBaseURL, "/") + mediaPath
				}
			}
		}
		return raw
	}

	if cfg != nil {
		switch {
		case strings.HasPrefix(raw, "/videos/"):
			return strings.TrimSuffix(cfg.CDNBaseURL, "/") + raw
		case strings.HasPrefix(raw, "videos/"):
			return strings.TrimSuffix(cfg.CDNBaseURL, "/") + "/" + raw
		case raw != "":
			return cfg.GetCDNFileURL(raw)
		case clip.Path != "":
			return cfg.GetCDNFileURL(clip.Path)
		}
	}

	return raw
}

func clipCaptionTitle(clip *models.Clip) string {
	if clip == nil {
		return ""
	}
	if title := strings.TrimSpace(clip.DisplayTitle()); title != "" {
		return title
	}
	return "Klip"
}

// DownloadClip GET /api/admin/clips/:id/download
// Streams the clip through backend with attachment headers so the browser downloads it.
func (h *ClipHandler) DownloadClip(c *gin.Context) {
	idStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	ctx := context.Background()
	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil || clip == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	downloadURL := resolveClipDownloadURL(clip)
	if downloadURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "clip url is missing"})
		return
	}

	log.Printf("[ClipHandler] DownloadClip: clip=%s filename=%q url=%s", clip.ID.Hex(), clip.Filename, downloadURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare clip download"})
		return
	}
	req.Header.Set("User-Agent", "Mozilla/5.0")

	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[ClipHandler] DownloadClip: upstream request failed: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to fetch clip"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyPreview, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		log.Printf("[ClipHandler] DownloadClip: upstream status=%d body=%s", resp.StatusCode, string(bodyPreview))
		c.JSON(http.StatusBadGateway, gin.H{"error": fmt.Sprintf("clip download failed: upstream status %d", resp.StatusCode)})
		return
	}

	filename := strings.TrimSpace(clip.Filename)
	if filename == "" {
		filename = "clip.mp4"
	}
	if !strings.HasSuffix(strings.ToLower(filename), ".mp4") {
		filename += ".mp4"
	}

	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Cache-Control", "no-store")
	if resp.ContentLength > 0 {
		c.Header("Content-Length", fmt.Sprintf("%d", resp.ContentLength))
	}

	if _, err := io.Copy(c.Writer, resp.Body); err != nil {
		log.Printf("[ClipHandler] DownloadClip: stream copy failed: %v", err)
	}
}

func (h *ClipHandler) SaveClips(c *gin.Context) {
	var req struct {
		Clips []struct {
			MovieID     string `json:"movie_id"`
			MovieTitle  string `json:"movie_title"`
			MovieSlug   string `json:"movie_slug"`
			MovieCode   string `json:"movie_code"`
			Filename    string `json:"filename"`
			Path        string `json:"path"`
			URL         string `json:"url"`
			Duration    int    `json:"duration"`
			Sequence    int    `json:"sequence"`
			StorageType string `json:"storage_type"`
		} `json:"clips"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.Clips) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no clips provided"})
		return
	}

	ctx := context.Background()
	var clips []models.Clip
	for _, clipData := range req.Clips {
		movieID, err := primitive.ObjectIDFromHex(clipData.MovieID)
		if err != nil {
			log.Printf("[ClipHandler] SaveClips: invalid movie id %s: %v", clipData.MovieID, err)
			continue
		}
		clips = append(clips, models.Clip{
			MovieID:     movieID,
			MovieTitle:  clipData.MovieTitle,
			MovieSlug:   clipData.MovieSlug,
			MovieCode:   clipData.MovieCode,
			Filename:    clipData.Filename,
			Path:        clipData.Path,
			URL:         clipData.URL,
			Duration:    clipData.Duration,
			Sequence:    clipData.Sequence,
			StorageType: clipData.StorageType,
		})
	}

	if len(clips) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid clips to save"})
		return
	}

	log.Printf("[ClipHandler] SaveClips: saving %d clips for movie %s", len(clips), clips[0].MovieID.Hex())
	if err := h.clipRepo.CreateMany(ctx, clips); err != nil {
		log.Printf("[ClipHandler] SaveClips: failed to save clips: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save clips"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "clips saved",
		"count":   len(clips),
	})
}

// ListInstagramAccounts GET /api/admin/instagram/accounts
// Returns account names from INSTAGRAM_ACCOUNTS_JSON env (no credentials exposed).
func (h *ClipHandler) ListInstagramAccounts(c *gin.Context) {
	accounts := services.LoadInstagramAccounts()
	names := make([]string, len(accounts))
	for i, a := range accounts {
		names[i] = a.Name
	}
	c.JSON(http.StatusOK, gin.H{"accounts": names})
}

// UploadToInstagram POST /api/admin/clips/:id/instagram
// Body: {"account_names": ["main", "backup1"]}
// Uploads the clip as a Reel to each selected account via the parser service.
func (h *ClipHandler) UploadToInstagram(c *gin.Context) {
	idStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	var req struct {
		AccountNames []string `json:"account_names"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || len(req.AccountNames) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "account_names required"})
		return
	}

	ctx := context.Background()
	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	caption := fmt.Sprintf("%s\n\nKinoni profildagi bot orqali toping!\nKino Kodi: %s", clipCaptionTitle(clip), clip.MovieCode)

	type accountResult struct {
		Account        string `json:"account"`
		Status         string `json:"status"`
		ErrorType      string `json:"error_type,omitempty"`
		HumanMessage   string `json:"human_message,omitempty"`
		ActionRequired string `json:"action_required,omitempty"`
		Error          string `json:"error,omitempty"`
	}
	results := make([]accountResult, 0, len(req.AccountNames))
	overallStatus := "success"

	for _, name := range req.AccountNames {
		account := services.GetInstagramAccount(name)
		if account == nil {
			log.Printf("[Instagram] account not found: %s", name)
			results = append(results, accountResult{
				Account:      name,
				Status:       "failed",
				ErrorType:    "account_not_configured",
				HumanMessage: "Akkaunt sozlanmagan (INSTAGRAM_ACCOUNTS_JSON)",
				Error:        "account not configured",
			})
			overallStatus = "failed"
			continue
		}
		log.Printf("[Instagram] uploading clip=%s to account=%s url=%s", clip.ID.Hex(), name, clip.URL)
		uploadErr := services.UploadReelToInstagram(h.parserURL, clip.URL, caption, account)
		if uploadErr != nil {
			errorType := "publish_failed"
			humanMsg := "Yuklash muvaffaqiyatsiz tugadi"
			actionReq := ""
			if igErr, ok := uploadErr.(*services.InstagramUploadError); ok {
				errorType = igErr.ErrorType
				humanMsg = igErr.HumanMessage
				actionReq = igErr.ActionRequired
			}
			log.Printf("[Instagram] upload failed account=%s type=%s: %v", name, errorType, uploadErr)
			results = append(results, accountResult{
				Account:        name,
				Status:         "failed",
				ErrorType:      errorType,
				HumanMessage:   humanMsg,
				ActionRequired: actionReq,
				Error:          uploadErr.Error(),
			})
			overallStatus = "failed"
		} else {
			log.Printf("[Instagram] upload success account=%s", name)
			results = append(results, accountResult{Account: name, Status: "success"})
		}
	}

	if err := h.clipRepo.RecordInstagramUpload(ctx, clipID, overallStatus); err != nil {
		log.Printf("[Instagram] failed to record upload: %v", err)
	}

	c.JSON(http.StatusOK, gin.H{"results": results, "overall_status": overallStatus})
}

func (h *ClipHandler) DeleteClipsByMovie(c *gin.Context) {
	movieIDStr := c.Param("movieId")
	movieID, err := primitive.ObjectIDFromHex(movieIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	ctx := context.Background()
	if err := h.clipRepo.DeleteByMovieID(ctx, movieID); err != nil {
		log.Printf("[ClipHandler] DeleteClipsByMovie: failed to delete clips: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete clips"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "clips deleted"})
}
