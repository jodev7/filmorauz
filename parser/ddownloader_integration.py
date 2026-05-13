"""
DDownloader Integration Module for FilmoraUz Parser Service

This module provides a clean integration with DDownloader's N_m3u8DL-RE binary
for downloading HLS/DASH/ISM streams, and aria2c for direct MP4 downloads.

Architecture:
- detect_url_type(url) -> str: Detects stream type from URL and Content-Type
- smart_download(url, output_name) -> dict: Main download entry point
- _download_with_ddownloader(url, output_path) -> str: Uses N_m3u8DL-RE for manifest streams
- _download_with_aria2c(url, output_path) -> str: Uses aria2c for MP4 direct downloads (1st try)
- _download_mp4_with_ffmpeg(url, output_path) -> str: Uses ffmpeg for signed CDN MP4s (2nd try)
- _download_with_curl(url, output_path) -> str: curl last-resort MP4 fallback (3rd try)
- validate_downloaded_file(path) -> bool: Validates downloaded file

Supported types:
- hls (.m3u8)
- dash (.mpd)
- ism (.ism/.ismc)
- direct mp4 (.mp4)

Author: FilmoraUz Parser Team
"""

import os
import re
import json
import subprocess
import logging
import time
import requests
import threading
import shutil
from urllib.parse import quote
from dataclasses import dataclass
from typing import Optional
from enum import Enum

# Load environment variables from .env file (parser directory)
from dotenv import load_dotenv
load_dotenv(os.path.join(os.path.dirname(__file__), '.env'))

# Configure logging
logger = logging.getLogger(__name__)

# Constants
USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"
DEFAULT_TIMEOUT = 60
PARALLEL_CONNECTIONS = 16  # For aria2c

DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS = int(os.environ.get("DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS", "300"))
M3U8_STUCK_TIMEOUT_SECONDS = int(os.environ.get("M3U8_STUCK_TIMEOUT_SECONDS", str(DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS)))
ARIA2C_USER_AGENT = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"


def _origin_for_referer(referer: Optional[str]) -> str:
    referer = (referer or "").strip()
    if not referer:
        return ""
    try:
        from urllib.parse import urlparse
        parsed = urlparse(referer)
        host = (parsed.netloc or "").lower()
        
        # Asilmedia uses multiple mirrors (org, net, biz, uz, etc.)
        if "asilmedia" in host:
            # For any asilmedia mirror, use the canonical origin if possible, 
            # or just the host's own origin.
            return f"{parsed.scheme}://{parsed.netloc}"
            
        if parsed.scheme and parsed.netloc:
            return f"{parsed.scheme}://{parsed.netloc}"
    except Exception:
        pass
    return ""


def _debug(debug_callback, event: str, payload: dict):
    if debug_callback:
        try:
            debug_callback(event, payload)
        except Exception:
            pass


class StreamType(Enum):
    """Supported stream types"""
    HLS = "hls"
    DASH = "dash"
    ISM = "ism"
    MP4 = "mp4"
    UNKNOWN = "unknown"


@dataclass
class DownloadResult:
    """Structured result for download operations"""
    success: bool
    source: str
    video_type: str
    local_path: Optional[str] = None
    output_filename: Optional[str] = None
    file_size: int = 0
    error: Optional[str] = None
    download_duration: float = 0.0
    
    def to_dict(self) -> dict:
        """Convert to dictionary for API response"""
        return {
            "success": self.success,
            "source": self.source,
            "video_type": self.video_type,
            "local_path": self.local_path,
            "output_filename": self.output_filename,
            "file_size": self.file_size,
            "error": self.error,
            "download_duration_seconds": round(self.download_duration, 2),
        }


class DDdownloaderIntegrationError(Exception):
    """Custom exception for DDownloader integration errors"""
    pass


