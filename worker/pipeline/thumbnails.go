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
// One frame is extracted every N seconds, so a 90 min video yields ~540 frames at 10s.
const ThumbnailIntervalSeconds = 10

// ThumbnailWidth / ThumbnailHeight match the player hover preview size (320x180 keeps
// the 16:9 aspect ratio and stays small on the wire).
const (
	ThumbnailWidth  = 320
	ThumbnailHeight = 180
)

// generateThumbnails extracts one JPG frame from inputPath every intervalSec seconds
// using ffmpeg, writing thumb-1.jpg, thumb-2.jpg, … into outDir. Returns the number
// of files written. Failures are non-fatal for the caller — they should fall back
// to the existing poster preview when no thumbnails are produced.
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

	pattern := filepath.Join(outDir, "thumb-%d.jpg")
	args := []string{
		"-y",
		"-i", inputPath,
		"-vf", fmt.Sprintf("fps=1/%d,scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			intervalSec, ThumbnailWidth, ThumbnailHeight, ThumbnailWidth, ThumbnailHeight),
		"-q:v", "5",
		"-start_number", "1",
		pattern,
	}

	cmd := exec.Command("ffmpeg", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return 0, fmt.Errorf("ffmpeg thumbnails failed: %w (output: %s)", err, strings.TrimSpace(string(out)))
	}

	entries, err := os.ReadDir(outDir)
	if err != nil {
		return 0, fmt.Errorf("read thumbnails dir: %w", err)
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), "thumb-") && strings.HasSuffix(e.Name(), ".jpg") {
			count++
		}
	}
	log.Printf("[THUMB] generated %d thumbnails in %s (every %ds)", count, outDir, intervalSec)
	return count, nil
}

// thumbnailsBaseURLFromMaster turns a master playlist URL (…/index.m3u8 or
// the legacy …/master.m3u8) into the directory URL where the thumbnail JPGs
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
