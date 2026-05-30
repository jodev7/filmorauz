import logging
import re
from typing import Dict, List

from bs4 import BeautifulSoup

from uzmedia import UzmediaParser
from helpers import canonical_episode_id, extract_source_id, clean_text

logger = logging.getLogger(__name__)


class UzmediaSerialParser:
    def __init__(self) -> None:
        self._movie = UzmediaParser()

    @property
    def session(self):
        return self._movie.session

    def parse(self, url: str) -> Dict:
        logger.info(f"[UZMEDIA SERIAL] parse start url={url}")
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

        # uzmedia serial pages don't list episodes as "N-qism" anchor links;
        # they embed one player iframe per episode (embed.html?file=<mp4>) plus
        # the same direct .mp4 URLs inline, with the episode number in the
        # filename. Discover all episodes from those URLs in a single page fetch
        # (no slow per-episode subpage requests). Season is always 1 here.
        season = 1
        for episode_no, video_url in sorted(self._movie.extract_serial_episode_urls(resp.text).items()):
            key = (season, episode_no)
            if key in grouped:
                continue
            grouped[key] = {
                "season": season,
                "episode": episode_no,
                "season_number": season,
                "episode_number": episode_no,
                "title": f"{episode_no}-qism",
                "episode_url": url,
                "detail_url": url,
                "source_episode_url": video_url,
                "source_id": canonical_episode_id(parent_id, season, episode_no),
                "video_url": video_url,
                "poster": poster,
                "quality_urls": {},
            }

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
            "provider": "uzmedia",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
        }