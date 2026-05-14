"""
Media Extractor Module - Comprehensive media URL extraction and validation

This module provides a unified system for:
1. Extracting all media candidates from HTML pages, scripts, and iframes
2. Classifying candidate URLs by media type (mp4, m3u8, mpd)
3. Validating candidates against strict page URL rejection rules
4. Selecting the best valid media URL using priority rules
5. Returning structured payloads for downloader/worker

The parser must NEVER return HTML page URLs as media URLs.
"""

import re
import logging
import json
import requests
from typing import List, Dict, Any, Optional, Tuple
from urllib.parse import urljoin, urlparse, unquote
from dataclasses import dataclass, field
from bs4 import BeautifulSoup

logger = logging.getLogger(__name__)

# Enable debug logging
DEBUG = __import__("os").environ.get("PARSER_DEBUG", "false").lower() == "true"


# ============================================================================
# DATA STRUCTURES
# ============================================================================

@dataclass
class MediaCandidate:
    """
    Represents a potential media URL candidate.
    
    Attributes:
        url: The candidate URL
        type: Media type ('mp4', 'm3u8', 'mpd', 'html', 'unknown')
        quality: Quality hint if available (e.g., '1080p', '720p', 'auto')
        source_hint: Where/how this candidate was found
        headers: Any required headers (e.g., Referer)
        confidence: Confidence score (0-1) for this being a valid media URL
        base_url: The page URL this candidate was extracted from
    """
    url: str
    type: str = "unknown"
    quality: str = "auto"
    source_hint: str = "unknown"
    headers: Dict[str, str] = field(default_factory=dict)
    confidence: float = 0.5
    base_url: str = ""
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            "url": self.url,
            "type": self.type,
            "quality": self.quality,
            "source_hint": self.source_hint,
            "headers": self.headers,
            "confidence": self.confidence,
        }


@dataclass
class MediaExtractionResult:
    """
    Structured result from media extraction process.
    
    This is the final output format that will be returned to the downloader.
    """
    success: bool
    source: str
    title: str
    video_url: Optional[str]
    video_url_type: Optional[str]
    poster_url: Optional[str] = None
    backdrop_url: Optional[str] = None
    headers: Dict[str, str] = field(default_factory=dict)
    local_path: Optional[str] = None
    download_needed: bool = True
    error: Optional[str] = None
    
    # Extended info for debugging
    candidates_found: int = 0
    candidates_rejected: int = 0
    candidate_details: List[Dict[str, Any]] = field(default_factory=list)
    
    def to_dict(self) -> Dict[str, Any]:
        return {
            "success": self.success,
            "source": self.source,
            "title": self.title,
            "video_url": self.video_url,
            "video_url_type": self.video_url_type,
            "poster_url": self.poster_url,
            "backdrop_url": self.backdrop_url,
            "headers": self.headers,
            "local_path": self.local_path,
            "download_needed": self.download_needed,
            "error": self.error,
        }


# ============================================================================
# URL NORMALIZATION
# ============================================================================

def normalize_url(url: str, base_url: str = "") -> str:
    """
    Normalize URL - make relative URLs absolute and clean up.
    
    Args:
        url: The URL to normalize
        base_url: Base URL for resolving relative URLs
        
    Returns:
        Normalized absolute URL
    """
    if not url:
        return ""
    
    url = url.strip()
    
    # Handle protocol-relative URLs
    if url.startswith("//"):
        return "https:" + url
    
    # Already absolute
    if url.startswith(("http://", "https://")):
        return url
    
    # Relative URL - resolve against base
    if base_url:
        return urljoin(base_url, url)
    
    return url


def decode_url(url: str) -> str:
    """
    Decode URL-encoded characters and escaped sequences.
    
    Handles:
    - Standard URL encoding (%XX)
    - Double-escaped sequences
    - Unicode escapes
    """
    if not url:
        return ""
    
    # First decode pass
    try:
        url = unquote(url)
    except Exception:
        pass
    
    # Handle escaped slashes and other patterns
    # Pattern: \\/ in URLs that should be /
    url = url.replace("\\/", "/")
    
    # Handle double-escaped patterns
    url = re.sub(r'\\\\/', '/', url)
    
    return url


def normalize_url_comprehensive(url: str, base_url: str = "") -> str:
    """
    Comprehensive URL normalization with decoding.
    
    Args:
        url: The URL to normalize
        base_url: Base URL for resolving relative URLs
        
    Returns:
        Fully normalized URL
    """
    if not url:
        return ""
    
    # First decode any escaped sequences
    url = decode_url(url)
    
    # Then normalize
    url = normalize_url(url, base_url)
    
    return url


# ============================================================================
# MEDIA TYPE CLASSIFICATION
# ============================================================================

