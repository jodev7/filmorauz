"""Download uzmovi.net videos by driving the browser through Playwright.

uzmovi.net signs every CDN request with x-path / x-match headers generated
inside the page's player JS. We can't replicate the signing — but the
browser's own `videojs.xhr()` can: uzmovi patches it to inject the headers
automatically. This module opens the movie page in headless Chromium,
harvests the HLS manifest as the player fetches it, then walks the
segment list and pulls each segment through `videojs.xhr` (auth headers
auto-injected). Segments are concatenated into a .ts file then transcoded
to .mp4 via ffmpeg copy (no re-encode) — fast and lossless.

Single entry point: ``download_uzmovi_video(detail_url, output_path, ...)``.
"""
from __future__ import annotations

import base64
import logging
import os
import re
import subprocess
import time
import urllib.parse
from typing import Callable, Optional

logger = logging.getLogger(__name__)

# JS that fetches one segment via videojs.xhr (uzmovi-patched, signs requests)
# and returns the body as base64 — the only safe way to ship binary across
# Playwright's evaluate boundary.
_FETCH_SEGMENT_JS = """
async (url) => {
  return await new Promise(resolve => {
    if (typeof videojs === 'undefined' || !videojs.xhr) {
      return resolve({error: 'videojs.xhr unavailable'});
    }
    videojs.xhr({uri: url, responseType: 'arraybuffer', timeout: 30000}, function(err, resp, body){
      if (err) return resolve({error: String(err)});
      if (!resp || resp.statusCode !== 200) return resolve({status: resp ? resp.statusCode : 0});
      let bin = '';
      const u8 = new Uint8Array(body);
      const chunk = 0x8000;
      for (let j = 0; j < u8.length; j += chunk) {
        bin += String.fromCharCode.apply(null, u8.subarray(j, j+chunk));
      }
      resolve({status: 200, b64: btoa(bin)});
    });
  });
}
"""


# Same signed channel as _FETCH_SEGMENT_JS, but for text (manifests).
_FETCH_TEXT_JS = """
async (url) => {
  return await new Promise(resolve => {
    if (typeof videojs === 'undefined' || !videojs.xhr) {
      return resolve({error: 'videojs.xhr unavailable'});
    }
    videojs.xhr({uri: url, timeout: 20000}, function(err, resp, body){
      if (err) return resolve({error: String(err)});
      if (!resp || resp.statusCode !== 200) return resolve({status: resp ? resp.statusCode : 0});
      resolve({status: 200, text: String(body || '')});
    });
  });
}
"""

# Reads what the player has actually been handed. video.src is a blob: URL
# once MSE takes over, so currentSources()/<source> are the reliable ones.
_PLAYER_SRC_JS = r"""
() => {
  const out = [];
  try {
    if (window.videojs && videojs.getPlayers) {
      for (const p of Object.values(videojs.getPlayers())) {
        try {
          const c = p.currentSrc && p.currentSrc();
          if (c) out.push(c);
          (p.currentSources ? p.currentSources() : []).forEach(s => s && s.src && out.push(s.src));
        } catch (e) {}
      }
    }
  } catch (e) {}
  document.querySelectorAll('video[src], video source[src]').forEach(
    v => out.push(v.getAttribute('src'))
  );
  return out.filter(Boolean);
}
"""

# Pre-roll/ad players live on these hosts and would otherwise be downloaded
# as if they were the episode.
_AD_MEDIA_HOSTS = ("sova.live", "doubleclick", "googlesyndication", "adservice")


def _is_ad_media(url: str) -> bool:
    low = (url or "").lower()
    return any(h in low for h in _AD_MEDIA_HOSTS)


