"""
Asilmedia serial parser.

Asilmedia renders a single serial page and swaps episodes via JS — the page URL
does not change per episode. However, every mp4/m3u8 source for every episode
(and every quality) is emitted directly into the HTML as plain links to the
`fayllar*` CDN. Filenames embed the episode number (`<Title> N-qism ... .mp4`),
so no browser is needed — we just group the URLs by episode number, pick the
highest quality for each, and return the list. If no such URLs exist on the
page we fail explicitly (no fake success).
"""

from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional

from bs4 import BeautifulSoup

from asilmedia import AsilmediaParser

logger = logging.getLogger(__name__)


# Matches direct CDN URLs for episodes. Asilmedia emits URLs with *literal
# spaces* inside filenames (e.g. `Bosqin 1-qism 1080p (asilmedia.net).mp4`)
# and embeds them in `<a href="…">`, so we need to allow spaces + parentheses
# up to the file extension.
_VIDEO_URL_RE = re.compile(
    r'https?://[^"\'<>]+?\.(?:mp4|m3u8|mpd)',
    re.IGNORECASE,
)

# Pull episode number from the filename segment, e.g. "... 12-qism ...".
_EPISODE_RE = re.compile(r"(?<![\w])(\d{1,4})[-\s]?qism", re.IGNORECASE)

# Pull season number (fasl / mavsum), e.g. "... 2-fasl ..." or "... 2-mavsum ...".
_SEASON_RE = re.compile(r"(?<![\w])(\d{1,2})[-\s]?(?:fasl|mavsum|sezon)", re.IGNORECASE)

# Quality label inside the filename: "1080p", "720p", "480p", etc.
_QUALITY_RE = re.compile(r"(\d{3,4})p", re.IGNORECASE)


def _rank_quality(url: str) -> int:
    m = _QUALITY_RE.search(url)
    return int(m.group(1)) if m else 0


def _episode_from_url(url: str) -> Optional[int]:
    m = _EPISODE_RE.search(url)
    return int(m.group(1)) if m else None


def _season_from_url(url: str) -> int:
    m = _SEASON_RE.search(url)
    return int(m.group(1)) if m else 1


class AsilmediaSerialParser:
    def __init__(self) -> None:
        self._movie = AsilmediaParser()
        self.session = self._movie.session
        self.base_url = getattr(self._movie, "BASE_URL", None) or "https://asilmedia.org"

    def parse(self, url: str) -> Dict:
        logger.info(f"[ASILMEDIA SERIAL] parse start url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        html = resp.text
        soup = BeautifulSoup(html, "lxml")

        title = ""
        og_title = soup.select_one("meta[property='og:title']")
        if og_title:
            title = (og_title.get("content") or "").strip()
        if not title:
            h1 = soup.select_one("h1")
            if h1:
                title = h1.get_text(strip=True)

        poster = ""
        og_img = soup.select_one("meta[property='og:image']")
        if og_img:
            poster = (og_img.get("content") or "").strip()

        backdrop = ""
        tw_img = soup.select_one("meta[name='twitter:image']")
        if tw_img:
            backdrop = (tw_img.get("content") or "").strip()

        description = ""
        og_desc = soup.select_one("meta[property='og:description']")
        if og_desc:
            description = (og_desc.get("content") or "").strip()

        year = 0
        m_year = re.search(r"(19|20)\d{2}", title) or re.search(r"(19|20)\d{2}", html[:6000])
        if m_year:
            year = int(m_year.group(0))

        # Harvest every direct video URL on the page. We check both parsed
        # element attributes (to preserve URL-embedded spaces that regex
        # otherwise trims) and the raw HTML (for inline JSON/JS references).
        raw_urls: set[str] = set()
        for el in soup.find_all(True):
            for attr in ("href", "src", "data-src", "data-url", "data-file"):
                val = el.get(attr)
                if isinstance(val, str) and _VIDEO_URL_RE.search(val):
                    m = _VIDEO_URL_RE.search(val)
                    if m:
                        raw_urls.add(m.group(0).strip())
        for m in _VIDEO_URL_RE.finditer(html):
            raw_urls.add(m.group(0).strip())
        logger.info(f"[ASILMEDIA SERIAL] raw video urls discovered={len(raw_urls)}")

        # Group by (season, episode) and keep the highest-quality URL per group.
        grouped: Dict[tuple[int, int], str] = {}
        for u in raw_urls:
            ep = _episode_from_url(u)
            if ep is None:
                continue
            season = _season_from_url(u)
            key = (season, ep)
            current = grouped.get(key)
            if current is None or _rank_quality(u) > _rank_quality(current):
                grouped[key] = u

        if not grouped:
            logger.warning(
                f"[ASILMEDIA SERIAL] no episode-tagged video urls on page url={url} — "
                f"player may have moved to a JS-only API; cannot resolve without a browser"
            )
            return {
                "success": False,
                "error": "asilmedia: no episode-tagged video URLs found on serial page",
                "provider": "asilmedia",
            }

        episodes: List[Dict] = []
        for (season, ep_no) in sorted(grouped.keys()):
            video_url = grouped[(season, ep_no)]
            logger.info(
                f"[ASILMEDIA SERIAL] S{season:02d}E{ep_no:02d} resolved src={video_url[:90]}"
            )
            episodes.append(
                {
                    "season": season,
                    "episode": ep_no,
                    "title": f"{ep_no}-qism",
                    # The serial page URL is the only URL that exists — episodes
                    # are switched via JS without a URL change, so we return
                    # the serial URL itself for traceability.
                    "episode_url": url,
                    "video_url": video_url,
                    "poster": poster,
                }
            )

        logger.info(
            f"[ASILMEDIA SERIAL] done title={title!r} resolved={len(episodes)}"
        )

        return {
            "success": True,
            "type": "serial",
            "provider": "asilmedia",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
        }