class DDownloaderIntegration:
    """
    DDownloader-based downloader for parser service.
    
    Uses N_m3u8DL-RE for HLS/DASH/ISM streams and aria2c for MP4 direct downloads.
    Provides proper validation and structured responses.
    """
    
    def __init__(self, download_dir: str = "downloads"):
        """
        Initialize the DDownloader integration.
        
        Args:
            download_dir: Base directory for downloads
        """
        self.download_dir = os.path.abspath(download_dir)
        os.makedirs(self.download_dir, exist_ok=True)
        
        # Find binary paths
        self._bin_dir = self._find_bin_directory()
        self._n_m3u8dl_path = self._find_n_m3u8dl_re()
        self._aria2c_path = self._find_aria2c()
        self._ffmpeg_path = self._find_ffmpeg()
        self._ffprobe_path = self._find_ffprobe()
        
        # Log initialization status
        logger.info(f"[DDOWNLOADER] Initialized with download_dir={self.download_dir}")
        logger.info(f"[DDOWNLOADER] N_m3u8DL-RE: {self._n_m3u8dl_path}")
        logger.info(f"[DDOWNLOADER] aria2c: {self._aria2c_path}")
        logger.info(f"[DDOWNLOADER] ffmpeg: {self._ffmpeg_path}")
        logger.info(f"[DDOWNLOADER] ffprobe: {self._ffprobe_path}")
    
    def _find_bin_directory(self) -> str:
        """Find the DDownloader bin directory"""
        # Try to find from DDownloader package
        try:
            import DDownloader.modules
            dd_path = os.path.dirname(DDownloader.modules.__file__)
            bin_path = os.path.join(os.path.dirname(dd_path), 'bin')
            if os.path.exists(bin_path):
                return bin_path
        except ImportError:
            pass
        
        # Fallback: use venv path
        venv_bin = os.path.join(os.path.dirname(__file__), 'venv', 'lib', 'python3.10', 'site-packages', 'DDownloader', 'bin')
        if os.path.exists(venv_bin):
            return venv_bin
        
        # Last resort: check current directory
        return os.path.join(os.path.dirname(__file__), 'bin')
    
    def _find_n_m3u8dl_re(self) -> str:
        """Find N_m3u8DL-RE binary"""
        binary_name = 'N_m3u8DL-RE'
        if os.name == 'nt':  # Windows
            binary_name += '.exe'
        
        # Check in bin directory
        path = os.path.join(self._bin_dir, binary_name)
        if os.path.exists(path):
            return path
        
        # Check in PATH
        result = subprocess.run(['which', binary_name], capture_output=True, text=True)
        if result.returncode == 0:
            return result.stdout.strip()
        
        raise DDdownloaderIntegrationError(f"N_m3u8DL-RE binary not found. Searched in: {self._bin_dir}")
    
    def _find_aria2c(self) -> str:
        """Find aria2c binary"""
        binary_name = 'aria2c'
        if os.name == 'nt':  # Windows
            binary_name += '.exe'
        
        # Check in bin directory
        path = os.path.join(self._bin_dir, binary_name)
        if os.path.exists(path):
            return path
        
        # Check in PATH
        result = subprocess.run(['which', 'aria2c'], capture_output=True, text=True)
        if result.returncode == 0:
            return result.stdout.strip()
        
        # Fallback: assume it's in PATH
        return 'aria2c'
    
    def _find_ffmpeg(self) -> str:
        """Find ffmpeg binary"""
        # Check in bin directory
        ffmpeg_name = 'ffmpeg.exe' if os.name == 'nt' else 'ffmpeg'
        path = os.path.join(self._bin_dir, ffmpeg_name)
        if os.path.exists(path):
            return path
        
        # Check system PATH
        result = subprocess.run(['which', 'ffmpeg'], capture_output=True, text=True)
        if result.returncode == 0:
            return result.stdout.strip()
        
        # Fallback to common locations
        common_paths = ['/usr/bin/ffmpeg', '/usr/local/bin/ffmpeg']
        for common_path in common_paths:
            if os.path.exists(common_path):
                return common_path
        
        return 'ffmpeg'  # Let it fail naturally if not found

    def _find_ffprobe(self) -> str:
        ffprobe_name = 'ffprobe.exe' if os.name == 'nt' else 'ffprobe'
        if self._ffmpeg_path:
            sibling = os.path.join(os.path.dirname(self._ffmpeg_path), ffprobe_name)
            if os.path.exists(sibling):
                return sibling

        result = subprocess.run(['which', 'ffprobe'], capture_output=True, text=True)
        if result.returncode == 0:
            return result.stdout.strip()

        for common_path in ['/usr/bin/ffprobe', '/usr/local/bin/ffprobe']:
            if os.path.exists(common_path):
                return common_path
        return ""
    
    def detect_url_type(self, url: str) -> str:
        """
        Detect stream type from URL and Content-Type headers.
        
        Detection order:
        1. URL extension check (.m3u8, .mpd, .ism, .mp4)
        2. HEAD request Content-Type check
        3. GET request fallback with streaming
        
        Args:
            url: The URL to check
            
        Returns:
            Stream type: "hls", "dash", "ism", "mp4", or "unknown"
        """
        logger.info(f"[DETECT] Checking URL type: {url[:60]}...")
        
        lower = url.lower()
        
        # 1. Check URL extension
        if ".m3u8" in lower:
            logger.info("[DETECT] Detected HLS from URL extension")
            return StreamType.HLS.value
        if ".mpd" in lower:
            logger.info("[DETECT] Detected DASH from URL extension")
            return StreamType.DASH.value
        if ".ism" in lower:
            logger.info("[DETECT] Detected ISM from URL extension")
            return StreamType.ISM.value
        if lower.endswith(".mp4") or ".mp4?" in lower or "/video.mp4" in lower:
            logger.info("[DETECT] Detected MP4 from URL extension")
            return StreamType.MP4.value
        
        # 2. Try HEAD request for Content-Type
        headers = {"User-Agent": USER_AGENT}
        try:
            logger.info("[DETECT] Trying HEAD request for Content-Type...")
            response = requests.head(url, allow_redirects=True, timeout=15, headers=headers)
            ct = (response.headers.get("Content-Type") or "").lower()
            logger.info(f"[DETECT] HEAD Content-Type: {ct}")
            
            if "mpegurl" in ct or "application/vnd.apple.mpegurl" in ct:
                logger.info("[DETECT] Detected HLS from Content-Type")
                return StreamType.HLS.value
            if "dash+xml" in ct:
                logger.info("[DETECT] Detected DASH from Content-Type")
                return StreamType.DASH.value
            if "video/mp4" in ct or "octet-stream" in ct or "application/mp4" in ct:
                logger.info("[DETECT] Detected MP4 from Content-Type")
                return StreamType.MP4.value
        except requests.exceptions.RequestException as e:
            logger.warning(f"[DETECT] HEAD request failed: {e}")
        
        # 3. Try GET request with streaming (only read first chunk)
        try:
            logger.info("[DETECT] Trying GET request for Content-Type...")
            response = requests.get(url, stream=True, allow_redirects=True, timeout=15, headers=headers)
            ct = (response.headers.get("Content-Type") or "").lower()
            logger.info(f"[DETECT] GET Content-Type: {ct}")
            
            if "mpegurl" in ct or "application/vnd.apple.mpegurl" in ct:
                logger.info("[DETECT] Detected HLS from GET Content-Type")
                return StreamType.HLS.value
            if "dash+xml" in ct:
                logger.info("[DETECT] Detected DASH from GET Content-Type")
                return StreamType.DASH.value
            if "video/mp4" in ct or "octet-stream" in ct or "application/mp4" in ct:
                logger.info("[DETECT] Detected MP4 from GET Content-Type")
                return StreamType.MP4.value
            
            # Try to detect from content (first 1KB)
            content_sample = next(response.iter_content(chunk_size=1024), b"")
            if b"#EXTM3U" in content_sample:
                logger.info("[DETECT] Detected HLS from content sample")
                return StreamType.HLS.value
            if b"<MPD" in content_sample or b"<manifest" in content_sample.lower():
                logger.info("[DETECT] Detected DASH from content sample")
                return StreamType.DASH.value
                
        except requests.exceptions.RequestException as e:
            logger.warning(f"[DETECT] GET request failed: {e}")
        
        logger.warning(f"[DETECT] Unknown stream type for URL: {url[:60]}...")
        return StreamType.UNKNOWN.value
    
    def validate_downloaded_file(self, path: str, min_size: int = 1024) -> tuple[bool, str]:
        """
        Validate that a downloaded file exists and is non-empty.
        
        Args:
            path: Path to the file to validate
            min_size: Minimum expected file size in bytes
            
        Returns:
            Tuple of (is_valid, error_message)
        """
        if not path:
            return False, "Path is empty"
        
        if not os.path.exists(path):
            return False, f"File does not exist: {path}"
        
        try:
            file_size = os.path.getsize(path)
        except OSError as e:
            return False, f"Cannot get file size: {e}"
        
        if file_size == 0:
            return False, f"File is empty: {path}"
        
        if file_size < min_size:
            return False, f"File size too small ({file_size} bytes): {path}"
        
        logger.info(f"[VALIDATE] File valid: {path} ({file_size} bytes)")
        return True, ""

    def validate_final_video_file(self, path: str, min_size: int = 10 * 1024 * 1024) -> tuple[bool, str]:
        is_valid, error_msg = self.validate_downloaded_file(path, min_size=min_size)
        if not is_valid:
            return False, error_msg

        if not self._ffprobe_path:
            return True, ""

        try:
            result = subprocess.run(
                [
                    self._ffprobe_path,
                    "-v", "error",
                    "-show_entries", "format=duration",
                    "-of", "default=noprint_wrappers=1:nokey=1",
                    path,
                ],
                capture_output=True,
                text=True,
                timeout=20,
                check=False,
            )
        except Exception as exc:
            logger.warning(f"[VALIDATE] ffprobe failed for {path}: {exc}")
            return True, ""

        if result.returncode != 0:
            logger.warning(f"[VALIDATE] ffprobe returned {result.returncode} for {path}: {result.stderr[:200]}")
            return True, ""

        try:
            duration_seconds = float((result.stdout or "").strip())
        except ValueError:
            return True, ""

        if duration_seconds <= 10:
            return False, f"video duration too short ({duration_seconds:.2f}s): {path}"
        return True, ""

    def _detect_final_download_file(self, save_dir: str, output_name: str) -> str:
        stem = os.path.splitext(output_name)[0]
        files = sorted(os.listdir(save_dir)) if os.path.isdir(save_dir) else []
        logger.info(f"[DOWNLOAD FILES] job_id={stem} files={files}")

        exact_mp4 = os.path.abspath(os.path.join(save_dir, f"{stem}.mp4"))
        exact_mux = os.path.abspath(os.path.join(save_dir, f"{stem}.MUX.mp4"))

        def valid(path: str) -> bool:
            return os.path.exists(path) and os.path.isfile(path) and os.path.getsize(path) > 0

        candidates = []
        if valid(exact_mp4):
            candidates.append(exact_mp4)
        if valid(exact_mux):
            candidates.append(exact_mux)

        for name in files:
            full = os.path.abspath(os.path.join(save_dir, name))
            if not name.startswith(stem) or not valid(full):
                continue
            if name.endswith(".MUX.mp4"):
                candidates.append(full)
        for name in files:
            full = os.path.abspath(os.path.join(save_dir, name))
            if not name.startswith(stem) or not valid(full):
                continue
            if name.endswith(".mp4"):
                candidates.append(full)

        matching_files = []
        for name in files:
            full = os.path.abspath(os.path.join(save_dir, name))
            if name.startswith(stem) and valid(full):
                matching_files.append(full)
        if matching_files:
            candidates.append(max(matching_files, key=os.path.getsize))

        seen = set()
        ordered = []
        for candidate in candidates:
            if candidate in seen:
                continue
            seen.add(candidate)
            ordered.append(candidate)

        if not ordered:
            raise DDdownloaderIntegrationError(
                f"N_m3u8DL-RE reported success but output file not found for stem={stem}"
            )

        final_path = ordered[0]
        if final_path.endswith(".MUX.mp4") and final_path != exact_mp4:
            logger.info(f"[DOWNLOAD RENAME] from={final_path} to={exact_mp4}")
            if os.path.exists(exact_mp4):
                os.remove(exact_mp4)
            os.replace(final_path, exact_mp4)
            final_path = exact_mp4

        logger.info(f"[DOWNLOAD FINAL DETECTED] job_id={stem} final={final_path}")
        return final_path

    def _cleanup_download_artifacts(self, save_dir: str, output_name: str, final_path: str):
        stem = os.path.splitext(output_name)[0]
        if not os.path.isdir(save_dir):
            return
        for name in os.listdir(save_dir):
            if not name.startswith(stem):
                continue
            path = os.path.abspath(os.path.join(save_dir, name))
            if path == os.path.abspath(final_path):
                continue
            if os.path.isdir(path):
                if name.endswith(".tmp") or name.endswith(".cache"):
                    shutil.rmtree(path, ignore_errors=True)
                continue
            if name.endswith((".ts", ".m4s", ".tmp", ".part", ".aria2", ".MUX.mp4")):
                try:
                    os.remove(path)
                except OSError:
                    pass
    
    def _download_with_ddownloader(
        self,
        url: str,
        output_path: str,
        stream_type: str,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        referer: Optional[str] = None,
        progress_callback=None,
        pid_callback=None,
        debug_callback=None,
    ) -> str:
        """
        Download HLS/DASH/ISM streams using N_m3u8DL-RE.
        """
        logger.info(f"[DDOWNLOADER] Starting manifest download: type={stream_type}, url={url[:60]}...")
        
        start_time = time.time()
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
        
        # Generate output name from path
        output_name = os.path.splitext(os.path.basename(output_path))[0]
        save_dir = output_dir if output_dir else self.download_dir
        
        # Build N_m3u8DL-RE command
        encoded_url = quote(url, safe=':/%')
        cmd = [
            self._n_m3u8dl_path,
            encoded_url,
            "--save-dir", save_dir,
            "--tmp-dir", save_dir,
            "--save-name", output_name,
            "--del-after-done",
            "--decryption-engine", "FFMPEG",
            "--decryption-binary-path", self._ffmpeg_path,
            "-mt",
            "-M", "format=mp4",
            "--log-level", "INFO",
        ]
        
        origin = _origin_for_referer(referer)
        if referer:
            cmd.extend(["-H", f"Referer: {referer}"])
            if origin:
                cmd.extend(["-H", f"Origin: {origin}"])

        cmd.extend(["-H", f"User-Agent: {USER_AGENT}"])
        cmd.extend(["-H", "Accept: */*"])
        cmd.extend(["-H", "Accept-Language: en-US,en;q=0.9"])
        cmd.extend(["-H", "Connection: keep-alive"])

        logger.info(f"--- [JOB {os.path.splitext(output_name)[0]}] DOWNLOAD START ---")
        logger.info(f"[JOB {os.path.splitext(output_name)[0]}] Command: {' '.join(cmd)}")
        _debug(debug_callback, "command", {
            "command": cmd,
            "command_string": " ".join(cmd),
            "output_path": output_path,
            "media_type": "m3u8" if stream_type == StreamType.HLS.value else ("mpd" if stream_type == StreamType.DASH.value else stream_type),
        })
        
        try:
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
                cwd=save_dir
            )
            
            if pid_callback:
                pid_callback(process.pid)
            _debug(debug_callback, "pid", {"pid": process.pid})
                
            job_slug = os.path.splitext(output_name)[0]
            logger.info(f"[JOB {job_slug}] Process PID: {process.pid}")

            downloaded_bytes = 0
            total_bytes = 0
            last_progress_reported = -1

            _stop_watchdog = threading.Event()
            _killed_for_stuck = threading.Event()
            last_activity_at = time.time()
            last_output_size = 0
            last_size_log_time = time.time()
            startup_logs = []

            def _watchdog():
                nonlocal last_activity_at, last_output_size, last_size_log_time
                while not _stop_watchdog.is_set():
                    if _stop_watchdog.wait(timeout=1):
                        break
                    
                    try:
                        sizes = []
                        if os.path.isdir(save_dir):
                            for entry in os.listdir(save_dir):
                                if entry.startswith(output_name) or entry.endswith(('.ts', '.m4s', '.tmp', '.mp4')):
                                    full = os.path.join(save_dir, entry)
                                    try:
                                        sizes.append(os.path.getsize(full))
                                    except OSError:
                                        pass
                        max_size = max(sizes) if sizes else 0
                    except OSError:
                        max_size = 0

                    if max_size > last_output_size:
                        last_output_size = max_size
                        last_activity_at = time.time()
                    
                    # Periodic size logging as requested (every 2 seconds)
                    now = time.time()
                    if now - last_size_log_time >= 2.0:
                        logger.info(f"[JOB {job_slug}] Current file size: {max_size:,} bytes (activity: {now - last_activity_at:.1f}s ago)")
                        _debug(debug_callback, "file_size", {"output_path": output_path, "file_size": max_size})
                        last_size_log_time = now

                    if time.time() - last_activity_at >= DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS:
                        logger.warning(
                            f"[JOB {job_slug}] NO PROGRESS TIMEOUT ({DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS}s) - killing process"
                        )
                        _killed_for_stuck.set()
                        try:
                            process.kill()
                        except Exception:
                            pass
                        return

            watchdog_thread = threading.Thread(target=_watchdog, daemon=True)
            watchdog_thread.start()

            # Parse stderr for progress information
            import re
            size_pattern = re.compile(r'(\d+(?:\.\d+)?)\s*(B|KB|MB|GB)', re.IGNORECASE)
            percent_pattern = re.compile(r'(\d+(?:\.\d+)?)\s*%')
            
            # Read stderr line by line while process runs
            while True:
                # Check if process has finished
                line = process.stderr.readline()
                if not line and process.poll() is not None:
                    break
                
                if line:
                    line = line.strip()
                    if line:
                        last_activity_at = time.time()
                        if len(startup_logs) < 50:
                            startup_logs.append(line)
                        _debug(debug_callback, "stderr", {"line": line})
                    
                    # Parse size information from N_m3u8DL-RE output
                    # Example: "Downloading: 123.45 MB / 456.78 MB"
                    size_match = size_pattern.search(line)
                    if size_match:
                        try:
                            size_val = float(size_match.group(1))
                            size_unit = size_match.group(2).upper()
                            
                            if size_unit == 'B':
                                size_bytes = size_val
                            elif size_unit == 'KB':
                                size_bytes = size_val * 1024
                            elif size_unit == 'MB':
                                size_bytes = size_val * 1024 * 1024
                            elif size_unit == 'GB':
                                size_bytes = size_val * 1024 * 1024 * 1024
                            
                            # Try to extract both downloaded and total
                            parts = line.split('/')
                            if len(parts) == 2:
                                # First part is downloaded, second is total
                                dl_match = size_pattern.search(parts[0])
                                if dl_match:
                                    dl_val = float(dl_match.group(1))
                                    dl_unit = dl_match.group(2).upper()
                                    if dl_unit == 'KB':
                                        dl_val *= 1024
                                    elif dl_unit == 'MB':
                                        dl_val *= 1024 * 1024
                                    elif dl_unit == 'GB':
                                        dl_val *= 1024 * 1024 * 1024
                                    downloaded_bytes = int(dl_val)
                                    total_bytes = int(size_bytes)
                                    
                                    # Calculate progress
                                    if total_bytes > 0:
                                        progress = int((downloaded_bytes / total_bytes) * 100)
                                        if progress > last_progress_reported:
                                            last_progress_reported = progress
                                            elapsed = time.time() - start_time
                                            speed = downloaded_bytes / elapsed if elapsed > 0 else 0
                                            eta = int((total_bytes - downloaded_bytes) / speed) if speed > 0 else 0
                                            
                                            if progress_callback:
                                                progress_callback(progress, downloaded_bytes, total_bytes, speed, eta)
                                            logger.info(f"[DDOWNLOADER] Progress: {progress}%, {downloaded_bytes}/{total_bytes} bytes")
                        except (ValueError, IndexError):
                            pass
                
                # Rate limit progress updates
                current_time = time.time()
                if current_time - last_update_time >= min_time_for_update:
                    last_update_time = current_time
                    # Still update progress even if we couldn't parse stderr
                    if progress_callback and total_bytes > 0:
                        elapsed = time.time() - start_time
                        speed = downloaded_bytes / elapsed if elapsed > 0 else 0
                        eta = int((total_bytes - downloaded_bytes) / speed) if speed > 0 else 0
                        progress = int((downloaded_bytes / total_bytes) * 100) if total_bytes > 0 else 0
                        progress_callback(progress, downloaded_bytes, total_bytes, speed, eta)
            
            # Wait for process to complete
            stdout, stderr = process.communicate()
            _stop_watchdog.set()
            watchdog_thread.join(timeout=2)
            _debug(debug_callback, "exit", {
                "exit_code": process.returncode,
                "stdout": stdout or "",
                "stderr": stderr or "\n".join(startup_logs),
            })

            if _killed_for_stuck.is_set():
                logger.error(f"[download] stderr={(stderr or '').strip() or '(empty)'}")
                raise DDdownloaderIntegrationError(
                    f"selected_url={url} inactivity_timeout={DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS}s stderr={(stderr or '').strip() or 'empty'}"
                )

            logger.info(f"[DOWNLOAD DONE] job_id={os.path.splitext(output_name)[0]} exit_code={process.returncode}")

            if process.returncode != 0:
                stderr_preview = stderr[:500] if stderr else "No stderr"
                logger.error(f"[download] stderr={stderr_preview}")
                logger.error(f"[DDOWNLOADER] N_m3u8DL-RE failed: {stderr_preview}")
                raise DDdownloaderIntegrationError(f"selected_url={url} n_m3u8dl_exit={process.returncode} stderr={stderr_preview}")

            logger.info(f"[DDOWNLOADER] N_m3u8DL-RE completed successfully")
            final_path = self._detect_final_download_file(save_dir, output_name)
            self._cleanup_download_artifacts(save_dir, output_name, final_path)
            return final_path
            
        except subprocess.TimeoutExpired:
            logger.error("[DDOWNLOADER] Download timed out")
            raise DDdownloaderIntegrationError("Manifest download timed out")
        except FileNotFoundError:
            logger.error(f"[DDOWNLOADER] N_m3u8DL-RE binary not found: {self._n_m3u8dl_path}")
            raise DDdownloaderIntegrationError("N_m3u8DL-RE binary not found")
        except Exception as e:
            logger.error(f"[DDOWNLOADER] Download failed: {e}")
            raise DDdownloaderIntegrationError(f"Manifest download failed: {e}")

    def _download_manifest_with_ffmpeg(
        self,
        url: str,
        output_path: str,
        stream_type: str,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        referer: Optional[str] = None,
        progress_callback=None,
        pid_callback=None,
        debug_callback=None,
    ) -> str:
        """
        Download HLS/DASH/ISM streams using ffmpeg directly as fallback.
        This is more reliable than N_m3u8DL-RE for certain URLs.
        
        Args:
            url: Stream manifest URL
            output_path: Output file path
            stream_type: Type of stream (hls, dash, ism)
            job_id: Internal job ID for progress
            backend_job_id: Backend job ID for progress
            referer: Optional referer header
            progress_callback: Optional callback for progress updates
            
        Returns:
            Path to downloaded file
            
        Raises:
            DDdownloaderIntegrationError: If download fails
        """
        logger.info(f"[FFMPEG] Starting manifest download with ffmpeg: type={stream_type}, url={url[:60]}...")
        
        start_time = time.time()
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
        
        # Build ffmpeg command for HLS download
        # URL-encode the URL to handle spaces and special characters
        encoded_url = quote(url, safe=':/%')
        cmd = [
            self._ffmpeg_path,
            "-y",  # Overwrite output
            "-hide_banner",
            "-loglevel", "info",
        ]
        
        # ffmpeg requires every custom header in a single -headers blob.
        # Origin is derived from any referer (not just uzmovi) so other
        # CDNs that gate playback on Origin will also serve bytes.
        header_lines = []
        origin = _origin_for_referer(referer)
        if referer:
            header_lines.append(f"Referer: {referer}")
            if origin:
                header_lines.append(f"Origin: {origin}")
        header_lines.append("Accept: */*")
        cmd.extend(["-headers", "\r\n".join(header_lines) + "\r\n"])
        cmd.extend(["-user_agent", USER_AGENT])

        logger.info(f"[DOWNLOAD] url={url}")
        logger.info(f"[HEADERS] referer={referer or '<none>'} origin={origin or '<none>'}")

        # Input URL (use encoded URL to handle spaces)
        cmd.extend(["-i", encoded_url])
        
        # Output options for MP4
        cmd.extend([
            "-c", "copy",  # Copy streams without re-encoding
            "-f", "mp4",
            "-bsf:a", "aac_adtstoasc",  # Fix AAC bitstream
            output_path
        ])
        
        logger.info(f"[FFMPEG] Executing: {' '.join(cmd[:10])}...")
        _debug(debug_callback, "command", {
            "command": cmd,
            "command_string": " ".join(cmd),
            "output_path": output_path,
            "media_type": "m3u8" if stream_type == StreamType.HLS.value else ("mpd" if stream_type == StreamType.DASH.value else stream_type),
        })
        
        # Progress tracking for ffmpeg downloads
        start_time = time.time()
        last_update_time = start_time
        min_time_for_update = 2.0  # Update progress every 2 seconds
        downloaded_bytes = 0
        total_bytes = 0
        last_progress_reported = -1
        total_duration_ms = 0
        
        # Parse patterns for ffmpeg output
        import re
        duration_pattern = re.compile(r'Duration: (\d+):(\d+):(\d+\.\d+)')
        time_pattern = re.compile(r'out_time_ms=(\d+)')
        
        try:
            # Run download with real-time stderr capture
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
                cwd=output_dir
            )
            if pid_callback:
                pid_callback(process.pid)
            _debug(debug_callback, "pid", {"pid": process.pid})

            # === Stuck-at-0 watchdog ===
            # ffmpeg writes the output mp4 incrementally; if it never grows,
            # the manifest fetch failed silently (TLS hang, redirect loop).
            # Kill after M3U8_STUCK_TIMEOUT_SECONDS so the next strategy fires.
            _stop_watchdog = threading.Event()
            _killed_for_stuck = threading.Event()

            def _watchdog():
                start = time.time()
                while not _stop_watchdog.is_set():
                    if _stop_watchdog.wait(timeout=1):
                        break
                    try:
                        size = os.path.getsize(output_path) if os.path.exists(output_path) else 0
                    except OSError:
                        size = 0
                    if size > 0:
                        return
                    if time.time() - start >= M3U8_STUCK_TIMEOUT_SECONDS:
                        logger.warning(
                            f"[FFMPEG] HLS produced 0 bytes after "
                            f"{M3U8_STUCK_TIMEOUT_SECONDS}s — killing ffmpeg"
                        )
                        _killed_for_stuck.set()
                        try:
                            process.kill()
                        except Exception:
                            pass
                        return

            watchdog_thread = threading.Thread(target=_watchdog, daemon=True)
            watchdog_thread.start()

            # Read stderr line by line while process runs
            while True:
                line = process.stderr.readline()
                if not line and process.poll() is not None:
                    break

                if line:
                    line = line.strip()
                    _debug(debug_callback, "stderr", {"line": line})

                    # Parse total duration from ffmpeg
                    duration_match = duration_pattern.search(line)
                    if duration_match:
                        hours = int(duration_match.group(1))
                        mins = int(duration_match.group(2))
                        secs = float(duration_match.group(3))
                        total_duration_ms = (hours * 3600 + mins * 60) * 1000 + int(secs * 1000)
                        logger.info(f"[FFMPEG] Total duration: {total_duration_ms}ms")
                    
                    # Parse current download time from ffmpeg progress
                    time_match = time_pattern.search(line)
                    if time_match and total_duration_ms > 0:
                        current_time_ms = int(time_match.group(1))
                        # Estimate progress based on time (ffmpeg downloads at ~1x speed for remux)
                        progress = min(int((current_time_ms / total_duration_ms) * 100), 99)
                        
                        if progress > last_progress_reported:
                            last_progress_reported = progress
                            elapsed = time.time() - start_time
                            speed = (current_time_ms / 1000) / elapsed if elapsed > 0 else 0  # bytes per second estimate
                            eta = int((total_duration_ms - current_time_ms) / 1000 / speed) if speed > 0 else 0
                            
                            # Estimate bytes based on speed and duration
                            estimated_total = int(speed * (total_duration_ms / 1000)) if speed > 0 else 0
                            estimated_downloaded = int(speed * elapsed) if speed > 0 else 0
                            
                            if progress_callback:
                                progress_callback(progress, estimated_downloaded, estimated_total, speed, eta)
                            logger.info(f"[FFMPEG] Progress: {progress}%, estimated {estimated_downloaded} bytes")
                
                # Rate limit progress updates
                current_time = time.time()
                if current_time - last_update_time >= min_time_for_update and last_progress_reported < 0:
                    last_update_time = current_time
                    # Report indeterminate progress for initial phase
                    if progress_callback:
                        elapsed = time.time() - start_time
                        progress_callback(-1, 0, 0, 0, 0)  # -1 indicates indeterminate
            
            # Wait for process to complete
            stdout, stderr = process.communicate()
            _stop_watchdog.set()
            watchdog_thread.join(timeout=2)
            combined_logs = "\n".join(part for part in ["\n".join(startup_logs), stdout or "", stderr or ""] if part).strip()
            _debug(debug_callback, "exit", {
                "exit_code": process.returncode,
                "stdout": stdout or "",
                "stderr": stderr or "",
            })

            if _killed_for_stuck.is_set():
                raise DDdownloaderIntegrationError(
                    f"ffmpeg HLS stalled at 0 bytes for {M3U8_STUCK_TIMEOUT_SECONDS}s — "
                    f"manifest unreachable or returned non-HLS body"
                )

            if process.returncode != 0:
                stderr_preview = stderr[:1000] if stderr else "No stderr"
                logger.error(f"[FFMPEG] ffmpeg failed: {stderr_preview}")
                raise DDdownloaderIntegrationError(f"ffmpeg failed with code {process.returncode}: {stderr_preview}")

            # Verify output file exists
            if not os.path.exists(output_path):
                raise DDdownloaderIntegrationError(f"ffmpeg reported success but output file not found: {output_path}")

            logger.info(f"[FFMPEG] Download completed: {output_path}")
            
            # Report completion
            file_size = os.path.getsize(output_path)
            if progress_callback:
                progress_callback(100, file_size, file_size, 0, 0)
            
            return output_path
            
        except subprocess.TimeoutExpired:
            logger.error("[FFMPEG] Download timed out")
            raise DDdownloaderIntegrationError("Manifest download timed out")
        except FileNotFoundError:
            logger.error(f"[FFMPEG] ffmpeg binary not found: {self._ffmpeg_path}")
            raise DDdownloaderIntegrationError("ffmpeg binary not found")
        except Exception as e:
            logger.error(f"[FFMPEG] Download failed: {e}")
            raise DDdownloaderIntegrationError(f"ffmpeg download failed: {e}")

    def _download_with_aria2c(
        self,
        url: str,
        output_path: str,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        referer: Optional[str] = None,
        progress_callback=None,
        pid_callback=None,
    ) -> str:
        """
        Download MP4 files using aria2c with parallel connections.
        """
        logger.info(f"[ARIA2C] Starting MP4 download: url={url[:60]}...")
        
        # Ensure output directory exists
        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)
        
        # Build aria2c command
        encoded_url = quote(url, safe=':/%')
        cmd = [
            self._aria2c_path,
            encoded_url,
            "-d", output_dir,
            "-o", os.path.basename(output_path),
            "-x", str(PARALLEL_CONNECTIONS),
            "-s", "4",
            "-k", "1M",
            "--file-allocation=none",
            "--continue=true",
            "--timeout=60",
            "--connect-timeout=30",
            "--retry-wait=5",
            "--max-tries=3",
            f"--header=User-Agent: {ARIA2C_USER_AGENT}",
        ]
        
        # Add referer and origin
        if referer:
            cmd.append(f"--header=Referer: {referer}")
            origin = _origin_for_referer(referer)
            if origin:
                cmd.append(f"--header=Origin: {origin}")
        
        cmd.extend(["--quiet=false"])
        
        job_slug = os.path.splitext(os.path.basename(output_path))[0]
        logger.info(f"--- [JOB {job_slug}] ARIA2C START ---")
        logger.info(f"[JOB {job_slug}] Command: {' '.join(cmd)}")
        _debug(debug_callback, "command", {
            "command": cmd,
            "command_string": " ".join(cmd),
            "output_path": output_path,
            "media_type": "mp4",
        })
        
        start_time = time.time()
        last_update_time = start_time
        min_time_for_update = 2.0
        downloaded_bytes = 0
        total_bytes = 0
        last_progress_reported = -1
        
        import re
        aria2c_pattern = re.compile(r'\[#[\da-f]+\s+([\d.]+\w+)/([\d.]+\w+)\s*\((\d+)%\).*DL:([\d.]+\w+)', re.IGNORECASE)
        size_pattern = re.compile(r'([\d.]+)\s*([KMGT]?i?B?)\s*/\s*([\d.]+)\s*([KMGT]?i?B?)', re.IGNORECASE)
        
        def to_bytes(val, unit):
            unit = unit.upper().replace('I', '').replace('B', 'B')
            if unit in ('B', ''): return val
            elif unit in ('KB', 'K'): return val * 1024
            elif unit in ('MB', 'M'): return val * 1024 * 1024
            elif unit in ('GB', 'G'): return val * 1024 * 1024 * 1024
            return val
        
        try:
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=1,
            )
            
            if pid_callback:
                pid_callback(process.pid)
            _debug(debug_callback, "pid", {"pid": process.pid})
                
            logger.info(f"[JOB {job_slug}] Process PID: {process.pid}")

            last_activity_at = time.time()
            last_output_size = 0
            last_size_log_time = time.time()
            _killed_for_stuck = threading.Event()
            _stop_watchdog = threading.Event()
            
            # Capture first 50 lines of output for debugging startup failures
            startup_logs = []
            stderr_lines = []

            def consume_stderr():
                for line in process.stderr:
                    if line:
                        line = line.strip()
                        _debug(debug_callback, "stderr", {"line": line})
                        if len(stderr_lines) < 50:
                            stderr_lines.append(line)
            
            stderr_thread = threading.Thread(target=consume_stderr, daemon=True)
            stderr_thread.start()

            def _watchdog():
                nonlocal last_activity_at, last_output_size, last_size_log_time
                while not _stop_watchdog.is_set():
                    if _stop_watchdog.wait(timeout=1):
                        break
                    try:
                        size = os.path.getsize(output_path) if os.path.exists(output_path) else 0
                    except OSError:
                        size = 0
                    if size > last_output_size:
                        last_output_size = size
                        last_activity_at = time.time()
                    
                    # Periodic size logging as requested (every 2 seconds)
                    now = time.time()
                    if now - last_size_log_time >= 2.0:
                        logger.info(f"[JOB {job_slug}] Current file size: {size:,} bytes (activity: {now - last_activity_at:.1f}s ago)")
                        _debug(debug_callback, "file_size", {"output_path": output_path, "file_size": size})
                        last_size_log_time = now

                    if time.time() - last_activity_at >= DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS:
                        logger.warning(f"[JOB {job_slug}] NO PROGRESS TIMEOUT ({DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS}s) - killing aria2c")
                        _killed_for_stuck.set()
                        try:
                            process.kill()
                        except Exception:
                            pass
                        return

            watchdog_thread = threading.Thread(target=_watchdog, daemon=True)
            watchdog_thread.start()
            
            # Read stdout line by line while process runs
            while True:
                line = process.stdout.readline()
                if not line and process.poll() is not None:
                    break
                
                if line:
                    line = line.strip()
                    _debug(debug_callback, "stdout", {"line": line})
                    if line:
                        last_activity_at = time.time()
                        if len(startup_logs) < 50:
                            startup_logs.append(line)
                    
                    # Parse full aria2c progress line: [#abc 12MiB/100MiB(12%) CN:4 DL:2.1MiB ETA:40s]
                    aria_match = aria2c_pattern.search(line)
                    if aria_match:
                        try:
                            dl_str = aria_match.group(1)
                            total_str = aria_match.group(2)
                            progress = int(aria_match.group(3))
                            speed_str = aria_match.group(4)
                            
                            # Parse size values with units
                            dl_match = re.match(r'([\d.]+)([KMGT]?i?B)?', dl_str, re.IGNORECASE)
                            total_match = re.match(r'([\d.]+)([KMGT]?i?B)?', total_str, re.IGNORECASE)
                            speed_match = re.match(r'([\d.]+)([KMGT]?i?B)?', speed_str, re.IGNORECASE)
                            
                            if dl_match and total_match:
                                downloaded_bytes = int(to_bytes(float(dl_match.group(1)), dl_match.group(2) or 'B'))
                                total_bytes = int(to_bytes(float(total_match.group(1)), total_match.group(2) or 'B'))
                                speed = to_bytes(float(speed_match.group(1)), speed_match.group(2) or 'B') if speed_match else 0
                                eta = int((total_bytes - downloaded_bytes) / speed) if speed > 0 else 0
                                
                                if progress_callback:
                                    progress_callback(progress, downloaded_bytes, total_bytes, speed, eta)
                                logger.info(f"[ARIA2C] Progress: {progress}%, {downloaded_bytes}/{total_bytes} bytes, {speed} bytes/s")
                        except (ValueError, IndexError, AttributeError):
                            pass
                    else:
                        # Fallback: parse size information
                        size_match = size_pattern.search(line)
                        if size_match:
                            try:
                                dl_val = float(size_match.group(1))
                                dl_unit = size_match.group(2).upper()
                                total_val = float(size_match.group(3))
                                total_unit = size_match.group(4).upper()
                                
                                downloaded_bytes = int(to_bytes(dl_val, dl_unit))
                                total_bytes = int(to_bytes(total_val, total_unit))
                            except (ValueError, IndexError):
                                pass
                
                # Rate limit progress updates based on time
                current_time = time.time()
                if current_time - last_update_time >= min_time_for_update:
                    last_update_time = current_time
                    elapsed = time.time() - start_time
                    if total_bytes > 0:
                        progress = int((downloaded_bytes / total_bytes) * 100)
                        speed = downloaded_bytes / elapsed if elapsed > 0 else 0
                        eta = int((total_bytes - downloaded_bytes) / speed) if speed > 0 else 0
                        if progress_callback and progress > last_progress_reported:
                            last_progress_reported = progress
                            progress_callback(progress, downloaded_bytes, total_bytes, speed, eta)
            
            # Wait for process to complete
            process.wait()
            _stop_watchdog.set()
            watchdog_thread.join(timeout=2)
            stderr_thread.join(timeout=2)
            
            combined_logs = "\n".join(part for part in ["\n".join(startup_logs), "\n".join(stderr_lines)] if part).strip()
            _debug(debug_callback, "exit", {
                "exit_code": process.returncode,
                "stdout": "",
                "stderr": "\n".join(stderr_lines),
            })

            if _killed_for_stuck.is_set():
                logs_summary = "\n".join(startup_logs)
                logger.error(f"[JOB {job_slug}] STUCK LOGS:\n{logs_summary}")
                raise DDdownloaderIntegrationError(
                    f"selected_url={url} inactivity_timeout={DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS}s startup_logs={logs_summary[:1000]}"
                )
            
            # Check if file was created
            if not os.path.exists(output_path):
                logs_summary = combined_logs
                logger.error(f"[JOB {job_slug}] FAILED LOGS:\n{logs_summary}")
                raise DDdownloaderIntegrationError(f"aria2c failed to create file: {logs_summary[:1000]}")
            
            # Check if download was successful (aria2c returns 0 on success)
            if process.returncode != 0:
                logs_summary = combined_logs
                logger.warning(f"[JOB {job_slug}] aria2c returned {process.returncode}, but file exists")
                if os.path.getsize(output_path) > 0:
                    logger.warning(f"[JOB {job_slug}] File exists with content, continuing...")
                else:
                    logger.error(f"[JOB {job_slug}] FAILED LOGS:\n{logs_summary}")
                    raise DDdownloaderIntegrationError(f"aria2c failed with code {process.returncode}: {logs_summary[:1000]}")
            
            logger.info(f"[ARIA2C] Download completed: {output_path}")
            
            # Report completion
            file_size = os.path.getsize(output_path)
            if progress_callback:
                progress_callback(100, file_size, file_size, 0, 0)
            
            return output_path
            
        except subprocess.TimeoutExpired:
            logger.error("[ARIA2C] Download timed out")
            raise DDdownloaderIntegrationError("MP4 download timed out")
        except FileNotFoundError:
            logger.error("[ARIA2C] aria2c not found")
            raise DDdownloaderIntegrationError("aria2c not found")
        except Exception as e:
            logger.error(f"[ARIA2C] Download failed: {e}")
            raise DDdownloaderIntegrationError(f"MP4 download failed: {e}")

    def _download_with_curl(
        self,
        url: str,
        output_path: str,
        referer: Optional[str] = None,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        progress_callback=None,
        pid_callback=None,
        debug_callback=None,
    ) -> str:
        """
        Download a direct MP4 URL using curl with browser-like headers.
        """
        DEBUG = os.environ.get("PARSER_DEBUG", "").lower() == "true"

        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)

        # Remove stale partial file so size tracking starts from 0
        if os.path.exists(output_path):
            try:
                os.remove(output_path)
            except OSError:
                pass

        # safe="/%" preserves slash separators and existing escapes.
        encoded_url = quote(url, safe=':/%')
        cmd = [
            "curl",
            "-L",                        # follow redirects
            "--fail",                    # non-zero exit on HTTP 4xx/5xx
            "--connect-timeout", "15",   # fail fast if server unreachable
            "--speed-limit", "51200",    # abort if throughput drops below 50 KB/s…
            "--speed-time", "60",        # …for 60 consecutive seconds (stall detection)
            "--retry", "2",
            "--retry-delay", "3",
            "-A", USER_AGENT,
        ]
        if referer:
            cmd.extend(["-e", referer])
        cmd.extend(["-o", output_path, encoded_url])

        job_slug = os.path.splitext(os.path.basename(output_path))[0]
        logger.info(f"--- [JOB {job_slug}] CURL START ---")
        logger.info(f"[JOB {job_slug}] Command: {' '.join(cmd)}")
        _debug(debug_callback, "command", {
            "command": cmd,
            "command_string": " ".join(cmd),
            "output_path": output_path,
            "media_type": "mp4",
        })

        # How long (seconds) with zero byte growth before we consider the download stalled.
        INACTIVITY_LIMIT = 120

        process = None
        _stop_monitor = threading.Event()
        _killed_for_inactivity = threading.Event()

        def _monitor_progress():
            """
            Poll output file size every 3 s.
            - Reports progress via progress_callback.
            - Kills the process if no new bytes arrive for INACTIVITY_LIMIT seconds.
            """
            total = 0
            try:
                head = requests.head(
                    url, headers={"User-Agent": USER_AGENT}, timeout=10, allow_redirects=True
                )
                total = int(head.headers.get("Content-Length", 0))
            except Exception:
                pass

            start = time.time()
            last_pct = -1
            prev_downloaded = 0
            last_progress_time = time.time()

            while not _stop_monitor.is_set():
                time.sleep(3)
                if _stop_monitor.is_set():
                    break

                try:
                    downloaded = os.path.getsize(output_path) if os.path.exists(output_path) else 0
                except OSError:
                    downloaded = 0

                # Update inactivity clock whenever bytes grow
                if downloaded > prev_downloaded:
                    prev_downloaded = downloaded
                    last_progress_time = time.time()
                else:
                    idle_secs = time.time() - last_progress_time
                    if idle_secs > INACTIVITY_LIMIT:
                        logger.warning(
                            f"[CURL] No progress for {idle_secs:.0f}s "
                            f"(downloaded={downloaded:,} bytes) — killing process"
                        )
                        _killed_for_inactivity.set()
                        try:
                            process.kill()
                        except Exception:
                            pass
                        _stop_monitor.set()
                        break

                elapsed = time.time() - start
                speed = downloaded / elapsed if elapsed > 0 else 0
                pct = int(downloaded / total * 100) if total > 0 else 0
                pct = min(pct, 99)
                eta = int((total - downloaded) / speed) if speed > 0 and total > downloaded else 0

                logger.info(
                    f"[CURL] Progress: {pct}% — {downloaded:,}/{total:,} bytes "
                    f"@ {speed/1024/1024:.1f} MB/s ETA {eta}s"
                )
                _debug(debug_callback, "file_size", {"output_path": output_path, "file_size": downloaded})
                if progress_callback and pct > last_pct:
                    last_pct = pct
                    progress_callback(pct, downloaded, total, speed, eta)

        monitor_thread = threading.Thread(target=_monitor_progress, daemon=True)

        try:
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
                text=True,
            )
            
            if pid_callback:
                pid_callback(process.pid)
            _debug(debug_callback, "pid", {"pid": process.pid})
                
            logger.info(f"[CURL] Process PID: {process.pid}")

            monitor_thread.start()

            # Python safety net only — real stall detection is in _monitor_progress above.
            # curl's --speed-limit/--speed-time also catches slow-but-not-zero cases.
            try:
                _, stderr = process.communicate(timeout=7200)
            except subprocess.TimeoutExpired:
                process.kill()
                try:
                    process.communicate(timeout=5)
                except Exception:
                    pass
                raise DDdownloaderIntegrationError(
                    "curl download safety-net timeout after 7200 s"
                )
            finally:
                _stop_monitor.set()
                monitor_thread.join(timeout=5)

            if _killed_for_inactivity.is_set():
                raise DDdownloaderIntegrationError(
                    f"curl stalled: no bytes received for {INACTIVITY_LIMIT}s"
                )

            rc = process.returncode
            _debug(debug_callback, "exit", {
                "exit_code": rc,
                "stdout": "",
                "stderr": stderr or "",
            })
            if DEBUG or rc != 0:
                logger.info(f"[CURL] Exit code: {rc}")
                if stderr:
                    logger.info(f"[CURL] stderr: {stderr[:400]}")

            if rc != 0:
                raise DDdownloaderIntegrationError(
                    f"curl failed (exit {rc}): {(stderr or 'no stderr')[:200]}"
                )

            if not os.path.exists(output_path) or os.path.getsize(output_path) == 0:
                raise DDdownloaderIntegrationError(
                    "curl exited 0 but output file is missing or empty"
                )

            file_size = os.path.getsize(output_path)
            logger.info(f"[CURL] Download complete: {output_path} ({file_size:,} bytes)")
            if progress_callback:
                progress_callback(100, file_size, file_size, 0, 0)
            return output_path

        except FileNotFoundError:
            raise DDdownloaderIntegrationError("curl not found on this system")
        except DDdownloaderIntegrationError:
            raise
        except Exception as e:
            raise DDdownloaderIntegrationError(f"curl download error: {e}")
        finally:
            _stop_monitor.set()

    def _download_mp4_with_ffmpeg(
        self,
        url: str,
        output_path: str,
        referer: Optional[str] = None,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        progress_callback=None,
        pid_callback=None,
    ) -> str:
        """
        Download a direct MP4 URL using ffmpeg (primary tool for signed CDN URLs).
        """
        logger.info(f"[FFMPEG-MP4] Starting direct MP4 download: url={url[:80]}...")

        output_dir = os.path.dirname(output_path)
        if output_dir:
            os.makedirs(output_dir, exist_ok=True)

        # Build combined -headers string (ffmpeg requires all custom headers in one -headers arg)
        headers_str = f"User-Agent: {USER_AGENT}\r\n"
        if referer:
            headers_str += f"Referer: {referer}\r\n"
            origin = _origin_for_referer(referer)
            if origin:
                headers_str += f"Origin: {origin}\r\n"

        # safe="/%" preserves slash separators and existing escapes.
        encoded_url = quote(url, safe=':/%')
        cmd = [
            self._ffmpeg_path,
            "-y",
            "-loglevel", "error",
            "-stats",
            "-headers", headers_str,
            "-i", encoded_url,
            "-c", "copy",
            output_path,
        ]

        job_slug = os.path.splitext(os.path.basename(output_path))[0]
        logger.info(f"--- [JOB {job_slug}] FFMPEG-MP4 START ---")
        logger.info(f"[JOB {job_slug}] Command: {' '.join(cmd)}")
        _debug(debug_callback, "command", {
            "command": cmd,
            "command_string": " ".join(cmd),
            "output_path": output_path,
            "media_type": "mp4",
        })

        # HEAD request for Content-Length so we can compute percentage
        total_bytes = 0
        try:
            head = requests.head(
                url,
                headers={"User-Agent": USER_AGENT},
                timeout=10,
                allow_redirects=True,
            )
            total_bytes = int(head.headers.get("Content-Length", 0))
        except Exception:
            pass

        # Kill ffmpeg if no new bytes arrive for too long.
        INACTIVITY_LIMIT = DOWNLOAD_INACTIVITY_TIMEOUT_SECONDS

        try:
            process = subprocess.Popen(
                cmd,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
                text=True,
            )
            
            if pid_callback:
                pid_callback(process.pid)
            _debug(debug_callback, "pid", {"pid": process.pid})
                
            logger.info(f"[FFMPEG-MP4] Process PID: {process.pid}")
            start_time = time.time()
            stderr_lines = []
            _stop_watchdog = threading.Event()
            _killed_for_inactivity = threading.Event()

            # Shared mutable last-activity timestamp (updated by _read_stderr)
            last_active = [time.time()]

            def _read_stderr():
                """Read ffmpeg stderr; parse -stats progress; update last_active."""
                try:
                    for raw in process.stderr:
                        for line in raw.replace("\r", "\n").split("\n"):
                            line = line.strip()
                            if not line:
                                continue
                            stderr_lines.append(line)
                            _debug(debug_callback, "stderr", {"line": line})

                            # Stats line: size= 102400kB time=00:01:23.00 ...
                            m = re.search(r"size=\s*(\d+)kB", line)
                            if m:
                                downloaded = int(m.group(1)) * 1024
                                last_active[0] = time.time()  # bytes are moving
                                elapsed = time.time() - start_time
                                speed = downloaded / elapsed if elapsed > 0 else 0
                                pct = int(downloaded / total_bytes * 100) if total_bytes > 0 else 0
                                pct = min(pct, 99)
                                eta = (
                                    int((total_bytes - downloaded) / speed)
                                    if speed > 0 and total_bytes > downloaded
                                    else 0
                                )
                                logger.info(
                                    f"[FFMPEG-MP4] Progress: {pct}% — "
                                    f"{downloaded:,}/{total_bytes:,} bytes "
                                    f"@ {speed/1024/1024:.1f} MB/s ETA {eta}s"
                                )
                                _debug(debug_callback, "file_size", {"output_path": output_path, "file_size": downloaded})
                                if progress_callback:
                                    progress_callback(pct, downloaded, total_bytes, speed, eta)
                except Exception:
                    pass

            def _watchdog():
                """
                Kill ffmpeg if no stats output for INACTIVITY_LIMIT seconds.
                Falls back to polling output file size in case ffmpeg stops
                printing stats but is somehow still writing bytes.
                """
                prev_file_size = 0
                while not _stop_watchdog.is_set():
                    _stop_watchdog.wait(timeout=10)
                    if _stop_watchdog.is_set():
                        break

                    # Also check file size growth as a secondary activity signal
                    try:
                        cur_size = os.path.getsize(output_path) if os.path.exists(output_path) else 0
                    except OSError:
                        cur_size = 0
                    if cur_size > prev_file_size:
                        prev_file_size = cur_size
                        last_active[0] = time.time()

                    idle_secs = time.time() - last_active[0]
                    if idle_secs > INACTIVITY_LIMIT:
                        logger.warning("[download] no progress timeout killed process")
                        _killed_for_inactivity.set()
                        try:
                            process.kill()
                        except Exception:
                            pass
                        break

            stderr_thread = threading.Thread(target=_read_stderr, daemon=True)
            watchdog_thread = threading.Thread(target=_watchdog, daemon=True)
            stderr_thread.start()
            watchdog_thread.start()

            try:
                # Python safety net only — real stall detection is in _watchdog above.
                process.wait(timeout=7200)
            except subprocess.TimeoutExpired:
                process.kill()
                process.wait()
                raise DDdownloaderIntegrationError(
                    "ffmpeg MP4 download safety-net timeout after 7200 s"
                )
            finally:
                _stop_watchdog.set()
                stderr_thread.join(timeout=5)
                watchdog_thread.join(timeout=5)

            if _killed_for_inactivity.is_set():
                snippet = "\n".join(stderr_lines[-10:]) or "no stderr"
                logger.error(f"[download] stderr={snippet[:1000]}")
                raise DDdownloaderIntegrationError(
                    f"selected_url={url} inactivity_timeout={INACTIVITY_LIMIT}s stderr={snippet[:1000]}"
                )

            rc = process.returncode
            logger.info(f"[FFMPEG-MP4] Exit code: {rc}")
            _debug(debug_callback, "exit", {
                "exit_code": rc,
                "stdout": "",
                "stderr": "\n".join(stderr_lines[-50:]),
            })
            if rc != 0:
                snippet = "\n".join(stderr_lines[-10:]) or "no stderr"
                logger.error(f"[download] stderr={snippet[:1000]}")
                logger.error(f"[FFMPEG-MP4] stderr tail: {snippet[:400]}")
                raise DDdownloaderIntegrationError(f"selected_url={url} ffmpeg_exit={rc} stderr={snippet[:200]}")

            if not os.path.exists(output_path) or os.path.getsize(output_path) == 0:
                raise DDdownloaderIntegrationError(
                    "ffmpeg exited 0 but output file is missing or empty"
                )

            file_size = os.path.getsize(output_path)
            logger.info(f"[FFMPEG-MP4] Complete: {output_path} ({file_size:,} bytes)")
            if progress_callback:
                progress_callback(100, file_size, file_size, 0, 0)
            return output_path

        except FileNotFoundError:
            raise DDdownloaderIntegrationError(f"ffmpeg not found: {self._ffmpeg_path}")
        except DDdownloaderIntegrationError:
            raise
        except Exception as e:
            raise DDdownloaderIntegrationError(f"ffmpeg MP4 download error: {e}")

    def smart_download(
        self,
        url: str,
        output_name: str,
        job_id: Optional[str] = None,
        backend_job_id: Optional[str] = None,
        referer: Optional[str] = None,
        max_retries: int = 3,
        pid_callback=None,
        debug_callback=None,
    ) -> DownloadResult:
        """
        Main download entry point with retry support and validation.
        
        Downloads video based on detected stream type:
        - HLS/DASH/ISM: Uses N_m3u8DL-RE (DDownloader)
        - MP4: Uses aria2c
        
        Args:
            url: Video URL
            output_name: Output filename (with or without extension)
            job_id: Internal job ID for progress
            backend_job_id: Backend job ID for progress
            referer: Optional referer header
            max_retries: Maximum retry attempts (default 3)
            
        Returns:
            DownloadResult with success status and file information
        """
        logger.info(f"[DDOWNLOADER] smart_download called: url={url[:50]}..., output_name={output_name}")
        logger.info(f"[DDOWNLOADER] job_id={job_id}, backend_job_id={backend_job_id}, max_retries={max_retries}")
        
        start_time = time.time()
        
        # Ensure download directory exists
        os.makedirs(self.download_dir, exist_ok=True)
        
        # Clean up extension for consistency
        if not output_name.endswith('.mp4'):
            output_name += '.mp4'
        
        output_path = os.path.join(self.download_dir, output_name)
        logger.info(f"[DDOWNLOADER] Output path: {output_path}")
        
        # Clean up any existing file
        if os.path.exists(output_path):
            logger.info(f"[DDOWNLOADER] Removing existing file: {output_path}")
            try:
                os.remove(output_path)
            except OSError:
                pass
        
        # Detect stream type
        stream_type = self.detect_url_type(url)
        logger.info(f"[DDOWNLOADER] Detected stream type: {stream_type}")
        
        # Fail fast for unknown types
        if stream_type == StreamType.UNKNOWN.value:
            return DownloadResult(
                success=False,
                source=url,
                video_type=stream_type,
                output_filename=output_name,
                error=f"Unknown stream type. Cannot determine how to download: {url}",
                download_duration=time.time() - start_time
            )
        
        # Create progress callback for real-time progress reporting
        # Note: Internal methods call this with positional args: (progress, downloaded, total, speed, eta)
        def progress_callback(progress_percent: int = 0, downloaded_bytes: int = 0, 
                              total_bytes: int = 0, speed_bytes_per_sec: float = 0, 
                              eta_seconds: int = 0):
            """
            Callback to handle download progress and report to backend.
            This is called periodically during the download.
            
            Args:
                progress_percent: Download progress percentage (0-100)
                downloaded_bytes: Number of bytes downloaded so far
                total_bytes: Total file size in bytes (0 if unknown)
                speed_bytes_per_sec: Current download speed in bytes/sec
                eta_seconds: Estimated time remaining in seconds
            """
            if not backend_job_id:
                return
            
            try:
                # Import here to avoid circular imports
                from downloader_service import report_progress_to_backend
                
                # Convert bytes/sec to MB/sec for backend compatibility
                speed_mbps = speed_bytes_per_sec / (1024 * 1024) if speed_bytes_per_sec else 0
                
                # Determine status and message based on progress
                if total_bytes > 0:
                    status = "downloading"
                    message = f"Downloading... {progress_percent}% ({downloaded_bytes:,} / {total_bytes:,} bytes)"
                else:
                    status = "downloading"
                    message = f"Downloading... {downloaded_bytes:,} bytes"
                
                # Build progress payload for backend
                payload = {
                    "stage": "download",
                    "status": status,
                    "progress": progress_percent,
                    "downloaded_bytes": downloaded_bytes,
                    "total_bytes": total_bytes,
                    "speed_mbps": speed_mbps,  # Backend expects speed_mbps
                    "message": message,
                }
                
                # Report to backend
                report_progress_to_backend(backend_job_id, payload)
                logger.debug(f"[DDOWNLOADER] Progress reported: {progress_percent}%, {downloaded_bytes:,}/{total_bytes:,} bytes")
                
            except Exception as e:
                logger.warning(f"[DDOWNLOADER] Failed to report progress: {e}")
        
        # Retry loop with exponential backoff
        last_error = None
        for attempt in range(1, max_retries + 1):
            attempt_start = time.time()
            
            if attempt > 1:
                wait_time = 2 ** (attempt - 1)
                logger.info(f"[DDOWNLOADER] Retry {attempt}/{max_retries} after {wait_time}s backoff")
                time.sleep(wait_time)
            
            try:
                logger.info(f"[DDOWNLOADER] Download attempt {attempt}/{max_retries}")
                
                if stream_type in (StreamType.HLS.value, StreamType.DASH.value, StreamType.ISM.value):
                    # Use DDownloader's N_m3u8DL-RE for manifest streams
                    try:
                        path = self._download_with_ddownloader(
                            url, output_path, stream_type,
                            job_id, backend_job_id, referer,
                            progress_callback=progress_callback,
                            pid_callback=pid_callback,
                            debug_callback=debug_callback,
                        )
                    except DDdownloaderIntegrationError as e:
                        # Fallback to ffmpeg if N_m3u8DL-RE fails
                        logger.warning(f"[DDOWNLOADER] N_m3u8DL-RE failed, trying ffmpeg fallback: {e}")
                        path = self._download_manifest_with_ffmpeg(
                            url, output_path, stream_type,
                            job_id, backend_job_id, referer,
                            progress_callback=progress_callback,
                            pid_callback=pid_callback,
                            debug_callback=debug_callback,
                        )
                elif stream_type == StreamType.MP4.value:
                    # Fallback chain: aria2c → ffmpeg → curl
                    mp4_error = None

                    # 1. aria2c
                    try:
                        path = self._download_with_aria2c(
                            url, output_path,
                            job_id, backend_job_id, referer,
                            progress_callback=progress_callback,
                            pid_callback=pid_callback,
                            debug_callback=debug_callback,
                        )
                        mp4_error = None
                    except DDdownloaderIntegrationError as aria_err:
                        mp4_error = aria_err
                        logger.warning(f"[DDOWNLOADER] aria2c failed ({aria_err}), trying ffmpeg")

                    # 2. ffmpeg (if aria2c failed)
                    if mp4_error is not None:
                        try:
                            path = self._download_mp4_with_ffmpeg(
                                url, output_path,
                                referer=referer,
                                job_id=job_id,
                                backend_job_id=backend_job_id,
                                progress_callback=progress_callback,
                                pid_callback=pid_callback,
                                debug_callback=debug_callback,
                            )
                            mp4_error = None
                        except DDdownloaderIntegrationError as ffmpeg_err:
                            mp4_error = ffmpeg_err
                            logger.warning(f"[DDOWNLOADER] ffmpeg failed ({ffmpeg_err}), trying curl")

                    # 3. curl
                    if mp4_error is not None:
                        path = self._download_with_curl(
                            url, output_path,
                            referer=referer,
                            job_id=job_id,
                            backend_job_id=backend_job_id,
                            progress_callback=progress_callback,
                            pid_callback=pid_callback,
                            debug_callback=debug_callback,
                        )
                else:
                    raise DDdownloaderIntegrationError(f"Unsupported stream type: {stream_type}")
                
                # Validate downloaded file
                is_valid, error_msg = self.validate_final_video_file(path)
                if not is_valid:
                    raise DDdownloaderIntegrationError(f"Validation failed: {error_msg}")
                
                # Get final file info
                file_size = os.path.getsize(path)
                download_duration = time.time() - start_time
                
                logger.info(f"[DOWNLOAD VALIDATED] job_id={os.path.splitext(output_name)[0]} size={file_size}")
                logger.info(f"[DDOWNLOADER] Download validated successfully: {path} ({file_size} bytes)")
                path = os.path.abspath(path)
                
                return DownloadResult(
                    success=True,
                    source=url,
                    video_type=stream_type,
                    local_path=path,
                    output_filename=output_name,
                    file_size=file_size,
                    download_duration=download_duration
                )
                
            except DDdownloaderIntegrationError as e:
                last_error = e
                logger.error(f"[DDOWNLOADER] Attempt {attempt} failed: {e}")
                
                # Clean up partial download
                if os.path.exists(output_path):
                    try:
                        os.remove(output_path)
                    except OSError:
                        pass
                
                if attempt == max_retries:
                    break
                    
            except Exception as e:
                last_error = DDdownloaderIntegrationError(f"Unexpected error: {e}")
                logger.error(f"[DDOWNLOADER] Attempt {attempt} unexpected error: {e}")
                
                if os.path.exists(output_path):
                    try:
                        os.remove(output_path)
                    except OSError:
                        pass
                
                if attempt == max_retries:
                    break
        
        # All retries exhausted — report failure so the backend job leaves "Downloading" state
        download_duration = time.time() - start_time
        error_msg = f"Download failed after {max_retries} attempts. Last error: {last_error}"
        logger.error(f"[DDOWNLOADER] {error_msg}")

        if backend_job_id:
            try:
                from downloader_service import report_progress_to_backend
                report_progress_to_backend(backend_job_id, {
                    "stage": "download",
                    "status": "failed",
                    "progress": 0,
                    "message": error_msg,
                    "error": error_msg,
                })
            except Exception:
                pass

        return DownloadResult(
            success=False,
            source=url,
            video_type=stream_type,
            output_filename=output_name,
            error=error_msg,
            download_duration=download_duration
        )


# Convenience function for quick usage
def download_video(url: str, output_name: str, download_dir: str = "downloads") -> dict:
    """
    Convenience function to download a video.
    
    Args:
        url: Video URL
        output_name: Output filename
        download_dir: Download directory
        
    Returns:
        Dictionary with download result
    """
    downloader = DDownloaderIntegration(download_dir=download_dir)
    result = downloader.smart_download(url, output_name)
    return result.to_dict()


if __name__ == "__main__":
    # Simple test
    logging.basicConfig(level=logging.INFO)
    
    if len(sys.argv) > 1:
        test_url = sys.argv[1]
        test_output = sys.argv[2] if len(sys.argv) > 2 else "test_video.mp4"
        
        print(f"Testing download: {test_url}")
        result = download_video(test_url, test_output)
        print(json.dumps(result, indent=2))
    else:
        print("Usage: python ddownloader_integration.py <url> [output_name]")
