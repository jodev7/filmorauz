# Uzmovi Parser Fix - Summary

## Root Cause Analysis

The uzmovi parser was failing due to **overly aggressive URL validation** that was rejecting valid video URLs from uzmovi's CDN (`srv*.uzdown.space`).

### Problems Identified:

1. **`is_valid_media_url()` in `media_extractor.py`** - Rejected URLs containing `/movie/` or `/film/` patterns unless they had explicit video extensions
2. **`classify_media_url()` in `media_extractor.py`** - Classified uzmovi CDN URLs as "html" due to pattern matching
3. **`isValidStreamUrl()` in `helpers.py`** - Same overly strict validation
4. **Multiple validation layers** - Same URLs were being validated multiple times and rejected at each layer
5. **Insufficient logging** - Could not debug why URLs were being rejected

### Files Changed:

1. **`parser/media_extractor.py`**:
   - `is_valid_media_url()`: Made uzmovi-specific domain validation more lenient
   - `classify_media_url()`: Check for uzmovi domains BEFORE HTML pattern rejection
   - `extract_from_uzmovi()`: More aggressive extraction of uzmovi CDN URLs

2. **`parser/helpers.py`**:
   - `isValidStreamUrl()`: Added uzmovi-specific domain acceptance

3. **`parser/uzmovi.py`**:
   - `_dedupe_media_urls()`: Enhanced logging throughout
   - `_extract_all_media_from_page()`: Comprehensive logging at every step

4. **`parser/server.py`**:
   - `_extract_best_video_url()`: More lenient fallback, accept unknown types

## Key Fixes Applied

### 1. Validation Leniency for uzmovi CDN

```python
# ACCEPT: ALL URLs from uzmovi video CDN domains
uzmovi_domains = [
    "srv",  # srv*.uzdown.space
    "uzdown",  # Fallback for any uzdown URLs
]

for domain in uzmovi_domains:
    if domain in url_lower:
        if "/" in url and len(url) > 20:
            return True, f"Valid media from uzmovi domain: {domain}"
```

### 2. Classification Order Fix

```python
# Check for uzmovi video CDN BEFORE checking for HTML patterns
uzmovi_video_domains = ["srv", "uzdown"]
for domain in uzmovi_video_domains:
    if domain in url_lower:
        return "mp4"  # Default to mp4 for uzmovi CDN
```

### 3. Enhanced uzmovi Extraction

```python
# Extract ALL URLs from srv*.uzdown.space domain
srv_pattern = r'https://srv\d*\.uzdown\.space/[^\s"\'<>]+'
# Also extract any uzdown.space URLs
uzdown_pattern = r'https://[^\s"\'<>]*uzdown[^\s"\'<>]+'
```

### 4. Server-Side Leniency

```python
# Accept unknown types as last resort for uzmovi
preferred_types = [
    "m3u8", "mpd", "ism", "mp4",
    # ... other types ...
    "unknown",  # FIXED: Accept unknown types
]
```

## Manual Test Checklist

### Test Environment Setup

```bash
cd /home/jodev/Desktop/filmorauz/parser
source venv/bin/activate
export PARSER_DEBUG=true
python server.py
```

### Test 1: Search for "Interstellar"

1. **Backend API Test**:
   ```bash
   curl "http://localhost:8082/search?source=uzmovi&q=interstellar"
   ```
   - Expected: Returns list of search results with titles, posters, and detail URLs
   - Log should show: `[SERVER] Search response: N results for query='interstellar'`

2. **Verify search results contain valid URLs**:
   - Check that each result has `detail_url` pointing to uzmovi
   - Check that `source_id` is extracted

### Test 2: Get Details for "Interstellar"

1. **Parser Details Test**:
   ```bash
   curl "http://localhost:8082/details?source=uzmovi&id=interstellar&job_id=test123"
   ```
   - Expected: Returns video_url, video_url_type, and eventually local_path

2. **Expected Logs**:
   ```
   [UZMOVI] === DETAILS EXTRACTION ===
   [UZMOVI] Page URL: https://uzmovi.tv/...
   [UZMOVI] === STEP 2: Specialized uzmovi extraction ===
   [UZMOVI] Specialized extraction found N candidates
   [UZMOVI] === STEP 3: Regex extraction ===
   [UZMOVI] === STEP 4: Iframe extraction ===
   [UZMOVI] === STEP 5: Deduplication ===
   [UZMOVI]   Accepted: X, Rejected: Y
   [UZMOVI] === FINAL MEDIA SUMMARY ===
   [UZMOVI]   Total media URLs: Z
   [SERVER] SELECTED media URL:
   [SERVER]   type: mp4/m3u8/mpd
   [SERVER]   url: https://srv*.uzdown.space/...
   [PARSER] Downloading video: ...
   [PARSER] Download successful: /path/to/file.mp4
   ```

