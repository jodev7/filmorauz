"""
Uzbeklar.biz parser — DataLife Engine site.

Layout summary (see investigation 2026-05-29):
- POST https://uzbeklar.biz/  body: do=search&subaction=search&story=<query>
  returns the homepage HTML with result cards in `<a class="short-title">`.
- Detail URL: https://uzbeklar.biz/<id>-<slug>.html (no category prefix).
- Detail page embeds the player config inline as:
    file:"https://a.uzbeklar.biz/film/<name>.mp4"
  A single direct MP4 URL per movie (no quality selector on this site).
- Inline labels under `<li>` rows: Yili, Janr, Davomiyligi, Sifat, Davlat.
"""

from __future__ import annotations

import logging
import os
import re
from typing import List, Dict, Any, Optional

from bs4 import BeautifulSoup

from base_parser import BaseParser, SearchResult, MovieDetails
from helpers import normalize_url, clean_text, extract_year

logger = logging.getLogger(__name__)
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"


_FILE_RE = re.compile(r"""file\s*:\s*["'](?P<url>https?://[^"']+\.(?:mp4|m3u8|mpd))["']""", re.IGNORECASE)
_DETAIL_URL_RE = re.compile(r"^https?://(?:www\.)?uzbeklar\.biz/(\d+)-[^/]+\.html$", re.IGNORECASE)


