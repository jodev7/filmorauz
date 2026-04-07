package handlers

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
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

	log.Printf("[ClipHandler] ListClips: limit=%d, offset=%d", limit, offset)

	ctx := context.Background()
	clips, total, err := h.clipRepo.List(ctx, limit, offset)
	if err != nil {
		log.Printf("[ClipHandler] ListClips: failed to fetch clips: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to fetch clips"})
		return
	}

	log.Printf("[ClipHandler] ListClips: returning %d clips (total=%d)", len(clips), total)

	c.JSON(http.StatusOK, gin.H{
		"data":   clips,
		"total":  total,
		"limit":  limit,
		"offset": offset,
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

	caption := fmt.Sprintf("%s\n\nKinoni profildagi botdan toping!", clip.MovieTitle)

	type accountResult struct {
		Account string `json:"account"`
		Status  string `json:"status"`
		Error   string `json:"error,omitempty"`
	}
	results := make([]accountResult, 0, len(req.AccountNames))
	overallStatus := "success"

	for _, name := range req.AccountNames {
		account := services.GetInstagramAccount(name)
		if account == nil {
			log.Printf("[Instagram] account not found: %s", name)
			results = append(results, accountResult{Account: name, Status: "failed", Error: "account not configured"})
			overallStatus = "failed"
			continue
		}
		log.Printf("[Instagram] uploading clip=%s to account=%s url=%s", clip.ID.Hex(), name, clip.URL)
		uploadErr := services.UploadReelToInstagram(h.parserURL, clip.URL, caption, account)
		if uploadErr != nil {
			log.Printf("[Instagram] upload failed account=%s: %v", name, uploadErr)
			results = append(results, accountResult{Account: name, Status: "failed", Error: uploadErr.Error()})
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
