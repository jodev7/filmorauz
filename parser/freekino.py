"""
Freekino Parser - With strict stage-by-stage diagnosis
"""
import requests
import re
import logging
import os
from typing import List, Dict, Any, Optional
from urllib.parse import urljoin, quote
from bs4 import BeautifulSoup

from media_extractor import (
    is_valid_media_url,
    classify_media_url,
    validate_media_url_strict,
)

logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


# ============================================================
# HELPER FUNCTIONS
# ============================================================

def normalize_url(base: str, url: str) -> str:
    if not url:
        return ""
    url = url.strip()
    if url.startswith(("http://", "https://", "//")):
        return "https:" + url if url.startswith("//") else url
    return urljoin(base, url)


def clean_text(text: str) -> str:
    if not text:
        return ""
    return " ".join(text.split()).strip()


def extract_year(text: str) -> Optional[int]:
    if not text:
        return None
    match = re.search(r'\b(19|20)\d{2}\b', text)
    return int(match.group()) if match else None


def extract_quality(text: str) -> str:
    if not text:
        return ""
    text_lower = text.lower()
    if '1080' in text_lower: return "1080p"
    if '720' in text_lower: return "720p"
    if '480' in text_lower: return "480p"
    if 'hd' in text_lower: return "HD"
    return ""


def deduplicate_by_link(results: List[Dict]) -> List[Dict]:
    seen = set()
    unique = []
    for r in results:
        link = r.get("link", "")
        if link and link not in seen:
            seen.add(link)
            unique.append(r)
    return unique


def filter_by_query(query: str, results: List[Dict]) -> List[Dict]:
    """Filter with safe fallback - returns unfiltered if filtering removes all"""
    if not query or not results:
        return results
    
    # Normalize query
    query_norm = clean_text(query).lower()
    query_words = query_norm.split()
    
    scored = []
    for r in results:
        title = r.get("title", "")
        title_norm = clean_text(title).lower()
        
        score = 0
        if query_norm in title_norm:
            score = 100
        elif all(w in title_norm for w in query_words):
            score = 50
        else:
            matches = sum(1 for w in query_words if w in title_norm)
            score = matches * 10
        
        if score > 0:
            r["_score"] = score
            scored.append(r)
    
    scored.sort(key=lambda x: x.get("_score", 0), reverse=True)
    for r in scored:
        r.pop("_score", None)
    
    # SAFE FALLBACK: if filtering removed all but cards existed, return unfiltered
    if not scored and results:
        logger.warning(f"[FREEKINO] Filter removed all, returning {len(results)} unfiltered")
        return results
    
    return scored


# ============================================================
# PARSER CLASS
# ============================================================