class UzbeklarParser(BaseParser):
    """Parser for uzbeklar.biz (DataLife Engine)."""

    BASE_URL = "https://uzbeklar.biz"

    @property
    def source_name(self) -> str:
        return "uzbeklar"

    @property
    def base_url(self) -> str:
        return self.BASE_URL

    def __init__(self) -> None:
        super().__init__()
        self._default_headers.update({
            "Referer": self.BASE_URL + "/",
            "Origin": self.BASE_URL,
        })

    # ------------------------------------------------------------------
    # Search
    # ------------------------------------------------------------------
    def search(self, query: str) -> List[SearchResult]:
        q = (query or "").strip()
        if not q:
            return []
        payload = {
            "do": "search",
            "subaction": "search",
            "story": q,
        }
        url = self.BASE_URL + "/"
        logger.info(f"[UZBEKLAR] POST search story={q!r}")
        try:
            resp = self.session.post(url, data=payload, timeout=30, allow_redirects=True)
        except Exception as e:
            logger.warning(f"[UZBEKLAR] search POST failed: {e}")
            return []
        if resp.status_code != 200:
            logger.warning(f"[UZBEKLAR] search non-200 status={resp.status_code}")
            return []
        return self._parse_search_results(resp.text)

    def _parse_search_results(self, html: str) -> List[SearchResult]:
        soup = BeautifulSoup(html, "lxml")
        results: List[SearchResult] = []
        seen: set[str] = set()
        for a in soup.select("a.short-title"):
            href = a.get("href", "").strip()
            if not href:
                continue
            url = normalize_url(href, self.BASE_URL)
            m = _DETAIL_URL_RE.match(url)
            if not m:
                continue
            if url in seen:
                continue
            seen.add(url)
            title = clean_text(a.get_text())
            source_id = m.group(1)

            poster = ""
            # The card structure puts the <img> in a sibling anchor inside the same item.
            item = a.find_parent(class_=re.compile(r"short-item|short-cols|short-desc"))
            if not item:
                item = a.find_parent("div")
            if item:
                img = item.find("img")
                if img:
                    src = img.get("data-src") or img.get("data-original") or img.get("src", "")
                    if src:
                        poster = normalize_url(src, self.BASE_URL)

            content_type = "movie"  # Serial vs movie cannot be told from card alone;
            # the serial detector in server.py will reclassify based on detail page.

            results.append(SearchResult(
                title=title,
                year=0,
                poster=poster,
                description="",
                source_id=source_id,
                detail_url=url,
                source=self.source_name,
                content_type=content_type,
            ))
        logger.info(f"[UZBEKLAR] search results: {len(results)}")
        return results

    # ------------------------------------------------------------------
    # Details
    # ------------------------------------------------------------------
    def get_details(self, url: str, source_id: str = "", is_serial: bool = False, episode_id: str = "") -> MovieDetails:
        logger.info(f"[UZBEKLAR] get_details url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        html = resp.text
        soup = BeautifulSoup(html, "lxml")

        title = ""
        og_title = soup.select_one("meta[property='og:title']")
        if og_title:
            title = clean_text(og_title.get("content", ""))
        if not title:
            h = soup.select_one("h1, .full-title, .short-title")
            if h:
                title = clean_text(h.get_text())

        description = ""
        og_desc = soup.select_one("meta[property='og:description']") or soup.select_one("meta[name='description']")
        if og_desc:
            description = clean_text(og_desc.get("content", ""))
        if not description:
            desc_el = soup.select_one(".full-text, .full-story, .description")
            if desc_el:
                description = clean_text(desc_el.get_text())

        poster = ""
        og_img = soup.select_one("meta[property='og:image']")
        if og_img:
            poster = normalize_url(og_img.get("content", ""), self.BASE_URL)
        if not poster:
            img = soup.select_one(".full-poster img, .short-img img, img.xfieldimage")
            if img:
                src = img.get("data-src") or img.get("src", "")
                if src:
                    poster = normalize_url(src, self.BASE_URL)

        backdrop = ""
        tw_img = soup.select_one("meta[name='twitter:image']")
        if tw_img:
            backdrop = normalize_url(tw_img.get("content", ""), self.BASE_URL)

        # Inline label parsing: pages render `<li><span>Yili:</span> 2014</li>`.
        labels = self._extract_inline_labels(soup)
        year = extract_year(labels.get("yili", "")) or 0
        if not year:
            year = extract_year(title) or 0
        duration_minutes = self._parse_duration_minutes(labels.get("davomiyligi", ""))
        genres = self._split_genres(labels.get("janr", ""))
        country = clean_text(labels.get("davlat", ""))
        quality = clean_text(labels.get("sifat", ""))

        video_urls = self._extract_video_urls(html)
        if not video_urls:
            logger.warning(f"[UZBEKLAR] no file:\"...\" pattern matched on {url}")

        detected_id = ""
        m = _DETAIL_URL_RE.match(url)
        if m:
            detected_id = m.group(1)

        return MovieDetails(
            title=title,
            description=description,
            poster=poster,
            backdrop=backdrop or None,
            year=year,
            genres=genres,
            country=country,
            duration=duration_minutes,
            video_page_url=url,
            video_urls=video_urls,
            source=self.source_name,
            source_id=source_id or detected_id,
            type="movie",
            quality=quality,
            detail_url=url,
            player_url="",
        )

    # ------------------------------------------------------------------
    # Helpers
    # ------------------------------------------------------------------
    @staticmethod
    def _extract_inline_labels(soup: BeautifulSoup) -> Dict[str, str]:
        """Return a {lowercase_label: value} dict for rows like
        `<li><span>Yili:</span> 2014</li>` that appear in the sidebar."""
        out: Dict[str, str] = {}
        for li in soup.find_all(["li", "div", "p"]):
            text = li.get_text(" ", strip=True)
            if ":" not in text:
                continue
            label, _, value = text.partition(":")
            label_l = clean_text(label).lower()
            if not label_l or len(label_l) > 32:
                continue
            value_clean = clean_text(value)
            if not value_clean:
                continue
            if label_l in out:
                continue
            out[label_l] = value_clean
        return out

    @staticmethod
    def _split_genres(raw: str) -> List[str]:
        if not raw:
            return []
        parts = re.split(r"\s*[/,;|]\s*", raw)
        return [p for p in (clean_text(p) for p in parts) if p]

    @staticmethod
    def _parse_duration_minutes(raw: str) -> int:
        if not raw:
            return 0
        # Examples: "169 мин. / 02:49", "120 min", "02:30"
        m = re.search(r"(\d+)\s*(?:min|мин|мин\.|daqiqa)", raw, re.IGNORECASE)
        if m:
            return int(m.group(1))
        m = re.search(r"(\d+):(\d{2})", raw)
        if m:
            return int(m.group(1)) * 60 + int(m.group(2))
        m = re.search(r"\d+", raw)
        if m:
            return int(m.group(0))
        return 0

    @staticmethod
    def _extract_video_urls(html: str) -> List[Dict[str, Any]]:
        urls: List[Dict[str, Any]] = []
        seen: set[str] = set()
        for m in _FILE_RE.finditer(html):
            url = m.group("url")
            if url in seen:
                continue
            seen.add(url)
            lower = url.lower()
            if lower.endswith(".m3u8"):
                kind = "m3u8"
            elif lower.endswith(".mpd"):
                kind = "mpd"
            else:
                kind = "mp4"
            urls.append({
                "url": url,
                "type": kind,
                "quality": "1080p" if "1080" in lower else ("720p" if "720" in lower else "auto"),
                "label": "",
            })
        return urls
