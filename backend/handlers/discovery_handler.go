package handlers

import (
	"net/http"
	"net/url"
	"strconv"
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

func parseExcludeIDs(raw string) []primitive.ObjectID {
	var exclude []primitive.ObjectID
	for _, part := range strings.Split(raw, ",") {
		if id, err := primitive.ObjectIDFromHex(strings.TrimSpace(part)); err == nil {
			exclude = append(exclude, id)
		}
		if len(exclude) >= 50 {
			break
		}
	}
	return exclude
}

// RandomMovies GET /api/movies/random-list?limit=12&genre=&exclude=id1,id2
func (h *MovieHandler) RandomMovies(c *gin.Context) {
	limit := 12
	if n, err := strconv.Atoi(c.Query("limit")); err == nil && n > 0 {
		limit = n
	}
	if limit > 30 {
		limit = 30
	}
	movies, err := h.movieService.RandomMovies(c.Query("genre"), parseExcludeIDs(c.Query("exclude")), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to pick movies"})
		return
	}
	for i := range movies {
		protectMovieMedia(&movies[i])
		stripMoviePlayback(&movies[i])
	}
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, gin.H{"data": movies})
}

// RandomMovie GET /api/movies/random?genre=&exclude=id1,id2
func (h *MovieHandler) RandomMovie(c *gin.Context) {
	exclude := parseExcludeIDs(c.Query("exclude"))
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
