"""
Uzmovi serial parser.

Uzmovi serial pages usually expose episode URLs via:
  - visible or hidden episode anchors
  - collapsed tabs/accordions still present in HTML
  - inline script / data blobs containing `/episode/<group>/<n>.html`

Each episode page then embeds the actual player in an iframe; the iframe HTML
typically contains a Playerjs-style `file:"<direct .m3u8 url>"`.
"""

from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional

from bs4 import BeautifulSoup

from uzmovi import UzmoviParser

logger = logging.getLogger(__name__)


_EP_HREF_RE = re.compile(
    r"https?://[^\"'<> ]+/episode/(\d+)/(\d+)\.html|/episode/(\d+)/(\d+)\.html",
    re.IGNORECASE,
)
_FILE_RE = re.compile(r'file\s*:\s*["\']([^"\']+)["\']', re.IGNORECASE)
_EP_LABEL_RE = re.compile(r"(\d{1,3})\s*-\s*qism", re.IGNORECASE)
_SEASON_LABEL_RE = re.compile(r"(\d{1,2})\s*-\s*(?:fasl|mavsum|sezon)", re.IGNORECASE)
_SEASON_TITLE_RE = re.compile(r"(\d{1,2})\s*(?:fasl|mavsum|sezon)", re.IGNORECASE)


def _text(el) -> str:
    return (el.get_text(strip=True) if el else "") or ""


def _extract_year(text: str) -> int:
    m = re.search(r"(19|20)\d{2}", text or "")
    return int(m.group(0)) if m else 0


def _normalize_episode_href(base_url: str, href: str) -> str:
    href = (href or "").strip()
    if not href:
        return ""
    if href.startswith("http://") or href.startswith("https://"):
        return href
    return f"{base_url.rstrip('/')}{href}"


def _parse_episode_href(href: str) -> Optional[tuple[str, int]]:
    m = _EP_HREF_RE.search(href or "")
    if not m:
        return None
    group_id = m.group(1) or m.group(3)
    episode_no = m.group(2) or m.group(4)
    if not group_id or not episode_no:
        return None
    return group_id, int(episode_no)


