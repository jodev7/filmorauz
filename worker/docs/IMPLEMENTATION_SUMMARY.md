# Watermark Removal Implementation Summary

## 1. Root Cause / Architecture Summary

### Why Watermark Removal Belongs in Worker, Not Parser

| Aspect | Parser | Worker |
|--------|--------|--------|
| **Purpose** | Source discovery and acquisition | Media processing and transformation |
| **Complexity** | Network I/O, URL resolution | CPU/GPU intensive processing |
| **Timing** | Fast, synchronous | Slow, can be parallelized |
| **Dependencies** | Source sites, network | AI models, FFmpeg |
| **Failure impact** | Job can't start | Job continues with fallback |

**Key Reasons:**
1. **Separation of Concerns**: Parser handles source acquisition, Worker handles transformation
2. **Resource Management**: AI processing requires significant resources; keeping it in Worker allows scaling independently
3. **Reliability**: If watermark removal fails, parser's work (downloaded video) is preserved
4. **Performance**: Watermark removal can be parallelized and optimized without affecting parsing

### Where It Is Inserted

```
Pipeline Flow:
─────────────────────────────────────────────────────────────────────────
Parser                           Worker
   │                                │
   ▼                                ▼
   ┌───────────────────────────────┴───────────────────────────────┐
   │                                                               │
   ▼                                                               │
   └──────────────► local_path ───────────────────────────────►   │
                                                                   │
   ▼                                                               │
   ┌───────────────────────────────────────────────────────────────┤
   │  WATERMARK REMOVAL (NEW)                                       │
   │  • Sample frames                                              │
   │  • Detect watermark regions                                   │
   │  • Generate masks                                             │
   │  • Inpaint (remove watermark)                                 │
   │  • Reconstruct clean video                                    │
   └───────────────────────────────────────────────────────────────┤
                                                                   │
   ▼                                                               │
   ┌───────────────────────────────────────────────────────────────┤
   │  Existing Worker Pipeline                                     │
   │  • Cut first 10 seconds                                       │
   │  • Add FilmoraUz.net logo                                     │
   │  • Generate adaptive HLS                                      │
   │  • Generate poster/backdrop                                   │
   │  • Storage finalization                                       │
   └───────────────────────────────────────────────────────────────┤
```

## 2. Files Changed

### New Files Created

```
worker/
├── watermark_removal/
│   ├── __init__.py          # Package exports
│   ├── config.py            # Configuration management
│   ├── types.py             # Type definitions
│   ├── sampler.py           # Frame sampling
│   ├── detector.py          # Watermark detection
│   ├── ocr.py               # OCR-based detection
│   ├── masks.py             # Mask generation
│   ├── inpaint.py           # Inpainting (FAST + PRO)
│   ├── pipeline.py          # Main pipeline orchestrator
│   ├── service.py           # HTTP service for Go integration
│   ├── requirements.txt     # Python dependencies
│   ├── README.md            # Documentation
│   └── TEST_CHECKLIST.md    # Testing guide
├── services/
│   └── watermark_removal.go # Go wrapper service
└── docs/
    ├── INTEGRATION.md       # Integration guide
    └── IMPLEMENTATION_SUMMARY.md # This file
```

### Files to Modify (Integration Required)

```
worker/
├── pipeline/
│   └── pipeline.go         # Add watermark service initialization and calls
└── go.mod                   # May need updates if new dependencies
```

## 3. Detection Approach

### Static Watermark Handling

```
┌─────────────────────────────────────────────────────────────┐
│ STATIC WATERMARK DETECTION                                  │
├─────────────────────────────────────────────────────────────┤
│ 1. Frame Sampling                                           │
│    • Sample N frames across video duration                  │
│    • Early, middle, and late portions                        │
│    • Skip intro/cut regions                                  │
│                                                              │
│ 2. Corner Priority Analysis                                 │
│    • Top-left, top-right, bottom-left, bottom-right        │
│    • Edge zones (10% from each edge)                        │
│    • Analyze pixel patterns in each zone                    │
│                                                              │
│ 3. Persistence Analysis                                      │
│    • Compare same regions across frames                      │
│    • Find stable patterns (present in most frames)          │
│    • Identify consistent opacity/colors                      │
│                                                              │
│ 4. Mask Generation                                           │
│    • Generate binary mask for detected region               │
│    • Add padding (configurable, default 8px)                │
│    • Support soft-edge expansion                             │
│                                                              │
│ 5. Reuse Strategy                                             │
│    • Detect ONCE per video                                   │
│    • Apply SAME mask to all frames                           │
│    • Process frames in parallel batches                       │
└─────────────────────────────────────────────────────────────┘
```

