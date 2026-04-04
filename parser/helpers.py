"""
Helper utilities for parsing
"""
import re
import os
from typing import List, Dict, Any, Optional
from urllib.parse import urljoin, urlparse


def normalize_url(url: str, base_url: str = "") -> str:
    """Normalize URL - make relative URLs absolute"""
    if not url:
        return ""
    
    url = url.strip()
    
    if url.startswith(("http://", "https://", "//")):
        if url.startswith("//"):
            return "https:" + url
        return url
    
    if base_url:
        return urljoin(base_url, url)
    
    return url


def clean_text(text: Optional[str]) -> str:
    """Clean text - remove extra whitespace, newlines"""
    if not text:
        return ""
    
    text = re.sub(r'\s+', ' ', text)
    text = text.strip()
    
    return text


def extract_year(text: str) -> Optional[int]:
    """Extract year from text"""
    if not text:
        return None
    
    match = re.search(r'\b(19|20)\d{2}\b', text)
    if match:
        return int(match.group())
    
    return None


def sanitize_filename(title: str) -> str:
    """
    Convert movie title to safe filename slug.
    
    Examples:
        "Forsaj 8" -> "forsaj8"
        "Spider-Man: No Way Home" -> "spidermannowayhome"
        "Harry Potter & The Sorcerer's Stone" -> "harrypotterthesorcerersstone"
    
    Rules:
        - lowercase
        - remove special characters (keep alphanumeric only)
        - remove extra spaces
        - concatenate words (no separators)
    """
    if not title:
        return ""
    
    # Convert to lowercase
    slug = title.lower()
    
    # Replace colons, hyphens, ampersands, apostrophes with spaces first
    slug = re.sub(r'[:\-&\'"!@#$%^*()+\[\]{}|\\;,./<>?]', ' ', slug)
    
    # Remove non-alphanumeric characters (keep spaces)
    slug = re.sub(r'[^a-z0-9\s]', '', slug)
    
    # Replace multiple spaces with single space
    slug = re.sub(r'\s+', ' ', slug)
    
    # Remove spaces entirely (concatenate words)
    slug = slug.replace(' ', '')
    
    return slug.strip()


def get_download_filename(title: str) -> str:
    """
    Generate the output filename for parser download.
    
    Returns: "<slug>_downloaded.mp4"
    """
    slug = sanitize_filename(title)
    if not slug:
        slug = "video"
    return f"{slug}_downloaded.mp4"


def get_upload_directory(title: str) -> str:
    """
    Generate the output directory for worker HLS upload.
    
    Returns: "<slug>_uploaded_ready"
    """
    slug = sanitize_filename(title)
    if not slug:
        slug = "video"
    return f"{slug}_uploaded_ready"


def extract_duration(text: str) -> Optional[int]:
    """Extract duration in minutes from text"""
    if not text:
        return None
    
    match = re.search(r'(\d+)\s*(?:min|m|dakika|minutes?)', text, re.IGNORECASE)
    if match:
        return int(match.group(1))
    
    match = re.search(r'\b(\d{2,3})\b', text)
    if match:
        val = int(match.group(1))
        if 1 <= val <= 300:
            return val
    
    return None


def deduplicate_results(results: List[Dict[str, Any]], key: str = "link") -> List[Dict[str, Any]]:
    """Deduplicate results by key"""
    seen = set()
    unique = []
    
    for result in results:
        value = result.get(key, "")
        if value and value not in seen:
            seen.add(value)
            unique.append(result)
    
    return unique


def normalize_for_match(text: str) -> str:
    """Normalize text for matching - lowercase, trim, collapse whitespace"""
    if not text:
        return ""
    return clean_text(text).lower()


def filter_and_rank_results(
    results: List[Dict[str, Any]], 
    query: str,
    title_key: str = "title",
    link_key: str = "link"
) -> List[Dict[str, Any]]:
    """Filter results by query relevance and rank them"""
    if not results or not query:
        return results
    
    query_normalized = normalize_for_match(query)
    query_words = query_normalized.split()
    
    # Filter out empty query words
    query_words = [w for w in query_words if len(w) >= 2]
    
    if not query_words:
        return results
    
    ranked = []
    
    for result in results:
        title = result.get(title_key, "")
        if not title:
            continue
        
        title_normalized = normalize_for_match(title)
        
        # Calculate relevance score
        score = 0
        
        # Exact match
        if query_normalized in title_normalized:
            score += 100
        # All words match
        elif all(word in title_normalized for word in query_words):
            score += 50
        # Some words match
        else:
            matching_words = sum(1 for word in query_words if word in title_normalized)
            score += matching_words * 10
        
        if score > 0:
            result["_relevance_score"] = score
            ranked.append(result)
    
    # Sort by relevance score descending
    ranked.sort(key=lambda x: x.get("_relevance_score", 0), reverse=True)
    
    # Remove internal score key
    for result in ranked:
        result.pop("_relevance_score", None)
    
    return ranked


