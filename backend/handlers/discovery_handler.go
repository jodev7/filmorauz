package handlers

import (
	"net/http"
	"net/url"
	"strings"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// PersonCredits GET /api/people/:name — actor/director page data.
func (h *MovieHandler) PersonCredits(c *gin.Context) {
	name, err := url.PathUnescape(c.Param("name"))
	if err != nil {
		name = c.Param("name")
	}
	name = strings.TrimSpace(name)
	if name == "" || len(name) > 80 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid name"})
		return
	}
	credits, err := h.movieService.PersonCredits(name)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load"})
		return
	}
	if len(credits.Acted) == 0 && len(credits.Directed) == 0 && len(credits.Series) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	for i := range credits.Acted {
		protectMovieMedia(&credits.Acted[i])
		stripMoviePlayback(&credits.Acted[i])
	}
	for i := range credits.Directed {
		protectMovieMedia(&credits.Directed[i])
		stripMoviePlayback(&credits.Directed[i])
	}
	for i := range credits.Series {
		protectSeriesMedia(&credits.Series[i])
	}
	c.JSON(http.StatusOK, credits)
}

// RandomMovie GET /api/movies/random?genre=&exclude=id1,id2
func (h *MovieHandler) RandomMovie(c *gin.Context) {
	var exclude []primitive.ObjectID
	for _, raw := range strings.Split(c.Query("exclude"), ",") {
		if id, err := primitive.ObjectIDFromHex(strings.TrimSpace(raw)); err == nil {
			exclude = append(exclude, id)
		}
		if len(exclude) >= 50 {
			break
		}
	}
	movie, err := h.movieService.RandomMovie(c.Query("genre"), exclude)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pick a movie"})
		return
	}
	if movie == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no movie found"})
		return
	}
	protectMovieMedia(movie)
	stripMoviePlayback(movie)
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": movie})
}
