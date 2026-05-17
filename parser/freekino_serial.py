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
from helpers import canonical_episode_id, extract_source_id

logger = logging.getLogger(__name__)

# -(\d+)-fasl-(\d+)-qism  OR  -(\d+)-qism  (season defaults to 1 if omitted)
_EP_URL_RE = re.compile(
    r"/serie/\d+-[a-z0-9\-]*?(?:-(\d+)-(?:fasl|mavsum|sezon))?-(\d+)-qism",
    re.IGNORECASE,
)
_EP_URL_ANY_RE = re.compile(
    r"https?://[^\"'<> ]+/serie/\d+-[a-z0-9\-]*?(?:-(\d+)-(?:fasl|mavsum|sezon))?-(\d+)-qism|/serie/\d+-[a-z0-9\-]*?(?:-(\d+)-(?:fasl|mavsum|sezon))?-(\d+)-qism",
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

    @property
    def session(self):
        # Delegate to the movie parser so we always pick up the per-thread
        # session (FreekinoParser.session is thread-local).
        return self._movie.session

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

        source_id = extract_source_id(url)
        parent_id = source_id

        ep_links, duplicates_removed, source_counts = self._collect_episode_links(soup, resp.text)

        # URL pattern fallback: derive a template from the longest-matching
        # existing episode URL and fill any gaps between min..max episode.
        pattern_added = 0
        if ep_links:
            by_season: Dict[int, List[tuple[str, int, int, str]]] = {}
            for it in ep_links:
                by_season.setdefault(it[1], []).append(it)
            template_pat = re.compile(
                r"^(?P<prefix>https?://[^/]+/serie/\d+-[a-z0-9\-]*?)"
                r"(?:-(?P<season>\d+)-fasl)?-(?P<ep>\d+)-qism(?P<suffix>[a-z0-9\-/.]*)$",
                re.IGNORECASE,
            )
            new_items: List[tuple[str, int, int, str]] = []
            for season, items in by_season.items():
                eps = sorted({i[2] for i in items})
                lo, hi = eps[0], eps[-1]
                template_url = items[0][0]
                m = template_pat.match(template_url)
                if not m:
                    continue
                prefix = m.group("prefix")
                suffix = m.group("suffix") or ""
                season_seg = f"-{season}-fasl" if m.group("season") else ""
                for n in range(lo, hi + 1):
                    if n in eps:
                        continue
                    href = f"{prefix}{season_seg}-{n}-qism{suffix}"
                    new_items.append((href, season, n, f"{n}-qism"))
                    pattern_added += 1
            ep_links.extend(new_items)
            ep_links.sort(key=lambda x: (x[1], x[2]))

        logger.info(
            f"[episode extractor] source=freekino found_visible={source_counts.get('visible', 0)} "
            f"found_script={source_counts.get('script', 0)} found_pattern={pattern_added} "
            f"final={len(ep_links)}"
        )

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
                entries, quality_urls = self._movie._extract_video(ep_soup, ep_resp.url)
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
                quality_urls = {}
                logger.warning(
                    f"[FREEKINO SERIAL] S{season:02d}E{episode:02d} failed href={href} err={e}"
                )

            episodes.append(
                {
                    "season": season,
                    "episode": episode,
                    "season_number": season,
                    "episode_number": episode,
                    "title": ep_title,
                    "episode_url": href,
                    "detail_url": href,
                    "source_episode_url": href,
                    "source_id": canonical_episode_id(parent_id, season, episode),
                    "video_url": video_url,
                    "quality_urls": quality_urls or {},
                    "poster": poster,
                    **({"error": "video_url not extracted for episode"} if not video_url else {}),
                }
            )

        episodes.sort(key=lambda item: (item["season"], item["episode"], item["video_url"] or item["episode_url"]))
        seasons_index: Dict[int, List[Dict]] = {}
        for item in episodes:
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
        missing_numbers = self._compute_missing_numbers(episodes)
        logger.info(
            f"[serial-details] source=freekino title={title!r} seasons={len(seasons)} "
            f"episodes_found={len(episodes)} missing_numbers={missing_numbers} duplicates_removed={duplicates_removed}"
        )
        warnings: List[str] = []
        if missing_numbers:
            warnings.append("Possible missing episodes: " + ",".join(str(n) for n in missing_numbers))
            logger.warning(f"[episode extractor] missing numbers={missing_numbers}")

        resolved = sum(1 for ep in episodes if ep["video_url"])
        logger.info(
            f"[FREEKINO SERIAL] done title={title!r} resolved={resolved}/{len(episodes)} failures={failures}"
        )

        return {
            "success": len(episodes) > 0,
            "type": "serial",
            "provider": "freekino",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
            "warnings": warnings,
            "missing_numbers": missing_numbers,
        }

    def _collect_episode_links(self, soup: BeautifulSoup, html: str) -> tuple[List[tuple[str, int, int, str]], int, Dict[str, int]]:
        by_key: Dict[tuple[int, int], tuple[str, int, int, str]] = {}
        duplicates_removed = 0
        counts = {"visible": 0, "script": 0}

        def add_candidate(raw_href: str, label: str = "", bucket: str = "visible") -> None:
            nonlocal duplicates_removed
            href = normalize_url(self._movie.BASE_URL, raw_href)
            parsed = _parse_season_episode(href)
            if not parsed:
                return
            season, episode = parsed
            key = (season, episode)
            candidate = (href, season, episode, clean_text(label))
            if key in by_key:
                existing = by_key[key]
                duplicates_removed += 1
                if len(candidate[3]) > len(existing[3]) or (not existing[3] and candidate[3]):
                    by_key[key] = candidate
                return
            by_key[key] = candidate
            counts[bucket] = counts.get(bucket, 0) + 1

        for a in soup.find_all("a", href=True):
            href = a["href"]
            if "/serie/" in href:
                add_candidate(href, a.get_text(" ", strip=True), "visible")

        for el in soup.find_all(True):
            label = el.get_text(" ", strip=True)
            for attr in ("data-href", "data-url", "data-link", "data-episode-url", "href"):
                href = el.get(attr) if hasattr(el, "get") else None
                if href and "/serie/" in str(href):
                    add_candidate(str(href), label, "visible")

        for m in _EP_URL_ANY_RE.finditer(html or ""):
            add_candidate(m.group(0), "", "script")

        items = sorted(by_key.values(), key=lambda x: (x[1], x[2], x[0]))
        return items, duplicates_removed, counts

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
