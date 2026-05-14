"""
Uzmedia (uzmedia.tv) Parser
uCoz based website - uses GET search /search/?q=query
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


class UzmediaParser(BaseParser):
    """Parser for uzmedia.tv - uses uCoz GET for search"""
    
    BASE_URL = "https://uzmedia.tv"
    
    # GET search endpoint
    SEARCH_ENDPOINT = "/search/"
    
    @property
    def source_name(self) -> str:
        return "uzmedia"
    
    @property
    def base_url(self) -> str:
        return self.BASE_URL
    
    def __init__(self):
        super().__init__()
        self.session.headers.update({
            "Referer": self.BASE_URL + "/",
            "Origin": self.BASE_URL,
        })
    
    def search(self, query: str) -> List[SearchResult]:
        """Search using GET request"""
        results = []
        
        try:
            logger.info(f"[UZMEDIA] GET search: {self.BASE_URL}{self.SEARCH_ENDPOINT}?q={query}")
            response = self.session.get(
                f"{self.BASE_URL}{self.SEARCH_ENDPOINT}",
                params={"q": query},
                timeout=30,
                allow_redirects=True,
                verify=False
            )
            
            if response.status_code != 200:
                logger.warning(f"[UZMEDIA] Non-200 status: {response.status_code}")
                return []
            
            soup = BeautifulSoup(response.text, "lxml")
            
            # Find cards. In uCoz, cards are often just links or divs wrapping them
            # We'll use a broad selector and filter links
            all_links = soup.find_all("a", href=True)
            dict_results = []
            seen_links = {} # link -> best_title
            
            for a in all_links:
                href = a.get("href", "")
                if "/load/" in href and len(href.split("/")) > 4:
                    link = normalize_url(href, self.BASE_URL)
                    title = clean_text(a.get("title") or a.get_text())
                    
                    if not title or title.lower() in ("batafsil", "skachat", "davomi", "на страницу материала"):
                        if link not in seen_links:
                            seen_links[link] = ""
                        continue
                    
                    if link not in seen_links or len(title) > len(seen_links[link]):
                        seen_links[link] = title
            
            for link, title in seen_links.items():
                if not title:
                    continue
                
                # Find poster (best effort)
                poster = ""
                # We can try to find an img in the same div as the link
                # But it's easier to just rely on get_details for posters if not found
                
                year = extract_year(title)
                
                dict_results.append({
                    "title": title,
                    "link": link,
                    "poster": "",
                    "year": year
                })
            
            if dict_results:
                dict_results = deduplicate_results(dict_results, key="link")
                dict_results = filter_and_rank_results(dict_results, query, title_key="title", link_key="link")
                
                for r in dict_results:
                    from helpers import detect_content_type
                    ct, _ = detect_content_type(r["link"], self.source_name)
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
            logger.error(f"[UZMEDIA] Search error: {e}")
        
        return results

    def _extract_source_id(self, url: str) -> str:
        # uCoz URLs usually end with /id or id.html or have ids like /29-1-0-9031
        import re
        match = re.search(r'/([^/]+)$', url)
        if match:
            return match.group(1).replace(".html", "")
        return ""

    def get_details(self, url: str, source_id: str, is_serial: bool = False, episode_id: str = "") -> MovieDetails:
        try:
            response = self.session.get(url, timeout=30, verify=False)
            soup = BeautifulSoup(response.text, "lxml")
            
            title = clean_text((soup.select_one("h1") or soup.select_one("title") or soup).get_text())
            poster = ""
            og_img = soup.select_one("meta[property='og:image']")
            if og_img:
                poster = normalize_url(og_img.get("content", ""), self.BASE_URL)
            
            desc = ""
            desc_elem = soup.select_one(".description, .text, [itemprop='description']")
            if desc_elem:
                desc = clean_text(desc_elem.get_text())
                
            iframe = soup.select_one("iframe[src*='//']")
            video_urls = []
            if iframe:
                v_url = iframe.get("src", "")
                if v_url.startswith("//"):
                    v_url = "https:" + v_url
                video_urls.append({"url": v_url, "type": "iframe_embed", "quality": "unknown"})
                
            return MovieDetails(
                title=title,
                description=desc,
                poster=poster,
                backdrop="",
                year=extract_year(title) or 0,
                genres=[],
                country="",
                duration=0,
                video_page_url=url,
                video_urls=video_urls,
                source_id=source_id,
                source=self.source_name
            )
        except Exception as e:
            logger.error(f"[UZMEDIA] Details error: {e}")
            return MovieDetails(
                title="", description="", poster="", backdrop="", year=0, 
                genres=[], country="", duration=0, source_id=source_id, source=self.source_name
            )
