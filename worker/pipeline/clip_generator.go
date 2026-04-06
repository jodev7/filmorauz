package pipeline

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
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

		// Build ffmpeg args.
		// Key points:
		//   -ss / -t before -i  → input-level seek + hard duration limit (fast, reliable)
		//   -loop 1 before logo → PNG loops for the full clip so overlay never runs out of frames
		//   :shortest=1 on overlay → stop compositing when the video stream ends
		//   No -progress pipe:1 → stdout not consumed in this path; omit to avoid pipe stalls
		var args []string
		if logoExists {
			filterComplex := fmt.Sprintf(
				// Scale to 9:16 (1080x1920) with center crop, then add text overlays, then logo
				"[0:v]scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=40:fontsize=40:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=h-text_h-40:fontsize=36:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10[vt];"+
					"[vt][1:v]overlay=W-w-20:H-h-20:shortest=1[out]",
				topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-t", fmt.Sprintf("%.3f", clipDur), // hard output duration limit
				"-i", baseVideoPath,
				"-loop", "1", // logo PNG loops for full clip duration
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
				outPath,
			}
		} else {
			textFilter := fmt.Sprintf(
				// Scale to 9:16 (1080x1920) with center crop, then add text overlays
				"scale=1080:1920:force_original_aspect_ratio=increase,crop=1080:1920,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=40:fontsize=40:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10,"+
					"drawtext=text='%s':x=(w-text_w)/2:y=h-text_h-40:fontsize=36:fontcolor=white:box=1:boxcolor=black@0.5:boxborderw=10",
				topText, bottomText,
			)
			args = []string{
				"-y",
				"-ss", fmt.Sprintf("%.3f", startSec),
				"-t", fmt.Sprintf("%.3f", clipDur), // hard output duration limit
				"-i", baseVideoPath,
				"-vf", textFilter,
				"-c:v", "libx264",
				"-preset", "veryfast",
				"-crf", "23",
				"-c:a", "aac",
				"-b:a", "128k",
				"-movflags", "+faststart",
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

		// Validate the generated clip: probe actual duration and resolution.
		// Fail the clip if duration exceeds the allowed maximum.
		if actualMs, probeErr := p.getVideoDurationMs(outPath); probeErr != nil {
			log.Printf("[CLIP] WARNING: Could not probe clip %d duration: %v", i+1, probeErr)
		} else {
			actualSec := float64(actualMs) / 1000.0
			if w, h, _ := p.getInputResolution(outPath); w > 0 {
				log.Printf("[CLIP] Clip %d validated — duration=%.1fs resolution=%dx%d", i+1, actualSec, w, h)
			} else {
				log.Printf("[CLIP] Clip %d validated — duration=%.1fs", i+1, actualSec)
			}
			if actualSec > float64(maxClipSeconds)+1 {
				log.Printf("[CLIP] ERROR: Clip %d duration %.1fs exceeds max %ds — discarding", i+1, actualSec, maxClipSeconds)
				os.Remove(outPath)
				failedClips = append(failedClips, i+1)
				continue
			}
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
	if p.config.DB == nil {
		return fmt.Errorf("database not configured — cannot save clips")
	}

	col := p.config.DB.Collection("clips")

	docs := make([]interface{}, 0, len(clips))
	for _, clip := range clips {
		var movieObjID primitive.ObjectID
		if oid, err := primitive.ObjectIDFromHex(clip.MovieID); err == nil {
			movieObjID = oid
		} else {
			log.Printf("[CLIP] WARNING: could not parse movie_id %q as ObjectID: %v", clip.MovieID, err)
		}

		doc := bson.M{
			"_id":          primitive.NewObjectID(),
			"movie_id":     movieObjID,
			"movie_title":  clip.MovieTitle,
			"movie_slug":   clip.MovieSlug,
			"movie_code":   clip.MovieCode,
			"filename":     clip.Filename,
			"path":         clip.Path,
			"url":          clip.URL,
			"duration":     clip.Duration,
			"sequence":     clip.Sequence,
			"storage_type": clip.StorageType,
			"created_at":   time.Now(),
		}
		docs = append(docs, doc)
	}

	result, err := col.InsertMany(ctx, docs)
	if err != nil {
		return fmt.Errorf("failed to insert clips into MongoDB: %w", err)
	}

	log.Printf("[CLIP] Saved %d clips to MongoDB", len(result.InsertedIDs))
	return nil
}