def _collect_player_manifest_urls(page) -> list:
    """Manifest URLs the player itself is pointing at, de-duplicated."""
    try:
        raw = page.evaluate(_PLAYER_SRC_JS) or []
    except Exception as e:
        logger.warning(f"[uzmovi-bd] player src scan failed: {e}")
        return []
    urls = []
    for u in raw:
        if not isinstance(u, str) or not u:
            continue
        if u.startswith(("blob:", "data:")) or _is_ad_media(u):
            continue
        if ".m3u8" not in u.lower() and ".mpd" not in u.lower():
            continue
        if u not in urls:
            urls.append(u)
    return urls


def _fetch_manifest_via_player(page, url: str) -> str:
    """Fetch a manifest through videojs.xhr (uzmovi request signing applied).

    Returns the manifest text, or "" when the URL does not answer with one.
    """
    try:
        res = page.evaluate(_FETCH_TEXT_JS, url) or {}
    except Exception as e:
        logger.warning(f"[uzmovi-bd] player fetch failed url={url[:120]} err={e}")
        return ""
    if res.get("error") or res.get("status") != 200:
        logger.info(f"[uzmovi-bd] player fetch url={url[:120]} "
                    f"status={res.get('status')} err={res.get('error')}")
        return ""
    text = res.get("text") or ""
    return text if text.lstrip().startswith("#EXTM3U") else ""


