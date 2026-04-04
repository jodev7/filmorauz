# Watermark Removal Test Checklist

## Manual Test Checklist

This document provides a comprehensive test checklist for the AI-based watermark removal pipeline.

---

## 1. STATIC WATERMARKS

### 1.1 Top-Right Static Logo
- [ ] Create test video with logo in top-right corner
- [ ] Run watermark removal
- [ ] Verify watermark is removed
- [ ] Verify video quality is acceptable
- [ ] Check logs show correct detection

### 1.2 Bottom-Right Semi-Transparent Site URL
- [ ] Create test video with semi-transparent text watermark
- [ ] Run watermark removal with FAST mode
- [ ] Verify watermark is removed
- [ ] Verify text artifacts are minimized
- [ ] Compare with PRO mode quality

### 1.3 Corner Text Watermark
- [ ] Create test video with text watermark (e.g., "uzmovi.com")
- [ ] Run detection only (no removal)
- [ ] Verify text is correctly detected
- [ ] Run full removal
- [ ] Verify text is removed

### 1.4 Multiple Corner Overlays
- [ ] Create test video with 2+ watermarks
- [ ] Verify all watermarks are detected
- [ ] Verify all are removed
- [ ] Verify no content outside watermarks is affected

---

## 2. DYNAMIC WATERMARKS

### 2.1 Watermark Shifts Slightly
- [ ] Create test video with watermark that moves slightly
- [ ] Run watermark removal
- [ ] Verify removal works for shifted positions
- [ ] Check that mask updates appropriately

### 2.2 Watermark Opacity Changes
- [ ] Create test video with varying opacity watermark
- [ ] Run watermark removal
- [ ] Verify removal works for different opacities
- [ ] Check no artifacts remain

### 2.3 Watermark Appears in Segments
- [ ] Create test video where watermark appears/disappears
- [ ] Run watermark removal
- [ ] Verify removal works for appearing segments
- [ ] Verify original content preserved when watermark absent

---

## 3. PIPELINE INTEGRATION

### 3.1 Parser Downloads Video Successfully
- [ ] Start parser service
- [ ] Download a test video
- [ ] Verify local_path is returned
- [ ] Verify video file is valid

### 3.2 Worker Receives local_path
- [ ] Start worker service
- [ ] Trigger job processing
- [ ] Verify worker receives local_path from backend
- [ ] Check worker logs show correct path

### 3.3 Watermark Removal Step Runs
- [ ] Trigger full pipeline
- [ ] Verify watermark removal stage executes
- [ ] Check stage logs: received_local_video, sampling_frames, detecting_watermark, etc.
- [ ] Verify progress is reported

### 3.4 Cleaned Video is Valid
- [ ] Run watermark removal on test video
- [ ] Verify output file exists
- [ ] Verify output is playable
- [ ] Verify video plays with correct duration
- [ ] Verify audio is preserved

### 3.5 Downstream Processing Works
- [ ] Run full pipeline including watermark removal
- [ ] Verify cut first 10 seconds still works
- [ ] Verify logo overlay still works
- [ ] Verify HLS generation works
- [ ] Verify final movie plays correctly

### 3.6 End-to-End Flow
- [ ] Start all services (parser, backend, worker)
- [ ] Create ingestion job via API
- [ ] Process movie through full pipeline
- [ ] Verify final HLS is accessible
- [ ] Verify movie plays on frontend

---

## 4. FALLBACKS

### 4.1 No Watermark Detected
- [ ] Use clean video without watermark
- [ ] Run pipeline
- [ ] Verify "no watermark detected" log
- [ ] Verify original video used
- [ ] Verify downstream processing continues

### 4.2 PRO Mode Failure
- [ ] Configure WATERMARK_MODE=pro
- [ ] Ensure LaMa model is not available
- [ ] Run pipeline
- [ ] Verify fallback to FAST mode
- [ ] Verify processing completes

### 4.3 FAST Mode Fallback
- [ ] Disable OpenCV inpainting (simulate failure)
- [ ] Run pipeline
- [ ] Verify graceful fallback
- [ ] Verify original video used

### 4.4 Cleaned Output Invalid
- [ ] Corrupt the cleaned output (simulate)
- [ ] Run pipeline
- [ ] Verify detection of invalid output
- [ ] Verify fallback to original video
- [ ] Verify warning is logged

### 4.5 Pipeline Continues Safely
- [ ] Simulate watermark removal failure
- [ ] Run full pipeline
- [ ] Verify job completes successfully
- [ ] Verify original video used for downstream
- [ ] Verify final output is valid

---

## 5. PERFORMANCE

### 5.1 Static Watermark Reuses Masks
- [ ] Run on video with static watermark
- [ ] Check logs show mask reuse
- [ ] Verify processing time is reasonable

