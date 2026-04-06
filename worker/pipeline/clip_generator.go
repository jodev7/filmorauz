package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const clipCount = 12
const maxClipSeconds = 60
const clipRandomOffsetMax = 5

type ClipInfo struct {
	MovieID     string `json:"movie_id"`
	MovieTitle  string `json:"movie_title"`
	MovieSlug   string `json:"movie_slug"`
	MovieCode   string `json:"movie_code"`
	Filename    string `json:"filename"`
	Path        string `json:"path"`
	URL         string `json:"url"`
	Duration    int    `json:"duration"`
	Sequence    int    `json:"sequence"`
	StorageType string `json:"storage_type"`
}

type ClipSaveRequest struct {
	Clips []ClipInfo `json:"clips"`
}

func sanitizeSlug(slug string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9\-]`)
	sanitized := re.ReplaceAllString(strings.ToLower(slug), "")
	sanitized = strings.Trim(sanitized, "-")
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	return sanitized
}

func sanitizeFilename(name string) string {
	re := regexp.MustCompile(`[^a-zA-Z0-9а-яА-ЯёЁүҚқҳЎўНнгМмЛлКкЙйЗзЖжЕеДдГгВвБбАоОъЫыЬьЭэЮюЯяЁё\s\-_]`)
	sanitized := re.ReplaceAllString(name, "")
	sanitized = strings.TrimSpace(sanitized)
	sanitized = strings.ReplaceAll(sanitized, " ", "-")
	sanitized = strings.ToLower(sanitized)
	if len(sanitized) > 50 {
		sanitized = sanitized[:50]
	}
	return sanitized
}

func (p *Pipeline) generateClips(ctx context.Context, canonicalFolderName string, movieCode string, movieResult *MovieCreationResult, processedMasterPath string, finalUploadsPath string) error {
	log.Printf("[CLIP] ===== CLIP GENERATION START =====")
	log.Printf("[CLIP] Movie code: %s", movieCode)
	log.Printf("[CLIP] Movie title: %s", movieResult.DisplayTitle)
	log.Printf("[CLIP] Canonical folder: %s", canonicalFolderName)
	log.Printf("[CHECKPOINT] clip_input_path: %s", processedMasterPath)

	// Fail clearly if the processed master is missing or was not passed
	if processedMasterPath == "" {
		return fmt.Errorf("processed master path is empty — cannot generate clips")
	}
	if _, err := os.Stat(processedMasterPath); err != nil {
		return fmt.Errorf("processed master not found at %s: %w", processedMasterPath, err)
	}

	baseVideoPath := processedMasterPath
	log.Printf("[CHECKPOINT] ffprobe_target_path: %s", baseVideoPath)

	baseDir, err := os.Getwd()
	if err != nil {
		log.Printf("[CLIP] ERROR: getwd failed: %v", err)
		return fmt.Errorf("getwd: %w", err)
	}

	movieSlug := sanitizeSlug(movieResult.Slug)
	if movieSlug == "" {
		movieSlug = sanitizeSlug(movieResult.DisplayTitle)
	}

	devMode := p.config.StorageConfig.Mode != "prod"
	var baseURL string
	var storagePath string
	var outDir string

	if devMode {
		outDir = filepath.Join(baseDir, "uploads", "movies", "clips", canonicalFolderName)
		baseURL = p.config.StorageConfig.BaseURL
		storagePath = fmt.Sprintf("videos/clips/%s", canonicalFolderName)
	} else {
		outDir = filepath.Join(baseDir, "uploads", "movies", "clips", canonicalFolderName)
		baseURL = p.config.StorageConfig.BaseURL
		storagePath = fmt.Sprintf("videos/clips/%s", canonicalFolderName)
	}

	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("[CLIP] ERROR: failed to create output dir: %v", err)
		return fmt.Errorf("failed to create output dir: %w", err)
	}
	log.Printf("[CHECKPOINT] clip_output_directory: %s", outDir)

	durationMs, err := p.getVideoDurationMs(baseVideoPath)
	if err != nil {
		log.Printf("[CLIP] ERROR: ffprobe failed to get duration: %v", err)
		return fmt.Errorf("ffprobe failed to get duration: %w", err)
	}
	durationSec := float64(durationMs) / 1000.0
	log.Printf("[CLIP] Video duration: %.1fs, generating %d clips (max %ds each)", durationSec, clipCount, maxClipSeconds)

	logoPath := filepath.Join(baseDir, "docs", "logo.png")
	logoExists := false
	if _, statErr := os.Stat(logoPath); statErr == nil {
		logoExists = true
		log.Printf("[CLIP] Logo found: %s", logoPath)
	} else {
		log.Printf("[CLIP] WARNING: logo not found at %s, clips will not have logo", logoPath)
	}

	log.Printf("[CLIP] Starting clip generation...")

	segmentSec := durationSec / float64(clipCount)
	clipDur := segmentSec
	if clipDur > maxClipSeconds {
		clipDur = maxClipSeconds
	}

	topText := fmt.Sprintf("Kino kodi\\: %s", movieCode)
	bottomText := "Kinoni profildagi botdan toping\\!"

	var generatedClips []ClipInfo
	var failedClips []int

	for i := 0; i < clipCount; i++ {
		select {
		case <-ctx.Done():
			log.Printf("[CLIP] Context cancelled, stopping clip generation")
			return ctx.Err()
		default:
		}

		randomOffset := float64(rand.Intn(clipRandomOffsetMax*2+1)) - float64(clipRandomOffsetMax)
		startSec := float64(i)*segmentSec + randomOffset
		if startSec < 0 {
			startSec = 0
		}
		maxStart := durationSec - clipDur
		if startSec > maxStart {
			startSec = maxStart
		}
		if startSec < 0 {
			startSec = 0
		}

		timestamp := time.Now().UnixNano()
		clipFilename := fmt.Sprintf("%s_%d_%d.mp4", movieSlug, timestamp, i+1)
		outPath := filepath.Join(outDir, clipFilename)

		log.Printf("[CLIP] Generating clip %d/%d: start=%.1fs (offset=%.1fs) dur=%.1fs -> %s",
			i+1, clipCount, startSec, randomOffset, clipDur, outPath)

		var args []string
		if logoExists {
			filterComplex := fmt.Sprintf(
				"[0:v]drawtext=text='%s':x=(w-text_w)/2:y=20:fontsize=40:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=h-text_h-20:fontsize=36:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10[text];[text][1:v]overlay=W-w-20:H-h-20[out]",
				topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-i", baseVideoPath,
				"-i", logoPath,
				"-filter_complex", filterComplex,
				"-map", "[out]",
				"-map", "0:a?",
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
				"-c:a", "aac",
				"-b:a", "128k",
				"-movflags", "+faststart",
				"-progress", "pipe:1",
				outPath,
			}
		} else {
			textFilter := fmt.Sprintf(
				"drawtext=text='%s':x=(w-text_w)/2:y=20:fontsize=40:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=h-text_h-20:fontsize=36:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10",
				topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-i", baseVideoPath,
				"-vf", textFilter,
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
				"-c:a", "aac",
				"-b:a", "128k",
				"-movflags", "+faststart",
				"-progress", "pipe:1",
				outPath,
			}
		}

		log.Printf("[CLIP] FFmpeg command: ffmpeg %s", strings.Join(args, " "))

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var stderrBuf bytes.Buffer
		cmd.Stderr = &stderrBuf

		if err := cmd.Start(); err != nil {
			log.Printf("[CLIP] ERROR: Failed to start ffmpeg for clip %d: %v", i+1, err)
			failedClips = append(failedClips, i+1)
			continue
		}

		if err := cmd.Wait(); err != nil {
			stderr := stderrBuf.String()
			log.Printf("[CLIP] ERROR: Clip %d failed: %v", i+1, err)
			log.Printf("[CLIP] FFmpeg stderr:\n%s", stderr)
			log.Printf("[CLIP] Command was: ffmpeg %s", strings.Join(args, " "))
			failedClips = append(failedClips, i+1)
			continue
		}

		clipDuration := int(clipDur)
		var clipURL string
		if devMode {
			clipURL = fmt.Sprintf("%s/stream/%s/clips/%s", baseURL, canonicalFolderName, clipFilename)
		} else {
			clipURL = fmt.Sprintf("%s/videos/clips/%s/%s", baseURL, canonicalFolderName, clipFilename)
		}

		clipInfo := ClipInfo{
			MovieID:     fmt.Sprintf("%v", movieResult.MovieID),
			MovieTitle:  movieResult.DisplayTitle,
			MovieSlug:   movieResult.Slug,
			MovieCode:   movieCode,
			Filename:    clipFilename,
			Path:        filepath.Join(storagePath, clipFilename),
			URL:         clipURL,
			Duration:    clipDuration,
			Sequence:    i + 1,
			StorageType: map[bool]string{true: "local", false: "b2"}[devMode],
		}
		generatedClips = append(generatedClips, clipInfo)

		log.Printf("[CLIP] Clip %d saved successfully: %s (duration: %ds)", i+1, outPath, clipDuration)
	}

	if len(generatedClips) == 0 {
		log.Printf("[CLIP] ERROR: No clips were generated successfully")
		return fmt.Errorf("no clips generated successfully")
	}

	if len(failedClips) > 0 {
		log.Printf("[CLIP] WARNING: %d clips failed: %v", len(failedClips), failedClips)
	}

	log.Printf("[CLIP] ===== UPLOAD START =====")
	log.Printf("[CLIP] Uploading %d clips to storage...", len(generatedClips))

	if !devMode {
		log.Printf("[CLIP] Production mode: uploading to B2...")
		for i := range generatedClips {
			srcPath := filepath.Join(outDir, generatedClips[i].Filename)
			remotePath := filepath.Join(storagePath, generatedClips[i].Filename)

			log.Printf("[CLIP] Uploading clip %d/%d: %s -> %s", i+1, len(generatedClips), srcPath, remotePath)

			url, err := p.storage.Upload(srcPath, remotePath)
			if err != nil {
				log.Printf("[CLIP] ERROR: Failed to upload clip %d: %v", i+1, err)
				continue
			}
			generatedClips[i].URL = url
			log.Printf("[CLIP] Uploaded: %s", url)
		}
	}
	log.Printf("[CLIP] ===== UPLOAD END =====")

	log.Printf("[CLIP] ===== MONGODB SAVE START =====")
	log.Printf("[CLIP] Saving %d clip records to MongoDB...", len(generatedClips))

	if err := p.saveClipsToMongoDB(ctx, generatedClips); err != nil {
		log.Printf("[CLIP] ERROR: Failed to save clips to MongoDB: %v", err)
		return fmt.Errorf("failed to save clips to MongoDB: %w", err)
	}

	log.Printf("[CLIP] ===== MONGODB SAVE END =====")

	log.Printf("[CLIP] ===== CLEANUP START =====")
	// Note: readyvideo folder cleanup is handled in pipeline.go after clip generation completes
	log.Printf("[CLIP] Clip generation cleanup complete")
	log.Printf("[CLIP] ===== CLEANUP END =====")

	log.Printf("[CLIP] ===== CLIP GENERATION COMPLETE =====")
	log.Printf("[CLIP] Movie: %s (code: %s)", movieResult.DisplayTitle, movieCode)
	log.Printf("[CLIP] Clips generated: %d/%d", len(generatedClips), clipCount)
	log.Printf("[CLIP] Clips failed: %d", len(failedClips))
	log.Printf("[CLIP] Output location: %s", outDir)
	if len(failedClips) > 0 {
		log.Printf("[CLIP] Failed clip indices: %v", failedClips)
	}

	return nil
}

func (p *Pipeline) saveClipsToMongoDB(ctx context.Context, clips []ClipInfo) error {
	apiURL := os.Getenv("API_URL")
	if apiURL == "" {
		apiURL = "http://localhost:8080"
	}

	saveURL := fmt.Sprintf("%s/api/admin/clips", apiURL)

	reqBody := ClipSaveRequest{Clips: clips}
	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal clip data: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", saveURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	log.Printf("[CLIP] Sending clip data to: %s", saveURL)
	log.Printf("[CLIP] Clip data: %s", string(jsonData))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	log.Printf("[CLIP] MongoDB save response status: %d", resp.StatusCode)
	log.Printf("[CLIP] MongoDB save response body: %s", string(body))

	if resp.StatusCode >= 400 {
		return fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