def classify_media_url(url: str, headers: Dict[str, str] = None) -> str:
    """
    Classify a URL by its media type.
    
    Returns:
        'mp4' - Direct MP4 video file
        'm3u8' - HLS manifest
        'mpd' - DASH manifest
        'ism' - ISM manifest
        'html' - HTML page (invalid for downloader)
        'unknown' - Cannot determine type
        
    FIXED: Now properly classifies uzmovi video URLs without rejecting them as 'html'.
    """
    if not url:
        return "unknown"
    
    url_lower = url.lower()
    
    # Direct video file extensions (highest priority)
    if ".m3u8" in url_lower:
        return "m3u8"
    
    if ".mpd" in url_lower:
        return "mpd"
    
    if ".ism" in url_lower:
        return "ism"
    
    if ".ismc" in url_lower:
        return "ism"
    
    if ".mp4" in url_lower:
        return "mp4"
    
    # CRITICAL FIX: Check for uzmovi video CDN BEFORE checking for HTML patterns
    # uzmovi uses srv*.uzdown.space which should NOT be classified as 'html'
    # even if URL contains /player/ or /embed/ which are common on uzmovi
    uzmovi_video_domains = [
        "srv",  # srv*.uzdown.space - uzmovi's main video CDN
        "uzdown",  # Fallback for any uzdown URLs
    ]
    
    for domain in uzmovi_video_domains:
        if domain in url_lower:
            # Check for manifest indicators
            if "mpd" in url_lower:
                return "mpd"
            if any(ind in url_lower for ind in ["index", "playlist", "m3u8", "/hls/", "chunk", "segment"]):
                return "m3u8"
            # Default to mp4 for uzmovi CDN URLs without explicit manifest extension
            return "mp4"
    
    # HTML page indicators - MUST reject these
    # FIXED: Be more lenient - only reject clearly page-like URLs
    # Don't reject URLs that contain video indicators
    html_patterns = [
        ".html", ".htm",
        "/details", "/detail",
        "/page/", "/pages/",
        "/search/", "/category/",
        "?page=", "&page=",
        ".php", ".asp", ".aspx",
    ]
    
    # Check if URL has video indicators that would make it valid
    has_video_indicator = any(vi in url_lower for vi in [
        ".mp4", ".m3u8", ".mpd", ".ism",
        "cdn", "video", "stream", "media", "srv",
        "uzdown", "/hls/", "/dash/", "index", "playlist", "chunk", "segment"
    ])
    
    # Only reject HTML patterns if URL doesn't have video indicators
    if not has_video_indicator:
        for pattern in html_patterns:
            if pattern in url_lower:
                return "html"
    
    # Manifest patterns in path
    manifest_patterns = [
        "/hls/", "/dash/",
        "/playlist.m3u8", "/master.m3u8",
        "/manifest.mpd",
        "/manifest", "/manifests",
    ]
    
    for pattern in manifest_patterns:
        if pattern in url_lower:
            # Check if it's actually an HLS or DASH manifest
            if "m3u8" in pattern or "/hls/" in pattern:
                return "m3u8"
            if "mpd" in pattern or "/dash/" in pattern:
                return "mpd"
            return "m3u8"  # Default to m3u8 for /hls/
    
    # Video segment patterns
    segment_patterns = [
        "/segment/", "/chunks/",
        "/ts/", "/fragments/",
    ]
    
    for pattern in segment_patterns:
        if pattern in url_lower:
            return "m3u8"  # These are typically HLS segments
    
    # Known video CDN domains (high confidence of being media)
    video_cdn_domains = [
        "cdn.", "video.", "stream.",
        "media.", "files.", "assets.",
    ]
    
    for domain in video_cdn_domains:
        if domain in url_lower:
            # If it has video-related indicators, classify appropriately
            if "index" in url_lower or "playlist" in url_lower:
                return "m3u8"
            return "mp4"  # Default for CDN URLs without extension
    
    # No extension but contains video path indicators
    video_path_indicators = [
        "/video/", "/videos/", "/media/",
        "/clip/", "/clips/",
    ]
    
    for pattern in video_path_indicators:
        if pattern in url_lower:
            return "mp4"
    
    return "unknown"


# ============================================================================
# STRICT VALIDATION
# ============================================================================

