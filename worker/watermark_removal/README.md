# AI-Based Watermark Removal Pipeline

## Overview

This document describes the AI-based watermark removal pipeline implemented in the FilmoraUz worker service. The pipeline is designed to detect and remove watermarks from video content before the standard Filmora processing pipeline continues.

## Architecture Summary

### Why Watermark Removal Belongs in Worker, Not Parser

1. **Separation of Concerns**: The parser is responsible for source search, parsing, URL resolution, and video download. Adding AI-based media processing would complicate the parser and slow down its response time.

2. **Performance**: Watermark removal is computationally expensive. By keeping it in the worker, we can:
   - Run watermark removal asynchronously
   - Use dedicated resources for AI processing
   - Scale workers independently from parsers

3. **Reliability**: The parser must return a valid `local_path` quickly. If watermark removal fails, the parser shouldn't be affected.

4. **Modularity**: Watermark removal can be updated/upgraded without touching the parser code.

## Where It Is Inserted

The watermark removal step is inserted in the worker pipeline as follows:

```
Parser downloads video → Worker receives local_path
                                 ↓
                    Watermark Removal Pipeline
                                 ↓
                    Clean video OR original (fallback)
                                 ↓
                    Cut first 10 seconds
                                 ↓
                    Add FilmoraUz.net logo
                                 ↓
                    Generate adaptive HLS
                                 ↓
                    Storage finalization
                                 ↓
                    DB update/save
```

## Files Changed

### New Python Files (worker/watermark_removal/)

| File | Purpose |
|------|---------|
| `__init__.py` | Package initialization |
| `config.py` | Configuration management |
| `detector.py` | Watermark detection module |
| `mask_generator.py` | Mask generation for detected watermarks |
| `inpainter.py` | Inpainting (FAST/PRO modes) |
| `video_processor.py` | Video reconstruction |
| `pipeline.py` | Main pipeline orchestrator |
| `service.py` | HTTP service for Go integration |
| `TEST_CHECKLIST.md` | Manual test checklist |

### New Go Files (worker/)

| File | Purpose |
|------|---------|
| `services/watermark_removal.go` | Go wrapper for Python service |
| `pipeline/watermark_integration.go` | Pipeline integration code |

## Detection Approach

### Static Watermark Handling

1. **Multi-frame Analysis**: Sample multiple frames from different parts of the video
2. **Corner Priority**: Prioritize corner/edge regions where watermarks commonly appear
3. **Edge Detection**: Use Canny/Sobel edge detection to identify watermark boundaries
4. **Pattern Recognition**: Identify text-like and logo-like patterns using morphological analysis
5. **Consistency Check**: Verify detection across multiple frames for static watermarks

### Dynamic Watermark Handling (Best-Effort)

1. **Frame Differencing**: Compare frames to identify persistent regions
2. **Tracking**: Track watermark position across frames when it shifts slightly
3. **Windowed Updates**: Update masks periodically rather than per-frame for efficiency

### OCR/Text Watermark Handling

1. **Edge Region Scanning**: Scan corner/edge regions with OCR
2. **Pattern Matching**: Identify common watermark text (domain names, site URLs)
3. **Bounding Box Extraction**: Convert detected text to mask regions

### Confidence Logic

Each detection includes:
- **Confidence Score** (0.0-1.0)
- **Detection Method** (rule_based, persistence_analysis, ocr, combined)
- **Watermark Type** (corner_logo, corner_text, persistent_overlay, etc.)

Processing proceeds only if confidence >= threshold (default: 0.65).

## Libraries Used

### Core Video/Frame Processing
- **ffmpeg/ffprobe**: Video decoding, frame extraction, encoding
- **opencv-python (cv2)**: Image processing, inpainting, edge detection

### Python Image/CV Processing
- **numpy**: Array operations, numerical computations

### Optional ML/AI Detection
- **ultralytics (YOLOv8)**: For watermark/logo region detection (optional, not yet integrated)
- **easyocr**: For overlay text detection (optional)

### AI Inpainting
- **LaMa**: Large Mask Inpainting neural network (for PRO mode)
  - Requires PyTorch
  - GPU recommended for best performance

### Parallel Processing
- **concurrent.futures**: For parallel frame processing

## Inpainting Modes

### FAST Mode (Default)

Uses OpenCV inpainting algorithms:
- **Telea**: Fast, good for small watermarks
- **Navier-Stokes**: Smoother results, better for larger areas

**Characteristics:**
- ~1-2 seconds per frame
- Good quality for corner logos and text
- No GPU required
- Fallback-safe

### PRO Mode

Uses LaMa neural network:
- Higher quality restoration
- Better for complex watermark patterns
- Requires GPU for best performance

**Characteristics:**
- ~5-10 seconds per frame (GPU)
- Superior quality for challenging cases
- Requires PyTorch + CUDA
- Falls back to FAST mode if unavailable

## Performance Strategy

### Frame Sampling

1. **Adaptive Sampling**: Sample 5-20 frames depending on video length
2. **Smart Positioning**: Skip first 10 seconds (likely intro), distribute evenly
3. **Persistence Analysis**: Use fewer frames for static watermarks

### Mask Reuse

1. **Static Watermarks**: Detect once, reuse mask for all frames
2. **Dynamic Watermarks**: Update mask periodically (every N frames)
3. **Efficient Storage**: Cache masks in memory, not on disk

### Parallel Processing

1. **Batch Processing**: Process frames in batches of 10-50
2. **Worker Pool**: Use multiprocessing pool with configurable workers
3. **Memory Management**: Limit memory by processing batches