### Dynamic Watermark Handling

```
┌─────────────────────────────────────────────────────────────┐
│ DYNAMIC WATERMARK DETECTION (Best Effort)                   │
├─────────────────────────────────────────────────────────────┤
│ 1. Windowed Detection                                        │
│    • Divide video into time windows                          │
│    • Detect watermark per window                              │
│    • Update masks when position shifts                        │
│                                                              │
│ 2. Position Tracking                                          │
│    • Cluster detections by position                          │
│    • Track region centroid over time                          │
│    • Detect significant shifts (>20% change)                   │
│                                                              │
│ 3. Confidence-Weighted Processing                            │
│    • High confidence regions: inpaint immediately            │
│    • Low confidence regions: defer or skip                    │
│    • Per-region mask updates                                  │
│                                                              │
│ 4. Opacity Variation Handling                                 │
│    • Average frame difference for region                      │
│    • Apply mask regardless of opacity                         │
│    • May need multiple passes                                 │
└─────────────────────────────────────────────────────────────┘
```

### OCR/Text Watermark Handling

```
┌─────────────────────────────────────────────────────────────┐
│ OCR-BASED TEXT DETECTION                                     │
├─────────────────────────────────────────────────────────────┤
│ 1. Edge Region OCR                                           │
│    • Apply OCR to corner/edge regions                         │
│    • Detect source URLs (e.g., "fimov.biz", "cdn.example")   │
│    • Identify repeated branding text                          │
│                                                              │
│ 2. Text-to-Mask Conversion                                    │
│    • Convert detected text bounding box to mask              │
│    • Expand slightly for soft edges                           │
│    • Merge with other detection methods                      │
│                                                              │
│ 3. Confidence Scoring                                         │
│    • High confidence: URL patterns, known sites              │
│    • Medium confidence: readable text in edge region          │
│    • Low confidence: unclear/noisy text                       │
│                                                              │
│ 4. Integration                                                │
│    • OCR regions combined with CV detection                   │
│    • Weighted by detection confidence                          │
│    • Final regions sorted by confidence                       │
└─────────────────────────────────────────────────────────────┘
```

### Confidence Logic

```python
# Confidence Factors
confidence_factors = {
    "persistence": 0.3,      # Present in multiple frames
    "edge_position": 0.2,    # Located in corner/edge
    "cv_consistency": 0.2,   # CV analysis match
    "ocr_match": 0.2,        # OCR text detected
    "size_stability": 0.1,   # Consistent size across frames
}

# Final Confidence Score
final_confidence = sum(factor * weight for factor in detected_factors)

# Threshold Check
if final_confidence >= config.confidence_threshold:
    proceed_with_removal()
else:
    log("Confidence too low, skipping removal")
    continue_with_original_video()
```

## 4. Libraries Added

### Python Packages

| Package | Purpose | Usage |
|---------|---------|-------|
| `opencv-python` | CV-based detection, frame processing, OpenCV inpainting | Frame analysis, mask generation, Telea/NS inpainting |
| `numpy` | Array operations | Image processing, mask manipulation |
| `Pillow` | Image loading/saving | Frame I/O, format conversion |
| `ultralytics` | YOLOv8 detection | Optional: watermark/logo region proposals |
| `easyocr` | OCR text recognition | Optional: text overlay detection |
| `torch` | Deep learning | PRO mode LaMa inpainting |
| `lama` | LaMa inpainting | PRO mode neural inpainting |

### External Tools