class FreekinoParser:
    """Parser for freekino.net"""
    
    BASE_URL = "https://freekino.net"
    
    @property
    def source_name(self) -> str:
        return "freekino"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL
    
    # Container selectors (strict first)
    CONTAINER_SELECTORS = [
        ".search_results",
        ".search-results",
        ".xsearch",
        ".results",
        ".movie-list",
        ".film-list",
        "#content",
        ".content",
    ]
    
    # Card selectors (strict first)
    CARD_SELECTORS = [
        ".shortstory",
        ".shortstory-item",
        ".shortstoryItem",
        ".film-item",
        ".movie-item",
        ".moviebox",
        ".item",
        "article[class]",
        ".movie-item",
        "article.shortstory",
        ".search-result",
        ".movie-card",
        ".search-item",
        ".movie-list .movie",
    ]
    
    # Title selectors
    TITLE_SELECTORS = [
        "h2 a", "h3 a", ".title a", ".film-title a", 
        ".movie-title a", ".short-title a", "a[title]"
    ]
    
    IMAGE_SELECTORS = ["img[data-src]", "img[data-lazy-src]", "img[src]"]
    
    def __init__(self):
        self.session = requests.Session()
        self.session.headers.update({
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36",
            "Accept": "text/html,application/xhtml+xml",
            "Referer": "https://freekino.net/",
        })
    
    def _is_valid_title(self, title: str) -> bool:
        """
        Validate that extracted text is actually a movie title, not metadata.
        Rejects: "169 min", "HD", "2024", "0", single words without letters, etc.
        Accepts: "Watch Interstellar (2014)" -> extracts "Interstellar (2014)"
        """
        if not title:
            return False
        
        title = title.strip()
        
        # Reject empty after strip
        if not title:
            return False
        
        # Strip common prefixes like "Watch ", "Смотреть ", etc.
        import re
        title = re.sub(r'^(Watch |Смотреть |Watch online |Смотреть онлайн )', '', title, flags=re.IGNORECASE)
        
        # Reject if it's just numbers (like year, duration, rating)
        if title.isdigit():
            return False
        
        # Reject if it's just a year pattern (4 digits)
        if len(title) == 4 and title.isdigit() and 1900 <= int(title) <= 2030:
            return False
        
        # Reject if it's just duration like "169 min", "1h 30m", etc.
        lower = title.lower()
        if "min" in lower or "m" == lower[-1] if lower else False:
            # Check if it's like "169 min" or "120m"
            if re.match(r'^\d+\s*(min|m)$', lower):
                return False
        
        # Reject if it's just quality like "HD", "1080p", "4K"
        if lower in ["hd", "4k", "8k", "1080p", "720p", "480p", "sd"]:
            return False
        
        # Reject single character or very short strings without spaces
        if len(title) < 3:
            return False
        
        # Must have at least one letter (not just numbers/symbols)
        if not any(c.isalpha() for c in title):
            return False
        
        # Reject if title is all uppercase single word (likely metadata)
        if len(title.split()) == 1 and title.isupper() and len(title) < 10:
            return False
        
        return True
    
    def search(self, query: str) -> List[Dict[str, Any]]:
        """
        Search with strict stage-by-stage diagnosis
        """
        results = []
        extracted_cards = []  # Track for fallback
        
        try:
            # === STAGE 1: URL construction ===
            search_url = f"{self.BASE_URL}/search?q={quote(query)}"
            logger.info(f"[FREEKINO] STAGE 1: Search URL = {search_url}")
            logger.info(f"[FREEKINO] STAGE 1: Query encoded = {quote(query)}")
            
            # === STAGE 2: HTTP request ===
            response = self.session.get(search_url, timeout=30, allow_redirects=True)
            final_url = response.url
            status = response.status_code
            
            logger.info(f"[FREEKINO] STAGE 2: Status = {status}")
            logger.info(f"[FREEKINO] STAGE 2: Final URL = {final_url}")
            logger.info(f"[FREEKINO] STAGE 2: HTML length = {len(response.text)}")
            
            # Parse HTML
            soup = BeautifulSoup(response.text, "lxml")
            
            # === STAGE 2b: Page title ===
            title_tag = soup.find("title")
            page_title = title_tag.get_text() if title_tag else "N/A"
            logger.info(f"[FREEKINO] STAGE 2b: Page title = {page_title}")
            
            # === STAGE 3: Container detection ===
            logger.info(f"[FREEKINO] STAGE 3: Detecting container...")
            container = None
            for sel in self.CONTAINER_SELECTORS:
                found = soup.select_one(sel)
                if found:
                    logger.info(f"[FREEKINO] STAGE 3: Found container: {sel}")
                    container = found
                    break
            
            if not container:
                logger.warning(f"[FREEKINO] STAGE 3: No container found, using full page")
                logger.info(f"[FREEKINO] STAGE 3: Attempted selectors: {self.CONTAINER_SELECTORS}")
                container = soup
            
            # === STAGE 4: Card extraction ===
            logger.info(f"[FREEKINO] STAGE 4: Finding cards...")
            cards = []
            for sel in self.CARD_SELECTORS:
                found = container.select(sel)
                if found:
                    logger.info(f"[FREEKINO] STAGE 4: Selector '{sel}' = {len(found)} cards")
                    cards = found
                    break
            
            if not cards:
                logger.warning(f"[FREEKINO] STAGE 4: No cards found with strict selectors")
                # Fallback: any film links
                all_links = container.find_all("a", href=True)
                cards = [a for a in all_links if "/film/" in a.get("href", "")]
                logger.info(f"[FREEKINO] STAGE 4: Fallback links = {len(cards)}")
            
            logger.info(f"[FREEKINO] STAGE 4: Total cards = {len(cards)}")
            
            # === STAGE 5: Field extraction ===
            logger.info(f"[FREEKINO] STAGE 5: Extracting fields...")
            
            for i, card in enumerate(cards):
                if i < 5:  # Log first 5
                    logger.info(f"[FREEKINO] STAGE 5: Card {i} HTML: {str(card)[:200]}")
                
                result = self._extract_card(card)
                if result:
                    title = result.get("title", "")
                    link = result.get("link", "")
                    
                    if i < 5:
                        logger.info(f"[FREEKINO] STAGE 5: Card {i} title='{title}', link='{link}'")
                    
                    # Only skip if title AND link are both missing
                    if not title and not link:
                        continue
                    
                    extracted_cards.append(result)
            
            logger.info(f"[FREEKINO] STAGE 5: Extracted {len(extracted_cards)} cards with title+link")
            
            # Store before filtering for fallback
            before_filter = len(extracted_cards)
            logger.info(f"[FREEKINO] STAGE 5: Before filter = {before_filter}")
            
            # === STAGE 6: Deduplication ===
            logger.info(f"[FREEKINO] STAGE 6: Deduplicating...")
            results = deduplicate_by_link(extracted_cards)
            logger.info(f"[FREEKINO] STAGE 6: After dedup = {len(results)}")
            
            before_filter = len(results)  # Update for filter fallback
            
            # === STAGE 7: Filtering ===
            logger.info(f"[FREEKINO] STAGE 7: Filtering by query...")
            titles_before = [r.get("title", "") for r in results[:5]]
            logger.info(f"[FREEKINO] STAGE 7: Titles before filter: {titles_before}")
            
            results = filter_by_query(query, results)
            
            titles_after = [r.get("title", "") for r in results[:5]]
            logger.info(f"[FREEKINO] STAGE 7: Titles after filter: {titles_after}")
            logger.info(f"[FREEKINO] STAGE 7: After filter = {len(results)}")
            
            # === FINAL SAFETY: if extracted cards existed but filter returned [], use extracted ===
            if not results and extracted_cards:
                logger.warning(f"[FREEKINO] FINAL: Filter returned [], using {len(extracted_cards)} extracted cards")
                results = extracted_cards
            
            logger.info(f"[FREEKINO] FINAL: Returning {len(results)} results")
            
        except requests.RequestException as e:
            logger.error(f"[FREEKINO] HTTP error: {e}")
        except Exception as e:
            logger.error(f"[FREEKINO] Error: {e}")
        
        return results
    
    def _extract_card(self, card) -> Optional[Dict[str, Any]]:
        """Extract title, link, image, year from card"""
        
        # Find title
        title = ""
        link = ""
        
        for sel in self.TITLE_SELECTORS:
            elem = card.select_one(sel)
            if elem:
                raw_title = clean_text(elem.get_text())
                # Validate title: reject metadata like "169 min", "HD", year-only, etc.
                if raw_title and self._is_valid_title(raw_title):
                    title = raw_title
                    link = elem.get("href", "")
                    if title and link:
                        break
        
        # Fallback: any film link - try multiple sources for title
        if not title or not link:
            film_links = card.find_all("a", href=True)
            for a in film_links:
                href = a.get("href", "")
                if "/film/" in href or "/movie/" in href:
                    link = href
                    # Try title attribute first (e.g., "Watch Interstellar (2014)")
                    raw_title = a.get("title", "")
                    if not raw_title:
                        # Try aria-label (e.g., "Watch Interstellar (2014)")
                        raw_title = a.get("aria-label", "")
                    if not raw_title:
                        # Last resort: text content (might be metadata like "169 min")
                        raw_title = a.get_text()
                    raw_title = clean_text(raw_title)
                    # Clean common prefixes
                    import re
                    raw_title = re.sub(r'^(Watch |Смотреть |Watch online |Смотреть онлайн )', '', raw_title, flags=re.IGNORECASE)
                    # Validate title in fallback too
                    if raw_title and self._is_valid_title(raw_title):
                        title = raw_title
                        break
        
        if not title or not link:
            return None

        # Also clean title in main path
        if title and title.startswith("Watch "):
            import re
            title = re.sub(r'^(Watch |Смотреть |Watch online |Смотреть онлайн )', '', title, flags=re.IGNORECASE)

        link = normalize_url(self.BASE_URL, link)
        
        # Image
        img = ""
        for sel in self.IMAGE_SELECTORS:
            img_elem = card.select_one(sel)
            if img_elem:
                img = img_elem.get("data-src") or img_elem.get("data-lazy-src") or img_elem.get("src", "")
                if img:
                    break
        
        if img:
            img = normalize_url(self.BASE_URL, img)
        
        # Year: Try to extract from title attribute (e.g., "Watch Interstellar (2014)")
        # Fall back to card text only if title attr doesn't have year
        year = ""
        film_link = card.select_one("a[href*='/movie/'], a[href*='/film/']")
        if film_link:
            title_attr = film_link.get("title", "")
            import re
            year_match = re.search(r'\((\d{4})\)', title_attr)
            if year_match:
                year = year_match.group(1)
        
        # Only use card text as fallback if we couldn't get from title attr
        if not year:
            card_text = card.get_text()
            extracted_year = extract_year(card_text)
            year = str(extracted_year) if extracted_year else ""
        
        quality = extract_quality(card.get_text()) if not year else ""
        
        # Extract source_id from the link
        # e.g., "https://freekino.net/movie/2631-interstellar" -> "2631"
        source_id = ""
        if link:
            import re
            match = re.search(r'/movie/(\d+)-', link)
            if match:
                source_id = f"freekino_{match.group(1)}"
            else:
                # Fallback: use the numeric ID if found
                match = re.search(r'/(\d+)/?', link)
                if match:
                    source_id = f"freekino_{match.group(1)}"
        
        return {
            "source": "freekino",
            "title": title,
            "link": link,
            "img": img,
            "year": year,  # Empty string if no year found - frontend handles this
            "quality": quality,
            "source_id": source_id,
            "detail_url": link,
        }
    
    def get_detail(self, url: str) -> Dict[str, Any]:
        """Get movie detail"""
        result = {
            "title": "", "video_url": "", "video_type": "",
            "poster": "", "description": "", "year": 2024,
            "genres": [], "type": "movie"
        }
        
        try:
            response = self.session.get(url, timeout=30)
            soup = BeautifulSoup(response.text, "lxml")
            
            # Title
            title_elem = soup.select_one("h1, .title, .film-title")
            if title_elem:
                result["title"] = clean_text(title_elem.get_text())
            
            # Poster
            og = soup.select_one("meta[property='og:image']")
            if og:
                result["poster"] = normalize_url(self.BASE_URL, og.get("content", ""))
            
            # Description
            desc = soup.select_one(".description, .desc, .synopsis, .text")
            if desc:
                result["description"] = clean_text(desc.get_text())
            
            # Year
            year_elem = soup.select_one(".year, [class*='year'], .date")
            if year_elem:
                year = extract_year(year_elem.get_text())
                if year:
                    result["year"] = year
            
            # Genres
            genre_elems = soup.select(".genres a, .genre a, [class*='genre'] a")
            result["genres"] = [clean_text(g.get_text()) for g in genre_elems if g.get_text()]
            
            # Type
            if "/serial/" in url:
                result["type"] = "serial"
            
            # Video
            video_url, video_type = self._extract_video(soup)
            result["video_url"] = video_url
            result["video_type"] = video_type
            
        except Exception as e:
            logger.error(f"[FREEKINO] Detail error: {e}")
        
        return result
    
    def _extract_video(self, soup):
        """Extract video URL with enhanced validation"""
        
        # iframe - fetch iframe and extract from it, don't return iframe URL directly
        iframe = soup.select_one('iframe[src*="player"]')
        if iframe:
            iframe_src = iframe.get("src", "")
            if iframe_src and not iframe_src.startswith("blob"):
                # Fetch iframe page and extract actual video
                video_url, video_type = self._extract_video_from_iframe(iframe_src)
                if video_url:
                    return video_url, video_type
                # If iframe fetch fails, don't return iframe URL (it's HTML)
                if DEBUG:
                    logger.info(f"[FREEKINO] No video found in iframe, not returning iframe URL")
        
        # video
        video = soup.select_one("video[src]")
        if video:
            src = video.get("src", "")
            if src and not src.startswith("blob"):
                # Validate URL
                error = validate_media_url_strict(src)
                if not error:
                    return src, classify_media_url(src)
                if DEBUG:
                    logger.info(f"[FREEKINO] Video URL rejected: {error}")
        
        # script regex
        for script in soup.find_all("script"):
            content = script.string or ""
            matches = re.findall(r'(?:file|src|url)["\']?\s*[:=]\s*["\']([^"\']+(?:\.mp4|\.m3u8|\.mpd))', content, re.I)
            for m in matches:
                if m and not m.startswith("blob"):
                    url = normalize_url(self.BASE_URL, m)
                    # Validate URL
                    error = validate_media_url_strict(url)
                    if error:
                        if DEBUG:
                            logger.info(f"[FREEKINO] Script URL rejected: {url[:60]}... Reason: {error}")
                        continue
                    return url, classify_media_url(url)
        
        return "", ""
    
    def _extract_video_from_iframe(self, iframe_url: str) -> tuple:
        """Fetch iframe page and extract video URL from it"""
        try:
            headers = {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            }
            response = requests.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
            
            if "text/html" in response.headers.get("Content-Type", ""):
                # Parse iframe HTML
                soup = BeautifulSoup(response.text, "lxml")
                video = soup.select_one("video[src]")
                if video:
                    src = video.get("src", "")
                    if src:
                        error = validate_media_url_strict(src)
                        if not error:
                            return src, classify_media_url(src)
                
                # Try script extraction
                for script in soup.find_all("script"):
                    content = script.string or ""
                    matches = re.findall(r'(?:file|src|url)["\']?\s*[:=]\s*["\']([^"\']+(?:\.mp4|\.m3u8|\.mpd))', content, re.I)
                    for m in matches:
                        if m and not m.startswith("blob"):
                            url = normalize_url(response.url, m)
                            error = validate_media_url_strict(url)
                            if not error:
                                return url, classify_media_url(url)
            else:
                # Direct media response
                return response.url, classify_media_url(response.url)
        except Exception as e:
            if DEBUG:
                logger.info(f"[FREEKINO] Iframe fetch error: {e}")
        
        return "", ""


    def list_categories(self):
        """Scrape genre/category links from freekino.net navigation."""
        try:
            response = self.session.get(self.BASE_URL + "/", timeout=20, headers={
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            })
            response.raise_for_status()
            soup = BeautifulSoup(response.text, "lxml")
        except Exception as e:
            logger.warning(f"[FREEKINO] list_categories: failed: {e}")
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
                full_url = normalize_url(self.BASE_URL, href)
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

        logger.info(f"[FREEKINO] list_categories: found {len(categories)} categories")
        return categories

    def list_catalog(self, page=1, limit=20, type_filter="", category_url=""):
        """
        List movies from freekino catalog with pagination.

        Args:
            page: Page number (1-based)
            limit: Max items per page
            type_filter: Optional filter
            category_url: Optional category/genre URL to browse instead of homepage

        Returns:
            dict with items, page, limit, total, total_pages, has_more
        """
        # Build candidate URLs
        if category_url:
            base = category_url.rstrip("/")
            candidate_urls = [f"{base}/page/{page}/"]
            if page == 1:
                candidate_urls.append(base + "/")
        else:
            # Try standard DLE pagination URL first, fall back to root for page 1
            candidate_urls = [f"{self.BASE_URL}/page/{page}/"]
            if page == 1:
                candidate_urls.append(self.BASE_URL + "/")

        soup = None
        for url in candidate_urls:
            logger.info(f"[FREEKINO] list_catalog: fetching {url}")
            try:
                response = self.session.get(url, timeout=30, headers={
                    "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                })
                response.raise_for_status()
                soup = BeautifulSoup(response.text, "lxml")
                break
            except Exception as e:
                logger.warning(f"[FREEKINO] list_catalog: failed to fetch {url}: {e}")

        if soup is None:
            logger.error(f"[FREEKINO] list_catalog: all URLs failed for page {page}")
            return {
                "items": [], "page": page, "limit": limit,
                "total": 0, "total_pages": 0, "has_more": False
            }

        # Find movie cards
        cards = []
        for selector in self.CARD_SELECTORS:
            cards = soup.select(selector)
            if cards:
                logger.info(f"[FREEKINO] list_catalog: found {len(cards)} cards with '{selector}'")
                break

        if not cards:
            # Broader detection: look for elements wrapping typical movie links
            content = soup.select_one("#dle-content, .content, #content, main, body")
            if content:
                import re as _re
                all_links = content.find_all("a", href=_re.compile(r'/movie/|/film/|/\d+-|\.(html|htm)'))
                seen_parents = set()
                for a in all_links:
                    parent = a.parent
                    parent_id = id(parent)
                    if parent_id not in seen_parents and parent.name in ("div", "article", "li"):
                        seen_parents.add(parent_id)
                        cards.append(parent)
                logger.info(f"[FREEKINO] list_catalog: link-based detection found {len(cards)} cards")
        
        items = []
        for card in cards:
            try:
                item = self._extract_catalog_card(card)
                if item and item.get("title"):
                    items.append(item)
            except Exception as e:
                logger.debug(f"[FREEKINO] list_catalog: error extracting card: {e}")
                continue
        
        logger.info(f"[FREEKINO] list_catalog: extracted {len(items)} items from page {page}")

        if type_filter:
            items = [i for i in items if i.get("type") == type_filter]


        # Check for next page
        has_more = False
        pagination = soup.select_one(".navigation, .pagination, .pager, .page-nav, #bottom-nav")
        if pagination:
            for link in pagination.select("a"):
                href = link.get("href", "")
                text = link.get_text(strip=True)
                if "next" in text.lower() or "»" in text or "›" in text or f"/page/{page + 1}" in href:
                    has_more = True
                    break
        
        if len(items) >= 10 and not has_more:
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
        
        for sel in self.TITLE_SELECTORS:
            el = card.select_one(sel)
            if el:
                raw_title = clean_text(el.get_text())
                if self._is_valid_title(raw_title):
                    title = raw_title
                    href = el.get("href", "")
                    if href:
                        detail_url = normalize_url(self.BASE_URL, href)
                    break
        
        if not title:
            for a in card.find_all("a"):
                text = clean_text(a.get_text())
                if text and len(text) > 3 and self._is_valid_title(text):
                    title = text
                    href = a.get("href", "")
                    if href:
                        detail_url = normalize_url(self.BASE_URL, href)
                    break
        
        if not title:
            return None
        
        # Poster
        poster = ""
        for sel in self.IMAGE_SELECTORS:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("src", "")
                if poster:
                    poster = normalize_url(self.BASE_URL, poster)
                break
        
        # Year
        year = extract_year(card.get_text()) or 0
        
        # Source ID from URL
        source_id = ""
        if detail_url:
            import re as _re
            # Try numeric ID first (e.g., /movie/2631-title -> "2631")
            m = _re.search(r'/(?:movie|film|serial)/(\d+)', detail_url)
            if m:
                source_id = m.group(1)
            else:
                # Fallback: use last path segment as slug
                slug = detail_url.rstrip('/').split('/')[-1]
                slug = slug.replace('.html', '').replace('.htm', '')
                if slug:
                    source_id = slug

        if not source_id:
            return None  # Skip items with no identifiable source_id

        # Description
        desc = ""
        for sel in [".description", ".desc", ".text", "p"]:
            el = card.select_one(sel)
            if el:
                desc = clean_text(el.get_text())[:200]
                break

        item_type = "serial" if "/serial/" in detail_url else "movie"

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


if __name__ == "__main__":
    import sys, json
    parser = FreekinoParser()
    cmd = sys.argv[1] if len(sys.argv) > 1 else None
    
    if cmd == "search":
        query = sys.argv[2] if len(sys.argv) > 2 else ""
        print(json.dumps(parser.search(query), indent=2, ensure_ascii=False))
    elif cmd == "detail":
        url = sys.argv[2] if len(sys.argv) > 2 else ""
        print(json.dumps(parser.get_detail(url), indent=2))
    else:
        print("Usage: python freekino.py search <query> | detail <url>")
