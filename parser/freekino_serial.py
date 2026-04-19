"""
Freekino serial parser.

Freekino exposes each episode at its own page: /serie/<id>-<slug>-<N>-fasl-<M>-qism
So the episode list is harvested statically from the serial detail page, and
each episode's video URL is extracted by visiting that episode's page and
reusing the existing movie-level video extractor (which already handles the
Playerjs encoded `file:` strings).
"""

from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional

from bs4 import BeautifulSoup

from freekino import FreekinoParser, clean_text, normalize_url, extract_year

logger = logging.getLogger(__name__)

# -(\d+)-fasl-(\d+)-qism  OR  -(\d+)-qism  (season defaults to 1 if omitted)
_EP_URL_RE = re.compile(
    r"/serie/\d+-[a-z0-9\-]*?(?:-(\d+)-fasl)?-(\d+)-qism",
    re.IGNORECASE,
)


def _parse_season_episode(url: str) -> Optional[tuple[int, int]]:
    m = _EP_URL_RE.search(url)
    if not m:
        return None
    season = int(m.group(1)) if m.group(1) else 1
    episode = int(m.group(2))
    return season, episode


class FreekinoSerialParser:
    """Serial-level wrapper around FreekinoParser."""

    def __init__(self) -> None:
        self._movie = FreekinoParser()  # reuse session + cookies
        self.session = self._movie.session

    def parse(self, url: str) -> Dict:
        logger.info(f"[FREEKINO SERIAL] parse start url={url}")
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
            poster = normalize_url(self._movie.BASE_URL, og.get("content", ""))

        backdrop = ""
        tw = soup.select_one("meta[name='twitter:image']")
        if tw:
            backdrop = normalize_url(self._movie.BASE_URL, tw.get("content", ""))

        description = ""
        ogd = soup.select_one("meta[property='og:description']")
        if ogd:
            description = clean_text(ogd.get("content", ""))
        if not description:
            desc_el = soup.select_one(".description, .desc, .synopsis, .text")
            if desc_el:
                description = clean_text(desc_el.get_text())

        year = 0
        year_el = soup.select_one(".year, [class*='year'], .date")
        if year_el:
            year = extract_year(year_el.get_text()) or 0

        # Harvest unique episode links, preserving order.
        seen = set()
        ep_links: List[tuple[str, int, int, str]] = []
        for a in soup.find_all("a", href=True):
            href = a["href"]
            if "/serie/" not in href:
                continue
            href = normalize_url(self._movie.BASE_URL, href)
            if href in seen:
                continue
            seen.add(href)
            parsed = _parse_season_episode(href)
            if not parsed:
                continue
            season, episode = parsed
            ep_links.append((href, season, episode, clean_text(a.get_text())))

        ep_links.sort(key=lambda x: (x[1], x[2]))
        logger.info(f"[FREEKINO SERIAL] title={title!r} episodes_found={len(ep_links)}")

        if not ep_links:
            return {
                "success": False,
                "error": "no episodes found on serial page",
                "provider": "freekino",
            }

        episodes: List[Dict] = []
        failures = 0
        for href, season, episode, link_text in ep_links:
            ep_title = link_text or f"{episode}-qism"
            try:
                ep_resp = self.session.get(href, timeout=30)
                ep_resp.raise_for_status()
                ep_soup = BeautifulSoup(ep_resp.text, "lxml")
                entries, _ = self._movie._extract_video(ep_soup, ep_resp.url)
                video_url = entries[0]["url"] if entries else ""
                if video_url:
                    logger.info(
                        f"[FREEKINO SERIAL] S{season:02d}E{episode:02d} OK src={video_url[:80]}"
                    )
                else:
                    failures += 1
                    logger.warning(
                        f"[FREEKINO SERIAL] S{season:02d}E{episode:02d} no video extracted href={href}"
                    )
            except Exception as e:
                failures += 1
                video_url = ""
                logger.warning(
                    f"[FREEKINO SERIAL] S{season:02d}E{episode:02d} failed href={href} err={e}"
                )

            episodes.append(
                {
                    "season": season,
                    "episode": episode,
                    "title": ep_title,
                    "episode_url": href,
                    "video_url": video_url,
                    "poster": poster,
                }
            )

        resolved = sum(1 for ep in episodes if ep["video_url"])
        logger.info(
            f"[FREEKINO SERIAL] done title={title!r} resolved={resolved}/{len(episodes)} failures={failures}"
        )

        return {
            "success": resolved > 0,
            "type": "serial",
            "provider": "freekino",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
        }