| Tool | Purpose | Usage |
|------|---------|-------|
| `ffmpeg` | Frame extraction, video reconstruction | Extract frames, rebuild video, audio preserve |
| `ffprobe` | Video metadata | Duration, resolution, frame rate info |

### Where Used

```
┌─────────────────────────────────────────────────────────────┐
│ TOOL USAGE MAP                                               │
├─────────────────────────────────────────────────────────────┤
│ ffmpeg / ffprobe                                             │
│   • sampler.py: Extract frames at specific timestamps       │
│   • inpaint.py: Rebuild video from processed frames          │
│   • pipeline.py: Video format validation                    │
│                                                              │
│ opencv-python (cv2)                                          │
│   • sampler.py: Frame reading, color analysis                │
│   • detector.py: Corner detection, diff analysis            │
│   • masks.py: Mask generation, morphology                    │
│   • inpaint.py: Telea/NS inpainting                          │
│                                                              │
│ numpy                                                        │
│   • detector.py: Array operations for diff calculation       │
│   • masks.py: Mask array manipulation                        │
│   • inpaint.py: Frame array processing                       │
│                                                              │
│ Pillow                                                       │
│   • sampler.py: Frame conversion (cv2 → PIL)                 │
│   • pipeline.py: Output image generation                     │
│                                                              │
│ ultralytics (YOLOv8)                                         │
│   • detector.py: Watermark region proposals (optional)       │
│                                                              │
│ easyocr / paddleocr                                           │
│   • ocr.py: Text detection in edge regions                   │
│                                                              │
│ torch + lama                                                  │
│   • inpaint.py: PRO mode LaMa inpainting                     │
└─────────────────────────────────────────────────────────────┘
```

## 5. Inpainting Modes

### FAST Mode

```
┌─────────────────────────────────────────────────────────────┐
│ FAST MODE: OpenCV Inpainting                                │
├─────────────────────────────────────────────────────────────┤
│ Algorithm: Telea or Navier-Stokes                            │
│                                                              │
│ Pros:                                                        │
│   • Fast (real-time capable)                                 │
│   • No GPU required                                          │
│   • Good for simple watermarks                                │
│   • Easy to integrate                                         │
│                                                              │
│ Cons:                                                        │
│   • Lower quality for complex textures                        │
│   • May leave visible artifacts                              │
│   • Not suitable for large watermark areas                   │
│                                                              │
│ When to use:                                                 │
│   • Production servers without GPU                           │
│   • Quick processing required                                 │
│   • Small, corner watermarks                                  │
│   • Resource-constrained environments                        │
│                                                              │
│ Configuration:                                               │
│   WATERMARK_MODE=fast                                        │
│   OPENCV_INPAINT_RADIUS=5  (optional, default 3)             │
│   OPENCV_INPAINT_METHOD=telea  (or "ns")                     │
└─────────────────────────────────────────────────────────────┘
```

### PRO Mode

```
┌─────────────────────────────────────────────────────────────┐
│ PRO MODE: LaMa Inpainting                                    │
├─────────────────────────────────────────────────────────────┤
│ Algorithm: Large Mask Model (LaMa)                           │
│                                                              │
│ Pros:                                                        │
│   • High quality restoration                                 │
│   • Handles complex textures                                 │
│   • Better edge blending                                     │
│   • Works with larger watermark areas                        │
│                                                              │
│ Cons:                                                        │
│   • Requires GPU                                             │
│   • Slower processing                                         │
│   • Higher resource usage                                     │
│   • Model download/installation needed                       │
│                                                              │
│ When to use:                                                 │
│   • GPU-equipped servers                                      │
│   • High-quality output required                              │
│   • Complex watermark patterns                                │
│   • Premium content processing                                 │
│                                                              │
│ Configuration:                                               │
│   WATERMARK_MODE=pro                                         │
│   LAMA_MODEL_PATH=/path/to/model  (optional, auto-download) │
│   PRO_FALLBACK_TO_FAST=true  (recommended)                  │
│                                                              │
│ Model:                                                       │
│   • Download: https://github.com/saic-mdal/lama              │
│   • Auto-downloads on first use if not present               │
└─────────────────────────────────────────────────────────────┘
```

