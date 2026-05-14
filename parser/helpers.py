"""
Helper utilities for parsing
"""
import re
import os
from difflib import SequenceMatcher
from typing import List, Dict, Any, Optional, Tuple
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


def normalize_identity_text(text: str) -> str:
    """Normalize human-readable identity text for fuzzy equality checks."""
    if not text:
        return ""
    text = clean_text(text).lower()
    text = re.sub(r"[\"'`’“”.,:;!?()\[\]{}_/\\|-]+", " ", text)
    text = re.sub(r"\b(uzbekcha|tarjima|tarjimaasi|o'zbek tilida|ozbek tilida|barcha qismlar|premyera)\b", " ", text)
    text = re.sub(r"\s+", " ", text).strip()
    return text


def title_similarity(a: str, b: str) -> float:
    """Return a 0..1 similarity score for two titles."""
    a_norm = normalize_identity_text(a)
    b_norm = normalize_identity_text(b)
    if not a_norm or not b_norm:
        return 0.0
    if a_norm == b_norm:
        return 1.0
    a_tokens = set(a_norm.split())
    b_tokens = set(b_norm.split())
    token_jaccard = len(a_tokens & b_tokens) / max(1, len(a_tokens | b_tokens))
    sequence = SequenceMatcher(None, a_norm, b_norm).ratio()
    return max(sequence, token_jaccard)


def normalize_quality_label(label: str) -> str:
    if not label:
        return "unknown"
    raw = clean_text(label).lower()
    if raw in ("original", "source", "auto"):
        return raw
    if "full hd" in raw or "fhd" in raw:
        return "1080p"
    if raw == "hd":
        return "720p"
    if raw == "sd":
        return "480p"
    match = re.search(r"(2160|1440|1080|720|480|360|240)", raw)
    if match:
        return f"{match.group(1)}p"
    return raw


def quality_height(label: str, url: str = "") -> int:
    label = normalize_quality_label(label)
    if label == "original":
        return 10000
    match = re.search(r"(2160|1440|1080|720|480|360|240)", label)
    if match:
        return int(match.group(1))
    match = re.search(r"(2160|1440|1080|720|480|360|240)", (url or "").lower())
    if match:
        return int(match.group(1))
    return 0


def score_video_candidate(candidate: Dict[str, Any]) -> Tuple[int, int, float, int]:
    """Score a parsed video candidate. Higher is better."""
    url = candidate.get("url", "")
    parsed_type = (candidate.get("type") or "").lower()
    quality = candidate.get("quality", "")
    height = int(candidate.get("height") or quality_height(quality, url))
    type_priority = {
        "m3u8": 40,
        "hls": 40,
        "mpd": 35,
        "ism": 30,
        "mp4": 25,
        "direct_mp4": 25,
        "direct_download": 20,
        "unknown": 10,
    }
    confidence = float(candidate.get("confidence") or 0)
    return (
        height,
        type_priority.get(parsed_type, 0),
        confidence,
        -len(url or ""),
    )


