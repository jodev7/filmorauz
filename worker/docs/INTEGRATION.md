# Watermark Removal Integration Guide

## Overview

This document describes how to integrate the watermark removal pipeline into the FilmoraUz worker service.

## Architecture

### Parser vs Worker Separation

```
Parser Service                    Worker Service
─────────────────                  ────────────────
• Source search                   • Watermark removal (NEW)
• Source parsing                  • Cut first 10 seconds
• Video URL resolution            • FilmoraUz.net logo overlay
• Source download                 • Adaptive bitrate HLS generation
• Returns local_path              • Poster/backdrop generation
                                 • Storage finalization
```

**Key Principle**: All heavy AI-based media processing happens in the WORKER, not the parser.

## Integration Steps

### 1. Add Import to Pipeline

In `worker/pipeline/pipeline.go`, add the import:

```go
import (
    // ... existing imports
    "github.com/filmorauz/worker/services"
)
```

### 2. Add Service to Pipeline Struct

Add the watermark service to the Pipeline struct:

```go
type Pipeline struct {
    // ... existing fields
    watermarkService *services.WatermarkRemovalService
}
```

### 3. Initialize in NewPipeline

In `NewPipeline()`, initialize the service:

```go
func NewPipeline(...) *Pipeline {
    // ... existing initialization
    
    watermarkConfig := services.DefaultWatermarkRemovalConfig()
    watermarkConfig.TempDir = config.TempDir
    p.watermarkService = services.NewWatermarkRemovalService(watermarkConfig)
    
    return p
}
```

### 4. Integrate in processVideo

In `processVideo()`, add watermark removal after video validation and before HLS generation:

```go
// Around line 1280 in processVideo()

// Clean video path starts as input path
cleanVideoPath := inputPath

// Step 1: Watermark Removal (NEW)
log.Printf("[PIPELINE] Starting watermark removal...")
status := p.updateJobStatus(jobID, "watermark_removal", "Watermark removal in progress...")
if status != nil {
    log.Printf("[PIPELINE] Job status updated: %s", status.Status)
}

// Perform watermark removal
if p.watermarkService != nil {
    wmResult, err := p.watermarkService.RemoveWatermark(ctx, inputPath)
    if err != nil {
        log.Printf("[WATERMARK] Error: %v", err)
        // Continue with original video (fallback)
        log.Printf("[WATERMARK] Falling back to original video")
    } else if wmResult.Success {
        log.Printf("[WATERMARK] Result: detected=%v, removed=%v, fallback=%v",
            wmResult.WatermarkDetected, wmResult.WatermarkRemoved, wmResult.FallbackUsed)
        
        // Use cleaned video if available
        if wmResult.OutputPath != "" && !wmResult.FallbackUsed {
            cleanVideoPath = wmResult.OutputPath
            log.Printf("[WATERMARK] Using cleaned video: %s", cleanVideoPath)
        }
        
        // Update job with watermark info
        if wmResult.WatermarkDetected {
            regionsJSON, _ := json.Marshal(wmResult.Regions)
            log.Printf("[WATERMARK] Detected %d watermark regions: %s", 
                len(wmResult.Regions), string(regionsJSON))
        }
        
        // Log stages
        for _, stage := range wmResult.Stages {
            log.Printf("[WATERMARK] Stage: %s", stage)
        }
        
        // Handle warning
        if wmResult.Warning != "" {
            log.Printf("[WATERMARK] Warning: %s", wmResult.Warning)
        }
    }
}

// Step 2: Cut first 10 seconds
log.Printf("[PIPELINE] Cutting first 10 seconds...")
tenSecPath := filepath.Join(tempDir, "cut_10sec.mp4")
if err := p.cutFirstTenSeconds(cleanVideoPath, tenSecPath); err != nil {
    // ...
}

// Step 3: Add logo overlay
log.Printf("[PIPELINE] Adding FilmoraUz.net logo...")

// Step 4: Generate adaptive HLS (use cleanVideoPath or cut video)
log.Printf("[PIPELINE] Generating adaptive HLS...")
hlsPath := filepath.Join(outputDir, "index.m3u8")
if err := p.processAdaptiveHLS(cleanVideoPath, hlsPath, posterPath); err != nil {
    // ...
}
```

