"""
Seezn TV (seezntv.uz) Parser

Unlike the other sources (uCoz / DLE sites that server-render HTML), seezntv.uz
is a Next.js SPA backed by a public JSON REST API at ``v2.seezntv.uz``. There is
nothing to scrape from the HTML — every field (metadata + the HLS stream URL)
comes from the API, and the streams live on an open Cloudflare CDN
(``cdn.seezntv.uz``) with no token / DRM.

API shape (all anonymous, no auth):
  - GET /api/contents/?search=<q>&limit=30   → paginated list
  - GET /api/contents/?limit=30&ordering=-created_at
  - GET /api/contents/<hashid>/              → single content (movie season)

A content record carries ``episodes[]`` where each episode has a ``media_url``
(HLS ``.m3u8``). A movie is a content with exactly one episode ("Film"); a serial
season is a content with many episodes, and ``franchise.items[]`` links the
seasons of the same show together (see seezntv_serial.py).
"""
import logging
import os
import re
from typing import List, Dict, Any, Optional

from base_parser import BaseParser, SearchResult, MovieDetails

logger = logging.getLogger(__name__)
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


class SeezntvParser(BaseParser):
    """Movie parser for seezntv.uz (JSON API client)."""

    WEB_BASE = "https://seezntv.uz"
    API_BASE = "https://v2.seezntv.uz/api"

    @property
    def source_name(self) -> str:
        return "seezntv"

    @property
    def base_url(self) -> str:
        return self.WEB_BASE

    def __init__(self):
        super().__init__()
        # The API is CORS-scoped to the web origin; send matching Origin/Referer
        # so requests look like the real front-end and never trip a referer check.
        self._default_headers.update({
            "Accept": "application/json, text/plain, */*",
            "Origin": self.WEB_BASE,
            "Referer": self.WEB_BASE + "/",
        })

    # ── helpers ─────────────────────────────────────────────────────────
    @staticmethod
    def content_id_from(url_or_id: str) -> str:
        """Extract the content hashid from a web URL, API URL, or raw id.

        Accepts: ``AbC123``, ``https://seezntv.uz/contents/AbC123``,
        ``https://seezntv.uz/en/contents/AbC123``,
        ``https://v2.seezntv.uz/api/contents/AbC123/``.
        """
        s = (url_or_id or "").strip()
        if not s:
            return ""
        m = re.search(r"/contents/([A-Za-z0-9]+)", s)
        if m:
            return m.group(1)
        # Raw id (no slashes) — take as-is.
        if "/" not in s:
            return s.split("?")[0].split("#")[0]
        return ""

    def web_url(self, content_id: str) -> str:
        return f"{self.WEB_BASE}/contents/{content_id}"

    def _api_get(self, path: str, params: Optional[Dict[str, Any]] = None) -> Any:
        """GET a JSON endpoint. ``path`` may be absolute or API-relative."""
        url = path if path.startswith("http") else f"{self.API_BASE}{path}"
        resp = self.session.get(url, params=params, timeout=30)
        resp.raise_for_status()
        return resp.json()

    def fetch_content(self, content_id: str) -> Dict[str, Any]:
        """Fetch a single content record by hashid."""
        return self._api_get(f"/contents/{content_id}/")

    # ── metadata mapping ────────────────────────────────────────────────
    @staticmethod
    def localized_name(rec: Dict[str, Any]) -> str:
        return (rec.get("name_uz") or rec.get("name_ru") or rec.get("name_en") or "").strip()

    @staticmethod
    def localized_description(rec: Dict[str, Any]) -> str:
        return (rec.get("description_uz") or rec.get("description_ru")
                or rec.get("description_en") or "").strip()

    @staticmethod
    def year_from(rec: Dict[str, Any]) -> int:
        rd = (rec.get("release_date") or "").strip()
        m = re.match(r"(\d{4})", rd)
        return int(m.group(1)) if m else 0

    @staticmethod
    def _category_type_name(cat: Dict[str, Any]) -> str:
        ct = cat.get("category_type") or {}
        return " ".join(str(ct.get(k) or "") for k in ("name_uz", "name_ru", "name_en")).lower()

    @classmethod
    def genres_from(cls, rec: Dict[str, Any]) -> List[str]:
        """Genres = categories whose category_type is 'Janrlar' / 'Genres'."""
        out: List[str] = []
        for cat in rec.get("categories") or []:
            type_name = cls._category_type_name(cat)
            if "janr" in type_name or "genre" in type_name or "жанр" in type_name:
                name = (cat.get("name_uz") or cat.get("name_en") or cat.get("name_ru") or "").strip()
                if name and name not in out:
                    out.append(name)
        return out

    @classmethod
    def content_type_from(cls, rec: Dict[str, Any]) -> str:
        """Return 'serial' or 'movie'.

        A '... — N-fasl' / '... Season N' title is an unambiguous serial-season
        marker and wins outright (seezntv sometimes mis-tags a season's 'Tarkib'
        Type category as 'Kino' — e.g. The Boys S1). Otherwise fall back to the
        'Tarkib' Type category (Kino / Multifilm / Serial)."""
        name = cls.localized_name(rec).lower()
        if re.search(r"-\s*fasl\b", name) or re.search(r"\bseason\b", name) or "сезон" in name:
            return "serial"
        for cat in rec.get("categories") or []:
            type_name = cls._category_type_name(cat)
            if "tarkib" in type_name or "type" in type_name or "тип" in type_name or "контент" in type_name:
                val = (cat.get("name_uz") or cat.get("name_en") or cat.get("name_ru") or "").lower()
                if "serial" in val or "сериал" in val or "series" in val:
                    return "serial"
                return "movie"
        return "movie"

    @classmethod
    def poster_from(cls, rec: Dict[str, Any]) -> str:
        return (rec.get("card_banner") or rec.get("mobile_banner")
                or rec.get("desktop_banner") or "").strip()

    @classmethod
    def backdrop_from(cls, rec: Dict[str, Any]) -> str:
        return (rec.get("desktop_banner") or rec.get("mobile_banner") or "").strip()

    # ── BaseParser interface ────────────────────────────────────────────
    def search(self, query: str) -> List[SearchResult]:
        results: List[SearchResult] = []
        try:
            data = self._api_get("/contents/", params={"search": query, "limit": 30})
            items = data.get("results", data) if isinstance(data, dict) else (data or [])
            for rec in items:
                cid = rec.get("id")
                if not cid:
                    continue
                results.append(SearchResult(
                    title=self.localized_name(rec),
                    year=self.year_from(rec),
                    poster=self.poster_from(rec),
                    description=self.localized_description(rec),
                    source_id=cid,
                    detail_url=self.web_url(cid),
                    source=self.source_name,
                    content_type=self.content_type_from(rec),
                ))
            logger.info(f"[SEEZNTV] search '{query}' -> {len(results)} results")
        except Exception as e:
            logger.error(f"[SEEZNTV] search error: {e}", exc_info=DEBUG)
        return results

    def _episode_media_url(self, rec: Dict[str, Any], episode_number: int) -> str:
        """Pick the media_url of the episode whose ``order`` matches
        ``episode_number`` (falls back to positional index)."""
        episodes = rec.get("episodes") or []
        for ep in episodes:
            try:
                if int(ep.get("order") or 0) == episode_number:
                    return (ep.get("media_url") or "").strip()
            except (TypeError, ValueError):
                continue
        if 1 <= episode_number <= len(episodes):
            return (episodes[episode_number - 1].get("media_url") or "").strip()
        return ""

    def get_details(self, url: str, source_id: str = "", is_serial: bool = False,
                    episode_id: str = "") -> MovieDetails:
        """Resolve a content's metadata + HLS video URL.

        Serial-episode jobs arrive with source_id ``<parent>:sNNeMM`` and a
        detail ``url`` pointing at that season's content page; we return the
        matching episode's stream. Movie jobs return episode #1 ("Film").
        """
        content_id = self.content_id_from(url) or self.content_id_from(source_id)
        if not content_id:
            logger.error(f"[SEEZNTV] cannot resolve content id from url={url!r} source_id={source_id!r}")
            return self._empty_details(source_id)

        try:
            rec = self.fetch_content(content_id)
        except Exception as e:
            logger.error(f"[SEEZNTV] get_details fetch error for {content_id}: {e}", exc_info=DEBUG)
            return self._empty_details(source_id)

        # Which episode? Serial episode marker wins; otherwise it's a movie (ep 1).
        ep_match = re.search(r":s(\d+)e(\d+)$", source_id or "")
        episode_number = int(ep_match.group(2)) if ep_match else 1
        media_url = self._episode_media_url(rec, episode_number)

        video_urls = []
        if media_url:
            url_type = "m3u8" if ".m3u8" in media_url.lower() else "mp4"
            video_urls.append({"url": media_url, "type": url_type, "quality": "auto"})
        else:
            logger.warning(f"[SEEZNTV] no media_url for {content_id} episode #{episode_number}")

        ctype = "serial" if ep_match else self.content_type_from(rec)
        return MovieDetails(
            title=self.localized_name(rec),
            description=self.localized_description(rec),
            poster=self.poster_from(rec),
            backdrop=self.backdrop_from(rec),
            year=self.year_from(rec),
            genres=self.genres_from(rec),
            country="",
            duration=0,
            video_page_url=self.web_url(content_id),
            video_urls=video_urls,
            source=self.source_name,
            source_id=source_id or content_id,
            type=ctype,
            detail_url=self.web_url(content_id),
        )

    # ── catalog browsing (admin UI) ─────────────────────────────────────
    def list_categories(self) -> List[Dict[str, Any]]:
        """Return browsable categories (genres + sections). ``url``/``slug`` carry
        the category hashid, which list_catalog() accepts as ``category_url``."""
        out: List[Dict[str, Any]] = []
        try:
            data = self._api_get("/categories/")
            items = data.get("results", data) if isinstance(data, dict) else (data or [])
            seen = set()
            for c in items:
                cid = c.get("id")
                name = (c.get("name_uz") or c.get("name_en") or c.get("name_ru") or "").strip()
                if not cid or not name or cid in seen:
                    continue
                seen.add(cid)
                out.append({"name": name, "url": cid, "slug": cid})
        except Exception as e:
            logger.warning(f"[SEEZNTV] list_categories error: {e}")
        return out

    def list_catalog(self, page: int = 1, limit: int = 20, type_filter: str = "",
                     category_url: str = "") -> Dict[str, Any]:
        """Paginated catalog browse (newest first), optional genre/section filter
        (``category_url`` = category hashid) and movie/serial ``type_filter``."""
        items: List[Dict[str, Any]] = []
        total = 0
        try:
            params: Dict[str, Any] = {
                "limit": limit,
                "offset": max(0, (page - 1) * limit),
                "ordering": "-created_at",
            }
            if category_url:
                params["category"] = category_url
            data = self._api_get("/contents/", params=params)
            total = int(data.get("count") or 0) if isinstance(data, dict) else 0
            records = data.get("results", data) if isinstance(data, dict) else (data or [])
            for rec in records:
                cid = rec.get("id")
                if not cid:
                    continue
                ctype = self.content_type_from(rec)
                if type_filter in ("movie", "serial") and ctype != type_filter:
                    continue
                items.append(SearchResult(
                    title=self.localized_name(rec),
                    year=self.year_from(rec),
                    poster=self.poster_from(rec),
                    description=self.localized_description(rec),
                    source_id=cid,
                    detail_url=self.web_url(cid),
                    source=self.source_name,
                    content_type=ctype,
                ).to_dict())
        except Exception as e:
            logger.error(f"[SEEZNTV] list_catalog error: {e}", exc_info=DEBUG)

        total_pages = (total + limit - 1) // limit if total and limit else page
        return {
            "items": items,
            "page": page,
            "limit": limit,
            "total": total,
            "total_pages": total_pages,
            "has_more": page < total_pages,
        }

    def _empty_details(self, source_id: str) -> MovieDetails:
        return MovieDetails(
            title="", description="", poster="", backdrop="", year=0,
            genres=[], country="", duration=0, source_id=source_id, source=self.source_name,
        )
