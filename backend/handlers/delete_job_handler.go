package handlers

import (
	"net/http"

	"github.com/filmorauz/backend/repositories"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type DeleteJobHandler struct {
	repo *repositories.DeleteJobRepository
}

func NewDeleteJobHandler(repo *repositories.DeleteJobRepository) *DeleteJobHandler {
	return &DeleteJobHandler{repo: repo}
}

func (h *DeleteJobHandler) GetJob(c *gin.Context) {
	id, err := primitive.ObjectIDFromHex(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	job, err := h.repo.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Job not found"})
		return
	}
	c.JSON(http.StatusOK, job)
}