3. **Verify response contains**:
   ```json
   {
     "success": true,
     "video_url": "https://srv*.uzdown.space/...",
     "video_url_type": "mp4",
     "local_path": "/absolute/path/to/downloaded/video.mp4",
     "download_completed": true,
     "steps_download": true
   }
   ```

### Test 3: Get Details for "Forsaj 8"

1. **Search for Forsaj 8**:
   ```bash
   curl "http://localhost:8082/search?source=uzmovi&q=forsaj+8"
   ```

2. **Get Details for Forsaj 8**:
   ```bash
   curl "http://localhost:8082/details?source=uzmovi&url=<full_uzmovi_url>&job_id=test456"
   ```

3. **Verify same success criteria as Test 2**

### Test 4: Verify Backend Progress Reporting

1. **Start parser with BACKEND_URL set**:
   ```bash
   export BACKEND_URL=http://localhost:8080
   python server.py
   ```

2. **Import a movie from admin panel**:
   - Navigate to admin panel
   - Go to Ingestion section
   - Search for "Interstellar"
   - Click Import

3. **Check progress updates**:
   - Should see real-time progress in admin dashboard
   - Progress should show: parsing -> downloading -> processing
   - When complete, status should be "processing" with steps.download=true

### Test 5: Verify Worker Receives local_path

1. **Check job in database**:
   - After download completes, `local_path` should be set in MongoDB
   - `steps.download` should be `true`
   - `steps.process` should be `false` (until worker starts)

2. **Check worker log**:
   - Worker should claim job with `steps.download=true`
   - Worker should find file at `local_path`
   - Worker should proceed to next stage (watermark, cut, HLS, etc.)

## Debugging Commands

### Enable Debug Logging

```bash
export PARSER_DEBUG=true
python server.py
```

### Check Parser Logs

```bash
# Watch logs in real-time
tail -f server.log

# Or run parser directly
cd /home/jodev/Desktop/filmorauz/parser
source venv/bin/activate
export PARSER_DEBUG=true
python -c "
from uzmovi import UzmoviParser
parser = UzmoviParser()
results = parser.search('interstellar')
print(f'Found {len(results)} results')
"
```

### Check Downloaded Files

```bash
ls -la /home/jodev/Desktop/filmorauz/parser/downloads/
```

## Expected Success Response Format

```json
{
  "success": true,
  "source": "uzmovi",
  "title": "Interstellar",
  "source_id": "6282",
  "detail_url": "https://uzmovi.tv/...",
  "video_url": "https://srv*.uzdown.space/...",
  "video_url_type": "mp4",
  "local_path": "/home/jodev/Desktop/filmorauz/parser/downloads/Interstellar.mp4",
  "download_completed": true,
  "download_needed": false,
  "poster_url": "https://uzmovi.tv/...",
  "file_path": "/home/jodev/Desktop/filmorauz/parser/downloads/Interstellar.mp4",
  "file_name": "Interstellar.mp4",
  "file_size": 1234567890,
  "stream_type": "mp4",
  "metadata": {...},
  "error": null
}
```

## Expected Failure Response Format

```json
{
  "success": false,
  "error": "No playable video URL found",
  "source": "uzmovi",
  "title": "Interstellar",
  "video_url_type": null,
  "local_path": null,
  "download_completed": false,
  "download_needed": false,
  "error": "Could not extract valid media URL"
}
```

## Common Issues and Solutions

### Issue: "No video_urls found"

**Symptoms**: Parser returns empty video_urls list

**Possible Causes**:
1. uzmovi page structure changed
2. JavaScript-rendered content (need Playwright)
3. Network error fetching page

**Debug Steps**:
1. Check if uzmovi.tv is accessible: `curl -I https://uzmovi.tv`
2. Enable DEBUG mode: `export PARSER_DEBUG=true`
3. Check HTML samples in logs

### Issue: "Download failed"

**Symptoms**: Download starts but fails

**Possible Causes**:
1. URL is not a direct video URL (might be HTML)
2. CDN requires authentication/cookies
3. Rate limiting

**Debug Steps**:
1. Check selected URL type in logs
2. Verify URL is not a player/embed page
3. Try with proper Referer header

### Issue: "File too small"

**Symptoms**: Download completes but file is < 1MB

**Possible Causes**:
1. Downloaded error page instead of video
2. Network interruption

**Debug Steps**:
1. Check file size: `ls -la downloaded_file.mp4`
2. Check if file is valid MP4: `file downloaded_file.mp4`
3. Check server logs for download validation errors