def extract_source_id(url: str) -> str:
    """Extract source ID from URL"""
    if not url:
        return ""
    
    # Standard patterns
    for pattern in ["/film/", "/movie/", "/serial/", "/kinolar/", "/multfilmlar/"]:
        if pattern in url:
            parts = url.split(pattern)
            if len(parts) > 1:
                id_part = parts[1].split("/")[0].split("?")[0].split("#")[0]
                if id_part:
                    return id_part
    
    # uzmovi.tv pattern: /category/ID-title.html
    # e.g., /tarjima-kinolarri/5689-alfa-test-premyera.html
    match = re.search(r'/([a-zA-Z_-]+)/(\d+)-[\w-]+\.html', url)
    if match:
        category = match.group(1)
        numeric_id = match.group(2)
        # Only match if category doesn't look like a title
        if numeric_id and len(numeric_id) >= 3:
            return numeric_id
    
    # asilmedia.org pattern: /ID-title.html (numeric ID at start before hyphen)
    # e.g., /9140-interstellar-uzbek-tarjima.html
    match = re.search(r'/(\d+)-[\w-]+\.html', url)
    if match:
        return match.group(1)
    
    # Generic numeric ID before .html
    # e.g., /something/12345.html
    match = re.search(r'/(\d+)\.html', url)
    if match:
        return match.group(1)
    
    return ""


def create_debug_summary(
    query: str,
    search_url: str,
    status_code: int,
    page_title: str,
    html_length: int,
    matched_cards: int,
    raw_titles: List[str],
    raw_links: List[str],
    before_filter: int,
    after_filter: int,
    after_dedup: int,
    final_count: int,
    error: str = None
) -> Dict[str, Any]:
    """Create structured debug summary"""
    return {
        "query": query,
        "search_url": search_url,
        "status_code": status_code,
        "page_title": page_title,
        "html_length": html_length,
        "matched_cards": matched_cards,
        "raw_titles": raw_titles[:10],  # Limit to 10
        "raw_links": raw_links[:10],
        "before_filter": before_filter,
        "after_filter": after_filter,
        "after_dedup": after_dedup,
        "final_count": final_count,
        "error": error
    }


