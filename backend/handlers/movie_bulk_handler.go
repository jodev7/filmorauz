package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

const maxBulkMovieIDs = 200

type bulkMovieRequest struct {
	IDs             []string `json:"ids"`
	IsPremium       *bool    `json:"is_premium,omitempty"`
	AddToCollection string   `json:"add_to_collection,omitempty"`
}

// BulkUpdateMovies POST /api/admin/movies/bulk-update
//
// Applies simple field changes to many movies at once:
//   - is_premium: true/false
//   - add_to_collection: collection id — appends the movies ($addToSet, so
//     existing members aren't duplicated)
//
// Approve/reject/delete are intentionally NOT here: they have side effects
// (Telegram broadcast, cascade delete jobs) that live in their single-movie
// handlers, and the admin UI calls those per movie.
func (h *MovieHandler) BulkUpdateMovies(c *gin.Context) {
	var req bulkMovieRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	if len(req.IDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ids is required"})
		return
	}
	if len(req.IDs) > maxBulkMovieIDs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many ids"})
		return
	}
	if req.IsPremium == nil && req.AddToCollection == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "nothing to update"})
		return
	}

	ids := make([]primitive.ObjectID, 0, len(req.IDs))
	for _, raw := range req.IDs {
		id, err := primitive.ObjectIDFromHex(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid movie id: " + raw})
			return
		}
		ids = append(ids, id)
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()

	resp := gin.H{"success": true}

	if req.IsPremium != nil {
		res, err := h.db.Collection("movies").UpdateMany(ctx,
			bson.M{"_id": bson.M{"$in": ids}},
			bson.M{"$set": bson.M{"is_premium": *req.IsPremium, "updated_at": time.Now()}},
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update premium flag"})
			return
		}
		resp["premium_updated"] = res.ModifiedCount
	}

	if req.AddToCollection != "" {
		colID, err := primitive.ObjectIDFromHex(req.AddToCollection)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collection id"})
			return
		}
		res, err := h.db.Collection("collections").UpdateOne(ctx,
			bson.M{"_id": colID},
			bson.M{
				"$addToSet": bson.M{"movie_ids": bson.M{"$each": ids}},
				"$set":      bson.M{"updated_at": time.Now()},
			},
		)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update collection"})
			return
		}
		if res.MatchedCount == 0 {
			c.JSON(http.StatusNotFound, gin.H{"error": "collection not found"})
			return
		}
		resp["collection_updated"] = true
	}

	c.JSON(http.StatusOK, resp)
}
