"""
Seezn TV serial parser.

A serial on seezntv.uz is modelled as: a *franchise* that groups one or more
*seasons*, where each season is its own content record (own hashid) carrying an
``episodes[]`` list. Each episode has a stable public HLS ``media_url`` on
``cdn.seezntv.uz`` — no signed/expiring URLs — so we can embed the stream URL
directly at extraction time.

``parse(url)`` accepts any season's web/API URL, discovers the whole franchise
(all seasons), and returns the flattened episode list plus a per-season index,
matching the dict contract the other *_serial parsers emit.
"""
import logging
import re
from typing import Dict, List, Any

from seezntv import SeezntvParser
from helpers import canonical_episode_id

logger = logging.getLogger(__name__)


class SeezntvSerialParser:
    def __init__(self) -> None:
        self._movie = SeezntvParser()

    @property
    def session(self):
        return self._movie.session

    def _fetch(self, content_id: str) -> Dict[str, Any]:
        return self._movie.fetch_content(content_id)

    def parse(self, url: str) -> Dict[str, Any]:
        logger.info(f"[SEEZNTV SERIAL] parse start url={url}")
        content_id = self._movie.content_id_from(url)
        if not content_id:
            return {"success": False, "provider": "seezntv", "error": f"cannot parse content id from {url}"}

        try:
            root = self._fetch(content_id)
        except Exception as e:
            logger.error(f"[SEEZNTV SERIAL] fetch failed for {content_id}: {e}")
            return {"success": False, "provider": "seezntv", "error": f"fetch failed: {e}"}

        franchise = root.get("franchise") or {}
        franchise_items = franchise.get("items") or []

        # Build the ordered (season_number, season_content_id) list.
        # With a franchise, every season is enumerated; without one this content
        # is a stand-alone single season.
        if franchise_items:
            series_title = (franchise.get("name_uz") or franchise.get("name_ru")
                            or franchise.get("name_en") or "").strip() \
                or self._strip_season_suffix(self._movie.localized_name(root))
            parent_id = franchise.get("id") or content_id
            seasons_spec = []
            for it in franchise_items:
                cid = it.get("content_id")
                if not cid:
                    continue
                try:
                    order = int(it.get("order") or (len(seasons_spec) + 1))
                except (TypeError, ValueError):
                    order = len(seasons_spec) + 1
                seasons_spec.append((order, cid))
            seasons_spec.sort(key=lambda x: x[0])
        else:
            series_title = self._strip_season_suffix(self._movie.localized_name(root))
            parent_id = content_id
            seasons_spec = [(1, content_id)]

        # Series-level artwork/description taken from the imported content.
        poster = self._movie.poster_from(root)
        backdrop = self._movie.backdrop_from(root)
        description = self._movie.localized_description(root)
        year = self._movie.year_from(root)

        episodes: List[Dict[str, Any]] = []
        warnings: List[str] = []

        for season_no, season_cid in seasons_spec:
            season_rec = root if season_cid == content_id else self._safe_fetch(season_cid, warnings)
            if season_rec is None:
                continue
            season_web_url = self._movie.web_url(season_cid)
            raw_eps = season_rec.get("episodes") or []
            # Sort by declared order so episode numbering is deterministic.
            raw_eps = sorted(raw_eps, key=lambda e: self._safe_int(e.get("order"), 0))
            for idx, ep in enumerate(raw_eps, start=1):
                episode_no = self._safe_int(ep.get("order"), idx)
                media_url = (ep.get("media_url") or "").strip()
                if not media_url:
                    warnings.append(f"S{season_no:02d}E{episode_no:02d}: empty media_url")
                    continue
                ep_title = (ep.get("name_uz") or ep.get("name_ru") or ep.get("name_en")
                            or f"{episode_no}-qism").strip()
                episodes.append({
                    "season": season_no,
                    "episode": episode_no,
                    "season_number": season_no,
                    "episode_number": episode_no,
                    "title": ep_title,
                    "episode_url": season_web_url,
                    "detail_url": season_web_url,
                    "source_episode_url": season_web_url,
                    "source_id": canonical_episode_id(parent_id, season_no, episode_no),
                    "video_url": media_url,
                    "poster": self._movie.poster_from(season_rec) or poster,
                    "intro_start": ep.get("intro_start"),
                    "intro_end": ep.get("intro_end"),
                    "ending_start": ep.get("ending_start"),
                    "ending_end": ep.get("ending_end"),
                    "quality_urls": {},
                })

        episodes.sort(key=lambda x: (x["season"], x["episode"]))

        seasons_index: Dict[int, List[Dict]] = {}
        for item in episodes:
            seasons_index.setdefault(item["season"], []).append(item)
        seasons = [
            {"season_number": s, "episodes": seasons_index[s]}
            for s in sorted(seasons_index.keys())
        ]

        logger.info(
            f"[SEEZNTV SERIAL] '{series_title}' -> {len(seasons)} season(s), "
            f"{len(episodes)} episode(s)"
        )
        return {
            "success": len(episodes) > 0,
            "type": "serial",
            "provider": "seezntv",
            "title": series_title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
            "warnings": warnings,
            "missing_numbers": [],
        }

    # ── helpers ─────────────────────────────────────────────────────────
    def _safe_fetch(self, content_id: str, warnings: List[str]):
        try:
            return self._fetch(content_id)
        except Exception as e:
            logger.warning(f"[SEEZNTV SERIAL] season fetch failed {content_id}: {e}")
            warnings.append(f"season {content_id}: fetch failed ({e})")
            return None

    @staticmethod
    def _safe_int(v, default: int) -> int:
        try:
            return int(v)
        except (TypeError, ValueError):
            return default

    @staticmethod
    def _strip_season_suffix(name: str) -> str:
        """'Yigitlar — 1-fasl' -> 'Yigitlar'."""
        name = (name or "").strip()
        # Trim a trailing "— N-fasl" / "- Season N" / "— N сезон" suffix.
        name = re.split(r"\s*[—–-]\s*\d+\s*[- ]?\s*(fasl|сезон)\b", name, flags=re.IGNORECASE)[0]
        name = re.sub(r"\s*[—–-]\s*season\s*\d+\s*$", "", name, flags=re.IGNORECASE)
        return name.strip(" —–-") or name
