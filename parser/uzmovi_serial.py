"""
Uzmovi serial parser.

Uzmovi episode URLs follow the pattern
  /<category>/<id>-<slug>/episode/<group_id>/<episode_no>.html
and each episode page hosts an <iframe> pointing at an embed (typically
uzdown.live/embed/<id>?episode=<n>), whose HTML contains a Playerjs-style
`file:"<direct .m3u8 url>"`. Multiple translation variants (distinct group_ids)
can coexist; we pick the first group encountered and use that consistently.
"""

from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional

from bs4 import BeautifulSoup

from uzmovi import UzmoviParser

logger = logging.getLogger(__name__)


_EP_HREF_RE = re.compile(r"/episode/(\d+)/(\d+)\.html", re.IGNORECASE)
_FILE_RE = re.compile(r'file\s*:\s*["\']([^"\']+)["\']', re.IGNORECASE)


def _text(el) -> str:
    return (el.get_text(strip=True) if el else "") or ""


def _extract_year(text: str) -> int:
    m = re.search(r"(19|20)\d{2}", text or "")
    return int(m.group(0)) if m else 0


class UzmoviSerialParser:
    def __init__(self) -> None:
        self._movie = UzmoviParser()
        self.session = self._movie.session
        self.base_url = getattr(self._movie, "BASE_URL", None) or "https://uzmovi.tv"

    def parse(self, url: str) -> Dict:
        logger.info(f"[UZMOVI SERIAL] parse start url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        soup = BeautifulSoup(resp.text, "lxml")

        title = _text(soup.select_one("h1")) or _text(soup.select_one("title"))
        title = title.strip()

        poster = ""
        og = soup.select_one("meta[property='og:image']")
        if og:
            poster = og.get("content", "").strip()

        backdrop = ""
        tw = soup.select_one("meta[name='twitter:image']")
        if tw:
            backdrop = tw.get("content", "").strip()

        description = ""
        ogd = soup.select_one("meta[property='og:description']")
        if ogd:
            description = ogd.get("content", "").strip()
        if not description:
            description = _text(soup.select_one(".full-story, .fullstory, .description"))

        year = _extract_year(title) or _extract_year(resp.text[:4000])

        # Collect episode links, picking a single group_id (first seen) so we
        # don't double-count translation variants.
        chosen_group: Optional[str] = None
        per_episode: Dict[int, Dict] = {}

        for a in soup.find_all("a", href=True):
            href = a["href"]
            m = _EP_HREF_RE.search(href)
            if not m:
                continue
            group_id, ep_no = m.group(1), int(m.group(2))
            if chosen_group is None:
                chosen_group = group_id
                logger.info(f"[UZMOVI SERIAL] chose translation group_id={group_id}")
            if group_id != chosen_group:
                continue
            full = href if href.startswith("http") else f"{self.base_url.rstrip('/')}{href}"
            if ep_no in per_episode:
                continue
            per_episode[ep_no] = {
                "episode": ep_no,
                "season": 1,
                "title": a.get_text(strip=True) or f"{ep_no}-qism",
                "episode_url": full,
            }

        ep_nos = sorted(per_episode.keys())
        logger.info(f"[UZMOVI SERIAL] title={title!r} episodes_found={len(ep_nos)}")

        if not ep_nos:
            return {
                "success": False,
                "error": "no episodes found on serial page",
                "provider": "uzmovi",
            }

        episodes: List[Dict] = []
        for ep_no in ep_nos:
            entry = per_episode[ep_no]
            video_url = self._extract_episode_video(entry["episode_url"])
            if video_url:
                logger.info(
                    f"[UZMOVI SERIAL] S01E{ep_no:02d} OK src={video_url[:80]}"
                )
            else:
                logger.warning(
                    f"[UZMOVI SERIAL] S01E{ep_no:02d} failed to resolve video url={entry['episode_url']}"
                )
            entry["video_url"] = video_url
            entry["poster"] = poster
            episodes.append(entry)

        resolved = sum(1 for ep in episodes if ep["video_url"])
        logger.info(
            f"[UZMOVI SERIAL] done title={title!r} resolved={resolved}/{len(episodes)}"
        )

        return {
            "success": resolved > 0,
            "type": "serial",
            "provider": "uzmovi",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
        }

    def _extract_episode_video(self, episode_url: str) -> str:
        try:
            resp = self.session.get(episode_url, timeout=30)
            resp.raise_for_status()
            soup = BeautifulSoup(resp.text, "lxml")
            iframe = soup.select_one("iframe[src]")
            if not iframe:
                return ""
            embed_src = iframe.get("src", "").strip()
            if not embed_src:
                return ""
            if embed_src.startswith("//"):
                embed_src = "https:" + embed_src
            embed_resp = self.session.get(
                embed_src,
                timeout=30,
                headers={"Referer": episode_url},
            )
            embed_resp.raise_for_status()
            m = _FILE_RE.search(embed_resp.text)
            if not m:
                return ""
            return m.group(1).strip()
        except Exception as e:
            logger.warning(f"[UZMOVI SERIAL] episode fetch error url={episode_url} err={e}")
            return ""
