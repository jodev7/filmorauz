"""
Kinolar (kinolar.tv) Parser
uCoz based website - uses POST form submission to /load/ for search
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


class KinolarParser(BaseParser):
    """Parser for kinolar.tv (dynamic mirror via source_config)"""
    
    # Mirror is resolved from source_config.py
    BASE_URL = get_base_url("kinolar") or "https://kinolar.tv"
    
    # POST search endpoint
    SEARCH_ENDPOINT = "/load/"
    
    # Form field names for uCoz POST search
    POST_PARAMS = {
        "a": "2",
        "subaction": "search",
        "query": "",  # This will be filled with search query
    }
    
    @property
    def source_name(self) -> str:
        return "kinolar"
    
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
        """Search using uCoz POST request"""
        results = []
        
        try:
            post_data = self.POST_PARAMS.copy()
            post_data["query"] = query
            
            logger.info(f"[KINOLAR] POST search: {self.BASE_URL}{self.SEARCH_ENDPOINT}")
            response = self.session.post(
                f"{self.BASE_URL}{self.SEARCH_ENDPOINT}",
                data=post_data,
                timeout=30,
                allow_redirects=True,
                verify=False
            )
            
            if response.status_code != 200:
                logger.warning(f"[KINOLAR] Non-200 status: {response.status_code}")
                return []
            
            soup = BeautifulSoup(response.text, "lxml")
            
            # Find cards. In uCoz, cards are often just links or divs wrapping them
            # We'll use a broad selector and filter links
            all_links = soup.find_all("a", href=True)
            dict_results = []
            seen_links = {} # link -> best_title
            
            import re
            for a in all_links:
                href = a.get("href", "")
                # Match valid detail URLs that end with ID pattern e.g. /1-1-0-1234
                if "/load/" in href and re.search(r'/\d+-\d+-\d+-\d+$', href):
                    link = normalize_url(href, self.BASE_URL)
                    title = clean_text(a.get("title") or a.get_text())
                    
                    # Try to find poster
                    poster = ""
                    img = a.find("img")
                    if not img:
                        # Broaden parent search to catch uCoz specific classes like card__cover
                        parent = a.find_parent(class_=re.compile(r"card|item|post|short|ml-item|cover|thumb"))
                        if not parent:
                            parent = a.find_parent(["div", "td", "article", "li", "span"])
                        if parent:
                            img = parent.find("img")
                    if img:
                        poster_url = img.get("data-original") or img.get("data-src") or img.get("src", "")
                        if poster_url:
                            poster = normalize_url(poster_url, self.BASE_URL)
                    
                    if not title or title.lower() in ("batafsil", "skachat", "davomi", "на страницу материала"):
                        if link not in seen_links:
                            seen_links[link] = {"title": "", "poster": poster}
                        elif poster and not seen_links[link].get("poster"):
                            seen_links[link]["poster"] = poster
                        continue
                    
                    if link not in seen_links or len(title) > len(seen_links[link].get("title", "")):
                        if not poster and link in seen_links and seen_links[link].get("poster"):
                            poster = seen_links[link].get("poster")
                        seen_links[link] = {"title": title, "poster": poster}
            
            for link, data in seen_links.items():
                title = data.get("title", "")
                if not title:
                    continue
                
                poster = data.get("poster", "")
                year = extract_year(title)
                
                dict_results.append({
                    "title": title,
                    "link": link,
                    "poster": poster,
                    "year": year
                })
            
            if dict_results:
                dict_results = deduplicate_results(dict_results, key="link")
                dict_results = filter_and_rank_results(dict_results, query, title_key="title", link_key="link")
                
                for r in dict_results:
                    from helpers import detect_content_type
                    ct, _ = detect_content_type(r["link"], self.source_name)
                    if ct == "unknown" or ct == "movie":
                        if any(x in r["link"].lower() or x in r["title"].lower() for x in ["serial", "dorama", "qismlar", "/tarjima_seriallar/"]):
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
            logger.error(f"[KINOLAR] Search error: {e}")
        
        return results

    def list_categories(self):
        """Scrape categories from kinolar.tv navigation/sidebar"""
        try:
            response = self.session.get(self.BASE_URL + "/", timeout=20, verify=False)
            soup = BeautifulSoup(response.text, "lxml")
        except Exception as e:
            logger.warning(f"[KINOLAR] list_categories: fetch failed: {e}")
            return []

        categories = []
        seen_urls = set()

        def _add(href, name):
            if not href or not name or len(name) < 2: return
            if href in ("#", ""): return
            full_url = normalize_url(href, self.BASE_URL)
            if full_url in seen_urls: return
            seen_urls.add(full_url)
            slug = full_url.rstrip("/").split("/")[-1]
            categories.append({"name": name, "url": full_url, "slug": slug})

        # Search for categories in navigation or sidebar links
        for a in soup.select("a[href*='/load/']"):
            _add(a.get("href"), clean_text(a.get_text()))

        return categories

    def list_catalog(self, page: int = 1, limit: int = 20, type_filter: str = "", category_url: str = "") -> Dict[Any, Any]:
        """List catalog items from kinolar.tv"""
        results = []
        # uCoz often uses /load/0-page or similar
        url = category_url if category_url else f"{self.BASE_URL}/load/0-{page}"
        if category_url and page > 1:
             url = f"{category_url.rstrip('/')}/0-{page}"
            
        try:
            logger.info(f"[KINOLAR] Catalog request: {url}")
            response = self.session.get(url, timeout=30, verify=False)
            if response.status_code == 200:
                soup = BeautifulSoup(response.text, "lxml")
                
                all_links = soup.find_all("a", href=True)
                seen_links = {}
                
                import re
                for a in all_links:
                    href = a.get("href", "")
                    # Match valid detail URLs that end with ID pattern e.g. /1-1-0-1234
                    if "/load/" in href and re.search(r'/\d+-\d+-\d+-\d+$', href):
                        link = normalize_url(href, self.BASE_URL)
                        title = clean_text(a.get("title") or a.get_text())
                        
                        poster = ""
                        img = a.find("img")
                        if not img:
                            parent = a.find_parent(class_=re.compile(r"card|item|post|short|ml-item"))
                            if not parent:
                                parent = a.find_parent(["div", "td", "article", "li"])
                            if parent:
                                img = parent.find("img")
                        if img:
                            poster_url = img.get("data-original") or img.get("data-src") or img.get("src", "")
                            if poster_url:
                                poster = normalize_url(poster_url, self.BASE_URL)
                                
                        if not title or title.lower() in ("batafsil", "skachat", "davomi", "на страницу материала"):
                            if link not in seen_links:
                                seen_links[link] = {"title": "", "poster": poster}
                            elif poster and not seen_links[link].get("poster"):
                                seen_links[link]["poster"] = poster
                            continue
                        
                        if link not in seen_links or len(title) > len(seen_links[link].get("title", "")):
                            if not poster and link in seen_links and seen_links[link].get("poster"):
                                poster = seen_links[link].get("poster")
                            seen_links[link] = {"title": title, "poster": poster}
                
                for link, data in seen_links.items():
                    title = data.get("title", "")
                    if not title: continue
                    poster = data.get("poster", "")
                    if poster and not poster.startswith("http"):
                        poster = normalize_url(poster, self.BASE_URL)
                    
                    from helpers import detect_content_type
                    ct, _ = detect_content_type(link, self.source_name)
                    if ct == "unknown" or ct == "movie": # Force re-evaluation since kinolar sometimes misdetects
                        if "serial" in link.lower() or "serial" in title.lower() or "/tarjima_seriallar/" in link.lower():
                            ct = "serial"
                        else:
                            ct = "movie"
                            
                    results.append(SearchResult(
                        title=title,
                        year=extract_year(title),
                        poster=poster, 
                        description="",
                        source_id=self._extract_source_id(link),
                        detail_url=link,
                        source=self.source_name,
                        content_type=ct
                    ).to_dict())
            
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
            logger.error(f"[KINOLAR] Catalog error: {e}")
            return {"items": [], "page": page, "limit": limit, "total": 0, "total_pages": 0, "has_more": False}

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
                # Resolve embed iframe to actual media candidates (m3u8/mp4)
                try:
                    from media_extractor import resolve_embed_to_candidates
                    resolved = resolve_embed_to_candidates(v_url, referer=url, session=self.session)
                    if resolved:
                        video_urls.extend(resolved)
                        logger.info(f"[KINOLAR] Resolved iframe -> {len(resolved)} candidate(s)")
                except Exception as e:
                    logger.warning(f"[KINOLAR] iframe resolver error: {e}")
                # Always keep the iframe URL itself as a last-resort fallback
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
            logger.error(f"[KINOLAR] Details error: {e}")
            return MovieDetails(
                title="", description="", poster="", backdrop="", year=0, 
                genres=[], country="", duration=0, source_id=source_id, source=self.source_name
            )
