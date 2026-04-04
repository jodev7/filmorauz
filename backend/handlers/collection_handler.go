package handlers

import (
	"log"
	"net/http"

	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
)

type CollectionHandler struct {
	collectionService *services.CollectionService
}

func NewCollectionHandler(collectionService *services.CollectionService) *CollectionHandler {
	return &CollectionHandler{collectionService: collectionService}
}

// --- Admin Handlers ---

// CreateCollection POST /api/v1/admin/collections
func (h *CollectionHandler) CreateCollection(c *gin.Context) {
	var input models.CollectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	collection, err := h.collectionService.Create(c.Request.Context(), input)
	if err != nil {
		log.Printf("[ERROR] CreateCollection: failed to create collection: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": collection})
}

// UpdateCollection PUT /api/v1/admin/collections/:id
func (h *CollectionHandler) UpdateCollection(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection id is required"})
		return
	}

	var input models.CollectionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	collection, err := h.collectionService.Update(c.Request.Context(), id, input)
	if err != nil {
		log.Printf("[ERROR] UpdateCollection: failed to update collection: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": collection})
}

// DeleteCollection DELETE /api/v1/admin/collections/:id
func (h *CollectionHandler) DeleteCollection(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection id is required"})
		return
	}

	err := h.collectionService.Delete(c.Request.Context(), id)
	if err != nil {
		log.Printf("[ERROR] DeleteCollection: failed to delete collection: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "collection deleted"})
}

// ListCollections GET /api/v1/admin/collections
func (h *CollectionHandler) ListCollections(c *gin.Context) {
	collections, err := h.collectionService.GetAll(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] ListCollections: failed to list collections: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list collections"})
		return
	}

	if collections == nil {
		collections = []models.Collection{}
	}

	c.JSON(http.StatusOK, gin.H{"data": collections})
}

// GetCollection GET /api/v1/admin/collections/:id
func (h *CollectionHandler) GetCollection(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "collection id is required"})
		return
	}

	collection, err := h.collectionService.GetByID(c.Request.Context(), id)
	if err != nil {
		log.Printf("[ERROR] GetCollection: failed to get collection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get collection"})
		return
	}

	if collection == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": collection})
}

// --- Public Handlers ---

// GetFeaturedCollections GET /api/v1/collections/featured
func (h *CollectionHandler) GetFeaturedCollections(c *gin.Context) {
	collections, err := h.collectionService.GetFeatured(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] GetFeaturedCollections: failed to get featured collections: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get featured collections"})
		return
	}

	if collections == nil {
		collections = []models.CollectionWithMovies{}
	}

	c.JSON(http.StatusOK, gin.H{"data": collections})
}

// GetCollections GET /api/v1/collections
func (h *CollectionHandler) GetCollections(c *gin.Context) {
	collections, err := h.collectionService.GetPublished(c.Request.Context())
	if err != nil {
		log.Printf("[ERROR] GetCollections: failed to get collections: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get collections"})
		return
	}

	if collections == nil {
		collections = []models.CollectionWithMovies{}
	}

	c.JSON(http.StatusOK, gin.H{"data": collections})
}

// GetCollectionBySlug GET /api/v1/collections/slug/:slug
func (h *CollectionHandler) GetCollectionBySlug(c *gin.Context) {
	slug := c.Param("slug")
	if slug == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "slug is required"})
		return
	}

	collection, err := h.collectionService.GetBySlug(c.Request.Context(), slug)
	if err != nil {
		log.Printf("[ERROR] GetCollectionBySlug: failed to get collection: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get collection"})
		return
	}

	if collection == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": collection})
}