def sort_video_candidates(candidates: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
    ordered = []
    for candidate in candidates or []:
        item = dict(candidate)
        item["quality"] = normalize_quality_label(item.get("quality", ""))
        item["height"] = int(item.get("height") or quality_height(item.get("quality", ""), item.get("url", "")))
        ordered.append(item)
    ordered.sort(key=score_video_candidate, reverse=True)
    return ordered


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
    for pattern in ["/film/", "/movie/", "/serial/", "/kinolar/", "/multfilmlar/", "/serie/", "/episode/"]:
        if pattern in url:
            parts = url.split(pattern)
            if len(parts) > 1:
                id_part = parts[1].split("/")[0].split("?")[0].split("#")[0]
                # If it's something like "12345-slug", take only the numeric part
                id_match = re.match(r'^(\d+)', id_part)
                if id_match:
                    return id_match.group(1)
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


def canonical_episode_id(parent_id: str, season: int, episode: int) -> str:
    """Build canonical source_id for an episode: parentID:sXXeXX"""
    if not parent_id:
        return ""
    return f"{parent_id}:s{season:02d}e{episode:02d}"


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


def detect_content_type(url: str, source: str, soup=None) -> tuple:
    """Detect movie vs serial.

    Returns: (content_type, reason) where content_type is "movie" | "serial" | "unknown"
    and reason is a short string explaining the decision (used in logs).
    """
    import re as _re

    u = (url or "").lower()
    src = (source or "").lower()

    # 1. Check URL first for obvious indicators
    if any(p in u for p in ["/serial/", "/seriya/", "/qism/", "/fasl/", "/mavsum/", "/serie/", "/episode/"]):
        return ("serial", "URL pattern match")

    # ── Soup signals ────────────────────────────────────────────────
    if soup is not None:
        # Try to find the main content area to avoid menu/footer noise
        main_content = None
        for sel in [
            "article", ".full-story", ".fullstory", ".movie-detail", 
            ".serial-detail", "#dle-content", ".content-main", "main",
            ".w-full.flex-col", # freekino specific
        ]:
            main_content = soup.select_one(sel)
            if main_content:
                break
        
        analysis_root = main_content or soup
        
        try:
            text_lower = analysis_root.get_text(" ", strip=True).lower() if analysis_root else ""
        except Exception:
            text_lower = ""

        # Check for serial-specific metadata/badges
        serial_badges = analysis_root.select(".badge--series, .serial-badge, .fasl-badge, .qism-badge")
        if serial_badges:
            return ("serial", "Found serial-specific badge")

        # Check for season/episode controls (tabs, dropdowns)
        serial_controls = analysis_root.select(".episodes-tabs, .season-select, .episode-list, #episodes-section")
        if serial_controls:
            return ("serial", "Found episode/season controls")

        if text_lower:
            # More specific serial patterns
            serial_patterns = [
                r'\b\d+\s*-\s*qism\b',
                r'\b\d+\s*-\s*fasl\b',
                r'\b\d+\s*-\s*mavsum\b',
                r'barcha qismlari',
                r'barcha qismlar',
            ]
            
            # Check title specifically
            title_node = analysis_root.select_one("h1, .title, .film-title")
            title_text = title_node.get_text().lower() if title_node else ""
            
            if any(kw in title_text for kw in ["serial", "сериал", "barcha qismlari"]):
                return ("serial", f"Title contains serial keyword")

            # Check for episode/season patterns in text
            for pattern in serial_patterns:
                if _re.search(pattern, text_lower):
                    return ("serial", f"Found serial pattern in content: {pattern}")

            # Fallback to general keywords but with cautious list
            strong_keywords = (
                "qismlardan tanlash", "qismlar to'liq", "qismlar to`liq",
                "mavsum", "сезон", "серии",
                "season ", "episode ", "episodes ",
            )
            for kw in strong_keywords:
                if kw in text_lower:
                    return ("serial", f"Found serial keyword in content: {kw}")

        # DOM blocks that almost certainly mean season/episode UI.
        dom_selectors = (
            ".series-list", ".episodes", ".episode-list",
            ".seasons", ".season-list", ".season-tabs",
            "[class*='episode']", "[class*='season']", "[class*='qismlar']",
            "[class*='fasl']", "ul.serii", "ol.episodes",
            ".fs-episodes", ".fs-poster__serial-badge", ".badge--series",
            "[itemtype*='TVSeries']",
            "a[href*='/episode/']", "a[title*='-qism']", "a[title*='-fasl']",
            ".batcoh-list", ".batcoh-item",
        )
        for sel in dom_selectors:
            try:
                if analysis_root.select_one(sel):
                    return ("serial", f"page has DOM block matching {sel}")
            except Exception:
                pass

        # Asilmedia specific: check for "Qismlar" section but verify it has real controls
        if src == "asilmedia":
            qismlar_found = False
            for sel in [".fs-episodes", "#episodes-section", "#episodes-raw-data"]:
                if analysis_root.select_one(sel):
                    qismlar_found = True
                    break
            if not qismlar_found:
                # If we are on a detail page but no episodes section, it's a movie
                if _re.search(r'/\d+-', u) or _re.search(r'/\d+\.html', u):
                    return ("movie", "no Qismlar section found")

        # Check for Movie indicators
        if analysis_root:
            if getattr(analysis_root, "get", None):
                itemtype = (analysis_root.get("itemtype") or "").lower()
                classes = " ".join(analysis_root.get("class", []) if analysis_root.get("class") else []).lower()
                if "schema.org/movie" in itemtype or "fullstory" in classes:
                    return ("movie", "Found movie-specific indicator (itemtype or class)")

    # ── URL fallback ──────────────────────────────────────────────────────
    if src == "uzmovi":
        if any(seg in u for seg in (
            "/serialar/", "/seriallar/", "/serial/", "/uzbek-serial",
            "/turk-serial", "/korea-serial", "/koreya-serial", "/hind-serial",
            "/yapon-serial", "/multserial",
        )):
            return ("serial", "uzmovi url path matches serial segment")
    
    if src == "freekino":
        if "/serial/" in u or "/seriallar" in u:
            return ("serial", "freekino url contains /serial/")
        if "/movie/" in u or "/film/" in u:
            return ("movie", "freekino url contains /movie/ or /film/")

    if src == "asilmedia":
        if any(seg in u for seg in (
            "/seriallar/", "/serial/", "/uzbek-seriallar", "/hind-seriallar",
            "/turk-seriallar", "/korea-seriallar", "/multseriallar",
        )):
            return ("serial", "asilmedia url path contains serial category")

    if "/movie" in u or "/film" in u or "/kino" in u or "/tarjima-kinolar" in u:
        return ("movie", "URL fallback")

    # If we are on a detail page (numeric ID in URL) and no serial indicators were found, it's likely a movie
    if _re.search(r'/\d+-', u) or _re.search(r'/\d+\.html', u):
        return ("movie", "Detail page without serial indicators")

    return ("unknown", "Could not confidently detect type")



def is_youtube_url(url: str) -> bool:
    """Return True if url is a YouTube watch/shorts/youtu.be link."""
    if not url:
        return False
    u = url.lower()
    return (
        "youtube.com/watch" in u
        or "youtube.com/shorts/" in u
        or "youtu.be/" in u
    )


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

    # YouTube URLs are handled by yt-dlp — always valid
    if is_youtube_url(url):
        return True

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
        ("video-cdn.org", ["mp4", "m3u8", "mpd", "video", "file"]),
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
