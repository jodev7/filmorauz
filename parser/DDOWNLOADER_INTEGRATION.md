# DDownloader Integration Documentation

## Overview

This document describes the integration of DDownloader's N_m3u8DL-RE binary and aria2c into the FilmoraUz parser service downloader.

---

## 1. Root Cause of the Old Downloader Problem

### Problems with the Previous FFmpeg-Based Approach

1. **Unstable HLS/DASH Progress Tracking**: The FFmpeg progress parsing was unreliable, with progress estimates based on elapsed time rather than actual bytes downloaded.

2. **Poor Error Handling**: FFmpeg would sometimes exit with partial files without proper validation, leading to corrupted downloads being treated as successful.

3. **No Proper Retry Logic**: Network interruptions during stream downloads would fail silently or require manual restart.

4. **Single-threaded by Default**: The FFmpeg approach doesn't naturally support parallel segment downloads.

5. **Content-Type Detection Issues**: The original detection sometimes failed to correctly identify manifest types, especially with CDN-served content.

### Problems with Python Parallel Download

1. **Range Request Dependency**: If servers don't support HTTP Range requests, the parallel download falls back to single-threaded mode.

2. **No Automatic Retries**: Network failures during parallel downloads could leave partial files.

3. **Memory Pressure**: Large files with many threads could cause memory issues.

---

## 2. Files Changed

### New Files

| File | Description |
|------|-------------|
| `parser/ddownloader_integration.py` | New standalone DDownloader integration module |

### Modified Files

| File | Changes |
|------|---------|
| `parser/downloader_service.py` | Added DDownloader integration imports and new methods |

---

## 3. Where DDownloader Was Integrated

### Integration Points in `downloader_service.py`

1. **Import Section (lines 16-27)**:
   - Added conditional import of `ddownloader_integration`
   - Added `USE_DDOWNLOADER` environment variable control

2. **New Method: `_download_mp4_with_aria2c()`**:
   - Uses aria2c via DDownloader for direct MP4 downloads
   - Provides parallel connections (16 connections, 4 splits)
   - Automatic retry support

3. **New Method: `_download_manifest_with_ddownloader()`**:
   - Uses N_m3u8DL-RE for HLS/DASH/ISM streams
   - Proper decryption engine support via FFmpeg
   - Multi-threaded segment downloading

4. **Modified `smart_download()` Method**:
   - Now checks for DDownloader availability
   - Routes to DDownloader methods when available
   - Falls back to original methods when DDownloader unavailable

---

## 4. How Type Detection Works Now

### Detection Order

1. **URL Extension Check** (fastest):
   ```
   .m3u8 → hls
   .mpd  → dash
   .ism  → ism
   .mp4  → mp4
   ```

2. **HEAD Request Content-Type** (fallback):
   - Sends HEAD request with User-Agent header
   - Checks Content-Type header
   - Handles CDN-modified types