def download_uzmovi_video(
    detail_url: str,
    output_path: str,
    progress_cb: Optional[Callable[[int, int, int], None]] = None,
    page_load_timeout_ms: int = 60_000,
    settle_ms: int = 8_000,
    max_retries_per_segment: int = 2,
) -> dict:
    """Download a full uzmovi.net video into ``output_path`` (mp4).

    Returns a dict shaped like the rest of the downloader_service results:
      {success, file_path, error, total_bytes, type, segments_total, segments_done}
    """
    out_dir = os.path.dirname(output_path) or "."
    os.makedirs(out_dir, exist_ok=True)
    tmp_ts = output_path + ".ts.tmp"

    started = time.time()
    segments_total = 0
    segments_done = 0
    bytes_done = 0

    # Lazy import so a missing Playwright install doesn't break the rest
    # of the parser at module-import time.
    from playwright.sync_api import sync_playwright  # type: ignore

    with sync_playwright() as pw:
        browser = pw.chromium.launch(headless=True, args=["--no-sandbox"])
        context = browser.new_context(
            user_agent=(
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
            ),
        )
        page = context.new_page()

        # Capture EVERY HLS manifest the player fetches — both the master
        # (multi-variant) playlist and any media playlists (per-variant
        # segment lists). The first-match-only strategy used previously
        # captured whichever response landed first, which on uzmovi is
        # typically the lowest-bandwidth variant the player picks for an
        # ABR cold start. We now keep all of them so we can deliberately
        # pick the highest-bitrate variant below.
        captured_manifests = []  # list of (url, body)
        # Every .mpd URL we see referenced anywhere — direct request URLs
        # and any XHR/HTML/JSON response bodies that contain mpd links.
        # Used as a wider candidate pool than the manifests the player
        # actually loaded.
        seen_mpd_refs = set()
        _mpd_url_re = re.compile(r"https?://[^\s\"'<>\\]+\.(?:mpd|m3u8)[^\s\"'<>\\]*")

        def _on_response(resp):
            try:
                u = resp.url
                if resp.status != 200:
                    return
                lower_u = u.lower()
                if _is_ad_media(lower_u):
                    return
                if ".mpd" in lower_u or ".m3u8" in lower_u:
                    seen_mpd_refs.add(u)
                if "uzdown" not in lower_u or not (".mpd" in lower_u or ".m3u8" in lower_u):
                    # Still scan smaller text bodies for embedded .mpd refs
                    try:
                        ct = (resp.headers or {}).get("content-type", "").lower()
                    except Exception:
                        ct = ""
                    if not any(t in ct for t in ("json", "html", "javascript", "text")):
                        return
                    try:
                        body = resp.body()
                    except Exception:
                        return
                    if not body or len(body) > 4_000_000:
                        return
                    try:
                        txt = body.decode("utf-8", errors="ignore")
                    except Exception:
                        return
                    for m in _mpd_url_re.findall(txt):
                        seen_mpd_refs.add(m)
                    return
                body = resp.body()
                txt = body.decode("utf-8", errors="ignore")
                if txt.lstrip().startswith("#EXTM3U"):
                    captured_manifests.append((u, txt))
            except Exception:
                pass

        page.on("response", _on_response)

        logger.info(f"[uzmovi-bd] navigating: {detail_url}")
        # "domcontentloaded", never "networkidle": the page embeds ad iframes
        # on hosts that never resolve, so the network never goes idle and every
        # navigation burns the full timeout before throwing — with the DOM and
        # the player already fully in place. An unsettled navigation is normal
        # here, so keep going and let the manifest check below be the verdict.
        try:
            page.goto(detail_url, wait_until="domcontentloaded", timeout=page_load_timeout_ms)
        except Exception as e:
            logger.info(f"[uzmovi-bd] navigation unsettled ({str(e)[:120]}); continuing")

        # Wait for the player to hand itself a source instead of sleeping a
        # fixed settle_ms — most episodes are ready in a couple of seconds.
        deadline = time.time() + (settle_ms / 1000.0)
        player_urls = []
        while time.time() < deadline:
            page.wait_for_timeout(1000)
            player_urls = _collect_player_manifest_urls(page)
            if captured_manifests or player_urls:
                break

        # Passive capture alone is not enough on serial episodes: the browser's
        # own request for the media playlist answers 301 (no body to read) and
        # the signed .mpd rewrite the player follows answers 502. The same URL
        # fetched through videojs.xhr — which injects uzmovi's x-path/x-match
        # signing — returns the real #EXTM3U. So pull whatever the player is
        # actually pointing at through that channel.
        for purl in player_urls:
            if any(purl == u for u, _ in captured_manifests):
                continue
            body = _fetch_manifest_via_player(page, purl)
            if body:
                logger.info(f"[uzmovi-bd] manifest via videojs.xhr: {purl[:120]}")
                captured_manifests.append((purl, body))

        if not captured_manifests:
            browser.close()
            return _failure("manifest not captured (page may not have a player or signing was reworked)",
                            started, segments_total, segments_done, bytes_done)

        # uzmovi pages typically embed multiple .mpd URLs (one per quality)
        # in player config, <source> tags, or inline scripts — but the
        # player only requests one of them on cold start. Grab every .mpd
        # URL we can find in the DOM/scripts and probe each to pick the
        # one with the largest first segment (a strong proxy for bitrate).
        dom_urls = _collect_mpd_urls_from_dom(page)
        for u, _ in captured_manifests:
            if u not in dom_urls:
                dom_urls.append(u)
        for u in seen_mpd_refs:
            if u not in dom_urls:
                dom_urls.append(u)
        logger.info(f"[uzmovi-bd] candidate .mpd urls (dom={len(_collect_mpd_urls_from_dom(page))}, "
                    f"captured={len(captured_manifests)}, refs={len(seen_mpd_refs)}, total={len(dom_urls)})")
        for u in dom_urls[:10]:
            logger.info(f"[uzmovi-bd]   candidate: {u[:160]}")

        manifest_url, manifest_body = _pick_best_manifest(
            captured_manifests, dom_urls, page
        )
        logger.info(f"[uzmovi-bd] picked manifest={manifest_url[:120]}")

        base = urllib.parse.urlsplit(manifest_url)
        host = f"{base.scheme}://{base.netloc}"
        seg_lines = [l.strip() for l in manifest_body.splitlines()
                     if l.strip() and not l.startswith("#")]
        segments = []
        for line in seg_lines:
            if line.startswith("http"):
                segments.append(line)
            elif line.startswith("/"):
                segments.append(host + urllib.parse.quote(line, safe="/"))
            else:
                segments.append(urllib.parse.urljoin(manifest_url, line))
        segments_total = len(segments)
        logger.info(f"[uzmovi-bd] manifest={manifest_url[:120]} segments={segments_total}")

        if segments_total == 0:
            browser.close()
            return _failure("manifest had zero segments", started, segments_total, segments_done, bytes_done)

        try:
            with open(tmp_ts, "wb") as f:
                for i, seg in enumerate(segments):
                    last_err = None
                    for attempt in range(max_retries_per_segment + 1):
                        try:
                            result = page.evaluate(_FETCH_SEGMENT_JS, seg)
                            if result.get("status") == 200 and result.get("b64"):
                                data = base64.b64decode(result["b64"])
                                f.write(data)
                                bytes_done += len(data)
                                segments_done += 1
                                break
                            last_err = f"status={result.get('status')} err={result.get('error')}"
                        except Exception as e:
                            last_err = str(e)
                        logger.warning(f"[uzmovi-bd] seg[{i}] attempt {attempt+1} failed: {last_err}")
                    else:
                        browser.close()
                        try:
                            os.unlink(tmp_ts)
                        except OSError:
                            pass
                        return _failure(f"segment {i} failed: {last_err}", started, segments_total, segments_done, bytes_done)

                    if progress_cb and (segments_done % 5 == 0 or segments_done == segments_total):
                        try:
                            progress_cb(segments_done, segments_total, bytes_done)
                        except Exception:
                            pass
        finally:
            browser.close()

    # Sanity-check the .ts before handing it to ffmpeg — a 254/SIGINT-style
    # failure on a missing or zero-byte input is otherwise indistinguishable
    # from a real container problem.
    if not os.path.exists(tmp_ts) or os.path.getsize(tmp_ts) == 0:
        return _failure(
            f"ts buffer missing or empty after segment download: {tmp_ts} exists={os.path.exists(tmp_ts)}",
            started, segments_total, segments_done, bytes_done,
        )

    # Transcode .ts -> .mp4. Try stream copy first (fast); fall back to an
    # audio re-encode if the input has an audio codec that the MP4 container
    # rejects without re-muxing. KEEP the .ts on disk if both attempts fail
    # so we can debug without re-downloading 700 MB of segments.
    def _run_ffmpeg(extra_args):
        cmd = ["ffmpeg", "-y", "-i", tmp_ts, "-loglevel", "warning"] + extra_args + [output_path]
        logger.info(f"[uzmovi-bd] ffmpeg cmd: {' '.join(cmd)}")
        try:
            return subprocess.run(cmd, capture_output=True, timeout=1800)
        except Exception as e:
            return None, e

    attempts = [
        ["-c", "copy", "-bsf:a", "aac_adtstoasc"],     # fast path
        ["-c:v", "copy", "-c:a", "aac", "-movflags", "+faststart"],  # audio re-encode
    ]
    last_err = ""
    for i, extra in enumerate(attempts):
        cp = _run_ffmpeg(extra)
        if isinstance(cp, tuple):
            last_err = f"ffmpeg invoke failed: {cp[1]}"
            continue
        if cp.returncode == 0:
            try:
                os.unlink(tmp_ts)
            except OSError:
                pass
            break
        # Take the TAIL of stderr — ffmpeg dumps its banner+config to stderr
        # before the actual error, so the head is useless. 2000 chars is
        # enough to see the failing demuxer line + cause.
        tail = cp.stderr.decode("utf-8", errors="ignore")[-2000:]
        last_err = f"ffmpeg attempt {i+1} returncode={cp.returncode} tail={tail!r}"
        logger.error(f"[uzmovi-bd] {last_err}")
    else:
        return _failure(last_err, started, segments_total, segments_done, bytes_done)

    file_size = os.path.getsize(output_path) if os.path.exists(output_path) else 0
    logger.info(
        f"[uzmovi-bd] done segments={segments_done}/{segments_total} "
        f"bytes_in={bytes_done} file_size={file_size} duration={time.time()-started:.1f}s"
    )
    return {
        "success": True,
        "file_path": output_path,
        "error": None,
        "total_bytes": file_size,
        "type": "mp4",
        "segments_total": segments_total,
        "segments_done": segments_done,
        "duration_sec": time.time() - started,
    }


