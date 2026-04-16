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
	"sync"

	"github.com/filmorauz/worker/models"
)

const DefaultMaxRenditionConcurrency = 2

func getMaxRenditionConcurrency(maxConcurrent int) int {
	if maxConcurrent > 0 {
		return maxConcurrent
	}
	return DefaultMaxRenditionConcurrency
}

// RenditionConfig defines the configuration for a single HLS rendition
type RenditionConfig struct {
	Name         string // Display name (e.g., "360p", "480p")
	Width        int    // Video width
	Height       int    // Video height
	VideoBitrate string // Target video bitrate (e.g., "4500k")
	MaxBitrate   string // Max bitrate (1.2x target)
	BufSize      string // Buffer size (2x target)
	AudioBitrate string // Audio bitrate (e.g., "128k")
	Bandwidth    int    // Total bandwidth in bits/sec for master playlist
	CRF          int    // Constant Rate Factor (18-23, lower = higher quality)
}

// DefaultRenditions returns the default adaptive HLS renditions (legacy - use getApplicableRenditions for production)
func DefaultRenditions() []RenditionConfig {
	return []RenditionConfig{
		{Name: "360p", Width: 640, Height: 360, VideoBitrate: "800k", AudioBitrate: "96k", Bandwidth: 896000, CRF: 23},
		{Name: "480p", Width: 854, Height: 480, VideoBitrate: "1200k", AudioBitrate: "128k", Bandwidth: 1328000, CRF: 22},
		{Name: "720p", Width: 1280, Height: 720, VideoBitrate: "2800k", AudioBitrate: "128k", Bandwidth: 2928000, CRF: 21},
		{Name: "1080p", Width: 1920, Height: 1080, VideoBitrate: "5000k", AudioBitrate: "128k", Bandwidth: 5128000, CRF: 20},
	}
}

