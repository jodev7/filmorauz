"""
Uzmovi (uzmovi.tv) Parser
Clean implementation using BaseParser and helpers
"""
import base64
import logging
import os
import re
import json
import urllib.parse
from typing import List, Optional, Dict
from urllib.parse import urljoin, urlparse
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
    isValidStreamUrl,
    select_best_stream_url,
    canonical_episode_id,
)

# Import media_extractor for specialized uzmovi extraction
from media_extractor import (
    extract_media_for_source,
    extract_from_uzmovi,
    is_valid_media_url,
    classify_media_url,
    MediaCandidate
)

logger = logging.getLogger(__name__)

# Enable debug logging in development
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


class UzmoviParser(BaseParser):
    """Parser for uzmovi.tv"""
    
    BASE_URL = "https://uzmovi.tv"
    
    # Specific card selectors (order matters - most specific first)
    CARD_SELECTORS = [
        ".shortstory",
        ".shortstory-item",
        "article.shortstory",
        ".short-story",
        ".film-item",
        ".movie-item",
        ".moviebox",
        ".movie-card",
        ".result-item",
        ".search-result",
        ".item",
        "article[class]",
    ]
    
    # Title selectors - prioritize heading elements
    TITLE_SELECTORS = [
        "h2 a", "h3 a", "h4 a",
        ".title a", ".film-title a",
        ".short-title a",
        ".card-title a",
    ]
    
    # Image selectors
    IMAGE_SELECTORS = [
        "img[data-src]",
        "img[data-lazy-src]",
        "img[data-original]",
        "img[src]",
    ]
    
    # Year selectors
    YEAR_SELECTORS = [
        ".year", ".film-year", ".date", 
        "[class*='year']", ".meta"
    ]
    
    # Quality selectors
    QUALITY_SELECTORS = [
        ".quality", ".hd", ".quality-badge",
        "[class*='quality']"
    ]
    
    def __init__(self):
        super().__init__()
        self.session.headers.update({
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
            "Accept-Language": "en-US,en;q=0.5",
        })
    
    @property
    def source_name(self) -> str:
        return "uzmovi"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL

    def _detect_uzmovi_type(self, detail_url: str = "", title: str = "", soup=None, card=None) -> str:
        """Detect movie vs serial using Uzmovi page/content signals first, URL second."""
        title_lower = clean_text(title).lower() if title else ""
        signal_text = title_lower

        nodes = []
        if card is not None:
            nodes.append(card)
        if soup is not None:
            nodes.append(soup)

        for node in nodes:
            try:
                node_text = clean_text(node.get_text(" ", strip=True)).lower()
            except Exception:
                node_text = ""
            if node_text:
                signal_text = f"{signal_text} {node_text}".strip()

        strong_title_signals = (
            "barcha qismlari",
            " serial",
            "serial ",
            "сериал",
        )
        if any(sig in signal_text for sig in strong_title_signals):
            return "serial"

        episode_patterns = (
            r'\b\d+\s*-\s*qism\b',
            r'\b\d+-qism\b',
            r'\b\d+\s*qism\b',
            r'\bepisode\s*\d+\b',
            r'\bseriya\s*\d+\b',
            r'\bсерия\s*\d+\b',
        )
        if any(re.search(pattern, signal_text, re.IGNORECASE) for pattern in episode_patterns):
            return "serial"

        page_signals = (
            "qismlardan tanlash",
            "barcha qismlari",
            "episode list",
            "episode grid",
            "serial",
            "сериал",
        )
        if any(sig in signal_text for sig in page_signals):
            return "serial"

        serial_selectors = (
            ".batcoh-list",
            ".batcoh-item",
            "a[href*='/episode/']",
            "a[title*='-qism']",
            "a[title*='qism']",
            "[class*='episode']",
            "[class*='serial']",
        )
        for node in nodes:
            for selector in serial_selectors:
                try:
                    if node.select_one(selector):
                        return "serial"
                except Exception:
                    continue

        genre_selectors = (
            ".finfo a",
            ".finfo-text a",
            ".genre a",
            ".genres a",
            "[class*='genre'] a",
            "a[title='Serial']",
        )
        for node in nodes:
            for selector in genre_selectors:
                try:
                    for el in node.select(selector):
                        genre_text = clean_text(el.get_text()).lower()
                        if genre_text in ("serial", "сериал") or " serial " in f" {genre_text} ":
                            return "serial"
                except Exception:
                    continue

        detail_lower = (detail_url or "").lower()
        if any(seg in detail_lower for seg in (
            "/serialar/", "/seriallar/", "/serial/", "/tv-series/", "/episode/",
            "/uzbek-serial", "/turk-serial", "/korea-serial", "/koreya-serial",
            "/hind-serial", "/multserial",
        )):
            return "serial"
        return "movie"
    
    def search(self, query: str) -> List[SearchResult]:
        """Search for movies on uzmovi.tv using the site's real search form."""
        results: List[SearchResult] = []
        search_url = f"{self.BASE_URL}/search.html"
        params = {
            "do": "search",
            "subaction": "search",
            "story": query,
        }
        try:
            response = self.session.get(search_url, params=params, timeout=30, allow_redirects=True)
            response.raise_for_status()
            soup = BeautifulSoup(response.content, "lxml")
            results = self._extract_search_results(soup, query)
        except Exception as e:
            logger.warning(f"[UZMOVI] search failed query={query!r}: {e}")
            results = []
        logger.info(f"[SEARCH] source=uzmovi query={query} items_found={len(results)}")
        return results
    
    def _extract_search_results(self, soup, query: str = "") -> List[SearchResult]:
        """Extract search results from parsed HTML"""
        
        # Remove sidebar/excluded areas first
        self._remove_excluded_areas(soup)
        
        # Find cards
        cards = self._find_cards(soup)
        
        if DEBUG:
            logger.info(f"[UZMOVI] Found {len(cards)} cards")
        
        # Extract from each card
        results = []
        for card in cards:
            try:
                result = self._extract_card(card)
                if result and result.title and result.detail_url:
                    results.append(result)
            except Exception as e:
                if DEBUG:
                    logger.warning(f"[UZMOVI] Card extraction error: {e}")
                continue
        
        # Deduplicate
        results = deduplicate_results(
            [{"title": r.title, "link": r.detail_url, "year": r.year, 
              "poster": r.poster, "source_id": r.source_id, 
              "description": r.description, "source": r.source,
              "type": r.content_type} for r in results],
            key="link"
        )

        results = [SearchResult(
            title=r["title"], year=r["year"], poster=r["poster"],
            description=r["description"], source_id=r["source_id"],
            detail_url=r["link"], source=r["source"],
            content_type=r.get("type") or self._detect_uzmovi_type(
                detail_url=r["link"], title=r["title"]
            )
        ) for r in results]

        for r in results:
            logger.info(f"[SEARCH] source=uzmovi result={r.detail_url[:80]} content_type={r.content_type}")
        
        if DEBUG:
            logger.info(f"[UZMOVI] Returning {len(results)} results")
        
        return results
    
    def _remove_excluded_areas(self, soup):
        """Remove sidebar and excluded areas"""
        excluded = [
            ".sidebar", ".side-bar", ".left-sidebar", ".right-sidebar",
            ".widget", ".sidebar-widget", ".random-movies", ".random-films",
            ".featured", ".top-movies", ".trending", ".popular", ".premium",
            ".banner", ".advertisement", ".ads",
        ]
        for sel in excluded:
            for elem in soup.select(sel):
                elem.decompose()
    
    def _find_cards(self, soup) -> list:
        """Find movie/film cards in the page"""
        # Try each selector in order
        for selector in self.CARD_SELECTORS:
            cards = soup.select(selector)
            if cards:
                if DEBUG:
                    logger.info(f"[UZMOVI] Using selector: {selector} ({len(cards)} items)")
                return cards
        
        # Fallback: find all links to movies
        all_links = soup.find_all("a", href=True)
        movie_links = [a for a in all_links if "/film/" in a.get("href", "") or "/movie/" in a.get("href", "")]
        return movie_links
    
    def _extract_card(self, card) -> Optional[SearchResult]:
        """Extract data from a single card"""
        
        # Find title - try heading elements first, then links
        title = ""
        link = ""
        
        # Try title selectors
        for sel in self.TITLE_SELECTORS:
            title_elem = card.select_one(sel)
            if title_elem:
                title = clean_text(title_elem.get_text())
                link = title_elem.get("href", "")
                break
        
        # Fallback: any prominent link in card
        if not title or not link:
            links = card.find_all("a", href=True)
            for a in links:
                href = a.get("href", "")
                if "/film/" in href or "/movie/" in href:
                    link = href
                    title = clean_text(a.get("title", "") or a.get_text())
                    break
        
        if not title or not link:
            return None
        
        # Normalize URL
        detail_url = normalize_url(link, self.BASE_URL)
        
        # Extract source_id
        source_id = extract_source_id(link)
        
        # Find image
        poster = ""
        for sel in self.IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("data-original") or img.get("src", "")
                break
        
        if poster:
            poster = normalize_url(poster, self.BASE_URL)
        
        # Find year - use None for missing year (not 0)
        year = None
        for sel in self.YEAR_SELECTORS:
            year_elem = card.select_one(sel)
            if year_elem:
                extracted = extract_year(year_elem.get_text())
                if extracted:
                    year = extracted
                    break
        
        return SearchResult(
            title=title,
            year=year,
            poster=poster,
            description="",
            source_id=source_id,
            detail_url=detail_url,
            source=self.source_name,
            content_type=self._detect_uzmovi_type(detail_url=detail_url, title=title, card=card),
        )
    
    def get_details(self, url: str) -> MovieDetails:
        """Get detailed movie information from uzmovi.tv"""
        soup = self._fetch_page(url)
        return self._extract_details(soup, url)
    
    def _extract_details(self, soup, url: str) -> MovieDetails:
        """Extract detailed movie information"""
        
        if DEBUG:
            logger.info(f"[UZMOVI] === DETAILS EXTRACTION ===")
            logger.info(f"[UZMOVI] Detail page URL: {url}")
        
        # Title
        title_elem = soup.select_one("h1, .title, .film-title")
        title = clean_text(title_elem.get_text()) if title_elem else "Unknown"
        
        # Description
        desc_elem = soup.select_one(".description, .desc, .synopsis, .text")
        description = clean_text(desc_elem.get_text()) if desc_elem else ""
        
        # Poster
        poster = ""
        og_image = soup.select_one("meta[property='og:image']")
        if og_image:
            poster = og_image.get("content", "")
        else:
            img = soup.select_one(".poster img, img.poster")
            if img:
                poster = img.get("src", "")
        
        if poster:
            poster = normalize_url(poster, self.BASE_URL)
        
        # Backdrop
        backdrop = ""
        backdrop_elem = soup.select_one(".backdrop img, .fanart img")
        if backdrop_elem:
            backdrop = backdrop_elem.get("src", "")
        if backdrop:
            backdrop = normalize_url(backdrop, self.BASE_URL)
        
        # Year - use None for missing year (not 0)
        year = None
        year_elem = soup.select_one(".year, [class*='year'], .date")
        if year_elem:
            extracted = extract_year(year_elem.get_text())
            if extracted:
                year = extracted
        
        # Genres
        genre_elems = soup.select(".genres a, .genre a, [class*='genre'] a")
        genres = [clean_text(g.get_text()) for g in genre_elems if g.get_text()]
        
        # Country
        country_elem = soup.select_one(".country, .country-name")
        country = clean_text(country_elem.get_text()) if country_elem else ""
        
        # Duration
        duration = 0
        duration_elem = soup.select_one(".duration, .runtime, [class*='duration']")
        if duration_elem:
            extracted = extract_duration(duration_elem.get_text())
            if extracted:
                duration = extracted
        
        # Video URLs - UNIVERSAL MEDIA EXTRACTION
        # Uses _extract_all_media_from_page which combines:
        # - Video tag extraction (video[src], video>source[src])
        # - Regex extraction for srv*.uzdown.space (mp4, m3u8, mpd)
        # - Playwright fallback if fast path finds no media
        
        if DEBUG:
            logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
            logger.info(f"[UZMOVI] === STARTING MEDIA EXTRACTION ===")
            logger.info(f"[UZMOVI] Detail page URL: {url}")
            logger.info(f"[UZMOVI] Calling _extract_all_media_from_page()...")
            logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        
        try:
            video_urls = self._extract_all_media_from_page(url)
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] ERROR in _extract_all_media_from_page: {e}")
                import traceback
                logger.info(f"[UZMOVI] Traceback: {traceback.format_exc()}")
            video_urls = []
        
        if DEBUG:
            mp4_count = sum(1 for v in video_urls if v.get('type') == 'mp4')
            m3u8_count = sum(1 for v in video_urls if v.get('type') == 'm3u8')
            mpd_count = sum(1 for v in video_urls if v.get('type') == 'mpd')
            logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
            logger.info(f"[UZMOVI] === _extract_all_media_from_page() RESULT ===")
            logger.info(f"[UZMOVI] Media URLs returned: {len(video_urls)} (mp4={mp4_count}, m3u8={m3u8_count}, mpd={mpd_count})")
            
            if not video_urls:
                logger.info(f"[UZMOVI] WARNING: No media URLs found after extraction!")
            else:
                logger.info(f"[UZMOVI] === RETURNED VIDEO URLs ===")
                for i, v in enumerate(video_urls):
                    logger.info(f"[UZMOVI]   [{i}] type={v['type']}, url={v['url']}")
            logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        
        player_url = ""
        video_candidates_checked = 0
        
        # If no media found, try iframe fallback
        if not video_urls:
            iframe = soup.select_one("iframe[src]")
            if iframe:
                iframe_src = iframe.get("src", "")
                if iframe_src:
                    iframe_src = normalize_url(iframe_src, self.BASE_URL)
                    player_url = iframe_src
                    video_candidates_checked += 1
                    if DEBUG:
                        logger.info(f"[UZMOVI] No media found, using iframe as fallback: {iframe_src}")
        
        if DEBUG:
            logger.info(f"[UZMOVI] === FINAL VIDEO EXTRACTION SUMMARY ===")
            logger.info(f"[UZMOVI] Detail page: {url}")
            logger.info(f"[UZMOVI] Checked selectors: video_urls={len(video_urls)}, iframes={video_candidates_checked}")
            logger.info(f"[UZMOVI] Player URL fallback: {player_url or 'none'}")
            logger.info(f"[UZMOVI] Found {len(video_urls)} playable video URL(s):")
            for i, v in enumerate(video_urls):
                logger.info(f"[UZMOVI]   [{i}] type={v['type']}, url={v['url']}")
            logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        
        # Extract source_id
        source_id = extract_source_id(url)
        
        movie_type = self._detect_uzmovi_type(detail_url=url, title=title, soup=soup)
        logger.info(f"[PARSER] detected content_type={movie_type} url={url}")
        
        # If it's a serial episode page, build canonical source_id
        if movie_type == "serial":
            from uzmovi_serial import _parse_episode_href, _parse_season_number
            parsed = _parse_episode_href(url)
            if parsed:
                # _parse_episode_href returns (group_id, episode_no)
                _, episode = parsed
                # For season, try to find it on page
                season = _parse_season_number(title) or 1
                parent_id = source_id
                source_id = canonical_episode_id(parent_id, season, episode)
                logger.info(
                    f"[episode-parse] title={title} parent={parent_id} season={season} episode={episode} source_id={source_id}"
                )

        return MovieDetails(
            title=title,
            description=description,
            poster=poster,
            backdrop=backdrop,
            year=year,
            genres=genres,
            country=country,
            duration=duration,
            video_page_url=url,
            player_url=player_url,
            video_urls=video_urls,
            source=self.source_name,
            source_id=source_id,
            type=movie_type
        )
        
        # [OLD CODE BELOW - Kept for reference but not executed]
        # Step 1: Look for iframe (this is the most common player embed)
        iframe = soup.select_one("iframe[src]")
        if iframe:
            iframe_src = iframe.get("src", "")
            if iframe_src:
                iframe_src = normalize_url(iframe_src, self.BASE_URL)
                player_url = iframe_src
                if DEBUG:
                    logger.info(f"[UZMOVI] Found iframe/player URL: {iframe_src}")
                
                # CRITICAL: Fetch the iframe page and extract actual video URL
                # The iframe src is NOT the video - it's a player page
                iframe_video = self._extract_video_from_iframe_page(iframe_src, url)
                
                if iframe_video:
                    video_urls.extend(iframe_video)
                    if DEBUG:
                        logger.info(f"[UZMOVI] Successfully extracted video from iframe")
                else:
                    if DEBUG:
                        logger.info(f"[UZMOVI] WARNING: Could not extract video from iframe - no playable URL found")
        
        # Step 2: Look for video elements on the page itself
        # [ENHANCED] Also check for <video> tags with src attribute
        video_elements = soup.find_all("video")
        for video in video_elements:
            src = video.get("src", "")
            if src:
                if DEBUG:
                    logger.info(f"[UZMOVI] Found <video src>: {src}")
                video_urls.append({
                    "quality": "unknown",
                    "url": normalize_url(src, self.BASE_URL),
                    "type": "html5_video"
                })
            
            # Also check for <source> children inside <video>
            sources = video.find_all("source")
            for source in sources:
                src = source.get("src", "")
                if src:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Found <video><source src>: {src}")
                    video_urls.append({
                        "quality": source.get("label", "auto"),
                        "url": normalize_url(src, self.BASE_URL),
                        "type": "html5_source"
                    })
        
        # [ENHANCED] Direct regex extraction for uzmovi-specific video servers
        # Pattern: https://srv*.uzdown.space/...
        html_text = str(soup)
        srv_pattern = r'https://srv\d+\.uzdown\.space/[^\s"\'<>]+'
        srv_matches = re.findall(srv_pattern, html_text)
        for match in srv_matches:
            if DEBUG:
                logger.info(f"[UZMOVI] Found srv*.uzdown.space URL: {match}")
            video_urls.append({
                "quality": "auto",
                "url": match,
                "type": "srv_direct"
            })
        
        # Step 3: Look for source tags (without video parent)
        source_elements = soup.select("video source[src], source[src]")
        for source in source_elements:
            src = source.get("src", "")
            if src and self._is_playable_video_url(src):
                video_urls.append({
                    "quality": source.get("label", "unknown"),
                    "url": normalize_url(src, self.BASE_URL),
                    "type": "html5_source"
                })
                if DEBUG:
                    logger.info(f"[UZMOVI] Found source element: {src}")
        
        # Step 4: Look for direct download links
        direct_links = soup.select("a[href*='.mp4'], a[href*='.m3u8']")
        for link in direct_links:
            href = link.get("href", "")
            if href and self._is_playable_video_url(href):
                video_urls.append({
                    "quality": "direct",
                    "url": normalize_url(href, self.BASE_URL),
                    "type": "direct_download"
                })
                if DEBUG:
                    logger.info(f"[UZMOVI] Found direct download link: {href}")
        
        # Step 5: Look for JavaScript-based video sources
        script_urls = self._extract_video_from_page_scripts(soup, url)
        video_urls.extend(script_urls)
        
        # Deduplicate and filter - ONLY keep actual playable video URLs
        # [FIX] Use allow_player=True for player_fallback types since they contain 'player'/'embed'
        seen = set()
        unique_video_urls = []
        for v in video_urls:
            url_key = v["url"]
            url_type = v.get("type", "")
            # Allow player URLs for fallback type
            allow_player = (url_type == "player_fallback")
            if url_key not in seen and url_key and self._is_playable_video_url(url_key, allow_player):
                seen.add(url_key)
                unique_video_urls.append(v)
        
        if DEBUG:
            logger.info(f"[UZMOVI] === VIDEO EXTRACTION SUMMARY ===")
            logger.info(f"[UZMOVI] Detail page: {url}")
            logger.info(f"[UZMOVI] Player URL: {player_url}")
            logger.info(f"[UZMOVI] Found {len(unique_video_urls)} playable video URLs:")
            for v in unique_video_urls:
                logger.info(f"[UZMOVI]   type={v['type']}, url={v['url'][:100]}")
        
        # [CRITICAL] If still no video_urls but we have a player_url, add it as fallback
        # This ensures video_urls is NEVER empty when there's a player
        if not unique_video_urls and player_url:
            if DEBUG:
                logger.info(f"[UZMOVI] CRITICAL: Adding player_url as final fallback: {player_url}")
            unique_video_urls.append({
                "quality": "unknown",
                "url": player_url,
                "type": "player_fallback"
            })
        
        # [ENHANCED] If STILL no video_urls, scan entire HTML for ANY video-related URLs
        # This is a last resort fallback - the downloader will validate which are playable
        if not unique_video_urls:
            if DEBUG:
                logger.info(f"[UZMOVI] No video_urls found yet - scanning full HTML for candidates...")
            
            # Get the raw HTML to scan
            try:
                # Fetch page again for raw HTML scan
                response = self.session.get(url, timeout=30, allow_redirects=True)
                html_content = response.text
                
                # Scan ALL URLs in HTML using regex
                all_url_pattern = r'https?://[^\s"\'<>]+'
                all_urls = re.findall(all_url_pattern, html_content)
                
                if DEBUG:
                    logger.info(f"[UZMOVI] Found {len(all_urls)} total URLs in HTML")
                
                # Keep candidates containing video-related keywords
                video_keywords = ['.mp4', '.m3u8', '.webm', 'video', 'stream', 'play', 
                                'media', 'cdn', 'file', 'watch', 'embed', 'player']
                
                for candidate in all_urls:
                    # Skip data URLs and very short URLs
                    if candidate.startswith('data:') or len(candidate) < 20:
                        continue
                    
                    # Check if it contains any video-related keyword
                    candidate_lower = candidate.lower()
                    if any(keyword in candidate_lower for keyword in video_keywords):
                        # Skip obviously non-video URLs
                        if '/css/' in candidate_lower or '/js/' in candidate_lower or '/img/' in candidate_lower:
                            continue
                        if '/film/' in candidate_lower or '/movie/' in candidate_lower and '.mp4' not in candidate_lower:
                            continue
                        
                        if DEBUG:
                            logger.info(f"[UZMOVI] Found video candidate: {candidate[:100]}")
                        
                        unique_video_urls.append({
                            "quality": "unknown",
                            "url": candidate,
                            "type": "html_scan_candidate"
                        })
                
                if DEBUG and unique_video_urls:
                    logger.info(f"[UZMOVI] Added {len(unique_video_urls)} URLs from HTML scan")
                    
            except Exception as e:
                if DEBUG:
                    logger.info(f"[UZMOVI] HTML scan failed: {e}")
        
        # Extract source_id
        source_id = extract_source_id(url)
        
        # Determine type via shared detector (URL + soup signals)
        from helpers import detect_content_type as _detect_ct
        movie_type, _ct_reason = _detect_ct(url, "uzmovi", soup=soup)
        if movie_type == "unknown":
            # Conservative legacy fallback for previously-working uzmovi flow
            movie_type = "serial" if ("/serial/" in url or "/tv-series/" in url) else "movie"
            _ct_reason = "fallback by legacy url heuristic"
        logger.info(f"[PARSER] detected content_type={movie_type} reason={_ct_reason} url={url}")
        
        return MovieDetails(
            title=title,
            description=description,
            poster=poster,
            backdrop=backdrop,
            year=year,
            genres=genres,
            country=country,
            duration=duration,
            video_page_url=url,
            player_url=player_url,
            video_urls=unique_video_urls,
            source=self.source_name,
            source_id=source_id,
            type=movie_type
        )
        
        # Debug: log final extraction result
        logger.info(f"[UZMOVI] FINAL: title={title}, video_urls_count={len(unique_video_urls)}, player_url={player_url or 'none'}")
        if not unique_video_urls:
            logger.warning(f"[UZMOVI] WARNING: No playable video URLs found for {url}")
    
    def _is_playable_video_url(self, url: str, allow_player: bool = False) -> bool:
        """
        Check if URL is a playable video file (not a page URL).
        
        This method now uses the new media_extractor for comprehensive validation.
        
        Args:
            url: The URL to check
            allow_player: If True, accept player/embed URLs (DEPRECATED - should not be used)
        """
        # Use the new media_extractor for proper validation
        # The allow_player parameter is deprecated - player pages are HTML, not video
        from media_extractor import is_valid_media_url, classify_media_url
        
        if not url:
            return False

        parsed = urlparse(url)
        if "{" in url or "}" in url or not parsed.netloc:
            if DEBUG:
                logger.info(f"[UZMOVI] URL rejected as malformed: {url[:100]}")
            return False
        if "/embed/" in parsed.path.lower() and not any(ext in url.lower() for ext in [".m3u8", ".mp4", ".mpd", ".ism"]):
            if DEBUG:
                logger.info(f"[UZMOVI] URL rejected as player page, not media: {url[:100]}")
            return False
        
        # Use enhanced validation from media_extractor
        is_valid, reason = is_valid_media_url(url)
        
        if not is_valid:
            if DEBUG:
                logger.info(f"[UZMOVI] URL rejected by media_extractor: {reason}")
            return False
        
        # Must have a recognized media type
        media_type = classify_media_url(url)
        if media_type in ['html', 'unknown']:
            if DEBUG:
                logger.info(f"[UZMOVI] URL has invalid type: {media_type}")
            return False
        
        return True
    
    def _extract_video_from_iframe_page(self, iframe_url: str, page_url: str) -> List[Dict[str, str]]:
        """
        CRITICAL: Fetch the iframe/player page and extract actual video URL.
        The iframe src is a PLAYER PAGE, not the video itself.
        Enhanced with base64 decoding, URL decoding, and external player script support.
        """
        if DEBUG:
            logger.info(f"[UZMOVI] === FETCHING IFRAME PAGE ===")
            logger.info(f"[UZMOVI] Player URL: {iframe_url}")
            logger.info(f"[UZMOVI] Page URL (referer): {page_url}")
        
        video_urls = []
        iframe_final_url = iframe_url
        
        try:
            # Fetch iframe page with proper Referer header
            headers = {
                "Referer": page_url,
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            }
            
            response = self.session.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
            iframe_final_url = response.url
            
            if DEBUG:
                logger.info(f"[UZMOVI] Iframe final URL: {iframe_final_url}")
                logger.info(f"[UZMOVI] Content-Type: {response.headers.get('Content-Type', 'unknown')}")
            
            # Check content type
            content_type = response.headers.get("Content-Type", "")
            
            # If it's not HTML, it might be direct video
            if "text/html" not in content_type:
                if DEBUG:
                    logger.info(f"[UZMOVI] Non-HTML response - checking if it's video")
                if any(vid_type in content_type for vid_type in ["video", "octet", "stream"]):
                    if self._is_playable_video_url(iframe_final_url):
                        video_urls.append({
                            "quality": "unknown",
                            "url": iframe_final_url,
                            "type": "direct_from_iframe"
                        })
                        if DEBUG:
                            logger.info(f"[UZMOVI] SUCCESS: Direct video from iframe: {iframe_final_url}")
                return video_urls
            
            # Parse HTML to find video
            soup = BeautifulSoup(response.content, "lxml")
            
            # [ENHANCED] Check for video element
            video = soup.select_one("video[src]")
            if video:
                src = video.get("src", "")
                if src and self._is_playable_video_url(src):
                    src = normalize_url(src, iframe_final_url)
                    video_urls.append({
                        "quality": "unknown",
                        "url": src,
                        "type": "html5_video_from_iframe"
                    })
                    if DEBUG:
                        logger.info(f"[UZMOVI] SUCCESS: Found video src in iframe: {src}")
            
            # [ENHANCED] Check source elements
            sources = soup.select("source[src]")
            for source in sources:
                src = source.get("src", "")
                if src and self._is_playable_video_url(src):
                    src = normalize_url(src, iframe_final_url)
                    video_urls.append({
                        "quality": source.get("label", "unknown"),
                        "url": src,
                        "type": "html5_source_from_iframe"
                    })
                    if DEBUG:
                        logger.info(f"[UZMOVI] SUCCESS: Found source in iframe: {src}")
            
            # [ENHANCED] Extract from JavaScript in iframe page with improved patterns
            script_videos = self._extract_video_from_scripts_content(response.text, iframe_final_url)
            video_urls.extend(script_videos)
            
            # [ENHANCED] Try to extract video from data attributes
            data_video = self._extract_from_data_attributes(soup, iframe_final_url)
            video_urls.extend(data_video)
            
            # [ENHANCED] Try URL parameters that might contain video URL
            param_videos = self._extract_from_url_params(soup, iframe_final_url)
            video_urls.extend(param_videos)
            
            # [ENHANCED] If still no video found, try to fetch external player scripts
            if not video_urls:
                script_urls = soup.select("script[src]")
                for script in script_urls:
                    src = script.get("src", "")
                    if src and any(player in src.lower() for player in ["player", "video", "cdn", "stream"]):
                        src = normalize_url(src, iframe_final_url)
                        if DEBUG:
                            logger.info(f"[UZMOVI] Found player script: {src}")
                        # Try to fetch and parse the player script
                        external_videos = self._fetch_external_player_script(src, iframe_final_url)
                        video_urls.extend(external_videos)
            
            if DEBUG:
                if video_urls:
                    logger.info(f"[UZMOVI] SUCCESS: Found {len(video_urls)} video URL(s) from iframe")
                    for v in video_urls:
                        logger.info(f"[UZMOVI]   - {v['type']}: {v['url'][:80]}...")
                else:
                    logger.info(f"[UZMOVI] FAILED: No playable video URL found in iframe page")
                    logger.info(f"[UZMOVI] Will use player_url as fallback")
        
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] ERROR fetching iframe page: {e}")
        
        # [CRITICAL FIX] DO NOT add iframe URL directly as video URL
        # The iframe URL is a PLAYER PAGE (HTML), not a video
        # The downloader CANNOT handle HTML page URLs
        # If no actual video URL is found, return empty list and let caller handle failure
        if not video_urls:
            if DEBUG:
                logger.warning(f"[UZMOVI] No actual video URL found in iframe page: {iframe_final_url}")
                logger.warning(f"[UZMOVI] NOT adding iframe page URL as video - this would cause downloader failure")
            # Return empty list - let the caller know we failed
            return []
        
        return video_urls
    
    def _extract_video_from_page_scripts(self, soup, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from JavaScript in the page"""
        video_urls = []
        
        for script in soup.select("script"):
            content = script.string or ""
            if content and len(content) > 50:
                videos = self._extract_video_from_scripts_content(content, base_url)
                video_urls.extend(videos)
        
        return video_urls
    
    def _extract_video_from_scripts_content(self, content: str, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from JavaScript content"""
        video_urls = []
        
        if not content or len(content) < 100:
            return video_urls
        
        if DEBUG:
            # Only log first 200 chars for debugging
            sample = content[:200].replace('\n', ' ')
            logger.info(f"[UZMOVI] Script content sample: {sample}...")
        
        # Comprehensive patterns for video URLs in JavaScript
        patterns = [
            # Direct URL patterns
            r'["\']([^"\']*\.mp4)["\']',
            r'["\']([^"\']*\.m3u8)["\']',
            r'(https?://[^\s"\'<>]+\.mp4)',
            r'(https?://[^\s"\'<>]+\.m3u8)',
            # Object/Config patterns
            r'"src"\s*[:=]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
            r'"file"\s*[:=]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
            r'"url"\s*[:=]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
            r'"video"\s*[:=]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
            # Playlist/source patterns
            r'"sources"\s*:\s*\[\s*\{[^}]*?"src"\s*:\s*"([^"]+\.(?:mp4|m3u8))',
            r'<source[^>]+src=["\']([^"\']+\.(?:mp4|m3u8))["\']',
            # Common player configurations
            r'(?:file|src|url|video)\s*[=:]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
        ]
        
        for pattern in patterns:
            try:
                matches = re.findall(pattern, content, re.IGNORECASE)
                for match in matches:
                    if match and not match.startswith("data:"):
                        # Additional filtering
                        if not self._is_playable_video_url(match):
                            continue
                        
                        url = normalize_url(match, base_url)
                        if DEBUG:
                            logger.info(f"[UZMOVI] SUCCESS: Found video in script: {url}")
                        
                        video_urls.append({
                            "quality": "unknown",
                            "url": url,
                            "type": "script_extracted"
                        })
            except re.error:
                continue
        
        # [ENHANCED] Try base64 decoding for encoded URLs
        decoded_urls = self._extract_decoded_urls(content, base_url)
        video_urls.extend(decoded_urls)
        
        # [ENHANCED] Try URL decoding for double-encoded URLs
        url_decoded_urls = self._extract_url_decoded_urls(content, base_url)
        video_urls.extend(url_decoded_urls)
        
        return video_urls
    
    def _extract_from_data_attributes(self, soup, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from data-* attributes in HTML elements"""
        video_urls = []
        
        if DEBUG:
            logger.info(f"[UZMOVI] Checking data attributes for video URLs...")
        
        # Find all elements with data-* attributes that might contain video
        data_patterns = ["data-src", "data-video", "data-url", "data-file", "data-player", 
                        "data-src-0", "data-src-1", "data-quality"]
        
        for pattern in data_patterns:
            elements = soup.select(f"[{pattern}]")
            for elem in elements:
                value = elem.get(pattern, "")
                if value and self._is_playable_video_url(value):
                    url = normalize_url(value, base_url)
                    video_urls.append({
                        "quality": "unknown",
                        "url": url,
                        "type": f"data_attribute_{pattern}"
                    })
                    if DEBUG:
                        logger.info(f"[UZMOVI] Found video in {pattern}: {url}")
        
        return video_urls
    
    def _extract_from_url_params(self, soup, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from URL parameters in elements"""
        video_urls = []
        
        if DEBUG:
            logger.info(f"[UZMOVI] Checking URL parameters for video URLs...")
        
        # Look for elements with URL-like attributes
        url_attributes = ["href", "data-href", "data-url", "data-link", "data-play", "onclick"]
        
        for attr in url_attributes:
            elements = soup.select(f"[{attr}]")
            for elem in elements:
                value = elem.get(attr, "")
                if value and any(ext in value.lower() for ext in [".mp4", ".m3u8", ".webm"]):
                    if self._is_playable_video_url(value):
                        url = normalize_url(value, base_url)
                        video_urls.append({
                            "quality": "unknown",
                            "url": url,
                            "type": f"url_param_{attr}"
                        })
                        if DEBUG:
                            logger.info(f"[UZMOVI] Found video in {attr}: {url}")
        
        return video_urls
    
    def _fetch_external_player_script(self, script_url: str, base_url: str) -> List[Dict[str, str]]:
        """Fetch external player script and extract video URLs"""
        video_urls = []
        
        if DEBUG:
            logger.info(f"[UZMOVI] Fetching external player script: {script_url}")
        
        try:
            headers = {
                "Referer": base_url,
                "Accept": "*/*",
            }
            response = self.session.get(script_url, headers=headers, timeout=15)
            
            if response.status_code == 200:
                content = response.text
                if len(content) > 100:
                    videos = self._extract_video_from_scripts_content(content, script_url)
                    video_urls.extend(videos)
                    if DEBUG and videos:
                        logger.info(f"[UZMOVI] Found {len(videos)} video(s) in external script")
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] Failed to fetch external script: {e}")
        
        return video_urls
    
    def _extract_decoded_urls(self, content: str, base_url: str) -> List[Dict[str, str]]:
        """Extract and decode base64-encoded video URLs"""
        video_urls = []
        
        # Look for base64-encoded strings that might contain video URLs
        b64_patterns = [
            r'["\']([A-Za-z0-9+/=]{20,})["\']',  # Generic base64 strings
            r'data:text/html;base64,([A-Za-z0-9+/=]+)',
        ]
        
        for pattern in b64_patterns:
            try:
                matches = re.findall(pattern, content)
                for match in matches:
                    try:
                        # Try to decode as base64
                        if match.startswith('data:'):
                            # data URL format
                            continue  # Skip for now
                        
                        decoded = base64.b64decode(match).decode('utf-8', errors='ignore')
                        
                        # Check if decoded content contains video URLs
                        if '.mp4' in decoded.lower() or '.m3u8' in decoded.lower():
                            # Extract URLs from decoded content
                            url_patterns = [
                                r'(https?://[^\s"\'<>]+\.(?:mp4|m3u8))',
                                r'["\']([^"\']+\.(?:mp4|m3u8))["\']',
                            ]
                            for url_pat in url_patterns:
                                urls = re.findall(url_pat, decoded, re.IGNORECASE)
                                for url in urls:
                                    if self._is_playable_video_url(url):
                                        url = normalize_url(url, base_url)
                                        video_urls.append({
                                            "quality": "unknown",
                                            "url": url,
                                            "type": "base64_decoded"
                                        })
                                        if DEBUG:
                                            logger.info(f"[UZMOVI] Found base64 decoded video: {url}")
                    except Exception:
                        continue
            except Exception:
                continue
        
        return video_urls
    
    def _extract_url_decoded_urls(self, content: str, base_url: str) -> List[Dict[str, str]]:
        """Extract URL-decoded video URLs (double-encoded)"""
        video_urls = []
        
        # Look for URL-encoded strings
        encoded_patterns = [
            r'%5B[^\s%]+%5D',  # URL-encoded array-like strings
            r'%2F[^\s%]+%2E(?:mp4|m3u8)',  # URL-encoded paths with extensions
        ]
        
        for pattern in encoded_patterns:
            try:
                matches = re.findall(pattern, content)
                for match in matches:
                    try:
                        decoded = urllib.parse.unquote(match)
                        decoded = urllib.parse.unquote(decoded)  # Double decode
                        
                        if '.mp4' in decoded.lower() or '.m3u8' in decoded.lower():
                            if self._is_playable_video_url(decoded):
                                url = normalize_url(decoded, base_url)
                                video_urls.append({
                                    "quality": "unknown",
                                    "url": url,
                                    "type": "url_decoded"
                                })
                                if DEBUG:
                                    logger.info(f"[UZMOVI] Found URL-decoded video: {url}")
                    except Exception:
                        continue
            except Exception:
                continue
        
        return video_urls
    
    def _extract_video_from_iframe(self, iframe_url: str, page_url: str) -> List[Dict[str, str]]:
        """
        Fetch iframe page and extract actual video URL.
        This is critical because iframe src is usually a player page, not the video itself.
        """
        if DEBUG:
            logger.info(f"[UZMOVI] Fetching iframe page: {iframe_url}")
        
        video_urls = []
        
        try:
            # Fetch iframe page with Referer header
            headers = {
                "Referer": page_url,
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            }
            
            response = self.session.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
            iframe_final_url = response.url
            
            if DEBUG:
                logger.info(f"[UZMOVI] Iframe final URL: {iframe_final_url}")
                logger.info(f"[UZMOVI] Iframe Content-Type: {response.headers.get('Content-Type', 'unknown')}")
            
            # Check if we got HTML or something else
            content_type = response.headers.get("Content-Type", "")
            if "text/html" not in content_type and "application/xhtml" not in content_type:
                # This might be direct video content
                if "video" in content_type or "octet-stream" in content_type:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Iframe returned video content directly!")
                    video_urls.append({
                        "quality": "unknown",
                        "url": iframe_final_url,
                        "type": "direct_mp4"
                    })
                return video_urls
            
            soup = BeautifulSoup(response.content, "lxml")
            
            # Try to find video element
            video = soup.select_one("video[src], video source[src]")
            if video:
                src = video.get("src") or (video.find("source") and video.find("source").get("src"))
                if src:
                    src = normalize_url(src, iframe_url)
                    if DEBUG:
                        logger.info(f"[UZMOVI] Found video src in iframe: {src}")
                    video_urls.append({
                        "quality": "unknown",
                        "url": src,
                        "type": "html5_video"
                    })
            
            # Try to extract from JavaScript
            script_videos = self._extract_video_from_content(response.text, iframe_final_url)
            video_urls.extend(script_videos)
            
            # Try common patterns
            # Pattern 1: file: "https://..."
            file_patterns = [
                r'["\']file["\']\s*:\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
                r'["\']src["\']\s*:\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
                r'"url"\s*:\s*"([^"]+\.(?:mp4|m3u8))"',
                r'"sources"\s*:\s*\[\{[^}]*?"src"\s*:\s*"([^"]+\.(?:mp4|m3u8))',
            ]
            
            for pattern in file_patterns:
                matches = re.findall(pattern, response.text, re.IGNORECASE)
                for match in matches:
                    if match and not match.startswith("data:"):
                        url = normalize_url(match, iframe_final_url)
                        if DEBUG:
                            logger.info(f"[UZMOVI] Found video via regex: {url}")
                        video_urls.append({
                            "quality": "unknown",
                            "url": url,
                            "type": "extracted"
                        })
            
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] Error fetching iframe: {e}")
        
        return video_urls
    
    def _extract_video_from_scripts(self, soup) -> List[Dict[str, str]]:
        """Extract video URLs from JavaScript code in the page"""
        video_urls = []
        
        # Find all script tags
        scripts = soup.select("script")
        
        for script in scripts:
            content = script.string or ""
            if content:
                videos = self._extract_video_from_content(content, self.BASE_URL)
                video_urls.extend(videos)
        
        return video_urls
    
    def _extract_video_from_content(self, content: str, base_url: str) -> List[Dict[str, str]]:
        """Extract video URLs from any text content (HTML or JavaScript)"""
        video_urls = []
        
        # Skip data URLs and empty
        if not content or len(content) < 100:
            return video_urls
        
        # Video file extensions
        video_extensions = [".mp4", ".m3u8", ".webm", ".mkv", ".avi"]
        
        # Pattern to find video URLs
        # Match URLs in quotes or after src/file/etc.
        patterns = [
            # Direct URLs with extensions
            r'["\']([^"\']*\.mp4)["\']',
            r'["\']([^"\']*\.m3u8)["\']',
            r'(https?://[^\s"\'<>]+\.mp4)',
            r'(https?://[^\s"\'<>]+\.m3u8)',
            # JSON-like patterns
            r'"src"\s*:\s*"([^"]+\.(?:mp4|m3u8))"',
            r'"file"\s*:\s*"([^"]+\.(?:mp4|m3u8))"',
            r'"url"\s*:\s*"([^"]+\.(?:mp4|m3u8))"',
            # HTML video/src attributes
            r'src=["\']([^"\']+\.(?:mp4|m3u8))["\']',
            # JavaScript variable assignments
            r'(?:video|src|file)\s*[=:]\s*["\']([^"\']+\.(?:mp4|m3u8))["\']',
        ]
        
        for pattern in patterns:
            try:
                matches = re.findall(pattern, content, re.IGNORECASE)
                for match in matches:
                    if match and not match.startswith("data:"):
                        # Skip common non-video URLs
                        skip_patterns = ["thumbnail", "poster", "image", "avatar", "logo", "icon", "css", ".js"]
                        if any(skip in match.lower() for skip in skip_patterns):
                            continue
                        
                        url = normalize_url(match, base_url)
                        if DEBUG:
                            logger.info(f"[UZMOVI] Content extraction found: {url}")
                        
                        video_urls.append({
                            "quality": "unknown",
                            "url": url,
                            "type": "script_extracted"
                        })
            except re.error:
                continue
        
        return video_urls
    
    # =========================================================================
    # UNIVERSAL MEDIA EXTRACTION HELPERS
    # =========================================================================
    
    def detect_media_type(self, url: str) -> str:
        """
        Detect media type from URL extension.
        Returns: 'mp4', 'm3u8', 'mpd', or 'unknown'
        """
        url_lower = url.lower()
        if '.mp4' in url_lower:
            return 'mp4'
        elif '.m3u8' in url_lower:
            return 'm3u8'
        elif '.mpd' in url_lower:
            return 'mpd'
        elif 'application/x-mpegurl' in url_lower or 'hls' in url_lower:
            return 'm3u8'
        elif 'dash+xml' in url_lower or 'mpd' in url_lower:
            return 'mpd'
        return 'unknown'
    
    def normalize_media_url(self, url: str, base_url: str = "") -> str:
        """
        Normalize a media URL by:
        - Converting relative URLs to absolute
        - Cleaning query parameters that are not part of the media URL
        - Trimming whitespace
        """
        if not url:
            return ""
        
        url = url.strip()
        
        # Skip data URLs
        if url.startswith('data:'):
            return ""
        
        # Convert relative URLs to absolute
        if base_url and not url.startswith('http'):
            url = normalize_url(url, base_url)
        
        return url
    
    def _extract_media_from_video_tags(self, html_text: str, base_url: str) -> List[Dict[str, str]]:
        """
        Extract media URLs from <video> and <source> tags in HTML.
        Supports: mp4, m3u8, mpd
        """
        media_urls = []
        
        if not html_text:
            return media_urls
        
        try:
            soup = BeautifulSoup(html_text, "lxml")
            
            # Find all video elements
            videos = soup.find_all("video")
            for video in videos:
                # Check video src
                src = video.get("src", "")
                if src:
                    media_type = self.detect_media_type(src)
                    if media_type != 'unknown':
                        normalized = self.normalize_media_url(src, base_url)
                        if normalized:
                            if DEBUG:
                                logger.info(f"[UZMOVI] Found video[src]: {normalized} ({media_type})")
                            media_urls.append({
                                "url": normalized,
                                "quality": video.get("data-quality", "auto"),
                                "type": media_type
                            })
                
                # Check source children
                sources = video.find_all("source")
                for source in sources:
                    src = source.get("src", "")
                    if src:
                        media_type = self.detect_media_type(src)
                        if media_type != 'unknown':
                            normalized = self.normalize_media_url(src, base_url)
                            if normalized:
                                if DEBUG:
                                    logger.info(f"[UZMOVI] Found video>source[src]: {normalized} ({media_type})")
                                media_urls.append({
                                    "url": normalized,
                                    "quality": source.get("label", source.get("data-quality", "auto")),
                                    "type": media_type
                                })
            
            if DEBUG and media_urls:
                mp4_count = sum(1 for m in media_urls if m['type'] == 'mp4')
                m3u8_count = sum(1 for m in media_urls if m['type'] == 'm3u8')
                mpd_count = sum(1 for m in media_urls if m['type'] == 'mpd')
                logger.info(f"[UZMOVI] Video tags: mp4={mp4_count}, m3u8={m3u8_count}, mpd={mpd_count}")
                
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] Error extracting from video tags: {e}")
        
        return media_urls
    
    def _extract_media_from_regex(self, html_text: str, base_url: str) -> List[Dict[str, str]]:
        """
        Extract media URLs using regex patterns.
        Prioritizes: srv*.uzdown.space URLs with mp4/m3u8/mpd
        Enhanced with more comprehensive patterns.
        """
        media_urls = []
        
        if not html_text or len(html_text) < 100:
            return media_urls
        
        if DEBUG:
            logger.info(f"[UZMOVI] Starting enhanced regex extraction for uzmovi URLs")
        
        # Priority patterns for uzmovi video servers (most specific first)
        # [ENHANCED] Added more patterns for uzmovi-specific URL structures
        priority_patterns = [
            # srv*.uzdown.space with any media extension - uzmovi's main video host
            r'https://srv\d+\.uzdown\.space/[^\s"\'<>]+',
            # Also match without digits (some patterns might vary)
            r'https://srv\.uzdown\.space/[^\s"\'<>]+',
            # All .mpd URLs
            r'https://[^\s"\'<>]+\.mpd[^\s"\'<>]*',
            # All .m3u8 URLs
            r'https?://[^\s"\'<>]+\.m3u8[^\s"\'<>]*',
            # All .mp4 URLs
            r'https?://[^\s"\'<>]+\.mp4[^\s"\'<>]*',
            # vimeo URLs
            r'https://player\.vimeo\.com/video/\d+[^\s"\'<>]*',
            # vk.com video URLs
            r'https://vk\.com/video[^\s"\'<>]+',
        ]
        
        # uzmovi-specific patterns - these handle the specific URL structures
        uzmovi_specific_patterns = [
            # Pattern for URLs containing "live" directory (common in uzmovi)
            r'https://[^\s"\'<>]*uzdown[^\s"\'<>]*/live/[^\s"\'<>]+',
            # Pattern for URLs with uzmovi domain
            r'https://[^\s"\'<>]*uzmovi[^\s"\'<>]*\.(m3u8|mpd|mp4)[^\s"\'<>]*',
            # Pattern for any URL containing "uzmovi" with video extensions
            r'https://[^\s"\'<>]*uzmovi[^\s"\'<>]+',
            # Pattern for any URL containing "uzdown" with video extensions
            r'https://[^\s"\'<>]*uzdown[^\s"\'<>]+',
        ]
        
        # General media patterns
        general_patterns = [
            # Manifest URLs with any extension
            r'https?://[^\s"\'<>]*manifest[^\s"\'<>]*',
            # HLS/playlist patterns
            r'https?://[^\s"\'<>]*(?:playlist|index)[^\s"\'<>]*\.m3u8[^\s"\'<>]*',
            # Segment patterns
            r'https?://[^\s"\'<>]*segment[^\s"\'<>]*',
            # Chunk patterns
            r'https?://[^\s"\'<>]*chunk[^\s"\'<>]*',
            # vod/stream patterns
            r'https?://[^\s"\'<>]*vod[^\s"\'<>]*',
            # stream patterns
            r'https?://[^\s"\'<>]*stream[^\s"\'<>]*',
            # [ENHANCED] Pattern for video.js, jwplayer, and other player configurations
            r'(?:file|source|src|videoUrl|mediaUrl)\s*[:=]\s*["\']https?://[^\s"\'<>]+["\']',
        ]
        
        all_patterns = priority_patterns + uzmovi_specific_patterns + general_patterns
        
        seen = set()
        raw_matches = []  # Track all raw matches for debugging
        
        for pattern in all_patterns:
            try:
                matches = re.findall(pattern, html_text, re.IGNORECASE)
                for match in matches:
                    # Handle tuple returns from regex groups
                    if isinstance(match, tuple):
                        match = match[0] if match[0] else match[-1]
                    
                    match = match.strip()
                    raw_matches.append(match)
                    
                    if not match or match.startswith('data:') or match in seen:
                        continue
                    
                    # Skip obvious non-media URLs
                    skip_patterns = ['/css/', '/js/', '/img/', '/font/', '.css?', '.js?', 'webmanifest', 
                                     'google-analytics', '/googletagmanager/', '/ads/', '/analytics/',
                                     '.json?', '/api/', '/embed/', '/wp-content/', '/wp-includes/']
                    if any(p in match.lower() for p in skip_patterns):
                        if DEBUG:
                            logger.info(f"[UZMOVI] Skipped non-media URL (skip pattern): {match[:80]}...")
                        continue
                    
                    # Skip URLs that are clearly HTML pages (contain /film/, /movie/, /serial/ with .html)
                    match_lower = match.lower()
                    if ('/film/' in match_lower or '/movie/' in match_lower or '/serial/' in match_lower) and '.html' in match_lower:
                        if DEBUG:
                            logger.info(f"[UZMOVI] Skipped HTML page URL: {match[:80]}...")
                        continue
                    
                    # Detect media type
                    media_type = self.detect_media_type(match)
                    
                    # [ENHANCED] If media_type is unknown, check if it's from a known video host
                    if media_type == 'unknown':
                        # Check for uzmovi/uzdown video hosts
                        if 'uzdown' in match_lower or 'uzmovi' in match_lower or 'srv' in match_lower:
                            # Check for specific video indicators
                            if any(ind in match_lower for ind in ['.m3u8', '.mpd', '.mp4', '/live/', '/video/', '/index/', '/playlist/', 'index.m3u8']):
                                media_type = self._detect_type_from_url(match_lower)
                                if DEBUG:
                                    logger.info(f"[UZMOVI] Detected type '{media_type}' from uzmovi host URL: {match[:80]}...")
                    
                    if media_type != 'unknown':
                        normalized = self.normalize_media_url(match, base_url)
                        if normalized and normalized not in seen:
                            seen.add(normalized)
                            if DEBUG:
                                logger.info(f"[UZMOVI] ACCEPTED: {normalized} ({media_type})")
                            media_urls.append({
                                "url": normalized,
                                "quality": "auto",
                                "type": media_type
                            })
                    else:
                        if DEBUG:
                            logger.info(f"[UZMOVI] REJECTED (unknown type): {match[:80]}...")
            except re.error:
                continue
        
        # Log raw matches for debugging
        if DEBUG:
            logger.info(f"[UZMOVI] === REGEX EXTRACTION DEBUG ===")
            logger.info(f"[UZMOVI] Total raw matches: {len(raw_matches)}")
            for i, m in enumerate(raw_matches[:10]):  # Log first 10
                logger.info(f"[UZMOVI]   Raw[{i}]: {m[:100]}...")
        
        # Count by type
        if DEBUG and media_urls:
            mp4_count = sum(1 for m in media_urls if m['type'] == 'mp4')
            m3u8_count = sum(1 for m in media_urls if m['type'] == 'm3u8')
            mpd_count = sum(1 for m in media_urls if m['type'] == 'mpd')
            logger.info(f"[UZMOVI] Regex extraction result: mp4={mp4_count}, m3u8={m3u8_count}, mpd={mpd_count}")
        
        return media_urls
    
    def _detect_type_from_url(self, url_lower: str) -> str:
        """
        Detect media type from URL string.
        Enhanced to handle uzmovi-specific patterns.
        """
        if '.m3u8' in url_lower:
            return 'm3u8'
        if '.mpd' in url_lower:
            return 'mpd'
        if '.mp4' in url_lower:
            return 'mp4'
        if '.ism' in url_lower:
            return 'ism'
        
        # [ENHANCED] Check for uzmovi video indicators
        if '/live/' in url_lower or 'uzdown' in url_lower:
            # uzmovi typically uses m3u8 for live streams
            if 'uzmovi' in url_lower or 'srv' in url_lower:
                return 'm3u8'  # Default to m3u8 for uzmovi URLs
        
        return 'unknown'
    
    def _dedupe_media_urls(self, media_urls: List[Dict[str, str]]) -> List[Dict[str, str]]:
        """
        Deduplicate media URLs while preserving all unique types.
        If both mp4 and m3u8 exist, keep both.
        
        IMPORTANT: Validates URLs using is_valid_media_url from media_extractor
        to ensure only valid stream URLs are returned.
        
        [FIXED] Now uses both isValidStreamUrl and is_valid_media_url for thorough validation.
        [ENHANCED] Added comprehensive logging for debugging URL filtering.
        """
        seen = set()
        unique = []
        invalid_rejected = 0
        duplicates_skipped = 0
        accepted_count = 0
        
        logger.info(f"[UZMOVI] === DEDUPLICATION START ===")
        logger.info(f"[UZMOVI] Input: {len(media_urls)} URLs")
        
        # Also log raw input URLs for debugging
        if DEBUG:
            logger.info(f"[UZMOVI] === RAW INPUT URLs ===")
            for i, media in enumerate(media_urls):
                url = media.get("url", "")
                url_type = media.get("type", "unknown")
                logger.info(f"[UZMOVI]   Input[{i}]: type={url_type}, url={url[:120]}...")
        
        for media in media_urls:
            url = media.get("url", "")
            
            # Skip empty URLs
            if not url:
                logger.info(f"[UZMOVI]   SKIP: Empty URL")
                continue

            parsed = urlparse(url)
            if "{" in url or "}" in url or not parsed.netloc:
                logger.info(f"[UZMOVI]   REJECTED (malformed URL): {url[:100]}... (type: {media.get('type', 'unknown')})")
                invalid_rejected += 1
                continue
            if "/embed/" in parsed.path.lower() and not any(ext in url.lower() for ext in [".m3u8", ".mp4", ".mpd", ".ism"]):
                logger.info(f"[UZMOVI]   REJECTED (player/embed page): {url[:100]}... (type: {media.get('type', 'unknown')})")
                invalid_rejected += 1
                continue

            media_type = classify_media_url(url)
            if media_type in ["mp4", "m3u8", "mpd", "ism"]:
                media["type"] = media_type
            
            # Skip duplicate URLs
            if url in seen:
                logger.info(f"[UZMOVI]   DUPLICATE: {url[:80]}...")
                duplicates_skipped += 1
                continue
            
            # CRITICAL: Validate URL is a real stream URL, not HTML page
            # First try the faster isValidStreamUrl
            if not isValidStreamUrl(url):
                logger.info(f"[UZMOVI]   REJECTED (isValidStreamUrl=False): {url[:100]}... (type: {media.get('type', 'unknown')})")
                invalid_rejected += 1
                continue
            
            # Also check with the more thorough is_valid_media_url
            is_valid, reason = is_valid_media_url(url)
            if not is_valid:
                logger.info(f"[UZMOVI]   REJECTED (is_valid_media_url): {reason}, url={url[:100]}... (type: {media.get('type', 'unknown')})")
                invalid_rejected += 1
                continue
            
            # URL is valid - add it
            seen.add(url)
            unique.append(media)
            accepted_count += 1
            logger.info(f"[UZMOVI]   ACCEPTED: {url[:100]}... (type: {media.get('type', 'unknown')})")
        
        logger.info(f"[UZMOVI] === DEDUPLICATION SUMMARY ===")
        logger.info(f"[UZMOVI]   Input total: {len(media_urls)}")
        logger.info(f"[UZMOVI]   Duplicates skipped: {duplicates_skipped}")
        logger.info(f"[UZMOVI]   Invalid rejected: {invalid_rejected}")
        logger.info(f"[UZMOVI]   Accepted: {accepted_count}")
        logger.info(f"[UZMOVI]   Final unique URLs: {len(unique)}")
        
        if unique:
            logger.info(f"[UZMOVI] === FINAL VALID URLs ===")
            for i, u in enumerate(unique):
                logger.info(f"[UZMOVI]   Final[{i}]: type={u.get('type', 'unknown')}, url={u.get('url', '')[:120]}...")
        
        if invalid_rejected > 0:
            logger.warning(f"[UZMOVI] Filtered out {invalid_rejected} invalid URL(s) during deduplication")
        
        return unique
    
    def _extract_media_with_playwright(self, url: str) -> List[Dict[str, str]]:
        """
        Universal media extraction using Playwright headless browser.
        
        Extraction strategy:
        1. Launch headless Chromium
        2. Open the page and wait for JS execution
        3. Extract from rendered DOM:
           - video.src, video.currentSrc
           - all <source src>
           - all URLs from DOM containing media extensions
        4. Scan JavaScript runtime via page.evaluate()
        5. Filter, deduplicate, and return
        """
        media_urls = []
        
        try:
            if DEBUG:
                logger.info(f"[UZMOVI] === PLAYWRIGHT FALLBACK TRIGGERED ===")
                logger.info(f"[UZMOVI] Opening page in headless Chromium: {url}")
            
            # Import here to make Playwright optional
            from playwright.sync_api import sync_playwright
            
            with sync_playwright() as p:
                # Launch headless Chromium
                browser = p.chromium.launch(headless=True)
                context = browser.new_context(
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                )
                page = context.new_page()
                
                # Navigate to the page - use domcontentloaded (NOT networkidle - uzmovi keeps connections alive)
                if DEBUG:
                    logger.info(f"[UZMOVI] Navigating to page...")
                page.goto(url, wait_until="domcontentloaded", timeout=60000)
                
                # Wait 7 seconds for JS to execute and video player to initialize
                if DEBUG:
                    logger.info(f"[UZMOVI] Waiting 7 seconds for JS execution...")
                page.wait_for_timeout(7000)
                
                # Try to wait for video or source elements to appear
                if DEBUG:
                    logger.info(f"[UZMOVI] Waiting for video/source elements...")
                try:
                    page.wait_for_selector("video, source", timeout=10000)
                    if DEBUG:
                        logger.info(f"[UZMOVI] Video/source elements appeared!")
                except Exception as selector_error:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Selector wait error (non-fatal): {selector_error}")
                
                if DEBUG:
                    logger.info(f"[UZMOVI] Extracting media from rendered DOM...")
                
                # === A. Extract from video elements ===
                try:
                    videos = page.query_selector_all("video")
                    for video in videos:
                        # video.src
                        src = video.get_attribute("src")
                        if src:
                            self._add_media_url(media_urls, src, "auto")
                        
                        # video.currentSrc (may differ from src)
                        current_src = video.get_attribute("currentSrc")
                        if current_src and current_src != src:
                            self._add_media_url(media_urls, current_src, "auto")
                        
                        # All source children
                        sources = video.query_selector_all("source")
                        for source in sources:
                            src = source.get_attribute("src")
                            if src:
                                quality = source.get_attribute("label") or source.get_attribute("data-quality") or "auto"
                                self._add_media_url(media_urls, src, quality)
                except Exception as e:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Error extracting video elements: {e}")
                
                # === B. Extract all <source> tags anywhere ===
                try:
                    sources = page.query_selector_all("source")
                    for source in sources:
                        src = source.get_attribute("src")
                        if src:
                            quality = source.get_attribute("label") or source.get_attribute("data-quality") or "auto"
                            self._add_media_url(media_urls, src, quality)
                except Exception as e:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Error extracting sources: {e}")
                
                # === C. Scan DOM for URLs containing media extensions ===
                try:
                    html_content = page.content()
                    regex_media = self._extract_media_from_regex(html_content, url)
                    for m in regex_media:
                        self._add_media_url(media_urls, m['url'], m.get('quality', 'auto'))
                except Exception as e:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Error in regex extraction: {e}")
                
                # === D. JavaScript runtime scanning via page.evaluate() ===
                try:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Running JS runtime scan...")
                    
                    # JavaScript to extract ALL media-related strings from the page
                    # Enhanced to scan scripts for media URLs
                    js_script = '''() => {
                        const results = [];
                        const seen = new Set();
                        
                        // Check all media elements
                        document.querySelectorAll("video, audio, source").forEach(el => {
                            const src = el.src || el.currentSrc || "";
                            if (src && !seen.has(src)) {
                                seen.add(src);
                                results.push({ url: src, type: "dom_media" });
                            }
                        });
                        
                        // Check source elements
                        document.querySelectorAll("source").forEach(el => {
                            const src = el.src || "";
                            if (src && !seen.has(src)) {
                                seen.add(src);
                                results.push({ url: src, type: "dom_source" });
                            }
                        });
                        
                        // Scan ALL text content for media URLs (including scripts)
                        const pageText = document.documentElement.innerHTML;
                        const urlPattern = /https?:\\/\\/[\\w\\-\\.]+(?:\\/[\\w\\-\\.\\/?=&%]+)+(?:\\.mp4|\\.m3u8|\\.mpd|\\/srv\\d*|uzdown)/gi;
                        const matches = pageText.match(urlPattern) || [];
                        matches.forEach(url => {
                            if (url && !seen.has(url)) {
                                seen.add(url);
                                results.push({ url: url, type: "scanned" });
                            }
                        });
                        
                        // Scan scripts for video configuration objects
                        document.querySelectorAll("script").forEach(script => {
                            const content = script.textContent || "";
                            // Look for video source patterns in JavaScript
                            const patterns = [
                                /["\']https?:\\/\\/[\\w\\-\\.]+(?:\\/[\\w\\-\\.\\/?=&%]+)+(?:\\.mp4|\\.m3u8|\\.mpd)/gi,
                                /file\\s*:\\s*["\']([^"\']+\\.(?:mp4|m3u8|mpd))/gi,
                                /src\\s*:\\s*["\']([^"\']+\\.(?:mp4|m3u8|mpd))/gi,
                                /videoUrl\\s*:\\s*["\']([^"\']+)/gi,
                                /video_url\\s*:\\s*["\']([^"\']+)/gi,
                            ];
                            patterns.forEach(pattern => {
                                let match;
                                while ((match = pattern.exec(content)) !== null) {
                                    let url = match[1] || match[0];
                                    if (!url.startsWith("http")) {
                                        url = "https:" + url;
                                    }
                                    if (url && !seen.has(url)) {
                                        seen.add(url);
                                        results.push({ url: url, type: "script" });
                                    }
                                }
                            });
                        });
                        
                        return results;
                    }'''
                    
                    js_results = page.evaluate(js_script)
                    
                    for item in js_results:
                        url = item.get('url', '')
                        if url:
                            self._add_media_url(media_urls, url, "auto")
                    
                    if DEBUG:
                        logger.info(f"[UZMOVI] JS runtime found {len(js_results)} candidates")
                        for item in js_results[:5]:
                            logger.info(f"[UZMOVI]   JS result: {item.get('type', 'unknown')} - {item.get('url', '')[:80]}...")
                        
                except Exception as e:
                    if DEBUG:
                        logger.info(f"[UZMOVI] Error in JS runtime scan: {e}")
                
                browser.close()
            
            # Deduplicate
            media_urls = self._dedupe_media_urls(media_urls)
            
            if DEBUG:
                mp4_count = sum(1 for m in media_urls if m['type'] == 'mp4')
                m3u8_count = sum(1 for m in media_urls if m['type'] == 'm3u8')
                mpd_count = sum(1 for m in media_urls if m['type'] == 'mpd')
                logger.info(f"[UZMOVI] Playwright found {len(media_urls)} media URLs")
                logger.info(f"[UZMOVI]   mp4: {mp4_count}, m3u8: {m3u8_count}, mpd: {mpd_count}")
                logger.info(f"[UZMOVI] === PLAYWRIGHT FALLBACK COMPLETE ===")
            
        except ImportError:
            if DEBUG:
                logger.info(f"[UZMOVI] Playwright not installed, skipping browser fallback")
        except Exception as e:
            if DEBUG:
                logger.info(f"[UZMOVI] Playwright fallback error: {e}")
        
        return media_urls
    
    def _add_media_url(self, media_urls: List[Dict], url: str, quality: str = "auto"):
        """
        Helper to add media URL with validation and filtering.
        Rejects .js, .css, webmanifest, etc.
        """
        if not url or url.startswith('data:'):
            return
        
        url = url.strip()
        if len(url) < 15:  # Too short to be a real URL
            return
        
        # Reject obvious non-media URLs
        skip_patterns = ['.js', '.css', 'webmanifest', 'analytics', '/ads/', 'google-analytics', 'googletagmanager']
        url_lower = url.lower()
        if any(p in url_lower for p in skip_patterns):
            return
        
        # Must contain media extension or known media host
        media_patterns = ['.mp4', '.m3u8', '.mpd', 'srv', 'uzdown', 'video', 'stream', 'media']
        if not any(p in url_lower for p in media_patterns):
            return
        
        # Detect media type
        media_type = self.detect_media_type(url)
        if media_type == 'unknown':
            # Check for srv/uzdown hosts
            if 'srv' in url_lower or 'uzdown' in url_lower:
                media_type = 'mp4'  # Default to mp4 for uzmovi hosts
        
        media_urls.append({
            "url": url,
            "quality": quality,
            "type": media_type
        })
    
    def _extract_all_media_from_page(self, url: str) -> List[Dict[str, str]]:
        """
        Universal media extraction from a page for uzmovi.
        
        Extraction pipeline:
        1. Try specialized uzmovi extraction using extract_media_for_source
        2. Also try regex extraction for srv*.uzdown.space URLs
        3. Check for iframes and try to extract from them
        4. Fallback: Playwright headless browser (if fast path finds no media)
        
        [FIXED] Now uses specialized extract_from_uzmovi function from media_extractor
        to properly handle uzmovi-specific video patterns.
        [ENHANCED] Comprehensive logging throughout the extraction process.
        """
        media_urls = []
        
        logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        logger.info(f"[UZMOVI] _extract_all_media_from_page() STARTING")
        logger.info(f"[UZMOVI] Page URL: {url}")
        logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        
        # === STEP 1: Fetch the page ===
        html_content = ""
        final_url = url
        try:
            logger.info(f"[UZMOVI] Fetching page: {url}")
            
            response = self.session.get(url, timeout=30, allow_redirects=True)
            html_content = response.text
            final_url = response.url
            
            logger.info(f"[UZMOVI] Page fetched successfully")
            logger.info(f"[UZMOVI] Final URL: {final_url}")
            logger.info(f"[UZMOVI] HTML content length: {len(html_content)} chars")
            
            # Show samples of the HTML content (around where video might be)
            if DEBUG:
                for keyword in ['uzdown', 'srv', 'm3u8', 'mp4', 'uzmovi', 'player', 'iframe', 'video', 'source']:
                    if keyword.lower() in html_content.lower():
                        idx = html_content.lower().find(keyword.lower())
                        sample = html_content[max(0, idx-100):idx+300]
                        logger.info(f"[UZMOVI] HTML sample around '{keyword}': ...{sample[:200]}...")
                        break
            
        except Exception as e:
            logger.error(f"[UZMOVI] Page fetch error: {e}")
            import traceback
            logger.error(f"[UZMOVI] Traceback: {traceback.format_exc()}")
            return media_urls
        
        # === STEP 2: Try specialized uzmovi extraction from media_extractor ===
        try:
            logger.info(f"[UZMOVI] === STEP 2: Specialized uzmovi extraction ===")
            
            # Parse HTML
            soup = BeautifulSoup(html_content, "lxml")
            
            # Use the specialized extract_from_uzmovi function
            uzmovi_candidates = extract_from_uzmovi(soup, final_url)
            
            logger.info(f"[UZMOVI] Specialized extraction found {len(uzmovi_candidates)} raw candidates")
            script_candidate_count = sum(1 for c in uzmovi_candidates if "script" in getattr(c, "source_hint", ""))
            logger.info(f"[UZMOVI] script candidates found - {script_candidate_count}")
            
            # Convert candidates to dict format
            accepted_count = 0
            for candidate in uzmovi_candidates:
                url_val = candidate.url
                if url_val and not url_val.startswith('data:'):
                    media_type = candidate.type
                    quality = candidate.quality or "auto"
                    
                    # Validate URL
                    is_valid, reason = is_valid_media_url(url_val)
                    if is_valid:
                        media_urls.append({
                            "url": url_val,
                            "quality": quality,
                            "type": media_type
                        })
                        accepted_count += 1
                        logger.info(f"[UZMOVI] ACCEPTED (media_extractor): type={media_type}, url={url_val[:120]}...")
                        logger.info(f"[UZMOVI] extracted url - pattern={candidate.source_hint}, url={url_val[:120]}")
                    else:
                        logger.info(f"[UZMOVI] REJECTED (media_extractor): {reason}, url={url_val[:120]}...")
            
            logger.info(f"[UZMOVI] Specialized extraction: {accepted_count}/{len(uzmovi_candidates)} candidates accepted")
            
        except Exception as e:
            logger.error(f"[UZMOVI] Specialized extraction error: {e}")
            import traceback
            logger.error(f"[UZMOVI] Traceback: {traceback.format_exc()}")
        
        # === STEP 3: Also try regex extraction for srv*.uzdown.space URLs ===
        try:
            logger.info(f"[UZMOVI] === STEP 3: Regex extraction ===")
            
            regex_media = self._extract_media_from_regex(html_content, final_url)
            logger.info(f"[UZMOVI] Regex extraction result: {len(regex_media)} URLs")
            
            added_count = 0
            for m in regex_media:
                url_val = m.get('url', '')
                if url_val:
                    # Check if we already have this URL
                    existing_urls = [x.get('url', '') for x in media_urls]
                    if url_val not in existing_urls:
                        media_urls.append(m)
                        added_count += 1
                        logger.info(f"[UZMOVI] ADDED (regex): type={m.get('type')}, url={url_val[:120]}...")
            
            logger.info(f"[UZMOVI] Regex added {added_count} new URLs")
            
        except Exception as e:
            logger.error(f"[UZMOVI] Regex extraction error: {e}")
        
        # === STEP 4: Check for iframes and try to extract from them ===
        try:
            soup = BeautifulSoup(html_content, "lxml")
            iframes = soup.select("iframe[src]")
            
            logger.info(f"[UZMOVI] === STEP 4: Iframe extraction ===")
            logger.info(f"[UZMOVI] Found {len(iframes)} iframe(s) in page")
            
            iframe_added = 0
            for iframe in iframes:
                iframe_src = iframe.get("src", "")
                if iframe_src:
                    iframe_src = normalize_url(iframe_src, final_url)
                    logger.info(f"[UZMOVI] Processing iframe: {iframe_src}")
                    
                    # Try to fetch and extract from iframe page
                    iframe_videos = self._extract_video_from_iframe_page(iframe_src, final_url)
                    if iframe_videos:
                        for v in iframe_videos:
                            url_val = v.get('url', '')
                            if url_val:
                                existing_urls = [x.get('url', '') for x in media_urls]
                                if url_val not in existing_urls:
                                    media_urls.append(v)
                                    iframe_added += 1
                                    logger.info(f"[UZMOVI] ADDED (iframe): type={v.get('type')}, url={url_val[:120]}...")
                    else:
                        logger.info(f"[UZMOVI] No video found in iframe page: {iframe_src}")
            
            logger.info(f"[UZMOVI] Iframe extraction added {iframe_added} URLs")
            
        except Exception as e:
            logger.error(f"[UZMOVI] Iframe extraction error: {e}")
        
        # === STEP 5: Deduplicate ===
        logger.info(f"[UZMOVI] === STEP 5: Deduplication ===")
        logger.info(f"[UZMOVI] Before deduplication: {len(media_urls)} URLs")
        
        media_urls = self._dedupe_media_urls(media_urls)
        
        logger.info(f"[UZMOVI] After deduplication: {len(media_urls)} URLs")
        
        # === STEP 6: Fallback to Playwright if no URLs found ===
        if not media_urls:
            logger.warning(f"[UZMOVI] === FALLBACK: Playwright browser extraction ===")
            logger.warning(f"[UZMOVI] No URLs found from fast path, trying Playwright...")
            
            playwright_media = self._extract_media_with_playwright(url)
            logger.info(f"[UZMOVI] Playwright extraction result: {len(playwright_media)} URLs")
            
            if playwright_media:
                media_urls.extend(playwright_media)
                media_urls = self._dedupe_media_urls(media_urls)
                logger.info(f"[UZMOVI] After Playwright fallback: {len(media_urls)} URLs")
            else:
                logger.error(f"[UZMOVI] Playwright also failed to find any media URLs!")
        else:
            logger.info(f"[UZMOVI] Fast path found {len(media_urls)} URLs, skipping Playwright fallback")
        
        # === FINAL SUMMARY ===
        mp4_count = sum(1 for m in media_urls if m['type'] == 'mp4')
        m3u8_count = sum(1 for m in media_urls if m['type'] == 'm3u8')
        mpd_count = sum(1 for m in media_urls if m['type'] == 'mpd')
        logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        logger.info(f"[UZMOVI] === FINAL MEDIA SUMMARY ===")
        logger.info(f"[UZMOVI] Total media URLs: {len(media_urls)}")
        logger.info(f"[UZMOVI]   mp4: {mp4_count}")
        logger.info(f"[UZMOVI]   m3u8: {m3u8_count}")
        logger.info(f"[UZMOVI]   mpd: {mpd_count}")
        for m in media_urls:
            logger.info(f"[UZMOVI]   FINAL: type={m['type']}, url={m['url'][:120]}...")
        logger.info(f"[UZMOVI] ═══════════════════════════════════════════")
        
        # Return all valid media URLs
        return media_urls
    
    # =========================================================================
    # DEBUG FUNCTIONS
    # =========================================================================
    
    def debug_playwright(self, url: str):
        """
        Debug function to test Playwright extraction directly.
        Opens page with Playwright, waits, and extracts media.
        
        Usage:
            python uzmovi.py debug-playwright "<url>"
        """
        import json
        
        print(f"[DEBUG] === PLAYWRIGHT DEBUG MODE ===")
        print(f"[DEBUG] URL: {url}")
        
        try:
            from playwright.sync_api import sync_playwright
            print(f"[DEBUG] Playwright imported successfully")
        except ImportError:
            print(f"[DEBUG] ERROR: Playwright not installed!")
            print(f"[DEBUG] Install with: pip install playwright && playwright install chromium")
            return None
        
        try:
            print(f"[DEBUG] Launching Chromium...")
            with sync_playwright() as p:
                browser = p.chromium.launch(headless=True)
                print(f"[DEBUG] Chromium launched")
                
                context = browser.new_context(
                    user_agent="Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                )
                page = context.new_page()
                
                print(f"[DEBUG] Navigating to: {url}")
                page.goto(url, wait_until="domcontentloaded", timeout=60000)
                print(f"[DEBUG] Waiting 7 seconds for JS execution (NOT networkidle - uzmovi keeps connections alive)...")
                page.wait_for_timeout(7000)
                
                print(f"[DEBUG] Trying to wait for video/source elements...")
                try:
                    page.wait_for_selector("video, source", timeout=10000)
                    print(f"[DEBUG] Video/source elements appeared!")
                except Exception as e:
                    print(f"[DEBUG] Selector wait error (non-fatal): {e}")
                
                print(f"[DEBUG] Extracting media from rendered DOM...")
                
                # Extract video.src
                videos_src = []
                videos = page.query_selector_all("video")
                print(f"[DEBUG] Found {len(videos)} <video> elements")
                for i, video in enumerate(videos):
                    src = video.get_attribute("src")
                    current_src = video.get_attribute("currentSrc")
                    print(f"[DEBUG]   Video {i}: src={src}, currentSrc={current_src}")
                    if src:
                        videos_src.append({"element": "video", "attribute": "src", "url": src})
                    if current_src:
                        videos_src.append({"element": "video", "attribute": "currentSrc", "url": current_src})
                
                # Extract all source.src
                sources = page.query_selector_all("source")
                sources_src = []
                print(f"[DEBUG] Found {len(sources)} <source> elements")
                for i, source in enumerate(sources):
                    src = source.get_attribute("src")
                    print(f"[DEBUG]   Source {i}: src={src}")
                    if src:
                        sources_src.append({"element": "source", "attribute": "src", "url": src})
                
                # Run JS to find media URLs
                print(f"[DEBUG] Running JS media scanner...")
                js_script = """
                () => {
                    const results = [];
                    const seen = new Set();
                    
                    // Keywords
                    const keywords = ['.mp4', '.m3u8', '.mpd', 'srv', 'uzdown'];
                    
                    // Check all media elements
                    document.querySelectorAll('video, source').forEach(el => {
                        const src = el.src || el.currentSrc || '';
                        if (src && !seen.has(src)) {
                            seen.add(src);
                            results.push({ url: src, source: 'dom_media' });
                        }
                    });
                    
                    // Scan scripts
                    document.querySelectorAll('script').forEach(script => {
                        const content = script.textContent || '';
                        keywords.forEach(kw => {
                            if (content.toLowerCase().includes(kw)) {
                                const matches = content.match(/https?://[^"'\\s]+/g) || [];
                                matches.forEach(url => {
                                    if (!seen.has(url)) {
                                        seen.add(url);
                                        results.push({ url: url, source: 'script' });
                                    }
                                });
                            }
                        });
                    });
                    
                    return results;
                }
                """
                js_results = page.evaluate(js_script)
                print(f"[DEBUG] JS scanner found {len(js_results)} candidates")
                
                browser.close()
                
                # Combine and output
                all_results = videos_src + sources_src + js_results
                
                print(f"[DEBUG] === EXTRACTION RESULTS ===")
                print(f"[DEBUG] Total media found: {len(all_results)}")
                
                for item in all_results:
                    print(f"[DEBUG]   {item}")
                
                # Print as JSON
                print(f"[DEBUG] === JSON OUTPUT ===")
                print(json.dumps(all_results, indent=2))
                
                return all_results
                
        except Exception as e:
            print(f"[DEBUG] ERROR: {e}")
            import traceback
            traceback.print_exc()
            return None


    def list_categories(self):
        """Scrape genre/category links from uzmovi.tv navigation.

        uzmovi.tv uses Bootstrap dropdowns inside .navbar-nav.
        Genres live in the .dropdown-menu of the 'Janr' dropdown <li>.
        Top-level category links (Seriallar, etc.) are also included.
        """
        try:
            response = self.session.get(self.BASE_URL + "/", timeout=20)
            response.raise_for_status()
            soup = BeautifulSoup(response.text, "lxml")
        except Exception as e:
            logger.warning(f"[UZMOVI] list_categories: fetch failed: {e}")
            return []

        categories = []
        seen_urls = set()

        def _add(href, name):
            if not href or not name or len(name) < 2:
                return
            if href in ("#", "javascript:void(0)", ""):
                return
            if href.startswith("http") and not href.startswith(self.BASE_URL):
                return
            full_url = normalize_url(href, self.BASE_URL)
            if not full_url or full_url.rstrip("/") == self.BASE_URL.rstrip("/"):
                return
            if full_url in seen_urls:
                return
            seen_urls.add(full_url)
            slug = full_url.rstrip("/").split("/")[-1]
            categories.append({"name": name, "url": full_url, "slug": slug})

        # Strategy 1: find the 'Janr' dropdown in .navbar-nav and grab its menu links
        for li in soup.select(".navbar-nav li.dropdown"):
            toggle = li.select_one("a.dropdown-toggle")
            if not toggle:
                continue
            toggle_text = clean_text(toggle.get_text())
            # "Janr" in Uzbek nav header
            if "janr" not in toggle_text.lower() and "жанр" not in toggle_text.lower():
                continue
            logger.info(f"[UZMOVI] list_categories: found Janr dropdown, extracting menu links")
            for a in li.select(".dropdown-menu a"):
                _add(a.get("href", "").strip(), clean_text(a.get_text()))
            break

        # Strategy 2: also include direct top-level nav links (type filters like Seriallar)
        for li in soup.select(".navbar-nav > li:not(.dropdown)"):
            a = li.select_one("a")
            if a:
                _add(a.get("href", "").strip(), clean_text(a.get_text()))

        # Fallback: grab all .dropdown-menu links from navbar if nothing found yet
        if not categories:
            logger.warning(f"[UZMOVI] list_categories: Janr dropdown not found, falling back to all navbar links")
            for a in soup.select(".navbar-nav .dropdown-menu a"):
                href = a.get("href", "").strip()
                # Skip year/country xfsearch links to keep list manageable
                if "/xfsearch/" in href:
                    continue
                _add(href, clean_text(a.get_text()))

        logger.info(f"[UZMOVI] list_categories: found {len(categories)} categories")
        return categories

    def list_catalog(self, page=1, limit=20, type_filter="", category_url=""):
        """
        List movies from uzmovi.tv catalog with pagination.
        Scrapes the listing page and extracts movie cards.

        Args:
            page: Page number (1-based)
            limit: Max items per page
            type_filter: Optional filter ("movie", "serial", or empty for all)
            category_url: Optional category/genre URL to browse instead of homepage

        Returns:
            dict with items, page, limit, total, total_pages, has_more
        """
        # When the caller requests the serial list without picking a category,
        # auto-select a serial-named category from list_categories(). The homepage
        # is a mixed feed that is film-heavy, so relying on a post-filter yields 0
        # items. This routes the request to a real serial listing page.
        if type_filter == "serial" and not category_url:
            try:
                cats = self.list_categories() or []
                for c in cats:
                    name = (c.get("name") or "").lower()
                    curl = (c.get("url") or "")
                    if "serial" in name or "/serial" in curl.lower():
                        category_url = curl
                        logger.info(f"[UZMOVI] list_catalog: auto-selected serial category: name={c.get('name')!r}, url={category_url}")
                        break
                if not category_url:
                    logger.warning("[UZMOVI] list_catalog: no serial category found via list_categories()")
            except Exception as e:
                logger.warning(f"[UZMOVI] list_catalog: serial category auto-detect failed: {e}")

        # Build candidate URLs.
        # Uzmovi homepage does NOT support /page/N/ (returns 404).
        # Only category/xfsearch URLs have real paginated listings.
        if category_url:
            base = category_url.rstrip("/")
            candidate_urls = [f"{base}/page/{page}/"]
            if page == 1:
                candidate_urls.append(base + "/")
        else:
            if page > 1:
                # Homepage has no /page/N/ support — return empty immediately.
                logger.info(f"[UZMOVI] list_catalog: homepage has no pagination; page {page} returning empty")
                return {
                    "items": [], "page": page, "limit": limit,
                    "total": 0, "total_pages": page, "has_more": False
                }
            candidate_urls = [self.BASE_URL + "/"]

        soup = None
        for url in candidate_urls:
            logger.info(f"[UZMOVI] list_catalog: fetching {url}")
            try:
                response = self.session.get(url, timeout=30, headers={
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                })
                response.raise_for_status()
                soup = BeautifulSoup(response.text, "lxml")
                break
            except Exception as e:
                logger.warning(f"[UZMOVI] list_catalog: failed to fetch {url}: {e}")

        if soup is None:
            logger.error(f"[UZMOVI] list_catalog: all URLs failed for page {page}")
            return {
                "items": [], "page": page, "limit": limit,
                "total": 0, "total_pages": 0, "has_more": False
            }

        # Find movie cards using known selectors
        cards = []
        for selector in self.CARD_SELECTORS:
            cards = soup.select(selector)
            if cards:
                logger.info(f"[UZMOVI] list_catalog: found {len(cards)} cards with '{selector}'")
                break

        if not cards:
            # Broader detection: search for link-based cards inside main content
            content = soup.select_one("#dle-content, .content, #content, main, body")
            if content:
                # Look for elements containing links with typical movie URL patterns
                import re as _re
                all_links = content.find_all("a", href=_re.compile(r'/\d+-|\.(html|htm)'))
                seen_parents = set()
                for a in all_links:
                    parent = a.parent
                    parent_id = id(parent)
                    if parent_id not in seen_parents and parent.name in ("div", "article", "li"):
                        seen_parents.add(parent_id)
                        cards.append(parent)
                logger.info(f"[UZMOVI] list_catalog: link-based detection found {len(cards)} cards")
        
        items = []
        for card in cards:
            try:
                item = self._extract_catalog_card(card)
                if item and item.get("title"):
                    items.append(item)
            except Exception as e:
                logger.debug(f"[UZMOVI] list_catalog: error extracting card: {e}")
                continue
        
        logger.info(f"[UZMOVI] list_catalog: extracted {len(items)} items from page {page}")

        # Uzmovi serial cards often point to /serialar/... (not /serial/), so the
        # per-URL heuristic in _extract_catalog_card tags them as "movie". When the
        # scraped page is a serial listing, force page-level type on all items.
        if type_filter == "serial" or "serial" in (category_url or "").lower():
            for it in items:
                it["type"] = "serial"
            logger.info(f"[UZMOVI] list_catalog: forced type=serial on {len(items)} items (serial listing page)")

        if type_filter:
            items = [i for i in items if i.get("type") == type_filter]
            logger.info(f"[UZMOVI] list_catalog: {len(items)} items after type_filter={type_filter!r}")

        # Check for next page using Uzmovi-specific selectors.
        # Uzmovi uses .pages (not .pagination/.navigation) and "Keyingi" for "Next" in Uzbek.
        has_more = False
        pagination = soup.select_one(".pages, .navigation, .pagination, .pager, .page-nav, #bottom-nav")
        if pagination:
            for link in pagination.select("a"):
                href = link.get("href", "")
                text = link.get_text(strip=True)
                if (
                    "next" in text.lower()
                    or "keyingi" in text.lower()
                    or "»" in text
                    or "›" in text
                    or f"/page/{page + 1}" in href
                    or f"/page/{page + 1}/" in href
                ):
                    has_more = True
                    break

        # Only apply has_more heuristic for category pages.
        # The Uzmovi homepage has no /page/N/ pagination — returning that heuristic as True
        # causes the Next button to appear and then load a 404 (empty page).
        if category_url and len(items) >= 10 and not has_more:
            has_more = True

        return {
            "items": items[:limit],
            "page": page,
            "limit": limit,
            "total": len(items),
            "total_pages": page + (1 if has_more else 0),
            "has_more": has_more,
        }
    
    def _extract_catalog_card(self, card):
        """Extract a catalog item from a movie card element."""
        title = ""
        detail_url = ""
        
        # Title extraction
        for sel in self.TITLE_SELECTORS:
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
        
        # Poster
        poster = ""
        for sel in self.IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("data-original") or img.get("src", "")
                if poster:
                    poster = normalize_url(poster, self.BASE_URL)
                break
        
        # Year
        year = 0
        for sel in self.YEAR_SELECTORS:
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

        # Quality
        quality = ""
        for sel in self.QUALITY_SELECTORS:
            el = card.select_one(sel)
            if el:
                quality = clean_text(el.get_text())
                break

        item_type = self._detect_uzmovi_type(detail_url=detail_url, title=title, card=card)

        return {
            "source_id": source_id,
            "title": title,
            "year": year,
            "type": item_type,
            "poster": poster,
            "description": desc,
            "genres": [],
            "detail_url": detail_url,
            "quality": quality,
        }


# CLI entry point
if __name__ == "__main__":
    import sys
    
    parser = UzmoviParser()
    
    if len(sys.argv) < 2:
        print("Usage: python uzmovi.py <command> [args]")
        print("Commands:")
        print("  search <query>        - Search for movies")
        print("  details <url>         - Get movie details")
        print("  debug-playwright <url> - Debug Playwright extraction")
        sys.exit(1)
    
    command = sys.argv[1]
    
    if command == "search":
        if len(sys.argv) < 3:
            print("Usage: python uzmovi.py search <query>")
            sys.exit(1)
        query = sys.argv[2]
        results = parser.search(query)
        import json
        print(json.dumps([r.to_dict() for r in results], indent=2, ensure_ascii=False))
    
    elif command == "details":
        if len(sys.argv) < 3:
            print("Usage: python uzmovi.py details <url>")
            sys.exit(1)
        url = sys.argv[2]
        details = parser.get_details(url)
        import json
        print(json.dumps(details.to_dict(), indent=2, ensure_ascii=False))
    
    elif command == "debug-playwright":
        if len(sys.argv) < 3:
            print("Usage: python uzmovi.py debug-playwright <url>")
            sys.exit(1)
        url = sys.argv[2]
        parser.debug_playwright(url)
    
    else:
        print(f"Unknown command: {command}")
        sys.exit(1)
