"""
Auto-recovery primitives for the parser /download flow.

Three independent recovery layers that the server.py /download handler can
call when the happy path fails:

  1. classify_download_error(msg) -> stable category from telemetry constants
  2. find_iframe_candidates / try_playwright_extraction — last-ditch URL extraction
  3. try_alternate_sources — search another source for the same title

All helpers are best-effort: they never raise; they return empty / None and
log on failure so the calling code can fall through cleanly.
"""
from __future__ import annotations

import logging
import re
from typing import Dict, List, Optional, Tuple

import requests
from bs4 import BeautifulSoup

from telemetry import (
    ERR_BLOCKED,
    ERR_CDN_EXPIRED,
    ERR_DOWNLOAD_STALL,
    ERR_DOWNLOADER,
    ERR_HTML_RESPONSE,
    ERR_NETWORK,
    ERR_NO_VIDEO_URL,
    ERR_NOT_FOUND,
    ERR_UNKNOWN,
    ERR_VALIDATION,
)

logger = logging.getLogger(__name__)


# ---------------------------------------------------------------------------
# Error classification
# ---------------------------------------------------------------------------

# NOTE: patterns are matched against lower-cased error messages with re.search.
# Order matters only when categories overlap — first match wins inside each
# category check below.
_CDN_EXPIRED_PATTERNS = [
    r"\b403\b", r"\b401\b", r"forbidden", r"expired",
    r"signature", r"access denied", r"token.*invalid", r"url.*invalid",
]
_NOT_FOUND_PATTERNS = [r"\b404\b", r"not found"]
_BLOCKED_PATTERNS = [
    r"cloudflare", r"captcha", r"bot[\s_-]*protection", r"rate.?limit",
    r"too many requests", r"anti[- _]?bot",
]
_STALL_PATTERNS = [
    r"no progress", r"stall", r"killed for inactivity",
    r"inactivity timeout", r"stuck",
]
_NETWORK_PATTERNS = [
    r"connection refused", r"connection reset", r"timed out",
    r"dns", r"name or service not known", r"tlsv1", r"ssl",
    r"unreachable", r"network is unreachable",
]
_HTML_PATTERNS = [
    r"expected m3u8/mp4", r"html/page url", r"invalid media url",
    r"got html", r"html response",
]
_DOWNLOADER_PATTERNS = [r"ffmpeg", r"n_m3u8dl", r"aria2c", r"yt-dlp"]


def classify_download_error(error_msg: Optional[str]) -> str:
    """Map a free-form download error to a stable category."""
    if not error_msg:
        return ERR_UNKNOWN
    msg = error_msg.lower()
    # Order: most-specific-first (blocked before generic 403).
    if any(re.search(p, msg) for p in _BLOCKED_PATTERNS):
        return ERR_BLOCKED
    if any(re.search(p, msg) for p in _HTML_PATTERNS):
        return ERR_HTML_RESPONSE
    if any(re.search(p, msg) for p in _STALL_PATTERNS):
        return ERR_DOWNLOAD_STALL
    if any(re.search(p, msg) for p in _NOT_FOUND_PATTERNS):
        return ERR_NOT_FOUND
    if any(re.search(p, msg) for p in _CDN_EXPIRED_PATTERNS):
        return ERR_CDN_EXPIRED
    if any(re.search(p, msg) for p in _NETWORK_PATTERNS):
        return ERR_NETWORK
    if any(re.search(p, msg) for p in _DOWNLOADER_PATTERNS):
        return ERR_DOWNLOADER
    return ERR_UNKNOWN


def is_recoverable_via_reresolve(category: str) -> bool:
    """True if re-fetching the detail page may yield a working URL."""
    return category in {
        ERR_CDN_EXPIRED,
        ERR_NOT_FOUND,
        ERR_DOWNLOAD_STALL,
        ERR_BLOCKED,
        ERR_HTML_RESPONSE,
    }


def is_recoverable_via_alternate_source(category: str) -> bool:
    """True if trying another source for the same title may work."""
    return category in {
        ERR_NO_VIDEO_URL,
        ERR_VALIDATION,
        ERR_CDN_EXPIRED,
        ERR_BLOCKED,
        ERR_NOT_FOUND,
        ERR_DOWNLOAD_STALL,
        ERR_HTML_RESPONSE,
        ERR_NETWORK,
    }


# ---------------------------------------------------------------------------
# Fallback URL extraction (when get_details returned no video_urls)
# ---------------------------------------------------------------------------