### 5. Environment Configuration

Add these environment variables to your worker configuration:

```bash
# Watermark Removal Configuration
WATERMARK_ENABLED=true
WATERMARK_MODE=fast  # or "pro" for LaMa-based inpainting
WATERMARK_SAMPLE_COUNT=10
WATERMARK_MASK_PADDING=8
WATERMARK_CONFIDENCE_THRESHOLD=0.65
WATERMARK_MAX_REGIONS=3
WATERMARK_TEMP_DIR=/tmp/filmora_watermark
WATERMARK_OCR_ENABLED=true
WATERMARK_PRO_FALLBACK_TO_FAST=true

# Optional: Use HTTP service instead of direct Python
WATERMARK_SERVICE_URL=http://localhost:8084
```

### 6. Dependencies

Ensure Python dependencies are installed:

```bash
pip install -r worker/watermark_removal/requirements.txt
```

For PRO mode (LaMa inpainting):
```bash
pip install torch torchvision
git clone https://github.com/saic-mdal/lama.git
cd lama && pip install -e .
```

## Service Modes

### Mode 1: Direct Python Execution (Default)

The Go worker calls the Python script directly:

```go
// Python script handles everything
cmd := exec.CommandContext(ctx, "python3", scriptPath, inputPath, outputPath)
```

### Mode 2: HTTP Service

Run the Python service separately:

```bash
python -m watermark_removal.service --port 8084
```

Then configure the Go worker to use it:

```bash
WATERMARK_SERVICE_URL=http://localhost:8084
```

## Result Handling

The `WatermarkRemovalResult` contains:

```go
type WatermarkRemovalResult struct {
    Success           bool              // Overall success
    InputPath         string            // Original video path
    OutputPath        string            // Clean video path (or original if fallback)
    WatermarkDetected bool              // Whether watermark was found
    WatermarkRemoved  bool              // Whether removal succeeded
    ModeUsed          string            // "fast" or "pro"
    Regions           []WatermarkRegion // Detected regions
    FallbackUsed      bool              // Whether fallback to original was used
    Warning           string            // Warning message if any
    Stages            []string          // Processing stages for logging
    TotalTime         float64           // Processing time in seconds
    Error             string            // Error message if failed
}
```

## Fallback Behavior

| Scenario | Behavior |
|----------|----------|
| No watermark detected | Use original video, continue pipeline |
| Detection fails | Log error, use original video, continue |
| Inpainting fails | Log error, use original video, continue |
| PRO mode fails | Fallback to FAST mode if enabled |
| Output invalid | Log error, use original video, continue |
| Pipeline continues | Yes, always (unless hard failure configured) |

## Logging

Watermark removal logs the following stages:

- `received_local_video`
- `sampling_frames`
- `detecting_watermark`
- `generating_masks`
- `inpainting_frames`
- `rebuilding_clean_video`
- `watermark_removal_complete`
- `watermark_removal_fallback`
- `continuing_ffmpeg_pipeline`

## Testing

See `worker/watermark_removal/TEST_CHECKLIST.md` for comprehensive testing instructions.

## Performance Notes

1. **Static Watermarks**: Detection runs once, mask is reused
2. **Dynamic Watermarks**: Detection runs per time window
3. **Parallel Processing**: Uses multiprocessing for frame batches
4. **Mask Reuse**: For static watermarks, same mask applied to all frames
5. **Region Cropping**: Optional - crop only watermark region, inpaint, composite back

## Troubleshooting

### Python script not found

```bash
# Check Python path
which python3
python3 --version

# Verify script location
ls -la worker/watermark_removal/*.py
```

### Import errors

```bash
pip install opencv-python numpy pillow
pip install ultralytics  # for YOLO detection
pip install easyocr paddleocr  # for OCR
```

### LaMa not working

```bash
# Check PyTorch
python3 -c "import torch; print(torch.cuda.is_available())"

# Install LaMa properly
cd lama && pip install -e .
```

### Service mode connection failed

```bash
# Start service manually
python -m watermark_removal.service --port 8084

# Test with curl
curl -X POST http://localhost:8084/health
```
