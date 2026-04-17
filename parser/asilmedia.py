"""
Asilmedia (asilmedia.org) Parser
DataLife Engine (DLE) based website - uses POST form submission for search
"""
import logging
import os
import re
import requests
from typing import List, Optional, Dict, Any
from urllib.parse import urljoin, urlparse, parse_qs
from bs4 import BeautifulSoup

from base_parser import BaseParser, SearchResult, MovieDetails
from source_config import get_source_config
from helpers import (
    normalize_url, 
    clean_text, 
    extract_year, 
    extract_duration,
    extract_source_id,
    deduplicate_results,
    filter_and_rank_results
)
from media_extractor import (
    is_valid_media_url,
    classify_media_url,
    validate_media_url_strict,
)

logger = logging.getLogger(__name__)

DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"

QUALITY_ORDER = [1080, 720, 480, 360, 240]


def _label_to_int(label: str) -> int:
    """Extract numeric resolution from a quality label like '720p' or '720'."""
    if not label:
        return 0
    m = re.search(r'(\d{3,4})', label)
    return int(m.group(1)) if m else 0


def _parse_quality_label(label: str) -> str:
    """Normalize quality label to standard format (e.g., '1080p', '720p')."""
    if not label:
        return "auto"
    label = label.strip().lower()
    if label.endswith('p'):
        return label
    if re.match(r'^\d{3,4}$', label):
        return f"{label}p"
    return label


