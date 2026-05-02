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
# Episode-button text patterns used by uzmovi's "Qismlardan tanlash" grid.
# We use these to count the maximum episode number the page advertises so we
# can detect under-extraction (e.g. parser saw 77 but page lists 90 buttons).
_EP_LABEL_RE = re.compile(r"(\d{1,3})\s*-\s*qism", re.IGNORECASE)
_EP_DATA_ATTR_RE = re.compile(r'\bdata-(?:episode|qism|number)\s*=\s*["\']?(\d{1,3})', re.IGNORECASE)


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

        # Collect every /episode/<group>/<n>.html link on the page. Uzmovi
        # frequently splits a long-running series across multiple translation
        # groups (e.g. group A covers 1–77, group B covers 78–90); the old
        # "first group wins" rule silently dropped everything from later
        # groups, which is exactly the Dexter 90→77 symptom.
        #
        # New strategy: merge by episode_number across all groups. Per
        # episode_number, prefer the group_id that has the highest episode
        # coverage so we keep the canonical run intact while still picking
        # up tail episodes from a secondary group.
        per_group: Dict[str, Dict[int, Dict]] = {}

        for a in soup.find_all("a", href=True):
            href = a["href"]
            m = _EP_HREF_RE.search(href)
            if not m:
                continue
            group_id, ep_no = m.group(1), int(m.group(2))
            full = href if href.startswith("http") else f"{self.base_url.rstrip('/')}{href}"
            group_map = per_group.setdefault(group_id, {})
            if ep_no in group_map:
                continue  # duplicate within the same group — drop, but
                # NEVER drop just because another group also has this number.
            group_map[ep_no] = {
                "episode": ep_no,
                "season": 1,
                "title": a.get_text(strip=True) or f"{ep_no}-qism",
                "episode_url": full,
                "_group_id": group_id,
            }

        # Sort groups by coverage size desc, then merge: the largest group
        # provides the baseline; smaller groups fill any gaps it has.
        sorted_groups = sorted(per_group.items(), key=lambda kv: len(kv[1]), reverse=True)
        per_episode: Dict[int, Dict] = {}
        for gid, gmap in sorted_groups:
            added = 0
            for ep_no, entry in gmap.items():
                if ep_no in per_episode:
                    continue
                per_episode[ep_no] = entry
                added += 1
            logger.info(
                f"[UZMOVI SERIAL] translation group_id={gid} contributes={added} total_in_group={len(gmap)}"
            )

        # Sanity-check against episode-button labels on the page. If the page
        # advertises "90-qism" anywhere but we only found 77 hrefs, something
        # is hiding behind JS / a collapsed tab — fall back to a raw-HTML
        # scan to surface those numbers (without a URL we can't fetch them,
        # but logging it loud beats silently shipping a 77-episode import).
        max_label_no = 0
        try:
            for el in soup.find_all(True):
                t = el.get_text(" ", strip=True)
                for mm in _EP_LABEL_RE.finditer(t or ""):
                    n = int(mm.group(1))
                    if 0 < n < 1000 and n > max_label_no:
                        max_label_no = n
                for attr in ("data-episode", "data-qism", "data-number", "data-id"):
                    val = el.get(attr) if hasattr(el, "get") else None
                    if val and str(val).isdigit():
                        n = int(val)
                        if 0 < n < 1000 and n > max_label_no:
                            max_label_no = n
        except Exception:
            pass

        if not per_episode and max_label_no == 0:
            # Last-ditch: regex over the raw HTML for "N-qism" labels that
            # weren't inside <a> elements (collapsed accordions, JS-rendered
            # buttons serialised in inline scripts, etc.).
            for mm in _EP_LABEL_RE.finditer(resp.text or ""):
                n = int(mm.group(1))
                if 0 < n < 1000 and n > max_label_no:
                    max_label_no = n

        ep_nos = sorted(per_episode.keys())
        max_parsed_no = ep_nos[-1] if ep_nos else 0
        logger.info(
            f"[UZMOVI SERIAL] title={title!r} episodes_found={len(ep_nos)} "
            f"max_parsed={max_parsed_no} max_label_seen={max_label_no} groups={len(per_group)}"
        )

        if max_label_no and (len(ep_nos) < max_label_no or max_parsed_no < max_label_no):
            logger.warning(
                f"[UZMOVI SERIAL] under-extraction detected: page advertises up to "
                f"{max_label_no}-qism but parser found {len(ep_nos)} episodes "
                f"(max number {max_parsed_no}); check translation groups / hidden tabs"
            )

        if not ep_nos:
            return {
                "success": False,
                "error": "no episodes found on serial page",
                "provider": "uzmovi",
                "max_label_no": max_label_no,
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
            entry.pop("_group_id", None)  # internal; don't ship to API consumers
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
