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


def detect_content_type(url: str, source: str, soup=None) -> tuple:
    """Detect movie vs serial.

    Returns: (content_type, reason) where content_type is "movie" | "serial" | "unknown"
    and reason is a short string explaining the decision (used in logs).

    Priority (soup signals beat URL — URLs lie, especially Asilmedia's generic
    /films/<category>/ which buckets serials under a film-y looking path):
      1) Strong soup signals — single hit on an explicit serial keyword
         ("fasl", "mavsum", "barcha qismlar", "N-qism", "N-fasl", "season N",
         "episode N", "сериал", ...) OR a season/episode DOM block.
      2) URL pattern (only used when soup gave nothing).

    Returning "unknown" is intentional — the caller (parser card extractor or
    backend /import handler) can decide to show "Aniqlanmoqda" in the badge or
    re-fetch /details before committing to a movie/serial pipeline.
    """
    import re as _re

    u = (url or "").lower()
    src = (source or "").lower()

    def _scope_asilmedia_soup(node):
        if node is None:
            return None
        try:
            if getattr(node, "get", None):
                itemtype = (node.get("itemtype") or "").lower()
                classes = " ".join(node.get("class", []) if node.get("class") else []).lower()
                if "schema.org/movie" in itemtype or "schema.org/tvseries" in itemtype or "fullstory" in classes:
                    return node
            scoped = node.select_one("article.fullstory, .fullstory, article[itemtype*='schema.org/Movie'], article[itemtype*='schema.org/TVSeries']")
            return scoped or node
        except Exception:
            return node

    def _is_asilmedia_detail_root(node):
        if node is None or not getattr(node, "get", None):
            return False
        try:
            itemtype = (node.get("itemtype") or "").lower()
            classes = " ".join(node.get("class", []) if node.get("class") else []).lower()
            return (
                "schema.org/movie" in itemtype or
                "schema.org/tvseries" in itemtype or
                "fullstory" in classes
            )
        except Exception:
            return False

    def _find_asilmedia_episode_section(node):
        if node is None:
            return None
        try:
            for sel in (".fs-episodes", "#episodes-section", "#episodes-raw-data"):
                for candidate in node.select(sel):
                    text = clean_text(candidate.get_text(" ", strip=True)).lower()
                    if "qismlar" in text:
                        return candidate
            for candidate in node.find_all(["section", "div", "article"]):
                text = clean_text(candidate.get_text(" ", strip=True)).lower()
                if "qismlar" in text:
                    return candidate
        except Exception:
            return None
        return None

    def _asilmedia_has_real_episode_controls(section):
        if section is None:
            return (False, "no Qismlar section found")
        ignore_words = ("360p", "480p", "720p", "1080p", "yuklab olish", "onlayn ko'rish", "skrinshotlar")
        episode_re = _re.compile(r"^\d+\s*-\s*qism$", _re.IGNORECASE)
        season_re = _re.compile(r"^\d+\s*-\s*fasl$", _re.IGNORECASE)
        try:
            for node in section.find_all(["a", "button"]):
                label_parts = [
                    node.get_text(" ", strip=True),
                    node.get("title", ""),
                    node.get("data-label", ""),
                    node.get("onclick", ""),
                ]
                for raw_label in label_parts:
                    if not raw_label:
                        continue
                    label = clean_text(raw_label).lower()
                    if not label or any(word in label for word in ignore_words):
                        continue
                    if episode_re.fullmatch(label):
                        return (True, f"Qismlar section has episode button {label!r}")
                    if season_re.fullmatch(label):
                        return (True, f"Qismlar section has season button {label!r}")
        except Exception:
            return (False, "failed to inspect Qismlar section")
        return (False, "Qismlar section has no real episode controls")

    # ── Soup signals first ────────────────────────────────────────────────
    if soup is not None:
        analysis_root = _scope_asilmedia_soup(soup) if src == "asilmedia" else soup
        try:
            text_lower = analysis_root.get_text(" ", strip=True).lower() if analysis_root else ""
        except Exception:
            text_lower = ""

        if src == "asilmedia" and analysis_root is not None:
            episode_section = _find_asilmedia_episode_section(analysis_root)
            has_controls, reason = _asilmedia_has_real_episode_controls(episode_section)
            if has_controls:
                return ("serial", reason)

            if _is_asilmedia_detail_root(analysis_root):
                return ("movie", reason)

        if text_lower:
            # Strong single-hit serial keywords. Hitting any of these means
            # the page is talking about episodes/seasons explicitly.
            strong_keywords = (
                "barcha qismlar", "barcha qismlari",
                "qismlardan tanlash", "qismlar to'liq", "qismlar to`liq",
                "fasl", "mavsum",            # uz "season"
                "сезон", "сериал", "серии",  # ru
                "seriallar",
                "season ", "episode ", "episodes ",
            )
            hit = next((kw for kw in strong_keywords if kw in text_lower), None)
            if hit:
                return ("serial", f"page text strong serial keyword: {hit!r}")

            # "1-qism", "2-qism", "1-fasl", "1-mavsum", "season 2", "episode 7"
            episode_patterns = (
                r'\b\d+\s*-\s*qism\b',
                r'\b\d+\s*-\s*fasl\b',
                r'\b\d+\s*-\s*mavsum\b',
                r'\bseason\s+\d+\b',
                r'\bepisode\s+\d+\b',
                r'\bseriya\s+\d+\b',
                r'\bсерия\s+\d+\b',
            )
            for pat in episode_patterns:
                if _re.search(pat, text_lower):
                    return ("serial", f"page text matches episode pattern: {pat}")

            # Weak keyword — needs at least 2 distinct hits to count.
            weak_keywords = ("qism", "qismi", "qisim", "epizod")
            weak_hits = [k for k in weak_keywords if k in text_lower]
            if len(weak_hits) >= 2:
                return ("serial", f"page text weak serial signals: {weak_hits[:3]}")

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

    # ── URL fallback ──────────────────────────────────────────────────────
    if src == "uzmovi":
        if any(seg in u for seg in (
            "/serialar/", "/seriallar/", "/serial/", "/uzbek-serial",
            "/turk-serial", "/korea-serial", "/koreya-serial", "/hind-serial",
            "/yapon-serial", "/multserial",
        )):
            return ("serial", "uzmovi url path matches serial segment")
        if any(seg in u for seg in (
            "/tarjima-kinolar/", "/tarjima-kino", "/kinolar/", "/film/",
            "/uzbek-kino", "/hind-kino", "/turk-kino", "/multfilm",
        )):
            return ("movie", "uzmovi url path matches movie segment")
        return ("unknown", "uzmovi url path did not match")

    if src == "freekino":
        if "/serial/" in u or "/seriallar" in u:
            return ("serial", "freekino url contains /serial/")
        if "/movie/" in u or "/film/" in u:
            return ("movie", "freekino url contains /movie/ or /film/")
        return ("unknown", "freekino url path did not match")

    if src == "asilmedia":
        if any(seg in u for seg in (
            "/seriallar/", "/serial/", "/uzbek-seriallar", "/hind-seriallar",
            "/turk-seriallar", "/korea-seriallar", "/multseriallar",
        )):
            return ("serial", "asilmedia url path contains serial category")
        # Asilmedia bundles serials under /films/<sub>/ too — never trust the
        # film-y URL when soup gave us no signal. Force "unknown" so callers
        # re-check via /details rather than mis-importing as a movie.
        if any(seg in u for seg in (
            "/kinolar/", "/film/", "/films/", "/multfilmlar/",
            "/uzbek-kinolari", "/hind-kinolari", "/turk-kinolari",
        )):
            if soup is None:
                return ("unknown", "asilmedia film-category url, no soup to confirm — defer to detail")
            return ("movie", "asilmedia url film-category and no serial signal in soup")

    return ("unknown", "no url or page signal matched")


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
