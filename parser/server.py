"""
Parser HTTP API Server
Provides REST API for Go backend to call parsers
"""
import json
import logging
import os
import time
import urllib.request
import urllib.error
import threading
import socketserver
from http.server import HTTPServer, BaseHTTPRequestHandler


class ThreadedHTTPServer(socketserver.ThreadingMixIn, HTTPServer):
    """Handle each request in a separate thread so long-running jobs
    (imports, downloads) do not block social uploads."""
    daemon_threads = True
from urllib.parse import urlparse, parse_qs
import sys

# Load environment variables from .env file FIRST
from dotenv import load_dotenv
load_dotenv()  # Loads parser/.env if it exists

# Pre-import all optional heavy dependencies in the main thread NOW, before
# ThreadedHTTPServer starts spawning request threads.  asyncio (and its
# sub-modules asyncio.coroutines / asyncio.exceptions) is transitively pulled
# in by instagrapi→httpx and by google-auth.  If two threads race to import it
# for the first time, one sees a partially-initialised asyncio package and hits:
#   "partially initialized module 'asyncio.coroutines' has no attribute 'iscoroutine'"
#   "NameError: name 'exceptions' is not defined"
# Importing everything here ensures sys.modules is fully populated before any
# handler thread runs.  Subsequent `import X` calls in threads are safe dict
# lookups — no initialisation race.
import asyncio as _asyncio  # noqa: F401 — force full asyncio init in main thread
try:
    from instagrapi import Client as _InstagrapiClient  # noqa: F401
except ImportError:
    pass
try:
    from google.oauth2.credentials import Credentials as _GoogleCredentials  # noqa: F401
    from google.auth.transport.requests import Request as _GoogleAuthRequest  # noqa: F401
    from googleapiclient.discovery import build as _googleapiclient_build  # noqa: F401
    from googleapiclient.http import MediaFileUpload as _GoogleMediaFileUpload  # noqa: F401
except ImportError:
    pass

from binary_manager import require_binary
from uzmovi import UzmoviParser
from freekino import FreekinoParser
from asilmedia import AsilmediaParser
from kinolar import KinolarParser
from downloader_service import DownloaderService
from metadata_normalizer import normalize_metadata, validate_metadata, create_worker_payload
from source_config import get_source_config

# Enable debug logging in development
DEBUG = os.environ.get("PARSER_DEBUG", "false").lower() == "true"
log_level = logging.DEBUG if DEBUG else logging.INFO

logging.basicConfig(
    level=log_level,
    format="%(asctime)s - %(name)s - %(levelname)s - %(message)s"
)
logger = logging.getLogger(__name__)

# CRITICAL: Ensure N_m3u8DL-RE binary exists before starting the server
# This will download the binary if missing and verify it works
# If the binary cannot be found/installed, the server will exit with an error
logger.info("[SERVER] Checking for N_m3u8DL-RE binary...")
N_M3U8DL_BINARY = require_binary()
logger.info(f"[SERVER] N_m3u8DL-RE binary confirmed: {N_M3U8DL_BINARY}")

# Available sources (including manual which doesn't need a parser)
AVAILABLE_SOURCES = ["uzmovi", "freekino", "asilmedia", "kinolar", "manual"]

# Initialize parsers (manual source doesn't have a parser - it receives direct URLs)
PARSERS = {
    "uzmovi": UzmoviParser(),
    "freekino": FreekinoParser(),
    "asilmedia": AsilmediaParser(),
    "kinolar": KinolarParser(),
}

# Initialize downloader service
DOWNLOAD_DIR = os.environ.get("DOWNLOAD_DIR", "downloads")
downloader_service = DownloaderService(DOWNLOAD_DIR)

# Worker URL (default to localhost:8083)
WORKER_URL = os.environ.get("WORKER_URL", "http://localhost:8083")

# Backend URL - for reporting progress
# Must be explicitly set for progress reporting to work
BACKEND_URL = os.environ.get("BACKEND_URL", "")


