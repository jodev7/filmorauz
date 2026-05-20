package handlers

import (
	"net/http"
	"strings"

	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services/seo"
	"github.com/gin-gonic/gin"
)

// SEOHandler exposes the admin SEO dashboard endpoints:
//
//	GET  /api/admin/seo/status   — provider config + most-recent events
//	POST /api/admin/seo/reindex  — re-ping a list of URLs (body: {urls:[]})
//	POST /api/admin/seo/reindex/all — re-ping every published movie/series
//	POST /api/admin/seo/sitemap-resubmit — re-submit the sitemap index
//
// All endpoints require an admin JWT — wiring lives in routes/main.go.
type SEOHandler struct {
	notifier   *seo.Notifier
	movieRepo  *repositories.MovieRepository
	seriesRepo *repositories.SeriesRepository
}

func NewSEOHandler(n *seo.Notifier, movieRepo *repositories.MovieRepository, seriesRepo *repositories.SeriesRepository) *SEOHandler {
	return &SEOHandler{notifier: n, movieRepo: movieRepo, seriesRepo: seriesRepo}
}

func (h *SEOHandler) Status(c *gin.Context) {
	if h.notifier == nil {
		c.JSON(http.StatusOK, gin.H{"enabled": false})
		return
	}
	events, _ := h.notifier.RecentEvents(100)
	c.JSON(http.StatusOK, gin.H{
		"status":         h.notifier.Status(),
		"recent_events":  events,
	})
}

type reindexBody struct {
	URLs []string `json:"urls"`
}

func (h *SEOHandler) Reindex(c *gin.Context) {
	if h.notifier == nil || !h.notifier.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "seo notifier disabled"})
		return
	}
	var body reindexBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}
	cleaned := make([]string, 0, len(body.URLs))
	for _, u := range body.URLs {
		u = strings.TrimSpace(u)
		if u != "" {
			cleaned = append(cleaned, u)
		}
	}
	if len(cleaned) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no urls provided"})
		return
	}
	if err := h.notifier.Submit(cleaned); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"submitted": len(cleaned)})
}

// ReindexAll re-pings every published movie and series. Use sparingly —
// IndexNow has no documented daily quota but Google Indexing API caps at
// 200/day for most accounts.
func (h *SEOHandler) ReindexAll(c *gin.Context) {
	if h.notifier == nil || !h.notifier.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "seo notifier disabled"})
		return
	}
	paths := make([]string, 0, 512)
	if movies, err := h.movieRepo.ListPublishedForSitemap(); err == nil {
		for _, m := range movies {
			if m.Slug != "" {
				paths = append(paths, "/movies/"+m.Slug)
			}
		}
	}
	if seriesList, err := h.seriesRepo.ListPublishedForSitemap(); err == nil {
		for _, s := range seriesList {
			if s.Slug != "" {
				paths = append(paths, "/series/"+s.Slug)
			}
		}
	}
	if len(paths) == 0 {
		c.JSON(http.StatusOK, gin.H{"submitted": 0})
		return
	}
	h.notifier.NotifyURLs(paths)
	c.JSON(http.StatusOK, gin.H{"submitted": len(paths)})
}

func (h *SEOHandler) SitemapResubmit(c *gin.Context) {
	if h.notifier == nil || !h.notifier.Enabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "seo notifier disabled"})
		return
	}
	h.notifier.NotifySitemap()
	c.JSON(http.StatusOK, gin.H{"queued": true})
}
