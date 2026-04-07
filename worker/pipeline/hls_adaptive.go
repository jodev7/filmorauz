package pipeline

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/filmorauz/worker/models"
)

// RenditionConfig defines the configuration for a single HLS rendition
type RenditionConfig struct {
	Name         string // Display name (e.g., "360p", "480p")
	Width        int    // Video width
	Height       int    // Video height
	VideoBitrate string // Video bitrate (e.g., "800k")
	AudioBitrate string // Audio bitrate (e.g., "96k")
	Bandwidth    int    // Total bandwidth in bits/sec for master playlist
}

// DefaultRenditions returns the default adaptive HLS renditions
func DefaultRenditions() []RenditionConfig {
	return []RenditionConfig{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", AudioBitrate: "96k", Bandwidth: 896000},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: "1400k", AudioBitrate: "128k", Bandwidth: 1528000},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2800k", AudioBitrate: "128k", Bandwidth: 2928000},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "128k", Bandwidth: 5128000},
	}
}

// processAdaptiveHLS generates multi-bitrate adaptive HLS with a master playlist
// This function:
// 1. First creates a clean base video with logo overlay applied
// 2. Then generates multiple quality renditions from the base
// 3. Creates a master.m3u8 playlist referencing all variants
// No watermark removal - use source video directly
func (p *Pipeline) processAdaptiveHLS(jobID, inputPath, outputDir string, cutSeconds int, jobStatusCallback func(status models.IngestionStatus, progress int)) (masterPlaylistPath, processedMasterPath string, err error) {
	log.Printf("[HLS] Starting adaptive HLS generation for job %s", jobID)
	log.Printf("[CHECKPOINT] hls_raw_input_path: %s", inputPath)
	log.Printf("[HLS] Output directory: %s", outputDir)

	// Create output directory structure
	// Structure: outputDir/
	//   - master.m3u8
	//   - 360p/
	//     - index.m3u8
	//     - segment_001.ts, ...
	//   - 480p/
	//   - 720p/
	//   - 1080p/

	if err = os.MkdirAll(outputDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get input resolution to determine which renditions to generate
	inputWidth, inputHeight, resErr := p.getInputResolution(inputPath)
	if resErr != nil {
		log.Printf("[HLS] WARNING: Could not detect input resolution: %v, using defaults", resErr)
		inputWidth = 1920
		inputHeight = 1080
	}
	log.Printf("[HLS] Input resolution: %dx%d", inputWidth, inputHeight)

	// Determine which renditions to generate based on input resolution
	renditions := p.getApplicableRenditions(inputWidth, inputHeight)
	log.Printf("[HLS] Generating %d renditions: %v", len(renditions), getRenditionNames(renditions))

	// Step 1: Cut + logo overlay → processed_master.mp4
	// This is the single processed master used for both HLS and clip generation.
	// It is NOT deleted here — the caller cleans it up after clip generation.
	processedMasterPath = filepath.Join(outputDir, "processed_master.mp4")
	log.Printf("[CHECKPOINT] processed_master_path: %s", processedMasterPath)
	log.Printf("[STAGE] logo_overlay start — raw_input: %s → master: %s", inputPath, processedMasterPath)
	if err = p.createBaseVideo(inputPath, processedMasterPath, cutSeconds, jobStatusCallback); err != nil {
		return "", "", fmt.Errorf("failed to create processed master: %w", err)
	}
	log.Printf("[STAGE] logo_overlay end — processed_master: %s", processedMasterPath)
	log.Printf("[CHECKPOINT] hls_input_path: %s", processedMasterPath)

	// Report progress: master created, starting HLS encoding (~55%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 55)
	}

	// Step 2: Generate each HLS rendition from the processed master
	log.Printf("[STAGE] hls_renditions start — renditions: %d, source: %s", len(renditions), processedMasterPath)
	totalRenditions := len(renditions)
	for i, rendition := range renditions {
		renditionBaseProgress := 55 + (i * 30 / totalRenditions)
		log.Printf("[HLS] Generating %s rendition (%d/%d) at progress %d%%...", rendition.Name, i+1, totalRenditions, renditionBaseProgress)

		renditionDir := filepath.Join(outputDir, rendition.Name)
		if err = os.MkdirAll(renditionDir, 0755); err != nil {
			return "", "", fmt.Errorf("failed to create rendition directory: %w", err)
		}

		if err = p.generateHLSRendition(jobID, processedMasterPath, renditionDir, rendition, renditionBaseProgress, jobStatusCallback); err != nil {
			return "", "", fmt.Errorf("failed to generate %s rendition: %w", rendition.Name, err)
		}
	}
	log.Printf("[STAGE] hls_renditions end — renditions: %d", len(renditions))

	// Report progress: HLS encoding complete, creating master playlist (~88%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 88)
	}

	// Step 3: Create master playlist
	log.Printf("[STAGE] master_playlist start")
	masterPlaylistPath = filepath.Join(outputDir, "master.m3u8")
	if err = p.createMasterPlaylist(masterPlaylistPath, renditions); err != nil {
		return "", "", fmt.Errorf("failed to create master playlist: %w", err)
	}
	log.Printf("[STAGE] master_playlist end")

	// Report progress: HLS generation complete (~92%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 92)
	}

	// Verify output
	files, dirErr := os.ReadDir(outputDir)
	if dirErr != nil {
		return "", "", fmt.Errorf("failed to read output directory: %w", dirErr)
	}
	log.Printf("[HLS] Adaptive HLS generation complete. Output files:")
	for _, f := range files {
		log.Printf("[HLS]   - %s", f.Name())
	}

	log.Printf("[HLS] Master playlist: %s", masterPlaylistPath)
	log.Printf("[CHECKPOINT] hls_master_playlist: %s", masterPlaylistPath)
	log.Printf("[STAGE] hls_processing end — master: %s, processed_master: %s", masterPlaylistPath, processedMasterPath)
	return masterPlaylistPath, processedMasterPath, nil
}

