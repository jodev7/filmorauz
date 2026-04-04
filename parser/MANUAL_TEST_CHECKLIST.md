# Parser Service Manual Test Checklist

## Overview

This checklist validates that the parser service correctly extracts valid media URLs and rejects HTML page URLs.

## Test Environment Setup

```bash
cd parser
export PARSER_DEBUG=true
python -m pytest tests/ -v  # Run unit tests if available
```

## Root Cause Analysis - Recent Fixes

### Fixed Issues

1. **URL Validation Too Strict**: The `is_valid_media_url()` function in `media_extractor.py` was rejecting valid URLs from video hosting sites. Fixed by:
   - Being more lenient with video hosting domains (.uz, .tk, .ru)
   - Accepting URLs from known video hosting domains even without explicit extensions
   - Allowing URLs with subdirectories from video sites

2. **helpers.py Validation**: Updated `isValidStreamUrl()` to match the improved validation logic

3. **DDownloader Integration**: Fixed variable naming (`DDdownloaderIntegration`)

### Key Changes

- `parser/media_extractor.py`: Enhanced `is_valid_media_url()` to accept more valid URLs
- `parser/helpers.py`: Updated `isValidStreamUrl()` with improved validation
- `parser/downloader_service.py`: Fixed `DDdownloaderIntegration` variable naming

---

## 1. Media URL Extraction Tests

### 1.1 Valid MP4 Source Extraction

**Test Case**: Parser correctly extracts direct MP4 URLs

```
Expected: Parser returns URLs ending in .mp4
URL Pattern: https://example.com/video.mp4
```

Steps:
1. Select a movie with direct MP4 source
2. Run parser extraction
3. Verify `video_url` ends with `.mp4`
4. Verify `video_url_type` is `mp4`

**Expected Result**:
```json
{
  "success": true,
  "video_url": "https://cdn.example.com/video/123.mp4",
  "video_url_type": "mp4"
}
```

### 1.2 Valid M3U8 Source Extraction

**Test Case**: Parser correctly extracts HLS manifest URLs

```
Expected: Parser returns URLs ending in .m3u8
URL Pattern: https://example.com/playlist.m3u8
```

Steps:
1. Select a movie with HLS streaming source
2. Run parser extraction
3. Verify `video_url` ends with `.m3u8`
4. Verify `video_url_type` is `m3u8`

**Expected Result**:
```json
{
  "success": true,
  "video_url": "https://cdn.example.com/hls/123/index.m3u8",
  "video_url_type": "m3u8"
}
```

### 1.3 Valid MPD Source Extraction

**Test Case**: Parser correctly extracts DASH manifest URLs

```
Expected: Parser returns URLs ending in .mpd
URL Pattern: https://example.com/manifest.mpd
```

Steps:
1. Select a movie with DASH streaming source
2. Run parser extraction
3. Verify `video_url` ends with `.mpd`
4. Verify `video_url_type` is `mpd`

**Expected Result**:
```json
{
  "success": true,
  "video_url": "https://cdn.example.com/dash/123/manifest.mpd",
  "video_url_type": "mpd"
}
```

---

## 2. URL Validation Tests

### 2.1 Page with Multiple Candidate URLs

**Test Case**: Parser selects best URL when multiple candidates exist

Steps:
1. Provide a page with both:
   - Direct MP4 URL
   - HLS manifest URL
   - Player iframe URL
2. Run parser extraction
3. Verify the **best** URL is selected (m3u8 > mpd > mp4)

**Expected Result**:
- M3U8 selected if available
- MPD selected if m3u8 not available
- MP4 selected as fallback
- Player iframe URL NOT selected

### 2.2 Page with Escaped JS URLs

**Test Case**: Parser decodes escaped URLs in JavaScript

Steps:
1. Provide a page with escaped URL patterns:
   - `https:\/\/cdn.example.com\/video.mp4`
   - `https%3A%2F%2Fcdn.example.com%2Fvideo.mp4`
   - Unicode escape sequences
2. Run parser extraction
3. Verify URL is properly decoded

**Expected Result**:
```json
{
  "success": true,
  "video_url": "https://cdn.example.com/video.mp4"
}
```

### 2.3 HTML Page URL Correctly Rejected

**Test Case**: Parser rejects HTML page URLs

**URLs that MUST be rejected**:
- `https://example.com/movie/123`
- `https://example.com/film/456/details`
- `https://example.com/player.php?id=789`
- `https://example.com/embed/player`
- `https://example.com/page/1`
- `https://example.com/video?id=123`

Steps:
1. Provide URLs from the rejection list
2. Run parser validation
3. Verify all URLs are rejected

**Expected Result**:
```json
{
  "success": false,
  "error": "Could not extract valid media URL"
}
```

**Expected Log Output**:
```
[MEDIA_EXTRACTOR] Rejected candidate: https://example.com/movie/123... Reason: Page route detected: /movie/
```

---

## 3. Source-Specific Tests

