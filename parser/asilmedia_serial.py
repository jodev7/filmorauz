"""
Asilmedia serial parser.

Modern Asilmedia serial pages expose episode links inside the hidden
`#episodes-raw-data` block. Each anchor contains:
  - `href`: direct mp4/m3u8 URL for one quality
  - `data-label`: season / episode / quality label

The player UI is built client-side from those anchors, so the parser must read
the hidden source data instead of requiring a video URL on a visible episode
button. We normalize that block into both:
  - flat `episodes[]` for the current Go backend contract
  - grouped `seasons[]` for richer serial consumers
"""

from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional
from urllib.parse import unquote, urlsplit

from bs4 import BeautifulSoup

from asilmedia import AsilmediaParser
from helpers import canonical_episode_id, extract_source_id

logger = logging.getLogger(__name__)


_VIDEO_URL_RE = re.compile(
    r'https?://[^"\'<>]+?\.(?:mp4|m3u8|mpd)',
    re.IGNORECASE,
)
_QUALITY_RE = re.compile(r"(\d{3,4})p", re.IGNORECASE)
_LABEL_SEASON_EP_RE = re.compile(
    r"(?:(\d{1,2})\s*-\s*fasl\s+)?(\d{1,4})\s*-\s*qism",
    re.IGNORECASE,
)
_FILENAME_SEASON_EP_RE = re.compile(
    r"(?<!\d)(\d{1,2})\s*-\s*(\d{1,4})(?!\d)",
    re.IGNORECASE,
)
_FILENAME_EP_RE = re.compile(r"(?<![\w])(\d{1,4})[-\s]?qism", re.IGNORECASE)
_TRAILING_STATUS_RE = re.compile(r"\s*\|\s*.*$")


def _rank_quality(url: str) -> int:
    m = _QUALITY_RE.search(url)
    return int(m.group(1)) if m else 0


def _parse_episode_title(label: str, episode_no: int) -> str:
    cleaned = _TRAILING_STATUS_RE.sub("", (label or "").strip())
    cleaned = _QUALITY_RE.sub("", cleaned).strip(" -|")
    return cleaned or f"{episode_no}-qism"


def _decoded_filename(url: str) -> str:
    try:
        path = urlsplit(url).path or ""
    except Exception:
        return url
    return unquote(path.rsplit("/", 1)[-1])


def _parse_label_season_episode(label: str) -> Optional[tuple[int, int]]:
    m = _LABEL_SEASON_EP_RE.search(label or "")
    if not m:
        return None
    season = int(m.group(1)) if m.group(1) else 1
    episode = int(m.group(2))
    return season, episode


def _parse_url_season_episode(url: str) -> Optional[tuple[int, int]]:
    decoded = _decoded_filename(url)
    m = _FILENAME_SEASON_EP_RE.search(decoded)
    if m:
        return int(m.group(1)), int(m.group(2))
    m = _FILENAME_EP_RE.search(decoded)
    if m:
        return 1, int(m.group(1))
    return None


def _extract_title(soup: BeautifulSoup) -> str:
    og_title = soup.select_one("meta[property='og:title']")
    if og_title:
        raw = (og_title.get("content") or "").strip()
        if raw:
            return raw.split("»", 1)[0].split("&raquo;", 1)[0].strip()
    h1 = soup.select_one("h1")
    return h1.get_text(strip=True) if h1 else ""