// getApplicableRenditions returns the renditions that should be generated based on input resolution
func (p *Pipeline) getApplicableRenditions(inputWidth, inputHeight int) []RenditionConfig {
	allRenditions := DefaultRenditions()
	applicable := make([]RenditionConfig, 0)

	for _, r := range allRenditions {
		// Generate rendition if input is at least 80% of rendition resolution
		// This ensures reasonable quality when upscaling
		if inputWidth >= int(float64(r.Width)*0.8) && inputHeight >= int(float64(r.Height)*0.8) {
			applicable = append(applicable, r)
		}
	}

	// Always include at least 360p if available
	if len(applicable) == 0 {
		applicable = append(applicable, allRenditions[0]) // 360p
	}

	return applicable
}

// getRenditionNames returns a slice of rendition names for logging
func getRenditionNames(renditions []RenditionConfig) []string {
	names := make([]string, len(renditions))
	for i, r := range renditions {
		names[i] = r.Name
	}
	return names
}

// createBaseVideo creates an intermediate base video with logo overlay applied
func (p *Pipeline) createBaseVideo(inputPath, outputPath string, cutSeconds int, jobStatusCallback func(status models.IngestionStatus, progress int)) error {
	log.Printf("[HLS] Creating base video: cut=%ds, logo overlay, input=%s", cutSeconds, inputPath)
	log.Printf("[CHECKPOINT] raw_downloaded_input_path: %s", inputPath)
	log.Printf("[HLS] NOTE: No watermark removal - using source video directly")

	// Video chain: ensure even dimensions for H.264 (required by libx264).
	videoChain := "scale=trunc(iw/2)*2:trunc(ih/2)*2"

	// Resolve logo path relative to working directory.
	cwd, _ := os.Getwd()
	logoPath := filepath.Join(cwd, "docs", "logo.png")
	logoExists := false
	if _, statErr := os.Stat(logoPath); statErr == nil {
		logoExists = true
		log.Printf("[HLS] Watermark logo found: %s", logoPath)
	} else {
		log.Printf("[HLS] WARNING: watermark logo not found at %s, skipping overlay", logoPath)
	}

	// Build ffmpeg args: use filter_complex (two inputs) when logo exists,
	// plain -vf otherwise. Audio is always copied from input 0.
	baseArgs := []string{
		"-y",
		"-ss", strconv.Itoa(cutSeconds),
		"-i", inputPath,
	}

	var filterArgs []string
	var filterDescription string
	if logoExists {
		// Second input: the logo PNG.
		// -loop 1 is required — without it, a PNG is a 1-frame stream; the overlay
		// would only appear on the first frame and then disappear for the rest of the video.
		// :shortest=1 terminates the overlay when the video stream ends.
		// scale=120:-1 keeps the logo compact — a subtle watermark, not a banner.
		filterComplex := fmt.Sprintf(
			"[0:v]%s[base];[1:v]scale=120:-1[logo];[base][logo]overlay=W-w-20:H-h-20:shortest=1[out]",
			videoChain,
		)
		filterArgs = []string{
			"-loop", "1", // keep logo PNG looping for full video duration
			"-i", logoPath,
			"-filter_complex", filterComplex,
			"-map", "[out]",
			"-map", "0:a?",
		}
		filterDescription = "logo watermark 120px bottom-right (loop=1, shortest=1) + scale filter"
		log.Printf("[HLS] APPLYING logo overlay — logo: %s", logoPath)
		log.Printf("[HLS] APPLYING logo overlay — filter_complex: %s", filterComplex)
	} else {
		filterArgs = []string{"-vf", videoChain}
		filterDescription = "scale filter only (no logo)"
		log.Printf("[HLS] NO logo overlay - using filter: %s", filterDescription)
	}

	encodeArgs := []string{
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "18",
		"-profile:v", "high",
		"-level", "4.1",
		"-c:a", "aac",
		"-b:a", "192k",
		"-movflags", "+faststart",
		"-progress", "pipe:1", // MUST appear before output path
		outputPath,
	}

	ffmpegArgs := append(append(baseArgs, filterArgs...), encodeArgs...)

	// Log the full ffmpeg command so it can be reproduced manually on failure
	log.Printf("[HLS] ===== BASE VIDEO FFMPEG COMMAND =====")
	log.Printf("[HLS] Filter description: %s", filterDescription)
	log.Printf("[CHECKPOINT] logo_applied_video_path: %s", outputPath)
	log.Printf("[HLS] ffmpeg %s", strings.Join(ffmpegArgs, " "))

	cmd := exec.Command("ffmpeg", ffmpegArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe for base video: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg base video: %w", err)
	}

	// Get total output duration (input minus cut) for progress mapping
	inputDurationMs, _ := p.getVideoDurationMs(inputPath)
	cutMs := int64(cutSeconds) * 1000
	totalMs := inputDurationMs - cutMs
	if totalMs <= 0 {
		totalMs = inputDurationMs
	}
	var lastReportedProgress int = 45

	// Report progress: starting base video encoding (~45%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 45)
	}

	// Read ffmpeg progress and map to 45-52% overall range
	buffer := make([]byte, 4096)
	for {
		n, readErr := stdout.Read(buffer)
		if readErr != nil {
			break
		}
		if n > 0 {
			lines := strings.Split(string(buffer[:n]), "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "out_time_ms=") {
					value := strings.TrimSpace(strings.TrimPrefix(line, "out_time_ms="))
					if t, parseErr := strconv.ParseInt(value, 10, 64); parseErr == nil && totalMs > 0 {
						outTimeMs := t / 1000 // Convert microseconds to milliseconds
						pct := int((outTimeMs * 100) / totalMs)
						// Map 0-100% ffmpeg progress to 45-52% overall
						overallProgress := 45 + int(float64(pct)*0.07)
						if overallProgress > 52 {
							overallProgress = 52
						}
						if overallProgress > lastReportedProgress && jobStatusCallback != nil {
							lastReportedProgress = overallProgress
							jobStatusCallback(models.IngestionStatusProcessing, overallProgress)
						}
					}
				}
			}
		}
	}

	if err := cmd.Wait(); err != nil {
		stderr := stderrBuf.String()
		log.Printf("[HLS] ===== FFMPEG BASE VIDEO FAILED =====")
		log.Printf("[HLS] Error: %v", err)
		log.Printf("[HLS] Stderr:\n%s", stderr)
		log.Printf("[HLS] Command was: ffmpeg %s", strings.Join(ffmpegArgs, " "))
		return fmt.Errorf("ffmpeg base video failed: %w — stderr: %s", err, stderr)
	}

	// Report progress: base video encoding complete (~52%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 52)
	}

	log.Printf("[HLS] Base video created: %s", outputPath)
	return nil
}