_STREAM_INF_RE = re.compile(r"#EXT-X-STREAM-INF:([^\n]*)", re.IGNORECASE)
_BANDWIDTH_RE = re.compile(r"BANDWIDTH=(\d+)", re.IGNORECASE)
_RESOLUTION_RE = re.compile(r"RESOLUTION=(\d+)x(\d+)", re.IGNORECASE)


def _looks_like_master(body: str) -> bool:
    return "#EXT-X-STREAM-INF" in body


def _parse_master_variants(master_url: str, master_body: str):
    """Return list of (bandwidth, height, absolute_variant_url) parsed from
    an HLS master playlist. Lines alternate #EXT-X-STREAM-INF / URL."""
    variants = []
    lines = [l.strip() for l in master_body.splitlines() if l.strip()]
    pending = None
    for ln in lines:
        if ln.startswith("#EXT-X-STREAM-INF"):
            bw_m = _BANDWIDTH_RE.search(ln)
            res_m = _RESOLUTION_RE.search(ln)
            pending = (int(bw_m.group(1)) if bw_m else 0,
                       int(res_m.group(2)) if res_m else 0)
            continue
        if ln.startswith("#"):
            continue
        if pending is None:
            continue
        bw, h = pending
        pending = None
        if ln.startswith("http"):
            url = ln
        elif ln.startswith("/"):
            base = urllib.parse.urlsplit(master_url)
            url = f"{base.scheme}://{base.netloc}{ln}"
        else:
            url = urllib.parse.urljoin(master_url, ln)
        variants.append((bw, h, url))
    return variants


