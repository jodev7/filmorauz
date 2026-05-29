"""
Uzbeklar.biz serial parser.

Uzbeklar embeds a single playlist URL on the serial detail page:
    file:"https://uzbeklar.biz/serial/<slug>.txt"
The .txt is a plain JSON array of {title, file} entries with flat episode
numbering across all seasons. Season boundaries are flagged inline in
the title text, e.g.:
    "13 qism (1 fasl tugadi)"
    "(2 fasl boshlandi) 14 qism"
We parse those markers to map flat episode positions to (season, episode)
tuples. Where markers are missing the whole show is treated as one season.
"""

from __future__ import annotations

import json
import logging
import re
from typing import Dict, List, Optional, Tuple

from bs4 import BeautifulSoup

from uzbeklar import UzbeklarParser
from helpers import (
    normalize_url,
    clean_text,
    extract_year,
    canonical_episode_id,
    extract_source_id,
)

logger = logging.getLogger(__name__)

_PLAYLIST_RE = re.compile(
    r"""file\s*:\s*["'](?P<url>https?://[^"']+\.txt)["']""", re.IGNORECASE,
)

# Season boundary markers used in episode titles.
# Examples handled:
#   "(2 fasl boshlandi) 14 qism" -> next episode is the start of season 2
#   "13 qism (1 fasl tugadi)"    -> this is the last episode of season 1
_SEASON_BEGIN_RE = re.compile(
    r"\(?\s*(\d+)\s*(?:fasl|mavsum|sezon)\s*boshlandi\s*\)?",
    re.IGNORECASE,
)
_SEASON_END_RE = re.compile(
    r"\(?\s*(\d+)\s*(?:fasl|mavsum|sezon)\s*tugadi\s*\)?",
    re.IGNORECASE,
)
_QISM_NUM_RE = re.compile(r"(\d+)\s*-?\s*qism", re.IGNORECASE)


def _assign_seasons(items: List[Dict]) -> List[Tuple[int, int, str, str]]:
    """Walk the flat playlist and return (season, episode_in_season, title, url).

    Episode numbering restarts at 1 in each new season. When no markers are
    present, everything is season 1.
    """
    out: List[Tuple[int, int, str, str]] = []
    current_season = 1
    ep_in_season = 0
    for entry in items:
        raw_title = clean_text(entry.get("title", "")) or ""
        url = (entry.get("file") or "").strip()
        if not url:
            continue
        # Detect "next season begins HERE" markers BEFORE counting the episode.
        m_begin = _SEASON_BEGIN_RE.search(raw_title)
        if m_begin:
            try:
                current_season = int(m_begin.group(1))
            except ValueError:
                current_season += 1
            ep_in_season = 0
        ep_in_season += 1
        # Clean visible title of the season markers and stray brackets so the
        # episode label in the UI stays readable.
        cleaned_title = raw_title
        cleaned_title = _SEASON_BEGIN_RE.sub("", cleaned_title)
        cleaned_title = _SEASON_END_RE.sub("", cleaned_title)
        cleaned_title = re.sub(r"\(\s*\)", "", cleaned_title)
        cleaned_title = clean_text(cleaned_title) or raw_title or f"{ep_in_season}-qism"
        out.append((current_season, ep_in_season, cleaned_title, url))
        # End-of-season markers do NOT change current_season — they are just
        # informational, and the NEXT title (begin marker) will switch it.
    return out


class UzbeklarSerialParser:
    """Serial-level wrapper around UzbeklarParser."""

    def __init__(self) -> None:
        self._movie = UzbeklarParser()

    @property
    def session(self):
        return self._movie.session

    def parse(self, url: str) -> Dict:
        logger.info(f"[UZBEKLAR SERIAL] parse start url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        html = resp.text
        soup = BeautifulSoup(html, "lxml")

        title = ""
        og = soup.select_one("meta[property='og:title']")
        if og:
            title = clean_text(og.get("content", ""))
        if not title:
            h = soup.select_one("h1, .full-title")
            if h:
                title = clean_text(h.get_text())

        poster = ""
        og_img = soup.select_one("meta[property='og:image']")
        if og_img:
            poster = normalize_url(og_img.get("content", ""), self._movie.BASE_URL)

        backdrop = ""
        tw_img = soup.select_one("meta[name='twitter:image']")
        if tw_img:
            backdrop = normalize_url(tw_img.get("content", ""), self._movie.BASE_URL)

        description = ""
        og_desc = soup.select_one("meta[property='og:description']")
        if og_desc:
            description = clean_text(og_desc.get("content", ""))

        labels = self._movie._extract_inline_labels(soup)
        year = extract_year(labels.get("yili", "")) or 0
        if not year:
            year = extract_year(title) or 0

        playlist_url = self._find_playlist_url(html)
        if not playlist_url:
            return {
                "success": False,
                "error": "no playlist .txt URL found in serial page",
                "provider": "uzbeklar",
            }

        try:
            pl_resp = self.session.get(playlist_url, timeout=30, headers={"Referer": url})
            pl_resp.raise_for_status()
            playlist_items = json.loads(pl_resp.text)
        except Exception as e:
            logger.warning(f"[UZBEKLAR SERIAL] playlist load failed url={playlist_url} err={e}")
            return {
                "success": False,
                "error": f"playlist fetch/parse failed: {e}",
                "provider": "uzbeklar",
            }
        if not isinstance(playlist_items, list):
            return {
                "success": False,
                "error": "playlist .txt is not a JSON array",
                "provider": "uzbeklar",
            }

        ep_tuples = _assign_seasons(playlist_items)
        if not ep_tuples:
            return {
                "success": False,
                "error": "playlist parsed but yielded zero episodes",
                "provider": "uzbeklar",
            }

        parent_id = extract_source_id(url)
        episodes: List[Dict] = []
        for season, ep_num, ep_title, video_url in ep_tuples:
            episodes.append({
                "season": season,
                "episode": ep_num,
                "season_number": season,
                "episode_number": ep_num,
                "title": ep_title,
                "episode_url": url,
                "detail_url": url,
                "source_episode_url": video_url,
                "source_id": canonical_episode_id(parent_id, season, ep_num),
                "video_url": video_url,
                "quality_urls": {},
                "poster": poster,
            })

        seasons_index: Dict[int, List[Dict]] = {}
        for item in episodes:
            seasons_index.setdefault(item["season"], []).append({
                "episode_number": item["episode"],
                "title": item["title"],
                "detail_url": item["detail_url"],
                "source_episode_url": item["source_episode_url"],
                "video_url": item["video_url"],
                "quality_urls": {},
            })
        seasons = [
            {"season_number": s, "episodes": seasons_index[s]}
            for s in sorted(seasons_index.keys())
        ]

        logger.info(
            f"[UZBEKLAR SERIAL] done title={title!r} seasons={len(seasons)} "
            f"episodes={len(episodes)} playlist={playlist_url}"
        )

        return {
            "success": True,
            "type": "serial",
            "provider": "uzbeklar",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
            "warnings": [],
            "missing_numbers": [],
        }

    @staticmethod
    def _find_playlist_url(html: str) -> Optional[str]:
        m = _PLAYLIST_RE.search(html)
        return m.group("url") if m else None