// generateHLSRendition generates a single HLS rendition from the base video
func (p *Pipeline) generateHLSRendition(jobID, baseVideoPath, outputDir string, rendition RenditionConfig, baseProgress int, jobStatusCallback func(status models.IngestionStatus, progress int)) error {
	log.Printf("[CHECKPOINT] hls_input_path: %s", baseVideoPath)
	log.Printf("[HLS] Generating %s rendition: %dx%d, video=%s, audio=%s",
		rendition.Name, rendition.Width, rendition.Height, rendition.VideoBitrate, rendition.AudioBitrate)

	// Report progress: starting rendition encoding
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, baseProgress)
	}

	// HLS playlist path
	hlsPlaylist := filepath.Join(outputDir, "index.m3u8")
	segmentPattern := filepath.Join(outputDir, "segment_%03d.ts")

	// Key settings for adaptive streaming:
	// - GOP of 2 seconds (keyframe interval)
	// - -sc_threshold 0: disable scene cut detection (forces keyframes at GOP boundary)
	// - -g: GOP size in frames (assuming 24fps, 2 seconds = 48 frames)
	// - -keyint_min: minimum keyframe interval
	// - -hls_time 6: 6-second segments
	// - -hls_list_size 0: include all segments
	// - -hls_playlist_type vod: VOD playlist

	ffmpegArgs := []string{
		"-y",                // Overwrite output
		"-i", baseVideoPath, // Input: base video with filters applied
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			rendition.Width, rendition.Height, rendition.Width, rendition.Height), // Scale and pad to exact resolution
		"-c:v", "libx264", // H.264 video codec
		"-preset", "veryfast", // Fast encoding preset
		"-b:v", rendition.VideoBitrate, // Video bitrate
		"-maxrate", fmt.Sprintf("%.0fk", parseBitrate(rendition.VideoBitrate)*1.5), // Max rate 1.5x bitrate
		"-bufsize", fmt.Sprintf("%.0fk", parseBitrate(rendition.VideoBitrate)*2), // Buffer size 2x bitrate
		"-profile:v", "main", // Main profile for good compatibility
		"-level", "3.1", // Level 3.1 for wide device compatibility
		"-sc_threshold", "0", // Disable scene cut detection - forces keyframes at GOP boundary
		"-g", "48", // GOP size: 2 seconds * 24fps = 48 frames
		"-keyint_min", "48", // Minimum keyframe interval
		"-c:a", "aac", // AAC audio codec
		"-b:a", rendition.AudioBitrate, // Audio bitrate
		"-f", "hls", // HLS output format
		"-hls_time", "6", // 6-second segment duration
		"-hls_list_size", "0", // Include all segments in playlist
		"-hls_playlist_type", "vod", // VOD playlist type
		"-hls_segment_filename", segmentPattern, // Segment filename pattern
		hlsPlaylist,           // Output playlist
		"-progress", "pipe:1", // Output progress to stdout
	}

	log.Printf("[HLS] %s FFmpeg: ffmpeg %s", rendition.Name, strings.Join(ffmpegArgs, " "))

	// Run ffmpeg with progress tracking
	cmd := exec.Command("ffmpeg", ffmpegArgs...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	// Read progress output and calculate total duration from base video
	totalMs, _ := p.getVideoDurationMs(baseVideoPath)
	var lastReportedProgress int = 0

	// Read progress output
	buffer := make([]byte, 4096)
	for {
		n, err := stdout.Read(buffer)
		if err != nil {
			break
		}
		if n > 0 {
			output := string(buffer[:n])
			lines := strings.Split(output, "\n")
			for _, line := range lines {
				if strings.HasPrefix(line, "out_time_ms=") {
					value := strings.TrimPrefix(line, "out_time_ms=")
					if t, err := strconv.ParseInt(value, 10, 64); err == nil && totalMs > 0 {
						outTimeMs := t / 1000 // Convert to ms
						progress := int((outTimeMs * 100) / totalMs)
						// Map rendition progress (0-100) to overall progress band for this rendition
						overallProgress := baseProgress + int(float64(progress)*0.3)
						if overallProgress > 92 {
							overallProgress = 92
						}
						// Only report if progress changed by at least 3%
						if overallProgress-lastReportedProgress >= 3 || progress >= 100 {
							lastReportedProgress = overallProgress
							if jobStatusCallback != nil {
								jobStatusCallback(models.IngestionStatusProcessing, overallProgress)
							}
						}
					}
				}
			}
		}
	}

	// Wait for command to finish
	err = cmd.Wait()
	if err != nil {
		log.Printf("[HLS] ===== FFMPEG %s RENDITION FAILED =====", rendition.Name)
		log.Printf("[HLS] Error: %v", err)
		log.Printf("[HLS] Stderr:\n%s", stderrBuf.String())
		log.Printf("[HLS] Command was: ffmpeg %s", strings.Join(ffmpegArgs, " "))
		return fmt.Errorf("ffmpeg %s failed: %w, stderr: %s", rendition.Name, err, stderrBuf.String())
	}

	// Report final progress for this rendition (capped at 92 to leave room for master playlist)
	if jobStatusCallback != nil {
		finalProgress := baseProgress + 30
		if finalProgress > 92 {
			finalProgress = 92
		}
		jobStatusCallback(models.IngestionStatusProcessing, finalProgress)
	}

	// Verify output
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no output files generated for %s", rendition.Name)
	}

	log.Printf("[HLS] %s rendition complete: %d files in %s", rendition.Name, len(files), outputDir)
	return nil
}