def is_valid_media_url(url: str, headers: Dict[str, str] = None) -> Tuple[bool, str]:
    """
    Validate if a URL is a valid media URL for the downloader.
    
    This function accepts:
    - Direct .mp4 files
    - .m3u8 HLS manifests
    - .mpd DASH manifests
    - URLs from known video CDNs (including uzmovi's srv*.uzdown.space)
    - Video hosting services (sukit, uzbek, video hosting sites)
    
    This function rejects:
    - HTML page URLs (with .html extension at the end)
    - URLs that are clearly page routes without any video indicators
    
    FIXED: Made validation less aggressive to accept valid uzmovi video URLs.
    
    Args:
        url: The URL to validate
        headers: Optional response headers (for content-type checking)
        
    Returns:
        Tuple of (is_valid, reason)
    """
    if not url:
        return False, "Empty URL"
    
    url_lower = url.lower()
    
    # === REJECT: HTML files at the end of URL ===
    # Be careful not to reject .mp4.html style CDN URLs
    html_extensions = [".html", ".htm"]
    for ext in html_extensions:
        if url_lower.endswith(ext):
            return False, f"HTML file extension detected: {ext}"
        # Also reject if .html appears without video extension before it
        if ext in url_lower and not any(ve in url_lower for ve in [".mp4", ".m3u8", ".mpd"]):
            # Check if it's in query params (might be OK)
            if f"?{ext}" in url_lower or f"&{ext}" in url_lower:
                continue
            return False, f"HTML extension detected: {ext}"
    
    # === VALIDATION: Direct media extensions (HIGHEST PRIORITY) ===
    # These are always valid - return early for performance
    
    if ".m3u8" in url_lower:
        return True, "Valid HLS manifest"
    
    if ".mpd" in url_lower:
        return True, "Valid DASH manifest"
    
    if ".ism" in url_lower or ".ismc" in url_lower:
        return True, "Valid ISM manifest"
    
    if ".mp4" in url_lower:
        return True, "Valid MP4 file"
    
    # === VALIDATION: uzmovi-specific patterns (HIGHEST PRIORITY FOR UZMOVI) ===
    # uzmovi uses srv*.uzdown.space for video hosting - accept any URL from these domains
    
    # CRITICAL FIX: Accept ALL URLs from uzmovi video CDN domains without strict indicators
    # This fixes the issue where valid uzmovi URLs were being rejected
    uzmovi_domains = [
        "srv",  # srv*.uzdown.space - uzmovi's main video CDN
        "uzdown",  # Fallback for any uzdown URLs
        "uzmovi",  # uzmovi.tv video URLs
    ]
    
    for domain in uzmovi_domains:
        if domain in url_lower:
            # Accept any URL from uzmovi domains that has a path component
            if "/" in url.split("://", 1)[-1]:
                return True, f"Valid media from uzmovi domain: {domain}"
    
    # === VALIDATION: Known video CDN patterns ===
    
    video_cdn_patterns = [
        # Domain + required indicators
        # uzmovi-specific patterns: accept "live/" directory and various path patterns
        ("uzdown", ["index", "playlist", "video", "movie", "clip", "mp4", "m3u8", "mpd", "file", "play", "live", "mob", "hd", "sd", "uzmovi", "content", "storage", "static", "assets", "download", "get", "v"]),
        # Accept any URL from srv*.uzdown.space as valid (uzmovi uses this pattern)
        # CRITICAL FIX: Added more flexible path patterns to accept various uzmovi URL structures
        ("srv", ["index", "playlist", "video", "movie", "clip", "mp4", "m3u8", "mpd", "file", "play", "live", "mob", "hd", "sd", "uzmovi", "content", "storage", "static", "assets", "download", "get", "v"]),
        ("sukit", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        ("cdn.", ["mp4", "m3u8", "mpd", "video", "media", "file"]),
        ("video.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        ("stream.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        ("media.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        # Uzbek/Kazakh video hosting patterns
        (".uz", ["mp4", "m3u8", "mpd", "video", "file", "index", "playlist", "live"]),
        (".tk", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        (".ru", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        # Free movie site patterns
        ("uzmovi", ["mp4", "m3u8", "mpd", "video", "file", "index", "live"]),
    ]
    
    for domain, indicators in video_cdn_patterns:
        if domain in url_lower:
            if any(ind in url_lower for ind in indicators):
                return True, f"Valid media from known CDN: {domain}"
    
    # === VALIDATION: Manifest path patterns ===
    
    manifest_path_patterns = [
        "/hls/", "/dash/",
        "/playlist.m3u8", "/master.m3u8",
        "/index.m3u8", "/live.m3u8",
        "/manifest.mpd", "/manifest",
        "/segment/", "/chunks/",
        "/videos/", "/video/",
        # CRITICAL FIX: Added more flexible path patterns for uzmovi URLs
        "/mob/", "/storage/", "/content/", "/static/", "/assets/",
        "/stream/", "/clip/", "/clips/",
    ]
    
    for pattern in manifest_path_patterns:
        if pattern in url_lower:
            return True, f"Valid manifest path: {pattern}"
    
    # === REJECT: Obviously invalid page routes (but be lenient with video sites) ===
    # These are only rejected if the URL doesn't have video indicators
    
    # FIXED: Be more lenient - only reject if URL is clearly a page without any video indicators
    page_route_patterns = [
        "/details", "/detail/",
        "/page/", "/pages/",
        "/search/", "/category/",
        "/watch/", "/view/",
    ]
    
    # Check if URL is purely a page route without video indicators
    has_video_indicator = any(vi in url_lower for vi in [
        ".mp4", ".m3u8", ".mpd", ".ism",
        "cdn", "video", "stream", "media", "srv",
        "uzdown", "sukit", "/hls/", "/dash/",
        "index", "playlist", "chunk", "segment",
        "mob", "storage", "content", "static", "assets",
        "download", "get", "v", "clip"
    ])
    
    # FIXED: Only reject page routes if NO video indicators at all
    if not has_video_indicator:
        for pattern in page_route_patterns:
            if pattern in url_lower:
                return False, f"Page route detected: {pattern}"
    
    # === FINAL CHECK: If URL has a domain and looks like video hosting ===
    # Allow URLs from video hosting sites even without explicit extensions
    
    # Check for common video hosting domains
    video_hosting_domains = [
        "uzmovi", "uzmov", "freekino", "asilmedia", "kinolar",
        "sukit", "uzbek", "tork", "kino",
        "cdn", "video", "stream", "media", "asset"
    ]
    
    if any(domain in url_lower for domain in video_hosting_domains):
        # If it has a proper URL structure with path, it might be valid
        if url_lower.startswith("http") and "/" in url:
            # Check if it looks like a media URL (has path segments)
            path_segments = url_lower.split("/")
            if len(path_segments) > 3:  # Has subdirectory, likely not homepage
                return True, "Valid video hosting URL"
    
    # === REJECT: Unknown URLs without video indicators ===
    
    return False, "No valid media indicators found"


def validate_media_url_strict(url: str, headers: Dict[str, str] = None) -> Optional[str]:
    """
    Strict validation that returns the reason if invalid, None if valid.
    
    This is a convenience wrapper around is_valid_media_url.
    
    Args:
        url: The URL to validate
        headers: Optional response headers
        
    Returns:
        None if valid, error reason string if invalid
    """
    is_valid, reason = is_valid_media_url(url, headers)
    if is_valid:
        return None
    return reason


# ============================================================================
# MEDIA CANDIDATE EXTRACTION
# ============================================================================

def extract_media_candidates(
    html_content: str,
    base_url: str,
    referer: str = ""
) -> List[MediaCandidate]:
    """
    Extract all possible media URL candidates from HTML content.
    
    This function searches multiple locations for media URLs:
    1. Direct <video> and <source> tags
    2. JavaScript variables and configs (file, src, url)
    3. JSON data in scripts
    4. Player library configs (jwplayer, videojs, etc.)
    5. iframe src attributes
    6. Direct .mp4/.m3u8/.mpd links in attributes
    
    Args:
        html_content: Raw HTML content (string)
        base_url: The page URL this content was fetched from
        referer: Referer header to use for iframe fetches
        
    Returns:
        List of MediaCandidate objects
    """
    candidates = []
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Extracting candidates from: {base_url}")
    
    # Parse HTML
    try:
        soup = BeautifulSoup(html_content, "lxml")
    except Exception as e:
        if DEBUG:
            logger.warning(f"[MEDIA_EXTRACTOR] Failed to parse HTML: {e}")
        return candidates
    
    # === 1. Extract from <video> and <source> tags ===
    video_candidates = _extract_from_video_tags(soup, base_url)
    candidates.extend(video_candidates)
    
    # === 2. Extract from JavaScript ===
    script_candidates = _extract_from_scripts(soup, base_url)
    candidates.extend(script_candidates)
    
    # === 3. Extract from iframes (but don't fetch them) ===
    iframe_candidates = _extract_from_iframes(soup, base_url)
    candidates.extend(iframe_candidates)
    
    # === 4. Extract from data attributes ===
    data_candidates = _extract_from_data_attributes(soup, base_url)
    candidates.extend(data_candidates)
    
    # === 5. Extract from JSON-LD ===
    jsonld_candidates = _extract_from_jsonld(soup, base_url)
    candidates.extend(jsonld_candidates)
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Found {len(candidates)} candidates")
    
    return candidates


def _extract_from_video_tags(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """Extract media from <video> and <source> HTML elements."""
    candidates = []
    
    # Find all video elements
    for video in soup.find_all("video"):
        # Check video src attribute
        src = video.get("src", "")
        if src:
            candidate = _create_candidate(src, base_url, "video_src")
            if candidate:
                candidates.append(candidate)
        
        # Check poster attribute
        poster = video.get("poster", "")
        # Don't add poster as media candidate
        
        # Check for <source> children
        for source in video.find_all("source"):
            src = source.get("src", "")
            if src:
                quality = source.get("label", source.get("data-quality", "auto"))
                candidate = _create_candidate(src, base_url, "video_source", quality)
                if candidate:
                    candidates.append(candidate)
    
    # Find standalone source elements
    for source in soup.find_all("source"):
        src = source.get("src", "")
        if src:
            quality = source.get("label", source.get("data-quality", "auto"))
            candidate = _create_candidate(src, base_url, "standalone_source", quality)
            if candidate:
                candidates.append(candidate)
    
    return candidates


def _extract_from_scripts(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """Extract media URLs from JavaScript code in <script> tags."""
    candidates = []
    
    # Regex patterns for various media URL formats
    patterns = [
        # Direct URL patterns
        (r'["\']([^"\']*\.mp4)["\']', "mp4"),
        (r'["\']([^"\']*\.m3u8)["\']', "m3u8"),
        (r'["\']([^"\']*\.mpd)["\']', "mpd"),
        (r'["\']([^"\']*\.ism)["\']', "ism"),
        
        # Object/property patterns
        (r'["\'](src|file|url|source|videoUrl|mediaUrl|sourceUrl)["\']\s*:\s*["\']([^"\']+\.(?:mp4|m3u8|mpd|ism))["\']', "object_prop"),
        (r'["\'](src|file|url|source|videoUrl|mediaUrl)["\']\s*:\s*["\']([^"\']+)["\']', "object_prop"),
        (r'(?:src|file|url|source|videoUrl)\s*[=:]\s*["\']([^"\']+(?:\.mp4|\.m3u8|\.mpd))["\']', "assignment"),
        
        # JWPlayer patterns
        (r'file\s*:\s*["\']([^"\']+(?:\.mp4|\.m3u8|\.mpd))["\']', "jwplayer"),
        (r'jwplayer\s*\([^)]*\)\.setup\s*\((.*?)\)', "jwplayer_setup"),
        (r'sources\s*:\s*\[([^\]]+)\]', "jwplayer_sources"),
        
        # VideoJS patterns
        (r'videoId\s*:\s*["\']([^"\']+)["\']', "videojs"),
        
        # DLE player patterns
        (r'"sound_file"\s*:\s*["\']([^"\']+)["\']', "dle"),
        (r'"video_url"\s*:\s*["\']([^"\']+)["\']', "dle"),
        
        # Generic http/https URLs with video extensions
        (r'(https?://[^\s"\'<>]+\.mp4)', "http_url"),
        (r'(https?://[^\s"\'<>]+\.m3u8)', "http_url"),
        (r'(https?://[^\s"\'<>]+\.mpd)', "http_url"),
        
        # CDN/video server patterns
        (r'(https?://srv\d*\.uzdown\.[^\s"\'<>]+)', "uzdown"),
        (r'(https?://[^\s"\'<>]+/(?:video|media|stream|clip)[^\s"\'<>]*)', "cdn_path"),
    ]
    
    for script in soup.find_all("script"):
        content = script.string or ""
        if not content or len(content) < 50:
            continue
        
        for pattern, pattern_type in patterns:
            try:
                matches = re.findall(pattern, content, re.IGNORECASE)
                for match in matches:
                    # Handle tuple matches from object patterns
                    if isinstance(match, tuple):
                        url = match[-1] if len(match) > 1 else match[0]
                    else:
                        url = match
                    
                    if not url or url.startswith("data:"):
                        continue
                    
                    # Skip non-media URLs
                    skip_patterns = ["thumbnail", "poster", "image", "avatar", "logo", "icon", ".jpg", ".jpeg", ".png", ".gif", ".webp"]
                    if any(p in url.lower() for p in skip_patterns):
                        continue
                    
                    if pattern_type in ("jwplayer_sources", "jwplayer_setup"):
                        nested_urls = re.findall(r'(?:file|src|url|source)\s*:\s*["\']([^"\']+)["\']', url, re.IGNORECASE)
                        nested_urls.extend(re.findall(r'(https?:\\?/\\?/[^"\'<>\s]+?(?:\.mp4|\.m3u8|\.mpd)(?:\?[^"\'<>\s]*)?)', url, re.IGNORECASE))
                        for nested_url in nested_urls:
                            candidate = _create_candidate(nested_url, base_url, f"script_{pattern_type}")
                            if candidate:
                                candidates.append(candidate)
                    else:
                        candidate = _create_candidate(url, base_url, f"script_{pattern_type}")
                        if candidate:
                            candidates.append(candidate)
            except re.error:
                continue
    
    return candidates


def _extract_from_iframes(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """Extract iframe src attributes as potential player pages."""
    candidates = []
    
    for iframe in soup.find_all("iframe"):
        src = iframe.get("src", "")
        if src and not src.startswith("about:"):
            candidate = _create_candidate(src, base_url, "iframe")
            if candidate:
                # Mark as lower confidence since we need to fetch it
                candidate.confidence = 0.3
                candidates.append(candidate)
    
    return candidates


def _extract_from_data_attributes(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """Extract media URLs from data-* attributes."""
    candidates = []
    
    # Look for common data attributes containing URLs
    data_url_attrs = [
        "data-src", "data-video", "data-url", "data-file",
        "data-video-url", "data-media-url", "data-stream-url",
        "data-player-url", "data-source",
    ]
    
    for attr in data_url_attrs:
        for elem in soup.find_all(attrs={attr: True}):
            value = elem.get(attr, "")
            if value and any(ext in value.lower() for ext in [".mp4", ".m3u8", ".mpd"]):
                candidate = _create_candidate(value, base_url, f"data_{attr}")
                if candidate:
                    candidates.append(candidate)
    
    return candidates


def _extract_from_jsonld(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """Extract media URLs from JSON-LD structured data."""
    candidates = []
    
    for script in soup.find_all("script", type="application/ld+json"):
        content = script.string or ""
        if not content:
            continue
        
        try:
            data = json.loads(content)
            candidates.extend(_extract_from_json_object(data, base_url))
        except json.JSONDecodeError:
            pass
    
    return candidates


def _extract_from_json_object(obj: Any, base_url: str) -> List[MediaCandidate]:
    """Recursively extract URLs from JSON objects."""
    candidates = []
    
    if isinstance(obj, str):
        if any(ext in obj.lower() for ext in [".mp4", ".m3u8", ".mpd"]):
            candidate = _create_candidate(obj, base_url, "jsonld")
            if candidate:
                candidates.append(candidate)
    elif isinstance(obj, dict):
        for key, value in obj.items():
            if any(key.lower() in ["url", "src", "file", "contenturl"] for key in [key]):
                if isinstance(value, str):
                    candidate = _create_candidate(value, base_url, "jsonld")
                    if candidate:
                        candidates.append(candidate)
            candidates.extend(_extract_from_json_object(value, base_url))
    elif isinstance(obj, list):
        for item in obj:
            candidates.extend(_extract_from_json_object(item, base_url))
    
    return candidates


def _create_candidate(
    url: str,
    base_url: str,
    source_hint: str,
    quality: str = "auto"
) -> Optional[MediaCandidate]:
    """
    Create a MediaCandidate from a URL string.
    
    Performs validation and normalization.
    """
    if not url:
        return None
    
    # Decode and normalize
    url = decode_url(url)
    url = normalize_url(url, base_url)
    
    # Skip blob URLs
    if url.startswith("blob:"):
        if DEBUG:
            logger.info(f"[MEDIA_EXTRACTOR] Skipping blob URL: {url}")
        return None
    
    # Skip empty or very short URLs
    if len(url) < 10:
        return None
    
    # Classify the URL type
    url_type = classify_media_url(url)
    
    # Skip HTML/Page URLs at extraction time (will be rejected later anyway)
    if url_type == "html":
        if DEBUG:
            logger.info(f"[MEDIA_EXTRACTOR] Rejected HTML candidate: {url}")
        return None
    
    # Create candidate
    candidate = MediaCandidate(
        url=url,
        type=url_type,
        quality=quality,
        source_hint=source_hint,
        base_url=base_url,
        confidence=0.7 if url_type in ["mp4", "m3u8", "mpd"] else 0.5
    )
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Found candidate: type={url_type}, source={source_hint}, url={url[:60]}...")
    
    return candidate


# ============================================================================
# CANDIDATE VALIDATION
# ============================================================================

def validate_candidates(candidates: List[MediaCandidate]) -> Tuple[List[MediaCandidate], List[Dict[str, Any]]]:
    """
    Validate all candidates and separate valid from invalid.
    
    Args:
        candidates: List of MediaCandidate objects
        
    Returns:
        Tuple of (valid_candidates, rejected_candidates_with_reasons)
    """
    valid = []
    rejected = []
    
    for candidate in candidates:
        error = validate_media_url_strict(candidate.url)
        if error:
            if DEBUG:
                logger.info(f"[MEDIA_EXTRACTOR] Rejected candidate: {candidate.url[:60]}... Reason: {error}")
            rejected.append({
                "url": candidate.url,
                "reason": error,
                "type": candidate.type,
                "source_hint": candidate.source_hint,
            })
        else:
            valid.append(candidate)
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Validation: {len(valid)} valid, {len(rejected)} rejected")
    
    return valid, rejected


# ============================================================================
# BEST CANDIDATE SELECTION
# ============================================================================

def choose_best_media_candidate(
    candidates: List[MediaCandidate],
    prefer_quality: str = None
) -> Optional[MediaCandidate]:
    """
    Select the best media candidate from a list.
    
    Priority rules (highest to lowest):
    1. m3u8 (HLS manifests)
    2. mpd (DASH manifests)
    3. ism (ISM manifests)
    4. mp4 (direct video files)
    
    Within same type:
    - Prefer higher confidence
    - Prefer higher quality if specified
    - Prefer shorter/more direct URLs
    
    Args:
        candidates: List of validated MediaCandidate objects
        prefer_quality: Optional quality preference (e.g., "1080p")
        
    Returns:
        Best MediaCandidate or None if no valid candidates
    """
    if not candidates:
        return None
    
    # Define priority by type
    type_priority = {
        "m3u8": 100,
        "mpd": 90,
        "ism": 80,
        "mp4": 70,
        "unknown": 50,
    }
    
    # Quality priority map
    quality_priority = {
        "4k": 400,
        "2160p": 400,
        "1080p": 300,
        "720p": 200,
        "480p": 100,
        "360p": 50,
        "240p": 25,
        "auto": 10,
        "unknown": 0,
    }
    
    def score_candidate(candidate: MediaCandidate) -> Tuple[int, int, float]:
        """Calculate score for a candidate. Higher is better."""
        type_score = type_priority.get(candidate.type, 0)
        quality_score = quality_priority.get(candidate.quality.lower() if isinstance(candidate.quality, str) else "auto", 0)
        confidence = candidate.confidence
        
        # Prefer quality if specified
        if prefer_quality:
            if candidate.quality.lower() == prefer_quality.lower():
                quality_score += 500
        
        return (type_score, quality_score, confidence)
    
    # Sort by score (descending)
    candidates.sort(key=score_candidate, reverse=True)
    
    best = candidates[0]
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Selected best candidate: type={best.type}, quality={best.quality}, url={best.url[:60]}...")
        logger.info(f"[MEDIA_EXTRACTOR]   confidence={best.confidence}, source={best.source_hint}")
    
    return best


# ============================================================================
# COMPREHENSIVE EXTRACTION FUNCTION
# ============================================================================

def extract_and_validate_media(
    html_content: str,
    base_url: str,
    source: str = "",
    title: str = "",
    referer: str = ""
) -> MediaExtractionResult:
    """
    Comprehensive media extraction pipeline.
    
    This is the main entry point for media extraction.
    
    Pipeline:
    1. Extract all candidates from HTML
    2. Validate candidates (reject HTML pages, invalid URLs)
    3. Select best candidate by priority rules
    4. Return structured result
    
    Args:
        html_content: Raw HTML content
        base_url: Page URL this content was fetched from
        source: Source name (uzmovi, asilmedia, freekino)
        title: Movie/show title for logging
        referer: Referer header for iframe fetches
        
    Returns:
        MediaExtractionResult with structured payload
    """
    result = MediaExtractionResult(
        success=False,
        source=source,
        title=title,
        video_url=None,
        video_url_type=None,
    )
    
    # Extract candidates
    candidates = extract_media_candidates(html_content, base_url, referer)
    result.candidates_found = len(candidates)
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] === EXTRACTION START ===")
        logger.info(f"[MEDIA_EXTRACTOR] Source: {source}, Title: {title}")
        logger.info(f"[MEDIA_EXTRACTOR] Found {len(candidates)} candidates")
    
    # Validate candidates
    valid_candidates, rejected_candidates = validate_candidates(candidates)
    result.candidates_rejected = len(rejected_candidates)
    result.candidate_details = [
        {"url": c.url, "type": c.type, "reason": r["reason"]}
        for c, r in zip(rejected_candidates, rejected_candidates)
    ]
    
    if not valid_candidates:
        if DEBUG:
            logger.warning(f"[MEDIA_EXTRACTOR] No valid media URLs found")
        result.error = "Could not extract valid media URL"
        return result
    
    # Select best candidate
    best_candidate = choose_best_media_candidate(valid_candidates)
    
    if not best_candidate:
        if DEBUG:
            logger.warning(f"[MEDIA_EXTRACTOR] Failed to select best candidate")
        result.error = "Could not select valid media URL from candidates"
        return result
    
    # Build result
    result.success = True
    result.video_url = best_candidate.url
    result.video_url_type = best_candidate.type
    result.headers = best_candidate.headers
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] === EXTRACTION SUCCESS ===")
        logger.info(f"[MEDIA_EXTRACTOR] Selected URL: {result.video_url[:80]}...")
        logger.info(f"[MEDIA_EXTRACTOR] Selected type: {result.video_url_type}")
    
    return result


# ============================================================================
# IFRAME FETCHING AND EXTRACTION
# ============================================================================

def fetch_iframe_and_extract(
    iframe_url: str,
    base_url: str,
    referer: str = "",
    session: requests.Session = None
) -> MediaExtractionResult:
    """
    Fetch an iframe URL and extract media from its content.
    
    This is used when an iframe is found but needs to be fetched
    to get the actual media URL.
    
    Args:
        iframe_url: The iframe src URL
        base_url: Parent page URL
        referer: Referer header to send
        session: Optional requests session
        
    Returns:
        MediaExtractionResult from the iframe content
    """
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Fetching iframe: {iframe_url}")
    
    # Validate iframe URL first
    error = validate_media_url_strict(iframe_url)
    if error and "iframe" in error.lower():
        if DEBUG:
            logger.warning(f"[MEDIA_EXTRACTOR] Iframe URL looks like a page, skipping: {iframe_url}")
        return MediaExtractionResult(
            success=False,
            source="iframe",
            title="",
            error=f"Iframe URL appears to be a page: {error}"
        )
    
    try:
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
        }
        if referer:
            headers["Referer"] = referer
        
        if session:
            response = session.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
        else:
            response = requests.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
        
        final_url = response.url
        content_type = response.headers.get("Content-Type", "")
        
        if DEBUG:
            logger.info(f"[MEDIA_EXTRACTOR] Iframe response: {final_url}")
            logger.info(f"[MEDIA_EXTRACTOR] Content-Type: {content_type}")
        
        # Check if it's HTML
        if "text/html" in content_type:
            # Parse HTML and extract
            return extract_and_validate_media(
                response.text,
                final_url,
                source="iframe",
            )
        else:
            # Direct media response
            return MediaExtractionResult(
                success=True,
                source="iframe",
                title="",
                video_url=final_url,
                video_url_type=classify_media_url(final_url),
            )
            
    except requests.RequestException as e:
        if DEBUG:
            logger.warning(f"[MEDIA_EXTRACTOR] Failed to fetch iframe: {e}")
        return MediaExtractionResult(
            success=False,
            source="iframe",
            title="",
            error=f"Failed to fetch iframe: {str(e)}"
        )


# ============================================================================
# SOURCE-SPECIFIC EXTRACTION HELPERS
# ============================================================================

def extract_from_uzmovi(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """
    Source-specific extraction for uzmovi.tv.
    
    uzmovi uses specific patterns that may not be caught by generic extraction.
    
    FIXED: Now extracts ALL URLs from uzmovi CDN domains, not just those with specific extensions.
    This is critical because uzmovi's srv*.uzdown.space URLs may not always have .mp4/.m3u8 extensions
    in the URL but are still valid video streams.
    """
    candidates = []
    
    # Generic extraction first
    candidates.extend(extract_media_candidates(str(soup), base_url))
    
    # uzmovi-specific patterns
    html_content = str(soup)
    
    # CRITICAL FIX: Pattern: ALL URLs from srv*.uzdown.space domain
    # uzmovi uses this CDN for video hosting - accept any URL from this domain
    # This is more aggressive than just looking for extensions
    srv_pattern = r'https://srv\d*\.uzdown\.space/[^\s"\'<>]+'
    srv_matches = re.findall(srv_pattern, html_content, re.IGNORECASE)
    logger.info(f"[MEDIA_EXTRACTOR] Found {len(srv_matches)} srv*.uzdown.space URLs")
    for match in srv_matches:
        if match and len(match) > 20:  # Filter out very short matches
            candidate = _create_candidate(match, base_url, "uzmovi_srv")
            if candidate:
                candidates.append(candidate)
                logger.info(f"[MEDIA_EXTRACTOR] Added uzmovi srv URL: {match[:100]}...")
    
    # CRITICAL FIX: Also look for any uzdown.space URLs (even without srv prefix)
    uzdown_pattern = r'https://[^\s"\'<>]*uzdown[^\s"\'<>]+'
    uzdown_matches = re.findall(uzdown_pattern, html_content, re.IGNORECASE)
    logger.info(f"[MEDIA_EXTRACTOR] Found {len(uzdown_matches)} uzdown URLs")
    for match in uzdown_matches:
        if match and len(match) > 20 and match not in srv_matches:
            candidate = _create_candidate(match, base_url, "uzmovi_uzdown")
            if candidate:
                candidates.append(candidate)
                logger.info(f"[MEDIA_EXTRACTOR] Added uzmovi uzdown URL: {match[:100]}...")
    
    # Pattern: uzmovi player configs - look for any video-related config
    uzmovi_config_patterns = [
        r'["\']?player["\']?\s*:\s*["\']([^"\']+)["\']',
        r'["\']?playlist["\']?\s*:\s*\[([^\]]+)\]',
        r'["\']?sources["\']?\s*:\s*\[([^\]]+)\]',
        r'["\']?file["\']?\s*:\s*["\']([^"\']+)["\']',
        r'["\']?source["\']?\s*:\s*["\']([^"\']+)["\']',
        r'["\']?src["\']?\s*:\s*["\']([^"\']+)["\']',
        r'["\']?url["\']?\s*:\s*["\']([^"\']+)["\']',
    ]
    
    for pattern in uzmovi_config_patterns:
        matches = re.findall(pattern, html_content, re.IGNORECASE)
        for match in matches:
            # Check if it's a video URL (any URL that looks like video hosting)
            if any(ind in match.lower() for ind in ['.mp4', '.m3u8', '.mpd', 'uzdown', 'uzmovi', 'video', 'stream', 'srv', 'cdn']):
                if match and len(match) > 10:
                    candidate = _create_candidate(match, base_url, "uzmovi_config")
                    if candidate:
                        candidates.append(candidate)
    
    # CRITICAL FIX: Look for encoded/encrypted video URLs
    # uzmovi sometimes encodes URLs in JavaScript - look for common patterns
    encoded_patterns = [
        r'atob\(["\']([^"\']+)["\']\)',  # Base64 encoded
        r'decodeURIComponent\(["\']([^"\']+)["\']\)',  # URL encoded
    ]
    
    for pattern in encoded_patterns:
        matches = re.findall(pattern, html_content)
        for match in matches:
            try:
                import base64
                decoded = base64.b64decode(match).decode('utf-8', errors='ignore')
                if any(ind in decoded.lower() for ind in ['.mp4', '.m3u8', '.mpd', 'uzdown', 'uzmovi']):
                    candidate = _create_candidate(decoded, base_url, "uzmovi_encoded")
                    if candidate:
                        candidates.append(candidate)
                        logger.info(f"[MEDIA_EXTRACTOR] Added decoded uzmovi URL: {decoded[:100]}...")
            except:
                pass
    
    logger.info(f"[MEDIA_EXTRACTOR] extract_from_uzmovi: {len(candidates)} total candidates")
    
    return candidates


def extract_from_asilmedia(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """
    Source-specific extraction for asilmedia.org.
    
    asilmedia typically uses DLE CMS player embeds.
    """
    candidates = []
    
    # Generic extraction first
    candidates.extend(extract_media_candidates(str(soup), base_url))
    
    # DLE-specific patterns
    html_content = str(soup)
    
    # DLE player patterns
    dle_patterns = [
        r'"file"?\s*:\s*["\']([^"\']+\.(?:mp4|m3u8|mpd))["\']',
        r'"url"?\s*:\s*["\']([^"\']+\.(?:mp4|m3u8|mpd))["\']',
        r'"src"?\s*:\s*["\']([^"\']+\.(?:mp4|m3u8|mpd))["\']',
    ]
    
    for pattern in dle_patterns:
        matches = re.findall(pattern, html_content, re.IGNORECASE)
        for match in matches:
            candidate = _create_candidate(match, base_url, "asilmedia_dle")
            if candidate:
                candidates.append(candidate)
    
    return candidates


def extract_from_freekino(soup: BeautifulSoup, base_url: str) -> List[MediaCandidate]:
    """
    Source-specific extraction for freekino.
    """
    candidates = []
    
    # Generic extraction first
    candidates.extend(extract_media_candidates(str(soup), base_url))
    
    # freekino-specific patterns
    html_content = str(soup)
    
    # Common player patterns
    patterns = [
        r'(?:file|source|src|url)\s*[=:]\s*["\']([^"\']+\.(?:mp4|m3u8|mpd))["\']',
        r'(?:file|source|src|url)\s*[=:]\s*["\']([^"\']*(?:video|stream|cdn)[^"\']*)["\']',
        r'sources\s*:\s*\[([^\]]+)\]',
        r'jwplayer\s*\([^)]*\)\.setup\s*\((.*?)\)',
    ]
    
    for pattern in patterns:
        matches = re.findall(pattern, html_content, re.IGNORECASE)
        for match in matches:
            nested = [match]
            if "sources" in pattern or "jwplayer" in pattern:
                nested = re.findall(r'(?:file|src|url|source)\s*:\s*["\']([^"\']+)["\']', match, re.IGNORECASE)
                nested.extend(re.findall(r'(https?:\\?/\\?/[^"\'<>\s]+?(?:\.mp4|\.m3u8|\.mpd)(?:\?[^"\'<>\s]*)?)', match, re.IGNORECASE))
            for raw_url in nested:
                candidate = _create_candidate(raw_url, base_url, "freekino")
                if candidate:
                    candidates.append(candidate)
    
    return candidates


# ============================================================================
# MAIN EXTRACTION FUNCTION WITH SOURCE AWARENESS
# ============================================================================

def extract_media_for_source(
    html_content: str,
    base_url: str,
    source: str,
    title: str = ""
) -> MediaExtractionResult:
    """
    Extract media with source-specific optimizations.
    
    Args:
        html_content: Raw HTML content
        base_url: Page URL
        source: Source name (uzmovi, asilmedia, freekino)
        title: Movie title for logging
        
    Returns:
        MediaExtractionResult
    """
    result = MediaExtractionResult(
        success=False,
        source=source,
        title=title,
    )
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Source-specific extraction for: {source}")
    
    # Parse HTML
    try:
        soup = BeautifulSoup(html_content, "lxml")
    except Exception as e:
        result.error = f"Failed to parse HTML: {str(e)}"
        return result
    
    # Source-specific extraction
    if source == "uzmovi":
        candidates = extract_from_uzmovi(soup, base_url)
    elif source == "asilmedia":
        candidates = extract_from_asilmedia(soup, base_url)
    elif source == "freekino":
        candidates = extract_from_freekino(soup, base_url)
    else:
        # Generic extraction
        candidates = extract_media_candidates(html_content, base_url)
    
    result.candidates_found = len(candidates)
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] Found {len(candidates)} candidates for {source}")
    
    # Validate
    valid_candidates, rejected_candidates = validate_candidates(candidates)
    result.candidates_rejected = len(rejected_candidates)
    
    if not valid_candidates:
        result.error = "Could not extract valid media URL"
        return result
    
    # Select best
    best_candidate = choose_best_media_candidate(valid_candidates)
    
    if not best_candidate:
        result.error = "Could not select valid media URL"
        return result
    
    result.success = True
    result.video_url = best_candidate.url
    result.video_url_type = best_candidate.type
    result.headers = best_candidate.headers
    
    if DEBUG:
        logger.info(f"[MEDIA_EXTRACTOR] SUCCESS: {result.video_url_type} - {result.video_url[:60]}...")
    
    return result


# ============================================================================
# LEGACY COMPATIBILITY FUNCTIONS
# ============================================================================

def isValidStreamUrl(url: str) -> bool:
    """
    Legacy compatibility function.
    
    Delegates to is_valid_media_url.
    """
    is_valid, _ = is_valid_media_url(url)
    return is_valid


def select_best_stream_url(urls: List[Dict[str, str]]) -> Optional[Dict[str, str]]:
    """
    Legacy compatibility function.
    
    Converts old format to MediaCandidate, processes, returns old format.
    """
    if not urls:
        return None
    
    # Convert to candidates
    candidates = []
    for url_info in urls:
        url = url_info.get("url", "")
        if url:
            candidate = _create_candidate(url, "", "legacy")
            if candidate:
                candidates.append(candidate)
    
    # Validate
    valid, _ = validate_candidates(candidates)
    
    if not valid:
        return None
    
    # Select best
    best = choose_best_media_candidate(valid)
    
    if best:
        return {
            "url": best.url,
            "type": best.type,
            "quality": best.quality,
        }

    return None


# ============================================================================
# SHARED IFRAME RESOLVER (kinolar / kinochilar / uzmedia / generic embeds)
# ============================================================================

def resolve_embed_to_candidates(
    iframe_url: str,
    referer: str = "",
    max_depth: int = 2,
    session: requests.Session = None,
) -> List[Dict[str, Any]]:
    """
    Fetch an embed iframe URL and return a list of resolved playable video
    candidates in parser format ({"url", "type", "quality"}).

    Recursively follows nested iframes up to `max_depth` levels — many uCoz/DLE
    sites point at an outer player page that itself iframes the real CDN host.

    Always returns a list (possibly empty). Never raises.
    """
    if not iframe_url:
        return []

    headers = {
        "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 "
                      "(KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
        "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
    }
    if referer:
        headers["Referer"] = referer

    visited = set()
    out: List[Dict[str, Any]] = []

    def _walk(url: str, ref: str, depth: int) -> None:
        if depth < 0 or not url or url in visited:
            return
        visited.add(url)
        try:
            getter = session.get if session else requests.get
            resp = getter(
                url,
                headers={**headers, "Referer": ref or referer or url},
                timeout=20,
                allow_redirects=True,
                verify=False,
            )
        except Exception as e:
            logger.warning(f"[IFRAME_RESOLVER] fetch failed {url}: {e}")
            return

        final_url = str(resp.url)
        content_type = resp.headers.get("Content-Type", "").lower()

        # Direct media response (CDN redirected the embed straight to a file)
        if any(t in content_type for t in ("video/", "application/vnd.apple.mpegurl",
                                            "application/dash+xml", "application/octet-stream")):
            out.append({
                "url": final_url,
                "type": classify_media_url(final_url),
                "quality": "auto",
                "headers": {"Referer": ref or referer or url},
            })
            return

        if "text/html" not in content_type and not resp.text:
            return

        html = resp.text or ""
        # Extract candidates from this embed page
        candidates = extract_media_candidates(html, final_url, referer=ref)
        valid, _ = validate_candidates(candidates)
        for c in valid:
            out.append({
                "url": c.url,
                "type": c.type if c.type in ("m3u8", "mpd", "mp4", "ism") else classify_media_url(c.url),
                "quality": c.quality or "auto",
                "headers": {"Referer": final_url},
            })

        # Look for nested iframes and follow them
        if depth > 0:
            try:
                soup = BeautifulSoup(html, "lxml")
                for nested in soup.find_all("iframe"):
                    src = (nested.get("src") or "").strip()
                    if not src:
                        continue
                    if src.startswith("//"):
                        src = "https:" + src
                    if src.startswith("/"):
                        src = urljoin(final_url, src)
                    if src.startswith(("http://", "https://")) and src not in visited:
                        _walk(src, final_url, depth - 1)
            except Exception as e:
                logger.warning(f"[IFRAME_RESOLVER] nested-iframe parse failed: {e}")

    if iframe_url.startswith("//"):
        iframe_url = "https:" + iframe_url
    _walk(iframe_url, referer, max_depth)

    # Deduplicate by URL, keep first (highest priority) occurrence
    seen = set()
    deduped: List[Dict[str, Any]] = []
    for c in out:
        u = c.get("url", "")
        if u and u not in seen:
            seen.add(u)
            deduped.append(c)

    # Filter out anything that still looks like an HTML page
    final: List[Dict[str, Any]] = []
    for c in deduped:
        ok, _reason = is_valid_media_url(c["url"])
        if ok and c.get("type") not in (None, "", "html", "unknown"):
            final.append(c)
        elif ok and c.get("type") in (None, "", "unknown"):
            # Last-resort: keep as unknown — downloader will probe content-type
            final.append(c)
    return final
