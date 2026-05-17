import logging
import re
from typing import Dict, List

from bs4 import BeautifulSoup

from kinolar import KinolarParser
from helpers import canonical_episode_id, extract_source_id, clean_text

logger = logging.getLogger(__name__)


class KinolarSerialParser:
    def __init__(self) -> None:
        self._movie = KinolarParser()

    @property
    def session(self):
        return self._movie.session

    def parse(self, url: str) -> Dict:
        logger.info(f"[KINOLAR SERIAL] parse start url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        soup = BeautifulSoup(resp.text, "lxml")

        title = ""
        title_el = soup.select_one("h1, .title, .film-title")
        if title_el:
            title = clean_text(title_el.get_text())

        poster = ""
        og = soup.select_one("meta[property='og:image']")
        if og:
            poster = og.get("content", "")
            if poster and not poster.startswith("http"):
                poster = self._movie.BASE_URL + poster

        backdrop = ""
        tw = soup.select_one("meta[name='twitter:image']")
        if tw:
            backdrop = tw.get("content", "")
            if backdrop and not backdrop.startswith("http"):
                backdrop = self._movie.BASE_URL + backdrop

        description = ""
        ogd = soup.select_one("meta[property='og:description']")
        if ogd:
            description = clean_text(ogd.get("content", ""))
        
        year = 0
        m_year = re.search(r"(19|20)\d{2}", title) or re.search(r"(19|20)\d{2}", resp.text[:6000])
        if m_year:
            year = int(m_year.group(0))

        parent_id = extract_source_id(url)
        grouped: Dict[tuple[int, int], Dict] = {}

        episode_re = re.compile(r"^(\d+)\s*-\s*qism$", re.IGNORECASE)
        season_ep_re = re.compile(r"^(?:(\d+)\s*-\s*fasl\s+)?(\d+)\s*-\s*qism$", re.IGNORECASE)
        seen_hrefs = set()
        ep_links = []
        
        for a in soup.find_all("a", href=True):
            href = a["href"].strip()
            if not href or href in seen_hrefs or href.startswith("#") or href.startswith("javascript:"):
                continue
            
            label = clean_text(a.get_text(" ", strip=True) or a.get("title", ""))
            m = season_ep_re.search(label) or episode_re.search(label)
            if m:
                seen_hrefs.add(href)
                season = int(m.group(1)) if len(m.groups()) > 1 and m.group(1) else 1
                episode_no = int(m.groups()[-1])
                full_href = href if href.startswith("http") else f"{self._movie.BASE_URL.rstrip('/')}/{href.lstrip('/')}"
                ep_links.append((full_href, season, episode_no, label))

        for full_href, season, episode_no, label in ep_links:
            key = (season, episode_no)
            if key in grouped:
                continue
            try:
                logger.info(f"[KINOLAR SERIAL] fetching subpage S{season}E{episode_no} url={full_href}")
                ep_resp = self.session.get(full_href, timeout=15)
                ep_soup = BeautifulSoup(ep_resp.text, "lxml")
                entries = self._movie._extract_video_urls(ep_soup, full_href)
                video_url = entries[0]["url"] if entries else ""
                
                grouped[key] = {
                    "season": season,
                    "episode": episode_no,
                    "season_number": season,
                    "episode_number": episode_no,
                    "title": f"{episode_no}-qism",
                    "episode_url": full_href,
                    "detail_url": full_href,
                    "source_episode_url": full_href,
                    "source_id": canonical_episode_id(parent_id, season, episode_no),
                    "video_url": video_url,
                    "poster": poster,
                    "quality_urls": {},
                }
            except Exception as e:
                logger.warning(f"[KINOLAR SERIAL] failed to fetch subpage S{season}E{episode_no}: {e}")

        episodes = list(grouped.values())
        episodes.sort(key=lambda x: (x["season"], x["episode"]))

        seasons_index: Dict[int, List[Dict]] = {}
        for item in episodes:
            seasons_index.setdefault(item["season"], []).append(item)
            
        seasons = [
            {"season_number": season_no, "episodes": seasons_index[season_no]}
            for season_no in sorted(seasons_index.keys())
        ]

        return {
            "success": len(episodes) > 0,
            "type": "serial",
            "provider": "kinolar",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
        }