3. **GET Request with Content Sampling** (last resort):
   - Opens connection and reads first 1KB
   - Checks for manifest signatures (#EXTM3U, <MPD, <manifest)

### Code Reference

```python
def detect_url_type(self, url: str) -> str:
    # 1. URL extension check
    # 2. HEAD request Content-Type
    # 3. GET request with content sampling
    # Returns: "hls", "dash", "ism", "mp4", or "unknown"
```

---

## 5. How MP4 Fallback Works Now

### Primary: aria2c (via DDownloader)

```python
def _download_mp4_with_aria2c(self, url, output_path, ...):
    # Uses aria2c with:
    # - 16 parallel connections (-x 16)
    # - 4-way split (-s 4)
    # - 1M chunk size (-k 1M)
    # - 3 automatic retries
    # - Continue support for partial downloads
```

### Fallback: Original Python Parallel Download

When `USE_DDOWNLOADER=false` or DDownloader is unavailable:
- Uses original 8-thread parallel download
- Range request detection
- Python-based progress tracking

---

## 6. How Download Validation Works

### File Validation Steps

1. **Existence Check**:
   ```python
   if not os.path.exists(path):
       return False, "File does not exist"
   ```

2. **Size Check**:
   ```python
   file_size = os.path.getsize(path)
   if file_size == 0:
       return False, "File is empty"
   ```

3. **Minimum Size Check** (1024 bytes default):
   ```python
   if file_size < min_size:
       return False, "File size too small"
   ```

### Validation in Download Flow

After every download attempt:
```python
is_valid, error_msg = validate_downloaded_file(path)
if not is_valid:
    raise DownloadError(f"Validation failed: {error_msg}")
```

---

## 7. Unsupported/Protected Sources Handling

### Unsupported Types (e.g., Unknown URLs)

```python
if stream_type == StreamType.UNKNOWN.value:
    return DownloadResult(
        success=False,
        source=url,
        video_type=stream_type,
        error=f"Unknown stream type. Cannot determine how to download: {url}",
        ...
    )
```

### Protected/DRM Content

The system does NOT attempt DRM circumvention:
- If a manifest requires decryption keys that aren't provided, N_m3u8DL-RE will fail
- The error is propagated with clear messaging
- No fake success is returned

### Clear Error Messages

All errors include:
- Source URL (truncated for logging)
- Stream type
- Exact failure reason
- Duration of attempt

---

## 8. Manual Test Checklist

### Test Environment Setup

```bash
# Enable DDownloader (default)
export USE_DDOWNLOADER=true

# Or disable to use original methods
export USE_DDOWNLOADER=false

# Run parser service
cd parser && python server.py
```

### Test Cases

#### 1. Valid M3U8 Stream
```bash
# Expected: Download succeeds via N_m3u8DL-RE
# Check: File exists, size > 0, no errors in logs
```

#### 2. Valid MPD Stream
```bash
# Expected: Download succeeds via N_m3u8DL-RE
# Check: File exists, size > 0, no errors in logs
```

#### 3. Valid ISM Stream
```bash
# Expected: Download succeeds via N_m3u8DL-RE
# Check: File exists, size > 0, no errors in logs
```

#### 4. Valid Direct MP4
```bash
# Expected: Download succeeds via aria2c
# Check: File exists, size > 0, parallel download logs visible
```

#### 5. Unknown URL Type
```bash
# Expected: Clear error message
# Check: "Unknown stream type" in response
```

#### 6. Broken URL
```bash
# Expected: Retry attempts followed by failure
# Check: 3 retry attempts with backoff
```

#### 7. Reported Success but Missing File
```bash
# Expected: Validation catches missing file
# Check: "File does not exist" error returned
```

#### 8. Output File Size Zero
```bash
# Expected: Validation catches empty file
# Check: "File is empty" error returned
```

#### 9. Parser Returns Clean Structured Error
```bash
# Expected: Structured response with success=false and error field
# Check: Response includes error message, no fake success
```

---

## 9. Structured Response Format

### Success Response
```json
{
  "success": true,
  "source": "https://example.com/video.m3u8",
  "video_type": "hls",
  "local_path": "/path/to/downloads/video.mp4",
  "output_filename": "video.mp4",
  "file_size": 1048576,
  "error": null,
  "download_duration_seconds": 45.2
}
```

### Error Response
```json
{
  "success": false,
  "source": "https://example.com/unknown",
  "video_type": "unknown",
  "local_path": null,
  "output_filename": "video.mp4",
  "file_size": 0,
  "error": "Unknown stream type. Cannot determine how to download: https://example.com/unknown",
  "download_duration_seconds": 0.5
}
```

---

## 10. Configuration Options

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `USE_DDOWNLOADER` | `true` | Enable/disable DDownloader integration |
| `BACKEND_URL` | `""` | Backend API URL for progress updates |

### Binary Paths

The integration automatically searches for binaries in:
1. `DDownloader/bin/` (bundled)
2. System PATH
3. Common locations (`/usr/bin/`, `/usr/local/bin/`)

---

## 11. Logging Reference

### Log Prefixes

| Prefix | Source |
|--------|--------|
| `[DDOWNLOADER]` | ddownloader_integration.py |
| `[ARIA2C]` | aria2c download operations |
| `[DETECT]` | URL type detection |
| `[VALIDATE]` | File validation |
| `[DOWNLOADER]` | downloader_service.py |

### Example Log Output

```
[DDOWNLOADER] smart_download called: url=https://example.com/video.m3u8...
[DETECT] Checking URL type: https://example.com/video.m3u8...
[DETECT] Detected HLS from URL extension
[DDOWNLOADER] Detected stream type: hls
[ARIA2C] Starting MP4 download: url=https://example.com/video.m3u8...
[VALIDATE] File valid: /path/to/downloads/video.mp4 (1048576 bytes)
[DDOWNLOADER] Download validated successfully: /path/to/downloads/video.mp4
```

---

## 12. Troubleshooting

### Binary Not Found

If you see "N_m3u8DL-RE binary not found":
1. Check that DDownloader package is installed
2. Verify binaries exist in `venv/lib/python*/site-packages/DDownloader/bin/`
3. Try reinstalling: `pip install DDownloader`

### aria2c Not Found

If aria2c download fails:
1. Verify aria2c is installed: `aria2c --version`
2. Check it's in PATH
3. Fallback to Python parallel download with `USE_DDOWNLOADER=false`

### FFmpeg Not Found

If manifest downloads fail with FFmpeg errors:
1. Verify FFmpeg is installed: `ffmpeg -version`
2. Check binary path in logs
3. Install FFmpeg: `apt install ffmpeg` (Debian/Ubuntu)

---

## 13. Migration Notes

### For Existing Deployments

1. **No Breaking Changes**: The integration is backward-compatible
2. **Graceful Fallback**: Original methods work if DDownloader unavailable
3. **Feature Flag**: Use `USE_DDOWNLOADER=false` to disable

### Recommended Upgrade Path

1. Deploy new code alongside existing
2. Test with `USE_DDOWNLOADER=true` on non-production first
3. Monitor logs for any integration issues
4. Enable by default after validation

---

## 14. Future Enhancements

Potential improvements for future iterations:

1. **Progress Reporting**: Implement real-time progress from N_m3u8DL-RE
2. **Quality Selection**: Add option to select specific quality streams
3. **Cookie Support**: Pass cookies for authenticated streams
4. **Rate Limiting**: Add download speed throttling option
5. **Checksum Verification**: Add SHA256/MD5 verification for downloads

---

## Summary

The DDownloader integration provides:

- ✅ **More robust** HLS/DASH/ISM downloads via N_m3u8DL-RE
- ✅ **Faster** MP4 downloads via aria2c parallel connections
- ✅ **Proper validation** to prevent fake success
- ✅ **Clear error handling** for unsupported sources
- ✅ **Structured responses** for the worker pipeline
- ✅ **Comprehensive logging** for debugging
- ✅ **Graceful fallback** to original methods
