# Adaptive Bitrate HLS Implementation

**Date:** 2026-03-31  
**Files Modified/Created:** [`worker/pipeline/hls_adaptive.go`](worker/pipeline/hls_adaptive.go), [`worker/pipeline/pipeline.go`](worker/pipeline/pipeline.go)  
**Goal:** Generate multi-bitrate adaptive HLS with master playlist for reliable playback across different network conditions

---

## 1. Root Cause / Current Limitation

### Previous Pipeline (Single-bitrate HLS)
- Generated only **one HLS stream** at 1080p
- No adaptive bitrate (ABR) support
- Fixed bitrate regardless of network conditions
- Poor playback experience on slower connections (buffering)
- Wasted bandwidth on faster connections

### Limitations Addressed
1. **No quality variants** - Users on mobile/3G had same experience as users on fiber
2. **Fixed bitrate** - 1080p @ CRF 20 meant high bandwidth usage
3. **No fallback** - Network fluctuations caused playback failures
4. **Single playlist** - No quality switching capability

---

## 2. Files Changed

| File | Change Type | Description |
|------|-------------|-------------|
| `worker/pipeline/hls_adaptive.go` | **Created** | New file containing adaptive HLS generation logic |
| `worker/pipeline/pipeline.go` | Modified | Updated `processVideo()` to use adaptive HLS, updated `uploadProcessedFiles()`, updated `validateAssetStorage()` |

---

## 3. New Adaptive HLS Pipeline

### Pipeline Overview

```
Input Video → Cut 10s → Base Video (delogo + logo) → Generate All Renditions → Master Playlist
```

### Step-by-Step Process

1. **Cut First 10 Seconds** - Skip intros/copyright
2. **Create Base Video** - Apply delogo + FilmoraUz logo once
3. **Generate Multiple Renditions** - Scale base to different resolutions
4. **Create Master Playlist** - Reference all variants

### Rendition Targets

| Resolution | Video Bitrate | Audio Bitrate | Total Bandwidth | Use Case |
|------------|---------------|---------------|-----------------|----------|
| **360p** | 800 kbps | 96 kbps | 896 kbps | Slow mobile (3G) |
| **480p** | 1400 kbps | 128 kbps | 1528 kbps | Mobile (4G) |
| **720p** | 2800 kbps | 128 kbps | 2928 kbps | WiFi / Moderate |
| **1080p** | 5000 kbps | 128 kbps | 5128 kbps | Fast WiFi / Fiber |

### FFmpeg Settings for Adaptive Streaming

```bash
# Key frame (GOP) alignment - CRITICAL for adaptive streaming
-g 48                    # GOP size: 2 seconds × 24fps = 48 frames
-keyint_min 48           # Minimum keyframe interval
-sc_threshold 0          # Disable scene cut detection (forces keyframes at GOP)

# Segment settings
-hls_time 6              # 6-second segments
-hls_list_size 0         # Include all segments
-hls_playlist_type vod   # VOD playlist

# Rate control
-b:v <bitrate>           # Target bitrate
-maxrate <1.5x>          # Peak rate (1.5× bitrate)
-bufsize <2x>            # Buffer size (2× bitrate)
```

---

## 4. Rendition List Used

```go
// From worker/pipeline/hls_adaptive.go
RenditionConfig{
    Name:        "360p",
    Width:       640,
    Height:      360,
    VideoBitrate: "800k",
    AudioBitrate: "96k",
    Bandwidth:   896000,  // bits/sec for master playlist
},
{
    Name:        "480p",
    Width:       854,
    Height:      480,
    VideoBitrate: "1400k",
    AudioBitrate: "128k",
    Bandwidth:   1528000,
},
{
    Name:        "720p",
    Width:       1280,
    Height:      720,
    VideoBitrate: "2800k",
    AudioBitrate: "128k",
    Bandwidth:   2928000,
},
{
    Name:        "1080p",
    Width:       1920,
    Height:      1080,
    VideoBitrate: "5000k",
    AudioBitrate: "128k",
    Bandwidth:   5128000,
}
```

---

## 5. Master Playlist Generation Details

### Output Structure