def find_iframe_candidates(parser, detail_url: str) -> List[dict]:
    """
    Fetch the detail page directly, locate any iframe-style embeds and resolve
    each via media_extractor.resolve_embed_to_candidates.

    Returns parser-format candidates: {"url", "type", "quality", "headers"}.
    """
    try:
        from media_extractor import resolve_embed_to_candidates
    except Exception:
        return []

    out: List[dict] = []
    try:
        sess = getattr(parser, "session", None) or requests.Session()
        resp = sess.get(detail_url, timeout=20, allow_redirects=True, verify=False)
        if resp.status_code != 200 or not resp.text:
            logger.info(
                f"[RECOVERY] find_iframe_candidates: detail page returned "
                f"status={resp.status_code} url={detail_url[:80]}"
            )
            return []
        soup = BeautifulSoup(resp.text, "lxml")
        seen_iframes: set = set()
        for iframe in soup.find_all("iframe"):
            src = (
                iframe.get("src")
                or iframe.get("data-src")
                or iframe.get("data-player-src")
                or ""
            ).strip()
            if not src or src in seen_iframes:
                continue
            seen_iframes.add(src)
            if src.startswith("//"):
                src = "https:" + src
            if not src.startswith(("http://", "https://")):
                continue
            try:
                cands = resolve_embed_to_candidates(src, referer=detail_url, max_depth=2)
                for c in cands:
                    if c.get("url"):
                        out.append(c)
            except Exception as e:
                logger.info(f"[RECOVERY] iframe resolve failed for {src[:80]}: {e}")
        logger.info(
            f"[RECOVERY] find_iframe_candidates produced {len(out)} candidate(s) "
            f"from {len(seen_iframes)} iframe(s)"
        )
    except Exception as e:
        logger.info(f"[RECOVERY] find_iframe_candidates failed: {e}")
    return out


def try_playwright_extraction(parser, detail_url: str) -> List[dict]:
    """
    If the parser exposes _extract_media_with_playwright(url), call it.
    Returns a list of candidates in parser format, or [] if unavailable.
    """
    fn = getattr(parser, "_extract_media_with_playwright", None)
    if not callable(fn):
        return []
    try:
        results = fn(detail_url) or []
    except Exception as e:
        logger.warning(f"[RECOVERY] playwright extraction failed: {e}")
        return []
    out: List[dict] = []
    for r in results:
        if isinstance(r, dict) and r.get("url"):
            out.append(r)
    logger.info(f"[RECOVERY] try_playwright_extraction produced {len(out)} candidate(s)")
    return out


# ---------------------------------------------------------------------------
# Candidate selection across attempts
# ---------------------------------------------------------------------------

def next_unseen_candidate(
    video_urls: List[dict],
    seen_urls: set,
) -> Optional[Tuple[str, str, str]]:
    """
    Pick the next candidate URL that hasn't been tried this job.
    Returns (url, type, referer) or None.
    """
    for v in video_urls or []:
        u = (v.get("url") or "").strip()
        if not u or u in seen_urls:
            continue
        t = v.get("type") or "unknown"
        ref = ""
        hdrs = v.get("headers")
        if isinstance(hdrs, dict):
            ref = hdrs.get("Referer", "") or hdrs.get("referer", "")
        return u, t, ref
    return None


# ---------------------------------------------------------------------------
# Multi-source fallback
# ---------------------------------------------------------------------------

def _title_match(query: str, candidate_title: str) -> bool:
    """Loose containment match — same word stem on either side."""
    if not query or not candidate_title:
        return False
    q = query.strip().lower()
    c = candidate_title.strip().lower()
    if q == c:
        return True
    return q in c or c in q


def try_alternate_sources(
    title: str,
    year: Optional[int],
    exclude_source: str,
    parsers_registry: Dict[str, object],
    max_sources_to_try: int = 3,
) -> Optional[Tuple[str, dict, List[dict]]]:
    """
    Search alternate sources for a title and return the first one with
    extractable video_urls.

    Returns (source_name, details_dict, candidate_urls) or None.
    """
    if not title:
        logger.info("[RECOVERY] try_alternate_sources: no title — skipping")
        return None

    tried = 0
    for src, parser in parsers_registry.items():
        if src == exclude_source:
            continue
        if not hasattr(parser, "search") or not hasattr(parser, "get_details"):
            continue
        if tried >= max_sources_to_try:
            break
        tried += 1

        try:
            logger.info(f"[RECOVERY] try_alternate_sources: searching '{title}' on {src}")
            search_results = parser.search(title) or []
        except Exception as e:
            logger.info(f"[RECOVERY] alternate source {src} search failed: {e}")
            continue
        if not search_results:
            continue

        # Pick the best result by loose title match, with optional year filter.
        pick = None
        for r in search_results[:8]:
            r_title = getattr(r, "title", None) or (
                r.get("title", "") if isinstance(r, dict) else ""
            )
            r_year = getattr(r, "year", None) or (
                r.get("year", None) if isinstance(r, dict) else None
            )
            if year and r_year:
                try:
                    if abs(int(r_year) - int(year)) > 1:
                        continue
                except (TypeError, ValueError):
                    pass
            if _title_match(title, r_title):
                pick = r
                break
        if not pick:
            pick = search_results[0]

        detail_url = (
            getattr(pick, "detail_url", None)
            or getattr(pick, "url", None)
            or (pick.get("detail_url") if isinstance(pick, dict) else None)
            or (pick.get("url") if isinstance(pick, dict) else None)
        )
        if not detail_url:
            continue
        try:
            details = parser.get_details(detail_url)
        except Exception as e:
            logger.info(f"[RECOVERY] alternate source {src} get_details failed: {e}")
            continue

        d = (
            details.to_dict() if hasattr(details, "to_dict")
            else (details if isinstance(details, dict) else {})
        )
        vurls = d.get("video_urls") or []
        if vurls:
            logger.info(
                f"[RECOVERY] alternate source {src} produced {len(vurls)} candidate(s) "
                f"for title='{title}'"
            )
            return src, d, vurls

    logger.info(
        f"[RECOVERY] try_alternate_sources: tried {tried} alternate(s), none had video_urls"
    )
    return None