class ParserHandler(BaseHTTPRequestHandler):
    """HTTP request handler for parser API"""
    
    # Store server address for building URLs
    server_address_str = ""
    
    def _get_parser_base_url(self):
        """Get the base URL for this parser server"""
        if ParserHandler.server_address_str:
            return ParserHandler.server_address_str
        # Fallback: try to construct from request
        host = self.headers.get('Host', 'localhost:8082')
        return f"http://{host}"
    
    def _send_json(self, data, status=200):
        """Send JSON response"""
        try:
            self.send_response(status)
            self.send_header("Content-Type", "application/json")
            self.end_headers()
            self.wfile.write(json.dumps(data).encode())
        except (BrokenPipeError, ConnectionResetError):
            pass  # client disconnected before response was sent

    def _send_error(self, message, status=400):
        """Send error response"""
        self._send_json({"error": message}, status)
    
    def _read_json_body(self):
        """Read and parse JSON body from request"""
        content_length = int(self.headers.get("Content-Length", 0))
        if content_length == 0:
            return {}
        body = self.rfile.read(content_length)
        return json.loads(body.decode("utf-8"))
    
    def _get_details_from_parser(self, parser, source, source_id=None, detail_url=None):
        """
        Get details from parser. Calls get_details(url) or get_detail(url).
        Errors from the parser method are NOT swallowed — they propagate immediately
        so the real cause is visible in logs.
        """
        url = detail_url if detail_url else source_id

        if hasattr(parser, "get_details"):
            return parser.get_details(url)

        if hasattr(parser, "get_detail"):
            return parser.get_detail(url)

        raise ValueError(f"Parser '{source}' does not have a valid get_details method")
    
    def _extract_best_video_url(self, details, source):
        """
        Extract the best playable video URL from details.
        Returns (url, url_type) or (None, None) if no playable URL found.
        
        This method uses enhanced validation from media_extractor to ensure
        that only valid media URLs (not HTML pages) are returned.
        
        [ENHANCED] Now with comprehensive logging for debugging uzmovi URL extraction.
        [FIXED] Now returns any valid URL even if type is unknown, instead of returning None.
        """
        # Import the enhanced validation
        from media_extractor import (
            is_valid_media_url, 
            classify_media_url,
            validate_media_url_strict,
            MediaCandidate,
            choose_best_media_candidate
        )
        
        # Handle both dict and object formats
        if hasattr(details, "to_dict"):
            details = details.to_dict()
        
        if not isinstance(details, dict):
            logger.warning(f"[SERVER] Details is not a dict, cannot extract video URL")
            return None, None
        
        # Get video_urls list
        video_urls = details.get("video_urls", [])
        
        logger.info(f"[SERVER] ═══════════════════════════════════════════")
        logger.info(f"[SERVER] _extract_best_video_url() called for source: {source}")
        logger.info(f"[SERVER] Received {len(video_urls)} video URL(s) from parser")
        
        if not video_urls:
            logger.warning(f"[SERVER] No video_urls in details for source '{source}'")
            logger.info(f"[SERVER] ═══════════════════════════════════════════")
            return None, None
        
        # Log all received video URLs
        for i, v in enumerate(video_urls):
            url = v.get("url", "")
            url_type = v.get("type", "unknown")
            logger.info(f"[SERVER]   Parser URL[{i}]: type={url_type}, url={url[:150]}...")
        
        # Convert to candidates and validate using enhanced validation
        candidates = []
        rejected_count = 0
        
        for v in video_urls:
            url = v.get("url", "")
            if not url:
                logger.info(f"[SERVER]   SKIP: Empty URL")
                continue
            
            # Skip URLs that are clearly HTML pages using enhanced validation
            error = validate_media_url_strict(url)
            if error:
                rejected_count += 1
                logger.info(f"[SERVER]   REJECTED (validate_media_url_strict): type={v.get('type', 'unknown')}, url={url[:100]}...")
                logger.info(f"[SERVER]   Reason: {error}")
                continue
            
            # Classify the URL type
            url_type = v.get("type", "") or classify_media_url(url)
            
            # FIXED: Accept any URL that passes validation, even if type is unknown
            # This is more lenient for uzmovi URLs which may not have standard extensions
            confidence = 0.9 if url_type in ["mp4", "m3u8", "mpd"] else 0.7
            
            # Create candidate
            candidate = MediaCandidate(
                url=url,
                type=url_type,
                quality=v.get("quality", "auto"),
                source_hint=v.get("type", "unknown"),
                confidence=confidence
            )
            candidates.append(candidate)
            
            logger.info(f"[SERVER]   ACCEPTED: type={url_type}, url={url[:120]}...")
        
        logger.info(f"[SERVER] Validation: {len(candidates)} accepted, {rejected_count} rejected")
        
        if not candidates:
            logger.warning(f"[SERVER] All {len(video_urls)} video URLs were rejected by enhanced validation")
            logger.info(f"[SERVER] ═══════════════════════════════════════════")
            return None, None
        
        # Use the enhanced selection function
        best_candidate = choose_best_media_candidate(candidates)
        
        if best_candidate:
            logger.info(f"[SERVER] ═══════════════════════════════════════════")
            logger.info(f"[SERVER] SELECTED media URL:")
            logger.info(f"[SERVER]   type: {best_candidate.type}")
            logger.info(f"[SERVER]   url: {best_candidate.url}")
            logger.info(f"[SERVER]   confidence: {best_candidate.confidence}")
            logger.info(f"[SERVER] ═══════════════════════════════════════════")
            return best_candidate.url, best_candidate.type
        
        # FIXED: If enhanced selection fails, try manual selection from validated candidates
        logger.info(f"[SERVER] Falling back to manual type-based selection")
        
        # Preferred URL types in order (best first)
        # FIXED: Include 'unknown' type as last resort - uzmovi URLs may not have standard extensions
        preferred_types = [
            "m3u8",
            "mpd",
            "ism",
            "mp4",
            "direct_mp4",
            "direct_download", 
            "hls",
            "html5_video",
            "html5_source",
            "script_extracted",
            "direct_from_iframe",
            "html5_video_from_iframe",
            "html5_source_from_iframe",
            "unknown",  # FIXED: Accept unknown types as last resort
        ]
        
        # Find best URL by type
        for url_type in preferred_types:
            for candidate in candidates:
                if candidate.type == url_type:
                    logger.info(f"[SERVER] Chosen media URL (manual fallback): type={url_type}, url={candidate.url[:80]}...")
                    return candidate.url, candidate.type
        
        # FIXED: If still no URL found, return the first valid candidate (last resort)
        if candidates:
            first = candidates[0]
            logger.warning(f"[SERVER] No preferred type found, using first valid URL: type={first.type}, url={first.url[:80]}...")
            return first.url, first.type
        
        # If we somehow get here with no candidates, return None
        logger.warning(f"[SERVER] No valid video URL found")
        logger.info(f"[SERVER] ═══════════════════════════════════════════")
        return None, None
    
    def _call_worker(self, input_file, title, cut_seconds=0):
        """
        Call the worker to process a downloaded video.
        The worker always applies the watermark from its own docs/logo.png —
        logo_path must NOT be sent so the worker uses its resolved path.
        Returns the worker's response dict or raises an exception.
        """
        request_data = {
            "source": "parser",
            "title": title,
            "input_file": input_file,
            "cut_seconds": cut_seconds,
        }
        
        logger.info(f"[SERVER] Calling worker at {WORKER_URL}/process")
        logger.debug(f"[SERVER] Worker request: {request_data}")
        
        req = urllib.request.Request(
            f"{WORKER_URL}/process",
            data=json.dumps(request_data).encode("utf-8"),
            headers={"Content-Type": "application/json"},
            method="POST",
        )
        
        try:
            with urllib.request.urlopen(req, timeout=300) as response:
                result = json.loads(response.read().decode("utf-8"))
                logger.info(f"[SERVER] Worker response: success={result.get('success')}")
                return result
        except urllib.error.HTTPError as e:
            error_body = e.read().decode("utf-8") if e.fp else ""
            logger.error(f"[SERVER] Worker HTTP error {e.code}: {error_body}")
            raise Exception(f"Worker returned HTTP {e.code}: {error_body}")
        except urllib.error.URLError as e:
            logger.error(f"[SERVER] Worker connection error: {e.reason}")
            raise Exception(f"Failed to connect to worker: {e.reason}")
        except Exception as e:
            logger.error(f"[SERVER] Worker call error: {e}")
            raise
    
    def _report_progress_to_backend(self, job_id: str, progress: dict):
        """
        Report download progress to the backend job API.
        Best-effort only - failures do not block the pipeline.
        """
        import time
        import urllib.error
        
        logger.info(f"[SERVER] _report_progress_to_backend called with job_id='{job_id}'")
        logger.info(f"[SERVER] progress payload: {progress}")
        
        if not job_id or job_id == "none":
            logger.warning("[SERVER] No job_id or job_id='none', skipping progress report")
            return
        
        if BACKEND_URL == "":
            logger.warning(f"[SERVER] BACKEND_URL not set, skipping progress report")
            return
        
        # Best-effort with retries
        max_retries = 3
        backoff = 0.5
        
        for attempt in range(max_retries):
            try:
                url = f"{BACKEND_URL}/api/ingestion/jobs/{job_id}/progress"
                data = json.dumps(progress).encode("utf-8")
                
                logger.info(f"[SERVER] Reporting progress: job_id={job_id}, progress={progress.get('progress_percent')}%, attempt {attempt+1}/{max_retries}")
                
                req = urllib.request.Request(
                    url,
                    data=data,
                    headers={"Content-Type": "application/json"},
                    method="POST"
                )
                
                with urllib.request.urlopen(req, timeout=15) as response:
                    logger.info(f"[SERVER] Progress reported: job_id={job_id}, status={response.status}")
                    return
            except urllib.error.HTTPError as e:
                body = ""
                try:
                    body = e.read().decode("utf-8", errors="replace")
                except:
                    pass
                if attempt < max_retries - 1:
                    logger.warning(f"[SERVER] HTTP {e.code} attempt {attempt+1}, retrying: {e.reason}")
                    time.sleep(backoff)
                    backoff *= 2
                else:
                    logger.warning(f"[SERVER] HTTP {e.code} after {max_retries} attempts, continuing: {e.reason}")
            except urllib.error.URLError as e:
                if attempt < max_retries - 1:
                    logger.warning(f"[SERVER] URL error attempt {attempt+1}, retrying: {e.reason}")
                    time.sleep(backoff)
                    backoff *= 2
                else:
                    logger.warning(f"[SERVER] URL error after {max_retries} attempts, continuing pipeline")
            except Exception as e:
                if attempt < max_retries - 1:
                    logger.warning(f"[SERVER] Error attempt {attempt+1}, retrying: {e}")
                    time.sleep(backoff)
                    backoff *= 2
                else:
                    logger.warning(f"[SERVER] Error after {max_retries} attempts, continuing pipeline: {e}")

    def do_GET(self):
        """Handle GET requests"""
        try:
            self._do_GET_inner()
        except (BrokenPipeError, ConnectionResetError):
            pass  # client disconnected mid-request

    def _do_GET_inner(self):
        parsed = urlparse(self.path)
        path = parsed.path
        query = parse_qs(parsed.query)

        logger.info(f"[SERVER] {self.command} {self.path}")
        
        # Routes
        if path == "/health":
            self._send_json({"status": "ok"})
            return
        
        # Progress: /progress/{job_id}
        elif path.startswith("/progress/"):
            job_id = path.split("/")[-1]
            if not job_id:
                self._send_error("Missing job_id")
                return
            
            progress = downloader_service.progress.get(job_id)
            if progress:
                self._send_json(progress)
            else:
                self._send_json({"status": "unknown", "message": "No active download for this job"})
            return
        
        # Search: /search?source=uzmovi&q=interstellar
        elif path == "/search":
            source = query.get("source", [""])[0]
            q = query.get("q", [""])[0]
            
            logger.info(f"[SERVER] Search request: source={source}, query={q}")
            
            # Manual source doesn't support search
            if source == "manual":
                self._send_json({
                    "source": "manual",
                    "query": q,
                    "results": [],
                    "message": "Manual source does not support search. Use direct video URL import."
                })
                return
            
            if not source or source not in PARSERS:
                self._send_error(f"Invalid source. Available: {AVAILABLE_SOURCES}")
                return
            
            if not q:
                self._send_error("Missing query parameter 'q'")
                return
            
            try:
                parser = PARSERS[source]
                results = parser.search(q)
                
                logger.info(f"[SERVER] Search response: {len(results)} results for query='{q}'")
                
                # Support both dict and object results
                serialized_results = []
                for r in results:
                    if hasattr(r, 'to_dict'):
                        serialized_results.append(r.to_dict())
                    elif isinstance(r, dict):
                        serialized_results.append(r)
                    else:
                        serialized_results.append(r)
                
                self._send_json({
                    "source": source,
                    "query": q,
                    "results": serialized_results
                })
            except Exception as e:
                logger.error(f"[SERVER] Search error: {e}", exc_info=True)
                self._send_error(f"Search failed: {str(e)}", 500)
            return
        
        # Details: /details?source=uzmovi&id=interstellar
        # Or: /details?source=uzmovi&url=https://uzmovi.tv/film/interstellar
        # Or: /details?source=manual&video_url=...&title=...&poster_url=...&year=...
        elif path == "/details":
            source = query.get("source", [""])[0]
            source_id = query.get("id", [""])[0]
            detail_url = query.get("url", [""])[0]
            
            # Get optional job_id for tracking download progress
            job_id = query.get("job_id", [""])[0]
            
            logger.info(f"[PARSER] Received job_id={job_id} from worker request")
            
            # ── MANUAL SOURCE HANDLING ──────────────────────────────────
            if source == "manual":
                video_url = query.get("video_url", [""])[0] or detail_url
                # Admin-entered values (forwarded by worker from job.Metadata)
                admin_title    = query.get("title", [""])[0].strip()
                admin_poster   = query.get("poster_url", [""])[0].strip()
                admin_backdrop = query.get("backdrop_url", [""])[0].strip()
                year_str       = query.get("year", ["0"])[0]
                try:
                    admin_year = int(year_str) if year_str else 0
                except ValueError:
                    admin_year = 0

                logger.info(f"[PARSER] Manual source: video_url={video_url}, admin_title={admin_title!r}")

                if not video_url:
                    self._send_error("Manual source requires 'video_url' or 'url' parameter")
                    return

                from helpers import is_youtube_url as _is_yt
                import re as _re, subprocess as _sp, sys as _sys

                # ── YouTube: fetch metadata via yt-dlp --dump-json ──────
                yt_title = ""
                yt_year  = 0
                yt_poster = ""
                yt_description = ""

                if _is_yt(video_url):
                    try:
                        meta_proc = _sp.run(
                            [_sys.executable, "-m", "yt_dlp", "--dump-json",
                             "--no-playlist", "--no-warnings", video_url],
                            capture_output=True, text=True, timeout=30,
                        )
                        if meta_proc.returncode == 0 and meta_proc.stdout.strip():
                            import json as _json
                            yt_meta = _json.loads(meta_proc.stdout.strip())
                            yt_title       = yt_meta.get("title", "")
                            yt_description = yt_meta.get("description", "")
                            # thumbnail: prefer the best-quality one
                            thumbs = yt_meta.get("thumbnails") or []
                            if thumbs:
                                yt_poster = thumbs[-1].get("url", "") or yt_meta.get("thumbnail", "")
                            else:
                                yt_poster = yt_meta.get("thumbnail", "")
                            upload_date = yt_meta.get("upload_date", "")  # YYYYMMDD
                            if upload_date and len(upload_date) >= 4:
                                try:
                                    yt_year = int(upload_date[:4])
                                except ValueError:
                                    pass
                            logger.info(f"[PARSER] YouTube metadata: title={yt_title!r}, year={yt_year}, thumb={yt_poster[:60]}")
                    except Exception as _yt_err:
                        logger.warning(f"[PARSER] yt-dlp metadata fetch failed (continuing): {_yt_err}")

                # ── Merge: admin-entered values override YouTube metadata ─
                title       = admin_title    or yt_title       or "Manual Import"
                year        = admin_year     or yt_year        or 0
                poster_url  = admin_poster   or yt_poster      or ""
                backdrop_url = admin_backdrop or ""
                description  = yt_description or ""

                # Detect video type (not relevant for YouTube, defaults to mp4)
                url_type = "mp4"
                url_lower = video_url.lower()
                if ".m3u8" in url_lower or "m3u8" in url_lower:
                    url_type = "m3u8"
                elif ".mpd" in url_lower:
                    url_type = "mpd"

                logger.info(f"[PARSER] Manual resolved: title={title!r}, year={year}, poster={poster_url[:60]}")

                # ── Sanitize output filename ───────────────────────────
                safe_title  = _re.sub(r'[^\w\s-]', '', title)
                safe_title  = _re.sub(r'[-\s]+', '_', safe_title)
                output_name = f"{safe_title}.mp4" if safe_title else "manual_import.mp4"

                backend_job_id = job_id if job_id else ""

                try:
                    if backend_job_id and BACKEND_URL:
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "downloading",
                            "progress_percent": 0,
                            "message": "Starting download...",
                        })

                    download_result = downloader_service.smart_download(
                        url=video_url,
                        output_name=output_name,
                        job_id=output_name,
                        backend_job_id=backend_job_id,
                        referer=None,
                    )

                    if not download_result.get("success"):
                        error_msg = download_result.get("error", "Download failed")
                        logger.error(f"[PARSER] Manual download failed: {error_msg}")
                        self._send_json({
                            "success": False,
                            "error": f"Download failed: {error_msg}",
                            "source": "manual",
                            "title": title,
                            "video_url": video_url,
                            "video_url_type": url_type,
                            "local_path": "",
                        }, 500)
                        return

                    local_path = download_result.get("file_path", "")
                    file_size  = os.path.getsize(local_path) if local_path and os.path.exists(local_path) else 0

                    logger.info("=" * 60)
                    logger.info("[PARSER DOWNLOAD COMPLETE] Manual source download finished")
                    logger.info(f"[PARSER DOWNLOAD COMPLETE] File: {local_path}")
                    logger.info(f"[PARSER DOWNLOAD COMPLETE] Size: {file_size} bytes")
                    logger.info("=" * 60)

                    if backend_job_id and BACKEND_URL:
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "process",
                            "status": "processing",
                            "progress_percent": 100,
                            "steps_download": True,
                            "file_path": local_path,
                            "message": "Download completed",
                        })

                    self._send_json({
                        "success":           True,
                        "source":            "manual",
                        "title":             title,
                        "description":       description,
                        "year":              year,
                        "poster":            poster_url,
                        "poster_url":        poster_url,
                        "backdrop":          backdrop_url,
                        "backdrop_url":      backdrop_url,
                        "video_url":         video_url,
                        "video_url_type":    url_type,
                        "video_found":       True,
                        "download_needed":   False,
                        "download_completed": True,
                        "local_path":        local_path,
                        "file_path":         local_path,
                        "file_size":         file_size,
                        "stream_type":       url_type,
                    })
                except Exception as e:
                    logger.error(f"[PARSER] Manual source error: {e}", exc_info=True)
                    self._send_json({
                        "success":       False,
                        "error":         str(e),
                        "source":        "manual",
                        "title":         title,
                        "video_url":     video_url,
                        "video_url_type": url_type,
                        "local_path":    "",
                    }, 500)
                return
            # ── END MANUAL SOURCE ───────────────────────────────────────
            
            if not source or source not in PARSERS:
                self._send_error(f"Invalid source. Available: {AVAILABLE_SOURCES}")
                return
            
            if not source_id and not detail_url:
                self._send_error("Missing 'id' or 'url' parameter")
                return
            
            try:
                parser = PARSERS[source]
                details = self._get_details_from_parser(parser, source, source_id, detail_url)
                
                # Convert to dict if needed
                if hasattr(details, 'to_dict'):
                    details = details.to_dict()
                
                logger.info(f"[PARSER] Details parsed for source={source}, id={source_id}")
                logger.info(f"[PARSER] Video URLs available: {len(details.get('video_urls', []))}")
                
                # Get source base URL for normalizing relative URLs
                source_config = get_source_config(source)
                source_base_url = source_config.get('base_url', '')
                
                # DEBUG: Log raw metadata from parser
                logger.info(f"[PARSER] RAW metadata before normalization:")
                logger.info(f"[PARSER]   title: {details.get('title', '')}")
                logger.info(f"[PARSER]   year: {details.get('year', '')}")
                logger.info(f"[PARSER]   genres: {details.get('genres', '')}")
                logger.info(f"[PARSER]   country: {details.get('country', '')}")
                logger.info(f"[PARSER]   poster: {details.get('poster', '')}")
                logger.info(f"[PARSER]   duration: {details.get('duration', '')}")
                logger.info(f"[PARSER]   quality: {details.get('quality', '')}")
                
                # Normalize metadata
                normalized_metadata = normalize_metadata(details, source, source_base_url)
                
                # Validate and log warnings
                is_valid, warnings = validate_metadata(normalized_metadata)
                if warnings:
                    logger.warning(f"[PARSER] Validation warnings: {warnings}")
                
                logger.info(f"[PARSER] NORMALIZED metadata:")
                logger.info(f"[PARSER]   title: {normalized_metadata.get('title', '')}")
                logger.info(f"[PARSER]   year: {normalized_metadata.get('year', '')}")
                logger.info(f"[PARSER]   genres: {normalized_metadata.get('genres', '')}")
                logger.info(f"[PARSER]   countries: {normalized_metadata.get('countries', '')}")
                logger.info(f"[PARSER]   poster_url: {normalized_metadata.get('poster_url', '')}")
                logger.info(f"[PARSER]   duration_minutes: {normalized_metadata.get('duration_minutes', '')}")
                logger.info(f"[PARSER]   quality: {normalized_metadata.get('quality', '')}")
                logger.info(f"[PARSER]   translation: {normalized_metadata.get('translation', '')}")
                
                # Extract the best video URL from the parsed details
                video_url, url_type = self._extract_best_video_url(details, source)
                
                # Get quality info from parser details
                source_quality = ""
                available_qualities = []
                video_urls_list = details.get('video_urls', [])
                
                # Extract quality from selected video URL
                if video_url:
                    for v in video_urls_list:
                        if v.get('url') == video_url:
                            source_quality = v.get('quality', '')
                            break
                
                # Get all available qualities from video_urls
                for v in video_urls_list:
                    q = v.get('quality', '')
                    if q and q != 'unknown' and q not in available_qualities:
                        available_qualities.append(q)
                
                if source_quality:
                    logger.info(f"[PARSER] Source quality: {source_quality}")
                if available_qualities:
                    logger.info(f"[PARSER] Available qualities: {available_qualities}")
                
                if not video_url:
                    # Debug: log what was actually found to diagnose blocked vs not-found
                    video_urls_list = details.get('video_urls', [])
                    found_types = [v.get('type', 'unknown') for v in video_urls_list]
                    player_url_found = details.get('player_url', '')
                    logger.error(f"[PARSER] video_url_not_found for source={source}, id={source_id}")
                    logger.error(f"[PARSER] Checked {len(video_urls_list)} video URLs, types found: {found_types}")
                    logger.error(f"[PARSER] Player URL fallback: {player_url_found or 'none'}")
                    # Detect likely blocked/anti-bot responses
                    if player_url_found and ('://' in player_url_found and 
                        any(bad in player_url_found.lower() for bad in ['blocked', 'captcha', 'cloudflare', '403', 'denied'])):
                        error_type = "site_blocked"
                        logger.error(f"[PARSER] DETECTED: Site likely blocked (player_url={player_url_found})")
                    else:
                        error_type = "video_url_not_found"
                    # Return explicit error - NEVER return empty video_url silently
                    page_url = detail_url if detail_url else source_id
                    
                    response_payload = create_worker_payload(
                        source=source,
                        source_url=source_base_url,
                        page_url=page_url,
                        video_url=None,
                        video_url_type="missing",
                        metadata=normalized_metadata,
                        local_path=""
                    )
                    response_payload["success"] = False
                    response_payload["error"] = error_type
                    response_payload["video_found"] = False
                    response_payload["download_needed"] = False
                    self._send_json(response_payload, 422)
                    return
                
                logger.info(f"[PARSER] Final video URL: {video_url[:80]}... (type: {url_type})")
                
                # CRITICAL: Parser MUST download the video and return local_path
                # Worker expects a valid local_path - cannot proceed without it
                
                # Get video page URL for referer
                video_page_url = details.get('video_page_url', detail_url or source_id)
                
                # Determine page URL
                page_url = detail_url if detail_url else source_id
                
                # Get cookies from parser session if available
                # This is critical for CDN URLs that require cookies from the video page
                parser_cookies = details.get('_session_cookies', {})
                if hasattr(details, '_session_cookies'):
                    parser_cookies = details._session_cookies
                elif hasattr(details, 'session_cookies'):
                    parser_cookies = details.session_cookies
                
                if parser_cookies:
                    logger.info(f"[PARSER] Captured {len(parser_cookies)} cookies from parser session")
                
                # Initialize referer_url variable
                referer_url = ""
                
                # Set referer_url from video_page_url if available, otherwise from page_url
                if details.get('video_page_url'):
                    referer_url = details.get('video_page_url')
                    logger.info(f"[PARSER] Using video_page_url as referer: {referer_url[:60]}...")
                elif page_url:
                    referer_url = page_url
                    logger.info(f"[PARSER] Using page_url as referer: {referer_url[:60]}...")
                else:
                    logger.info(f"[PARSER] No referer URL available")
                
                # Generate output filename from movie title
                output_name = normalized_metadata.get('title', source_id or 'video')
                # Sanitize filename - replace special chars
                import re
                output_name = re.sub(r'[^\w\s-]', '', output_name)
                output_name = re.sub(r'[-\s]+', '_', output_name)
                output_name = f"{output_name}.mp4"
                
                # Get job_id for progress reporting
                backend_job_id = job_id if job_id else ""
                
                # Start download progress reporting thread
                progress_thread = None
                if backend_job_id and BACKEND_URL:
                    import threading
                    
                    # CRITICAL: Use the SAME job_id as the downloader service
                    # The downloader uses output_name as the job_id for progress tracking
                    # We must use the same key to read progress updates
                    progress_job_id = output_name
                    
                    # Send initial progress
                    self._report_progress_to_backend(backend_job_id, {
                        "stage": "download",
                        "status": "downloading",
                        "progress_percent": 0,
                        "message": "Starting download...",
                    })
                    
                    def report_progress():
                        last_pct = -1
                        last_bytes = 0
                        while True:
                            # CRITICAL FIX: Use the SAME key as downloader service (output_name, not MD5 hash)
                            prog = downloader_service.progress.get(progress_job_id)
                            if prog:
                                pct = prog.get("progress_percent", 0)
                                downloaded = prog.get("downloaded_bytes", 0)
                                # Always report if bytes changed, even if percent is same
                                if pct != last_pct or downloaded != last_bytes:
                                    last_pct = pct
                                    last_bytes = downloaded
                                    self._report_progress_to_backend(backend_job_id, {
                                        "stage": "download",
                                        "status": "downloading",
                                        "progress_percent": pct,
                                        "downloaded_bytes": downloaded,
                                        "total_bytes": prog.get("total_bytes", 0),
                                        "speed_mbps": prog.get("speed_mb_per_sec", 0.0),
                                        "eta_seconds": prog.get("eta_seconds", 0),
                                        "message": prog.get("message", ""),
                                    })
                            # Check for completion
                            if prog and prog.get("status") in ["completed", "failed"]:
                                break
                            time.sleep(0.5)  # Poll more frequently for smoother updates
                    
                    progress_thread = threading.Thread(target=report_progress, daemon=True)
                    progress_thread.start()
                
                # Download the video
                try:
                    logger.info(f"[PARSER] Downloading video: {output_name}")
                    logger.info(f"[PARSER]   source: {source}")
                    logger.info(f"[PARSER]   video_url: {video_url[:80]}...")
                    logger.info(f"[PARSER]   video_url_type: {url_type}")
                    
                    # Send parsing stage complete, starting download
                    if backend_job_id and BACKEND_URL:
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "downloading",
                            "progress_percent": 0,
                            "message": "Downloading video...",
                        })
                    
                    # Download the file (referer_url computed at lines above from video_page_url / page_url)
                    download_result = downloader_service.smart_download(
                        url=video_url,
                        output_name=output_name,
                        job_id=output_name,
                        backend_job_id=backend_job_id,
                        referer=referer_url if referer_url else None,
                    )
                    
                    # Validate download result
                    if not download_result.get("success"):
                        error_msg = download_result.get("error", "Download failed")
                        logger.error(f"[PARSER] Download failed: {error_msg}")
                        raise Exception(error_msg)
                    
                    # Get downloaded file path
                    local_path = download_result.get("file_path", "")
                    
                    # CRITICAL: Validate the file exists and has content
                    if not local_path:
                        raise Exception("Downloader returned empty file_path")
                    
                    if not os.path.exists(local_path):
                        raise Exception(f"Downloaded file does not exist: {local_path}")
                    
                    file_size = os.path.getsize(local_path)
                    if file_size == 0:
                        raise Exception(f"Downloaded file is empty: {local_path}")
                    
                    logger.info(f"[PARSER] Download successful: {local_path} ({file_size} bytes)")
                    
                    # Send download complete progress
                    if backend_job_id and BACKEND_URL:
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "process",
                            "status": "processing",
                            "progress_percent": 100,
                            "steps_download": True,
                            "file_path": local_path,
                            "message": "Download completed, starting processing",
                        })
                    
                except Exception as download_error:
                    error_msg = str(download_error)
                    logger.error(f"[PARSER] Download failed: {error_msg}")
                    
                    # Send download failed progress
                    if backend_job_id and BACKEND_URL:
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "failed",
                            "progress_percent": 0,
                            "error": error_msg,
                            "message": f"Download failed: {error_msg}",
                        })
                    
                    # Return failure response - parser MUST return success=false when download fails
                    self._send_json({
                        "success": False,
                        "error": f"Download failed: {error_msg}",
                        "source": source,
                        "title": normalized_metadata.get("title", ""),
                        "video_url_type": url_type,
                        "local_path": "",
                        "download_needed": False,
                        "download_completed": False,
                        "download_error": error_msg,
                    }, 500)
                    return
                
                # Create structured worker payload with normalized metadata
                # Parser has downloaded the file and returns local_path
                response_payload = create_worker_payload(
                        source=source,
                        source_url=source_base_url,
                        page_url=page_url,
                        video_url=video_url,
                        video_url_type=url_type,
                        metadata=normalized_metadata,
                        local_path=local_path,
                        source_quality=source_quality,
                        available_qualities=available_qualities
                    )
                
                # Add additional fields for worker
                response_payload["success"] = True
                response_payload["video_found"] = True
                response_payload["download_needed"] = False  # Parser already downloaded
                response_payload["download_completed"] = True
                response_payload["video_page_url"] = video_page_url
                response_payload["file_path"] = local_path
                response_payload["file_name"] = output_name
                response_payload["file_size"] = file_size
                response_payload["stream_type"] = download_result.get("type", url_type)
                
                logger.info("=" * 60)
                logger.info("[PARSER DOWNLOAD COMPLETE] File ready for pipeline processing")
                logger.info(f"[PARSER DOWNLOAD COMPLETE] local_path: {local_path}")
                logger.info(f"[PARSER DOWNLOAD COMPLETE] file_size: {file_size} bytes")
                logger.info(f"[PARSER DOWNLOAD COMPLETE] video_url_type: {url_type}")
                logger.info("=" * 60)
                
                logger.info(f"[PARSER] Returning structured payload with local_path: {local_path}")
                logger.info(f"[PARSER]   video_url_type: {response_payload.get('video_url_type')}")
                logger.info(f"[PARSER]   download_completed: {response_payload.get('download_completed')}")
                logger.info(f"[PARSER]   file_size: {file_size} bytes")
                
                self._send_json(response_payload)
                
            except Exception as e:
                logger.error(f"Details error: {e}", exc_info=True)
                self._send_error(f"Failed to get details: {str(e)}", 500)
            return
        
        # Categories: /categories?source=uzmovi
        elif path == "/categories":
            source = query.get("source", [""])[0]

            if not source or source not in PARSERS:
                self._send_error(f"Invalid source for categories. Available: {list(PARSERS.keys())}")
                return

            try:
                parser = PARSERS[source]
                if hasattr(parser, 'list_categories'):
                    categories = parser.list_categories()
                else:
                    categories = []
                logger.info(f"[SERVER] Categories: {len(categories)} categories from {source}")
                self._send_json({"source": source, "categories": categories})
            except Exception as e:
                logger.error(f"[SERVER] Categories error for {source}: {e}", exc_info=True)
                self._send_error(f"Categories failed: {str(e)}", 500)
            return

        # Catalog: /catalog?source=uzmovi&page=1&limit=20
        elif path == "/catalog":
            source = query.get("source", [""])[0]
            page_str = query.get("page", ["1"])[0]
            limit_str = query.get("limit", ["20"])[0]
            type_filter = query.get("type", [""])[0]
            category_url = query.get("category_url", [""])[0]
            
            try:
                page = int(page_str)
            except ValueError:
                page = 1
            try:
                limit = int(limit_str)
            except ValueError:
                limit = 20
            
            if page < 1:
                page = 1
            if limit < 1 or limit > 50:
                limit = 20
            
            logger.info(f"[SERVER] Catalog request: source={source}, page={page}, limit={limit}, type={type_filter}")
            
            if not source or source not in PARSERS:
                self._send_error(f"Invalid source for catalog. Available: {list(PARSERS.keys())}")
                return
            
            try:
                parser = PARSERS[source]

                # Check if parser has list_catalog method
                if hasattr(parser, 'list_catalog'):
                    catalog_result = parser.list_catalog(page=page, limit=limit, type_filter=type_filter, category_url=category_url)
                    logger.info(f"[SERVER] Catalog: {len(catalog_result.get('items', []))} items from {source}")
                    self._send_json(catalog_result)
                else:
                    # Fallback: scrape the homepage/listing page
                    logger.info(f"[SERVER] Catalog: parser '{source}' has no list_catalog, using fallback scraper")
                    catalog_result = self._scrape_catalog_fallback(parser, source, page, limit)
                    logger.info(f"[SERVER] Catalog fallback: {len(catalog_result.get('items', []))} items from {source}")
                    self._send_json(catalog_result)

            except Exception as e:
                logger.error(f"[SERVER] Catalog error for {source}: {e}", exc_info=True)
                self._send_error(f"Catalog failed: {str(e)}", 500)
            return
        
        # List: /list?source=uzmovi&page=1 (alias for /catalog)
        elif path == "/list":
            # Redirect to catalog handler by rewriting path
            source = query.get("source", [""])[0]
            page_str = query.get("page", ["1"])[0]
            limit_str = query.get("limit", ["20"])[0]
            type_filter = query.get("type", [""])[0]
            
            try:
                page = int(page_str)
            except ValueError:
                page = 1
            try:
                limit = int(limit_str)
            except ValueError:
                limit = 20
            
            if page < 1:
                page = 1
            if limit < 1 or limit > 50:
                limit = 20
            
            logger.info(f"[SERVER] List request (alias for catalog): source={source}, page={page}")
            
            if not source or source not in PARSERS:
                self._send_error(f"Invalid source for list. Available: {list(PARSERS.keys())}")
                return
            
            try:
                parser = PARSERS[source]
                
                if hasattr(parser, 'list_catalog'):
                    catalog_result = parser.list_catalog(page=page, limit=limit, type_filter=type_filter)
                else:
                    catalog_result = self._scrape_catalog_fallback(parser, source, page, limit)
                
                logger.info(f"[SERVER] List: {len(catalog_result.get('items', []))} items from {source}")
                self._send_json(catalog_result)
                    
            except Exception as e:
                logger.error(f"[SERVER] List error for {source}: {e}", exc_info=True)
                self._send_error(f"List failed: {str(e)}", 500)
            return
        
        # List available sources: /sources
        elif path == "/sources":
            self._send_json({
                "sources": [
                    {
                        "name": name,
                        "base_url": parser.base_url,
                        "source_name": parser.source_name
                    }
                    for name, parser in PARSERS.items()
                ] + [
                    {
                        "name": "manual",
                        "base_url": "",
                        "source_name": "Manual Import"
                    }
                ]
            })
            return
        
        # 404
        self._send_error("Not found", 404)
    
    def _scrape_catalog_fallback(self, parser, source, page, limit):
        """
        Fallback catalog scraper for parsers that don't implement list_catalog().
        Scrapes the source website's listing/homepage to extract movie cards.
        """
        import requests as _requests
        from bs4 import BeautifulSoup as _BS
        
        logger.info(f"[CATALOG] Fallback scraper for source={source}, page={page}")
        
        # Source-specific listing URLs
        listing_urls = {
            "uzmovi": f"https://uzmovi.tv/page/{page}/",
            "freekino": f"https://freekino.net/page/{page}/",
            "asilmedia": f"http://asilmedia.org/page/{page}/",
            "kinolar": f"https://kinolar.uz/page/{page}/",
        }
        
        url = listing_urls.get(source)
        if not url:
            logger.warning(f"[CATALOG] No listing URL for source={source}")
            return {"items": [], "page": page, "limit": limit, "total": 0, "total_pages": 0, "has_more": False}
        
        logger.info(f"[CATALOG] Fetching listing page: {url}")
        
        try:
            headers = {
                "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8",
                "Accept-Language": "en-US,en;q=0.5",
            }
            resp = _requests.get(url, headers=headers, timeout=30)
            resp.raise_for_status()
        except Exception as e:
            logger.error(f"[CATALOG] Failed to fetch {url}: {e}")
            return {"items": [], "page": page, "limit": limit, "total": 0, "total_pages": 0, "has_more": False}
        
        soup = _BS(resp.text, "lxml")
        
        # Common card selectors for DLE-based sites
        card_selectors = [
            ".shortstory",
            "article.shortstory",
            ".short-story",
            ".film-item",
            ".movie-item",
            ".movie-card",
            ".content-block",
        ]
        
        cards = []
        for selector in card_selectors:
            cards = soup.select(selector)
            if cards:
                logger.info(f"[CATALOG] Found {len(cards)} cards with selector '{selector}'")
                break
        
        if not cards:
            logger.warning(f"[CATALOG] No movie cards found on {url}")
            # Try broader approach - look for links with images
            logger.info(f"[CATALOG] Trying broader card detection...")
            # Look for common DLE patterns
            main_content = soup.select_one("#dle-content, .content, #content, main, .main")
            if main_content:
                # Find all article-like elements
                cards = main_content.find_all(["article", "div"], class_=True, recursive=False)
                if not cards:
                    cards = main_content.find_all(["article", "div"], recursive=False)
                logger.info(f"[CATALOG] Broader detection found {len(cards)} potential cards")
        
        items = []
        base_url = parser.base_url if hasattr(parser, 'base_url') else url.rsplit('/page/', 1)[0]
        
        for card in cards:
            try:
                item = self._extract_card_item(card, source, base_url)
                if item and item.get("title"):
                    items.append(item)
            except Exception as e:
                logger.debug(f"[CATALOG] Error extracting card: {e}")
                continue
        
        logger.info(f"[CATALOG] Extracted {len(items)} items from {source} page {page}")
        
        # Check for next page
        has_more = False
        # Look for pagination
        pagination = soup.select_one(".navigation, .pagination, .pager, .page-nav, #bottom-nav")
        if pagination:
            next_links = pagination.select("a")
            for link in next_links:
                href = link.get("href", "")
                text = link.get_text(strip=True)
                if "next" in text.lower() or "»" in text or "›" in text or f"/page/{page + 1}" in href:
                    has_more = True
                    break
        
        # If we got items equal to or more than typical page size, assume there's more
        if len(items) >= 10 and not has_more:
            has_more = True
        
        return {
            "items": items[:limit],
            "page": page,
            "limit": limit,
            "total": len(items),
            "total_pages": page + (1 if has_more else 0),
            "has_more": has_more,
        }
    
    def _extract_card_item(self, card, source, base_url):
        """Extract a catalog item from a movie card element."""
        from urllib.parse import urljoin as _urljoin
        import re as _re
        
        # Title extraction
        title = ""
        detail_url = ""
        
        title_selectors = ["h2 a", "h3 a", "h4 a", ".title a", ".film-title a", ".short-title a", ".card-title a", "a[title]"]
        for sel in title_selectors:
            el = card.select_one(sel)
            if el:
                title = el.get_text(strip=True)
                href = el.get("href", "")
                if href:
                    detail_url = _urljoin(base_url, href)
                break
        
        # If no title found from selectors, try first <a> with text
        if not title:
            for a in card.find_all("a"):
                text = a.get_text(strip=True)
                if text and len(text) > 3 and not text.isdigit():
                    title = text
                    href = a.get("href", "")
                    if href:
                        detail_url = _urljoin(base_url, href)
                    break
        
        if not title:
            return None
        
        # Poster extraction
        poster = ""
        img_selectors = ["img[data-src]", "img[data-lazy-src]", "img[data-original]", "img[src]"]
        for sel in img_selectors:
            img = card.select_one(sel)
            if img:
                poster = img.get("data-src") or img.get("data-lazy-src") or img.get("data-original") or img.get("src", "")
                if poster:
                    poster = _urljoin(base_url, poster)
                break
        
        # Year extraction
        year = 0
        year_match = _re.search(r'\b(19|20)\d{2}\b', card.get_text())
        if year_match:
            year = int(year_match.group())
        
        # Source ID extraction from detail_url
        source_id = ""
        if detail_url:
            # Try to extract numeric ID from URL
            id_match = _re.search(r'/(\d+)[-/]', detail_url)
            if id_match:
                source_id = id_match.group(1)
            else:
                # Use URL slug as ID
                parts = detail_url.rstrip('/').split('/')
                if parts:
                    source_id = parts[-1].replace('.html', '')
        
        # Description
        desc = ""
        desc_selectors = [".description", ".desc", ".text", ".short-desc", "p"]
        for sel in desc_selectors:
            el = card.select_one(sel)
            if el:
                desc = el.get_text(strip=True)[:200]
                break
        
        # Genres
        genres = []
        genre_el = card.select_one(".genre, .genres, .category")
        if genre_el:
            genres = [g.strip() for g in genre_el.get_text().split(",") if g.strip()]
        
        return {
            "source_id": source_id,
            "title": title,
            "year": year,
            "type": "movie",
            "poster": poster,
            "description": desc,
            "genres": genres,
            "detail_url": detail_url,
        }
    
    def do_POST(self):
        """Handle POST requests"""
        try:
            self._do_POST_inner()
        except (BrokenPipeError, ConnectionResetError):
            pass  # client disconnected mid-request

    def _do_POST_inner(self):
        parsed = urlparse(self.path)
        path = parsed.path

        logger.info(f"[SERVER] {self.command} {self.path}")
        
        # /download - Download a video
        if path == "/download":
            try:
                body = self._read_json_body()
                
                # DEBUG: Log incoming request body
                logger.info(f"[SERVER] DOWNLOAD REQUEST BODY: {json.dumps(body)}")
                
                source = body.get("source", "")
                source_id = body.get("id", "")
                detail_url = body.get("url", "")
                output_name = body.get("output_name", "")
                referer = body.get("referer", "")
                
                # DEBUG: Log received job_id
                backend_job_id = body.get("job_id", "")
                logger.info(f"[SERVER] JOB_ID from request: '{backend_job_id}' (type: {type(backend_job_id).__name__})")
                logger.info(f"[SERVER] BACKEND_URL configured: '{BACKEND_URL}'")
                
                # Validate required fields
                if not source or (source not in PARSERS and source != "manual"):
                    self._send_error(f"Invalid source. Available: {AVAILABLE_SOURCES}")
                    return
                
                # Manual source: direct video URL download
                if source == "manual":
                    video_url = body.get("video_url", "") or body.get("url", "")
                    if not video_url:
                        self._send_error("Manual source requires 'video_url' parameter")
                        return
                    
                    title = body.get("title", "manual_import")
                    import re as _re
                    output_name = output_name or _re.sub(r'[^\w\s-]', '', title)
                    output_name = _re.sub(r'[-\s]+', '_', output_name)
                    if not output_name.endswith((".mp4", ".m3u8", ".mkv")):
                        output_name += ".mp4"
                    
                    # Detect type
                    url_type = "mp4"
                    if ".m3u8" in video_url.lower():
                        url_type = "m3u8"
                    elif ".mpd" in video_url.lower():
                        url_type = "mpd"
                    
                    try:
                        result = downloader_service.smart_download(
                            url=video_url,
                            output_name=output_name,
                            job_id=output_name,
                            backend_job_id=backend_job_id,
                            referer=referer if referer else None,
                        )
                        
                        self._send_json({
                            "success": True,
                            "source": "manual",
                            "selected_media": {"url": video_url, "type": url_type},
                            "download": {
                                "file_path": result.get("file_path", ""),
                                "file_name": result.get("file_name", ""),
                                "type": result.get("type", url_type),
                            },
                        })
                    except Exception as e:
                        self._send_json({
                            "success": False,
                            "error": f"Manual download failed: {str(e)}",
                            "source": "manual",
                        }, 500)
                    return
                
                if not source_id and not detail_url:
                    self._send_error("Missing 'id' or 'url' parameter")
                    return
                
                if not output_name:
                    # Generate default output name from source_id or URL
                    if source_id:
                        output_name = f"{source}_{source_id}.mp4"
                    else:
                        import hashlib
                        url_hash = hashlib.md5(detail_url.encode()).hexdigest()[:8]
                        output_name = f"{source}_{url_hash}.mp4"
                
                # Ensure output_name has proper extension
                if not output_name.endswith((".mp4", ".m3u8", ".mkv")):
                    output_name += ".mp4"
                
                logger.info(f"[SERVER] Download request: source={source}, id={source_id}, url={detail_url[:50] if detail_url else 'none'}...")
                
                # Get parser and fetch details
                parser = PARSERS[source]
                details = self._get_details_from_parser(parser, source, source_id, detail_url)
                
                # Get details as dict for response
                if hasattr(details, 'to_dict'):
                    details_dict = details.to_dict()
                elif isinstance(details, dict):
                    details_dict = details
                else:
                    details_dict = {}
                
                # Extract best video URL
                video_url, url_type = self._extract_best_video_url(details, source)
                
                if not video_url:
                    logger.error(f"[SERVER] No playable video URL found for source '{source}'")
                    self._send_json({
                        "success": False,
                        "error": "No playable video URL found",
                        "source": source,
                        "details": details_dict,
                    }, 400)
                    return
                
                logger.info(f"[SERVER] Parser selected media URL: type={url_type}, url={video_url[:80]}...")
                
                # Use referer from details if not provided
                if not referer:
                    referer = details_dict.get("video_page_url", "")
                
                # Download using downloader service
                try:
                    logger.info(f"[SERVER] Parser download started: {output_name}")
                    
                    # Get job_id from request body (backend job ID) or generate one
                    # If provided, we'll report progress to backend
                    backend_job_id = body.get("job_id", "")
                    
                    logger.info(f"[SERVER] Download request - source={source}, output_name={output_name}, backend_job_id='{backend_job_id}'")
                    
                    # DEBUG: Log the progress endpoint URL that will be called
                    if backend_job_id and BACKEND_URL:
                        progress_url = f"{BACKEND_URL}/api/ingestion/jobs/{backend_job_id}/progress"
                        logger.info(f"[SERVER] Progress will be reported to: {progress_url}")
                    
                    # Generate internal job_id for progress tracking
                    import hashlib
                    internal_job_id = hashlib.md5(output_name.encode()).hexdigest()[:12]
                    
                    # Start progress reporting thread BEFORE download starts
                    # This is critical - it must run IN PARALLEL with download
                    progress_thread = None
                    if backend_job_id:
                        import threading
                        logger.info(f"[SERVER] Starting progress reporting thread for job_id={backend_job_id}")

                        # CRITICAL: Use the SAME job_id as the downloader service
                        # The downloader uses internal_job_id for progress tracking
                        progress_job_id = internal_job_id

                        # Send initial "starting" progress immediately
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "downloading",
                            "progress_percent": 0,
                            "message": "Starting download...",
                        })

                        def report_progress():
                            last_pct = -1
                            last_bytes = 0
                            while True:
                                prog = downloader_service.progress.get(progress_job_id)
                                if prog:
                                    pct = prog.get("progress_percent", 0)
                                    downloaded = prog.get("downloaded_bytes", 0)
                                    # Always report if bytes changed, even if percent is same
                                    if pct != last_pct or downloaded != last_bytes:
                                        last_pct = pct
                                        last_bytes = downloaded
                                        logger.info(f"[SERVER] Progress update: job_id={backend_job_id}, progress={pct}%, downloaded={downloaded} bytes")
                                        # Map to backend format
                                        self._report_progress_to_backend(backend_job_id, {
                                            "stage": prog.get("stage", "download"),
                                            "status": prog.get("status", "downloading"),
                                            "progress_percent": pct,
                                            "downloaded_bytes": downloaded,
                                            "total_bytes": prog.get("total_bytes", 0),
                                            "speed_mbps": prog.get("speed_mb_per_sec", 0.0),
                                            "eta_seconds": prog.get("eta_seconds", 0),
                                            "message": prog.get("message", ""),
                                        })
                                # Check for download completion
                                if prog and prog.get("status") in ["completed", "failed"]:
                                    logger.info(f"[SERVER] Download finished, stopping progress thread")
                                    break
                                time.sleep(1)
                        
                        progress_thread = threading.Thread(target=report_progress, daemon=True)
                        progress_thread.start()
                    else:
                        logger.warning(f"[SERVER] No backend_job_id provided, progress will not be reported!")
                    
                    # Now start the actual download (this is blocking)
                    logger.info(f"[SERVER] Starting download: {output_name}")
                    result = downloader_service.smart_download(
                        url=video_url,
                        output_name=output_name,
                        job_id=internal_job_id,
                        backend_job_id=backend_job_id,
                        referer=referer if referer else None,
                    )
                    
                    logger.info(f"[PARSER] Parallel download completed")
                    logger.info(f"[PARSER] Merged parts into {result['file_path']}")
                    logger.info(f"[PARSER] Removed temporary part files")
                    logger.info(f"[PARSER] Updated backend: steps.download=true, stage=process")
                    
                    # Final progress update
                    # CRITICAL: Set steps_download=true to trigger backend state transition
                    # Also include file_path so backend can store local_path
                    if backend_job_id:
                        time.sleep(1)
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "process",
                            "status": "processing",
                            "progress_percent": 100,
                            "steps_download": True,
                            "file_path": result["file_path"],
                            "message": "Download completed, starting processing",
                        })
                    
                    logger.info(f"[SERVER] Parser download completed: {result['file_path']}")
                    
                    # Build response with structured format
                    response_data = {
                        "success": True,
                        "source": source,
                        "details": details_dict,
                        "selected_media": {
                            "url": video_url,
                            "type": url_type,
                        },
                        "download": {
                            "file_path": result["file_path"],
                            "file_name": result["file_name"],
                            "type": result["type"],
                        },
                        "worker_called": False,
                        "worker_success": False,
                    }
                    
                    # Optionally call worker to process video (add logo, cut, etc.)
                    call_worker = body.get("call_worker", False)
                    if call_worker:
                        logger.info(f"[SERVER] Calling worker to process video...")
                        
                        # Send initial worker progress update
                        if backend_job_id:
                            self._report_progress_to_backend(backend_job_id, {
                                "stage": "process",
                                "status": "processing",
                                "progress": 40,
                                "message": "Starting video processing...",
                            })
                        
                        # Start background progress reporter thread
                        stop_progress_thread = threading.Event()
                        progress_thread = None
                        if backend_job_id:
                            def report_worker_progress():
                                stages = [
                                    (45, "process", "Preparing video..."),
                                    (55, "process", "Adding watermark..."),
                                    (65, "process", "FFmpeg processing..."),
                                    (75, "process", "Generating HLS..."),
                                    (85, "process", "Extracting poster..."),
                                    (95, "process", "Finalizing..."),
                                ]
                                for progress, stage, message in stages:
                                    if stop_progress_thread.is_set():
                                        break
                                    time.sleep(5)
                                    if stop_progress_thread.is_set():
                                        break
                                    self._report_progress_to_backend(backend_job_id, {
                                        "stage": stage,
                                        "status": "processing",
                                        "progress": progress,
                                        "message": message,
                                    })
                            
                            progress_thread = threading.Thread(target=report_worker_progress, daemon=True)
                            progress_thread.start()
                        
                        try:
                            worker_result = self._call_worker(
                                input_file=result["file_path"],
                                title=details_dict.get("title", output_name),
                                cut_seconds=body.get("cut_seconds", 0),
                            )
                            
                            if worker_result.get("success"):
                                response_data["worker_success"] = True
                                response_data["worker_result"] = worker_result
                                response_data["output_file"] = worker_result.get("output_file")
                                logger.info(f"[SERVER] Worker processing successful: {worker_result.get('output_file')}")
                                
                                # Send final success progress
                                if backend_job_id:
                                    self._report_progress_to_backend(backend_job_id, {
                                        "stage": "complete",
                                        "status": "completed",
                                        "progress": 100,
                                        "message": "Processing complete",
                                    })
                            else:
                                response_data["worker_error"] = worker_result.get("error", "Unknown error")
                                logger.error(f"[SERVER] Worker processing failed: {response_data['worker_error']}")
                                
                                # Send error progress
                                if backend_job_id:
                                    self._report_progress_to_backend(backend_job_id, {
                                        "stage": "process",
                                        "status": "failed",
                                        "progress": 35,
                                        "message": f"Processing failed: {response_data['worker_error']}",
                                    })
                            
                            response_data["worker_called"] = True
                            
                        except Exception as worker_error:
                            logger.error(f"[SERVER] Worker call failed: {worker_error}")
                            response_data["worker_error"] = str(worker_error)
                            # Don't fail the whole request if worker fails
                            
                            # Send error progress
                            if backend_job_id:
                                self._report_progress_to_backend(backend_job_id, {
                                    "stage": "process",
                                    "status": "failed",
                                    "progress": 35,
                                    "message": f"Processing error: {str(worker_error)}",
                                })
                        finally:
                            # Stop the progress thread
                            stop_progress_thread.set()
                            if progress_thread:
                                progress_thread.join(timeout=1)
                    
                    self._send_json(response_data)
                    
                except Exception as download_error:
                    logger.error(f"[SERVER] Download failed: {download_error}")
                    self._send_json({
                        "success": False,
                        "error": f"Download failed: {str(download_error)}",
                        "source": source,
                        "details": details_dict,
                        "selected_media": {
                            "url": video_url,
                            "type": url_type,
                        },
                    }, 500)
                
            except json.JSONDecodeError as e:
                logger.error(f"[SERVER] Invalid JSON in request body: {e}")
                self._send_error(f"Invalid JSON in request body: {str(e)}", 400)
            except Exception as e:
                logger.error(f"[SERVER] Download endpoint error: {e}", exc_info=True)
                self._send_error(f"Download failed: {str(e)}", 500)
            return
        
        # /instagram/upload — upload a video clip as an Instagram Reel
        elif path == "/instagram/upload":
            try:
                body = self._read_json_body()
                username = body.get("username", "")
                password = body.get("password", "")
                video_url = body.get("video_url", "")
                caption = body.get("caption", "")

                if not username or not password or not video_url:
                    self._send_error("username, password and video_url are required", 400)
                    return

                logger.info(f"[Instagram] upload requested for account={username} url={video_url}")

                try:
                    from instagrapi import Client
                except ImportError:
                    self._send_error("instagrapi not installed — run: pip install instagrapi", 500)
                    return

                import tempfile, urllib.request as _urlreq
                from pathlib import Path

                account_name = body.get("account_name", "") or username
                session_dir = Path(__file__).parent / "ig_sessions"
                session_dir.mkdir(exist_ok=True)
                session_file = session_dir / f"{account_name}.json"

                tmp_path = None
                try:
                    cl = Client()
                    cl.delay_range = [1, 3]

                    # Auth error types that require re-login — password fallback won't help
                    _AUTH_FAIL_TYPES = {"challenge_required", "checkpoint_required", "session_expired"}

                    def _classify_ig_error(err_str):
                        e = err_str.lower()
                        if "challenge_required" in e or "feedback_required" in e:
                            return "challenge_required"
                        if "checkpoint_required" in e:
                            return "checkpoint_required"
                        if ("email" in e or "phone" in e) and ("send" in e or "verify" in e or "help" in e or "confirm" in e):
                            return "checkpoint_required"
                        if "account recovery" in e or "account has been compromised" in e:
                            return "checkpoint_required"
                        if "login_required" in e or "not_authenticated" in e or "not authenticated" in e or "login required" in e:
                            return "session_expired"
                        if "bad_password" in e or ("password" in e and ("incorrect" in e or "wrong" in e)):
                            return "bad_credentials"
                        if "please wait" in e or "too many" in e or "spam" in e:
                            return "rate_limited"
                        return "publish_failed"

                    if session_file.exists():
                        cl.load_settings(session_file)
                        logger.info(f"[Instagram] loaded session for account={account_name}")
                        # Pre-check: verify session is valid before attempting upload
                        try:
                            cl.get_timeline_feed()
                        except Exception as session_err:
                            session_err_type = _classify_ig_error(str(session_err))
                            if session_err_type in _AUTH_FAIL_TYPES:
                                # Auth error — password fallback cannot fix this; fail immediately
                                logger.error(f"[Instagram] session auth error ({session_err_type}) for account={account_name}, re-login required")
                                raise
                            # Non-auth failure (network, unknown) — try password fallback
                            if password:
                                logger.warning(f"[Instagram] session check failed ({session_err}), retrying with password")
                                cl = Client()
                                cl.delay_range = [1, 3]
                                cl.login(username, password)
                                cl.dump_settings(session_file)
                                logger.info(f"[Instagram] re-authenticated for account={account_name}")
                            else:
                                raise
                    else:
                        if not password:
                            raise Exception(f"no_session: No session file for account={account_name}. Run ig_login.py first.")
                        logger.warning(f"[Instagram] no session for account={account_name}, logging in with password (may trigger challenge)")
                        cl.login(username, password)
                        cl.dump_settings(session_file)
                        logger.info(f"[Instagram] session saved for account={account_name}")

                    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
                        tmp_path = f.name
                    _urlreq.urlretrieve(video_url, tmp_path)

                    media = cl.clip_upload(Path(tmp_path), caption)
                    logger.info(f"[Instagram] upload success media_id={media.pk} account={account_name}")
                    self._send_json({"status": "success", "media_id": str(media.pk)})
                except Exception as e:
                    error_str = str(e)
                    error_type = _classify_ig_error(error_str)
                    # Refine: no_session prefix set explicitly above
                    if "no_session:" in error_str.lower():
                        error_type = "no_session"
                    _action_map = {
                        "challenge_required":  "ig_login.py orqali sessiyani yangilang",
                        "checkpoint_required": "ig_login.py orqali sessiyani yangilang",
                        "session_expired":     "ig_login.py orqali qayta login qiling",
                        "no_session":          "ig_login.py orqali birinchi marta login qiling",
                        "bad_credentials":     "Login va parolni tekshiring",
                        "rate_limited":        "Bir necha soatdan keyin urinib ko'ring",
                        "publish_failed":      "Qayta urinib ko'ring",
                    }
                    action_required = _action_map.get(error_type, "Qayta urinib ko'ring")
                    logger.error(f"[Instagram] error account={account_name} type={error_type}: {error_str}",
                                 exc_info=(error_type == "publish_failed"))
                    self._send_json({
                        "status": "failed",
                        "error": error_str,
                        "error_type": error_type,
                        "action_required": action_required,
                    })
                finally:
                    if tmp_path and os.path.exists(tmp_path):
                        os.unlink(tmp_path)
            except Exception as e:
                logger.error(f"[Instagram] endpoint error: {e}", exc_info=True)
                self._send_error(str(e), 500)
            return

        # /youtube/upload — upload a video clip as a YouTube Short
        elif path == "/youtube/upload":
            try:
                body = self._read_json_body()
                token_file = body.get("token_file", "")
                video_url = body.get("video_url", "")
                title = body.get("title", "")
                description = body.get("description", "")
                account_name = body.get("account_name", "") or token_file

                if not token_file or not video_url:
                    self._send_error("token_file and video_url are required", 400)
                    return

                logger.info(f"[YouTube] upload requested for account={account_name} url={video_url}")

                try:
                    from google.oauth2.credentials import Credentials
                    from google.auth.transport.requests import Request
                    from googleapiclient.discovery import build
                    from googleapiclient.http import MediaFileUpload
                except ImportError as _ie:
                    import sys as _sys
                    self._send_error(
                        f"Missing YouTube dependency: {_ie}. "
                        f"Run: {_sys.executable} -m pip install google-api-python-client google-auth google-auth-oauthlib google-auth-httplib2",
                        500,
                    )
                    return

                import tempfile, json as _json
                from pathlib import Path

                token_path = Path(__file__).parent / token_file
                if not token_path.exists():
                    self._send_error(f"token_file not found: {token_file}", 400)
                    return

                creds_data = _json.loads(token_path.read_text())
                creds = Credentials(
                    token=creds_data.get("token"),
                    refresh_token=creds_data.get("refresh_token"),
                    token_uri=creds_data.get("token_uri", "https://oauth2.googleapis.com/token"),
                    client_id=creds_data.get("client_id"),
                    client_secret=creds_data.get("client_secret"),
                )
                if creds.expired and creds.refresh_token:
                    creds.refresh(Request())
                    creds_data["token"] = creds.token
                    token_path.write_text(_json.dumps(creds_data))
                    logger.info(f"[YouTube] token refreshed for account={account_name}")

                youtube = build("youtube", "v3", credentials=creds)

                tmp_path = None
                try:
                    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
                        tmp_path = f.name
                    logger.info(f"[YouTube] downloading video to {tmp_path}")
                    urllib.request.urlretrieve(video_url, tmp_path)

                    yt_body = {
                        "snippet": {
                            "title": (title or "Clip")[:100],
                            "description": description or "",
                            "tags": [],
                            "categoryId": "22",  # People & Blogs
                        },
                        "status": {
                            "privacyStatus": "public",
                            "selfDeclaredMadeForKids": False,
                        },
                    }
                    media = MediaFileUpload(tmp_path, mimetype="video/mp4", resumable=True)
                    request = youtube.videos().insert(
                        part="snippet,status",
                        body=yt_body,
                        media_body=media,
                    )
                    response = None
                    while response is None:
                        _, response = request.next_chunk()

                    video_id = response.get("id", "")
                    logger.info(f"[YouTube] upload success video_id={video_id} account={account_name}")
                    self._send_json({"status": "success", "video_id": video_id, "account": account_name, "platform": "youtube"})
                except Exception as e:
                    logger.error(f"[YouTube] upload error account={account_name}: {e}", exc_info=True)
                    self._send_json({"status": "failed", "error": str(e)})
                finally:
                    try:
                        if tmp_path and os.path.exists(tmp_path):
                            os.unlink(tmp_path)
                    except Exception as _cleanup_err:
                        logger.warning(f"[YouTube] temp file cleanup failed: {_cleanup_err}")
            except Exception as e:
                logger.error(f"[YouTube] endpoint error: {e}", exc_info=True)
                self._send_error(str(e), 500)
            return

        # /tiktok/upload — upload a video clip to TikTok using Content Posting API v2
        elif path == "/tiktok/upload":
            try:
                body = self._read_json_body()
                token_file = body.get("token_file", "")
                video_url = body.get("video_url", "")
                caption = body.get("caption", "")
                account_name = body.get("account_name", "")

                if not token_file or not video_url:
                    self._send_error("token_file and video_url are required", 400)
                    return

                logger.info(f"[TikTok] upload requested for account={account_name} url={video_url}")

                try:
                    import urllib.request as _urlreq2
                    import json as _json2
                    from pathlib import Path as _Path2

                    token_path = _Path2(__file__).parent / token_file
                    if not token_path.exists():
                        self._send_json({"status": "failed", "error": f"token_file not found: {token_file}. Run tt_login.py first."})
                        return

                    token_data = _json2.loads(token_path.read_text())
                    access_token = token_data.get("access_token", "")
                    refresh_token = token_data.get("refresh_token", "")
                    client_key = token_data.get("client_key", "")
                    client_secret = token_data.get("client_secret", "")

                    def _tt_refresh():
                        """Refresh TikTok access token using refresh_token."""
                        payload = {
                            "client_key": client_key,
                            "client_secret": client_secret,
                            "grant_type": "refresh_token",
                            "refresh_token": refresh_token,
                        }
                        data = urllib.parse.urlencode(payload).encode("utf-8")
                        req = _urlreq2.Request(
                            "https://open.tiktokapis.com/v2/oauth/token/",
                            data=data,
                            headers={"Content-Type": "application/x-www-form-urlencoded"},
                            method="POST",
                        )
                        with _urlreq2.urlopen(req, timeout=15) as r:
                            resp_data = _json2.loads(r.read().decode("utf-8"))
                        return resp_data.get("data", {})

                    def _tt_post(token):
                        headers = {
                            "Authorization": f"Bearer {token}",
                            "Content-Type": "application/json; charset=UTF-8",
                        }
                        post_info = {
                            "title": (caption or "")[:150],
                            "privacy_level": "PUBLIC_TO_EVERYONE",
                            "disable_duet": False,
                            "disable_comment": False,
                            "disable_stitch": False,
                            "video_cover_timestamp_ms": 1000,
                        }
                        source_info = {"source": "PULL_FROM_URL", "video_url": video_url}
                        payload = _json2.dumps({"post_info": post_info, "source_info": source_info}).encode("utf-8")
                        req = _urlreq2.Request(
                            "https://open.tiktokapis.com/v2/post/publish/video/init/",
                            data=payload,
                            headers=headers,
                            method="POST",
                        )
                        with _urlreq2.urlopen(req, timeout=30) as r:
                            return _json2.loads(r.read().decode("utf-8"))

                    # Attempt upload; if token expired, refresh once and retry
                    init_data = _tt_post(access_token)
                    err_info = init_data.get("error", {})
                    err_code = err_info.get("code", "ok")

                    if err_code in ("access_token_invalid", "access_token_expired") and refresh_token and client_key:
                        logger.warning(f"[TikTok] token expired for account={account_name}, refreshing...")
                        new_tokens = _tt_refresh()
                        if new_tokens.get("access_token"):
                            access_token = new_tokens["access_token"]
                            token_data["access_token"] = access_token
                            if new_tokens.get("refresh_token"):
                                token_data["refresh_token"] = new_tokens["refresh_token"]
                            token_path.write_text(_json2.dumps(token_data, indent=2))
                            logger.info(f"[TikTok] token refreshed for account={account_name}")
                            init_data = _tt_post(access_token)
                            err_info = init_data.get("error", {})
                            err_code = err_info.get("code", "ok")

                    if err_code != "ok":
                        msg = err_info.get("message", "unknown error")
                        logger.error(f"[TikTok] init error account={account_name}: code={err_code} msg={msg}")
                        self._send_json({"status": "failed", "error": f"{err_code}: {msg}"})
                        return

                    publish_id = init_data.get("data", {}).get("publish_id", "")
                    logger.info(f"[TikTok] upload initiated publish_id={publish_id} account={account_name}")
                    self._send_json({"status": "success", "video_id": publish_id})

                except Exception as e:
                    logger.error(f"[TikTok] upload error account={account_name}: {e}", exc_info=True)
                    self._send_json({"status": "failed", "error": str(e)})
            except Exception as e:
                logger.error(f"[TikTok] endpoint error: {e}", exc_info=True)
                self._send_error(str(e), 500)
            return

        elif path == "/clip/analyze":
            try:
                import os as _os, subprocess as _sp, tempfile as _tmp
                from pathlib import Path as _Path

                body = self._read_json_body()
                video_path = body.get("video_path", "")
                if not video_path or not _os.path.exists(video_path):
                    self._send_error("video_path not found", 400)
                    return

                result = {"speech_segments": [], "face_frames": []}

                # ── Whisper speech analysis (full video, chunked) ─────────────
                tmp_audio = None
                try:
                    import whisper as _whisper
                    tmp_audio = _tmp.mktemp(suffix=".wav")
                    # Extract full audio — no duration limit
                    _sp.run(
                        ["ffmpeg", "-y", "-i", video_path, "-ac", "1", "-ar", "16000", tmp_audio],
                        capture_output=True,
                    )
                    model = _whisper.load_model("base")
                    wresult = model.transcribe(tmp_audio, fp16=False, verbose=False)
                    result["speech_segments"] = [
                        {"start": s["start"], "end": s["end"], "text": s["text"].strip()}
                        for s in wresult.get("segments", [])
                    ]
                    logger.info(f"[CLIP-AI] speech segments: {len(result['speech_segments'])}")
                except ImportError:
                    logger.warning("[CLIP-AI] whisper not installed — skipping speech analysis")
                except Exception as e:
                    logger.warning(f"[CLIP-AI] whisper failed (skipping): {e}")
                finally:
                    if tmp_audio and _os.path.exists(tmp_audio):
                        _os.unlink(tmp_audio)

                # ── Face detection (opencv haarcascade, 0.75s sampling) ───────
                try:
                    import cv2 as _cv2
                    cap = _cv2.VideoCapture(video_path)
                    fps = cap.get(_cv2.CAP_PROP_FPS) or 25.0
                    # Sample every ~0.75s for better temporal resolution
                    sample_interval = max(1, int(fps * 0.75))
                    face_cascade = _cv2.CascadeClassifier(
                        _cv2.data.haarcascades + "haarcascade_frontalface_default.xml"
                    )
                    face_frames = []
                    frame_idx = 0
                    while True:
                        ret, frame = cap.read()
                        if not ret:
                            break
                        if frame_idx % sample_interval == 0:
                            t = frame_idx / fps
                            fh_img, fw_img = frame.shape[:2]
                            gray = _cv2.cvtColor(frame, _cv2.COLOR_BGR2GRAY)
                            faces = face_cascade.detectMultiScale(gray, 1.1, 4, minSize=(30, 30))
                            max_size = 0.0
                            cx = 0.5  # default center
                            cy = 0.5
                            if len(faces) > 0:
                                # Pick largest face for size/center metrics
                                largest = max(faces, key=lambda f: f[2] * f[3])
                                x, y, fw, fh = largest
                                max_size = (fw * fh) / (fw_img * fh_img)
                                cx = (x + fw / 2) / fw_img
                                cy = (y + fh / 2) / fh_img
                            face_frames.append({
                                "time": t,
                                "face_count": int(len(faces)),
                                "max_size": max_size,
                                "cx": cx,
                                "cy": cy,
                            })
                        frame_idx += 1
                    cap.release()
                    result["face_frames"] = face_frames
                    logger.info(f"[CLIP-AI] faces detected: {len(face_frames)} frames sampled")
                except ImportError:
                    logger.warning("[CLIP-AI] opencv not installed — skipping face detection")
                except Exception as e:
                    logger.warning(f"[CLIP-AI] face detection failed (skipping): {e}")

                self._send_json(result)
            except Exception as e:
                logger.error(f"[CLIP-AI] analyze endpoint error: {e}", exc_info=True)
                self._send_error(str(e), 500)
            return

        # 404 for unknown POST endpoints
        self._send_error("Not found", 404)
    
    def log_message(self, format, *args):
        """Custom log format"""
        logger.info(f"{self.address_string()} - {format % args}")


