package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/filmorauz/backend/models"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

const ContentTypeBulkParent = "bulk_parent"

type BulkImportRequest struct {
	Source      string `json:"source" binding:"required"`
	CategoryURL string `json:"category_url" binding:"required"`
	PageStart   int    `json:"page_start"`
	PageEnd     int    `json:"page_end"`
	Type        string `json:"type"` // "movie" or "serial"
}

// BulkImportFromCategory starts a background process to import multiple items from a category
// POST /api/admin/ingestion/bulk-import
func (h *IngestionHandler) BulkImportFromCategory(c *gin.Context) {
	var req BulkImportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.PageStart < 1 {
		req.PageStart = 1
	}
	if req.PageEnd < req.PageStart {
		req.PageEnd = req.PageStart
	}

	// Limit bulk import to 50 pages at once to avoid overwhelming the system
	if req.PageEnd-req.PageStart > 50 {
		req.PageEnd = req.PageStart + 50
	}

	ctx := context.Background()

	// Create a bulk parent job to track progress
	title := fmt.Sprintf("Bulk Import: %s (%s)", req.Source, req.CategoryURL)
	parent := &models.IngestionJob{
		Title:       title,
		Source:      req.Source,
		SourceID:    fmt.Sprintf("bulk:%s:%d-%d", req.Source, req.PageStart, req.PageEnd),
		DetailURL:   req.CategoryURL,
		Status:      models.IngestionStatusQueued,
		Stage:       string(models.IngestionStatusQueued),
		ContentType: ContentTypeBulkParent,
		Steps:       models.JobSteps{},
		Logs:        []models.IngestionLog{},
		Message:     fmt.Sprintf("Bulk import queued for pages %d to %d", req.PageStart, req.PageEnd),
	}

	if err := h.jobRepo.Create(ctx, parent); err != nil {
		log.Printf("[BULK] Failed to create parent job: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to queue bulk import", "details": err.Error()})
		return
	}

	log.Printf("[BULK] Queued bulk import job=%s source=%s pages=%d-%d", parent.ID.Hex(), req.Source, req.PageStart, req.PageEnd)

	// Start async crawler
	go h.runBulkImportAsync(parent.ID, req)

	c.JSON(http.StatusAccepted, gin.H{
		"ok":      true,
		"job_id":  parent.ID.Hex(),
		"message": "Bulk import boshlandi",
	})
}

func (h *IngestionHandler) runBulkImportAsync(parentID primitive.ObjectID, req BulkImportRequest) {
	ctx := context.Background()
	
	h.jobRepo.UpdateStatus(ctx, parentID.Hex(), models.IngestionStatusProcessing, 5)
	h.appendParentLog(ctx, parentID.Hex(), "Bulk import process started", "info")

	totalCreated := 0
	totalSkipped := 0
	totalPages := req.PageEnd - req.PageStart + 1

	for page := req.PageStart; page <= req.PageEnd; page++ {
		h.jobRepo.GetCollection().UpdateByID(ctx, parentID, bson.M{
			"$set": bson.M{
				"message":  fmt.Sprintf("Crawling page %d of %d", page-req.PageStart+1, totalPages),
				"progress": int(float64(page-req.PageStart) / float64(totalPages) * 100),
			},
		})

		items, hasMore, err := h.fetchCatalogPage(req.Source, req.CategoryURL, page, req.Type)
		if err != nil {
			h.appendParentLog(ctx, parentID.Hex(), fmt.Sprintf("Error fetching page %d: %v", page, err), "error")
			continue
		}

		if len(items) == 0 {
			h.appendParentLog(ctx, parentID.Hex(), fmt.Sprintf("No items found on page %d", page), "warn")
			if !hasMore {
				break
			}
			continue
		}

		h.appendParentLog(ctx, parentID.Hex(), fmt.Sprintf("Found %d items on page %d", len(items), page), "info")

		for _, item := range items {
			// Trigger import for each item
			// We reuse the ImportFromCatalog logic but without the Gin context
			created, err := h.importSingleItemFromCatalog(ctx, req.Source, item)
			if err != nil {
				log.Printf("[BULK] Error importing item %s: %v", item.DetailURL, err)
				h.appendParentLog(ctx, parentID.Hex(), fmt.Sprintf("Failed to import %s: %v", item.Title, err), "error")
				continue
			}
			if created {
				totalCreated++
			} else {
				totalSkipped++
			}
		}

		if !hasMore {
			h.appendParentLog(ctx, parentID.Hex(), "Reached end of catalog", "info")
			break
		}
		
		// Small delay to be nice to the parser and source site
		time.Sleep(1 * time.Second)
	}

	h.jobRepo.GetCollection().UpdateByID(ctx, parentID, bson.M{
		"$set": bson.M{
			"status":             models.IngestionStatusCompleted,
			"stage":              string(models.IngestionStatusCompleted),
			"progress":           100,
			"child_jobs_created": totalCreated,
			"message":            fmt.Sprintf("Bulk import finished: %d created, %d skipped", totalCreated, totalSkipped),
			"completed_at":       time.Now(),
		},
	})
	h.appendParentLog(ctx, parentID.Hex(), "Bulk import process completed", "info")
}