### 5.2 Frame Processing in Parallel
- [ ] Run on multi-core system
- [ ] Monitor CPU usage
- [ ] Verify parallel processing

### 5.3 Processing Time
- [ ] Measure time for typical video
- [ ] Compare FAST vs PRO mode
- [ ] Verify time is acceptable

### 5.4 Memory Usage
- [ ] Monitor memory during processing
- [ ] Verify no memory leaks
- [ ] Check peak memory is acceptable

---

## 6. CONFIGURATION

### 6.1 WATERMARK_ENABLED=false
- [ ] Set environment variable
- [ ] Run pipeline
- [ ] Verify watermark removal is skipped
- [ ] Verify pipeline completes quickly

### 6.2 WATERMARK_MODE=fast
- [ ] Set environment variable
- [ ] Run pipeline
- [ ] Verify OpenCV inpainting is used
- [ ] Check processing speed

### 6.3 WATERMARK_MODE=pro
- [ ] Set environment variable
- [ ] Configure LaMa model path
- [ ] Run pipeline
- [ ] Verify LaMa inpainting is used
- [ ] Check quality improvement

### 6.4 WATERMARK_SAMPLE_COUNT
- [ ] Test with 5 samples
- [ ] Test with 10 samples
- [ ] Test with 20 samples
- [ ] Compare detection accuracy

### 6.5 WATERMARK_CONFIDENCE_THRESHOLD
- [ ] Test with threshold 0.5
- [ ] Test with threshold 0.65
- [ ] Test with threshold 0.8
- [ ] Verify threshold affects detection

### 6.6 WATERMARK_MAX_REGIONS
- [ ] Test with 1 region
- [ ] Test with 3 regions
- [ ] Test with 5 regions
- [ ] Verify limit is respected

---

## 7. ERROR HANDLING

### 7.1 Missing Input File
- [ ] Provide non-existent path
- [ ] Run pipeline
- [ ] Verify proper error message
- [ ] Verify no crash

### 7.2 Corrupted Video File
- [ ] Provide corrupted video
- [ ] Run pipeline
- [ ] Verify graceful failure
- [ ] Verify fallback behavior

### 7.3 Insufficient Disk Space
- [ ] Limit disk space
- [ ] Run pipeline
- [ ] Verify proper error
- [ ] Verify cleanup

### 7.4 Python Not Available
- [ ] Ensure Python not installed
- [ ] Run pipeline
- [ ] Verify fallback to Go-only mode
- [ ] Verify pipeline continues

---

## 8. LOGGING & MONITORING

### 8.1 Stage Logging
- [ ] Verify received_local_video is logged
- [ ] Verify sampling_frames is logged
- [ ] Verify detecting_watermark is logged
- [ ] Verify generating_masks is logged
- [ ] Verify inpainting_frames is logged
- [ ] Verify rebuilding_clean_video is logged
- [ ] Verify watermark_removal_complete is logged

### 8.2 Admin Dashboard Integration
- [ ] View job status in admin UI
- [ ] Verify watermark stage appears
- [ ] Verify progress updates
- [ ] Verify logs are accessible

### 8.3 Error Logging
- [ ] Trigger an error condition
- [ ] Check logs capture full context
- [ ] Verify stack trace if applicable

---

## Test Video Creation Guide

Create test videos with watermarks using:

```bash
# Add text watermark to existing video
ffmpeg -i input.mp4 -vf "drawtext=text='uzmovi.com':fontsize=24:fontcolor=white@0.8:x=W-tw-10:y=10" output.mp4

# Add logo watermark
ffmpeg -i input.mp4 -i logo.png -filter_complex "overlay=W-w-10:10" output.mp4

# Add semi-transparent watermark
ffmpeg -i input.mp4 -vf "drawtext=text='Film Site':fontsize=20:fontcolor=white@0.5:x=10:y=H-th-10" output.mp4
```

---

## Expected Results Format

After running watermark removal, check for:

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
  "stages": [
    "received_local_video",
    "sampling_frames",
    "detecting_watermark",
    "generating_masks",
    "inpainting_frames",
    "rebuilding_clean_video",
    "watermark_removal_complete"
  ],
  "total_time": 45.2
}
```

---

## Running Tests

### Quick Detection Test
```bash
python -m watermark_removal.service &
curl -X GET "http://localhost:8084/detect?path=/path/to/video.mp4"
```

### Full Removal Test
```bash
curl -X POST http://localhost:8084/remove \
  -H "Content-Type: application/json" \
  -d '{"input_path": "/path/to/video.mp4"}'
```

### Go Worker Test
```bash
# Set environment
export WATERMARK_ENABLED=true
export WATERMARK_MODE=fast

# Start worker and trigger job
# Check logs for watermark removal stages
```