def run_server(host="0.0.0.0", port=8082):
    """Run the parser API server"""
    # IMPORTANT: Validate BACKEND_URL is set for progress reporting
    # Without this, admin UI won't receive download progress updates
    if not BACKEND_URL:
        logger.warning("=" * 60)
        logger.warning("WARNING: BACKEND_URL is not set!")
        logger.warning("Progress reporting to admin UI will be DISABLED.")
        logger.warning("Set BACKEND_URL in environment or parser/.env file.")
        logger.warning("Example: BACKEND_URL=http://127.0.0.1:8080")
        logger.warning("=" * 60)
    else:
        logger.info(f"Backend URL configured: {BACKEND_URL}")
    
    server_address = (host, port)
    # Set the server address string for use in handlers
    ParserHandler.server_address_str = f"http://{host}:{port}"
    httpd = ThreadedHTTPServer(server_address, ParserHandler)
    logger.info(f"Parser API server running on http://{host}:{port}")
    logger.info(f"Available sources: {AVAILABLE_SOURCES}")
    logger.info(f"Parser base URL for workers: {ParserHandler.server_address_str}")
    
    try:
        httpd.serve_forever()
    except KeyboardInterrupt:
        logger.info("Shutting down server...")
        httpd.shutdown()


if __name__ == "__main__":
    import argparse

    _env_host = os.environ.get("PARSER_HOST", "0.0.0.0")
    _env_port = int(os.environ.get("PARSER_PORT", "8082"))

    _arg_parser = argparse.ArgumentParser(description="Parser API Server")
    _arg_parser.add_argument("--host", default=_env_host, help="Host to bind to")
    _arg_parser.add_argument("--port", type=int, default=_env_port, help="Port to bind to")

    args = _arg_parser.parse_args()

    logger.info(f"[STARTUP] BACKEND_URL  = {os.environ.get('BACKEND_URL', '(not set)')}")
    logger.info(f"[STARTUP] PARSER_HOST  = {os.environ.get('PARSER_HOST', '(not set)')}")
    logger.info(f"[STARTUP] PARSER_PORT  = {os.environ.get('PARSER_PORT', '(not set)')}")
    logger.info(f"[STARTUP] Binding to   = {args.host}:{args.port}")

    run_server(args.host, args.port)