// parseBitrate parses a bitrate string like "800k" to float64
func parseBitrate(bitrate string) float64 {
	bitrate = strings.TrimSuffix(bitrate, "k")
	bitrate = strings.TrimSuffix(bitrate, "K")
	val, _ := strconv.ParseFloat(bitrate, 64)
	return val
}

// createMasterPlaylist creates the master.m3u8 playlist referencing all renditions
func (p *Pipeline) createMasterPlaylist(masterPath string, renditions []RenditionConfig) error {
	log.Printf("[HLS] Creating master playlist: %s", masterPath)

	var builder strings.Builder

	// Write M3U header
	builder.WriteString("#EXTM3U\n")
	builder.WriteString("#EXT-X-VERSION:3\n")

	// Write stream info for each rendition
	for _, r := range renditions {
		// EXT-X-STREAM-INF attributes:
		// - BANDWIDTH: total bandwidth in bits/sec
		// - RESOLUTION: video resolution
		// - NAME: display name for quality selector
		builder.WriteString(fmt.Sprintf("#EXT-X-STREAM-INF:BANDWIDTH=%d,RESOLUTION=%dx%d,NAME=\"%s\"\n",
			r.Bandwidth, r.Width, r.Height, r.Name))
		builder.WriteString(fmt.Sprintf("%s/index.m3u8\n", r.Name))
	}

	// Write the master playlist file
	content := builder.String()
	if err := os.WriteFile(masterPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write master playlist: %w", err)
	}

	log.Printf("[HLS] Master playlist written:\n%s", content)
	return nil
}

