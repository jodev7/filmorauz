package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// DeleteMovieCascade DELETE /api/admin/movies/:id/delete-cascade
func (h *MovieHandler) DeleteMovieCascade(c *gin.Context) {
	id := c.Param("id")
	movieID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id"})
		return
	}

	repo := repositories.NewDeleteJobRepository(h.db)
	job, err := repo.FindPending(c.Request.Context(), movieID)
	if err != nil || job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no pending delete job found"})
		return
	}

	job.CurrentStep = "Starting B2 deletion"
	now := time.Now()
	job.StartedAt = &now
	job.UpdatedAt = time.Now()
	repo.Update(context.Background(), job)

	// Define progress callback
	tracker := func(step string, progress int) {
		job.CurrentStep = step
		job.Progress = progress
		job.UpdatedAt = time.Now()
		repo.Update(context.Background(), job)
	}

	// Use background context for the actual deletion to prevent cancellation on timeout
	result, err := h.movieService.DeleteMovieWithProgress(id, tracker)
	if err != nil {
		job.Error = err.Error()
		job.Status = "failed"
		job.UpdatedAt = time.Now()
		repo.Update(context.Background(), job)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "result": result})
		return
	}

	job.CurrentStep = "Completed"
	job.Progress = 100
	job.Status = "completed"
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.UpdatedAt = time.Now()
	repo.Update(context.Background(), job)

	c.JSON(http.StatusOK, gin.H{"success": true, "result": result})
}

// DeleteSeriesCascade DELETE /api/admin/series/:id/delete-cascade
func (h *SeriesHandler) DeleteSeriesCascade(c *gin.Context) {
	id := c.Param("id")
	seriesID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid series id"})
		return
	}

	repo := repositories.NewDeleteJobRepository(h.db)
	job, err := repo.FindPending(c.Request.Context(), seriesID)
	if err != nil || job == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no pending delete job found"})
		return
	}

	job.CurrentStep = "Starting B2 deletion"
	now := time.Now()
	job.StartedAt = &now
	job.UpdatedAt = time.Now()
	repo.Update(context.Background(), job)

	// Define progress callback
	tracker := func(step string, progress int) {
		job.CurrentStep = step
		job.Progress = progress
		job.UpdatedAt = time.Now()
		repo.Update(context.Background(), job)
	}

	result, err := h.seriesService.DeleteSeriesWithProgress(seriesID, tracker)
	if err != nil {
		job.Error = err.Error()
		job.Status = "failed"
		job.UpdatedAt = time.Now()
		repo.Update(context.Background(), job)
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error(), "result": result})
		return
	}

	job.CurrentStep = "Completed"
	job.Progress = 100
	job.Status = "completed"
	completedAt := time.Now()
	job.CompletedAt = &completedAt
	job.UpdatedAt = time.Now()
	repo.Update(context.Background(), job)

	c.JSON(http.StatusOK, gin.H{"success": true, "result": result})
}
