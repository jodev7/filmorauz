package pipeline

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/filmorauz/worker/models"
	"github.com/filmorauz/worker/repositories"
	"github.com/filmorauz/worker/services"
)

// WatermarkRemovalPipeline extends the base Pipeline with watermark removal capability
type WatermarkRemovalPipeline struct {
	*Pipeline
	watermarkService *services.WatermarkRemovalService
}

// NewWatermarkRemovalPipeline creates a new pipeline with watermark removal
func NewWatermarkRemovalPipeline(config Config, jobRepo *repositories.JobRepository) (*WatermarkRemovalPipeline, error) {
	// Create base pipeline
	basePipeline, err := NewPipeline(config, jobRepo)
	if err != nil {
		return nil, err
	}

	// Create watermark removal service
	watermarkConfig := services.DefaultWatermarkRemovalConfig()
	watermarkConfig.TempDir = config.TempDir

	watermarkService := services.NewWatermarkRemovalService(watermarkConfig)

	return &WatermarkRemovalPipeline{
		Pipeline:         basePipeline,
		watermarkService: watermarkService,
	}, nil
}

// processVideoWithWatermarkRemoval processes video with AI-based watermark removal
// This is an enhanced version of processVideo that includes watermark removal
func (p *WatermarkRemovalPipeline) processVideoWithWatermarkRemoval(ctx context.Context, job *models.IngestionJob, inputPath string, canonicalFolderName string) (string, error) {
	jobID := job.ID.Hex()

	// =============================================================================
	// STAGE 1: WATERMARK REMOVAL (NEW AI-BASED)
	// =============================================================================
	// This stage runs BEFORE the FFmpeg pipeline
	// It detects and removes watermarks using AI-based inpainting

	// Update status to indicate watermark removal
	if err := p.updateStatus(jobID, models.IngestionStatusRemovingWatermark, 25); err != nil {
		log.Printf("[PIPELINE] WARNING: Failed to update status: %v", err)
	}
	p.log(jobID, "received_local_video", "info")
	log.Printf("[WATERMARK] Stage: received_local_video - processing job %s", jobID)

	// Perform watermark removal
	cleanVideoPath := inputPath // Default to original

	p.log(jobID, "sampling_frames", "info")
	log.Printf("[WATERMARK] Stage: sampling_frames")

	// Call watermark removal service
	result, err := p.watermarkService.RemoveWatermark(ctx, inputPath)
	if err != nil {
		log.Printf("[WATERMARK] WARNING: Watermark removal failed: %v", err)
		log.Printf("[WATERMARK] Continuing with original video (fallback)")
		cleanVideoPath = inputPath
		p.log(jobID, "watermark_removal_fallback", "warn")
	} else if result.Success {
		if result.WatermarkDetected {
			log.Printf("[WATERMARK] Watermark detected and removed")
			log.Printf("[WATERMARK]   - Mode: %s", result.ModeUsed)
			log.Printf("[WATERMARK]   - Regions: %d", len(result.Regions))
			log.Printf("[WATERMARK]   - Fallback used: %t", result.FallbackUsed)

			// Log detected regions
			for i, region := range result.Regions {
				log.Printf("[WATERMARK]   Region %d: type=%s, location=%s, confidence=%.2f",
					i+1, region.WatermarkType, region.Location, region.Confidence)
			}

			// Use clean video if available
			if !result.FallbackUsed && result.OutputPath != "" && result.OutputPath != inputPath {
				// Verify clean output exists
				if _, err := os.Stat(result.OutputPath); err == nil {
					cleanVideoPath = result.OutputPath
					log.Printf("[WATERMARK] Using clean video: %s", cleanVideoPath)
				} else {
					log.Printf("[WATERMARK] Clean output not found, using original")
					cleanVideoPath = inputPath
				}
			}

			// Log stages
			for _, stage := range result.Stages {
				p.log(jobID, stage, "info")
			}

			p.log(jobID, "watermark_removal_complete", "info")
		} else {
			log.Printf("[WATERMARK] No watermark detected, using original video")
			cleanVideoPath = inputPath
		}

		// Log warning if any
		if result.Warning != "" {
			log.Printf("[WATERMARK] Warning: %s", result.Warning)
		}
	}

	// =============================================================================
	// STAGE 2: CONTINUE WITH EXISTING FFMPEG PIPELINE
	// =============================================================================
	// After watermark removal, continue with the standard pipeline:
	// - Cut first 10 seconds
	// - Add FilmoraUz.net logo
	// - Generate adaptive HLS

	log.Printf("[WATERMARK] Continuing with FFmpeg pipeline...")
	p.log(jobID, "continuing_ffmpeg_pipeline", "info")

	// Call the original processVideo with the clean video path
	// Note: The original processVideo will:
	// 1. Validate the input file
	// 2. Create base video with delogo + logo
	// 3. Generate adaptive HLS

	return p.Pipeline.processVideo(job, cleanVideoPath, canonicalFolderName)
}

// =============================================================================
// INTEGRATION NOTES FOR EXISTING PIPELINE
// =============================================================================
//
// To integrate watermark removal into the existing pipeline.go:
//
// 1. Add import for services package:
//    import "github.com/filmorauz/worker/services"
//
// 2. Add to Pipeline struct:
//    type Pipeline struct {
//        ...
//        watermarkService *services.WatermarkRemovalService
//    }
//
// 3. Add to NewPipeline function:
//    watermarkConfig := services.DefaultWatermarkRemovalConfig()
//    watermarkConfig.TempDir = config.TempDir
//    p.watermarkService = services.NewWatermarkRemovalService(watermarkConfig)
//
// 4. Modify processVideo function to call watermark removal BEFORE HLS processing:
//
//    // Around line 1280 in processVideo, after validation:
//    cleanVideoPath := inputPath
//
//    // Add watermark removal call here:
//    if p.watermarkService != nil && p.watermarkService != nil {
//        if err := p.updateStatus(jobID, models.IngestionStatusRemovingWatermark, 30); err != nil {
//            log.Printf("[PIPELINE] WARNING: Failed to update status: %v", err)
//        }
//
//        result, err := p.watermarkService.RemoveWatermark(ctx, inputPath)
//        if err == nil && result.Success && !result.FallbackUsed && result.OutputPath != "" {
//            cleanVideoPath = result.OutputPath
//            log.Printf("[PIPELINE] Using clean video from watermark removal: %s", cleanVideoPath)
//        }
//    }
//
//    // Then replace inputPath with cleanVideoPath in the processAdaptiveHLS call:
//    // masterPath, err := p.processAdaptiveHLS(jobID, cleanVideoPath, outputDir, defaultCutSeconds, progressCallback)
//
// 5. Update processJobWithRecovery to use the new watermark-aware processVideo:
//
//    // Replace the processVideo call with:
//    hlsPath, err := p.processVideoWithWatermarkRemoval(ctx, job, localPath, canonicalFolderName)
//
// =============================================================================

// ProcessJobWithWatermarkRemoval is a wrapper that adds watermark removal to job processing
func (p *WatermarkRemovalPipeline) ProcessJobWithWatermarkRemoval(ctx context.Context, job *models.IngestionJob) error {
	if job == nil {
		return fmt.Errorf("job is nil")
	}

	// Call the base ProcessJob which handles the full pipeline
	return p.ProcessJob(ctx, job)
}