// uploadAdaptiveHLSFiles uploads all adaptive HLS files to storage
// Returns the master playlist URL
func (p *Pipeline) uploadAdaptiveHLSFiles(job *models.IngestionJob, hlsDir string, folderName string) (string, error) {
	jobID := job.ID.Hex()
	log.Printf("[HLS] Uploading adaptive HLS files for job %s from %s", jobID, hlsDir)

	var streamingURL string

	// Check MODE from storage config
	mode := p.config.StorageConfig.Mode
	log.Printf("[HLS] MODE=%s", mode)

	if mode == "prod" {
		// PRODUCTION: Upload to B2/CDN
		log.Printf("[HLS] Uploading to B2/CDN...")

		// Walk the directory and upload all files
		err := filepath.Walk(hlsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			// Calculate relative path
			relPath, _ := filepath.Rel(hlsDir, path)
			remotePath := filepath.Join("videos", folderName, relPath)

			log.Printf("[HLS] Uploading: %s -> %s", path, remotePath)

			// Upload file
			url, err := p.storage.Upload(path, remotePath)
			if err != nil {
				log.Printf("[HLS] Failed to upload %s: %v", path, err)
				return err
			}

			// Track master playlist URL
			if relPath == "master.m3u8" {
				streamingURL = url
				log.Printf("[HLS] Master playlist URL: %s", streamingURL)
			}

			return nil
		})

		if err != nil {
			return "", fmt.Errorf("failed to upload HLS files: %w", err)
		}

		if streamingURL == "" {
			return "", fmt.Errorf("master playlist not found")
		}

		log.Printf("[HLS] Final CDN master playlist URL: %s", streamingURL)

	} else {
		// DEVELOPMENT: Save locally
		log.Printf("[HLS] Development mode: copying files locally...")

		targetDir := filepath.Join(p.config.StorageConfig.LocalPath, "movies", folderName)

		// Create target directory
		if err := os.MkdirAll(targetDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create target directory: %w", err)
		}

		// Copy all files
		err := filepath.Walk(hlsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			// Calculate relative path
			relPath, _ := filepath.Rel(hlsDir, path)
			dstPath := filepath.Join(targetDir, relPath)

			// Ensure subdirectory exists
			if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
				return err
			}

			// Copy file
			src, err := os.Open(path)
			if err != nil {
				return err
			}
			defer src.Close()

			dst, err := os.Create(dstPath)
			if err != nil {
				return err
			}
			defer dst.Close()

			if _, err := io.Copy(dst, src); err != nil {
				return err
			}

			// Track master playlist URL
			if relPath == "master.m3u8" {
				streamingURL = p.config.StorageConfig.BaseURL + "/stream/" + folderName + "/master.m3u8"
				log.Printf("[HLS] Master playlist URL: %s", streamingURL)
			}

			return nil
		})

		if err != nil {
			return "", fmt.Errorf("failed to copy files: %w", err)
		}

		log.Printf("[HLS] Files copied to: %s", targetDir)
	}

	return streamingURL, nil
}
