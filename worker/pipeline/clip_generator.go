package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
)

const clipCount = 10
const maxClipSeconds = 60

// generateClips creates 10 promotional clips from base_video.mp4 after a successful
// movie save. Only runs in development mode. Clips are saved alongside the HLS files
// in uploads/movies/{folder}/1clip.mp4 … 10clip.mp4.
//
// Each clip:
//   - Duration: up to 60s
//   - Drawn text top:    "Kino kodi: {code}"
//   - Drawn text bottom: "Kinoni profildagi bot orqali toping!"
func (p *Pipeline) generateClips(ctx context.Context, canonicalFolderName string, movieCode string) error {
	baseDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// base_video.mp4 lives in readyvideo/{folder}/ (not removed in dev mode)
	srcVideo := filepath.Join(baseDir, "readyvideo", canonicalFolderName, "base_video.mp4")
	if _, err := os.Stat(srcVideo); err != nil {
		return fmt.Errorf("base_video.mp4 not found at %s: %w", srcVideo, err)
	}

	// Output directory: same folder as the HLS files visible to the frontend
	outDir := filepath.Join(baseDir, "uploads", "movies", canonicalFolderName)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return fmt.Errorf("failed to create output dir: %w", err)
	}

	// Get video duration
	durationMs, err := p.getVideoDurationMs(srcVideo)
	if err != nil {
		return fmt.Errorf("failed to get duration: %w", err)
	}
	durationSec := float64(durationMs) / 1000.0
	log.Printf("[CLIP] Video duration: %.1fs, generating %d clips (max %ds each)", durationSec, clipCount, maxClipSeconds)

	// Distribute clips evenly across the video
	segmentSec := durationSec / float64(clipCount)
	clipDur := segmentSec
	if clipDur > maxClipSeconds {
		clipDur = maxClipSeconds
	}

	// Build drawtext filter — escape colons and special chars for ffmpeg
	topText := fmt.Sprintf("Kino kodi\\: %s", movieCode)
	bottomText := "Kinoni profildagi bot orqali toping\\!"
	textFilter := fmt.Sprintf(
		"drawtext=text='%s':x=(w-text_w)/2:y=20:fontsize=40:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10,"+
			"drawtext=text='%s':x=(w-text_w)/2:y=h-text_h-20:fontsize=36:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10",
		topText, bottomText,
	)

	for i := 0; i < clipCount; i++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		startSec := float64(i) * segmentSec
		outPath := filepath.Join(outDir, fmt.Sprintf("%dclip.mp4", i+1))

		log.Printf("[CLIP] Generating clip %d/%d: start=%.1fs dur=%.1fs → %s",
			i+1, clipCount, startSec, clipDur, outPath)

		args := []string{
			"-y",
			"-ss", fmt.Sprintf("%.3f", startSec),
			"-i", srcVideo,
			"-t", fmt.Sprintf("%.3f", clipDur),
			"-vf", textFilter,
			"-c:v", "libx264",
			"-preset", "veryfast",
			"-crf", "23",
			"-c:a", "aac",
			"-b:a", "128k",
			"-movflags", "+faststart",
			outPath,
		}

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		if out, err := cmd.CombinedOutput(); err != nil {
			log.Printf("[CLIP] WARNING: clip %d failed: %v\n%s", i+1, err, string(out))
			continue // non-fatal — generate remaining clips
		}

		log.Printf("[CLIP] Clip %d saved: %s", i+1, outPath)
	}

	log.Printf("[CLIP] Done generating clips for movie code=%s folder=%s", movieCode, canonicalFolderName)
	return nil
}
