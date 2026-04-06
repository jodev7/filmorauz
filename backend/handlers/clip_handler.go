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

	ctx := context.Background()
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