### 3.1 uzmovi.tv Tests

**Test Case 1**: Extract from uzmovi with direct MP4

```bash
python -c "
from uzmovi import UzmoviParser
parser = UzmoviParser()
details = parser.get_details('https://uzmovi.tv/film/123-movie-title.html')
print('Video URLs:', details.video_urls)
"
```

Expected: Returns valid MP4/M3U8/MPD URLs, NOT iframe/player URLs

**Test Case 2**: uzmovi with iframe player

```bash
python -c "
from uzmovi import UzmoviParser
parser = UzmoviParser()
# Find a movie that uses iframe player
details = parser.get_details('https://uzmovi.tv/film/456-iframe-movie.html')
print('Video URLs:', details.video_urls)
"
```

Expected: Parser fetches iframe and extracts actual video URL

### 3.2 asilmedia.org Tests

**Test Case 1**: Extract from DLE-based player

```bash
python -c "
from asilmedia import AsilmediaParser
parser = AsilmediaParser()
details = parser.get_details('https://asilmedia.org/123-movie-title.html')
print('Video URLs:', details.video_urls)
"
```

Expected: Returns valid media URLs from DLE player config

**Test Case 2**: asilmedia with nested player

```bash
python -c "
from asilmedia import AsilmediaParser
parser = AsilmediaParser()
details = parser.get_details('https://asilmedia.org/456-nested-player.html')
print('Video URLs:', details.video_urls)
"
```

Expected: Parser follows nested players and extracts actual video URL

### 3.3 freekino Tests

**Test Case 1**: Extract from freekino

```bash
python -c "
from freekino import FreekinoParser
parser = FreekinoParser()
details = parser.get_detail('https://freekino.com/movie/123')
print('Video URL:', details.get('video_url'))
print('Video Type:', details.get('video_type'))
"
```

Expected: Returns valid media URL with correct type

---

## 4. Structured Response Tests

### 4.1 Success Response Format

**Test Case**: Parser returns properly structured success payload

```bash
curl -X POST http://localhost:5000/parse \
  -H "Content-Type: application/json" \
  -d '{
    "source": "uzmovi",
    "source_id": "12345",
    "title": "Test Movie"
  }'
```

**Expected Response**:
```json
{
  "success": true,
  "source": "uzmovi",
  "title": "Test Movie",
  "video_url": "https://cdn.example.com/video.m3u8",
  "video_url_type": "m3u8",
  "poster_url": "https://example.com/poster.jpg",
  "backdrop_url": "https://example.com/backdrop.jpg",
  "headers": {
    "Referer": "https://uzmovi.tv/",
    "User-Agent": "Mozilla/5.0..."
  },
  "local_path": null,
  "download_needed": true,
  "error": null
}
```

### 4.2 Failure Response Format

**Test Case**: Parser returns properly structured failure payload

**Expected Response**:
```json
{
  "success": false,
  "source": "uzmovi",
  "title": "Test Movie",
  "video_url": null,
  "video_url_type": null,
  "poster_url": null,
  "backdrop_url": null,
  "headers": {},
  "local_path": null,
  "download_needed": false,
  "error": "Could not extract valid media URL"
}
```

### 4.3 Validation Logging

**Test Case**: Parser logs validation steps

Enable debug logging and verify output:

```bash
export PARSER_DEBUG=true
# Run parser...
```

**Expected Log Output**:
```
[MEDIA_EXTRACTOR] Found candidate: type=mp4, source=script_uzdown, url=https://srv1.uzdown.space/...
[MEDIA_EXTRACTOR] Validation: 1 valid, 0 rejected
[MEDIA_EXTRACTOR] Selected best candidate: type=m3u8, quality=auto, url=https://...
```

---

## 5. Edge Cases

### 5.1 Empty Page (No Video)

**Test Case**: Page has no video content

Steps:
1. Provide a page with no video elements, scripts, or iframes
2. Run parser extraction
3. Verify failure response

**Expected Result**:
```json
{
  "success": false,
  "error": "Could not extract valid media URL"
}
```

### 5.2 Mixed Valid/Invalid Candidates

**Test Case**: Page has mix of valid and invalid URLs

Steps:
1. Provide a page with:
   - Valid .m3u8 URL
   - Invalid .html page URL
   - Invalid /player/ iframe URL
2. Run parser extraction
3. Verify only valid URL is returned

**Expected Result**: Valid m3u8 URL selected, invalid URLs rejected with logging

### 5.3 CDN URLs Without Extension

**Test Case**: Parser handles CDN URLs without file extension

URL Pattern: `https://cdn.example.com/video/123?token=abc`

Steps:
1. Provide a CDN URL without extension
2. Verify it's correctly validated

**Expected Result**: URL accepted if it passes validation rules (contains video indicators)

---

## 6. Integration Tests

### 6.1 End-to-End with Downloader

**Test Case**: Parser output works with downloader

