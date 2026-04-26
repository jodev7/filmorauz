"""
Freekino Parser - With strict stage-by-stage diagnosis
"""
import requests
import re
import logging
import os
import html
import base64
from typing import List, Dict, Any, Optional
from urllib.parse import urljoin, quote, unquote
from bs4 import BeautifulSoup

from media_extractor import (
    is_valid_media_url,
    classify_media_url,
    validate_media_url_strict,
    extract_from_freekino,
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
    HASH2_MARKERS = [
        "6u3kEQLAkT",
        "4e0OIbkv8D",
        "EXOThnsl1y",
        "wHiWWKB8U9",
        "rMTS46lVY0",
    ]
    
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
        """Get movie detail — returns video_urls list (for server) + quality_urls dict."""
        result = {
            "title": "", "video_url": "", "video_type": "",
            "poster": "", "description": "", "year": 2024,
            "genres": [], "type": "movie",
            # Passed to downloader as Referer header — CDN requires it
            "video_page_url": url,
            # server.py reads this list via _extract_best_video_url()
            "video_urls": [],
            # structured quality map: {360: url, 480: url, 720: url, 1080: url}
            "quality_urls": {},
        }

        try:
            response = self.session.get(url, timeout=30)
            response.raise_for_status()
            logger.info(f"[FREEKINO] page fetched - url={response.url}, bytes={len(response.text)}")
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

            # Video — extract all quality variants
            all_entries, quality_urls = self._extract_video(soup, response.url)

            # Final sanity pass: drop any entry whose URL still carries
            # multi-URL artifacts. video_url must be a single, real URL.
            clean_entries = []
            for e in all_entries or []:
                u = (e or {}).get("url", "")
                if not u or any(ch in u for ch in ('[', ']', ',')):
                    logger.warning(f"[FREEKINO] dropping malformed entry: quality={e.get('quality','?')}, url={u[:120]}")
                    continue
                clean_entries.append(e)
            all_entries = clean_entries
            quality_urls = {res: url for res, url in (quality_urls or {}).items()
                            if url and not any(ch in url for ch in ('[', ']', ','))}

            if all_entries:
                # Build video_urls list expected by server._extract_best_video_url()
                result["video_urls"] = all_entries
                result["quality_urls"] = quality_urls

                # Best URL = highest quality entry (list is ordered best-first)
                best = all_entries[0]
                result["video_url"] = best["url"]
                result["video_type"] = best["type"]

                qualities_summary = [e.get("quality", "?") for e in all_entries]
                logger.info(f"[FREEKINO] qualities extracted: {qualities_summary}")
                logger.info(
                    f"[FREEKINO] chosen video_url: quality={best.get('quality','?')}, "
                    f"type={best.get('type','?')}, url={best['url'][:120]}"
                )
            else:
                logger.warning(f"[FREEKINO] No valid video URL found for {url}")

        except Exception as e:
            logger.error(f"[FREEKINO] Detail error: {e}")

        return result
    
    # ------------------------------------------------------------------
    # Quality resolution ordering — higher index = lower priority
    # ------------------------------------------------------------------
    _QUALITY_ORDER = [1080, 720, 480, 360, 240]

    @staticmethod
    def _label_to_int(label: str) -> int:
        """Extract numeric resolution from a quality label like '720p' or '720'."""
        m = re.search(r'(\d{3,4})', label)
        return int(m.group(1)) if m else 0

    def _extract_video(self, soup, page_url: str = ""):
        """
        Extract all available video quality variants from the page.

        Returns:
            (all_entries, quality_urls)
            - all_entries: list of {"url", "type", "quality"} dicts, best quality first.
              This is what server._extract_best_video_url() reads via details["video_urls"].
            - quality_urls: dict mapping int resolution to URL, e.g. {720: "https://...", 480: "..."}
        """

        # === PRIMARY: Playerjs({file:"[quality]url,[quality]url"}) ===
        for script in soup.find_all("script"):
            content = script.string or ""
            if "Playerjs" not in content and "jwplayer" not in content.lower():
                continue
            entries, quality_urls = self._parse_playerjs_file(content, page_url or self.BASE_URL)
            if entries:
                logger.info(f"[FREEKINO] extracted url - pattern=player_config, count={len(entries)}, url={entries[0]['url'][:120]}")
                return entries, quality_urls

        # === FALLBACK: FreekinoPlayer custom player with #2 hash encoding ===
        entries, quality_urls = self._extract_freekino_player(soup, page_url or self.BASE_URL)
        if entries:
            logger.info(f"[FREEKINO] extracted url - pattern=freekino_player, count={len(entries)}, url={entries[0]['url'][:120]}")
            return entries, quality_urls

        # === FALLBACK: <video> tag with <source> children ===
        entries, quality_urls = self._extract_video_tags(soup, page_url or self.BASE_URL)
        if entries:
            logger.info(f"[FREEKINO] extracted url - selector=video/source, count={len(entries)}, url={entries[0]['url'][:120]}")
            return entries, quality_urls

        # === FALLBACK: any iframe — fetch and recurse ===
        for iframe in soup.find_all("iframe"):
            iframe_src = iframe.get("src", "")
            if not iframe_src or iframe_src.startswith("blob"):
                continue
            iframe_src = normalize_url(page_url or self.BASE_URL, iframe_src)
            logger.info(f"[FREEKINO] iframe found - src={iframe_src}")
            entries, quality_urls = self._extract_from_iframe(iframe_src)
            if entries:
                logger.info(f"[FREEKINO] extracted url - selector=iframe, count={len(entries)}, url={entries[0]['url'][:120]}")
                return entries, quality_urls

        # === FALLBACK: generic regex for absolute mp4/m3u8 URLs in scripts ===
        entries, quality_urls = self._extract_script_fallback(soup, page_url or self.BASE_URL)
        if entries:
            logger.info(f"[FREEKINO] extracted url - pattern=script_fallback, count={len(entries)}, url={entries[0]['url'][:120]}")
            return entries, quality_urls

        logger.warning("[FREEKINO] script candidates found - 0")
        return [], {}

    def _extract_freekino_player(self, soup, base_url: str = ""):
        """
        Extract video URLs from FreekinoPlayer custom player config.
        This handles the #2 hash format used by freekino.net.
        """
        html_content = str(soup)

        player_config_patterns = [
            r'file\s*:\s*"([^"]+)"',
            r"file\s*:\s*'([^']+)'",
            r'playerConfig\s*=\s*\{[^}]*file\s*:\s*"([^"]+)"',
            r'new\s+FreekinoPlayer\s*\(\s*\{[^}]*file\s*:\s*"([^"]+)"',
        ]

        for pattern in player_config_patterns:
            matches = re.findall(pattern, html_content, re.S | re.I)
            for match in matches:
                decoded = self._decode_video_url(match, base_url or self.BASE_URL, make_absolute=False)
                entries, quality_urls = self._parse_quality_bundle(decoded, base_url or self.BASE_URL)
                if entries:
                    logger.info(f"[PARSER] found stream_url={entries[0]['url']}")
                    return entries, quality_urls

        return [], {}

    def _decode_freekino_hash(self, encoded: str) -> str:
        """
        Decode the Freekino #2 format used by freekino-player.min.js.
        """
        if not encoded:
            return ""

        try:
            cleaned = encoded
            for marker in reversed(self.HASH2_MARKERS):
                encoded_marker = "//" + base64.b64encode(marker.encode("utf-8")).decode("ascii")
                cleaned = cleaned.replace(encoded_marker, "")
            cleaned += "=" * ((4 - len(cleaned) % 4) % 4)
            decoded = base64.b64decode(cleaned).decode("utf-8", errors="ignore")
            decoded = re.sub(r'[\x00-\x1f]', '', decoded).strip()
            if decoded:
                return decoded
        except Exception as e:
            logger.info(f"[FREEKINO] hash2 decode failed: {e}")

        return ""

    def _decode_video_url(self, url: str, base_url: str = "", make_absolute: bool = True) -> str:
        if not url:
            return ""
        url = html.unescape(url).strip().strip('"\'')
        for _ in range(2):
            try:
                url = unquote(url)
            except Exception:
                break
        url = url.replace("\\/", "/").replace("\\u0026", "&")
        try:
            url = url.encode("utf-8").decode("unicode_escape")
        except Exception:
            pass
        if url.startswith("#2"):
            try:
                decoded = self._decode_freekino_hash(url[2:])
                if decoded:
                    url = decoded
                    logger.info("[FREEKINO] extracted url - pattern=freekino_hash2_decode")
                else:
                    logger.warning("[FREEKINO] hash2 payload decoded to empty result")
            except Exception as e:
                logger.info(f"[FREEKINO] playerjs hash decode failed: {e}")
        if not make_absolute:
            return url
        return normalize_url(base_url or self.BASE_URL, url)

    def _entry_from_url(self, url: str, base_url: str, quality: str = "auto"):
        url = self._decode_video_url(url, base_url)
        # Stricter character class: excludes ',', '[', ']' so the regex cannot
        # swallow across multiple Playerjs entries like "[480p]a.mp4,[720p]b.mp4".
        media_match = re.search(
            r'(https?://[A-Za-z0-9._~:/?#@!$&()*+;=%-]+?\.(?:mp4|m3u8|mpd)(?:\?[A-Za-z0-9._~:/?#@!$&()*+;=%-]*)?)',
            url,
            re.I,
        )
        if media_match:
            url = media_match.group(1)
        if not url or url.startswith("blob"):
            return None
        # Defense in depth: reject anything that still carries multi-URL artifacts.
        if any(ch in url for ch in ('[', ']', ',')):
            logger.warning(f"[FREEKINO] rejected malformed url (contains [ ] or , ): {url[:120]}")
            return None
        if validate_media_url_strict(url):
            return None
        return {"url": url, "type": classify_media_url(url), "quality": quality or "auto"}

    def _parse_playerjs_file(self, script_content: str, base_url: str = ""):
        """
        Parse Playerjs({file:"[480p]url,[720p]url"}) — the primary freekino format.

        JS escaping in the raw string:
          \\/ → /   (forward-slash escape)
          \\u0026 → &  (ampersand in query strings)

        Returns (all_entries, quality_urls):
          all_entries — list of {"url", "type", "quality"} ordered best-first
          quality_urls — {720: url, 480: url, ...}
        """
        # Match common player configs: Playerjs, jwplayer.setup, file/source/sources.
        m = re.search(
            r'\b(?:file|source|src|url)\s*:\s*(["\'])((?:.(?!\1))*.?)\1',
            script_content,
            re.S | re.I,
        )
        if not m:
            return [], {}

        raw = self._decode_video_url(m.group(2), base_url or self.BASE_URL, make_absolute=False)
        return self._parse_quality_bundle(raw, base_url or self.BASE_URL)

    def _parse_quality_bundle(self, raw: str, base_url: str = ""):
        if not raw:
            return [], {}

        # Parse quality-labelled entries: [label]https://...
        labeled = re.findall(r'\[([^\]]+)\]((?:https?:)?//[^,\[]+|/[^,\[]+)', raw)

        if not labeled:
            # No quality labels — single bare URL
            url = raw.strip().rstrip(",")
            entry = self._entry_from_url(url, base_url or self.BASE_URL, "auto")
            if entry:
                return [entry], {}
            return [], {}

        # Build per-quality map, dedup by resolution
        quality_map: Dict[int, str] = {}
        for label, url in labeled:
            entry = self._entry_from_url(url.strip().rstrip(", "), base_url or self.BASE_URL, label)
            if not entry:
                continue
            res = self._label_to_int(label)
            # Keep first occurrence of each resolution (avoid overwrite)
            if res not in quality_map:
                quality_map[res] = entry["url"]

        if not quality_map:
            # Validation rejected everything — use raw first entry as fallback
            entry = self._entry_from_url(labeled[0][1].strip().rstrip(", "), base_url or self.BASE_URL, labeled[0][0])
            if entry:
                return [entry], {}
            return [], {}

        # Sort by resolution descending
        sorted_res = sorted(quality_map.keys(), reverse=True)
        all_entries = []
        for res in sorted_res:
            url = quality_map[res]
            vtype = classify_media_url(url)
            quality_label = f"{res}p" if res else "auto"
            all_entries.append({"url": url, "type": vtype, "quality": quality_label})

        quality_urls = {res: quality_map[res] for res in sorted_res}
        return all_entries, quality_urls

    def _extract_video_tags(self, soup, base_url: str = ""):
        """Extract URLs from <video src> and <video><source src> tags."""
        entries = []
        quality_map: Dict[int, str] = {}

        for video in soup.find_all("video"):
            # Inline src on the <video> element
            src = video.get("src", "")
            if src and not src.startswith("blob"):
                src = normalize_url(base_url or self.BASE_URL, src)
                if not validate_media_url_strict(src):
                    res = 0
                    quality_map.setdefault(res, src)
                    entries.append({
                        "url": src,
                        "type": classify_media_url(src),
                        "quality": "auto",
                    })

            # <source> children (may carry label/size attributes)
            for source in video.find_all("source"):
                src = source.get("src", "")
                if not src or src.startswith("blob"):
                    continue
                src = normalize_url(base_url or self.BASE_URL, src)
                if validate_media_url_strict(src):
                    continue
                label = source.get("label", "") or source.get("size", "") or ""
                res = self._label_to_int(label) if label else 0
                quality_map.setdefault(res, src)
                entries.append({
                    "url": src,
                    "type": classify_media_url(src),
                    "quality": f"{res}p" if res else label or "auto",
                })

        # Re-sort by resolution descending
        entries.sort(key=lambda e: self._label_to_int(e["quality"]), reverse=True)
        quality_urls = {res: url for res, url in quality_map.items() if res}
        return entries, quality_urls

    def _extract_script_fallback(self, soup, base_url: str = ""):
        """Regex fallback: find absolute https:// mp4/m3u8/mpd URLs in script tags."""
        seen: set = set()
        entries = []
        candidates_found = 0

        def add_url(raw_url: str, quality: str = "auto"):
            nonlocal candidates_found
            candidates_found += 1
            entry = self._entry_from_url(raw_url, base_url or self.BASE_URL, quality)
            if not entry or entry["url"] in seen:
                return
            seen.add(entry["url"])
            entries.append(entry)
            logger.info(f"[FREEKINO] extracted url - pattern=script, url={entry['url'][:120]}")

        for script in soup.find_all("script"):
            content = script.string or script.get_text() or ""
            if not content:
                continue

            patterns = [
                r'\b(?:file|source|src|url|video_url|videoUrl)\s*[:=]\s*(["\'])(.+?)\1',
                r'\bsources\s*:\s*\[([^\]]+)\]',
                r'(https?:\\?/\\?/[^"\'<>\s]+?(?:\.m3u8|\.mp4|\.mpd)(?:\?[^"\'<>\s]*)?)',
                r'((?:/[^"\'<>\s]+?)(?:\.m3u8|\.mp4|\.mpd)(?:\?[^"\'<>\s]*)?)',
            ]

            for pattern in patterns:
                for match in re.findall(pattern, content, re.I | re.S):
                    if isinstance(match, tuple):
                        raw = match[-1]
                    else:
                        raw = match
                    if not raw:
                        continue
                    # Playerjs quality-labelled bundle: "[480p]url1,[720p]url2".
                    # Split into per-quality URLs instead of letting the broad
                    # regex capture the whole string as a single (broken) URL.
                    if re.search(r'\[\s*(?:auto|\d+p?)\s*\]', raw, re.I):
                        labeled = re.findall(r'\[([^\]]+)\]((?:https?:)?//[^,\[]+|/[^,\[]+)', raw)
                        if labeled:
                            for label, lab_url in labeled:
                                add_url(lab_url.strip().rstrip(", "), label.strip())
                            continue
                    if "sources" in pattern:
                        for nested in re.findall(r'\b(?:file|src|url)\s*:\s*(["\'])(.+?)\1', raw, re.I | re.S):
                            add_url(nested[-1])
                        for nested_url in re.findall(r'(https?:\\?/\\?/[^"\'<>\s]+?(?:\.m3u8|\.mp4|\.mpd)(?:\?[^"\'<>\s]*)?)', raw, re.I):
                            add_url(nested_url)
                    else:
                        add_url(raw)

        try:
            extractor_candidates = extract_from_freekino(soup, base_url or self.BASE_URL)
            for candidate in extractor_candidates:
                add_url(candidate.url, candidate.quality)
        except Exception as e:
            logger.info(f"[FREEKINO] media_extractor fallback failed: {e}")

        logger.info(f"[FREEKINO] script candidates found - {candidates_found}")

        entries.sort(key=lambda e: self._label_to_int(e["quality"]), reverse=True)
        quality_urls = {}
        for e in entries:
            res = self._label_to_int(e["quality"])
            if res:
                quality_urls.setdefault(res, e["url"])
        return entries, quality_urls

    def _extract_from_iframe(self, iframe_url: str):
        """Fetch an iframe page and extract video URLs from it."""
        try:
            headers = {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
            }
            response = requests.get(iframe_url, headers=headers, timeout=30, allow_redirects=True)
            content_type = response.headers.get("Content-Type", "")

            if "text/html" not in content_type:
                # Direct media response
                url = response.url
                if not validate_media_url_strict(url):
                    vtype = classify_media_url(url)
                    return [{"url": url, "type": vtype, "quality": "auto"}], {}
                return [], {}

            iframe_soup = BeautifulSoup(response.text, "lxml")

            entries, quality_urls = self._extract_freekino_player(iframe_soup, response.url)
            if entries:
                return entries, quality_urls

            for nested_iframe in iframe_soup.find_all("iframe"):
                nested_src = normalize_url(response.url, nested_iframe.get("src", ""))
                if not nested_src or nested_src == iframe_url:
                    continue
                entries, quality_urls = self._extract_from_iframe(nested_src)
                if entries:
                    return entries, quality_urls

            # Try Playerjs first (same format as main page)
            for script in iframe_soup.find_all("script"):
                content = script.string or script.get_text() or ""
                if "Playerjs" in content or "FreekinoPlayer" in content:
                    entries, quality_urls = self._parse_playerjs_file(content, response.url)
                    if entries:
                        logger.info(f"[PARSER] found stream_url={entries[0]['url']}")
                        return entries, quality_urls

            # <video> tags
            entries, quality_urls = self._extract_video_tags(iframe_soup, response.url)
            if entries:
                return entries, quality_urls

            # Script regex fallback
            entries, quality_urls = self._extract_script_fallback(iframe_soup, response.url)
            if entries:
                logger.info(f"[PARSER] found stream_url={entries[0]['url']}")
            return entries, quality_urls

        except Exception as e:
            if DEBUG:
                logger.info(f"[FREEKINO] Iframe fetch error ({iframe_url[:60]}): {e}")
        return [], {}
    


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

    # Default catalog page — all translated movies, has reliable pagination
    CATALOG_BASE = "https://freekino.net/movie/genre/15-tarjima-kinolar"

    def list_catalog(self, page=1, limit=20, type_filter="", category_url=""):
        """
        List movies from freekino catalog with pagination.

        Freekino uses Bootstrap pagination with ?page=N query strings:
          page 1 → {base}
          page N → {base}?page=N

        Args:
            page: Page number (1-based)
            limit: Max items per page
            type_filter: Optional filter
            category_url: Optional category/genre URL to browse instead of default

        Returns:
            dict with items, page, limit, total, total_pages, has_more, next_url
        """
        # Default catalog is film-only (/movie/genre/...). When the caller asks
        # for serials without picking a category, auto-select a serial-named
        # category; otherwise we'd scrape a film listing and filter to zero.
        if type_filter == "serial" and not category_url:
            try:
                cats = self.list_categories() or []
                for c in cats:
                    name = (c.get("name") or "").lower()
                    curl = (c.get("url") or "")
                    if "serial" in name or "/serial" in curl.lower():
                        category_url = curl
                        logger.info(f"[FREEKINO] list_catalog: auto-selected serial category: name={c.get('name')!r}, url={category_url}")
                        break
                if not category_url:
                    logger.warning("[FREEKINO] list_catalog: no serial category found via list_categories()")
            except Exception as e:
                logger.warning(f"[FREEKINO] list_catalog: serial category auto-detect failed: {e}")

        base = (category_url.rstrip("/") if category_url else self.CATALOG_BASE)

        # Build the target URL for this page
        if page <= 1:
            candidate_urls = [base]
        else:
            # Freekino uses ?page=N (query param, NOT /page/N/ path segment)
            sep = "&" if "?" in base else "?"
            candidate_urls = [f"{base}{sep}page={page}"]

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

        # When the scraped page is a serial listing, force page-level type on all
        # items — freekino serial card URLs do contain /serial/ so this is mostly
        # belt-and-suspenders, but it keeps behavior consistent with other providers.
        if type_filter == "serial" or "serial" in (category_url or "").lower():
            for it in items:
                it["type"] = "serial"
            logger.info(f"[FREEKINO] list_catalog: forced type=serial on {len(items)} items (serial listing page)")

        if type_filter:
            items = [i for i in items if i.get("type") == type_filter]
            logger.info(f"[FREEKINO] list_catalog: {len(items)} items after type_filter={type_filter!r}")


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
