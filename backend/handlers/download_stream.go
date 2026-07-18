package handlers

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/filmorauz/backend/middleware"
	"github.com/filmorauz/backend/models"
	"github.com/gin-gonic/gin"
)

// canDownload reports whether the authenticated request is allowed to download
// source video. Admins and superadmins may always download; premium users may
// too (the frontend simply doesn't expose a button for them yet).
//
// It relies on the fresh *models.User that RequireAuth stores under
// "banned_user" so the premium/role check reflects the current DB state rather
// than a possibly-stale JWT claim.
func canDownload(c *gin.Context) bool {
	raw, exists := c.Get("banned_user")
	if !exists {
		return false
	}
	u, ok := raw.(*models.User)
	if !ok || u == nil {
		return false
	}
	if middleware.IsAdmin(u.Role) {
		return true
	}
	return u.IsPremiumActive()
}

// pickBestDownloadQuality returns the highest quality suitable for download.
// Preference order: 1080p, then 720p. If neither is present it falls back to
// the highest numeric rendition available, and finally "" when the list is
// empty (caller should treat that as "use the master playlist as-is").
func pickBestDownloadQuality(qualities []string) string {
	has := func(q string) bool {
		for _, v := range qualities {
			if strings.EqualFold(strings.TrimSpace(v), q) {
				return true
			}
		}
		return false
	}
	if has("1080p") {
		return "1080p"
	}
	if has("720p") {
		return "720p"
	}
	// Fall back to the numerically-highest rendition present.
	best := ""
	bestN := -1
	re := regexp.MustCompile(`(\d+)`)
	for _, q := range qualities {
		m := re.FindString(q)
		if m == "" {
			continue
		}
		n := 0
		fmt.Sscanf(m, "%d", &n)
		if n > bestN {
			bestN = n
			best = strings.TrimSpace(q)
		}
	}
	return best
}

// renditionPlaylistURL rewrites a master playlist URL (…/<folder>/index.m3u8 or
// …/<folder>/master.m3u8) into the per-quality rendition playlist
// (…/<folder>/<quality>/index.m3u8). If quality is empty or the URL doesn't end
// in a recognizable master filename, the original URL is returned unchanged.
func renditionPlaylistURL(masterURL, quality string) string {
	if quality == "" || masterURL == "" {
		return masterURL
	}
	idx := strings.LastIndex(masterURL, "/")
	if idx < 0 {
		return masterURL
	}
	base := masterURL[:idx]          // …/<folder>
	last := masterURL[idx+1:]        // index.m3u8 / master.m3u8 (+ optional ?query)
	name := last
	if q := strings.IndexByte(name, '?'); q >= 0 {
		name = name[:q]
	}
	if name != "index.m3u8" && name != "master.m3u8" {
		// Unexpected shape — don't guess, let ffmpeg read the master directly.
		return masterURL
	}
	return fmt.Sprintf("%s/%s/index.m3u8", base, quality)
}

var downloadNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

// sanitizeDownloadName turns an arbitrary title into a safe ASCII filename base
// (without extension). Empty/invalid input yields "video".
func sanitizeDownloadName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, " ", "_")
	name = downloadNameSanitizer.ReplaceAllString(name, "")
	name = strings.Trim(name, "._-")
	if name == "" {
		return "video"
	}
	if len(name) > 100 {
		name = name[:100]
	}
	return name
}

// streamHLSAsMP4 remuxes the highest-quality HLS rendition into a progressive
// MP4 and streams it to the client as an attachment. It uses ffmpeg with
// `-c copy` (no re-encoding — cheap, IO-bound) and a fragmented-mp4 muxer so
// bytes flow to the browser as segments are pulled, keeping memory flat even
// for multi-GB videos.
//
// sourceType must be direct_hls with a non-empty masterPlaylistURL; otherwise a
// 422 is returned (embed/iframe sources have no downloadable file).
func streamHLSAsMP4(c *gin.Context, sourceType models.VideoSourceType, masterPlaylistURL string, qualities []string, downloadBase string) {
	if sourceType != models.VideoSourceDirectHLS || strings.TrimSpace(masterPlaylistURL) == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "not_downloadable",
			"message": "Bu kontent yuklab olish uchun mavjud emas (HLS manba yo'q)",
		})
		return
	}

	quality := pickBestDownloadQuality(qualities)
	inputURL := renditionPlaylistURL(masterPlaylistURL, quality)

	filename := sanitizeDownloadName(downloadBase)
	if quality != "" {
		filename += "_" + quality
	}
	filename += ".mp4"

	log.Printf("[Download] streaming %s quality=%q input=%s", filename, quality, inputURL)

	// Bind ffmpeg's lifetime to the request: if the client disconnects mid-
	// download the context is cancelled and ffmpeg is killed.
	ctx := c.Request.Context()
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-i", inputURL,
		"-c", "copy",
		"-bsf:a", "aac_adtstoasc",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof",
		"-f", "mp4",
		"pipe:1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start download"})
		return
	}
	var stderr strings.Builder
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start download"})
		return
	}

	c.Header("Content-Type", "video/mp4")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Cache-Control", "no-store")
	c.Header("X-Accel-Buffering", "no") // disable nginx proxy buffering so bytes flow live
	c.Status(http.StatusOK)

	// Stream ffmpeg stdout straight to the response. Once headers are written we
	// can no longer send a JSON error, so failures past this point are only
	// logged. Gin's ResponseWriter flushes on write; disabling proxy buffering
	// (above) keeps bytes moving to the client in real time.
	if _, err := io.Copy(c.Writer, stdout); err != nil {
		log.Printf("[Download] stream copy failed for %s: %v", filename, err)
	}

	if err := cmd.Wait(); err != nil {
		// Client cancellation (context done) is expected and not an error worth
		// alarming about.
		if ctx.Err() != nil {
			log.Printf("[Download] client cancelled %s", filename)
		} else {
			log.Printf("[Download] ffmpeg failed for %s: %v: %s", filename, err, stderr.String())
		}
	}
}