```
<canonical_folder>/
├── master.m3u8                    # Master playlist (entry point)
├── 360p/
│   ├── index.m3u8                # Variant playlist
│   ├── segment_001.ts
│   ├── segment_002.ts
│   └── ...
├── 480p/
│   ├── index.m3u8
│   ├── segment_001.ts
│   └── ...
├── 720p/
│   ├── index.m3u8
│   └── ...
└── 1080p/
    ├── index.m3u8
    └── ...
```

### Master Playlist Format

```m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-STREAM-INF:BANDWIDTH=896000,RESOLUTION=640x360,NAME="360p"
360p/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=1528000,RESOLUTION=854x480,NAME="480p"
480p/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=2928000,RESOLUTION=1280x720,NAME="720p"
720p/index.m3u8
#EXT-X-STREAM-INF:BANDWIDTH=5128000,RESOLUTION=1920x1080,NAME="1080p"
1080p/index.m3u8
```

### Key Features
- **EXT-X-STREAM-INF** - Defines each variant with bandwidth and resolution
- **NAME attribute** - Display name in quality selector (e.g., "360p", "480p")
- **Relative paths** - All variant playlists use relative paths from master location

---

## 6. Dev vs Prod Storage Behavior

### Development Mode (MODE=dev)

```go
// From hls_adaptive.go:uploadAdaptiveHLSFiles()
targetDir := filepath.Join(p.config.StorageConfig.LocalPath, "movies", folderName)
// Result: worker/uploads/movies/<slug>/master.m3u8
streamingURL := p.config.StorageConfig.BaseURL + "/stream/" + folderName + "/master.m3u8"
// Example: http://localhost:8080/stream/movietitle/index.m3u8
```

**Files are copied recursively to:**
- `worker/uploads/movies/<slug>/master.m3u8`
- `worker/uploads/movies/<slug>/360p/index.m3u8`
- `worker/uploads/movies/<slug>/360p/segment_001.ts`
- etc.

### Production Mode (MODE=prod)

```go
// From hls_adaptive.go:uploadAdaptiveHLSFiles()
remotePath := filepath.Join("videos", folderName, relPath)
// Result: B2/videos/<slug>/master.m3u8
streamingURL := p.storage.Upload(localPath, remotePath)
// Example: https://cdn.filmorauz.uz/videos/movietitle/master.m3u8
```

**Files are uploaded to B2/CDN with structure:**
- `videos/<slug>/master.m3u8`
- `videos/<slug>/360p/index.m3u8`
- etc.

---

## 7. DB / video_url Behavior

### How video_url is Stored

```go
// From pipeline.go:createMovieInDatabaseWithEnrichment()
movieDoc := bson.M{
    // ...
    "video_url": streamingURL,  // Master playlist URL
    // ...
}
```

### What Gets Stored

| Environment | video_url Value |
|-------------|-----------------|
| **Development** | `http://localhost:8080/stream/<slug>/master.m3u8` |
| **Production** | `https://cdn.filmorauz.uz/videos/<slug>/master.m3u8` |

### Frontend Player Integration

The frontend should use the master playlist URL directly:
```html
<video>
    <source src="http://localhost:8080/stream/movietitle/master.m3u8" type="application/x-mpegURL">
</video>
```

Modern HLS players (hls.js, video.js, native browser support) will:
1. Fetch the master playlist
2. Select appropriate quality based on bandwidth
3. Switch qualities automatically as network conditions change

---

## 8. Test Checklist

### Pre-Production Testing

```bash
# 1. Build the worker
cd worker && go build -o filmora-worker .

# 2. Prepare a test video
ffmpeg -f lavfi -i testsrc=duration=120:size=1920x1080:rate=24 \
  -c:v libx264 -pix_fmt yuv420p -y test_input.mp4

# 3. Run worker with test video (simulate job processing)
# Option A: Process a real ingestion job
# Option B: Direct test with ffmpeg commands
```

### Manual Test Cases