class AsilmediaParser(BaseParser):
    """Parser for asilmedia.org - DLE-based website"""
    
    # IMPORTANT: Use HTTP (not HTTPS) - the site returns different results
    # HTTP returns proper search results, HTTPS returns empty/invalid results
    BASE_URL = "http://asilmedia.org"
    
    # DLE search endpoint
    SEARCH_URL = BASE_URL + "/index.php?do=search"
    
    # DLE form fields (extracted from search form)
    SEARCH_FORM_FIELDS = {
        "do": "search",
        "subaction": "search",
        "search_start": "0",
        "full_search": "0", 
        "result_from": "1"
    }
    
    # DLE result container selectors (priority order)
    DLE_CONTAINER_SELECTORS = [
        ".search-page",           # DLE search results container
        ".search-results",        # Generic search results
        ".dle-search-results",    # DLE-specific
        ".xfr",                   # DLE template class
        "article.dle-search",     # DLE article
        "#searchresult",          # DLE search result ID
    ]
    
    # DLE card/item selectors (priority order)
    DLE_CARD_SELECTORS = [
        ".shortstory-item",               # DLE shortstory (used on this site)
        ".shortstory-item.moviebox",      # Movie cards on this site
        "article.shortstory",             # DLE article shortstory
        ".moviebox",                      # Movie box on this site
        "article:not(.carousel-card)",    # Non-carousel articles (category listing pages)
        "article",                        # Generic article (fallback)
        ".film-item",                     # Film items
    ]
    
    # DLE title selectors (priority order)
    DLE_TITLE_SELECTORS = [
        ".title h2 a",           # Title inside h2
        ".title a",              # Title link
        ".shortstory-headline", # DLE headline
        "h2.title a",           # h2 with title class
        "h2 a",                 # Any h2 link
        ".name a",              # Name link
    ]
    
    # DLE image selectors
    DLE_IMAGE_SELECTORS = [
        "img[data-src]",         # Lazy load data-src (KEY for this site)
        "img[data-lazy-src]", 
        "img[data-original]",
        "img.lazyload",
        "img.img-fit",
        "img.poster-img",
        "img",
    ]
    
    # DLE year selectors
    DLE_YEAR_SELECTORS = [
        "a[href*='xfsearch/year']",  # DLE year filter links
        ".year",                     # Year class
        ".date",                     # Date class
        "[class*='year']",          # Any year class
    ]
    
    # Category pages for fallback
    CATEGORY_PAGES = [
        "/films/tarjima_kinolar/",  # Translated movies
        "/films/",                  # All movies  
        "/films/yangi_filmlar/",    # New movies
    ]
    
    def __init__(self):
        super().__init__()
        self.session.headers.update({
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.5",
            "Referer": "https://asilmedia.org/",
            "Origin": "https://asilmedia.org",
        })
    
    @property
    def source_name(self) -> str:
        return "asilmedia"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL
    
    def search(self, query: str) -> List[SearchResult]:
        """Search for movies using DLE POST form with strong query relevance filtering"""

        logger.info(f"[ASILMEDIA] search query='{query}'")
        
        # First, try DLE POST search
        results = self._dle_post_search(query)

        # If no results, fallback to category browsing
        if not results:
            logger.info("[ASILMEDIA] POST search returned 0 results, trying category fallback")
            results = self._category_search(query)

        # Apply STRONG query relevance filtering
        if results and query:
            logger.info(f"[ASILMEDIA] Before relevance filtering: {len(results)} results")
            results = self._filter_by_query_relevance(results, query)
            logger.info(f"[ASILMEDIA] After relevance filtering: {len(results)} results")

        logger.info(f"[ASILMEDIA] search returning {len(results)} results")
        return results
    
    def _filter_by_query_relevance(self, results: List[SearchResult], query: str) -> List[SearchResult]:
        """
        Filter results by query relevance with strong matching.
        Returns only results where title contains the query terms.
        """
        if not results or not query:
            return results
        
        # Normalize query
        query_normalized = self._normalize_for_match(query)
        query_words = query_normalized.split()
        
        if not query_words:
            return results
        
        # Score each result
        scored = []
        for r in results:
            title = r.title or ""
            title_normalized = self._normalize_for_match(title)
            
            # Calculate relevance score with more granular scoring
            score = 0
            
            # Exact match (full query in title) - 100 points
            if query_normalized in title_normalized:
                score = 100
            # All query words match - 80 points
            elif all(word in title_normalized for word in query_words):
                score = 80
            # Query as substring in title - 70 points  
            else:
                # Check if any significant match
                has_match = False
                for word in query_words:
                    if len(word) >= 3 and word in title_normalized:  # Only words >= 3 chars
                        has_match = True
                        break
                if has_match:
                    # Count matching words
                    matches = sum(1 for word in query_words if len(word) >= 3 and word in title_normalized)
                    score = min(60, matches * 20)  # Max 60 points
            
            scored.append((score, r))
            
            if DEBUG:
                logger.info(f"[ASILMEDIA DLE] Scored: score={score}, title={title[:40]}")
        
        # Sort by score descending
        scored.sort(key=lambda x: x[0], reverse=True)
        
        # Keep ONLY results with score >= 40 (relevant matches)
        # This ensures we never return unrelated movies
        filtered = [r for score, r in scored if score >= 40]
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Final results: {len(filtered)} (kept score >= 40)")
        
        # NOTE: We intentionally return empty if no relevant results
        # Better to return nothing than unrelated movies
        # This is because the DLE site's search doesn't work, and category pages
        # don't contain the queried content
        
        return filtered
    
    def _normalize_for_match(self, text: str) -> str:
        """Normalize text for matching: lowercase, trim, collapse whitespace"""
        if not text:
            return ""
        # Lowercase
        text = text.lower()
        # Remove extra whitespace
        import re
        text = re.sub(r'\s+', ' ', text)
        return text.strip()
    
    def _dle_post_search(self, query: str) -> List[SearchResult]:
        """
        Perform DLE POST search with proper form fields.
        DLE-specific behavior: POST form submission, may need session cookies.
        """
        results = []

        try:
            form_data = self.SEARCH_FORM_FIELDS.copy()
            form_data["story"] = query

            # Establish session cookies before posting
            self.session.get(self.BASE_URL, timeout=30)

            logger.info(f"[ASILMEDIA] POST search url={self.SEARCH_URL} query='{query}'")

            response = self.session.post(
                self.SEARCH_URL,
                data=form_data,
                timeout=30,
                allow_redirects=True,
                headers={
                    "Content-Type": "application/x-www-form-urlencoded",
                    "Referer": self.BASE_URL + "/",
                    "Origin": self.BASE_URL,
                }
            )

            logger.info(f"[ASILMEDIA] POST response status={response.status_code} final_url={response.url}")

            soup = BeautifulSoup(response.text, "html.parser")

            page_type = self._detect_dle_page_type(soup, response.url)
            logger.info(f"[ASILMEDIA] Detected page type: {page_type}")

            # Only skip if it's clearly a non-search page (e.g. pure category listing).
            # "unknown" is fine — just try to extract whatever cards are there.
            if page_type == "no_results":
                logger.info(f"[ASILMEDIA] Search page says no results")
                return []

            results = self._extract_dle_results(soup, query)
            logger.info(f"[ASILMEDIA] POST search extracted {len(results)} results")
            return results

        except requests.RequestException as e:
            logger.warning(f"[ASILMEDIA] POST search request error: {e}")
            return []
        except Exception as e:
            logger.warning(f"[ASILMEDIA] POST search parse error: {e}")
            return []
    
    def _detect_dle_page_type(self, soup: BeautifulSoup, url: str) -> str:
        """
        Detect what type of DLE page this is:
        - search_results: Has actual search results
        - no_results: Search returned no results  
        - home: Home page or category page
        - unknown: Cannot determine
        """
        
        # Check URL for indicators
        if "do=search" in url:
            # Could be search results or no-results page
            pass
        elif "/films/" in url:
            return "category"
        
        # Look for DLE result containers
        for sel in self.DLE_CONTAINER_SELECTORS:
            container = soup.select_one(sel)
            if container:
                # Check if it has content
                text = container.get_text()
                if text and len(text.strip()) > 100:
                    return "search_results"
        
        # Check for shortstory items anywhere on page
        for sel in self.DLE_CARD_SELECTORS:
            items = soup.select(sel)
            if items:
                return "search_results"
        
        # Check for no-results message
        no_results_sel = [".no-results", ".not-found", ".empty-search", 
                         "[class*='no-result']", "[class*='empty']"]
        for sel in no_results_sel:
            msg = soup.select_one(sel)
            if msg:
                text = msg.get_text().lower()
                if any(w in text for w in ["no result", "not found", "empty", "не найден"]):
                    return "no_results"
        
        # Check page title
        title = soup.find("title")
        if title:
            title_text = title.get_text().lower()
            if "поиск" in title_text or "search" in title_text:
                # Search page but might be no-results
                # Check for any content
                content = soup.select_one("#content, .content, main")
                if content:
                    text = content.get_text().strip()
                    if len(text) < 200:
                        return "no_results"
        
        return "unknown"
    
    def _extract_dle_results(self, soup: BeautifulSoup, query: str) -> List[SearchResult]:
        """
        Extract search results from a DLE page.
        Searches the full soup (DLE search items are at root level, not in a container).
        """
        results = []

        # Search FULL page — DLE search results are not always inside a named container
        cards = []
        for sel in self.DLE_CARD_SELECTORS:
            cards = soup.select(sel)
            if cards:
                logger.info(f"[ASILMEDIA] Cards found with selector '{sel}': {len(cards)}")
                break

        # Fallback: any movie-like links on the page
        if not cards:
            all_links = soup.find_all("a", href=True)  # Fixed: was undefined 'search_area'
            cards = [a for a in all_links if self._is_movie_link(a.get("href", ""))]
            if cards:
                logger.info(f"[ASILMEDIA] Fallback movie links found: {len(cards)}")

        logger.info(f"[ASILMEDIA] Processing {len(cards)} cards")

        for card in cards:
            try:
                result = self._extract_dle_card(card)
                if result and result.title and result.detail_url:
                    results.append(result)
            except Exception as e:
                logger.debug(f"[ASILMEDIA] Card extract error: {e}")
                continue

        logger.info(f"[ASILMEDIA] Extracted {len(results)} raw results")

        # Deduplicate
        if results:
            dict_results = [{"title": r.title, "link": r.detail_url, "year": r.year,
                             "poster": r.poster, "source_id": r.source_id,
                             "description": r.description, "source": r.source} for r in results]
            dict_results = deduplicate_results(dict_results, key="link")
            results = [SearchResult(
                title=r["title"], year=r["year"], poster=r["poster"],
                description=r["description"], source_id=r["source_id"],
                detail_url=r["link"], source=r["source"]
            ) for r in dict_results]

        return results
    
    def _extract_dle_card(self, card) -> Optional[SearchResult]:
        """
        Extract data from a single DLE card/item.
        Handles both anchor elements and article/div elements.
        """
        
        title = ""
        link = ""
        
        # Case 1: Card is an anchor element
        if card.name == 'a':
            link = card.get("href", "")
            title = clean_text(card.get("title", "") or card.get_text())
        
        # Case 2: Card is a container (div, article, etc.)
        else:
            # Find the movie link (has numeric ID pattern)
            links = card.find_all("a", href=True)
            for a in links:
                href = a.get("href", "")
                if self._is_movie_link(href):
                    link = href
                    # Get title from link
                    title = clean_text(a.get("title", "") or a.get_text())
                    break
            
            # If still no title, try title selectors
            if not title:
                for sel in self.DLE_TITLE_SELECTORS:
                    title_elem = card.select_one(sel)
                    if title_elem:
                        title = clean_text(title_elem.get_text())
                        link = title_elem.get("href", "") or link
                        break
        
        if not title or not link:
            return None
        
        # Clean title (remove quality, ratings, site suffixes)
        title = self._clean_dle_title(title)
        
        detail_url = normalize_url(link, self.BASE_URL)
        source_id = extract_source_id(link)
        
        # Find poster/image
        poster = ""
        for sel in self.DLE_IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = (img.get("data-src") or img.get("data-lazy-src") or 
                         img.get("data-original") or img.get("src", ""))
                # Skip base64 images
                if poster and not poster.startswith("data:"):
                    break
        
        if poster and not poster.startswith("data:"):
            poster = normalize_url(poster, self.BASE_URL)
        else:
            poster = ""
        
        # Find year
        year = None
        year_link = card.select_one("a[href*='xfsearch/year']")
        if year_link:
            year = extract_year(year_link.get_text())
        
        if not year:
            for sel in self.DLE_YEAR_SELECTORS:
                year_elem = card.select_one(sel)
                if year_elem:
                    year = extract_year(year_elem.get_text())
                    if year:
                        break
        
        return SearchResult(
            title=title,
            year=year,
            poster=poster,
            description="",
            source_id=source_id,
            detail_url=detail_url,
            source=self.source_name
        )
    
    def _is_movie_link(self, href: str) -> bool:
        """Check if link is a movie detail page (handles both absolute and relative URLs)"""
        if not href:
            return False

        # For absolute URLs, must be on asilmedia.org
        if href.startswith("http") and "asilmedia" not in href:
            return False

        if not href.endswith(".html"):
            return False

        # Exclude category/search/filter links
        exclude = ["/films/", "/xfsearch/", "/category/", "/page/", "/index.php"]
        for pattern in exclude:
            if pattern in href:
                return False

        # Must have numeric ID: /12345-title.html
        return bool(re.search(r'/\d+-', href))
    
    def _clean_dle_title(self, title: str) -> str:
        """Clean DLE title - remove quality, ratings, suffixes"""
        if not title:
            return ""
        
        title = title.strip()
        
        # Remove quality patterns: 720p 1080p 4k etc.
        title = re.sub(r'^(?:\d+p\s*)+', '', title)
        
        # Remove rating patterns: +11 +36 etc.
        title = re.sub(r'^(?:\+\d+\s*)+', '', title)
        
        # Remove HD patterns
        title = re.sub(r'^(?:HD|Full\s*HD|4K|UHD)\s+', '', title, flags=re.IGNORECASE)
        
        # Remove site suffixes
        title = re.sub(r'\s*[•·]\s*Tarjima\s+Kinolar.*$', '', title, flags=re.IGNORECASE)
        title = re.sub(r'\s*[•·]\s*Multfilmlar.*$', '', title, flags=re.IGNORECASE)
        title = re.sub(r'\s+Uzbek\s+tilida.*$', '', title, flags=re.IGNORECASE)
        
        return clean_text(title)
    
    def _category_search(self, query: str) -> List[SearchResult]:
        """
        Fallback: Browse category pages to find movies.
        Used when DLE POST search returns empty results.
        """
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] === CATEGORY FALLBACK ===")
        
        all_results = []
        
        for cat_path in self.CATEGORY_PAGES:
            try:
                url = f"{self.BASE_URL.rstrip('/')}{cat_path}"
                
                if DEBUG:
                    logger.info(f"[ASILMEDIA DLE] Category: {url}")
                
                response = self.session.get(url, timeout=30)
                if response.status_code != 200:
                    continue
                
                soup = BeautifulSoup(response.text, "html.parser")
                
                # Extract using DLE selectors
                results = self._extract_dle_results(soup, query)
                
                if results:
                    if DEBUG:
                        logger.info(f"[ASILMEDIA DLE] Category {cat_path}: {len(results)} results")
                    all_results.extend(results)
                    
            except Exception as e:
                if DEBUG:
                    logger.warning(f"[ASILMEDIA DLE] Category error: {e}")
                continue
        
        # Deduplicate
        if all_results:
            # Convert to dicts for deduplication
            dict_results = [{"title": r.title, "link": r.detail_url, "year": r.year, 
                           "poster": r.poster, "source_id": r.source_id, 
                           "description": r.description, "source": r.source} for r in all_results]
            dict_results = deduplicate_results(dict_results, key="link")
            # Convert back to SearchResult objects
            all_results = [SearchResult(
                title=r["title"], year=r["year"], poster=r["poster"],
                description=r["description"], source_id=r["source_id"],
                detail_url=r["link"], source=r["source"]
            ) for r in dict_results]
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Category fallback total: {len(all_results)} results")
        
        return all_results
    
    def get_details(self, url: str) -> MovieDetails:
        """Get detailed movie information from detail page"""
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Getting details for: {url}")
        
        soup = self._fetch_page(url)
        return self._extract_details(soup, url)
    
    def _extract_details(self, soup, url: str) -> MovieDetails:
        """Extract movie details from DLE article page"""
        
        title = ""
        description = ""
        poster = ""
        backdrop = ""
        year = None
        duration = None
        quality = ""
        country = ""
        
        # Get title
        title_elem = soup.select_one("h1, .title, .shortstory-title")
        if title_elem:
            title = clean_text(title_elem.get_text())
        
        # Get description/synopsis
        desc_elem = soup.select_one(".full-story, .short-story, .description, [itemprop='description']")
        if desc_elem:
            description = clean_text(desc_elem.get_text())
        
        # Get poster
        poster_elem = soup.select_one("img[data-src], img[data-original], .poster img")
        if poster_elem:
            poster = poster_elem.get("data-src") or poster_elem.get("data-original") or poster_elem.get("src", "")
            if poster and not poster.startswith("data:"):
                poster = normalize_url(poster, self.BASE_URL)
        
        # Get backdrop
        backdrop_elem = soup.select_one(".backdrop img, .fanart img, [class*='backdrop'] img")
        if backdrop_elem:
            backdrop = backdrop_elem.get("src", "") or backdrop_elem.get("data-src", "")
            if backdrop and not backdrop.startswith("data:"):
                backdrop = normalize_url(backdrop, self.BASE_URL)
        
        # Get year
        year_link = soup.select_one("a[href*='xfsearch/year']")
        if year_link:
            year = extract_year(year_link.get_text())
        
        # Get duration
        duration_elem = soup.select_one(".duration, .time, [itemprop='duration']")
        if duration_elem:
            duration = extract_duration(duration_elem.get_text())
        
        # Get quality
        quality_elem = soup.select_one(".quality, .hd, [class*='quality']")
        if quality_elem:
            quality = clean_text(quality_elem.get_text())
        
        # Get genre/category
        genres = []
        genre_links = soup.select("a[href*='xfsearch/category'], a[href*='/films/']")
        for g in genre_links[:5]:
            genres.append(clean_text(g.get_text()))
        
        # Get country
        country_elem = soup.select_one(".country, .country-name, [class*='country']")
        if country_elem:
            country = clean_text(country_elem.get_text())
        
        # Extract video URLs
        video_urls = self._extract_video_urls(soup, url)
        
        source_id = extract_source_id(url)
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Video URLs found: {len(video_urls)}")
            for v in video_urls:
                logger.info(f"[ASILMEDIA DLE]   - type={v['type']}, url={v['url'][:80]}")
        
        return MovieDetails(
            title=title,
            year=year or 0,
            description=description,
            poster=poster,
            backdrop=backdrop,
            duration=duration or 0,
            quality=quality,
            genres=genres,
            country=country,
            source_id=source_id,
            detail_url=url,
            source=self.source_name,
            video_page_url=url,
            video_urls=video_urls
        )
    
    def _extract_video_urls(self, soup, page_url: str) -> List[Dict[str, str]]:
        """
        Extract video URLs from the detail page.
        This is critical - we need to get actual video URLs, not iframe URLs.
        
        Enhanced to detect and extract quality-labeled download/watch links from the page.
        """
        video_urls = []
        quality_urls = {}  # Map resolution (int) to URL
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] === VIDEO EXTRACTION ===")
        
        # FIRST: Try to find quality-labeled download links on the page
        # These are typically buttons/links that say "1080p", "720p", etc.
        quality_entries, quality_map = self._extract_quality_links(soup, page_url)
        if quality_entries:
            if DEBUG:
                logger.info(f"[ASILMEDIA DLE] Found {len(quality_entries)} quality-labeled video URLs")
                for q in quality_entries:
                    logger.info(f"[ASILMEDIA DLE]   Quality: {q.get('quality')}, URL: {q.get('url')[:60]}...")
            video_urls.extend(quality_entries)
            quality_urls = quality_map
        
        # If we found quality links, use those (best quality selection happens in server.py)
        if quality_urls:
            if DEBUG:
                logger.info(f"[ASILMEDIA DLE] Using quality-labeled links, skipping other extraction")
        
        # SECOND: If no quality links found, try direct video links
        if not quality_urls:
            direct_links = soup.select("a[href*='.mp4'], a[href*='.m3u8']")
            for link in direct_links:
                href = link.get("href", "")
                if href and (href.endswith(".mp4") or href.endswith(".m3u8")):
                    if not any(skip in href for skip in ['/film/', '/page/', 'asilmedia.org']):
                        video_urls.append({
                            "quality": "direct",
                            "url": normalize_url(href, self.BASE_URL),
                            "type": "direct_mp4" if href.endswith(".mp4") else "hls"
                        })
                        if DEBUG:
                            logger.info(f"[ASILMEDIA DLE] Found direct video: {href}")
        
        # THIRD: If still no quality URLs, try iframe
        if not quality_urls:
            iframe = soup.select_one("iframe[src]")
            if iframe:
                iframe_src = iframe.get("src", "")
                if iframe_src:
                    if DEBUG:
                        logger.info(f"[ASILMEDIA DLE] Found iframe: {iframe_src}")
                    
                    iframe_videos = self._extract_video_from_iframe(iframe_src, page_url)
                    video_urls.extend(iframe_videos)
                    
                    if not iframe_videos:
                        if DEBUG:
                            logger.warning(f"[ASILMEDIA DLE] No actual video URL found in iframe: {iframe_src}")
        
        # FOURTH: Video elements
        if not quality_urls:
            video_elements = soup.select("video[src], video source[src]")
            for video in video_elements:
                src = video.get("src", "") or (video.find("source") and video.find("source").get("src"))
                if src:
                    video_urls.append({
                        "quality": "unknown",
                        "url": normalize_url(src, self.BASE_URL),
                        "type": "html5_video"
                    })
                    if DEBUG:
                        logger.info(f"[ASILMEDIA DLE] Found HTML5 video: {src}")
        
        # FIFTH: JavaScript extraction
        if not quality_urls:
            script_videos = self._extract_video_from_scripts(soup, page_url)
            video_urls.extend(script_videos)
        
        # Deduplicate
        seen = set()
        unique = []
        for v in video_urls:
            if v["url"] not in seen and v["url"]:
                seen.add(v["url"])
                unique.append(v)
        
        return unique
    
    def _extract_quality_links(self, soup, page_url: str) -> tuple[List[Dict[str, str]], dict]:
        """
        Extract quality-labeled video links from the page.
        
        Looks for:
        - Links/buttons with quality labels (1080p, 720p, 480p)
        - Links containing quality indicators in text
        - PlayerJS-style quality entries
        
        Returns:
            (entries, quality_map): List of video entries sorted by quality, and quality->url map
        """
        entries = []
        quality_map = {}
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] === QUALITY LINK EXTRACTION ===")
        
        # Pattern 1: Look for links with quality text (e.g., buttons saying "1080p", "720p")
        quality_patterns = [
            r'1080p', r'1080', r'full\s*hd', r'fullhd',
            r'720p', r'720', r'hd',
            r'480p', r'480', r'sd',
            r'360p', r'360',
        ]
        
        # Find all links that might be quality selectors
        all_links = soup.find_all("a", href=True)
        
        for link in all_links:
            href = link.get("href", "")
            text = link.get_text(strip=True).lower()
            
            if not href:
                continue
            
            # Skip non-video links
            if any(skip in href for skip in ['/film/', '/page/', '/category/', '/search', 'asilmedia.org']):
                continue
            
            # Check if this link has quality indicators
            is_quality_link = False
            detected_quality = ""
            
            for pattern in quality_patterns:
                if re.search(pattern, text) or re.search(pattern, href.lower()):
                    is_quality_link = True
                    if '1080' in pattern or 'full' in pattern:
                        detected_quality = "1080p"
                    elif '720' in pattern:
                        detected_quality = "720p"
                    elif '480' in pattern:
                        detected_quality = "480p"
                    elif '360' in pattern:
                        detected_quality = "360p"
                    break
            
            if not is_quality_link:
                continue
            
            # Validate URL
            if not (href.endswith('.mp4') or href.endswith('.m3u8') or href.endswith('.m3u')):
                continue
            
            error = validate_media_url_strict(href)
            if error:
                if DEBUG:
                    logger.info(f"[ASILMEDIA DLE] Skip quality link (invalid): {href[:60]} - {error}")
                continue
            
            url = normalize_url(href, self.BASE_URL)
            res = _label_to_int(detected_quality)
            
            if res not in quality_map:
                quality_map[res] = url
                entries.append({
                    "quality": _parse_quality_label(detected_quality),
                    "url": url,
                    "type": classify_media_url(url)
                })
                if DEBUG:
                    logger.info(f"[ASILMEDIA DLE] Found quality link: {detected_quality} -> {url[:60]}...")
        
        # Pattern 2: Look for PlayerJS-style quality entries in scripts
        playerjs_entries = self._extract_playerjs_quality(soup, page_url)
        for entry in playerjs_entries:
            res = _label_to_int(entry.get("quality", ""))
            if res not in quality_map:
                quality_map[res] = entry["url"]
                entries.append(entry)
                if DEBUG:
                    logger.info(f"[ASILMEDIA DLE] Found PlayerJS quality: {entry.get('quality')} -> {entry['url'][:60]}...")
        
        # Pattern 3: Look for quality buttons in specific containers
        # Common patterns: .quality-list, .download-buttons, .player-qualities
        quality_containers = soup.select(".quality-list, .download-buttons, .player-qualities, .video-qualities, [class*='quality']")
        for container in quality_containers:
            links = container.find_all("a", href=True)
            for link in links:
                href = link.get("href", "")
                text = link.get_text(strip=True)
                
                if not href or not (href.endswith('.mp4') or href.endswith('.m3u8')):
                    continue
                
                # Extract quality from text
                detected_quality = ""
                for pattern, label in [(r'1080', '1080p'), (r'720', '720p'), (r'480', '480p'), (r'360', '360p')]:
                    if re.search(pattern, text, re.IGNORECASE):
                        detected_quality = label
                        break
                
                if not detected_quality:
                    continue
                
                error = validate_media_url_strict(href)
                if error:
                    continue
                
                url = normalize_url(href, self.BASE_URL)
                res = _label_to_int(detected_quality)
                
                if res not in quality_map:
                    quality_map[res] = url
                    entries.append({
                        "quality": detected_quality,
                        "url": url,
                        "type": classify_media_url(url)
                    })
                    if DEBUG:
                        logger.info(f"[ASILMEDIA DLE] Found container quality: {detected_quality} -> {url[:60]}...")
        
        # Sort entries by quality (highest first)
        if entries:
            sorted_resolutions = sorted(quality_map.keys(), reverse=True)
            sorted_entries = []
            for res in sorted_resolutions:
                for e in entries:
                    if _label_to_int(e.get("quality", "")) == res:
                        sorted_entries.append(e)
                        break
            entries = sorted_entries
        
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Quality extraction complete: {len(entries)} entries, qualities: {list(quality_map.keys())}")
        
        return entries, quality_map
    
    def _extract_playerjs_quality(self, soup, page_url: str) -> List[Dict[str, str]]:
        """Extract quality-labeled video URLs from PlayerJS-style scripts"""
        entries = []
        
        for script in soup.find_all("script"):
            content = script.string or ""
            if not content:
                continue
            
            # Look for PlayerJS or similar patterns with quality labels
            # Pattern: file:"[1080p]url,[720p]url" or file:"url" with quality labels
            if "Playerjs" not in content and "file" not in content.lower():
                continue
            
            # Try pattern: [quality]url
            labeled = re.findall(r'\[(\d+p?)\](https?://[^\s,\]]+)', content, re.IGNORECASE)
            
            for label, url in labeled:
                url = url.strip()
                if not url.startswith("http"):
                    continue
                
                error = validate_media_url_strict(url)
                if error:
                    continue
                
                url = normalize_url(url, page_url)
                quality_label = _parse_quality_label(label)
                
                entries.append({
                    "quality": quality_label,
                    "url": url,
                    "type": classify_media_url(url)
                })
                if DEBUG:
                    logger.info(f"[ASILMEDIA DLE] PlayerJS quality: {quality_label} -> {url[:60]}...")
            
            # Also try file:"url" pattern
            file_match = re.search(r'file\s*:\s*"([^"]+)"', content)
            if file_match:
                file_content = file_match.group(1)
                # Check if it's a single URL or has quality labels
                if not re.search(r'\d+p?', file_content):
                    # Single URL, no quality
                    pass
        
        return entries
    
    def _extract_video_from_iframe(self, iframe_url: str, page_url: str) -> List[Dict[str, str]]:
        """Fetch iframe page and extract actual video URL"""
        if DEBUG:
            logger.info(f"[ASILMEDIA DLE] Fetching iframe: {iframe_url}")
        
        video_urls = []
        
        try:
            headers = {
                "Referer": page_url,
                "Accept": "text/html,application/xhtml+xml,*/*;q=0.8",
            }
            
            response = self.session.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
            iframe_final_url = response.url
            
            if DEBUG:
                logger.info(f"[ASILMEDIA DLE] Iframe final URL: {iframe_final_url}")
                logger.info(f"[ASILMEDIA DLE] Content-Type: {response.headers.get('Content-Type')}")
            
            # Check content type
            content_type = response.headers.get("Content-Type", "")
            if "text/html" not in content_type:
                # Might be direct video
                if "video" in content_type or "octet" in content_type:
                    video_urls.append({
                        "quality": "unknown",
                        "url": iframe_final_url,
                        "type": "direct_mp4"
                    })
                return video_urls
            
            # Parse HTML
            soup = BeautifulSoup(response.content, "lxml")
            
            # Try video element
            video = soup.select_one("video[src], video source[src]")
            if video:
                src = video.get("src") or (video.find("source") and video.find("source").get("src"))
                if src:
                    src = normalize_url(src, iframe_final_url)
                    video_urls.append({
                        "quality": "unknown",
                        "url": src,
                        "type": "html5_video"
                    })
                    if DEBUG:
                        logger.info(f"[ASILMEDIA DLE] Video from iframe: {src}")
            
            # Extract from scripts
            script_videos = self._extract_video_from_content(response.text, iframe_final_url)
            video_urls.extend(script_videos)
            
        except Exception as e:
            if DEBUG:
                logger.warning(f"[ASILMEDIA DLE] Error fetching iframe: {e}")
        
        return video_urls
    
    def _extract_video_from_scripts(self, soup, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from JavaScript code"""
        video_urls = []
        
        for script in soup.select("script"):
            content = script.string or ""
            if content:
                videos = self._extract_video_from_content(content, base_url)
                video_urls.extend(videos)
        
        return video_urls
    
    def _extract_video_from_content(self, content: str, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from any text content with enhanced validation"""
        video_urls = []
        
        if not content or len(content) < 100:
            return video_urls
        
        # Patterns for video URLs
        patterns = [
            r'["\']([^"\']*\.mp4)["\']',
            r'["\']([^"\']*\.m3u8)["\']',
            r'["\']([^"\']*\.mpd)["\']',
            r'(https?://[^\s"\'<>]+\.mp4)',
            r'(https?://[^\s"\'<>]+\.m3u8)',
            r'(https?://[^\s"\'<>]+\.mpd)',
            r'"src"\s*:\s*"([^"]+\.(?:mp4|m3u8|mpd))"',
            r'"file"\s*:\s*"([^"]+\.(?:mp4|m3u8|mpd))"',
            r'file\s*[=:]\s*["\']([^"\']+\.(?:mp4|m3u8|mpd))["\']',
        ]
        
        for pattern in patterns:
            try:
                matches = re.findall(pattern, content, re.IGNORECASE)
                for match in matches:
                    if match and not match.startswith("data:"):
                        # Skip non-video URLs
                        skip = ["thumbnail", "poster", "image", "avatar", "logo", "icon"]
                        if any(s in match.lower() for s in skip):
                            continue
                        
                        url = normalize_url(match, base_url)
                        
                        # [ENHANCED] Validate URL using media_extractor before adding
                        error = validate_media_url_strict(url)
                        if error:
                            if DEBUG:
                                logger.info(f"[ASILMEDIA DLE] Skipping invalid URL: {url[:60]}... Reason: {error}")
                            continue
                        
                        # Classify the URL type
                        url_type = classify_media_url(url)
                        
                        video_urls.append({
                            "quality": "unknown",
                            "url": url,
                            "type": url_type
                        })
                        if DEBUG:
                            logger.info(f"[ASILMEDIA DLE] Script found: {url} (type: {url_type})")
            except re.error:
                continue
        
        return video_urls
    
    def list_categories(self):
        """Scrape genre/category links from asilmedia.org navigation."""
        try:
            response = self.session.get(self.BASE_URL + "/", timeout=20)
            response.raise_for_status()
            soup = BeautifulSoup(response.text, "lxml")
        except Exception as e:
            logger.warning(f"[ASILMEDIA] list_categories: failed: {e}")
            return []

        import re as _re
        skip_re = _re.compile(r'(?:login|register|lostpassword|account|profile|search|rss|feed|logout)', _re.IGNORECASE)
        categories = []
        seen_urls = set()

        for sel in [".genres a", ".genre-list a", ".categories a", ".category-list a",
                    "nav a", ".nav a", ".menu a", "#menu a", ".main-menu a", ".top-menu a",
                    ".navigation a", ".header-menu a"]:
            links = soup.select(sel)
            for a in links:
                href = a.get("href", "").strip()
                name = clean_text(a.get_text())
                if not href or not name or len(name) < 2:
                    continue
                if href.startswith("http") and not href.startswith(self.BASE_URL):
                    continue
                full_url = normalize_url(href, self.BASE_URL)
                if not full_url or full_url.rstrip("/") == self.BASE_URL.rstrip("/"):
                    continue
                if skip_re.search(href) or skip_re.search(name):
                    continue
                if full_url in seen_urls:
                    continue
                seen_urls.add(full_url)
                slug = full_url.rstrip("/").split("/")[-1]
                categories.append({"name": name, "url": full_url, "slug": slug})
            if categories:
                break

        logger.info(f"[ASILMEDIA] list_categories: found {len(categories)} categories")
        return categories

    def list_catalog(self, page=1, limit=20, type_filter="", category_url=""):
        """
        List movies from asilmedia.org catalog with pagination.

        Args:
            page: Page number (1-based)
            limit: Max items per page
            type_filter: Optional filter
            category_url: Optional category/genre URL to browse instead of homepage

        Returns:
            dict with items, page, limit, total, total_pages, has_more
        """
        # Asilmedia: category pages have proper paginated listings.
        # The homepage has no real pagination — use the films category as the default.
        DEFAULT_CATALOG = self.BASE_URL + "/films/tarjima_kinolar"
        base = (category_url.rstrip("/") if category_url else DEFAULT_CATALOG)
        url = f"{base}/page/{page}/" if page > 1 else base + "/"
        logger.info(f"[ASILMEDIA] list_catalog: page={page} url={url}")

        try:
            response = self.session.get(url, timeout=30)
            response.raise_for_status()
        except Exception as e:
            logger.error(f"[ASILMEDIA] list_catalog: failed to fetch {url}: {e}")
            return {
                "items": [], "page": page, "limit": limit,
                "total": 0, "total_pages": 0, "has_more": False
            }

        soup = BeautifulSoup(response.text, "lxml")

        # Find movie cards using DLE selectors
        cards = []
        for selector in self.DLE_CARD_SELECTORS:
            cards = soup.select(selector)
            if cards:
                logger.info(f"[ASILMEDIA] list_catalog: found {len(cards)} cards with '{selector}'")
                break

        if not cards:
            content = soup.select_one("#dle-content, .content, #content, main")
            if content:
                cards = content.find_all(["article", "div"], class_=True, recursive=False)
                logger.info(f"[ASILMEDIA] list_catalog: broader detection found {len(cards)} cards")

        items = []
        seen_urls = set()
        for card in cards:
            try:
                item = self._extract_catalog_card(card)
                if item and item.get("title"):
                    url_key = item.get("detail_url", "") or item.get("source_id", "")
                    if url_key and url_key in seen_urls:
                        continue
                    if url_key:
                        seen_urls.add(url_key)
                    items.append(item)
            except Exception as e:
                logger.debug(f"[ASILMEDIA] list_catalog: error extracting card: {e}")
                continue

        logger.info(f"[ASILMEDIA] list_catalog: extracted {len(items)} items from page {page}")

        if type_filter:
            items = [i for i in items if i.get("type") == type_filter]

        # Parse actual total pages from pagination HTML
        total_pages = self._parse_total_pages(soup, page)

        # Heuristic fallback for category pages: if we got a full page of items and
        # pagination parsing didn't find more pages, assume there is at least one more.
        if category_url and len(items) >= limit and total_pages <= page:
            total_pages = page + 1

        has_more = total_pages > page
        logger.info(f"[ASILMEDIA] list_catalog: page={page} total_pages={total_pages} has_more={has_more}")

        return {
            "items": items[:limit],
            "page": page,
            "limit": limit,
            "total": len(items),
            "total_pages": total_pages,
            "has_more": has_more,
        }
    
    def _parse_total_pages(self, soup, current_page: int) -> int:
        """
        Parse the actual total page count from pagination HTML.
        DLE sites use /page/N/ in href — find the largest N across all pagination links.
        """
        pagination = soup.select_one(
            ".navigation, .pagination, .pages, .navig, .pager, .page-nav, #bottom-nav, .page-links"
        )
        if not pagination:
            return current_page

        max_page = current_page
        for link in pagination.select("a[href]"):
            href = link.get("href", "")
            m = re.search(r'/page/(\d+)/?', href)
            if m:
                pn = int(m.group(1))
                if pn > max_page:
                    max_page = pn

        logger.info(f"[ASILMEDIA] _parse_total_pages: current={current_page} parsed_max={max_page}")
        return max_page

    def _extract_catalog_card(self, card):
        """Extract a catalog item from a DLE movie card element."""
        title = ""
        detail_url = ""

        # Title extraction using DLE selectors
        for sel in self.DLE_TITLE_SELECTORS:
            el = card.select_one(sel)
            if el:
                title = clean_text(el.get_text())
                href = el.get("href", "")
                if href:
                    detail_url = normalize_url(href, self.BASE_URL)
                break

        if not title:
            for a in card.find_all("a"):
                text = clean_text(a.get_text())
                if text and len(text) > 3 and not text.isdigit():
                    title = text
                    href = a.get("href", "")
                    if href:
                        detail_url = normalize_url(href, self.BASE_URL)
                    break

        if not title:
            return None

        # Poster using DLE image selectors
        poster = ""
        for sel in self.DLE_IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("data-original") or img.get("src", "")
                if poster:
                    poster = normalize_url(poster, self.BASE_URL)
                break
        
        # Year using DLE year selectors
        year = 0
        for sel in self.DLE_YEAR_SELECTORS:
            el = card.select_one(sel)
            if el:
                y = extract_year(el.get_text())
                if y:
                    year = y
                    break
        if not year:
            y = extract_year(card.get_text())
            if y:
                year = y
        
        # Source ID
        source_id = ""
        if detail_url:
            source_id = extract_source_id(detail_url)
            if not source_id:
                # Fallback: use URL path slug as source_id
                slug = detail_url.rstrip("/").split("/")[-1]
                slug = slug.replace(".html", "").replace(".htm", "")
                if slug and slug not in ("", self.BASE_URL.rstrip("/")):
                    source_id = slug

        if not source_id:
            return None  # Skip items with no identifiable source_id

        # Description
        desc = ""
        for sel in [".description", ".desc", ".text", ".short-desc", "p"]:
            el = card.select_one(sel)
            if el:
                desc = clean_text(el.get_text())[:200]
                break

        item_type = "serial" if ("/serial/" in detail_url or "/series/" in detail_url) else "movie"

        return {
            "source_id": source_id,
            "title": title,
            "year": year,
            "type": item_type,
            "poster": poster,
            "description": desc,
            "genres": [],
            "detail_url": detail_url,
        }