def isValidStreamUrl(url: str) -> bool:
    """
    Validate if a URL is a valid media stream URL (not an HTML page).
    
    This function checks URLs to ensure they are actual media URLs that
    can be used with N_m3u8DL-RE or other video downloaders.
    
    ACCEPT URLs containing:
    - ".m3u8" (HLS manifest)
    - ".mpd" (DASH manifest)
    - ".ism" (ISM manifest)
    - ".mp4" (direct video)
    - Known CDN patterns for video delivery (including uzmovi's srv*.uzdown.space)
    - Video hosting domains
    
    REJECT URLs containing:
    - ".html" at the end (HTML page URLs)
    - Page routes without video indicators (but be lenient with video sites)
    
    FIXED: Made validation less aggressive to accept valid uzmovi video URLs.
    
    Args:
        url: The URL to validate
        
    Returns:
        True if URL is a valid stream URL, False otherwise
    """
    if not url:
        return False
    
    url_lower = url.lower()
    
    # === REJECT: HTML files at the end of URL ===
    # Be careful not to reject .mp4.html style CDN URLs
    if url_lower.endswith(".html") or url_lower.endswith(".htm"):
        return False
    
    # === ACCEPT: Direct media extensions (HIGHEST PRIORITY) ===
    
    if ".m3u8" in url_lower:
        return True
    
    if ".mpd" in url_lower:
        return True
    
    if ".ism" in url_lower or ".ismc" in url_lower:
        return True
    
    if ".mp4" in url_lower:
        return True
    
    # === CRITICAL FIX: uzmovi-specific patterns (HIGHEST PRIORITY FOR UZMOVI) ===
    # uzmovi uses srv*.uzdown.space for video hosting - accept any URL from these domains
    uzmovi_domains = [
        "srv",  # srv*.uzdown.space - uzmovi's main video CDN
        "uzdown",  # Fallback for any uzdown URLs
    ]
    
    for domain in uzmovi_domains:
        if domain in url_lower:
            # Check if it has a proper URL structure (has slashes for path)
            if "/" in url and len(url) > 20:
                # Accept any URL from uzmovi domains with a path - they are likely video URLs
                return True
    
    # === ACCEPT: Known video CDN patterns with indicators ===
    
    video_cdn_patterns = [
        # Domain + required indicators
        # uzmovi-specific patterns: accept "live/" directory and various path patterns
        ("uzdown", ["index", "playlist", "video", "movie", "clip", "mp4", "m3u8", "mpd", "file", "play", "live", "mob", "hd", "sd", "content", "storage", "static", "assets", "download", "get", "v"]),
        # Accept any URL from srv*.uzdown.space as valid (uzmovi uses this pattern)
        # CRITICAL FIX: Added more flexible path patterns to accept various uzmovi URL structures
        ("srv", ["index", "playlist", "video", "movie", "clip", "mp4", "m3u8", "mpd", "file", "play", "live", "mob", "hd", "sd", "uzmovi", "content", "storage", "static", "assets", "download", "get", "v"]),
        ("sukit", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        ("cdn.", ["mp4", "m3u8", "mpd", "video", "media", "file"]),
        ("video.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        ("stream.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        ("media.", ["mp4", "m3u8", "mpd", "index", "playlist", "file"]),
        # Uzbek/Kazakh/Russian video hosting patterns
        (".uz", ["mp4", "m3u8", "mpd", "video", "file", "index", "playlist"]),
        (".tk", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        (".ru", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        # Free movie site patterns
        ("uzmovi", ["mp4", "m3u8", "mpd", "video", "file", "index", "live"]),
        ("freekino", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        ("asilmedia", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
        ("kinolar", ["mp4", "m3u8", "mpd", "video", "file", "index"]),
    ]
    
    for domain, indicators in video_cdn_patterns:
        if domain in url_lower:
            if any(ind in url_lower for ind in indicators):
                return True
    
    # === ACCEPT: Manifest path patterns ===
    
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
            return True
    
    # === FINAL CHECK: If URL has a domain and looks like video hosting ===
    
    # Check for common video hosting domains
    video_hosting_domains = [
        "uzmovi", "uzmov", "freekino", "asilmedia", "kinolar",
        "sukit", "uzbek", "tork", "kino",
        "cdn", "video", "stream", "media", "asset"
    ]
    
    if any(domain in url_lower for domain in video_hosting_domains):
        # If it has a proper TLD and path, it might be valid
        if url_lower.startswith("http") and "/" in url:
            # Check if it looks like a media URL (has path segments)
            path_segments = url_lower.split("/")
            if len(path_segments) > 3:  # Has subdirectory, likely not homepage
                return True
    
    # === DEFAULT: REJECT ===
    # If URL doesn't match any valid pattern, reject it
    return False


def select_best_stream_url(urls: List[Dict[str, str]]) -> Optional[Dict[str, str]]:
    """
    Select the best stream URL from a list of candidates.
    
    Prioritization:
    1. m3u8 (HLS manifests - most common for streaming)
    2. mpd (DASH manifests)
    3. ism (ISM manifests)
    4. mp4 (direct video files)
    
    For URLs without extensions, prefer CDN-hosted URLs.
    
    Args:
        urls: List of URL dictionaries with 'url' and optionally 'type' and 'quality' keys
        
    Returns:
        The best URL dictionary, or None if no valid URLs found
    """
    if not urls:
        return None
    
    valid_urls = []
    invalid_urls = []
    
    for url_info in urls:
        url = url_info.get("url", "")
        
        if isValidStreamUrl(url):
            valid_urls.append(url_info)
        else:
            invalid_urls.append(url_info)
    
    # Log rejected URLs
    import logging
    logger = logging.getLogger(__name__)
    for url_info in invalid_urls:
        logger.info(f"[URL_VALIDATION] Rejected invalid URL: {url_info.get('url', '')[:80]}... (type: {url_info.get('type', 'unknown')})")
    
    if not valid_urls:
        return None
    
    # Prioritize by URL type
    def url_priority(url_info: Dict[str, str]) -> int:
        """Return priority score (higher is better)"""
        url = url_info.get("url", "").lower()
        url_type = url_info.get("type", "").lower()
        
        # m3u8 is highest priority (HLS)
        if ".m3u8" in url:
            return 100
        if "m3u8" in url_type:
            return 100
        
        # mpd is second priority (DASH)
        if ".mpd" in url:
            return 90
        if "mpd" in url_type:
            return 90
        
        # ism is third priority
        if ".ism" in url:
            return 80
        if "ism" in url_type:
            return 80
        
        # mp4 direct video
        if ".mp4" in url:
            return 70
        if "mp4" in url_type:
            return 70
        
        # CDN-hosted URLs without extension (might be HLS/DASH)
        cdn_patterns = ["cdn.", "video.", "stream.", "uzdown", "srv"]
        if any(pattern in url for pattern in cdn_patterns):
            return 60
        
        return 50
    
    # Sort by priority and return the best one
    valid_urls.sort(key=url_priority, reverse=True)
    
    best_url = valid_urls[0]
    import logging
    logger = logging.getLogger(__name__)
    logger.info(f"[URL_VALIDATION] Selected media URL: {best_url.get('url', '')[:80]}... (type: {best_url.get('type', 'unknown')}, priority: {url_priority(best_url)})")
    
    return best_url