_DOM_MPD_SCAN_JS = r"""
() => {
  const urls = new Set();
  const isManifest = u => /\.(mpd|m3u8)/i.test(u || '');
  document.querySelectorAll('source').forEach(s => {
    if (isManifest(s.src)) urls.add(s.src);
    const ds = s.getAttribute('data-src') || s.getAttribute('data-file');
    if (isManifest(ds)) urls.add(ds);
  });
  document.querySelectorAll('video').forEach(v => {
    if (isManifest(v.src)) urls.add(v.src);
  });
  const html = document.documentElement.outerHTML;
  const re = /https?:\/\/[^"'\s<>\\]+\.(?:mpd|m3u8)[^"'\s<>\\]*/g;
  let m;
  while ((m = re.exec(html)) !== null) urls.add(m[0]);
  return Array.from(urls);
}
"""


def _collect_mpd_urls_from_dom(page) -> list:
    try:
        urls = page.evaluate(_DOM_MPD_SCAN_JS) or []
        return [u for u in urls if isinstance(u, str) and not _is_ad_media(u)]
    except Exception as e:
        logger.warning(f"[uzmovi-bd] DOM scan failed: {e}")
        return []


def _fetch_manifest(page, url: str):
    """Fetch an HLS manifest, preferring the browser context (cookies +
    headers preserved) and falling back to videojs.xhr.

    The context request is the cheaper path but the uzdown CDN rejects it for
    media playlists (301 with no body, or an outright ECONNRESET) — only
    requests signed by the page's own player JS are answered in full.

    Returns (status, text) or (None, None) on error."""
    status = None
    try:
        resp = page.context.request.get(url, timeout=20000)
        status = resp.status
        if status == 200:
            return 200, resp.text()
    except Exception as e:
        logger.info(f"[uzmovi-bd] context fetch failed url={url[:120]} err={str(e)[:120]}")

    body = _fetch_manifest_via_player(page, url)
    if body:
        return 200, body
    return status, None


