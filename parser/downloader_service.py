import os
import re
import json
import subprocess
import threading
import logging
import time
import urllib.request
import urllib.error
from urllib.parse import urlparse
import tempfile
import shutil

import requests

# Load environment variables from .env file (parser directory)
from dotenv import load_dotenv
load_dotenv(os.path.join(os.path.dirname(__file__), '.env'))

# Import validation function for URL validation
from helpers import isValidStreamUrl, is_youtube_url

logger = logging.getLogger(__name__)

# DDownloader Integration
# Use DDownloader for HLS/DASH/ISM streams (N_m3u8DL-RE) and aria2c for MP4
USE_DDOWNLOADER = os.environ.get("USE_DDOWNLOADER", "true").lower() == "true"

# Try to import DDownloader integration if available
DDdownloaderIntegration = None
if USE_DDOWNLOADER:
    try:
        from ddownloader_integration import DDownloaderIntegration
        DDdownloaderIntegration = DDownloaderIntegration
        logger.info("[CONFIG] DDownloader integration enabled")
    except ImportError as e:
        logger.warning(f"[CONFIG] DDownloader integration not available: {e}")
        DDdownloaderIntegration = None

# Create a persistent session for better connection reuse
http_session = requests.Session()
# Configure connection pool
adapter = requests.adapters.HTTPAdapter(
    pool_connections=10,
    pool_maxsize=10,
    max_retries=3
)
http_session.mount('http://', adapter)
http_session.mount('https://', adapter)

# Parallel download configuration
PARALLEL_THREADS = 8
CHUNK_SIZE = 512 * 1024  # 512KB chunks for writes

# Thread-safe progress tracking
progress_lock = threading.Lock()
g_downloaded_bytes = 0
g_total_bytes = 0
g_start_time = 0


def _check_range_support(url: str, headers: dict) -> tuple[bool, int]:
    """
    Check if server supports HTTP Range requests.
    Returns (supports_range, total_size)
    """
    try:
        # Send a HEAD request to check range support
        response = http_session.head(url, headers=headers, timeout=10, allow_redirects=True)
        accept_ranges = response.headers.get('Accept-Ranges', '').lower()
        content_length = int(response.headers.get('Content-Length', 0))
        
        supports_range = accept_ranges == 'bytes'
        logger.info(f"[DOWNLOAD] Range support: {supports_range}, total size: {content_length}")
        return supports_range, content_length
    except Exception as e:
        logger.warning(f"[DOWNLOAD] Could not check range support: {e}")
        return False, 0


def _download_part(url: str, headers: dict, start_byte: int, end_byte: int, part_path: str, thread_id: int) -> bool:
    """
    Download a part of the file using HTTP Range header.
    Returns True if successful.
    """
    range_headers = headers.copy()
    range_headers['Range'] = f'bytes={start_byte}-{end_byte}'
    
    try:
        logger.info(f"[THREAD-{thread_id}] Downloading range {start_byte}-{end_byte}")
        response = http_session.get(url, headers=range_headers, stream=True, timeout=60, allow_redirects=True)
        response.raise_for_status()
        
        with open(part_path, 'wb') as f:
            for chunk in response.iter_content(chunk_size=CHUNK_SIZE):
                if chunk:
                    f.write(chunk)
        
        logger.info(f"[THREAD-{thread_id}] Completed: {part_path}")
        return True
        
    except Exception as e:
        logger.error(f"[THREAD-{thread_id}] Failed: {e}")
        return False


def _parallel_download_worker(url: str, headers: dict, part_ranges: list, temp_dir: str, 
                               num_threads: int, job_id: str, backend_job_id: str, 
                               progress_callback) -> bool:
    """
    Worker function that downloads file parts in parallel threads.
    Returns True if all parts downloaded successfully.
    """
    global g_downloaded_bytes, g_total_bytes, g_start_time
    
    threads = []
    part_files = []
    
    # Create part files and start threads
    for i, (start, end) in enumerate(part_ranges):
        part_path = os.path.join(temp_dir, f'part_{i}.tmp')
        part_files.append(part_path)
        
        t = threading.Thread(
            target=_download_part,
            args=(url, headers, start, end, part_path, i)
        )
        threads.append(t)
    
    # Start all threads
    for t in threads:
        t.start()
    
    # Monitor progress while threads are running
    last_update_time = time.time()
    last_reported_progress = -1

    while any(t.is_alive() for t in threads):
        # Calculate total downloaded bytes from part files
        total_downloaded = 0
        for part_file in part_files:
            if os.path.exists(part_file):
                total_downloaded += os.path.getsize(part_file)

        with progress_lock:
            g_downloaded_bytes = total_downloaded

        # Report when +1% gained OR 2s elapsed (avoid per-chunk DB writes)
        current_time = time.time()
        elapsed = current_time - g_start_time
        if elapsed > 0 and g_total_bytes > 0:
            speed = g_downloaded_bytes / elapsed
            eta = int((g_total_bytes - g_downloaded_bytes) / speed) if speed > 0 else 0
            progress = int((g_downloaded_bytes / g_total_bytes) * 100)

            if progress > last_reported_progress or current_time - last_update_time >= 2.0:
                if progress_callback:
                    progress_callback(progress, g_downloaded_bytes, g_total_bytes, speed, eta)
                last_reported_progress = progress
                last_update_time = current_time
        
        time.sleep(0.1)
    
    # Wait for all threads to complete
    for t in threads:
        t.join()
    
    # Verify all parts exist
    for part_file in part_files:
        if not os.path.exists(part_file):
            logger.error(f"[DOWNLOAD] Missing part file: {part_file}")
            return False
    
    return True


def _merge_parts(part_files: list, output_path: str) -> bool:
    """Merge all part files into final output file."""
    try:
        with open(output_path, 'wb') as outfile:
            for part_file in part_files:
                with open(part_file, 'rb') as infile:
                    outfile.write(infile.read())
        
        # Clean up part files after successful merge
        logger.info(f"[DOWNLOAD] Cleaning up {len(part_files)} temporary part files")
        for part_file in part_files:
            try:
                os.remove(part_file)
                logger.info(f"[DOWNLOAD] Removed temp file: {part_file}")
            except Exception as e:
                logger.warning(f"[DOWNLOAD] Failed to remove temp file {part_file}: {e}")
        
        return True
    except Exception as e:
        logger.error(f"[DOWNLOAD] Failed to merge parts: {e}")
        return False

