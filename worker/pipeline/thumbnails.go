package pipeline

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ThumbnailIntervalSeconds is the default interval between hover-preview frames.
// 5s gives a YouTube-like scrub density without ballooning sprite size.
const ThumbnailIntervalSeconds = 5

// Hover-preview tiles are deliberately small: the player only needs them as
// a fingernail-sized scrub preview. 160x90 keeps 16:9 and cuts bandwidth ~75%
// vs. the previous 320x180 JPGs.
const (
	ThumbnailWidth  = 160
	ThumbnailHeight = 90
)

// SpriteCols is the number of columns in the generated sprite sheet. Rows are
// derived from the tile count. 10 columns is a common choice that keeps the
// sprite under typical max-texture limits even for long movies.
const SpriteCols = 10

// SpriteFileName is the sprite image stored alongside the individual tiles.
const SpriteFileName = "sprite.webp"

// VTTFileName is the WebVTT cues file mapping timecodes -> sprite regions.
const VTTFileName = "preview.vtt"

// generateThumbnails extracts one WebP frame every intervalSec seconds, writes
// thumb-1.webp, thumb-2.webp, … into outDir, then composes a sprite sheet and
// a WebVTT cues file pointing at sprite regions. Failures are non-fatal for
// the caller — they fall back to the static poster preview.
func generateThumbnails(inputPath, outDir string, intervalSec int) (int, error) {
	if intervalSec <= 0 {
		intervalSec = ThumbnailIntervalSeconds
	}
	if _, err := os.Stat(inputPath); err != nil {
		return 0, fmt.Errorf("thumbnails input missing: %w", err)
	}
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return 0, fmt.Errorf("thumbnails mkdir: %w", err)
	}

	pattern := filepath.Join(outDir, "frame_%04d.webp")
	tileFilter := fmt.Sprintf(
		"fps=1/%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
		intervalSec, ThumbnailWidth, ThumbnailHeight, ThumbnailWidth, ThumbnailHeight,
	)
	tileArgs := []string{
		"-y",
		"-i", inputPath,
		"-vf", tileFilter,
		"-c:v", "libwebp",
		"-quality", "70",
		"-start_number", "1",
		pattern,
	}
	if out, err := exec.Command("ffmpeg", tileArgs...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("ffmpeg thumbnails failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return 0, fmt.Errorf("read thumbnails dir: %w", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "frame_") && strings.HasSuffix(e.Name(), ".webp") {
			count++
		}
	}
	if count == 0 {
		return 0, fmt.Errorf("ffmpeg produced no thumbnails")
	}

	rows := (count + SpriteCols - 1) / SpriteCols
	spritePath := filepath.Join(outDir, SpriteFileName)
	spriteArgs := []string{
		"-y",
		"-i", inputPath,
		"-vf", fmt.Sprintf("%s,tile=%dx%d", tileFilter, SpriteCols, rows),
		"-c:v", "libwebp",
		"-quality", "70",
		"-frames:v", "1",
		spritePath,
	}
	if out, err := exec.Command("ffmpeg", spriteArgs...).CombinedOutput(); err != nil {
		log.Printf("[THUMB] WARNING: sprite generation failed: %v (output: %s)", err, strings.TrimSpace(string(out)))
	} else {
		if err := writeThumbnailVTT(filepath.Join(outDir, VTTFileName), count, intervalSec, SpriteCols); err != nil {
			log.Printf("[THUMB] WARNING: vtt write failed: %v", err)
		}
	}

	log.Printf("[THUMB] generated %d webp tiles (+sprite/vtt) in %s (every %ds)", count, outDir, intervalSec)
	return count, nil
}

// writeThumbnailVTT emits a WebVTT cues file that maps each interval to a
// region of SpriteFileName. Players (videojs-vtt-thumbnails, custom hover UIs)
// can resolve sprite-relative coords via #xywh=x,y,w,h URI fragments.
func writeThumbnailVTT(path string, count, intervalSec, cols int) error {
	var b strings.Builder
	b.WriteString("WEBVTT\n\n")
	for i := 0; i < count; i++ {
		start := i * intervalSec
		end := start + intervalSec
		col := i % cols
		row := i / cols
		x := col * ThumbnailWidth
		y := row * ThumbnailHeight
		fmt.Fprintf(&b, "%s --> %s\n%s#xywh=%d,%d,%d,%d\n\n",
			vttTimestamp(start), vttTimestamp(end),
			SpriteFileName, x, y, ThumbnailWidth, ThumbnailHeight,
		)
	}
	return os.WriteFile(path, []byte(b.String()), 0644)
}

func vttTimestamp(sec int) string {
	h := sec / 3600
	m := (sec % 3600) / 60
	s := sec % 60
	return fmt.Sprintf("%02d:%02d:%02d.000", h, m, s)
}

// thumbnailsBaseURLFromMaster turns a master playlist URL (…/index.m3u8 or
// the legacy …/master.m3u8) into the directory URL where the thumbnail tiles
// were uploaded (…/thumbnails/). Returns empty string if streamingURL is
// empty or does not end in a recognised master playlist name.
func thumbnailsBaseURLFromMaster(streamingURL string) string {
	if streamingURL == "" {
		return ""
	}
	for _, suffix := range []string{"/" + MasterPlaylistName, "/master.m3u8"} {
		if strings.HasSuffix(streamingURL, suffix) {
			return strings.TrimSuffix(streamingURL, suffix) + "/thumbnails/"
		}
	}
	return ""
}