def _first_segment_url(manifest_url: str, manifest_body: str):
    base = urllib.parse.urlsplit(manifest_url)
    host = f"{base.scheme}://{base.netloc}"
    for line in manifest_body.splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        if line.startswith("http"):
            return line
        if line.startswith("/"):
            return host + urllib.parse.quote(line, safe="/")
        return urllib.parse.urljoin(manifest_url, line)
    return None


def _probe_first_segment_size(page, seg_url: str) -> int:
    """Return Content-Length of a segment, or 0. We use a ranged GET because
    some CDNs do not implement HEAD; the first 1 byte gives us
    Content-Range and is cheap."""
    try:
        resp = page.context.request.get(seg_url, headers={"Range": "bytes=0-0"}, timeout=15000)
        if resp.status not in (200, 206):
            return 0
        cr = resp.headers.get("content-range") or resp.headers.get("Content-Range")
        if cr and "/" in cr:
            tail = cr.split("/")[-1].strip()
            if tail.isdigit():
                return int(tail)
        cl = resp.headers.get("content-length") or resp.headers.get("Content-Length")
        if cl and cl.isdigit():
            return int(cl)
    except Exception as e:
        logger.warning(f"[uzmovi-bd] segment probe failed url={seg_url[:120]} err={e}")
    return 0


def _pick_best_manifest(captured, dom_urls, page):
    """Pick the highest-quality media playlist.

    Strategy:
    1) If any captured manifest is an HLS master, use the highest-BANDWIDTH
       variant from it.
    2) Otherwise, probe every candidate .mpd URL (from the page DOM plus
       what the player captured), fetch its manifest, look at the first
       segment's reported size, and pick the URL whose first segment is
       largest. Higher bitrate ⇒ larger segments at fixed duration, so
       this reliably distinguishes 480p from 720p/1080p.
    3) Fall back to the most recently captured manifest if probing fails.
    """
    by_url = {u: b for (u, b) in captured}

    masters = [(u, b) for (u, b) in captured if _looks_like_master(b)]
    if masters:
        master_url, master_body = masters[-1]
        variants = _parse_master_variants(master_url, master_body)
        if variants:
            variants.sort(key=lambda v: (v[0], v[1]), reverse=True)
            best_bw, best_h, best_url = variants[0]
            logger.info(f"[uzmovi-bd] master had {len(variants)} variants; "
                        f"picked bandwidth={best_bw} height={best_h}")
            if best_url in by_url:
                return best_url, by_url[best_url]
            status, body = _fetch_manifest(page, best_url)
            if body:
                return best_url, body

    scored = []
    for url in dom_urls:
        if url in by_url:
            body = by_url[url]
        else:
            status, body = _fetch_manifest(page, url)
            if not body:
                logger.info(f"[uzmovi-bd] candidate skipped url={url[:120]} status={status}")
                continue
            if _looks_like_master(body):
                logger.info(f"[uzmovi-bd] candidate is a master; deferring")
                continue
        seg = _first_segment_url(url, body)
        if not seg:
            continue
        size = _probe_first_segment_size(page, seg)
        logger.info(f"[uzmovi-bd] candidate probe url={url[:80]} first_seg_size={size}")
        scored.append((size, url, body))

    if scored:
        scored.sort(key=lambda x: x[0], reverse=True)
        best_size, best_url, best_body = scored[0]
        logger.info(f"[uzmovi-bd] selected best by first_seg_size={best_size} url={best_url[:120]}")
        return best_url, best_body

    return captured[-1]


def _failure(msg, started, seg_total, seg_done, bytes_done):
    return {
        "success": False,
        "file_path": "",
        "error": msg,
        "total_bytes": bytes_done,
        "type": "mp4",
        "segments_total": seg_total,
        "segments_done": seg_done,
        "duration_sec": time.time() - started,
    }
