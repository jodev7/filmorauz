import logging
import re
from typing import Dict, List

from bs4 import BeautifulSoup

from kinolar import KinolarParser
from helpers import canonical_episode_id, extract_source_id, clean_text

logger = logging.getLogger(__name__)


class KinolarSerialParser:
    def __init__(self) -> None:
        self._movie = KinolarParser()

    @property
    def session(self):
        return self._movie.session

    def parse(self, url: str) -> Dict:
        logger.info(f"[KINOLAR SERIAL] parse start url={url}")
        resp = self.session.get(url, timeout=30)
        resp.raise_for_status()
        html_text = resp.text
        soup = BeautifulSoup(html_text, "lxml")

        # kinolar.tv injects the episode <a> list (with faylmovi.ru direct
        # .mp4 hrefs) via JS after page load, mirroring uzmovi's rebuild.
        # Probe the static DOM first; if it carries no qism/часть anchors
        # AND no _qism.mp4 in #dispnone, swap in a headless-rendered copy.
        def _has_episode_signal(s: BeautifulSoup, raw: str) -> bool:
            for a in s.find_all("a", href=True):
                t = (a.get_text(" ", strip=True) or "").lower()
                if "qism" in t or "часть" in t or "episode" in t:
                    return True
            return "_qism.mp4" in raw.lower()

        if not _has_episode_signal(soup, html_text):
            rendered = self._render_with_playwright(url)
            if rendered:
                html_text = rendered
                soup = BeautifulSoup(html_text, "lxml")
                logger.info(f"[KINOLAR SERIAL] playwright fallback DOM len={len(html_text)}")

        title = ""
        title_el = soup.select_one("h1, .title, .film-title")
        if title_el:
            title = clean_text(title_el.get_text())

        poster = ""
        og = soup.select_one("meta[property='og:image']")
        if og:
            poster = og.get("content", "")
            if poster and not poster.startswith("http"):
                poster = self._movie.BASE_URL + poster

        backdrop = ""
        tw = soup.select_one("meta[name='twitter:image']")
        if tw:
            backdrop = tw.get("content", "")
            if backdrop and not backdrop.startswith("http"):
                backdrop = self._movie.BASE_URL + backdrop

        description = ""
        ogd = soup.select_one("meta[property='og:description']")
        if ogd:
            description = clean_text(ogd.get("content", ""))
        
        year = 0
        m_year = re.search(r"(19|20)\d{2}", title) or re.search(r"(19|20)\d{2}", html_text[:6000])
        if m_year:
            year = int(m_year.group(0))

        parent_id = extract_source_id(url)
        grouped: Dict[tuple[int, int], Dict] = {}

        # JS-rendered anchor text on kinolar.tv now reads "1-qism | Часть 1"
        # (and similar dual-locale labels), so the old `^...$` anchored regex
        # missed every episode. Use .search() against a relaxed pattern.
        episode_re = re.compile(r"\b(\d+)\s*-\s*qism\b", re.IGNORECASE)
        season_ep_re = re.compile(r"\b(?:(\d+)\s*-\s*fasl\s+)?(\d+)\s*-\s*qism\b", re.IGNORECASE)
        seen_hrefs = set()
        ep_links = []
        
        for a in soup.find_all("a", href=True):
            href = a["href"].strip()
            if not href or href in seen_hrefs or href.startswith("#") or href.startswith("javascript:"):
                continue
            
            label = clean_text(a.get_text(" ", strip=True) or a.get("title", ""))
            m = season_ep_re.search(label) or episode_re.search(label)
            if m:
                seen_hrefs.add(href)
                season = int(m.group(1)) if len(m.groups()) > 1 and m.group(1) else 1
                episode_no = int(m.groups()[-1])
                full_href = href if href.startswith("http") else f"{self._movie.BASE_URL.rstrip('/')}/{href.lstrip('/')}"
                ep_links.append((full_href, season, episode_no, label))

        for full_href, season, episode_no, label in ep_links:
            key = (season, episode_no)
            if key in grouped:
                continue
            # JS-rendered anchors on kinolar.tv now point straight at a
            # direct .mp4 on faylmovi.ru (no per-episode HTML wrapper).
            # When the href IS the playable URL, skip the subpage fetch —
            # otherwise self.session.get would download the entire video.
            direct_lower = full_href.lower()
            if direct_lower.endswith((".mp4", ".m3u8", ".mpd", ".ism")):
                grouped[key] = {
                    "season": season,
                    "episode": episode_no,
                    "season_number": season,
                    "episode_number": episode_no,
                    "title": f"{episode_no}-qism",
                    "episode_url": full_href,
                    "detail_url": url,
                    "source_episode_url": full_href,
                    "source_id": canonical_episode_id(parent_id, season, episode_no),
                    "video_url": full_href,
                    "poster": poster,
                    "quality_urls": {},
                }
                continue
            try:
                logger.info(f"[KINOLAR SERIAL] fetching subpage S{season}E{episode_no} url={full_href}")
                ep_resp = self.session.get(full_href, timeout=15)
                ep_soup = BeautifulSoup(ep_resp.text, "lxml")
                entries = self._movie._extract_video_urls(ep_soup, full_href)
                video_url = entries[0]["url"] if entries else ""

                grouped[key] = {
                    "season": season,
                    "episode": episode_no,
                    "season_number": season,
                    "episode_number": episode_no,
                    "title": f"{episode_no}-qism",
                    "episode_url": full_href,
                    "detail_url": full_href,
                    "source_episode_url": full_href,
                    "source_id": canonical_episode_id(parent_id, season, episode_no),
                    "video_url": video_url,
                    "poster": poster,
                    "quality_urls": {},
                }
            except Exception as e:
                logger.warning(f"[KINOLAR SERIAL] failed to fetch subpage S{season}E{episode_no}: {e}")

        # Fallback: many kinolar.tv serial pages do not link out to per-episode
        # subpages. Instead, the page embeds a hidden <p id="dispnone"> block
        # with one row per episode: "<mp4-url> | duration | resolution | size".
        # When the <a>-based scan returns nothing, parse that block directly.
        if not grouped:
            direct_url_re = re.compile(
                r"https?://[^\s\"'<>]+?_(\d+)_qism\.mp4", re.IGNORECASE
            )
            dispnone = soup.select_one("#dispnone")
            search_text = dispnone.get_text("\n", strip=True) if dispnone else html_text
            seen_urls = set()
            for match in direct_url_re.finditer(search_text):
                video_url = match.group(0)
                if video_url in seen_urls:
                    continue
                seen_urls.add(video_url)
                episode_no = int(match.group(1))
                season = 1
                key = (season, episode_no)
                if key in grouped:
                    continue
                grouped[key] = {
                    "season": season,
                    "episode": episode_no,
                    "season_number": season,
                    "episode_number": episode_no,
                    "title": f"{episode_no}-qism",
                    "episode_url": url,
                    "detail_url": url,
                    "source_episode_url": url,
                    "source_id": canonical_episode_id(parent_id, season, episode_no),
                    "video_url": video_url,
                    "poster": poster,
                    "quality_urls": {},
                }
            logger.info(
                f"[KINOLAR SERIAL] direct-mp4 fallback found {len(grouped)} episodes"
            )

        episodes = list(grouped.values())
        episodes.sort(key=lambda x: (x["season"], x["episode"]))

        seasons_index: Dict[int, List[Dict]] = {}
        for item in episodes:
            seasons_index.setdefault(item["season"], []).append(item)
            
        seasons = [
            {"season_number": season_no, "episodes": seasons_index[season_no]}
            for season_no in sorted(seasons_index.keys())
        ]

        return {
            "success": len(episodes) > 0,
            "type": "serial",
            "provider": "kinolar",
            "title": title,
            "year": year,
            "poster": poster,
            "backdrop": backdrop,
            "description": description,
            "episodes": episodes,
            "seasons": seasons,
        }
    def _render_with_playwright(self, url: str) -> str:
        """Headless-render the serial page; kinolar.tv injects the per-episode
        <a> list and direct faylmovi.ru .mp4 URLs via JS, so a plain
        requests.get can never see them.
        """
        try:
            from playwright.sync_api import sync_playwright
        except ImportError:
            logger.warning("[KINOLAR SERIAL] playwright not installed; skipping JS render fallback")
            return ""
        try:
            with sync_playwright() as pw:
                browser = pw.chromium.launch(headless=True, args=["--no-sandbox"])
                ctx = browser.new_context(
                    user_agent=(
                        "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                        "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
                    ),
                )
                page = ctx.new_page()
                page.goto(url, wait_until="networkidle", timeout=45000)
                page.wait_for_timeout(6000)
                html = page.content()
                browser.close()
                return html
        except Exception as e:
            logger.error(f"[KINOLAR SERIAL] playwright render failed: {e}")
            return ""
