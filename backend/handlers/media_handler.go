package handlers

import (
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/filmorauz/backend/config"
	"github.com/filmorauz/backend/models"
	"github.com/filmorauz/backend/repositories"
	"github.com/filmorauz/backend/services"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type MediaHandler struct {
	cfg           *config.Config
	movieService  *services.MovieService
	seriesService *services.SeriesService
	userRepo      *repositories.UserRepository
}

func NewMediaHandler(cfg *config.Config, movieService *services.MovieService, seriesService *services.SeriesService, userRepo *repositories.UserRepository) *MediaHandler {
	return &MediaHandler{
		cfg:           cfg,
		movieService:  movieService,
		seriesService: seriesService,
		userRepo:      userRepo,
	}
}

func (h *MediaHandler) GetMediaToken(c *gin.Context) {
	user := h.currentUser(c)

	var (
		sourceURL string
		err       error
	)

	if movieID := strings.TrimSpace(c.Query("movie_id")); movieID != "" {
		sourceURL, err = h.resolveMovieMediaURL(movieID, user)
		if err != nil {
			h.handleAccessError(c, err)
			return
		}
	} else if episodeID := strings.TrimSpace(c.Query("episode_id")); episodeID != "" {
		sourceURL, err = h.resolveEpisodeMediaURL(episodeID, user)
		if err != nil {
			h.handleAccessError(c, err)
			return
		}
	} else if mediaPath := strings.TrimSpace(c.Query("path")); mediaPath != "" {
		sourceURL, err = h.resolvePublicAssetPath(mediaPath)
		if err != nil {
			h.handleAccessError(c, err)
			return
		}
	} else {
		c.JSON(http.StatusBadRequest, gin.H{"error": "movie_id, episode_id or path is required"})
		return
	}

	playbackURL, cookieValue, expiresAt, err := h.buildProtectedPlayback(c, sourceURL)
	if err != nil {
		h.handleAccessError(c, err)
		return
	}
	h.setMediaCookie(c, cookieValue, expiresAt)

	c.JSON(http.StatusOK, gin.H{
		"success":      true,
		"protected":    true,
		"playback_url": playbackURL,
		"expires_at":   expiresAt.UTC().Format(time.RFC3339),
		"cookie_name":  h.cfg.MediaCookieName,
	})
}

func (h *MediaHandler) resolveMovieMediaURL(movieID string, user *models.User) (string, error) {
	movie, err := h.movieService.GetMovieByID(movieID)
	if err != nil {
		return "", errNotFound("movie not found")
	}
	if !movie.IsPublished {
		return "", errNotFound("movie not found")
	}
	if !models.CanAccessMovie(user, movie) {
		return "", errForbidden("premium_required", "Premium subscription required to watch this content")
	}
	if movie.MasterPlaylistURL != "" {
		return movie.MasterPlaylistURL, nil
	}
	if movie.VideoURL != "" {
		return movie.VideoURL, nil
	}
	return "", errBadRequest("media source not available")
}

func (h *MediaHandler) resolveEpisodeMediaURL(episodeID string, user *models.User) (string, error) {
	oid, err := primitive.ObjectIDFromHex(episodeID)
	if err != nil {
		return "", errBadRequest("invalid episode id")
	}
	episode, err := h.seriesService.GetEpisodeByID(oid)
	if err != nil {
		return "", errNotFound("episode not found")
	}
	series, err := h.seriesService.GetSeriesByID(episode.SeriesID)
	if err != nil || series == nil || !series.IsPublished {
		return "", errNotFound("episode not found")
	}
	if !models.CanAccessSeries(user, series) {
		return "", errForbidden("premium_required", "Premium subscription required to watch this content")
	}
	if episode.VideoURL == "" {
		return "", errBadRequest("media source not available")
	}
	return episode.VideoURL, nil
}

func (h *MediaHandler) resolvePublicAssetPath(mediaPath string) (string, error) {
	if !strings.HasPrefix(mediaPath, "/") {
		mediaPath = "/" + mediaPath
	}
	for _, prefix := range []string{"/posters/", "/backdrops/", "/movies/", "/series/", "/collections/"} {
		if strings.HasPrefix(mediaPath, prefix) {
			return mediaPath, nil
		}
	}
	return "", errForbidden("invalid_path", "unsupported media path")
}

func (h *MediaHandler) currentUser(c *gin.Context) *models.User {
	userIDStr := c.GetString("user_id")
	if userIDStr == "" {
		return nil
	}
	user, err := h.userRepo.FindByHex(userIDStr)
	if err != nil {
		return nil
	}
	return user
}

func (h *MediaHandler) buildProtectedPlayback(c *gin.Context, sourceURL string) (string, string, time.Time, error) {
	if h.cfg.MediaSigningSecret == "" {
		return "", "", time.Time{}, errBadRequest("media protection is not configured")
	}
	mediaPath, originQS, ok := extractMediaPath(sourceURL)
	if !ok {
		return "", "", time.Time{}, errBadRequest("failed to build protected playback url")
	}

	token, expiresAt := buildMediaToken(h.cfg.MediaSigningSecret, mediaPath, h.cfg.MediaTokenTTLSeconds, mediaTokenOptions{
		ClientIP: clientIPFromRequest(c),
		UAHash:   hashUserAgent(c.GetHeader("User-Agent")),
	})

	base := strings.TrimSuffix(h.cfg.MediaProtectedBaseURL, "/")
	if base == "" {
		base = "/media"
	}
	protected := base + mediaPath
	if originQS != "" {
		protected += "?origin_qs=" + url.QueryEscape(originQS)
	}

	return protected, token, expiresAt, nil
}

func (h *MediaHandler) setMediaCookie(c *gin.Context, value string, expiresAt time.Time) {
	if value == "" || h.cfg.MediaCookieName == "" {
		return
	}
	domain := mediaCookieDomain(c.Request.Host)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     h.cfg.MediaCookieName,
		Value:    value,
		Path:     "/media/",
		Domain:   domain,
		HttpOnly: true,
		Secure:   !h.cfg.IsDev,
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt.UTC(),
		MaxAge:   int(time.Until(expiresAt).Seconds()),
	})
}