def _parse_season_number(text: str) -> int:
    m = _SEASON_LABEL_RE.search(text or "") or _SEASON_TITLE_RE.search(text or "")
    return int(m.group(1)) if m else 1


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

        episode_candidates = self._collect_episode_candidates(soup, resp.text)
        if not episode_candidates:
            return {
                "success": False,
                "error": "no episodes found on serial page",
                "provider": "uzmovi",
            }

        # Prefer groups with broader coverage but still keep all candidate URLs
        # for each (season, episode) pair so later groups can fill holes.
        coverage_by_group: Dict[str, set[tuple[int, int]]] = {}
        for item in episode_candidates:
            coverage_by_group.setdefault(item["_group_id"], set()).add((item["season"], item["episode"]))
        group_priority = {
            gid: len(pairs)
            for gid, pairs in coverage_by_group.items()
        }

        grouped_candidates: Dict[tuple[int, int], List[Dict]] = {}
        for item in episode_candidates:
            key = (item["season"], item["episode"])
            grouped_candidates.setdefault(key, []).append(item)

        inventory: List[Dict] = []
        duplicates_removed = 0
        for key in sorted(grouped_candidates.keys()):
            choices = grouped_candidates[key]
            if len(choices) > 1:
                duplicates_removed += len(choices) - 1
            choices.sort(
                key=lambda item: (
                    -group_priority.get(item["_group_id"], 0),
                    item["_group_id"],
                    item["episode_url"],
                )
            )
            inventory.append(choices[0])

        resolved_rows: List[Dict] = []
        seen_final: set[tuple[int, int, str]] = set()

        for entry in inventory:
            video_url = self._extract_episode_video(entry["episode_url"])
            if video_url:
                logger.info(
                    f"[UZMOVI SERIAL] S{entry['season']:02d}E{entry['episode']:02d} OK src={video_url[:80]}"
                )
            else:
                logger.warning(
                    f"[UZMOVI SERIAL] S{entry['season']:02d}E{entry['episode']:02d} failed to resolve video url={entry['episode_url']}"
                )

            dedupe_key = (entry["season"], entry["episode"], video_url or entry["episode_url"])
            if dedupe_key in seen_final:
                continue
            seen_final.add(dedupe_key)

            resolved_rows.append(
                {
                    "season": entry["season"],
                    "episode": entry["episode"],
                    "season_number": entry["season"],
                    "episode_number": entry["episode"],
                    "title": entry["title"],
                    "episode_url": entry["episode_url"],
                    "detail_url": entry["episode_url"],
                    "source_episode_url": entry["episode_url"],
                    "video_url": video_url,
                    "quality_urls": {},
                    "poster": poster,
                    **({"error": "video_url not extracted for episode"} if not video_url else {}),
                }
            )

        resolved_rows.sort(key=lambda item: (item["season"], item["episode"], item["video_url"] or item["episode_url"]))

        seasons_index: Dict[int, List[Dict]] = {}
        for item in resolved_rows:
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
            {"season_number": season_no, "episodes": seasons_index[season_no]}
            for season_no in sorted(seasons_index.keys())
        ]

        missing_numbers = self._compute_missing_numbers(resolved_rows)
        logger.info(
            f"[serial-details] source=uzmovi title={title!r} seasons={len(seasons)} "
            f"episodes_found={len(resolved_rows)} missing_numbers={missing_numbers} duplicates_removed={duplicates_removed}"
        )

        resolved = sum(1 for ep in resolved_rows if ep["video_url"])
        logger.info(
            f"[UZMOVI SERIAL] done title={title!r} resolved={resolved}/{len(resolved_rows)}"
        )

        return {
            "success": len(resolved_rows) > 0,
            "type": "serial",
            "provider": "uzmovi",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": resolved_rows,
            "seasons": seasons,
        }

    def _collect_episode_candidates(self, soup: BeautifulSoup, html: str) -> List[Dict]:
        per_group: Dict[str, Dict[tuple[int, int], Dict]] = {}

        def add_candidate(href: str, label: str = "", season_hint: Optional[int] = None) -> None:
            parsed = _parse_episode_href(href)
            if not parsed:
                return
            group_id, ep_no = parsed
            full = _normalize_episode_href(self.base_url, href)
            season_no = season_hint or _parse_season_number(label) or 1
            title = (label or f"{ep_no}-qism").strip()
            key = (season_no, ep_no)
            group_map = per_group.setdefault(group_id, {})
            existing = group_map.get(key)
            if existing and len(existing["title"]) >= len(title):
                return
            group_map[key] = {
                "season": season_no,
                "episode": ep_no,
                "title": title or f"{ep_no}-qism",
                "episode_url": full,
                "_group_id": group_id,
            }

        # 1) Parsed DOM anchors, including hidden tabs/accordions still in HTML.
        for a in soup.find_all("a", href=True):
            href = a.get("href", "")
            if "/episode/" not in href:
                continue
            add_candidate(
                href,
                label=a.get_text(" ", strip=True),
                season_hint=_parse_season_number(a.get("title", "") or a.get_text(" ", strip=True)),
            )

        # 2) Data attributes / non-anchor controls.
        for el in soup.find_all(True):
            label = el.get_text(" ", strip=True)
            season_hint = _parse_season_number(label)
            for attr in ("data-href", "data-url", "data-episode-url", "data-link", "href"):
                href = el.get(attr) if hasattr(el, "get") else None
                if not href or "/episode/" not in str(href):
                    continue
                add_candidate(str(href), label=label, season_hint=season_hint)

        # 3) Raw HTML / inline scripts / hidden JSON blobs.
        for m in _EP_HREF_RE.finditer(html or ""):
            raw_href = m.group(0)
            season_hint = 1
            prefix = (html or "")[max(0, m.start() - 120):m.start()]
            season_hint = _parse_season_number(prefix) or 1
            add_candidate(raw_href, label="", season_hint=season_hint)

        flat: List[Dict] = []
        for gid, gmap in sorted(per_group.items(), key=lambda kv: len(kv[1]), reverse=True):
            logger.info(
                f"[UZMOVI SERIAL] translation group_id={gid} total_in_group={len(gmap)}"
            )
            flat.extend(gmap.values())
        return flat

    def _compute_missing_numbers(self, episodes: List[Dict]) -> List[int]:
        if not episodes:
            return []
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

    def _extract_episode_video(self, episode_url: str) -> str:
        try:
            resp = self.session.get(episode_url, timeout=30)
            resp.raise_for_status()
            soup = BeautifulSoup(resp.text, "lxml")

            # Visible iframe.
            iframe = soup.select_one("iframe[src]")
            if iframe:
                embed_src = iframe.get("src", "").strip()
                if embed_src:
                    video = self._extract_video_from_embed(embed_src, episode_url)
                    if video:
                        return video

            # Hidden player/data attrs on the episode page itself.
            for el in soup.find_all(True):
                for attr in ("data-src", "data-video", "data-url", "data-file", "data-player"):
                    val = el.get(attr) if hasattr(el, "get") else None
                    if not val:
                        continue
                    if ".m3u8" in str(val) or ".mp4" in str(val):
                        return str(val).strip()
                    if "embed" in str(val) or "player" in str(val):
                        video = self._extract_video_from_embed(str(val).strip(), episode_url)
                        if video:
                            return video

            # Script/player config on the episode page.
            m = _FILE_RE.search(resp.text)
            if m:
                return m.group(1).strip()
            return ""
        except Exception as e:
            logger.warning(f"[UZMOVI SERIAL] episode fetch error url={episode_url} err={e}")
            return ""

    def _extract_video_from_embed(self, embed_src: str, episode_url: str) -> str:
        if embed_src.startswith("//"):
            embed_src = "https:" + embed_src
        elif embed_src.startswith("/"):
            embed_src = f"{self.base_url.rstrip('/')}{embed_src}"
        try:
            embed_resp = self.session.get(
                embed_src,
                timeout=30,
                headers={"Referer": episode_url},
            )
            embed_resp.raise_for_status()

            m = _FILE_RE.search(embed_resp.text)
            if m:
                return m.group(1).strip()

            iframe_soup = BeautifulSoup(embed_resp.text, "lxml")
            nested_iframe = iframe_soup.select_one("iframe[src]")
            if nested_iframe:
                nested_src = nested_iframe.get("src", "").strip()
                if nested_src and nested_src != embed_src:
                    return self._extract_video_from_embed(nested_src, episode_url)

            for el in iframe_soup.find_all(True):
                for attr in ("data-src", "data-video", "data-url", "data-file"):
                    val = el.get(attr) if hasattr(el, "get") else None
                    if val and (".m3u8" in str(val) or ".mp4" in str(val)):
                        return str(val).strip()
            return ""
        except Exception as e:
            logger.warning(f"[UZMOVI SERIAL] embed fetch error url={embed_src} err={e}")
            return ""