func (h *IngestionHandler) fetchCatalogPage(source, categoryURL string, page int, typeFilter string) ([]CatalogItem, bool, error) {
	params := url.Values{}
	params.Set("source", source)
	params.Set("page", fmt.Sprintf("%d", page))
	params.Set("limit", "24")
	if typeFilter != "" {
		params.Set("type", typeFilter)
	}
	if categoryURL != "" {
		params.Set("category_url", categoryURL)
	}

	parserURL := fmt.Sprintf("%s/catalog?%s", h.parserURL, params.Encode())
	resp, err := h.httpClient.Get(parserURL)
	if err != nil {
		return nil, false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, false, fmt.Errorf("parser returned status %d: %s", resp.StatusCode, string(body))
	}

	var catalogResp CatalogResponse
	if err := json.NewDecoder(resp.Body).Decode(&catalogResp); err != nil {
		return nil, false, err
	}

	return catalogResp.Items, catalogResp.HasMore, nil
}

func (h *IngestionHandler) importSingleItemFromCatalog(ctx context.Context, source string, item CatalogItem) (bool, error) {
	// 1. Check if job already exists
	existing, err := h.jobRepo.GetBySourceAndID(ctx, source, item.SourceID)
	if err == nil && existing != nil {
		return false, nil // Already exists, skip
	}

	// 2. Resolve type if unknown
	if item.Type == "unknown" || item.Type == "" {
		// We could call /details here, but for bulk we might want to skip items that need resolution
		// or just default to movie if it's a movie catalog.
		// For now, let's just skip unknown types to be safe.
		return false, fmt.Errorf("unknown content type for %s", item.Title)
	}

	// 3. Serial vs Movie
	if item.Type == "serial" || item.Type == "series" {
		// For serials, we trigger the async serial extractor
		// We need to fetch full details first to get the detailURL
		// but CatalogItem already has it.
		
		// Create a serial parent job
		parent := &models.IngestionJob{
			Title:       item.Title,
			Source:      source,
			SourceID:    item.SourceID,
			DetailURL:   item.DetailURL,
			Status:      models.IngestionStatusQueued,
			Stage:       string(models.IngestionStatusQueued),
			ContentType: ContentTypeSerialParent,
			Steps:       models.JobSteps{},
			Logs:        []models.IngestionLog{},
			Message:     "Import queued (bulk)",
		}
		if err := h.jobRepo.Create(ctx, parent); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return false, nil
			}
			return false, err
		}
		
		go h.runSerialExtractionAsync(parent, source, item.SourceID, item.DetailURL, item.Title, h.parserURL)
		return true, nil
	}

	// 4. Create Movie Ingestion Job
	job := &models.IngestionJob{
		Title:       item.Title,
		Source:      source,
		SourceID:    item.SourceID,
		DetailURL:   item.DetailURL,
		Status:      models.IngestionStatusQueued,
		Stage:       string(models.IngestionStatusQueued),
		Progress:    0,
		Steps:       models.JobSteps{},
		Logs:        []models.IngestionLog{},
		ContentType: "movie",
		Metadata: &models.ParsedMovieMetadata{
			Title:        item.Title,
			Poster:       item.Poster,
			Description:  item.Description,
			Year:         item.Year,
			VideoPageURL: item.DetailURL,
		},
	}

	if err := h.jobRepo.Create(ctx, job); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}