### Windowed Updates

For dynamic watermarks:
- Detect every N frames (e.g., every 100 frames)
- Interpolate mask positions between detections
- Balance accuracy vs. performance

## Integration Flow

### Parser Output

```json
{
  "local_path": "/path/to/downloaded/video.mp4",
  "metadata": {...}
}
```

### Worker Watermark Step

```python
# Python watermark removal
result = pipeline.process(input_path, output_path)
# Returns PipelineResult with:
# - success: bool
# - output_path: path to clean video
# - watermark_detected: bool
# - regions: list of detected watermarks
```

### Downstream FFmpeg/HLS Step

The clean video path (or original if no watermark/fallback) is passed to the existing FFmpeg pipeline:
1. Cut first 10 seconds
2. Apply delogo filter (additional safety)
3. Add FilmoraUz.net logo overlay
4. Generate adaptive HLS

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `WATERMARK_ENABLED` | `true` | Enable/disable watermark removal |
| `WATERMARK_MODE` | `fast` | Processing mode: `fast` or `pro` |
| `WATERMARK_SAMPLE_COUNT` | `10` | Number of frames to sample |
| `WATERMARK_MASK_PADDING` | `8` | Mask expansion padding (pixels) |
| `WATERMARK_CONFIDENCE_THRESHOLD` | `0.65` | Minimum confidence to proceed |
| `WATERMARK_MAX_REGIONS` | `3` | Maximum watermark regions |
| `WATERMARK_TEMP_DIR` | `./tmp` | Temporary directory |
| `WATERMARK_OCR_ENABLED` | `true` | Enable OCR text detection |
| `WATERMARK_YOLO_MODEL` | - | Path to YOLO model (optional) |
| `WATERMARK_LAMA_MODEL` | - | Path to LaMa model (optional) |
| `WATERMARK_SERVICE_URL` | - | HTTP service URL (optional) |
| `WATERMARK_PYTHON_PATH` | `python3` | Python interpreter path |

### HTTP Service (Alternative)

Instead of calling Python directly, the Go worker can call an HTTP service:

```bash
# Start service
python -m watermark_removal.service --port 8084

# API calls
POST /remove - Remove watermark from video
GET /detect?path=... - Detect watermark only
GET /health - Health check
```

## Output Contract

```json
{
  "success": true,
  "input_path": "/path/to/input.mp4",
  "output_path": "/path/to/output_clean.mp4",
  "watermark_detected": true,
  "mode_used": "fast",
  "regions": [
    {
      "x": 100,
      "y": 10,
      "width": 150,
      "height": 50,
      "confidence": 0.85,
      "watermark_type": "corner_text",
      "location": "top-right"
    }
  ],
  "fallback_used": false,
  "warning": null,
  "stages": [
    "received_local_video",
    "sampling_frames",
    "detecting_watermark",
    "generating_masks",
    "inpainting_frames",
    "rebuilding_clean_video",
    "watermark_removal_complete"
  ],
  "total_time": 45.2,
  "error": ""
}
```

## Logging & Job Status

### Pipeline Stages Logged

1. `received_local_video` - Worker received video for processing
2. `sampling_frames` - Extracting frames for analysis
3. `detecting_watermark` - Running watermark detection
4. `generating_masks` - Creating masks for detected regions
5. `inpainting_frames` - Applying inpainting to remove watermarks
6. `rebuilding_clean_video` - Reconstructing clean video
7. `watermark_removal_complete` - Watermark removal finished
8. `watermark_removal_fallback` - Fallback was used
9. `continuing_ffmpeg_pipeline` - Proceeding to standard pipeline

### Job Status Updates

The worker updates the job status in MongoDB:
- `IngestionStatusRemovingWatermark` - During watermark removal
- Progress percentage updates (0-100%)

## Error Handling & Fallback

1. **No Watermark Detected**: Continue with original video
2. **Detection Failure**: Continue with original, log warning
3. **Inpainting Failure**: Fallback to original video
4. **PRO Mode Unavailable**: Auto-fallback to FAST mode
5. **Invalid Output**: Detect corruption, fallback to original
6. **Pipeline Error**: Graceful degradation, never corrupt final output

## Manual Test Checklist

See [TEST_CHECKLIST.md](TEST_CHECKLIST.md) for comprehensive testing instructions covering:

- Static watermarks (top-right, bottom-right, corner text, multiple)
- Dynamic watermarks (shifting, varying opacity, segmented)
- Pipeline integration tests
- Fallback scenarios
- Performance benchmarks
- Configuration variations

## Installation

### Prerequisites

```bash
# Python 3.8+
python3 --version

# FFmpeg
ffmpeg -version

# Optional: PyTorch for PRO mode
pip install torch torchvision
```

### Install Dependencies

```bash
cd worker
pip install opencv-python numpy

# Optional: OCR
pip install easyocr

# Optional: LaMa (see LaMa documentation)
```

### Run HTTP Service

```bash
cd worker
python -m watermark_removal.service --port 8084
```

### Or Call Directly from Go

```go
// In worker main.go
config := services.DefaultWatermarkRemovalConfig()
service := services.NewWatermarkRemovalService(config)

result, err := service.RemoveWatermark(ctx, inputPath)
```

## Future Enhancements

1. **YOLO Integration**: Use YOLOv8 for more accurate watermark detection
2. **Video inpainting**: Process video directly instead of frame-by-frame
3. **GPU Acceleration**: Optimize with CUDA for faster processing
4. **Watermark Learning**: Learn from user feedback to improve detection
5. **Batch Processing**: Process multiple videos in parallel