| # | Test | Expected Result | Pass/Fail |
|---|------|-----------------|-----------|
| 1 | Processed movie produces `master.m3u8` | File exists in output directory | ☐ |
| 2 | All quality variants exist | `360p/`, `480p/`, `720p/`, `1080p/` directories exist | ☐ |
| 3 | Each variant has `index.m3u8` | All variant playlists exist | ☐ |
| 4 | Segments are created | `segment_*.ts` files exist in each variant | ☐ |
| 5 | Master playlist format valid | Valid M3U8 with EXT-X-STREAM-INF entries | ☐ |
| 6 | Player loads master playlist | HLS player can parse master playlist | ☐ |
| 7 | Player switches quality | Quality selector shows options | ☐ |
| 8 | Playback on slow network | Player automatically uses 360p | ☐ |
| 9 | Playback on fast network | Player uses 720p or 1080p | ☐ |
| 10 | Network fluctuation handling | Player adapts to bandwidth changes | ☐ |
| 11 | Dev local URLs work | `http://localhost:8080/stream/...` serves files | ☐ |
| 12 | Prod CDN URLs work | CDN serves files correctly | ☐ |
| 13 | Video cuts first 10 seconds | Output starts at 0:10 | ☐ |
| 14 | FilmoraUz logo visible | Logo appears in bottom-right corner | ☐ |

### Verification Commands

```bash
# Check output structure
ls -la output/
ls -la output/360p/
ls -la output/480p/

# Check master playlist
cat output/master.m3u8

# Check variant playlist
cat output/360p/index.m3u8

# Verify segment count
find output/ -name "segment_*.ts" | wc -l

# Play with ffplay (if available)
ffplay output/master.m3u8

# Check video info for each rendition
ffprobe -v error -show_entries format=duration:stream=codec_name,width,height output/360p/index.m3u8
ffprobe -v error -show_entries format=duration:stream=codec_name,width,height output/1080p/index.m3u8

# Validate HLS compliance
ffmpeg -i output/master.m3u8 -f null - 2>&1 | grep -E "error|warning"
```

### Test with Different Input Resolutions

| Input Resolution | Expected Renditions |
|------------------|---------------------|
| 1920x1080 | 360p, 480p, 720p, 1080p (all 4) |
| 1280x720 | 360p, 480p, 720p (no 1080p) |
| 854x480 | 360p, 480p (no 720p/1080p) |
| 640x360 | 360p only |

---

## 9. Rollback Plan

If issues are found, rollback to single-bitrate HLS:

```bash
# View the changes
git diff HEAD worker/pipeline/pipeline.go

# Revert if needed
git checkout HEAD -- worker/pipeline/pipeline.go
git checkout HEAD -- worker/pipeline/hls_adaptive.go
```

### Key Differences to Verify During Rollback
- `processVideo()` should generate `index.m3u8` at top level (not in subdirectories)
- No `master.m3u8` file
- No rendition subdirectories (`360p/`, `480p/`, etc.)
- Single HLS stream (1080p only)

---

## 10. Performance Considerations

### Processing Time
- **Base video creation:** ~1x video duration
- **Each rendition:** ~0.25-0.5x video duration (parallelizable)
- **Total estimated:** 2-3x video duration

### Storage Requirements
| Resolution | Est. Size/minute | Notes |
|------------|------------------|-------|
| 360p | ~6 MB | Lowest quality |
| 480p | ~11 MB | |
| 720p | ~21 MB | |
| 1080p | ~38 MB | Highest quality |
| **Total** | ~76 MB/min | Sum of all |

### Bandwidth Savings
| User Network | With ABR | Without ABR |
|--------------|----------|-------------|
| 3G (~1 Mbps) | 360p (896 kbps) | 1080p buffering |
| 4G (~5 Mbps) | 720p (2.9 Mbps) | 1080p works |
| WiFi (~20 Mbps) | 1080p (5.1 Mbps) | 1080p works |

---

## 11. Monitoring Recommendations

After deployment, monitor:
- Average processing time per video
- Storage usage per movie (now ~4x)
- HLS playback issues reported by users
- Quality selection distribution (analytics)
- CDN bandwidth costs

```bash
# Watch worker logs during processing
tail -f worker.log | grep -E "HLS|adaptive|rendition|master"

# Monitor storage usage
du -sh worker/uploads/movies/*

# Check segment sizes
find output/ -name "segment_*.ts" -exec ls -lh {} \; | awk '{print $5, $9}'
```
