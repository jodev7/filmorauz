package handlers

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/gin-gonic/gin"
)

// GifHandler proxies the watch-room chat GIF picker to GIPHY. The API key
// lives only in the backend config, so it's never shipped to the browser —
// the frontend calls our /api/gifs/search endpoint instead of api.giphy.com.
type GifHandler struct {
	apiKey     string
	httpClient *http.Client
}

func NewGifHandler(cfg *config.Config) *GifHandler {
	return &GifHandler{
		apiKey:     cfg.GiphyAPIKey,
		httpClient: &http.Client{Timeout: 8 * time.Second},
	}
}

// gifItem is the trimmed shape we hand back to the client — just the ids and
// the URLs the picker needs, not GIPHY's full (large) payload.
type gifItem struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Preview string `json:"preview"` // small still/animated thumb for the grid
	URL     string `json:"url"`     // full gif sent into chat
}

// SearchGifs GET /api/gifs/search?q=cat&limit=24
// Empty q → GIPHY trending. Always rating=pg-13 to keep a couples/family
// room SFW-ish.
func (h *GifHandler) SearchGifs(c *gin.Context) {
	if h.apiKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "gif service not configured"})
		return
	}

	q := c.Query("q")
	limit := 24
	if v, err := strconv.Atoi(c.Query("limit")); err == nil && v > 0 && v <= 50 {
		limit = v
	}

	params := url.Values{}
	params.Set("api_key", h.apiKey)
	params.Set("limit", strconv.Itoa(limit))
	params.Set("rating", "pg-13")
	params.Set("bundle", "fixed_height")

	endpoint := "https://api.giphy.com/v1/gifs/trending"
	if q != "" {
		endpoint = "https://api.giphy.com/v1/gifs/search"
		params.Set("q", q)
	}

	req, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, endpoint+"?"+params.Encode(), nil)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gif request failed"})
		return
	}
	resp, err := h.httpClient.Do(req)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gif upstream unreachable"})
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gif upstream error"})
		return
	}

	var giphy struct {
		Data []struct {
			ID     string `json:"id"`
			Title  string `json:"title"`
			Images struct {
				FixedHeight      struct{ URL string } `json:"fixed_height"`
				FixedHeightSmall struct{ URL string } `json:"fixed_height_small"`
			} `json:"images"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&giphy); err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "gif decode failed"})
		return
	}

	items := make([]gifItem, 0, len(giphy.Data))
	for _, g := range giphy.Data {
		if g.Images.FixedHeight.URL == "" {
			continue
		}
		preview := g.Images.FixedHeightSmall.URL
		if preview == "" {
			preview = g.Images.FixedHeight.URL
		}
		items = append(items, gifItem{
			ID:      g.ID,
			Title:   g.Title,
			Preview: preview,
			URL:     g.Images.FixedHeight.URL,
		})
	}

	c.JSON(http.StatusOK, gin.H{"gifs": items})
}
