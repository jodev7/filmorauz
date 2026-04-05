package pipeline

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/filmorauz/worker/models"
)

// removeWatermarkDynamic runs the Python watermark removal pipeline on inputPath.
// Returns the path to the clean video, or inputPath if removal failed/was skipped.
// Never returns an error that should abort the pipeline — always falls back.
func (p *Pipeline) removeWatermarkDynamic(ctx context.Context, job *models.IngestionJob, inputPath string) (string, error) {
	jobID := job.ID.Hex()
	log.Printf("[WATERMARK] Starting dynamic watermark removal for job=%s input=%s", jobID, inputPath)

	// Build output path alongside input: <name>_clean.<ext>
	dir := filepath.Dir(inputPath)
	base := filepath.Base(inputPath)
	ext := filepath.Ext(base)
	nameNoExt := strings.TrimSuffix(base, ext)
	outputPath := filepath.Join(dir, nameNoExt+"_clean"+ext)

	result, err := p.watermarkService.RemoveWatermark(ctx, inputPath)
	if err != nil {
		log.Printf("[WATERMARK] RemoveWatermark error: %v — using original", err)
		return inputPath, nil
	}

	if !result.Success {
		log.Printf("[WATERMARK] Removal not successful (error=%s) — using original", result.Error)
		return inputPath, nil
	}

	if !result.WatermarkDetected {
		log.Printf("[WATERMARK] No watermark detected — using original")
		return inputPath, nil
	}

	if result.FallbackUsed {
		log.Printf("[WATERMARK] Fallback was used — using original")
		return inputPath, nil
	}

	// Use the output path from the result if provided and exists
	cleanPath := result.OutputPath
	if cleanPath == "" {
		cleanPath = outputPath
	}

	if _, statErr := os.Stat(cleanPath); statErr != nil {
		log.Printf("[WATERMARK] Clean output not found at %s — using original", cleanPath)
		return inputPath, nil
	}

	log.Printf("[WATERMARK] Watermark removed successfully: %s → %s (mode=%s, regions=%d)",
		inputPath, cleanPath, result.ModeUsed, len(result.Regions))
	for i, r := range result.Regions {
		log.Printf("[WATERMARK]   Region %d: type=%s location=%s confidence=%.2f",
			i+1, r.WatermarkType, r.Location, r.Confidence)
	}

	return cleanPath, nil
}
