"""
Uzmedia (uzmedia.tv) Parser
uCoz based website - uses GET search /search/?q=query
"""
import logging
import os
import re
from typing import List, Dict, Any, Optional
from urllib.parse import unquote
from bs4 import BeautifulSoup

from base_parser import BaseParser, SearchResult, MovieDetails
from source_config import get_base_url
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
    """Parser for uzmedia.tv (dynamic mirror via source_config)"""
    
    # Mirror is resolved from source_config.py
    BASE_URL = get_base_url("uzmedia") or "https://uzmedia.tv"
    
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
        self._default_headers.update({
            "Referer": self.BASE_URL + "/",
            "Origin": self.BASE_URL,
        })
        # uzmedia.tv serves a self-signed TLS certificate, so verification must
        # be disabled. Setting it on the shared session covers both the movie
        # parser's explicit verify=False calls AND UzmediaSerialParser, which
        # reuses this session and previously failed with CERTIFICATE_VERIFY_FAILED.
        self.session.verify = False
        try:
            import urllib3
            urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)
        except Exception:
            pass
    
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
            
            import re
            for a in all_links:
                href = a.get("href", "")
                # Match valid detail URLs that end with ID pattern e.g. /1-1-0-1234
                if "/load/" in href and re.search(r'/\d+-\d+-\d+-\d+$', href):
                    link = normalize_url(href, self.BASE_URL)
                    title = clean_text(a.get("title") or a.get_text())
                    
                    if not title or title.lower() in ("batafsil", "skachat", "davomi", "на страницу материала"):
                        if link not in seen_links:
                            seen_links[link] = {"title": "", "poster": ""}
                        continue
                        
                    # Try to find poster
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
                    ct, _ = detect_content_type(link, self.source_name)
                    if ct == "unknown" or ct == "movie":
                        # Heuristic for serials in uCoz sites
                        if any(x in link.lower() or x in title.lower() for x in ["serial", "dorama", "qismlar", "/serialy/"]):
                            ct = "serial"
                        else:
                            ct = "movie"

                    results.append(SearchResult(                        title=r["title"],
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

    def list_categories(self):
        """Scrape categories from uzmedia.tv sidebar/nav"""
        try:
            response = self.session.get(self.BASE_URL + "/", timeout=20, verify=False)
            soup = BeautifulSoup(response.text, "lxml")
        except Exception as e:
            logger.warning(f"[UZMEDIA] list_categories: fetch failed: {e}")
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

        # Sidebar links usually contain categories in uCoz
        for a in soup.select("a[href*='/load/']"):
            _add(a.get("href"), clean_text(a.get_text()))

        return categories

    def list_catalog(self, page: int = 1, limit: int = 20, type_filter: str = "", category_url: str = "") -> Dict[Any, Any]:
        """List catalog items from uzmedia.tv"""
        results = []
        # uCoz often uses /load/0-page or similar
        url = category_url if category_url else f"{self.BASE_URL}/load/0-{page}"
        if category_url and page > 1:
             url = f"{category_url.rstrip('/')}/0-{page}"

        try:
            logger.info(f"[UZMEDIA] Catalog request: {url}")
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

                        # Try to find poster
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
                                # ENSURE ABSOLUTE URL
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
                    if ct == "unknown":
                        if "serial" in link.lower() or "serial" in title.lower():
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
            logger.error(f"[UZMEDIA] Catalog error: {e}")
            return {"items": [], "page": page, "limit": limit, "total": 0, "total_pages": 0, "has_more": False}

    def _extract_source_id(self, url: str) -> str:
        # uCoz URLs usually end with /id or id.html or have ids like /29-1-0-9031
        import re
        match = re.search(r'/([^/]+)$', url)
        if match:
            return match.group(1).replace(".html", "")
        return ""

    # Episode source_id produced by canonical_episode_id(): "<id>:sXXeYY".
    _EPISODE_SID_RE = re.compile(r":s(\d+)e(\d+)\s*$", re.IGNORECASE)
    _EP_NUM_RE = re.compile(r"(\d+)\s*-\s*qism", re.IGNORECASE)

    def extract_serial_episode_urls(self, html: str) -> Dict[int, str]:
        """Map episode_number -> direct .mp4 URL from a uzmedia serial page.

        uzmedia serial pages embed one player iframe per episode as
        ``embed.html?file=http://s1.uzmedia.tv/seriallar/<name> N-qism hd ...mp4``
        (the same .mp4 URLs also appear inline). The episode number lives in the
        filename, so we read it from there rather than from absent "N-qism"
        anchor links."""
        out: Dict[int, str] = {}
        candidates = re.findall(r"embed\.html\?file=([^\"'<>\s]+)", html)
        candidates += re.findall(r"https?://[^\s\"'<>]+?\.mp4", html)
        for raw in candidates:
            raw = raw.strip()
            # Keep the URL percent-encoded for the downloader; only unquote a
            # copy to read the episode number from the "N-qism" filename.
            if not unquote(raw).lower().endswith(".mp4"):
                continue
            m = self._EP_NUM_RE.search(unquote(raw))
            if not m:
                continue
            out.setdefault(int(m.group(1)), raw)
        return out

    def get_details(self, url: str, source_id: str, is_serial: bool = False, episode_id: str = "") -> MovieDetails:
        try:
            response = self.session.get(url, timeout=30, verify=False)
            soup = BeautifulSoup(response.text, "lxml")

            # Serial episode: the page embeds one direct .mp4 per episode and the
            # first iframe is always episode 1, so resolve the requested episode
            # (source_id like "<id>:s01e03") to its own .mp4 by filename instead.
            ep_m = self._EPISODE_SID_RE.search(source_id or "")
            if ep_m:
                ep_no = int(ep_m.group(2))
                ep_url = self.extract_serial_episode_urls(response.text).get(ep_no)
                if ep_url:
                    title = clean_text((soup.select_one("h1") or soup.select_one("title") or soup).get_text())
                    poster = ""
                    og_img = soup.select_one("meta[property='og:image']")
                    if og_img:
                        poster = normalize_url(og_img.get("content", ""), self.BASE_URL)
                    lower = ep_url.lower()
                    kind = "m3u8" if lower.endswith(".m3u8") else "mp4"
                    logger.info(f"[UZMEDIA] resolved episode {source_id} -> {ep_url}")
                    return MovieDetails(
                        title=title, description="", poster=poster, backdrop="",
                        year=extract_year(title) or 0, genres=[], country="", duration=0,
                        video_page_url=url,
                        video_urls=[{"url": ep_url, "type": kind, "quality": "auto", "label": ""}],
                        source_id=source_id, source=self.source_name,
                    )
                logger.warning(f"[UZMEDIA] episode {source_id} not found on page {url}")

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
                try:
                    from media_extractor import resolve_embed_to_candidates
                    resolved = resolve_embed_to_candidates(v_url, referer=url, session=self.session)
                    if resolved:
                        video_urls.extend(resolved)
                        logger.info(f"[UZMEDIA] Resolved iframe -> {len(resolved)} candidate(s)")
                except Exception as e:
                    logger.warning(f"[UZMEDIA] iframe resolver error: {e}")
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