func clientIPFromRequest(c *gin.Context) string {
	if c == nil || c.Request == nil {
		return ""
	}
	if ip := strings.TrimSpace(c.GetHeader("CF-Connecting-IP")); ip != "" {
		return ip
	}
	if forwarded := strings.TrimSpace(c.GetHeader("X-Forwarded-For")); forwarded != "" {
		if idx := strings.Index(forwarded, ","); idx >= 0 {
			return strings.TrimSpace(forwarded[:idx])
		}
		return forwarded
	}
	return strings.TrimSpace(c.ClientIP())
}

func mediaCookieDomain(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if parsed, err := url.Parse("http://" + host); err == nil {
		host = parsed.Hostname()
	}
	host = strings.TrimSpace(host)
	if host == "" || host == "localhost" || strings.Contains(host, ":") {
		return ""
	}
	if strings.HasSuffix(host, ".filmorauz.net") {
		return ".filmorauz.net"
	}
	if host == "filmorauz.net" {
		return ".filmorauz.net"
	}
	return ""
}

type mediaHTTPError struct {
	status  int
	code    string
	message string
}

func (e *mediaHTTPError) Error() string {
	return e.message
}

func errForbidden(code, message string) error {
	return &mediaHTTPError{status: http.StatusForbidden, code: code, message: message}
}

func errNotFound(message string) error {
	return &mediaHTTPError{status: http.StatusNotFound, code: "not_found", message: message}
}

func errBadRequest(message string) error {
	return &mediaHTTPError{status: http.StatusBadRequest, code: "bad_request", message: message}
}

func (h *MediaHandler) handleAccessError(c *gin.Context, err error) {
	if httpErr, ok := err.(*mediaHTTPError); ok {
		c.JSON(httpErr.status, gin.H{
			"error":   httpErr.code,
			"message": httpErr.message,
		})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed_to_generate_media_token"})
}
