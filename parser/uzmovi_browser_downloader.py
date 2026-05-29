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

        def _on_response(resp):
            try:
                u = resp.url
                if resp.status != 200:
                    return
                if "uzdown" not in u or ".mpd" not in u.lower():
                    return
                body = resp.body()
                txt = body.decode("utf-8", errors="ignore")
                if txt.lstrip().startswith("#EXTM3U"):
                    captured_manifests.append((u, txt))
            except Exception:
                pass

        page.on("response", _on_response)

        logger.info(f"[uzmovi-bd] navigating: {detail_url}")
        try:
            page.goto(detail_url, wait_until="networkidle", timeout=page_load_timeout_ms)
        except Exception as e:
            browser.close()
            return _failure(f"page navigation failed: {e}", started, segments_total, segments_done, bytes_done)
        page.wait_for_timeout(settle_ms)

        if not captured_manifests:
            browser.close()
            return _failure("manifest not captured (page may not have a player or signing was reworked)",
                            started, segments_total, segments_done, bytes_done)

        # Prefer the highest-bandwidth variant. HLS master playlists have
        # #EXT-X-STREAM-INF lines with BANDWIDTH= attributes; media playlists
        # have #EXTINF lines and TS segments. If we captured a master we
        # parse it and fetch its top variant; otherwise we score each
        # captured media playlist by its highest TS segment size hint
        # (URLs encode the quality, e.g. ".../720p/seg.ts") with a fallback
        # to the most recently captured one.
        manifest_url, manifest_body = _pick_best_manifest(
            captured_manifests, page
        )
        logger.info(f"[uzmovi-bd] picked manifest={manifest_url[:120]} from {len(captured_manifests)} captured")

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


def _pick_best_manifest(captured, page):
    """Given a list of (url, body) tuples captured during page load, return
    the (url, body) of the highest-quality media playlist we can identify.

    Strategy:
    1) If any master playlist was captured, parse its variants and pick the
       one with the highest BANDWIDTH. If that variant body is also among
       the captured items, return it directly; otherwise fetch it via the
       browser's authenticated context.
    2) Otherwise, fall back to a simple URL heuristic: look for a quality
       token (1080, 720, 480, 360) in the captured URLs and prefer the
       highest. If none matches, use the most recently captured manifest.
    """
    masters = [(u, b) for (u, b) in captured if _looks_like_master(b)]
    by_url = {u: b for (u, b) in captured}

    if masters:
        # Use the latest master in case the player loaded several.
        master_url, master_body = masters[-1]
        variants = _parse_master_variants(master_url, master_body)
        if variants:
            variants.sort(key=lambda v: (v[0], v[1]), reverse=True)
            best_bw, best_h, best_url = variants[0]
            logger.info(f"[uzmovi-bd] master had {len(variants)} variants; "
                        f"picked bandwidth={best_bw} height={best_h} url={best_url[:120]}")
            if best_url in by_url:
                return best_url, by_url[best_url]
            # Fetch the chosen variant with the page's cookies/headers.
            try:
                resp = page.context.request.get(best_url)
                if resp.status == 200:
                    return best_url, resp.text()
                logger.warning(f"[uzmovi-bd] best variant fetch status={resp.status}; falling back")
            except Exception as e:
                logger.warning(f"[uzmovi-bd] best variant fetch error: {e}")

    quality_order = [("1080", 4), ("720", 3), ("480", 2), ("360", 1)]
    scored = []
    for u, b in captured:
        if _looks_like_master(b):
            continue
        score = 0
        ul = u.lower()
        for token, weight in quality_order:
            if token in ul:
                score = weight
                break
        scored.append((score, u, b))
    if scored:
        scored.sort(key=lambda x: x[0], reverse=True)
        return scored[0][1], scored[0][2]

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