// processAdaptiveHLS generates multi-bitrate adaptive HLS with a master playlist
// This function:
// 1. First creates a clean base video with logo overlay applied
// 2. Then generates multiple quality renditions from the base
// 3. Creates a master.m3u8 playlist referencing all variants
// No watermark removal - use source video directly
func (p *Pipeline) processAdaptiveHLS(jobID, inputPath, outputDir string, cutSeconds int, jobStatusCallback func(status models.IngestionStatus, progress int), maxConcurrentRenditions int, segmentUploadWorkers int, segmentUploadRetries int) (masterPlaylistPath, processedMasterPath string, generatedQualities []string, err error) {
	log.Printf("[HLS] Starting adaptive HLS generation for job %s", jobID)
	log.Printf("[CHECKPOINT] hls_raw_input_path: %s", inputPath)
	log.Printf("[HLS] Output directory: %s", outputDir)

	if segmentUploadWorkers <= 0 {
		segmentUploadWorkers = SegmentUploadConcurrency
	}
	if segmentUploadRetries <= 0 {
		segmentUploadRetries = SegmentRetryMax
	}

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
		return "", "", nil, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Get input resolution to determine which renditions to generate
	inputWidth, inputHeight, resErr := p.getInputResolution(inputPath)
	if resErr != nil {
		log.Printf("[HLS] WARNING: Could not detect input resolution: %v, using defaults", resErr)
		inputWidth = 1920
		inputHeight = 1080
	}
	log.Printf("[HLS] Input resolution detected: %dx%d", inputWidth, inputHeight)

	// Determine which renditions to generate based on input resolution
	renditions := p.getApplicableRenditions(inputWidth, inputHeight)
	log.Printf("[HLS] Source resolution %dx%d - generating %d renditions: %v", inputWidth, inputHeight, len(renditions), getRenditionNames(renditions))

	// Step 1: Cut + logo overlay → processed_master.mp4
	// This is the single processed master used for both HLS and clip generation.
	// It is NOT deleted here — the caller cleans it up after clip generation.
	processedMasterPath = filepath.Join(outputDir, "processed_master.mp4")
	log.Printf("[CHECKPOINT] processed_master_path: %s", processedMasterPath)
	log.Printf("[STAGE] logo_overlay start — raw_input: %s → master: %s", inputPath, processedMasterPath)
	if err = p.createBaseVideo(inputPath, processedMasterPath, cutSeconds, jobStatusCallback); err != nil {
		return "", "", nil, fmt.Errorf("failed to create processed master: %w", err)
	}
	log.Printf("[STAGE] logo_overlay end — processed_master: %s", processedMasterPath)
	log.Printf("[CHECKPOINT] hls_input_path: %s", processedMasterPath)

	// Report progress: master created, starting HLS encoding (~55%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 55)
	}

	// Step 2: Generate each HLS rendition from the processed master
	log.Printf("[STAGE] hls_renditions start — renditions: %d, source: %s", len(renditions), processedMasterPath)

	streamingUploader := newStreamingUploader(p.storage, jobID, renditions, segmentUploadWorkers, segmentUploadRetries, outputDir)
	streamingUploader.start()
	log.Printf("[STAGE] streaming_upload start — folder: %s, renditions: %d, workers: %d, retries: %d", jobID, len(renditions), segmentUploadWorkers, segmentUploadRetries)

	type renditionJob struct {
		index        int
		rendition    RenditionConfig
		renditionDir string
		baseProgress int
	}

	jobs := make([]renditionJob, len(renditions))
	for i, r := range renditions {
		renditionDir := filepath.Join(outputDir, r.Name)
		if err = os.MkdirAll(renditionDir, 0755); err != nil {
			return "", "", nil, fmt.Errorf("failed to create rendition directory: %w", err)
		}
		baseProgress := 55 + (i * 30 / len(renditions))
		jobs[i] = renditionJob{
			index:        i,
			rendition:    r,
			renditionDir: renditionDir,
			baseProgress: baseProgress,
		}
	}

	maxConc := getMaxRenditionConcurrency(maxConcurrentRenditions)

	sem := make(chan struct{}, maxConc)
	var wg sync.WaitGroup
	errors := make([]error, len(jobs))

	log.Printf("[HLS] Starting parallel rendition generation (concurrency: %d)", maxConc)

	for _, job := range jobs {
		wg.Add(1)
		go func(j renditionJob) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			log.Printf("[HLS] Starting %s rendition...", j.rendition.Name)
			err := p.generateHLSRendition(jobID, processedMasterPath, j.renditionDir, j.rendition, j.baseProgress, jobStatusCallback)
			if err != nil {
				errors[j.index] = fmt.Errorf("failed to generate %s rendition: %w", j.rendition.Name, err)
				log.Printf("[HLS] ERROR %s: %v", j.rendition.Name, err)
			} else {
				log.Printf("[HLS] Completed %s rendition", j.rendition.Name)
			}
		}(job)
	}

	wg.Wait()

	for _, err := range errors {
		if err != nil {
			streamingUploader.stopAndWait()
			return "", "", nil, fmt.Errorf("rendition generation failed: %w", err)
		}
	}

	log.Printf("[HLS] All renditions complete, waiting for segment uploads...")
	log.Printf("[STAGE] streaming_upload wait")

	if err = streamingUploader.stopAndWait(); err != nil {
		return "", "", nil, fmt.Errorf("segment upload failed: %w", err)
	}

	log.Printf("[STAGE] streaming_upload done")
	log.Printf("[HLS] All renditions complete, creating master playlist")
	log.Printf("[STAGE] hls_renditions end — renditions: %d", len(renditions))

	// Report progress: HLS encoding complete, creating master playlist (~88%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 88)
	}

	// Step 3: Create master playlist
	log.Printf("[STAGE] master_playlist start")
	masterPlaylistPath = filepath.Join(outputDir, "master.m3u8")
	if err = p.createMasterPlaylist(masterPlaylistPath, renditions); err != nil {
		return "", "", nil, fmt.Errorf("failed to create master playlist: %w", err)
	}
	log.Printf("[STAGE] master_playlist end")

	// Report progress: HLS generation complete (~92%)
	if jobStatusCallback != nil {
		jobStatusCallback(models.IngestionStatusProcessing, 92)
	}

	// Verify output
	files, dirErr := os.ReadDir(outputDir)
	if dirErr != nil {
		return "", "", nil, fmt.Errorf("failed to read output directory: %w", dirErr)
	}
	log.Printf("[HLS] Adaptive HLS generation complete. Output files:")
	for _, f := range files {
		log.Printf("[HLS]   - %s", f.Name())
	}

	log.Printf("[HLS] Master playlist: %s", masterPlaylistPath)
	log.Printf("[CHECKPOINT] hls_master_playlist: %s", masterPlaylistPath)
	log.Printf("[STAGE] hls_processing end — master: %s, processed_master: %s", masterPlaylistPath, processedMasterPath)

	// Build list of generated qualities
	generatedQualities = make([]string, len(renditions))
	for i, r := range renditions {
		generatedQualities[i] = r.Name
	}
	log.Printf("[HLS] Generated qualities: %v", generatedQualities)

	return masterPlaylistPath, processedMasterPath, generatedQualities, nil
}

