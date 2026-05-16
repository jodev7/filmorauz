"""
Kinochilar (kinochilar.com) Parser
DataLife Engine (DLE) based website - uses POST form submission for search
"""
import logging
import os
from typing import List, Dict, Any, Optional
from bs4 import BeautifulSoup

from base_parser import BaseParser, SearchResult, MovieDetails
from helpers import (
    normalize_url, 
    clean_text, 
    extract_year, 
    deduplicate_results,
    filter_and_rank_results
)

logger = logging.getLogger(__name__)
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


class KinochilarParser(BaseParser):
    """Parser for kinochilar.com - uses POST for search"""
    
    BASE_URL = "https://kinochilar.com"
    
    # POST search endpoint
    SEARCH_ENDPOINT = "/index.php"
    
    # Form field names for POST search
    POST_PARAMS = {
        "do": "search",
        "subaction": "search",
        "story": "",  # This will be filled with query
    }
    
    # Card selectors
    CARD_SELECTORS = [
        ".movie-card-premium",
        ".poster-collection",
        ".search-card-premium",
        ".shortstory",
        ".film-item",
        ".movie-item",
        "article",
        ".kino-item",
    ]
    
    @property
    def source_name(self) -> str:
        return "kinochilar"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL
    
    def __init__(self):
        super().__init__()
        self._default_headers.update({
            "Referer": self.BASE_URL + "/",
            "Origin": self.BASE_URL,
        })
    
    def search(self, query: str) -> List[SearchResult]:
        """Search using POST request"""
        results = []
        
        try:
            post_data = self.POST_PARAMS.copy()
            post_data["story"] = query
            
            logger.info(f"[KINOCHILAR] POST search: {self.BASE_URL}{self.SEARCH_ENDPOINT}")
            response = self.session.post(
                f"{self.BASE_URL}{self.SEARCH_ENDPOINT}",
                data=post_data,
                timeout=30,
                allow_redirects=True,
                verify=False # Bypass strict SSL if any
            )
            
            if response.status_code != 200:
                logger.warning(f"[KINOCHILAR] Non-200 status: {response.status_code}")
                return []
            
            soup = BeautifulSoup(response.text, "lxml")
            
            # Find cards
            cards = []
            for sel in self.CARD_SELECTORS:
                found = soup.select(sel)
                if found:
                    cards = found
                    break
            
            # Fallback for links
            if not cards:
                all_links = soup.find_all("a", href=True)
                for a in all_links:
                    href = a.get("href", "")
                    if ".html" in href and ("/" in href.replace(self.BASE_URL, "")):
                        parent = a.find_parent("div")
                        if parent and parent not in cards:
                            cards.append(parent)
            
            # Extract fields from each card
            dict_results = []
            for card in cards:
                result = self._extract_card(card)
                if result and result.get("title") and result.get("link"):
                    dict_results.append(result)
            
            # Deduplicate and filter
            if dict_results:
                dict_results = deduplicate_results(dict_results, key="link")
                dict_results = filter_and_rank_results(dict_results, query, title_key="title", link_key="link")
                
                for r in dict_results:
                    from helpers import detect_content_type
                    ct, _ = detect_content_type(r["link"], self.source_name)
                    if ct == "unknown" or ct == "movie":
                        if any(x in r["link"].lower() or x in r["title"].lower() for x in ["serial", "dorama", "qismlar", "/tarjima-seriallar/"]):
                            ct = "serial"
                        else:
                            ct = "movie"
                    results.append(SearchResult(
                        title=r["title"],
                        year=r.get("year"),
                        poster=r.get("poster"),
                        description="",
                        source_id=self._extract_source_id(r["link"]),
                        detail_url=r["link"],
                        source=self.source_name,
                        content_type=ct
                    ))
            
        except Exception as e:
            logger.error(f"[KINOCHILAR] Search error: {e}")
        
        return results

    def list_catalog(self, page: int = 1, limit: int = 20, type_filter: str = "", category_url: str = "") -> Dict[Any, Any]:
        """List catalog items from kinochilar.com"""
        results = []
        url = category_url if category_url else f"{self.BASE_URL}/page/{page}/"
        if category_url and page > 1:
            url = f"{category_url.rstrip('/')}/page/{page}/"
            
        try:
            logger.info(f"[KINOCHILAR] Catalog request: {url}")
            response = self.session.get(url, timeout=30, verify=False)
            if response.status_code == 200:
                soup = BeautifulSoup(response.text, "lxml")
                cards = []
                for selector in self.CARD_SELECTORS:
                    found = soup.select(selector)
                    if found:
                        cards = found
                        break
                
                if not cards:
                    # Fallback card detection
                    for a in soup.find_all("a", href=True):
                        href = a.get("href", "")
                        if "/film/" in href or "/multfilm/" in href:
                            parent = a.find_parent("div")
                            if parent and parent not in cards:
                                cards.append(parent)
                
                for card in cards:
                    res = self._extract_card(card)
                    if res and res.get("title") and res.get("link"):
                        from helpers import detect_content_type
                        ct, _ = detect_content_type(res["link"], self.source_name)
                        results.append(SearchResult(
                            title=res["title"],
                            year=res.get("year"),
                            poster=res.get("poster"),
                            description="",
                            source_id=self._extract_source_id(res["link"]),
                            detail_url=res["link"],
                            source=self.source_name,
                            content_type=ct
                        ).to_dict())
            
            from helpers import deduplicate_results
            results = deduplicate_results(results, key="detail_url")
            
            return {
                "items": results[:limit],
                "page": page,
                "limit": limit,
                "total": 0,
                "total_pages": page + 1 if len(results) >= 10 else page,
                "has_more": len(results) >= 10
            }
        except Exception as e:
            logger.error(f"[KINOCHILAR] Catalog error: {e}")
            return {"items": [], "page": page, "limit": limit, "total": 0, "total_pages": 0, "has_more": False}
    
    def _extract_card(self, card) -> Optional[Dict[str, Any]]:
        title = ""
        link = ""
        poster = ""
        year = None
        
        a_tag = card.select_one("a[href]")
        if a_tag:
            link = normalize_url(a_tag.get("href", ""), self.BASE_URL)
            
        title_elem = card.select_one(".movie-card-title, .title, .kino-title, .name")
        if title_elem:
            title = clean_text(title_elem.get_text())
        elif a_tag:
            title = clean_text(a_tag.get("title") or a_tag.get_text())
            
        img = card.select_one("img")
        if img:
            poster_url = img.get("data-src") or img.get("data-lazy-src") or img.get("src", "")
            poster = normalize_url(poster_url, self.BASE_URL)
            
        if not title or title.lower() in ("fhd tomosha qilish", "tomosha qilish"):
            # Fallback to finding title from image alt
            if img and img.get("alt"):
                title = clean_text(img.get("alt"))
                
        if not title:
            return None
            
        from helpers import detect_content_type
        ct, _ = detect_content_type(link, self.source_name)
        if ct == "unknown":
            if any(x in link.lower() or x in title.lower() for x in ["serial", "dorama", "qismlar"]):
                ct = "serial"
            else:
                ct = "movie"
            
        year_elem = card.select_one(".year, .date")
        if year_elem:
            year = extract_year(year_elem.get_text())
            
        if not year:
            year = extract_year(title)
            
        return {
            "title": title,
            "link": link,
            "poster": poster,
            "year": year,
            "type": ct
        }

    def _extract_source_id(self, url: str) -> str:
        import re
        match = re.search(r'/(\d+)-[\w-]+\.html', url)
        if match:
            return match.group(1)
        return ""

    def get_details(self, url: str, source_id: str, is_serial: bool = False, episode_id: str = "") -> MovieDetails:
        try:
            response = self.session.get(url, timeout=30, verify=False)
            soup = BeautifulSoup(response.text, "lxml")
            
            # Title
            title_elem = soup.select_one("h1, .fs-title")
            title = clean_text(title_elem.get_text()) if title_elem else ""
            
            # Poster
            poster = ""
            og_img = soup.select_one("meta[property='og:image']")
            if og_img:
                poster = normalize_url(og_img.get("content", ""), self.BASE_URL)
            
            # Backdrop
            backdrop = ""
            
            # Description
            desc = ""
            desc_elem = soup.select_one(".full-text, .fs-description, [itemprop='description']")
            if desc_elem:
                desc = clean_text(desc_elem.get_text())
            
            # Metadata
            year = 0
            genres = []
            country = ""
            duration = 0
            
            # DLE often has info in a list or div
            for info_item in soup.select(".fs-meta-item, .finfo-item"):
                text = info_item.get_text().lower()
                if "yil" in text or "year" in text:
                    extracted_year = extract_year(info_item.get_text())
                    if extracted_year: year = extracted_year
                elif "janr" in text or "genre" in text:
                    genres = [clean_text(g.get_text()) for g in info_item.select("a")]
                elif "davlat" in text or "mamlakat" in text or "country" in text:
                    country = clean_text(info_item.get_text().split(":")[-1]) if ":" in info_item.get_text() else ""
            
            if year == 0:
                year = extract_year(title) or 0
                
            iframe = soup.select_one("iframe[src*='//']")
            video_urls = []
            if iframe:
                v_url = iframe.get("src", "")
                if v_url.startswith("//"):
                    v_url = "https:" + v_url
                try:
                    from media_extractor import resolve_embed_to_candidates
                    resolved = resolve_embed_to_candidates(v_url, referer=url, session=self.session)
                    if resolved:
                        video_urls.extend(resolved)
                        logger.info(f"[KINOCHILAR] Resolved iframe -> {len(resolved)} candidate(s)")
                except Exception as e:
                    logger.warning(f"[KINOCHILAR] iframe resolver error: {e}")
                video_urls.append({"url": v_url, "type": "iframe_embed", "quality": "unknown"})
                
            return MovieDetails(
                title=title,
                description=desc,
                poster=poster,
                backdrop=backdrop,
                year=year,
                genres=genres,
                country=country,
                duration=duration,
                video_page_url=url,
                video_urls=video_urls,
                source_id=source_id,
                source=self.source_name
            )
        except Exception as e:
            logger.error(f"[KINOCHILAR] Details error: {e}")
            return MovieDetails(
                title="", description="", poster="", backdrop="", year=0, 
                genres=[], country="", duration=0, source_id=source_id, source=self.source_name
            )