# Backend URL for progress updates
BACKEND_URL = os.environ.get("BACKEND_URL", "")

# Log BACKEND_URL at startup
logger.info(f"[CONFIG] BACKEND_URL = '{BACKEND_URL}'")


def report_progress_to_backend(job_id: str, progress_data: dict):
    """Send progress update to backend API - best-effort only, retries on failure"""
    import time
    
    logger.info(f"[PROGRESS] report_progress_to_backend called with job_id='{job_id}'")
    logger.info(f"[PROGRESS] BACKEND_URL='{BACKEND_URL}'")
    logger.info(f"[PROGRESS] progress_data: {progress_data}")
    
    if not job_id:
        logger.warning("[PROGRESS] ERROR: No job_id provided, skipping backend update")
        return
    if not BACKEND_URL:
        logger.warning("[PROGRESS] ERROR: BACKEND_URL is empty! Set BACKEND_URL=http://127.0.0.1:8080")
        return
    
    endpoint = f"{BACKEND_URL}/api/ingestion/jobs/{job_id}/progress"
    logger.info(f"[PROGRESS] POST {endpoint}")
    logger.info(f"[PROGRESS] PAYLOAD: {json.dumps(progress_data)}")
    
    # Best-effort with retries
    max_retries = 3
    backoff = 0.5
    
    for attempt in range(max_retries):
        try:
            data = json.dumps(progress_data).encode("utf-8")
            req = urllib.request.Request(
                endpoint,
                data=data,
                headers={"Content-Type": "application/json"},
                method="POST"
            )
            with urllib.request.urlopen(req, timeout=15) as response:
                response_body = response.read().decode("utf-8")
                logger.info(f"[PROGRESS] RESPONSE {response.status}: {response_body}")
                logger.info(f"[PROGRESS] SUCCESS: job={job_id}, progress={progress_data.get('progress', progress_data.get('progress_percent', 0))}%")
                return
        except Exception as e:
            if attempt < max_retries - 1:
                logger.warning(f"[PROGRESS] Attempt {attempt+1}/{max_retries} failed: {e}, retrying in {backoff}s...")
                time.sleep(backoff)
                backoff *= 2
            else:
                logger.warning(f"[PROGRESS] All {max_retries} attempts failed, continuing pipeline: {e}")


def trigger_worker(job_id: str):
    """Trigger worker to process the downloaded video"""
    logger.info(f"[WORKER] trigger_worker called with job_id='{job_id}'")
    logger.info(f"[WORKER] BACKEND_URL configured: '{BACKEND_URL}'")
    
    if not job_id or not BACKEND_URL:
        logger.warning(f"[WORKER] No job_id or BACKEND_URL, skipping worker trigger (job_id={job_id}, BACKEND_URL={BACKEND_URL})")
        return False
    
    try:
        url = f"{BACKEND_URL}/api/ingestion/jobs/{job_id}/process"
        req = urllib.request.Request(url, method="POST")
        
        with urllib.request.urlopen(req, timeout=30) as response:
            if response.status == 200:
                logger.info(f"[WORKER] Triggered worker for job: {job_id}")
                return True
            else:
                logger.warning(f"[WORKER] Failed to trigger worker: status={response.status}")
                return False
    except Exception as e:
        logger.warning(f"[WORKER] Failed to trigger worker: {e}")
        return False


class DownloadError(Exception):
    pass


class DownloadProgress:
    """Thread-safe download progress tracker using file-based storage"""
    
    def __init__(self, progress_dir: str = "progress"):
        self.progress_dir = progress_dir
        os.makedirs(self.progress_dir, exist_ok=True)
        self._lock = threading.Lock()
    
    def _get_progress_file(self, job_id: str) -> str:
        """Get the progress file path for a job"""
        safe_id = re.sub(r'[^\w\-_.]', '_', job_id)
        return os.path.join(self.progress_dir, f"{safe_id}.json")
    
    def update(self, job_id: str, progress: dict):
        """Update progress for a job"""
        with self._lock:
            progress_file = self._get_progress_file(job_id)
            progress["updated_at"] = time.time()
            with open(progress_file, 'w') as f:
                json.dump(progress, f)
    
    def get(self, job_id: str) -> dict | None:
        """Get progress for a job"""
        with self._lock:
            progress_file = self._get_progress_file(job_id)
            if os.path.exists(progress_file):
                try:
                    with open(progress_file, 'r') as f:
                        return json.load(f)
                except (json.JSONDecodeError, IOError):
                    return None
            return None
    
    def clear(self, job_id: str):
        """Clear progress for a completed/failed job"""
        with self._lock:
            progress_file = self._get_progress_file(job_id)
            if os.path.exists(progress_file):
                try:
                    os.remove(progress_file)
                except OSError:
                    pass
    
    def parse_size(self, size_str: str) -> int:
        """Parse size string like '1.5MiB', '100KiB', '500B' to bytes"""
        size_str = size_str.strip()
        match = re.match(r'^([\d.]+)\s*(B|KiB|MiB|GiB|TiB)$', size_str, re.IGNORECASE)
        if not match:
            return 0
        value, unit = match.groups()
        value = float(value)
        unit = unit.lower()
        if unit == 'b':
            return int(value)
        elif unit == 'kib':
            return int(value * 1024)
        elif unit == 'mib':
            return int(value * 1024 * 1024)
        elif unit == 'gib':
            return int(value * 1024 * 1024 * 1024)
        elif unit == 'tib':
            return int(value * 1024 * 1024 * 1024 * 1024)
        return 0