// getApplicableRenditions returns the renditions that should be generated based on input resolution
// Uses height-based smart selection - never upscale
// Height >= 1080: generate [1080p, 720p, 480p]
// Height >= 720:  generate [720p, 480p]
// Height < 720:   generate [480p only]
// Each rendition includes optimized bitrate settings for streaming
func (p *Pipeline) getApplicableRenditions(inputWidth, inputHeight int) []RenditionConfig {
	applicable := make([]RenditionConfig, 0)

	// Smart height-based selection - never upscale
	// Bitrate strategy: target → maxrate=1.2x, bufsize=2x
	if inputHeight >= 1080 {
		// Source is 1080p or higher - generate all three
		// 1080p: target 5000k, max 6000k, buf 10000k, CRF 20
		applicable = append(applicable, RenditionConfig{
			Name: "1080p", Width: 1920, Height: 1080,
			VideoBitrate: "5000k", MaxBitrate: "6000k", BufSize: "10000k",
			AudioBitrate: "128k", Bandwidth: 5300000, CRF: 20,
		})
		// 720p: target 2800k, max 3360k, buf 5600k, CRF 21
		applicable = append(applicable, RenditionConfig{
			Name: "720p", Width: 1280, Height: 720,
			VideoBitrate: "2800k", MaxBitrate: "3360k", BufSize: "5600k",
			AudioBitrate: "128k", Bandwidth: 3020000, CRF: 21,
		})
		// 480p: target 1200k, max 1440k, buf 2400k, CRF 22
		applicable = append(applicable, RenditionConfig{
			Name: "480p", Width: 854, Height: 480,
			VideoBitrate: "1200k", MaxBitrate: "1440k", BufSize: "2400k",
			AudioBitrate: "128k", Bandwidth: 1320000, CRF: 22,
		})
		log.Printf("[HLS] Source height >= 1080px: generating [1080p(5000k), 720p(2800k), 480p(1200k)]")
	} else if inputHeight >= 720 {
		// Source is 720p - generate 720p and 480p
		// 720p: target 3000k, max 3600k, buf 6000k, CRF 20
		applicable = append(applicable, RenditionConfig{
			Name: "720p", Width: 1280, Height: 720,
			VideoBitrate: "3000k", MaxBitrate: "3600k", BufSize: "6000k",
			AudioBitrate: "128k", Bandwidth: 3200000, CRF: 20,
		})
		// 480p: target 1200k, max 1440k, buf 2400k, CRF 22
		applicable = append(applicable, RenditionConfig{
			Name: "480p", Width: 854, Height: 480,
			VideoBitrate: "1200k", MaxBitrate: "1440k", BufSize: "2400k",
			AudioBitrate: "128k", Bandwidth: 1320000, CRF: 22,
		})
		log.Printf("[HLS] Source height >= 720px and < 1080px: generating [720p(3000k), 480p(1200k)]")
	} else {
		// Source is below 720p - only generate 480p (no upscaling)
		// 480p: target 1500k, max 1800k, buf 3000k, CRF 20 (higher quality for low-res source)
		applicable = append(applicable, RenditionConfig{
			Name: "480p", Width: 854, Height: 480,
			VideoBitrate: "1500k", MaxBitrate: "1800k", BufSize: "3000k",
			AudioBitrate: "128k", Bandwidth: 1620000, CRF: 20,
		})
		log.Printf("[HLS] Source height < 720px: generating [480p(1500k) only] - no upscaling")
	}

	// Defensive: if somehow nothing was added, add 480p as fallback
	if len(applicable) == 0 {
		applicable = append(applicable, RenditionConfig{
			Name: "480p", Width: 854, Height: 480,
			VideoBitrate: "1200k", MaxBitrate: "1440k", BufSize: "2400k",
			AudioBitrate: "128k", Bandwidth: 1320000, CRF: 22,
		})
		log.Printf("[HLS] Fallback: generating [480p(1200k)]")
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

// fileExists reports whether the named file exists and is readable.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// createBaseVideo creates an intermediate base video with logo overlay applied
func (p *Pipeline) createBaseVideo(inputPath, outputPath string, cutSeconds int, jobStatusCallback func(status models.IngestionStatus, progress int)) error {
	log.Printf("[HLS] Creating base video: cut=%ds, logo overlay, input=%s", cutSeconds, inputPath)
	log.Printf("[CHECKPOINT] raw_downloaded_input_path: %s", inputPath)
	log.Printf("[HLS] NOTE: No watermark removal - using source video directly")

	// Video chain: ensure even dimensions for H.264 (required by libx264).
	videoChain := "scale=trunc(iw/2)*2:trunc(ih/2)*2"

	// Resolve logo path: try CWD/docs/logo.png first (works for 'go run .'),
	// then fall back to the directory containing the running executable (works
	// for deployed binaries run from an arbitrary working directory).
	logoPath := ""
	if cwd, err := os.Getwd(); err == nil {
		if candidate := filepath.Join(cwd, "docs", "logo.png"); fileExists(candidate) {
			logoPath = candidate
		}
	}
	if logoPath == "" {
		if exe, err := os.Executable(); err == nil {
			if candidate := filepath.Join(filepath.Dir(exe), "docs", "logo.png"); fileExists(candidate) {
				logoPath = candidate
			}
		}
	}
	logoExists := logoPath != ""
	if logoExists {
		log.Printf("[HLS] Watermark logo found: %s", logoPath)
	} else {
		log.Printf("[HLS] WARNING: watermark logo not found in docs/logo.png (checked CWD and executable dir), skipping overlay")
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
		// scale2ref=w=iw*0.18 → logo is 18% of video width (~3× larger than iw/7):
		//   1080p(1920px)→346px  720p(1280px)→230px  360p(640px)→115px
		// overlay=W-w-20:H-h-20 → bottom-right corner, 20px padding.
		filterComplex := fmt.Sprintf(
			"[0:v]%s[vscaled];[1:v][vscaled]scale2ref=w=iw*0.18:h=-1[logo][vref];[vref][logo]overlay=W-w-20:H-h-20:shortest=1[out]",
			videoChain,
		)
		filterArgs = []string{
			"-loop", "1",
			"-i", logoPath,
			"-filter_complex", filterComplex,
			"-map", "[out]",
			"-map", "0:a?",
		}
		filterDescription = "logo watermark 18%-video-width bottom-right 20px padding"
		log.Printf("[HLS] APPLYING logo overlay — scale=video_width*0.18  position=bottom-right+20px")
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
	log.Printf("[ENCODER] === START %s encoding === (target: %dx%d, bitrate: %s)", rendition.Name, rendition.Width, rendition.Height, rendition.VideoBitrate)

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

	// Build maxrate and bufsize from rendition config or use defaults
	maxRate := rendition.MaxBitrate
	bufSize := rendition.BufSize
	if maxRate == "" {
		maxRate = fmt.Sprintf("%.0fk", parseBitrate(rendition.VideoBitrate)*1.2)
	}
	if bufSize == "" {
		bufSize = fmt.Sprintf("%.0fk", parseBitrate(rendition.VideoBitrate)*2)
	}

	// Keyframe settings for smooth HLS streaming:
	// - GOP (Group of Pictures) = 2 seconds for optimal seeking
	// - -sc_threshold 0: disable random scene detection (forces keyframes at GOP)
	// - -force_key_frames: force IDR at segment boundaries for clean switching
	// - -hls_time 4: 4-second segments (aligns with 2s GOP = 2 keyframes per segment)

	// Determine FPS from base video (default 24fps)
	fps := 24.0
	if fpsInfo, err := p.getVideoFPS(baseVideoPath); err == nil {
		fps = fpsInfo
	}
	gopSize := int(fps * 2) // 2 seconds * fps = GOP in frames
	segmentDuration := 4    // 4-second segments align with 2s GOP (2 keyframes per segment)

	// Force keyframes every segment duration (4 seconds)
	// This ensures IDR frames at segment boundaries for clean switching
	forceKeyframes := fmt.Sprintf("expr:gte(t,n_forced*%.0f)", float64(segmentDuration))

	crf := rendition.CRF
	if crf == 0 {
		crf = 22 // default CRF
	}

	log.Printf("[KEYFRAME] %s: GOP=%d frames (%.1ffps × 2s), segment=%ds, force_keyframes=%s",
		rendition.Name, gopSize, fps, segmentDuration, forceKeyframes)

	ffmpegArgs := []string{
		"-y",                // Overwrite output
		"-i", baseVideoPath, // Input: base video with filters applied
		"-vf", fmt.Sprintf("scale=%d:%d:force_original_aspect_ratio=decrease,pad=%d:%d:(ow-iw)/2:(oh-ih)/2",
			rendition.Width, rendition.Height, rendition.Width, rendition.Height), // Scale and pad to exact resolution
		"-c:v", "libx264", // H.264 video codec
		"-preset", "veryfast", // Fast encoding preset
		"-crf", fmt.Sprintf("%d", crf), // CRF for quality (lower = better quality)
		"-b:v", rendition.VideoBitrate, // Target bitrate (used with CRF for rate control)
		"-maxrate", maxRate, // Max bitrate (1.2x target)
		"-bufsize", bufSize, // Buffer size (2x target)
		"-profile:v", "high", // High profile for better quality
		"-level", "4.1", // Level 4.1 for HD content
		"-sc_threshold", "0", // Disable scene cut detection - forces keyframes at GOP
		"-g", fmt.Sprintf("%d", gopSize), // GOP size: 2 seconds × fps
		"-keyint_min", fmt.Sprintf("%d", gopSize), // Minimum keyframe interval = GOP
		"-force_key_frames", forceKeyframes, // Force IDR at segment boundaries
		"-c:a", "aac", // AAC audio codec
		"-b:a", rendition.AudioBitrate, // Audio bitrate (128k stereo)
		"-ac", "2", // Stereo audio
		"-f", "hls", // HLS output format
		"-hls_time", fmt.Sprintf("%d", segmentDuration), // Segment duration
		"-hls_list_size", "0", // Include all segments in playlist
		"-hls_playlist_type", "vod", // VOD playlist type
		"-hls_segment_filename", segmentPattern, // Segment filename pattern
		hlsPlaylist,           // Output playlist
		"-progress", "pipe:1", // Output progress to stdout
	}

	log.Printf("[BITRATE] %s: target=%s, max=%s, buf=%s, crf=%d, audio=%s",
		rendition.Name, rendition.VideoBitrate, maxRate, bufSize, crf, rendition.AudioBitrate)
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

	// Verify output and log file sizes
	files, err := os.ReadDir(outputDir)
	if err != nil || len(files) == 0 {
		return fmt.Errorf("no output files generated for %s", rendition.Name)
	}

	// Calculate total size of all segments
	totalSize := int64(0)
	for _, f := range files {
		if info, err := f.Info(); err == nil {
			totalSize += info.Size()
		}
	}
	totalSizeMB := float64(totalSize) / (1024 * 1024)

	log.Printf("[BITRATE] %s encoding complete: %d files, total size: %.2f MB", rendition.Name, len(files), totalSizeMB)
	log.Printf("[ENCODER] === END %s encoding === (%d files, %.2f MB)", rendition.Name, len(files), totalSizeMB)
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
		log.Printf("[HLS] Uploading to B2/CDN with parallel upload...")

		type fileJob struct {
			LocalPath  string
			RemotePath string
			Data       []byte
		}

		var jobs []fileJob

		err := filepath.Walk(hlsDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if info.IsDir() {
				return nil
			}

			relPath, _ := filepath.Rel(hlsDir, path)
			remotePath := filepath.Join("videos", folderName, relPath)

			if strings.HasSuffix(path, ".ts") {
				log.Printf("[HLS] Skipping .ts segment (already uploaded via streaming): %s", relPath)
				return nil
			}

			log.Printf("[HLS] Uploading playlist: %s -> %s", path, remotePath)

			data, err := os.ReadFile(path)
			if err != nil {
				log.Printf("[HLS] Failed to read %s: %v", path, err)
				return err
			}

			jobs = append(jobs, fileJob{
				LocalPath:  path,
				RemotePath: remotePath,
				Data:       data,
			})

			return nil
		})

		if err != nil {
			return "", fmt.Errorf("failed to read HLS files: %w", err)
		}

		log.Printf("[HLS] Collected %d files for parallel upload", len(jobs))

		ParallelUploadJobs := make([]struct {
			LocalPath  string
			RemotePath string
			Data       []byte
		}, len(jobs))

		for i, job := range jobs {
			ParallelUploadJobs[i] = struct {
				LocalPath  string
				RemotePath string
				Data       []byte
			}{
				LocalPath:  job.LocalPath,
				RemotePath: job.RemotePath,
				Data:       job.Data,
			}
		}

		results, err := p.storage.ParallelUpload(ParallelUploadJobs)
		if err != nil {
			log.Printf("[HLS] Parallel upload error: %v", err)
			return "", fmt.Errorf("failed to upload HLS files: %w", err)
		}

		for _, r := range results {
			if r.Err != nil {
				log.Printf("[HLS] Upload failed for %s: %v", r.RemotePath, r.Err)
			}
			if r.RemotePath == "videos/"+folderName+"/master.m3u8" || strings.HasSuffix(r.RemotePath, "/master.m3u8") {
				streamingURL = r.URL
				log.Printf("[HLS] Master playlist URL: %s", streamingURL)
			}
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