def _extract_meta_content(soup: BeautifulSoup, selector: str) -> str:
    node = soup.select_one(selector)
    return (node.get("content") or "").strip() if node else ""


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

        title = _extract_title(soup)
        poster = _extract_meta_content(soup, "meta[property='og:image']")
        backdrop = _extract_meta_content(soup, "meta[name='twitter:image']")
        description = _extract_meta_content(soup, "meta[property='og:description']")

        year = 0
        m_year = re.search(r"(19|20)\d{2}", title) or re.search(r"(19|20)\d{2}", html[:6000])
        if m_year:
            year = int(m_year.group(0))

        grouped: Dict[tuple[int, int], Dict] = {}
        duplicates_removed = 0

        # Primary path: modern Asilmedia serial data lives in the hidden anchor
        # bundle the frontend JS uses to build the episode picker.
        raw_section = soup.select_one("#episodes-raw-data")
        if raw_section:
            for anchor in raw_section.select("a[href]"):
                href = (anchor.get("href") or "").strip()
                if not href:
                    continue
                href = self._movie._sanitize_video_url(href)
                if not _VIDEO_URL_RE.search(href):
                    continue

                label = (anchor.get("data-label") or anchor.get_text(" ", strip=True) or "").strip()
                parsed = _parse_label_season_episode(label) or _parse_url_season_episode(href)
                if not parsed:
                    logger.info(
                        "[ASILMEDIA SERIAL] could not map raw anchor to episode "
                        f"label={label!r} href={href[:120]}"
                    )
                    continue

                season, episode_no = parsed
                key = (season, episode_no)
                quality_match = _QUALITY_RE.search(label) or _QUALITY_RE.search(href)
                quality_key = f"{quality_match.group(1)}p" if quality_match else "unknown"
                if key in grouped:
                    duplicates_removed += 1
                item = grouped.setdefault(
                    key,
                    {
                        "season": season,
                        "episode": episode_no,
                        "season_number": season,
                        "episode_number": episode_no,
                        "title": _parse_episode_title(label, episode_no),
                        "episode_url": url,
                        "detail_url": url,
                        "source_episode_url": url,
                        "source_id": canonical_episode_id(parent_id, season, episode_no),
                        "video_url": "",
                        "poster": poster,
                        "quality_urls": {},
                    },
                )
                if key not in grouped:
                    logger.info(f"[episode-parse] title={item['title']} parent={parent_id} season={season} episode={episode_no} source_id={item['source_id']}")
                item["quality_urls"][quality_key] = href
                current = item.get("video_url") or ""
                if not current or _rank_quality(href) > _rank_quality(current):
                    item["video_url"] = href

        # Fallback for older layouts or inline raw HTML references.
        if not grouped:
            raw_urls: set[str] = set()
            for el in soup.find_all(True):
                for attr in ("href", "src", "data-src", "data-url", "data-file"):
                    val = el.get(attr)
                    if isinstance(val, str) and _VIDEO_URL_RE.search(val):
                        m = _VIDEO_URL_RE.search(val)
                        if m:
                            raw_urls.add(self._movie._sanitize_video_url(m.group(0).strip()))
            for m in _VIDEO_URL_RE.finditer(html):
                raw_urls.add(self._movie._sanitize_video_url(m.group(0).strip()))

            for href in raw_urls:
                parsed = _parse_url_season_episode(href)
                if not parsed:
                    continue
                season, episode_no = parsed
                key = (season, episode_no)
                if key in grouped:
                    duplicates_removed += 1
                item = grouped.setdefault(
                    key,
                    {
                        "season": season,
                        "episode": episode_no,
                        "season_number": season,
                        "episode_number": episode_no,
                        "title": f"{episode_no}-qism",
                        "episode_url": url,
                        "detail_url": url,
                        "source_episode_url": url,
                        "source_id": canonical_episode_id(parent_id, season, episode_no),
                        "video_url": "",
                        "poster": poster,
                        "quality_urls": {},
                    },
                )
                logger.info(f"[episode-parse] title={item['title']} parent={parent_id} season={season} episode={episode_no} source_id={item['source_id']}")
                quality_match = _QUALITY_RE.search(href)
                quality_key = f"{quality_match.group(1)}p" if quality_match else "unknown"
                item["quality_urls"][quality_key] = href
                current = item.get("video_url") or ""
                if not current or _rank_quality(href) > _rank_quality(current):
                    item["video_url"] = href

        if not grouped:
            logger.warning(
                f"[ASILMEDIA SERIAL] no episodes discovered on serial page url={url}"
            )
            return {
                "success": False,
                "error": "asilmedia: no episode data found on serial page",
                "provider": "asilmedia",
            }

        flat_episodes: List[Dict] = []
        seasons_index: Dict[int, List[Dict]] = {}
        resolved = 0

        for key in sorted(grouped.keys()):
            item = grouped[key]
            if item.get("video_url"):
                resolved += 1
            else:
                item["error"] = "video_url not extracted for episode"
            flat_episodes.append(item)
            seasons_index.setdefault(item["season"], []).append(
                {
                    "episode_number": item["episode"],
                    "title": item["title"],
                    "detail_url": item["detail_url"],
                    "source_episode_url": item["source_episode_url"],
                    "video_url": item.get("video_url", ""),
                    "quality_urls": item.get("quality_urls") or {},
                    **({"error": item["error"]} if item.get("error") else {}),
                }
            )

        seasons = [
            {
                "season_number": season_no,
                "episodes": seasons_index[season_no],
            }
            for season_no in sorted(seasons_index.keys())
        ]

        missing_numbers = self._compute_missing_numbers(flat_episodes)
        logger.info(
            f"[serial-details] source=asilmedia title={title!r} seasons={len(seasons)} "
            f"episodes_found={len(flat_episodes)} missing_numbers={missing_numbers} duplicates_removed={duplicates_removed}"
        )
        logger.info(
            f"[episode extractor] source=asilmedia found_visible={len(flat_episodes)} "
            f"found_script=0 found_pattern=0 final={len(flat_episodes)}"
        )
        warnings: List[str] = []
        if missing_numbers:
            warnings.append("Possible missing episodes: " + ",".join(str(n) for n in missing_numbers))
            logger.warning(f"[episode extractor] missing numbers={missing_numbers}")
        logger.info(
            f"[ASILMEDIA SERIAL] done title={title!r} episodes={len(flat_episodes)} resolved={resolved}"
        )

        return {
            "success": len(flat_episodes) > 0,
            "type": "serial",
            "provider": "asilmedia",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": flat_episodes,
            "seasons": seasons,
            "warnings": warnings,
            "missing_numbers": missing_numbers,
        }

    def _compute_missing_numbers(self, episodes: List[Dict]) -> List[int]:
        by_season: Dict[int, set[int]] = {}
        for item in episodes:
            by_season.setdefault(item["season"], set()).add(item["episode"])
        missing: List[int] = []
        for season_no in sorted(by_season.keys()):
            nums = sorted(by_season[season_no])
            if not nums:
                continue
            for expected in range(nums[0], nums[-1] + 1):
                if expected not in by_season[season_no]:
                    missing.append(expected)
        return missing