class DownloaderService:
    def __init__(self, download_dir: str = "downloads", progress_dir: str = "progress"):
        self.download_dir = os.path.abspath(download_dir)
        self.progress = DownloadProgress(progress_dir)
        os.makedirs(self.download_dir, exist_ok=True)

    def detect_url_type(self, url: str) -> str:
        lower = url.lower()

        if ".m3u8" in lower:
            return "hls"
        if ".mpd" in lower:
            return "dash"
        if ".ism" in lower:
            return "ism"
        if lower.endswith(".mp4") or ".mp4?" in lower:
            return "mp4"

        headers = {"User-Agent": "Mozilla/5.0"}

        try:
            r = requests.head(url, allow_redirects=True, timeout=15, headers=headers)
            ct = (r.headers.get("Content-Type") or "").lower()

            if "mpegurl" in ct or "application/vnd.apple.mpegurl" in ct:
                return "hls"
            if "dash+xml" in ct:
                return "dash"
            if "video/mp4" in ct or "octet-stream" in ct:
                return "mp4"
        except Exception:
            pass

        try:
            r = requests.get(url, stream=True, allow_redirects=True, timeout=15, headers=headers)
            ct = (r.headers.get("Content-Type") or "").lower()

            if "mpegurl" in ct or "application/vnd.apple.mpegurl" in ct:
                return "hls"
            if "dash+xml" in ct:
                return "dash"
            if "video/mp4" in ct or "octet-stream" in ct:
                return "mp4"
        except Exception:
            pass

        return "unknown"

    def _download_mp4(self, url: str, output_path: str, job_id: str | None = None, backend_job_id: str | None = None, referer: str | None = None) -> str:
        """
        Download mp4 with parallel streaming support.
        Uses 8-thread parallel download if server supports range requests,
        otherwise falls back to single-thread download.
        """
        
        logger.info(f"[DOWNLOAD] Starting download: url={url[:50]}..., job_id={job_id}, backend_job_id={backend_job_id}")
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
        
        # Prepare headers
        headers = {"User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"}
        if referer:
            headers["Referer"] = referer
        
        # Check if server supports range requests
        supports_range, total_size = _check_range_support(url, headers)
        
        if supports_range and total_size > 10 * 1024 * 1024:  # Use parallel for files > 10MB
            logger.info(f"[DOWNLOAD] Using PARALLEL download ({PARALLEL_THREADS} threads) for {total_size} bytes")
            return self._parallel_download_mp4(url, output_path, total_size, headers, job_id, backend_job_id)
        else:
            logger.info(f"[DOWNLOAD] Using SINGLE-THREAD download (range not supported or file too small)")
            return self._single_download_mp4(url, output_path, headers, job_id, backend_job_id)
    
    
    def _parallel_download_mp4(self, url: str, output_path: str, total_size: int, headers: dict, 
                                 job_id: str | None, backend_job_id: str | None) -> str:
        """Download using parallel threads with HTTP Range requests."""
        global g_downloaded_bytes, g_total_bytes, g_start_time
        
        g_total_bytes = total_size
        g_downloaded_bytes = 0
        g_start_time = time.time()
        
        # Initialize progress
        if job_id:
            self.progress.update(job_id, {
                "status": "downloading",
                "stage": "download",
                "progress_percent": 0,
                "downloaded_bytes": 0,
                "total_bytes": total_size,
                "speed_bytes_per_sec": 0,
                "speed_mb_per_sec": 0.0,
                "eta_seconds": 0,
                "message": "Starting parallel download...",
            })
        
        # Send initial progress
        if backend_job_id:
            report_progress_to_backend(backend_job_id, {
                "stage": "download",
                "status": "downloading",
                "progress": 0,
                "downloaded_bytes": 0,
                "total_bytes": total_size,
                "speed_mbps": 0.0,
                "eta_seconds": 0,
                "message": "Starting parallel download..."
            })
        
        # Calculate byte ranges for each thread
        num_threads = PARALLEL_THREADS
        chunk_size = total_size // num_threads
        part_ranges = []
        
        for i in range(num_threads):
            start = i * chunk_size
            end = start + chunk_size - 1 if i < num_threads - 1 else total_size - 1
            part_ranges.append((start, end))
        
        logger.info(f"[DOWNLOAD] Part ranges: {part_ranges}")
        
        # Create temp directory for parts
        temp_dir = os.path.dirname(output_path)
        
        # Progress callback
        def on_progress(progress: int, downloaded: int, total: int, speed: float, eta: int):
            if job_id:
                self.progress.update(job_id, {
                    "status": "downloading",
                    "stage": "download",
                    "progress_percent": progress,
                    "downloaded_bytes": downloaded,
                    "total_bytes": total,
                    "speed_bytes_per_sec": int(speed),
                    "speed_mb_per_sec": round(speed / (1024 * 1024), 1),
                    "eta_seconds": eta,
                    "message": f"Parallel downloading... {progress}%",
                })
            
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "downloading",
                    "progress": progress,
                    "downloaded_bytes": downloaded,
                    "total_bytes": total,
                    "speed_mbps": round(speed / (1024 * 1024), 1),
                    "eta_seconds": eta,
                    "message": f"Parallel downloading... {progress}%"
                })
        
        # Run parallel download
        success = _parallel_download_worker(
            url, headers, part_ranges, temp_dir, num_threads,
            job_id, backend_job_id, on_progress
        )
        
        if not success:
            raise Exception("Parallel download failed - falling back to single-thread")
        
        # Merge parts
        logger.info("[DOWNLOAD] Merging parts...")
        part_files = [os.path.join(temp_dir, f'part_{i}.tmp') for i in range(num_threads)]
        
        if not _merge_parts(part_files, output_path):
            raise Exception("Failed to merge downloaded parts")
        
        # Clean up temporary part files after successful merge
        logger.info("[DOWNLOAD] Cleaning up temporary part files...")
        for part_file in part_files:
            try:
                if os.path.exists(part_file):
                    os.remove(part_file)
                    logger.info(f"[DOWNLOAD] Removed: {part_file}")
            except Exception as e:
                logger.warning(f"[DOWNLOAD] Failed to remove part file {part_file}: {e}")
        
        # Final progress update
        final_size = os.path.getsize(output_path)
        logger.info(f"[DOWNLOAD] Parallel download completed: {output_path} ({final_size} bytes)")
        
        if job_id:
            self.progress.update(job_id, {
                "status": "completed",
                "stage": "download",
                "progress_percent": 100,
                "downloaded_bytes": final_size,
                "total_bytes": total_size,
                "message": "Download completed",
            })
        
        if backend_job_id:
            report_progress_to_backend(backend_job_id, {
                "stage": "download",
                "status": "processing",
                "progress": 50,
                "downloaded_bytes": final_size,
                "total_bytes": total_size,
                "speed_mbps": 0,
                "eta_seconds": 0,
                "message": "Download completed - worker will process",
                "steps_download": True,
                "file_path": output_path  # NEW: Send local file path to backend
            })
            # NOTE: Do NOT trigger worker here - worker already initiated the download
            # Worker will continue processing after parser returns
            # trigger_worker(backend_job_id)
        
        return output_path
    
    
    def _single_download_mp4(self, url: str, output_path: str, headers: dict, 
                              job_id: str | None, backend_job_id: str | None) -> str:
        """Single-thread fallback download (original implementation)."""
        
        logger.info(f"[DOWNLOAD] Starting streaming download: url={url[:50]}..., job_id={job_id}, backend_job_id={backend_job_id}")
        
        # Get referer from headers if present
        referer = headers.get("Referer", "")
        
        # Initialize progress tracking
        start_time = time.time()
        downloaded_bytes = 0
        total_bytes = 0
        last_update_time = time.time()
        last_reported_bytes = 0
        last_reported_progress = -1  # Track last progress sent to avoid duplicates
        
        # Initialize local progress
        if job_id:
            self.progress.update(job_id, {
                "status": "downloading",
                "stage": "download",
                "progress_percent": 0,
                "downloaded_bytes": 0,
                "total_bytes": 0,
                "downloaded_mb": 0.0,
                "total_mb": 0.0,
                "speed_bytes_per_sec": 0,
                "speed_mb_per_sec": 0.0,
                "eta_seconds": 0,
                "message": "Starting download...",
            })
        
        # Initialize progress tracking variables
        last_reported_bytes = 0
        last_update_time = time.time()
        last_reported_progress = -1  # track last % sent; update on +1% OR 2s elapsed
        
        # Send initial progress to backend
        if backend_job_id:
            logger.info(f"[PROGRESS] Sending initial progress: job_id={backend_job_id}, progress=0%")
            report_progress_to_backend(backend_job_id, {
                "stage": "download",
                "status": "downloading",
                "progress": 0,
                "downloaded_bytes": 0,
                "total_bytes": 0,
                "speed_mbps": 0.0,
                "eta_seconds": 0,
                "message": "Starting download..."
            })
        
        try:
            # Stream download with persistent session for better performance
            response = http_session.get(url, headers=headers, stream=True, timeout=60, allow_redirects=True)
            response.raise_for_status()
            
            # Get total size if available
            total_bytes = int(response.headers.get('Content-Length', 0))
            logger.info(f"[DOWNLOAD] Total size: {total_bytes} bytes")
            
            # Use 512KB chunks for smoother progress updates
            chunk_size = 512 * 1024
            
            with open(output_path, 'wb') as f:
                for chunk in response.iter_content(chunk_size=chunk_size):
                    if chunk:
                        f.write(chunk)
                        downloaded_bytes += len(chunk)
                        
                        # Calculate progress
                        if total_bytes > 0:
                            progress_percent = int((downloaded_bytes / total_bytes) * 100)
                        else:
                            progress_percent = 0
                        
                        # Calculate speed and ETA
                        elapsed = time.time() - start_time
                        if elapsed > 0:
                            speed_bytes_per_sec = downloaded_bytes / elapsed
                            speed_mb_per_sec = speed_bytes_per_sec / (1024 * 1024)
                            
                            if total_bytes > 0 and speed_bytes_per_sec > 0:
                                remaining_bytes = total_bytes - downloaded_bytes
                                eta_seconds = int(remaining_bytes / speed_bytes_per_sec)
                            else:
                                eta_seconds = 0
                        else:
                            speed_bytes_per_sec = 0
                            speed_mb_per_sec = 0.0
                            eta_seconds = 0
                        
                        downloaded_mb = downloaded_bytes / (1024 * 1024)
                        total_mb = total_bytes / (1024 * 1024) if total_bytes > 0 else 0
                        
                        # Update local progress
                        if job_id:
                            self.progress.update(job_id, {
                                "status": "downloading",
                                "stage": "download",
                                "progress_percent": progress_percent,
                                "downloaded_bytes": downloaded_bytes,
                                "total_bytes": total_bytes,
                                "downloaded_mb": round(downloaded_mb, 1),
                                "total_mb": round(total_mb, 1),
                                "speed_bytes_per_sec": int(speed_bytes_per_sec),
                                "speed_mb_per_sec": round(speed_mb_per_sec, 1),
                                "eta_seconds": eta_seconds,
                                "message": f"Downloading... {downloaded_mb:.1f} / {total_mb:.1f} MB ({progress_percent}%)",
                            })
                        
                        # Report to backend on +1% progress OR every 2s (avoid per-chunk DB writes)
                        current_time = time.time()
                        time_since_last_update = current_time - last_update_time
                        should_update = (
                            backend_job_id and (
                                progress_percent > last_reported_progress or
                                time_since_last_update >= 2.0
                            )
                        )
                        
                        if should_update:
                            # Log only occasionally to reduce overhead
                            if progress_percent % 10 == 0 or progress_percent == 1:
                                logger.info(f"[PROGRESS] job_id={backend_job_id}, downloaded={downloaded_bytes}, progress={progress_percent}%")
                            
                            # Send progress with actual bytes
                            report_progress_to_backend(backend_job_id, {
                                "stage": "download",
                                "status": "downloading",
                                "progress": progress_percent,
                                "downloaded_bytes": downloaded_bytes,
                                "total_bytes": total_bytes,
                                "speed_mbps": round(speed_mb_per_sec, 1),
                                "eta_seconds": eta_seconds,
                                "message": f"Downloading... {downloaded_mb:.1f} / {total_mb:.1f} MB ({progress_percent}%)"
                            })
                            last_reported_bytes = downloaded_bytes
                            last_reported_progress = progress_percent
                            last_update_time = current_time
            
            # Download completed successfully
            logger.info(f"[DOWNLOAD] Completed: {output_path}")
            
            # Update local progress to completed
            if job_id:
                self.progress.update(job_id, {
                    "status": "completed",
                    "stage": "download",
                    "progress_percent": 100,
                    "downloaded_bytes": downloaded_bytes,
                    "total_bytes": total_bytes,
                    "downloaded_mb": round(downloaded_mb, 1),
                    "total_mb": round(total_mb, 1),
                    "speed_bytes_per_sec": 0,
                    "speed_mb_per_sec": 0.0,
                    "eta_seconds": 0,
                    "message": "Download completed",
                })
            
            # Report completion to backend
            if backend_job_id:
                logger.info(f"[PROGRESS] Sending completion: job_id={backend_job_id}, progress=50%, steps_download=True")
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "processing",
                    "progress": 50,
                    "downloaded_bytes": downloaded_bytes,
                    "total_bytes": total_bytes,
                    "speed_mbps": 0,
                    "eta_seconds": 0,
                    "message": "Download completed - worker will process",
                    "steps_download": True,
                    "file_path": output_path  # NEW: Send local file path to backend
                })
                
                # NOTE: Do NOT call trigger_worker - worker already initiated this download
                # Worker will continue processing after parser returns
                # logger.info(f"[WORKER] Triggering worker for job: {backend_job_id}")
                # trigger_worker(backend_job_id)
            
            return output_path
            
        except requests.exceptions.RequestException as e:
            logger.error(f"[DOWNLOAD] Request failed: {e}")
            if job_id:
                self.progress.update(job_id, {
                    "status": "failed",
                    "stage": "download",
                    "progress_percent": 0,
                    "message": f"Download failed: {str(e)}",
                })
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": f"Download failed: {str(e)}"
                })
            raise DownloadError(f"Download failed: {str(e)}")

    def _download_manifest(self, url: str, output_path: str, job_id: str | None = None, backend_job_id: str | None = None, referer: str | None = None) -> str:
        """
        Download HLS/DASH manifest using ffmpeg.
        This is a robust, reliable method for downloading streaming content.
        
        Strategy:
        1. Use ffmpeg to download and re-mux the stream to MP4
        2. Monitor progress via ffmpeg's stderr output
        3. Validate output file after completion
        
        Args:
            url: HLS/DASH manifest URL
            output_path: Where to save the downloaded file
            job_id: Internal job ID for progress tracking
            backend_job_id: Backend job ID for progress reporting
            referer: Optional referer header
            
        Returns:
            Path to downloaded file
            
        Raises:
            DownloadError: If download fails
        """
        logger.info(f"[DOWNLOAD] _download_manifest: url={url[:80]}..., job_id={job_id}, backend_job_id={backend_job_id}")
        logger.info(f"[DOWNLOAD] Manifest detected: HLS/DASH stream")
        
        download_start_time = time.time()
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
        
        # Local progress - guarded by job_id
        if job_id:
            self.progress.update(job_id, {
                "status": "downloading",
                "stage": "download",
                "progress_percent": 0,
                "downloaded_bytes": 0,
                "total_bytes": 0,
                "downloaded_mb": 0.0,
                "total_mb": 0.0,
                "speed_bytes_per_sec": 0,
                "speed_mb_per_sec": 0.0,
                "eta_seconds": 0,
                "message": "Starting HLS download with ffmpeg...",
            })
        
        # Backend progress - guarded by backend_job_id
        if backend_job_id:
            logger.info(f"[PROGRESS] Sending initial progress: job_id={backend_job_id}, progress=0%")
            report_progress_to_backend(backend_job_id, {
                "stage": "download",
                "status": "downloading",
                "progress": 0,
                "message": "Starting HLS download with ffmpeg..."
            })
        
        # Build ffmpeg command for HLS download
        # Use -i for input, -c copy for fast re-mux (no re-encoding)
        # -bsf:a aac_adtstoasc fixes some AAC issues
        # -y to overwrite output file
        cmd = [
            "ffmpeg",
            "-y",  # Overwrite output
            "-i", url,  # Input URL
            "-c", "copy",  # Copy streams without re-encoding (fast)
            "-bsf:a", "aac_adtstoasc",  # Fix AAC bitstream filter
            "-progress", "pipe:1",  # Output progress to stdout
            "-nostats",  # Suppress default stats
            output_path  # Output file
        ]
        
        # Add referer header if provided
        env = os.environ.copy()
        if referer:
            # ffmpeg doesn't support -headers directly in all versions
            # Use environment variable approach
            logger.info(f"[DOWNLOAD] Using referer: {referer[:50]}...")
        
        try:
            logger.info(f"[DOWNLOAD] Starting ffmpeg HLS download: {output_path}")
            logger.info(f"[DOWNLOAD] FFmpeg command: {' '.join(cmd[:5])}...")
            
            # Start ffmpeg process
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
                env=env
            )
            
            # Track progress by parsing ffmpeg output
            total_size = 0
            downloaded_bytes = 0
            last_update_time = time.time()
            min_time_for_update = 1.0  # Update progress every second
            
            # Parse ffmpeg progress output
            progress_pattern = re.compile(r'out_time_ms=(\d+)')
            
            for line in process.stderr:
                line = line.strip()
                
                # Parse progress
                if "out_time_ms=" in line:
                    try:
                        match = progress_pattern.search(line)
                        if match:
                            time_ms = int(match.group(1))
                            # Estimate bytes from time (rough estimate based on duration)
                            # FFmpeg doesn't give us total_bytes for HLS, so we estimate
                            if time_ms > 0:
                                # Use a placeholder until we get actual duration
                                pass
                    except:
                        pass
                
                # Parse duration info
                if "Duration:" in line:
                    # Extract duration from line like: "Duration: 01:23:45.67, start: 0.000000, bitrate: 5000 kb/s"
                    duration_match = re.search(r'Duration: (\d+):(\d+):(\d+)\.(\d+)', line)
                    if duration_match:
                        hours, mins, secs, _ = duration_match.groups()
                        total_seconds = int(hours) * 3600 + int(mins) * 60 + int(secs)
                        logger.info(f"[DOWNLOAD] HLS duration: {total_seconds} seconds")
                
                # Check for errors
                if "Error" in line or "Failed" in line:
                    logger.warning(f"[DOWNLOAD] FFmpeg warning: {line}")
                
                # Calculate elapsed time and progress
                elapsed = time.time() - download_start_time
                current_time = time.time()
                
                if current_time - last_update_time >= min_time_for_update:
                    # Estimate progress (we don't know total bytes for HLS)
                    # Report based on time elapsed
                    progress_percent = min(95, int(elapsed * 2))  # Estimate based on time
                    
                    if job_id:
                        self.progress.update(job_id, {
                            "status": "downloading",
                            "stage": "download",
                            "progress_percent": progress_percent,
                            "downloaded_bytes": int(elapsed * 1024 * 1024),  # Rough estimate
                            "total_bytes": 0,  # Unknown for HLS
                            "downloaded_mb": round(elapsed * 1, 1),
                            "total_mb": 0.0,
                            "speed_bytes_per_sec": int(1024 * 1024),  # Estimate
                            "speed_mb_per_sec": 1.0,
                            "eta_seconds": max(0, int(60 - elapsed)),  # Estimate
                            "message": f"Downloading HLS... {progress_percent}%",
                        })
                    
                    if backend_job_id:
                        report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "downloading",
                            "progress": progress_percent,
                            "message": f"Downloading HLS... {progress_percent}%"
                        })
                    
                    last_update_time = current_time
            
            # Wait for process to complete
            return_code = process.wait()
            
            download_duration = time.time() - download_start_time
            logger.info(f"[DOWNLOAD] FFmpeg exited with code {return_code} after {download_duration:.1f}s")
            
            if return_code != 0:
                stderr_output = process.stderr.read()
                logger.error(f"[DOWNLOAD] FFmpeg stderr: {stderr_output[:500]}")
                raise DownloadError(f"FFmpeg failed with exit code {return_code}")
            
            # CRITICAL: Verify file exists
            if not os.path.exists(output_path):
                raise DownloadError(f"FFmpeg reported success but file not found: {output_path}")
            
            file_size = os.path.getsize(output_path)
            if file_size == 0:
                raise DownloadError(f"Downloaded file is empty: {output_path}")
            
            logger.info(f"[DOWNLOAD] HLS download completed: {output_path} ({file_size} bytes)")
            
            # Local progress - complete
            if job_id:
                self.progress.update(job_id, {
                    "status": "completed",
                    "stage": "download",
                    "progress_percent": 100,
                    "downloaded_bytes": file_size,
                    "total_bytes": file_size,
                    "downloaded_mb": round(file_size / (1024*1024), 2),
                    "total_mb": round(file_size / (1024*1024), 2),
                    "speed_bytes_per_sec": 0,
                    "speed_mb_per_sec": 0.0,
                    "eta_seconds": 0,
                    "message": "Download completed",
                    "file_path": output_path,
                })
            
            # Backend progress - complete
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "completed",
                    "progress": 100,
                    "downloaded_bytes": file_size,
                    "total_bytes": file_size,
                    "message": f"HLS download completed in {download_duration:.1f}s - ready for processing",
                    "steps_download": True,
                    "file_path": output_path,
                    "download_duration_seconds": download_duration,
                })
            
            return output_path
            
        except subprocess.TimeoutExpired:
            logger.error(f"[DOWNLOAD] FFmpeg timed out after {download_duration:.1f}s")
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": "HLS download timed out"
                })
            raise DownloadError("HLS download timed out")
            
        except FileNotFoundError:
            logger.error("[DOWNLOAD] FFmpeg not found! Please install ffmpeg.")
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": "FFmpeg not installed"
                })
            raise DownloadError("FFmpeg not installed. Please install ffmpeg.")
            
        except DownloadError:
            raise
            
        except Exception as e:
            logger.error(f"[DOWNLOAD] HLS download failed: {e}", exc_info=True)
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": f"HLS download failed: {str(e)}"
                })
                raise DownloadError(f"HLS download failed: {e}") from e

    def _download_mp4_with_aria2c(
        self,
        url: str,
        output_path: str,
        job_id: str | None = None,
        backend_job_id: str | None = None,
        referer: str | None = None
    ) -> str:
        """
        Download MP4 using aria2c with parallel connections.
        
        This is the preferred method for direct MP4 downloads as aria2c provides
        better performance with parallel connections and automatic retries.
        
        Args:
            url: Direct MP4 URL
            output_path: Output file path
            job_id: Internal job ID for progress
            backend_job_id: Backend job ID for progress
            referer: Optional referer header
            
        Returns:
            Path to downloaded file
            
        Raises:
            DownloadError: If download fails
        """
        if DDdownloaderIntegration is None:
            raise DownloadError("DDownloader integration not available for aria2c")
        
        logger.info(f"[ARIA2C] _download_mp4_with_aria2c: url={url[:60]}...")
        
        # Create DDownloader integration instance
        dd = DDdownloaderIntegration(download_dir=self.download_dir)
        
        # Use the DDownloader integration's aria2c for MP4
        result = dd.smart_download(
            url=url,
            output_name=os.path.basename(output_path),
            job_id=job_id,
            backend_job_id=backend_job_id,
            referer=referer,
            max_retries=3
        )
        
        if not result.success:
            raise DownloadError(f"aria2c download failed: {result.error}")
        
        return result.local_path

    def _download_manifest_with_ddownloader(
        self,
        url: str,
        output_path: str,
        stream_type: str,
        job_id: str | None = None,
        backend_job_id: str | None = None,
        referer: str | None = None
    ) -> str:
        """
        Download HLS/DASH/ISM manifest using DDownloader's N_m3u8DL-RE.
        
        This is the preferred method for manifest downloads as it's more robust
        and handles edge cases better than raw FFmpeg.
        
        Args:
            url: Manifest URL (m3u8, mpd, ism)
            output_path: Output file path (without extension)
            stream_type: Type of stream (hls, dash, ism)
            job_id: Internal job ID for progress
            backend_job_id: Backend job ID for progress
            referer: Optional referer header
            
        Returns:
            Path to downloaded file
            
        Raises:
            DownloadError: If download fails
        """
        if DDdownloaderIntegration is None:
            raise DownloadError("DDownloader integration not available")
        
        # === CRITICAL URL VALIDATION FOR N_m3u8DL ===
        # N_m3u8DL-RE expects a direct m3u8 stream URL, NOT an HTML page URL
        if not isValidStreamUrl(url):
            error_msg = f"Invalid media URL for N_m3u8DL-RE: expected m3u8 manifest, got HTML/page URL: {url[:80]}..."
            logger.error(f"[DDOWNLOADER] {error_msg}")
            raise DownloadError(error_msg)
        
        logger.info(f"[DDOWNLOADER] URL validation passed for N_m3u8DL-RE: {url[:60]}...")
        logger.info(f"[DDOWNLOADER] _download_manifest_with_ddownloader: url={url[:60]}...")
        
        # Create DDownloader integration instance
        dd = DDdownloaderIntegration(download_dir=self.download_dir)
        
        # Use the DDownloader integration
        result = dd.smart_download(
            url=url,
            output_name=os.path.basename(output_path),
            job_id=job_id,
            backend_job_id=backend_job_id,
            referer=referer,
            max_retries=3
        )
        
        if not result.success:
            raise DownloadError(f"DDownloader failed: {result.error}")

        return result.local_path

    def _download_youtube(
        self,
        url: str,
        output_name: str,
        backend_job_id: str | None = None,
    ) -> dict:
        """Download a YouTube video using yt-dlp and return smart_download-compatible dict."""
        import subprocess, sys as _sys, os

        os.makedirs(self.download_dir, exist_ok=True)

        # Strip extension — yt-dlp adds its own
        base_name = output_name.rsplit(".", 1)[0] if "." in output_name else output_name
        output_template = os.path.join(self.download_dir, base_name + ".%(ext)s")

        # YouTube cookies file from env
        cookies_path = os.environ.get("YTDLP_COOKIE_FILE", "/opt/filmorauz/parser/cookies.txt")
        user_agent = os.environ.get("YTDLP_USER_AGENT", "Mozilla/5.0 (Linux; Android 13) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.6099.210 Mobile Safari/537.36")

        cmd = [
            _sys.executable, "-m", "yt_dlp",
            "--no-playlist",
            "-f", "bestvideo[ext=mp4]+bestaudio[ext=m4a]/best[ext=mp4]/best",
            "--merge-output-format", "mp4",
            "-o", output_template,
            "--no-warnings",
            "--user-agent", user_agent,
            "--extractor-args", "youtube:player_client=android",
        ]

        # Add cookies if file exists
        if os.path.exists(cookies_path):
            cmd.extend(["--cookies", cookies_path])
            logger.info(f"[YTDLP] Using cookies file: {cookies_path}")
        else:
            logger.warning(f"[YTDLP] Cookies file not found: {cookies_path} - using fallback config (may be rate limited)")

        cmd.append(url)

        logger.info(f"[YTDLP] Downloading: {url[:80]}")

        try:
            result = subprocess.run(
                cmd,
                capture_output=True,
                text=True,
                timeout=3600,
            )
        except subprocess.TimeoutExpired:
            raise DownloadError("yt-dlp timed out after 60 minutes")

        if result.returncode != 0:
            err = (result.stderr or result.stdout or "").strip()
            err_lower = err.lower()
            # Detect anti-bot / authentication required errors
            if any(phrase in err_lower for phrase in [
                "sign in to confirm you're not a bot",
                "this video is available to members only",
                "video requires authentication",
                "login required",
                "yt-dlp: 403",
                "http error 403",
                "please sign in",
                "is not available to view",
                "unable to extract",
                "video unavailable",
            ]):
                raise DownloadError(
                    "YouTube download failed: " + err[:200]
                )
            raise DownloadError("yt-dlp failed: " + err[:300])

        # Find the downloaded file
        local_path = None
        for ext in ("mp4", "mkv", "webm", "m4v"):
            candidate = os.path.join(self.download_dir, f"{base_name}.{ext}")
            if os.path.exists(candidate):
                local_path = candidate
                break

        if not local_path:
            raise DownloadError("yt-dlp completed but output file not found")

        file_size = os.path.getsize(local_path)
        logger.info(f"[YTDLP] Download complete: {local_path} ({file_size} bytes)")

        return {
            "success": True,
            "type": "mp4",
            "file_path": local_path,
            "file_name": os.path.basename(local_path),
            "file_size": file_size,
        }

    def smart_download(
        self,
        url: str,
        output_name: str,
        job_id: str | None = None,
        backend_job_id: str | None = None,
        referer: str | None = None,
        max_retries: int = 3,
    ) -> dict:
        """
        Download video with retry support and validation.
        
        Args:
            url: Video URL
            output_name: Output filename
            job_id: Internal job ID for progress
            backend_job_id: Backend job ID for progress reporting
            referer: Optional referer header
            max_retries: Maximum retry attempts (default 3)
            
        Returns:
            dict with success, type, file_path, file_name
            
        Raises:
            DownloadError: If all retries fail
        """
        logger.info(f"[DOWNLOADER] smart_download called: url={url[:50]}..., output_name={output_name}")
        logger.info(f"[DOWNLOADER] job_id={job_id}, backend_job_id='{backend_job_id}', max_retries={max_retries}")

        # === YOUTUBE FAST-PATH ===
        if is_youtube_url(url):
            logger.info(f"[DOWNLOADER] YouTube URL detected — using yt-dlp")
            return self._download_youtube(url, output_name, backend_job_id=backend_job_id)

        # === CRITICAL URL VALIDATION ===
        # Ensure URL is a valid stream URL before attempting download
        # This prevents passing HTML pages to N_m3u8DL which would cause errors
        if not isValidStreamUrl(url):
            error_msg = f"Invalid media URL: expected m3u8/mp4 stream, got HTML/page URL: {url[:80]}..."
            logger.error(f"[DOWNLOADER] {error_msg}")
            
            if backend_job_id:
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": error_msg,
                    "error": error_msg,
                })
            
            raise DownloadError(error_msg)
        
        logger.info(f"[DOWNLOADER] URL validation passed: {url[:80]}...")
        
        # Detect stream type before using it
        stream_type = self.detect_url_type(url)
        logger.info(f"[DOWNLOADER] Detected stream type: {stream_type}")
        logger.info(f"[DOWNLOADER] Starting download: url_type={stream_type}, referer={referer[:50] if referer else 'None'}...")
        
        # Log URL type detection details
        if stream_type == "hls":
            logger.info(f"[DOWNLOADER] Will use N_m3u8DL-RE for HLS download")
        elif stream_type == "dash":
            logger.info(f"[DOWNLOADER] Will use N_m3u8DL-RE for DASH download")
        elif stream_type == "mp4":
            logger.info(f"[DOWNLOADER] Will use direct HTTP download for MP4")
        else:
            logger.warning(f"[DOWNLOADER] Unknown stream type: {stream_type}, attempting download anyway")
        
        output_path = os.path.join(self.download_dir, output_name)
        logger.info(f"[DOWNLOADER] Downloading to {output_path}")
        
        # Ensure download directory exists
        os.makedirs(self.download_dir, exist_ok=True)
        
        # Clean up any existing partial download
        if os.path.exists(output_path):
            logger.info(f"[DOWNLOADER] Removing existing file: {output_path}")
            try:
                os.remove(output_path)
            except:
                pass
        
        # Retry loop with exponential backoff
        last_error = None
        for attempt in range(1, max_retries + 1):
            attempt_job_id = f"{job_id}_attempt{attempt}" if job_id else None
            
            if attempt > 1:
                wait_time = 2 ** (attempt - 1)  # Exponential backoff: 1s, 2s, 4s, ...
                logger.info(f"[DOWNLOADER] Retry {attempt}/{max_retries} after {wait_time}s backoff")
                time.sleep(wait_time)
                
                if backend_job_id:
                    report_progress_to_backend(backend_job_id, {
                        "stage": "download",
                        "status": "retrying",
                        "progress": 0,
                        "message": f"Retry {attempt}/{max_retries}..."
                    })
            
            try:
                logger.info(f"[DOWNLOADER] Download attempt {attempt}/{max_retries}")
                
                if stream_type == "mp4":
                    # Try aria2c via DDownloader first for MP4 (faster with parallel connections)
                    if DDdownloaderIntegration is not None and USE_DDOWNLOADER:
                        logger.info(f"[DOWNLOADER] Using _download_mp4_with_aria2c()")
                        path = self._download_mp4_with_aria2c(
                            url, output_path,
                            job_id=attempt_job_id, backend_job_id=backend_job_id, referer=referer
                        )
                    else:
                        logger.info(f"[DOWNLOADER] Using _download_mp4() (fallback to Python parallel download)")
                        path = self._download_mp4(url, output_path, job_id=attempt_job_id, backend_job_id=backend_job_id, referer=referer)
                elif stream_type in ("hls", "dash", "ism"):
                    # Try DDownloader first for manifest streams (more robust)
                    if DDdownloaderIntegration is not None and USE_DDOWNLOADER:
                        logger.info(f"[DOWNLOADER] Using _download_manifest_with_ddownloader()")
                        path = self._download_manifest_with_ddownloader(
                            url, output_path, stream_type,
                            job_id=attempt_job_id, backend_job_id=backend_job_id, referer=referer
                        )
                    else:
                        logger.info(f"[DOWNLOADER] Using _download_manifest() (fallback to ffmpeg)")
                        path = self._download_manifest(url, output_path, job_id=attempt_job_id, backend_job_id=backend_job_id, referer=referer)
                else:
                    raise DownloadError(f"Unknown stream type: {url}")
                
                # CRITICAL: Validate downloaded file
                if not os.path.exists(path):
                    raise DownloadError(f"Download returned but file does not exist: {path}")
                
                file_size = os.path.getsize(path)
                if file_size == 0:
                    raise DownloadError(f"Downloaded file is empty: {path}")
                
                # Minimum file size check (at least 1MB for a valid video)
                MIN_FILE_SIZE = 1024 * 1024
                if file_size < MIN_FILE_SIZE:
                    logger.warning(f"[DOWNLOADER] File too small ({file_size} bytes), may be corrupted")
                    # Don't fail on small files - some short videos might be legitimate
                
                logger.info(f"[DOWNLOADER] Download validated successfully: {path} ({file_size} bytes)")
                
                return {
                    "success": True,
                    "type": stream_type,
                    "file_path": path,
                    "file_name": output_name,
                    "file_size": file_size,
                }
                
            except DownloadError as e:
                last_error = e
                logger.error(f"[DOWNLOADER] Download attempt {attempt} failed: {e}")
                
                # Clean up partial download on failure
                if os.path.exists(output_path):
                    try:
                        os.remove(output_path)
                        logger.info(f"[DOWNLOADER] Cleaned up partial download: {output_path}")
                    except:
                        pass
                
                if attempt == max_retries:
                    logger.error(f"[DOWNLOADER] All {max_retries} attempts failed")
                    break
                    
            except Exception as e:
                last_error = DownloadError(f"Unexpected error: {e}")
                logger.error(f"[DOWNLOADER] Download attempt {attempt} unexpected error: {e}")
                
                if os.path.exists(output_path):
                    try:
                        os.remove(output_path)
                    except:
                        pass
                
                if attempt == max_retries:
                    break
        
        # All retries exhausted
        error_msg = f"Download failed after {max_retries} attempts. Last error: {last_error}"
        logger.error(f"[DOWNLOADER] {error_msg}")
        
        if backend_job_id:
            report_progress_to_backend(backend_job_id, {
                "stage": "download",
                "status": "failed",
                "progress": 0,
                "message": error_msg,
                "error": str(last_error),
            })
        
        raise DownloadError(error_msg)
    
    def get_progress(self, job_id: str) -> dict | None:
        """Get download progress for a job"""
        return self.progress.get(job_id)