### Fallback Logic

```
┌─────────────────────────────────────────────────────────────┐
│ FALLBACK LOGIC FLOW                                           │
├─────────────────────────────────────────────────────────────┤
│                                                               │
│  Start                                                        │
│    │                                                          │
│    ▼                                                          │
│  ┌──────────────────────┐                                    │
│  │ Watermark Detected?  │──No──► Use original video          │
│  └──────────┬───────────┘                                    │
│             │ Yes                                            │
│             ▼                                                │
│  ┌──────────────────────┐                                    │
│  │ Mode = PRO?          │──No──► Run FAST inpainting         │
│  └──────────┬───────────┘                                    │
│             │ Yes                                            │
│             ▼                                                │
│  ┌──────────────────────┐                                    │
│  │ Run PRO inpainting   │                                    │
│  └──────────┬───────────┘                                    │
│             │                                                │
│    ┌────────┴────────┐                                       │
│    │ Success?        │──No──► [PRO_FALLBACK_TO_FAST?]       │
│    └────────┬────────┘         │                            │
│             │ Yes              │ No                          │
│             ▼                  ▼                              │
│  ┌──────────────────┐  ┌──────────────────┐                  │
│  │ Validate output  │  │ Use original    │                  │
│  └────────┬────────┘  │ video           │                  │
│           │            └──────────────────┘                  │
│    ┌──────┴──────┐                                          │
│    │ Valid?      │──No──► Use original video                │
│    └─────────────┘                                          │
│           │                                                  │
│           ▼                                                  │
│  ┌──────────────────┐                                        │
│  │ Return clean     │                                        │
│  │ video path       │                                        │
│  └──────────────────┘                                        │
│                                                               │
└─────────────────────────────────────────────────────────────┘
```

## 6. Performance Strategy

### Frame Sampling

```
┌─────────────────────────────────────────────────────────────┐
│ SAMPLING STRATEGY                                            │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Video Duration: T seconds                                     │
│ Sample Count: N (configurable, default 10)                   │
│                                                              │
│ Distribution:                                                 │
│   • Early: 20% of samples (skip first 10 seconds if cut)     │
│   • Middle: 50% of samples                                    │
│   • Late: 30% of samples                                      │
│                                                              │
│ Example (N=10):                                              │
│   T=600s → timestamps: [30, 60, 120, 180, 240, 300, 360, 420, 480, 540]
│                                                              │
│ Skip frames:                                                  │
│   • First 10 seconds (intro/credits)                         │
│   • Black frames (detect via histogram)                     │
│   • Transition frames (detect via motion)                   │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Mask Reuse

```
┌─────────────────────────────────────────────────────────────┐
│ STATIC WATERMARK MASK REUSE                                  │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Detection Phase (once):                                      │
│   1. Sample frames [f1, f2, ..., fN]                         │
│   2. Analyze each frame for watermark                       │
│   3. Find consistent region across frames                   │
│   4. Generate single mask M                                 │
│   5. Store: {region, mask, confidence}                       │
│                                                              │
│ Processing Phase:                                            │
│   ┌────────────────────────────────────────┐                │
│   │ Batch 1: frames [0-100]                │                │
│   │   • Apply mask M to each frame         │                │
│   │   • Inpaint in parallel                 │                │
│   │   • Collect processed frames           │                │
│   └────────────────────────────────────────┘                │
│   ┌────────────────────────────────────────┐                │
│   │ Batch 2: frames [100-200]               │                │
│   │   • Apply same mask M                  │                │
│   │   • Process in parallel                 │                │
│   └────────────────────────────────────────┘                │
│   ...                                                        │
│                                                              │
│ Benefit: 1 detection + N×batch processing                   │
│          vs. N×(detection + processing)                      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

### Multiprocessing

