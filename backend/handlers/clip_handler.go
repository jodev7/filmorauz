package handlers

import (
	"context"
	"log"
	"net/http"
	"strconv"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type ClipHandler struct {
	clipRepo *repositories.ClipRepository
}

func NewClipHandler(clipRepo *repositories.ClipRepository) *ClipHandler {
	return &ClipHandler{clipRepo: clipRepo}
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

// UploadToInstagram POST /api/admin/clips/:id/instagram
// Triggers an Instagram upload for the given clip and records the result.
// The actual upload logic is delegated to instagramUpload() below — replace
// that function body when the real Instagram API is integrated.
func (h *ClipHandler) UploadToInstagram(c *gin.Context) {
	idStr := c.Param("id")
	clipID, err := primitive.ObjectIDFromHex(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid clip id"})
		return
	}

	ctx := context.Background()

	clip, err := h.clipRepo.FindByID(ctx, clipID)
	if err != nil {
		log.Printf("[Instagram] clip not found: %s — %v", idStr, err)
		c.JSON(http.StatusNotFound, gin.H{"error": "clip not found"})
		return
	}

	log.Printf("[Instagram] upload requested — clip=%s url=%s", clip.ID.Hex(), clip.URL)

	status := instagramUpload(clip)

	log.Printf("[Instagram] upload result — clip=%s status=%s", clip.ID.Hex(), status)

	if err := h.clipRepo.RecordInstagramUpload(ctx, clipID, status); err != nil {
		log.Printf("[Instagram] failed to record upload: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "upload tracking failed"})
		return
	}

	if status == "success" {
		c.JSON(http.StatusOK, gin.H{"message": "uploaded to Instagram", "status": status})
	} else {
		c.JSON(http.StatusOK, gin.H{"message": "upload failed", "status": status})
	}
}

// instagramUpload is the integration point for the real Instagram API.
// Replace the body of this function when the API is ready.
// Returns "success" or "failed".
func instagramUpload(clip interface{}) string {
	// TODO: integrate Instagram Graph API here.
	// clip has fields: URL, Filename, MovieTitle, etc.
	// For now, return "success" so the tracking flow is exercised end-to-end.
	log.Printf("[Instagram] placeholder upload — real API not connected yet")
	return "success"
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