Steps:
1. Parse a movie and get video URL
2. Pass URL to downloader service
3. Verify download succeeds

**Expected Result**: Download completes successfully

### 6.2 Server API Endpoint

**Test Case**: `/parse` endpoint works correctly

```bash
curl -X POST http://localhost:5000/parse \
  -H "Content-Type: application/json" \
  -d '{
    "source": "uzmovi",
    "source_id": "test-123",
    "job_id": "job-456"
  }'
```

**Expected Result**: Structured JSON response with video URL or error

---

## 7. Validation Rule Verification

### 7.1 Rejection Rules Test

Verify these URL patterns are ALWAYS rejected:

| Pattern | Should Reject | Reason |
|---------|--------------|--------|
| `.html` | YES | HTML page |
| `/details` | YES | Page route |
| `/page/` | YES | Page route |
| `/movie/` (no extension) | YES | Page route |
| `/film/` (no extension) | YES | Page route |
| `/serial/` (no extension) | YES | Page route |
| `?page=` | YES | Page parameter |
| `?id=` | YES | Page parameter |
| `/player/` | YES | Player page |
| `/embed/` | YES | Embed page |

### 7.2 Acceptance Rules Test

Verify these URL patterns are ACCEPTED:

| Pattern | Should Accept | Reason |
|---------|--------------|--------|
| `.m3u8` | YES | HLS manifest |
| `.mpd` | YES | DASH manifest |
| `.ism` | YES | ISM manifest |
| `.mp4` | YES | Direct video |
| `/hls/` | YES | HLS path |
| `/dash/` | YES | DASH path |
| `cdn.` + video indicator | YES | CDN |
| `uzdown` + video indicator | YES | Known host |

---

## 8. Regression Tests

### 8.1 Previously Working URLs Still Work

**Test Case**: Ensure existing working URLs are not broken

Steps:
1. Collect list of previously working movie URLs
2. Test each one with updated parser
3. Verify success rate

**Expected Result**: 100% success rate (or same as before if some sources changed)

### 8.2 No HTML URLs in Output

**Test Case**: Verify no HTML URLs in any response

Steps:
1. Parse all test movies
2. Check all `video_url` values
3. Verify none contain HTML indicators

**Expected Result**: Zero HTML URLs in responses

---

## Test Execution Commands

### Run All Tests

```bash
cd parser
export PARSER_DEBUG=true

# Test each source
python -c "
from uzmovi import UzmoviParser
from asilmedia import AsilmediaParser
from freekino import FreekinoParser

# Test uzmovi
parser = UzmoviParser()
# Add test URLs...

# Test asilmedia
parser = AsilmediaParser()
# Add test URLs...

# Test freekino
parser = FreekinoParser()
# Add test URLs...
"
```

### Run Validation Tests

```bash
python -c "
from media_extractor import (
    is_valid_media_url,
    validate_media_url_strict,
    classify_media_url,
)

# Test URLs
test_urls = [
    ('https://cdn.example.com/video.m3u8', True),
    ('https://example.com/movie/123', False),
    ('https://cdn.example.com/video.mp4', True),
    ('https://example.com/player.php?id=123', False),
]

for url, expected in test_urls:
    result = is_valid_media_url(url)
    actual = result[0]
    status = 'PASS' if actual == expected else 'FAIL'
    print(f'{status}: {url} -> expected={expected}, got={actual}')
"
```

### Test Server Endpoint

```bash
# Start server
python server.py &

# Test success case
curl -X POST http://localhost:5000/parse \
  -H "Content-Type: application/json" \
  -d '{"source": "uzmovi", "source_id": "123"}'

# Test failure case
curl -X POST http://localhost:5000/parse \
  -H "Content-Type: application/json" \
  -d '{"source": "uzmovi", "source_id": "nonexistent"}'
```

---

## Success Criteria

All tests must pass before deployment:

- [ ] Valid MP4 URLs extracted correctly
- [ ] Valid M3U8 URLs extracted correctly
- [ ] Valid MPD URLs extracted correctly
- [ ] HTML page URLs rejected
- [ ] Multiple candidates handled correctly
- [ ] Escaped URLs decoded correctly
- [ ] Iframe URLs handled correctly (fetched, not returned)
- [ ] Structured response format correct
- [ ] Error response format correct
- [ ] Logging output clear and useful
- [ ] All sources (uzmovi, asilmedia, freekino) work correctly
- [ ] Integration with downloader works
- [ ] No regression in existing functionality

---

## Debugging Tips

If a test fails:

1. Enable debug logging: `export PARSER_DEBUG=true`
2. Check logs for rejection reasons
3. Verify URL classification: `classify_media_url(url)`
4. Check validation: `validate_media_url_strict(url)`
5. Review extraction pipeline:
   - Extract candidates
   - Validate candidates
   - Select best candidate

---

## Contact for Issues

If tests fail unexpectedly:
1. Check changelog for recent modifications
2. Verify source websites haven't changed structure
3. Review error logs for details