```python
# Parallel frame processing
from concurrent.futures import ProcessPoolExecutor
from multiprocessing import cpu_count

def process_frames_batch(frames, mask, mode):
    """Process a batch of frames in parallel."""
    results = []
    for frame in frames:
        processed = inpaint_frame(frame, mask, mode)
        results.append(processed)
    return results

# Determine worker count
MAX_WORKERS = min(cpu_count(), 8)  # Cap at 8 to avoid overhead

# Process in parallel
with ProcessPoolExecutor(max_workers=MAX_WORKERS) as executor:
    batches = chunk_list(frames, batch_size=50)
    results = list(executor.map(process_batch, batches))
```

### Windowed Updates (Dynamic Watermark)

```
┌─────────────────────────────────────────────────────────────┐
│ DYNAMIC WATERMARK WINDOWED PROCESSING                       │
├─────────────────────────────────────────────────────────────┤
│                                                              │
│ Video divided into W time windows                           │
│ Each window: detect once, apply to all frames in window     │
│                                                              │
│ Window 1 [0-120s]     Window 2 [120-240s]   Window 3 [240s+]
│   • Detect once         • Detect once         • Detect once
│   • Mask M1             • Mask M2             • Mask M3
│   • Apply to frames     • Apply to frames     • Apply to frames
│                                                              │
│ Detection frequency:                                         │
│   • Per window (default)                                    │
│   • If position changes > threshold: update mask            │
│   • Threshold: 20% region displacement                      │
│                                                              │
└─────────────────────────────────────────────────────────────┘
```

## 7. Integration Flow

### Parser Output

```go
// Parser returns this structure
type ParserResult struct {
    LocalPath   string      // Path to downloaded video
    Metadata    MovieMetadata
    SourceInfo  SourceInfo
    Error       error
}
```

### Worker Watermark Step

```go
// In worker/pipeline/pipeline.go

// Receive local_path from parser
inputPath := parserResult.LocalPath

// Run watermark removal
wmResult, err := p.watermarkService.RemoveWatermark(ctx, inputPath)
if err != nil {
    log.Printf("[WATERMARK] Error: %v", err)
    cleanPath := inputPath  // Fallback
} else if wmResult.Success {
    if wmResult.OutputPath != "" && !wmResult.FallbackUsed {
        cleanPath := wmResult.OutputPath
    } else {
        cleanPath := inputPath  // No watermark or fallback
    }
}

// Pass clean path to next pipeline stage
processFirstTenSeconds(cleanPath)
processLogoOverlay(cleanPath)
processAdaptiveHLS(cleanPath)
```

### Downstream FFmpeg/HLS Step

```go
// No changes needed - watermark removal outputs same format
// Input: MP4 video
// Output: Clean MP4 video (same format)

// HLS generation continues as before
hlsPath, err := p.processAdaptiveHLS(cleanPath, outputPath, posterPath)
```

## 8. Manual Test Checklist

See `worker/watermark_removal/TEST_CHECKLIST.md` for detailed test procedures.

### Quick Test Categories

| Category | Tests |
|----------|-------|
| Static Watermarks | Top-right, bottom-right, corner text, multiple corners |
| Dynamic Watermarks | Shifting position, opacity changes, segment appearance |
| Pipeline Integration | Parser→Worker→HLS flow, status updates, error handling |
| Fallbacks | No watermark, PRO failure, FAST fallback, invalid output |
| Performance | Mask reuse, parallel processing, timing benchmarks |

---

## Configuration Quick Reference

```bash
# Environment Variables
WATERMARK_ENABLED=true
WATERMARK_MODE=fast
WATERMARK_SAMPLE_COUNT=10
WATERMARK_MASK_PADDING=8
WATERMARK_CONFIDENCE_THRESHOLD=0.65
WATERMARK_MAX_REGIONS=3
WATERMARK_TEMP_DIR=/tmp/filmora_watermark
WATERMARK_OCR_ENABLED=true
WATERMARK_PRO_FALLBACK_TO_FAST=true
WATERMARK_SERVICE_URL=
WATERMARK_PYTHON_PATH=python3
```

## Support

For issues or questions:
1. Check logs for error messages
2. Verify dependencies are installed
3. Test with sample video containing known watermark
4. Enable debug logging for detailed analysis
