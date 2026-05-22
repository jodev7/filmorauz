"""
Parser HTTP API Server
Provides REST API for Go backend to call parsers
"""
import json
import logging
import os
import random
import re
import shutil
import subprocess
import time
import urllib.request
import urllib.error
import threading
import socketserver
from http.server import HTTPServer, BaseHTTPRequestHandler
from pathlib import Path
from urllib.parse import urlparse, parse_qs
from typing import Optional
import sys

try:
    git_hash = subprocess.check_output(["git", "rev-parse", "--short", "HEAD"]).decode("utf-8").strip()
except Exception:
    git_hash = "unknown"

logger = logging.getLogger(__name__)
logger.info(f"[PARSER] Starting version={git_hash} built_at={time.strftime('%Y-%m-%d %H:%M:%S')}")
import hashlib
import requests


# Safe string truncation - never raises IndexError
# Process start time used by /healthz to report parser process uptime.
_PROCESS_START_TS = time.time()


def _host_health_snapshot() -> dict:
    """Return a HostStatus-compatible dict for the parser host.

    Format mirrors backend/handlers/system_handler.go HostStatus so the
    admin dashboard renders both hosts uniformly. psutil is the only
    extra dependency; if it's missing the snapshot still returns
    something useful (services map, error string) so the panel can show
    a degraded state instead of crashing.
    """
    import socket
    snap: dict = {
        "ok": True,
        "service": "parser",
        "host": os.environ.get("PUBLIC_HOST", "").strip() or socket.gethostname(),
        "hostname": socket.gethostname(),
        "process_uptime_seconds": int(time.time() - _PROCESS_START_TS),
        "services": {"parser": "up"},
        "checks": {},
        "checked_at": time.strftime("%Y-%m-%dT%H:%M:%SZ", time.gmtime()),
    }
    try:
        import psutil  # type: ignore
        snap["uptime_seconds"] = int(time.time() - psutil.boot_time())
        snap["cpu_percent"] = round(psutil.cpu_percent(interval=0.2), 1)
        snap["cpu_cores"] = psutil.cpu_count(logical=True) or 0
        vm = psutil.virtual_memory()
        snap["memory"] = {
            "total_mb": int(vm.total / (1024 * 1024)),
            "used_mb": int(vm.used / (1024 * 1024)),
            "percent": round(vm.percent, 1),
        }
        du = psutil.disk_usage("/")
        snap["disk"] = {
            "total_gb": int(du.total / (1024 ** 3)),
            "used_gb": int(du.used / (1024 ** 3)),
            "percent": round(du.percent, 1),
        }
        try:
            la = psutil.getloadavg()
            snap["load_avg"] = [round(la[0], 2), round(la[1], 2), round(la[2], 2)]
        except (AttributeError, OSError):
            snap["load_avg"] = [0.0, 0.0, 0.0]
    except ImportError:
        snap["checks"]["psutil"] = "missing — host metrics unavailable"
    except Exception as exc:  # noqa: BLE001 — never let metrics break the health response
        snap["checks"]["psutil_error"] = str(exc)

    # Worker lives on the same VPS — probe its local port to decide if
    # it's up. Default port matches worker/main.go (WORKER_HTTP_PORT=8083).
    worker_port = os.environ.get("WORKER_HTTP_PORT", "8083")
    snap["services"]["worker"] = _probe_local_port(int(worker_port))
    return snap


def _probe_local_port(port: int) -> str:
    """Return "up" if a local TCP listener accepts a connect within 500ms."""
    import socket
    try:
        with socket.create_connection(("127.0.0.1", port), timeout=0.5):
            return "up"
    except OSError:
        return "down"


def safe_truncate(s: str, max_len: int) -> str:
    """Truncate string to max_len without raising IndexError"""
    if not s:
        return ""
    if len(s) <= max_len:
        return s
    return s[:max_len]


class ThreadedHTTPServer(socketserver.ThreadingMixIn, HTTPServer):
    """Handle each request in a separate thread so long-running jobs
    (imports, downloads) do not block social uploads."""
    daemon_threads = True

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
from kinochilar import KinochilarParser
from uzmedia import UzmediaParser
from asilmedia_serial import AsilmediaSerialParser
from freekino_serial import FreekinoSerialParser
from uzmovi_serial import UzmoviSerialParser
from kinochilar_serial import KinochilarSerialParser
from kinolar_serial import KinolarSerialParser
from uzmedia_serial import UzmediaSerialParser
from downloader_service import DownloaderService, _validate_download_target, report_progress_to_backend
from metadata_normalizer import normalize_metadata, validate_metadata, create_worker_payload
from helpers import sort_video_candidates, normalize_quality_label, quality_height, detect_content_type
from source_config import get_source_config
import recovery
from telemetry import (
    TELEMETRY,
    record_outcome,
    OUTCOME_SUCCESS,
    OUTCOME_FAIL,
    ERR_NO_VIDEO_URL,
    ERR_UNKNOWN,
)

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
AVAILABLE_SOURCES = ["uzmovi", "freekino", "asilmedia", "kinolar", "kinochilar", "uzmedia", "manual"]

# Initialize parsers (manual source doesn't have a parser - it receives direct URLs)
PARSERS = {
    "uzmovi": UzmoviParser(),
    "freekino": FreekinoParser(),
    "asilmedia": AsilmediaParser(),
    "kinolar": KinolarParser(),
    "kinochilar": KinochilarParser(),
    "uzmedia": UzmediaParser(),
}

# Provider-specific serial parsers. Distinct from PARSERS because serial
# scraping follows a different contract (episode list + per-episode video).
SERIAL_PARSERS = {
    "asilmedia": AsilmediaSerialParser(),
    "freekino": FreekinoSerialParser(),
    "uzmovi": UzmoviSerialParser(),
    "kinochilar": KinochilarSerialParser(),
    "kinolar": KinolarSerialParser(),
    "uzmedia": UzmediaSerialParser(),
}


def _detect_serial_provider(url: str) -> str:
    """Return a SERIAL_PARSERS key based on the URL host, or '' if unknown."""
    u = (url or "").lower()
    if "asilmedia." in u:
        return "asilmedia"
    if "freekino." in u:
        return "freekino"
    if "uzmovi." in u:
        return "uzmovi"
    if "kinochilar." in u:
        return "kinochilar"
    if "kinolar." in u:
        return "kinolar"
    if "uzmedia." in u:
        return "uzmedia"
    return ""

# Initialize downloader service
DOWNLOAD_DIR = os.path.abspath(os.environ.get("DOWNLOAD_DIR", str(Path(__file__).parent / "downloads")))
downloader_service = DownloaderService(DOWNLOAD_DIR)

# Active download registry for non-blocking downloads
# Key: job_id, Value: DownloadState object
_active_downloads = {}
_downloads_lock = threading.Lock()
_serial_extract_jobs = {}
_serial_jobs_lock = threading.Lock()
IG_SESSION_DIR = Path(__file__).parent / "ig_sessions"
IG_SESSION_DIR.mkdir(exist_ok=True)
# Sidecar directory: holds per-publish_key result files so backend can recover
# the upload outcome after a proxy 504 / client-side timeout. The instagrapi
# upload itself often completes successfully even when the HTTP response is
# never delivered to the caller; without this sidecar the backend would mark
# the schedule failed even though the post is live on Instagram.
IG_PUBLISH_STATE_DIR = Path(__file__).parent / "ig_publish_state"
IG_PUBLISH_STATE_DIR.mkdir(exist_ok=True)


def _ig_safe_key(raw: str) -> str:
    raw = (raw or "").strip()
    if not raw:
        return ""
    return re.sub(r"[^A-Za-z0-9_\-]", "_", raw)[:128]


def _ig_publish_state_path(publish_key: str) -> Optional[Path]:
    safe = _ig_safe_key(publish_key)
    if not safe:
        return None
    return IG_PUBLISH_STATE_DIR / f"{safe}.json"


def _ig_save_publish_success(publish_key: str, payload: dict) -> None:
    path = _ig_publish_state_path(publish_key)
    if not path:
        return
    try:
        record = dict(payload)
        record["status"] = "success"
        record["saved_at"] = int(time.time())
        tmp_path = path.with_suffix(".tmp")
        tmp_path.write_text(json.dumps(record))
        tmp_path.replace(path)
        logger.info(f"[Instagram] sidecar saved key={publish_key} media_id={record.get('media_id')}")
    except Exception as exc:
        logger.warning(f"[Instagram] sidecar write failed key={publish_key}: {exc}")


def _ig_load_publish_success(publish_key: str) -> Optional[dict]:
    path = _ig_publish_state_path(publish_key)
    if not path or not path.exists():
        return None
    try:
        return json.loads(path.read_text())
    except Exception as exc:
        logger.warning(f"[Instagram] sidecar read failed key={publish_key}: {exc}")
        return None
IG_UPLOAD_BACKOFFS = (5, 15, 45)
IG_PRE_UPLOAD_SLEEP_RANGE = (3, 8)
IG_UPLOAD_COOLDOWN_SECONDS = int(os.environ.get("IG_UPLOAD_COOLDOWN_SECONDS", "60"))
_ig_last_upload_times = {}
_ig_upload_state_lock = threading.Lock()
REMOTE_VIDEO_DOWNLOAD_TIMEOUT = int(os.environ.get("REMOTE_VIDEO_DOWNLOAD_TIMEOUT", "45"))
REMOTE_VIDEO_DOWNLOAD_CHUNK_SIZE = 1024 * 1024


def build_job_output_name(job_id: str, fallback: str = "download") -> str:
    safe_name = re.sub(r'[^\w\s-]', '', (job_id or fallback).strip())
    safe_name = re.sub(r'[-\s]+', '_', safe_name).strip("_") or fallback
    return f"{safe_name}.mp4"


def resolve_downloaded_artifact(job_id: str, raw_path: str = "") -> str:
    def _canonicalize(path: str) -> str:
        path = os.path.abspath(path)
        if not path.endswith(".MUX.mp4"):
            return path
        canonical = path[:-8] + ".mp4"
        try:
            if os.path.exists(path) and (not os.path.exists(canonical) or os.path.getsize(canonical) <= 0):
                logger.info(f"[DOWNLOAD RENAME] from={path} to={canonical}")
                os.replace(path, canonical)
                return canonical
        except OSError:
            pass
        return path

    candidates = []
    if raw_path:
        candidates.append(os.path.abspath(raw_path))
    base = (job_id or "").strip()
    if base:
        candidates.extend([
            os.path.join(DOWNLOAD_DIR, f"{base}.mp4"),
            os.path.join(DOWNLOAD_DIR, f"{base}.MUX.mp4"),
        ])
        candidates.extend(sorted(Path(DOWNLOAD_DIR).glob(f"{base}*")))

    seen = set()
    for candidate in candidates:
        path = str(candidate)
        if path in seen:
            continue
        seen.add(path)
        if os.path.exists(path) and os.path.isfile(path) and os.path.getsize(path) > 0:
            return _canonicalize(path)
    return ""


class InstagramUploadError(Exception):
    """Structured error used by Instagram upload flow."""

    def __init__(self, error_type: str, account: str, message: str, action_required: str):
        super().__init__(message)
        self.error_type = error_type
        self.account = account
        self.message = message
        self.action_required = action_required


def _ig_action_required(error_type: str, account: str) -> str:
    actions = {
        "session_expired": "ig_login.py orqali qayta login qiling",
        "challenge_required": "Instagram appda challenge/checkpoint ni confirm qiling, keyin ig_login.py orqali qayta login qiling",
        "action_blocked": "Instagram vaqtincha blok qo'ygan. Biroz kutib qayta urinib ko'ring",
        "proxy_failed": "Proxy almashtiring yoki o'chirib qayta urinib ko'ring",
        "upload_failed": "Qayta urinib ko'ring",
    }
    return actions.get(error_type, f"{account} account uchun qayta urinib ko'ring")


def _ig_raise(error_type: str, account: str, message: str):
    raise InstagramUploadError(error_type, account, message, _ig_action_required(error_type, account))


def _serial_job_snapshot(job_id: str):
    with _serial_jobs_lock:
        job = _serial_extract_jobs.get(job_id)
        if not job:
            return None
        snapshot = dict(job)
        snapshot["episodes"] = list(job.get("episodes", []))
        if job.get("result") is not None:
            snapshot["result"] = dict(job["result"])
        return snapshot


def _update_serial_job(job_id: str, **fields):
    with _serial_jobs_lock:
        job = _serial_extract_jobs.get(job_id)
        if not job:
            return
        job.update(fields)
        job["updated_at"] = int(time.time())


def _classifier_confidence(content_type: str, evidence: str) -> float:
    kind = (content_type or "").strip().lower()
    reason = (evidence or "").strip().lower()
    if kind == "unknown":
        return 0.0
    if any(token in reason for token in ("episode", "season", "serial", "fasl", "qism", "barcha qismlar")):
        return 0.95
    if any(token in reason for token in ("single playback", "single source", "download button", "movie")):
        return 0.90
    return 0.85


def _run_serial_extract_job(job_id: str, provider: str, serial_url: str):
    parser = SERIAL_PARSERS[provider]

    def emit_progress(payload: dict):
        episodes = payload.get("episodes")
        updates = {
            "status": "processing" if payload.get("stage") != "completed" else "completed",
            "stage": payload.get("stage", "processing"),
            "message": payload.get("message", "Extracting episodes"),
            "expected_total": int(payload.get("expected_total") or 0),
            "discovered_count": int(payload.get("discovered_count") or 0),
            "resolved_count": int(payload.get("resolved_count") or 0),
            "missing_numbers": list(payload.get("missing_numbers") or []),
            "warnings": list(payload.get("warnings") or []),
            "title": payload.get("title", ""),
            "year": int(payload.get("year") or 0),
            "poster": payload.get("poster", ""),
            "backdrop": payload.get("backdrop", ""),
            "description": payload.get("description", ""),
        }
        if episodes is not None:
            updates["episodes"] = list(episodes)
        if payload.get("result") is not None:
            updates["result"] = dict(payload["result"])
        _update_serial_job(job_id, **updates)

    try:
        _update_serial_job(
            job_id,
            status="processing",
            stage="starting",
            message="Starting serial extraction...",
        )
        if provider == "uzmovi":
            result = parser.parse(serial_url, progress_callback=emit_progress)
        else:
            result = parser.parse(serial_url)

        status = "completed" if result.get("success") else "failed"
        _update_serial_job(
            job_id,
            status=status,
            stage="completed" if status == "completed" else "failed",
            message=f"Extracted {len(result.get('episodes', []))} episodes" if status == "completed" else result.get("error", "serial parse failed"),
            expected_total=max(
                int(_serial_job_snapshot(job_id).get("expected_total", 0) or 0) if _serial_job_snapshot(job_id) else 0,
                len(result.get("episodes", [])),
            ),
            discovered_count=len(result.get("episodes", [])),
            resolved_count=len(result.get("episodes", [])),
            episodes=list(result.get("episodes", [])),
            warnings=list(result.get("warnings", [])),
            missing_numbers=list(result.get("missing_numbers", [])),
            result=result,
            error=result.get("error", ""),
        )
    except Exception as exc:
        logger.exception(f"[SERIAL] async parse failed provider={provider} job_id={job_id}")
        _update_serial_job(
            job_id,
            status="failed",
            stage="failed",
            message=f"{provider} serial parse failed",
            error=str(exc),
        )


def _ig_normalize_account_name(name: str) -> str:
    return re.sub(r"[^A-Za-z0-9]+", "_", (name or "").strip()).strip("_").lower()


def _ig_env_prefix(account: str) -> str:
    return _ig_normalize_account_name(account).upper()


def _ig_mask_proxy(proxy: str) -> str:
    if not proxy:
        return ""
    parsed = urlparse(proxy)
    if not parsed.scheme or not parsed.netloc:
        return "***"
    if "@" not in parsed.netloc:
        return f"{parsed.scheme}://{parsed.netloc}"
    creds, host = parsed.netloc.rsplit("@", 1)
    if ":" in creds:
        user, _ = creds.split(":", 1)
        creds = f"{user}:***"
    else:
        creds = "***"
    return f"{parsed.scheme}://{creds}@{host}"


def _ig_configured_accounts():
    raw = os.environ.get("IG_ACCOUNTS", "main,backup1")
    accounts = []
    for item in raw.split(","):
        normalized = _ig_normalize_account_name(item)
        if normalized and normalized not in accounts:
            accounts.append(normalized)
    return accounts or ["main", "backup1"]


def _ig_accounts_to_try(requested_account: str):
    configured = _ig_configured_accounts()
    requested = _ig_normalize_account_name(requested_account)
    if not requested:
        return configured
    if requested == "main":
        ordered = [requested] + [acc for acc in configured if acc != requested]
        return ordered
    return [requested]


def _ig_classify_error(err) -> str:
    text = str(err).lower()
    if any(token in text for token in ("proxy", "tunnel connection failed", "cannot connect", "connection refused", "connect timeout")):
        return "proxy_failed"
    if any(token in text for token in ("challenge_required", "checkpoint_required", "account recovery", "confirm", "verify", "two_factor")):
        return "challenge_required"
    if any(token in text for token in ("feedback_required", "please wait", "too many", "spam", "rate limit", "action blocked")):
        return "action_blocked"
    if any(token in text for token in ("login_required", "not_authenticated", "not authenticated", "session_expired", "forbidden", "403")):
        return "session_expired"
    return "upload_failed"


def _ig_maybe_migrate_legacy_session(account: str, username: str, session_file: Path):
    if session_file.exists() or not username:
        return
    legacy_path = IG_SESSION_DIR / f"{hashlib.md5(username.encode()).hexdigest()}.json"
    if legacy_path.exists():
        shutil.move(str(legacy_path), str(session_file))
        logger.info(f"[Instagram] migrated legacy session account={account} path={session_file}")


def _ig_get_account_config(account: str, body_username: str = "", body_password: str = ""):
    normalized = _ig_normalize_account_name(account)
    env_prefix = _ig_env_prefix(normalized)
    username = (os.environ.get(f"IG_{env_prefix}_USERNAME", "") or body_username or "").strip()
    password = (os.environ.get(f"IG_{env_prefix}_PASSWORD", "") or body_password or "").strip()
    proxy = os.environ.get(f"IG_{env_prefix}_PROXY", "").strip()
    session_file = IG_SESSION_DIR / f"{normalized}.json"
    _ig_maybe_migrate_legacy_session(normalized, username, session_file)
    return {
        "account": normalized,
        "username": username,
        "password": password,
        "proxy": proxy,
        "session_file": session_file,
    }


def _ig_create_client(account_config):
    from instagrapi import Client

    cl = Client()
    cl.delay_range = [1, 3]
    proxy = account_config.get("proxy", "")
    masked_proxy = _ig_mask_proxy(proxy)
    if proxy:
        try:
            cl.set_proxy(proxy)
        except Exception as exc:
            _ig_raise(
                "proxy_failed",
                account_config["account"],
                f"Proxy ishlamadi ({masked_proxy}): {exc}",
            )
    logger.info(
        f"[Instagram] account={account_config['account']} proxy={'yes' if proxy else 'no'} "
        f"session={'loaded' if account_config['session_file'].exists() else 'missing'}"
    )
    return cl


def _ig_archive_session(session_file: Path, reason: str):
    if not session_file.exists():
        return None
    archived = session_file.with_name(f"{session_file.stem}.{reason}.{int(time.time())}.json")
    session_file.replace(archived)
    return archived


def _ig_login_and_save(account_config):
    username = account_config.get("username", "")
    password = account_config.get("password", "")
    session_file = account_config["session_file"]
    if not username or not password:
        _ig_raise(
            "session_expired",
            account_config["account"],
            f"Instagram credentials topilmadi for account='{account_config['account']}'. "
            f"IG_{_ig_env_prefix(account_config['account'])}_USERNAME va PASSWORD ni sozlang.",
        )

    cl = _ig_create_client(account_config)
    try:
        cl.login(username, password)
        cl.dump_settings(session_file)
        logger.info(f"[Instagram] session saved account={account_config['account']} path={session_file}")
        return cl
    except Exception as exc:
        error_type = _ig_classify_error(exc)
        _ig_raise(
            error_type,
            account_config["account"],
            f"Instagram login failed for account='{account_config['account']}' username='{username}': {exc}",
        )


def _ig_validate_or_relogin(account_config):
    session_file = account_config["session_file"]
    cl = _ig_create_client(account_config)
    if session_file.exists():
        try:
            cl.load_settings(session_file)
            cl.get_timeline_feed()
            logger.info(f"[Instagram] session valid account={account_config['account']} path={session_file}")
            return cl
        except Exception as exc:
            error_type = _ig_classify_error(exc)
            logger.warning(
                f"[Instagram] session invalid account={account_config['account']} "
                f"path={session_file} type={error_type}: {exc}"
            )
            if error_type in {"session_expired", "challenge_required"}:
                archived = _ig_archive_session(session_file, "expired")
                if archived:
                    logger.info(
                        f"[Instagram] archived stale session account={account_config['account']} "
                        f"old_path={archived}"
                    )
                return _ig_login_and_save(account_config)
            _ig_raise(
                error_type,
                account_config["account"],
                f"Instagram session tekshiruvi muvaffaqiyatsiz tugadi for account='{account_config['account']}': {exc}",
            )
    logger.info(f"[Instagram] session missing account={account_config['account']} path={session_file}")
    return _ig_login_and_save(account_config)


def _ig_apply_pre_upload_delay(account: str):
    delay = random.uniform(*IG_PRE_UPLOAD_SLEEP_RANGE)
    logger.info(f"[Instagram] account={account} pre-upload sleep {delay:.1f}s")
    time.sleep(delay)


def _ig_wait_for_cooldown(account: str):
    with _ig_upload_state_lock:
        last_upload_at = _ig_last_upload_times.get(account, 0.0)
    wait_time = max(0.0, IG_UPLOAD_COOLDOWN_SECONDS - (time.time() - last_upload_at))
    if wait_time > 0:
        logger.info(f"[Instagram] account={account} cooldown wait {wait_time:.1f}s")
        time.sleep(wait_time)


def _ig_mark_upload(account: str):
    with _ig_upload_state_lock:
        _ig_last_upload_times[account] = time.time()


def _ig_upload_once(cl, account_config, video_path: Path, caption: str):
    media = cl.clip_upload(video_path, caption)
    _ig_mark_upload(account_config["account"])
    return media


def _ig_upload_for_account(account_config, video_path: Path, caption: str, publish_key: str = ""):
    account = account_config["account"]
    cl = _ig_validate_or_relogin(account_config)
    last_error = None
    relogin_retry_used = False

    for attempt, backoff in enumerate(IG_UPLOAD_BACKOFFS, start=1):
        logger.info(f"[Instagram] account={account} upload attempt {attempt}/{len(IG_UPLOAD_BACKOFFS)}")
        _ig_wait_for_cooldown(account)
        _ig_apply_pre_upload_delay(account)
        try:
            media = _ig_upload_once(cl, account_config, video_path, caption)
            logger.info(f"[Instagram] final success account={account} media_id={media.pk}")
            media_code = getattr(media, "code", "") or ""
            post_url = f"https://www.instagram.com/reel/{media_code}/" if media_code else ""
            result = {
                "status": "success",
                "account": account,
                "media_id": str(media.pk),
                "media_code": media_code,
                "post_url": post_url,
            }
            # Persist the success BEFORE returning so backend can recover via
            # /instagram/upload/status if the response never reaches it.
            if publish_key:
                _ig_save_publish_success(publish_key, result)
            return result
        except Exception as exc:
            error_type = _ig_classify_error(exc)
            last_error = InstagramUploadError(
                error_type,
                account,
                f"Instagram upload failed for account='{account}': {exc}",
                _ig_action_required(error_type, account),
            )
            logger.warning(
                f"[Instagram] account={account} attempt={attempt} failed type={error_type}: {exc}"
            )

            if error_type in {"session_expired", "challenge_required"} and not relogin_retry_used:
                archived = _ig_archive_session(account_config["session_file"], "expired")
                if archived:
                    logger.info(
                        f"[Instagram] archived session before relogin account={account} path={archived}"
                    )
                cl = _ig_login_and_save(account_config)
                relogin_retry_used = True
                continue

            if attempt < len(IG_UPLOAD_BACKOFFS):
                logger.info(f"[Instagram] account={account} backoff {backoff}s before retry")
                time.sleep(backoff)

    if last_error:
        raise last_error
    _ig_raise("upload_failed", account, f"Instagram upload failed for account='{account}'")


def _download_remote_video_to_path(video_url: str, output_path: str):
    headers = {
        "User-Agent": "Mozilla/5.0",
        "Accept": "*/*",
        "Connection": "keep-alive",
    }
    logger.info(f"[VIDEO DOWNLOAD] starting url={video_url}")

    try:
        with requests.get(video_url, headers=headers, stream=True, timeout=REMOTE_VIDEO_DOWNLOAD_TIMEOUT) as response:
            logger.info(f"[VIDEO DOWNLOAD] response status={response.status_code} url={video_url}")
            if response.status_code != 200:
                body_preview = ""
                try:
                    body_preview = response.text[:500]
                except Exception:
                    body_preview = "<unavailable>"
                logger.error(
                    f"[VIDEO DOWNLOAD] failed status={response.status_code} url={video_url} body={body_preview}"
                )
                raise Exception(
                    f"Video download failed: HTTP {response.status_code} for {video_url}"
                )

            with open(output_path, "wb") as file_handle:
                for chunk in response.iter_content(chunk_size=REMOTE_VIDEO_DOWNLOAD_CHUNK_SIZE):
                    if chunk:
                        file_handle.write(chunk)
    except requests.RequestException as exc:
        raise Exception(f"Video download request failed for {video_url}: {exc}") from exc

def _guess_media_type(url: str) -> str:
    lower = (url or "").lower()
    if ".m3u8" in lower:
        return "m3u8"
    if ".mpd" in lower:
        return "mpd"
    if ".ism" in lower:
        return "ism"
    if ".mp4" in lower:
        return "mp4"
    return "unknown"


def _tail_text(text: str, limit: int = 4000) -> str:
    if not text:
        return ""
    return text[-limit:]


class DownloadState:
    """Represents the state of an active download"""
    def __init__(self, job_id, source_url, output_name, source="", detail_url="", selected_quality=""):
        self.job_id = job_id
        self.source = source
        self.source_url = source_url
        self.video_url = source_url
        self.download_url = source_url
        self.output_name = output_name
        self.status = "starting"  # starting, downloading, completed, failed
        self.started_at = time.time()
        self.last_progress_at = self.started_at
        self.done = False
        self.progress_percent = 0
        self.downloaded_bytes = 0
        self.total_bytes = 0
        self.speed_mbps = 0.0
        self.eta_seconds = 0
        self.local_path = ""
        self.file_size = 0
        self.error = ""
        self.pid = 0
        self.detail_url = detail_url
        self.referer = detail_url
        self.selected_quality = selected_quality
        self.media_type = _guess_media_type(source_url)
        self.downloader_command = []
        self.downloader_command_string = ""
        self.stdout_tail = ""
        self.stderr_tail = ""
        self.exit_code = None
        self.temp_output_path = ""

    def append_stdout(self, line: str):
        if line:
            self.stdout_tail = _tail_text((self.stdout_tail + "\n" + line).strip())

    def append_stderr(self, line: str):
        if line:
            self.stderr_tail = _tail_text((self.stderr_tail + "\n" + line).strip())

    def apply_debug_event(self, event: str, payload: dict):
        if not isinstance(payload, dict):
            return
        if event == "command":
            cmd = payload.get("command") or []
            self.downloader_command = cmd
            self.downloader_command_string = payload.get("command_string") or " ".join(str(x) for x in cmd)
            self.temp_output_path = payload.get("output_path", self.temp_output_path)
            self.media_type = payload.get("media_type", self.media_type)
        elif event == "pid":
            self.pid = int(payload.get("pid") or 0)
        elif event == "stdout":
            self.append_stdout(str(payload.get("line") or ""))
        elif event == "stderr":
            self.append_stderr(str(payload.get("line") or ""))
        elif event == "exit":
            self.exit_code = payload.get("exit_code")
            if payload.get("stderr"):
                self.append_stderr(str(payload.get("stderr")))
            if payload.get("stdout"):
                self.append_stdout(str(payload.get("stdout")))
        elif event == "file_size":
            file_size = int(payload.get("file_size") or 0)
            if file_size > self.file_size:
                self.last_progress_at = time.time()
                self.downloaded_bytes = max(self.downloaded_bytes, file_size)
                if self.total_bytes <= 0:
                    self.progress_percent = max(self.progress_percent, 1)
            self.file_size = file_size
            self.temp_output_path = payload.get("output_path", self.temp_output_path)
        elif event == "media_type":
            self.media_type = payload.get("media_type", self.media_type)

    def to_dict(self):
        return {
            "job_id": self.job_id,
            "source": self.source,
            "status": self.status,
            "started_at": self.started_at,
            "last_progress_at": self.last_progress_at,
            "done": self.done,
            "progress_percent": self.progress_percent,
            "downloaded_bytes": self.downloaded_bytes,
            "total_bytes": self.total_bytes,
            "speed_mbps": self.speed_mbps,
            "eta_seconds": self.eta_seconds,
            "local_path": self.local_path,
            "file_size": self.file_size,
            "error": self.error,
            "source_url": self.source_url,
            "video_url": self.video_url,
            "download_url": self.download_url,
            "output_name": self.output_name,
            "pid": self.pid,
            "detail_url": self.detail_url,
            "referer": self.referer,
            "selected_quality": self.selected_quality,
            "media_type": self.media_type,
            "downloader_command": self.downloader_command,
            "downloader_command_string": self.downloader_command_string,
            "stdout_tail": self.stdout_tail,
            "stderr_tail": self.stderr_tail,
            "exit_code": self.exit_code,
            "temp_output_path": self.temp_output_path,
        }
def get_active_download(job_id):
    """Get active download state for job_id"""
    with _downloads_lock:
        return _active_downloads.get(job_id)

def set_active_download(job_id, state):
    """Set active download state for job_id"""
    with _downloads_lock:
        _active_downloads[job_id] = state

def clear_active_download(job_id):
    """Clear active download state for job_id.

    Also kills the running downloader process (ffmpeg / aria2c / n_m3u8dl) for
    that job. Without this, a /download?force=1 retry from the worker (e.g.
    after trying a different quality URL) leaves the previous process running
    in the background. Both processes then write to the same output file and
    keep reporting progress for the same backend job_id, which shows up as
    bytes oscillating (400MB ↔ 800MB) in the admin UI.
    """
    with _downloads_lock:
        state = _active_downloads.pop(job_id, None)

    if state is None:
        return

    pid = getattr(state, "pid", 0) or 0
    if pid > 0:
        try:
            import signal as _sig
            os.kill(pid, _sig.SIGKILL)
            logger.warning(f"[CLEAR] killed leaked downloader pid={pid} for job_id={job_id}")
        except ProcessLookupError:
            pass
        except Exception as exc:
            logger.warning(f"[CLEAR] kill pid={pid} for job_id={job_id} failed: {exc}")

def is_download_active(job_id):
    """Check if download is active for job_id"""
    with _downloads_lock:
        return job_id in _active_downloads

def get_all_downloads():
    """Get all active downloads (for cleanup, monitoring)"""
    with _downloads_lock:
        return dict(_active_downloads)

# Worker URL (default to localhost:8083)
WORKER_URL = os.environ.get("WORKER_URL", "http://localhost:8083")

# Backend URL - for reporting progress
# Must be explicitly set for progress reporting to work
BACKEND_URL = os.environ.get("BACKEND_URL", "")
DOWNLOAD_CONCURRENCY = max(1, int(os.environ.get("DOWNLOAD_CONCURRENCY", "6")))
DOWNLOAD_QUEUE_POLL_SECONDS = max(1, int(os.environ.get("DOWNLOAD_QUEUE_POLL_SECONDS", "5")))
_download_queue_started = False
_download_queue_lock = threading.Lock()


def _report_backend_failure(job_id: str, message: str):
    if not job_id or not BACKEND_URL:
        return
    # Mark job failed cleanly: stage=failed, status=failed, progress=0,
    # error/message carry the parser reason. Backend's UpdateProgress
    # persists status/stage; error is also set when status=failed.
    report_progress_to_backend(job_id, {
        "stage": "failed",
        "status": "failed",
        "progress": 0,
        "progress_percent": 0,
        "message": message,
        "error": message,
    })


def _unwrap_embed_url(url: str) -> str:
    """Unwrap player-embed wrappers like
    /player/playerjs.html?file=https://...master.m3u8 and
    /embed.html?file=http://...mp4 into the bare video URL the
    file= parameter points to.

    Some sources (kinochilar, uzmedia) return the embed iframe URL from
    /details instead of the real video. The downloader's URL validator
    then rejects it as "invalid URL scheme/netloc" because the embed
    path is relative. We unwrap once here so downstream code can treat
    every source uniformly.
    """
    if not url:
        return url
    try:
        from urllib.parse import urlparse, parse_qs, unquote
        parsed = urlparse(url)
        # Recognise common embed players whose real video URL sits in
        # the `file` query parameter.
        path = (parsed.path or "").lower()
        is_embed = (
            "playerjs.html" in path
            or path.endswith("/embed.html")
            or path == "/embed.html"
        )
        if not is_embed:
            return url
        qs = parse_qs(parsed.query)
        inner = (qs.get("file") or [""])[0]
        inner = unquote(inner).strip()
        if not inner:
            return url
        # kinolar packs multiple-quality URLs into a single file= value,
        # comma-separated (file=URL1,URL2,URL3&qualities=480P,720P,1080P).
        # Pick the LAST entry — typically the highest quality.
        if "," in inner:
            for candidate in reversed(inner.split(",")):
                candidate = candidate.strip()
                if candidate.startswith("http://") or candidate.startswith("https://"):
                    return candidate
            return url
        if inner.startswith("http://") or inner.startswith("https://"):
            return inner
    except Exception:
        pass
    return url


def _resolve_claimed_job_video(job: dict, parser_base_url: str) -> tuple[str, str]:
    stored_video_url = (job.get("video_url") or "").strip()
    referer = ""
    metadata = job.get("metadata") or {}
    if isinstance(metadata, dict):
        referer = (metadata.get("video_page_url") or "").strip()

    source = (job.get("source") or "").lower()
    detail_url = (job.get("detail_url") or "").strip()

    # Sources that hand out short-lived signed CDN URLs (e.g. freekino's
    # a*.video-cdn.org with ?md5=&expires=, asilmedia's fayllar*.ru,
    # uzmovi's srv*.uzdown.space which expires seconds after the Playwright
    # session ends). The stored video_url is captured at /details time and
    # frequently expires before the worker claims the job — re-resolve so
    # we hand a fresh URL to the downloader. Other sources can keep using
    # the stored value.
    sources_with_signed_urls = {"freekino", "asilmedia", "uzmovi"}
    needs_refresh = source in sources_with_signed_urls and detail_url

    if stored_video_url and not needs_refresh:
        if not referer:
            referer = detail_url
        return _unwrap_embed_url(stored_video_url), referer

    params = {
        "source": job.get("source", ""),
        "id": job.get("source_id", ""),
        "url": job.get("detail_url", ""),
        "job_id": job.get("job_id", ""),
    }
    if job.get("source") == "manual" and isinstance(metadata, dict):
        if metadata.get("title"):
            params["title"] = metadata["title"]
        if metadata.get("year"):
            params["year"] = str(metadata["year"])
        if metadata.get("poster"):
            params["poster_url"] = metadata["poster"]
        if metadata.get("backdrop"):
            params["backdrop_url"] = metadata["backdrop"]

    response = requests.get(f"{parser_base_url}/details", params=params, timeout=180)

    # Try to decode JSON regardless of status, so we can surface parser reasons
    # for 4xx responses (e.g. freekino 403 → /details 422 video_url_not_found).
    try:
        payload = response.json()
    except Exception:
        payload = {}

    if response.status_code in (422, 400, 403, 500, 502, 503):
        reason = (
            payload.get("reason")
            or payload.get("manual_reason")
            or payload.get("error")
            or f"parser /details http {response.status_code}"
        )
        src = payload.get("source") or job.get("source", "")
        sid = payload.get("source_id") or job.get("source_id", "")
        durl = payload.get("detail_url") or job.get("detail_url", "")
        raise RuntimeError(
            f"parser /details failed http={response.status_code} "
            f"source={src} source_id={sid} detail_url={durl} reason={reason}"
        )

    response.raise_for_status()

    if payload.get("error"):
        src = payload.get("source") or job.get("source", "")
        sid = payload.get("source_id") or job.get("source_id", "")
        durl = payload.get("detail_url") or job.get("detail_url", "")
        reason = payload.get("reason") or payload.get("manual_reason") or payload["error"]
        raise RuntimeError(
            f"parser /details error source={src} source_id={sid} detail_url={durl} reason={reason}"
        )

    video_url = (payload.get("video_url") or "").strip()
    if not video_url:
        src = job.get("source", "")
        sid = job.get("source_id", "")
        durl = job.get("detail_url", "")
        raise RuntimeError(
            f"video_url_not_found source={src} source_id={sid} detail_url={durl}"
        )

    referer = (payload.get("video_page_url") or referer or job.get("detail_url") or "").strip()
    return _unwrap_embed_url(video_url), referer


def _run_claimed_download(job: dict, parser_base_url: str):
    job_id = (job.get("job_id") or "").strip()
    if not job_id:
        raise RuntimeError("claimed job missing job_id")

    video_url, referer = _resolve_claimed_job_video(job, parser_base_url)
    source = (job.get("source") or "").strip()
    selected_quality = (job.get("source_quality") or "").strip()
    metadata = job.get("metadata") or {}
    if not selected_quality and isinstance(metadata, dict):
        selected_quality = (metadata.get("quality") or "").strip()
    # Note: previously this raised when asilmedia jobs lacked source_quality,
    # but serial-episode jobs are queued without a preselected quality and
    # were being failed at claim time. Let the downloader auto-pick when
    # quality is unset — movies typically have it set by the catalog import.
    if source == "asilmedia" and not selected_quality:
        logger.info(f"[QUEUE] asilmedia job {job_id} has no source_quality; downloader will auto-select")
    ok, validation_error = _validate_download_target(video_url, referer=referer or None)
    if not ok:
        raise RuntimeError(f"selected_url={video_url} validation_failed={validation_error}")
    output_name = build_job_output_name(job_id, "queue_download")

    logger.info(f"[download] job={job_id} source={source} selected_quality={selected_quality or 'auto'}")
    logger.info(f"[download] url={video_url}")
    logger.info(f"[QUEUE] download start job_id={job_id} source={source} output={output_name}")

    # De-dup: if another queue slot (or a stale claim) already has this job
    # in flight, do NOT start a parallel downloader. Multiple ffmpeg processes
    # writing to the same output file corrupt the file and produce thrashing
    # progress (2% → 21% → 9% → 2% as each reports independently).
    existing = get_active_download(job_id)
    if existing and existing.status in ("starting", "downloading"):
        logger.warning(
            f"[QUEUE] duplicate claim — job_id={job_id} already {existing.status} "
            f"(pid={existing.pid}); skipping to avoid concurrent ffmpeg on same output"
        )
        return

    # Register DownloadState so /progress?job_id= can report status to the
    # worker watchdog. Without this, /progress returns 404 and the watchdog
    # fails the job after 90s as "no active downloader PID".
    state = DownloadState(
        job_id,
        video_url,
        output_name,
        source=source,
        detail_url=referer,
        selected_quality=selected_quality,
    )
    set_active_download(job_id, state)

    def progress_callback(percent, downloaded, total, speed, eta):
        st = get_active_download(job_id)
        if st and st.status != "failed":
            st.progress_percent = percent
            st.downloaded_bytes = downloaded
            st.total_bytes = total
            st.speed_mbps = speed / (1024 * 1024) if speed > 0 else 0.0
            st.eta_seconds = eta
            st.file_size = downloaded
            st.last_progress_at = time.time()
            st.status = "downloading"

    def pid_callback(pid):
        st = get_active_download(job_id)
        if st:
            st.pid = pid

    def debug_callback(event, payload):
        st = get_active_download(job_id)
        if st:
            st.apply_debug_event(event, payload or {})

    try:
        result = downloader_service.smart_download(
            url=video_url,
            output_name=output_name,
            job_id=job_id,
            backend_job_id=job_id,
            referer=referer or None,
            progress_callback=progress_callback,
            pid_callback=pid_callback,
            debug_callback=debug_callback,
        )
    except Exception:
        st = get_active_download(job_id)
        if st:
            st.status = "failed"
            st.done = True
        raise

    if not result.get("success"):
        st = get_active_download(job_id)
        if st:
            st.status = "failed"
            st.error = result.get("error") or "download failed"
            st.done = True
            st.exit_code = st.exit_code if st.exit_code is not None else -1
        raise RuntimeError(result.get("error") or "download failed")

    local_path = resolve_downloaded_artifact(job_id, result.get("file_path", ""))
    st = get_active_download(job_id)
    if st:
        st.status = "completed"
        st.done = True
        st.progress_percent = 100
        st.local_path = local_path or result.get("file_path", "")
        if st.local_path and os.path.exists(st.local_path):
            try:
                size = os.path.getsize(st.local_path)
                st.file_size = size
                st.downloaded_bytes = size
                st.total_bytes = size
            except OSError:
                pass
        st.last_progress_at = time.time()
        st.media_type = result.get("type", st.media_type)


def _download_queue_worker(slot: int, parser_base_url: str):
    claim_url = f"{BACKEND_URL}/api/ingestion/jobs/parser/claim"
    while True:
        if not BACKEND_URL:
            time.sleep(DOWNLOAD_QUEUE_POLL_SECONDS)
            continue

        try:
            response = requests.get(claim_url, timeout=30)
            if response.status_code == 404:
                logger.info("[QUEUE] no queued jobs")
                time.sleep(DOWNLOAD_QUEUE_POLL_SECONDS)
                continue
            response.raise_for_status()
            job = response.json()
            logger.info(f"[QUEUE] claimed job id={job.get('job_id')} queue=download slot={slot} source={job.get('source')}")
            try:
                _run_claimed_download(job, parser_base_url)
            except Exception as exc:
                logger.error(
                    f"[QUEUE] parser failed -> job failed slot={slot} "
                    f"job_id={job.get('job_id')} source={job.get('source')} "
                    f"source_id={job.get('source_id')} detail_url={job.get('detail_url')} "
                    f"error={exc}",
                    exc_info=True,
                )
                _report_backend_failure(job.get("job_id", ""), str(exc))
        except Exception as exc:
            logger.warning(f"[QUEUE] slot={slot} claim error: {exc}")
            time.sleep(DOWNLOAD_QUEUE_POLL_SECONDS)


def start_download_queue(parser_base_url: str):
    global _download_queue_started

    with _download_queue_lock:
        if _download_queue_started:
            return
        _download_queue_started = True

    logger.info(f"[QUEUE] download worker started concurrency={DOWNLOAD_CONCURRENCY}")
    for slot in range(DOWNLOAD_CONCURRENCY):
        thread = threading.Thread(
            target=_download_queue_worker,
            args=(slot + 1, parser_base_url),
            daemon=True,
        )
        thread.start()


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
        """Send JSON response with standard envelope and UTF-8 encoding"""
        try:
            # Standardize response format
            response_body = {}
            
            if 200 <= status < 300:
                if isinstance(data, dict):
                    # Ensure success field
                    success = data.get("success", True)
                    # If it's already an envelope, use it
                    if "data" in data and "success" in data:
                        response_body = data
                    else:
                        # Build standard envelope
                        response_body["success"] = success
                        response_body["data"] = data
                        
                        # Keep flat fields at root for backward compatibility with worker
                        for k, v in data.items():
                            if k != "data":
                                response_body[k] = v
                else:
                    # Non-dict data wrapped in envelope
                    response_body = {"success": True, "data": data}
            else:
                # Error responses
                if isinstance(data, dict):
                    response_body["success"] = data.get("success", False)
                    response_body["error"] = data.get("error", data.get("message", "Unknown error"))
                    # Keep other fields for debugging
                    for k, v in data.items():
                        if k not in ("success", "error"):
                            response_body[k] = v
                else:
                    response_body = {"success": False, "error": str(data)}

            self.send_response(status)
            self.send_header("Content-Type", "application/json; charset=utf-8")
            self.end_headers()
            
            # Use ensure_ascii=False for proper UTF-8 output
            json_str = json.dumps(response_body, ensure_ascii=False)
            self.wfile.write(json_str.encode('utf-8'))
        except (BrokenPipeError, ConnectionResetError):
            pass  # client disconnected before response was sent

    def _send_error(self, message, status=400):
        """Send error response using standard format"""
        error_payload = {
            "success": False,
            "error": message
        }
        self._send_json(error_payload, status)
    
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

        Parser methods come in two shapes:
          - simple:   get_details(url)
          - extended: get_details(url, source_id, is_serial=False, episode_id="")
        We introspect the signature so the helper passes whichever extras
        the target parser declares. Previously the helper always called
        get_details(url) and three parsers (kinolar, kinochilar, uzmedia)
        crashed with "missing 1 required positional argument: 'source_id'".
        """
        import inspect

        url = detail_url if detail_url else source_id

        method = None
        if hasattr(parser, "get_details"):
            method = getattr(parser, "get_details")
        elif hasattr(parser, "get_detail"):
            method = getattr(parser, "get_detail")
        if method is None:
            raise ValueError(f"Parser '{source}' does not have a valid get_details method")

        try:
            sig = inspect.signature(method)
            params = sig.parameters
        except (TypeError, ValueError):
            # Builtin / C-implemented method without an introspectable
            # signature — fall back to the simple call.
            return method(url)

        kwargs = {}
        if "source_id" in params and source_id is not None:
            kwargs["source_id"] = source_id
        # is_serial / episode_id are optional everywhere — only pass
        # them if the parser explicitly declares them so we don't
        # accidentally break a future overload.
        return method(url, **kwargs)
    
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
            
            # Classify the URL type. Parser source hints like "script_extracted"
            # are not media types and must not outrank real m3u8/mp4 detection.
            parsed_type = v.get("type", "")
            classified_type = classify_media_url(url)
            url_type = parsed_type if parsed_type in ["mp4", "m3u8", "mpd", "ism"] else classified_type
            
            # FIXED: Accept any URL that passes validation, even if type is unknown
            # This is more lenient for uzmovi URLs which may not have standard extensions
            confidence = 0.9 if url_type in ["mp4", "m3u8", "mpd", "hls"] else 0.7
            
            # Create candidate
            quality = normalize_quality_label(v.get("quality", "auto"))
            candidate = MediaCandidate(
                url=url,
                type=url_type,
                quality=quality,
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
        
        candidate_dicts = sort_video_candidates([
            {
                "url": c.url,
                "type": c.type,
                "quality": c.quality,
                "confidence": c.confidence,
                "height": quality_height(c.quality, c.url),
            }
            for c in candidates
        ])
        logger.info("[quality] candidates=%s", json.dumps([
            {
                "quality": item.get("quality", "unknown"),
                "height": item.get("height", 0),
                "type": item.get("type", "unknown"),
                "url": safe_truncate(item.get("url", ""), 160),
            }
            for item in candidate_dicts
        ]))

        # Use the enhanced selection function after quality ordering so equal
        # quality/type candidates still benefit from its confidence logic.
        best_candidate = choose_best_media_candidate([
            MediaCandidate(
                url=item["url"],
                type=item.get("type", "unknown"),
                quality=item.get("quality", "unknown"),
                source_hint=item.get("type", "unknown"),
                confidence=float(item.get("confidence", 0.7)),
            )
            for item in candidate_dicts
        ])
        
        if best_candidate:
            logger.info(f"[SERVER] ═══════════════════════════════════════════")
            logger.info(f"[SERVER] SELECTED media URL:")
            logger.info(f"[SERVER]   type: {best_candidate.type}")
            logger.info(f"[SERVER]   url: {best_candidate.url}")
            logger.info(f"[SERVER]   confidence: {best_candidate.confidence}")
            logger.info(f"[quality] selected={best_candidate.quality} url={best_candidate.url}")
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

    def _verify_video_url(self, video_url: str, referer: str = ""):
        """Verify a media URL with HEAD first, then a tiny GET fallback."""
        headers = {
            "User-Agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36",
            "Accept": "*/*",
        }
        if referer:
            headers["Referer"] = referer

        for method in ("HEAD", "GET"):
            try:
                req_headers = dict(headers)
                if method == "GET":
                    req_headers["Range"] = "bytes=0-4095"

                req = urllib.request.Request(video_url, headers=req_headers, method=method)
                with urllib.request.urlopen(req, timeout=20) as response:
                    status = getattr(response, "status", 0)
                    content_type = response.headers.get("Content-Type", "")
                    if status and status < 400:
                        logger.info(f"[PARSER] verified url - method={method}, status={status}, content_type={content_type}")
                        return True, ""
                    logger.warning(f"[PARSER] verify failed - method={method}, status={status}, content_type={content_type}")
            except urllib.error.HTTPError as e:
                if method == "HEAD" and e.code in (403, 405):
                    logger.info(f"[PARSER] HEAD verify not allowed ({e.code}), trying short GET")
                    continue
                return False, f"HTTP {e.code}: {e.reason}"
            except Exception as e:
                if method == "HEAD":
                    logger.info(f"[PARSER] HEAD verify failed, trying short GET: {e}")
                    continue
                return False, str(e)

        return False, "URL verification failed"
    
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
                except Exception:
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
            self._send_json({"ok": True, "status": "ok", "service": "parser"})
            return

        # Host-level health snapshot consumed by the backend admin
        # dashboard. Gated by X-Internal-Token (same token the backend
        # uses for bot ↔ backend internal calls) so machine metrics don't
        # leak to the public.
        elif path == "/healthz":
            expected = os.environ.get("BOT_INTERNAL_TOKEN", "")
            provided = self.headers.get("X-Internal-Token", "")
            if not expected or provided != expected:
                self._send_error("unauthorized", 401)
                return
            self._send_json(_host_health_snapshot())
            return

        # Parser-wide health metrics: per-source success/fail counts + last N jobs.
        # Used by ops to see which sources are flaky without grepping logs.
        elif path == "/parser/health":
            recent_n = 20
            try:
                recent_n = int(query.get("recent", ["20"])[0])
                recent_n = max(1, min(recent_n, 200))
            except (TypeError, ValueError):
                recent_n = 20
            self._send_json(TELEMETRY.health_snapshot(recent_n=recent_n))
            return

        # Instagram publish status sidecar lookup.
        # GET /instagram/upload/status?publish_key=<key>
        # Returns {"status":"success", media_id, media_code, post_url, ...} if
        # an upload tagged with that key has previously succeeded; otherwise
        # {"status":"unknown"}. Used by the backend to recover from proxy 504s
        # where instagrapi succeeded but the HTTP response never made it back.
        elif path == "/instagram/upload/status":
            publish_key = query.get("publish_key", [""])[0].strip() or query.get("key", [""])[0].strip()
            if not publish_key:
                self._send_error("publish_key is required", 400)
                return
            existing = _ig_load_publish_success(publish_key)
            if existing and existing.get("status") == "success":
                self._send_json(existing)
            else:
                self._send_json({"status": "unknown", "publish_key": publish_key})
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
        
        # Serial import: /serial-details?url=<serial page url>  [&source=<provider>]
        # Returns a normalized serial payload with an episodes[] list.
        elif path == "/serial-details":
            serial_url = query.get("url", [""])[0].strip()
            source_hint = query.get("source", [""])[0].strip().lower()

            if not serial_url:
                self._send_error("Missing 'url' parameter")
                return

            provider = source_hint or _detect_serial_provider(serial_url)
            logger.info(f"[SERIAL] provider detected={provider!r} for url={serial_url}")

            if provider not in SERIAL_PARSERS:
                self._send_error(
                    f"Unsupported serial provider. Supported: {sorted(SERIAL_PARSERS.keys())}",
                    400,
                )
                return

            try:
                result = SERIAL_PARSERS[provider].parse(serial_url)
                # Standardize: Always return 200 for successful request processing.
                # success: False in payload indicates extraction failure (0 episodes).
                self._send_json(result, status=200)
            except Exception as e:
                logger.exception(f"[SERIAL] {provider} parse failed")
                self._send_json(
                    {
                        "success": False,
                        "provider": provider,
                        "error": f"{provider} serial parse failed: {e}",
                    },
                    status=500,
                )
            return

        # Async serial extraction status:
        # GET /serial/extract/status/<job_id>
        elif path.startswith("/serial/extract/status/"):
            job_id = path.rsplit("/", 1)[-1].strip()
            if not job_id:
                self._send_error("Missing job_id parameter", 400)
                return
            snapshot = _serial_job_snapshot(job_id)
            if snapshot is None:
                self._send_error("serial extraction job not found", 404)
                return
            self._send_json(snapshot)
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
                        item = r.to_dict()
                    elif isinstance(r, dict):
                        item = dict(r)
                    else:
                        item = r
                    if isinstance(item, dict):
                        item.setdefault("confidence", 0.7)
                        item.setdefault("available_qualities", [])
                        item.setdefault("selected_quality", "")
                        item.setdefault("selected_video_url", "")
                    serialized_results.append(item)
                
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
                normalized_metadata["type"] = details.get("type", "")
                parsed_source_id = details.get("source_id", source_id)
                if source_id and ":s" in source_id and "e" in source_id.lower():
                    parsed_source_id = source_id
                normalized_metadata["source_id"] = parsed_source_id
                normalized_metadata["detail_url"] = details.get("detail_url", detail_url or source_id)
                normalized_metadata["video_urls"] = details.get("video_urls", [])
                
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
                video_urls_list = sort_video_candidates(video_urls_list)
                
                # Extract quality from selected video URL
                if video_url:
                    for v in video_urls_list:
                        if v.get('url') == video_url:
                            source_quality = normalize_quality_label(v.get('quality', ''))
                            break
                
                # Get all available qualities from video_urls
                for v in video_urls_list:
                    q = normalize_quality_label(v.get('quality', ''))
                    if q and q != 'unknown' and q not in available_qualities:
                        available_qualities.append(q)
                
                if not source_quality:
                    source_quality = normalize_quality_label(normalized_metadata.get("quality", ""))

                classifier_evidence = str(details.get("content_type_reason") or "")
                currentType = (details.get("type") or "").strip().lower()
                if currentType == "series":
                    currentType = "serial"
                if currentType not in ("movie", "serial"):
                    inferredType, inferredReason = detect_content_type(
                        details.get("detail_url", detail_url or source_id),
                        source,
                    )
                    currentType = inferredType
                    classifier_evidence = inferredReason
                    logger.info(f"[type] inferred={currentType} reason={inferredReason}")
                classifier_confidence = _classifier_confidence(currentType, classifier_evidence)
                logger.info(f"[classifier] type={currentType} confidence={classifier_confidence:.2f} evidence={classifier_evidence}")
                normalized_metadata["type"] = currentType
                parsed_source_id = details.get("source_id", source_id)
                if source_id and ":s" in source_id and "e" in source_id.lower():
                    parsed_source_id = source_id
                normalized_metadata["source_id"] = parsed_source_id
                normalized_metadata["detail_url"] = details.get("detail_url", detail_url or source_id)
                normalized_metadata["video_urls"] = video_urls_list

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

                    # Surface specific reason from parser (e.g. freekino_403)
                    parser_error = details.get('error', '') if isinstance(details, dict) else ''
                    parser_reason = details.get('error_reason', '') if isinstance(details, dict) else ''
                    http_status = details.get('http_status', 0) if isinstance(details, dict) else 0

                    if parser_error == 'freekino_403' or http_status == 403:
                        manual_reason = "Freekino blocked request / 403 Forbidden"
                        error_type = "video_url_not_found"
                    elif parser_error:
                        manual_reason = parser_reason or parser_error
                        error_type = "video_url_not_found"
                    elif player_url_found and ('://' in player_url_found and
                        any(bad in player_url_found.lower() for bad in ['blocked', 'captcha', 'cloudflare', '403', 'denied'])):
                        manual_reason = "site_blocked"
                        error_type = "video_url_not_found"
                        logger.error(f"[PARSER] DETECTED: Site likely blocked (player_url={player_url_found})")
                    else:
                        manual_reason = "403 Forbidden or no video urls"
                        error_type = "video_url_not_found"

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
                    response_payload["manual_reason"] = manual_reason
                    response_payload["reason"] = manual_reason
                    response_payload["source"] = source
                    response_payload["source_id"] = source_id
                    response_payload["detail_url"] = detail_url or ""
                    response_payload["video_found"] = False
                    response_payload["download_needed"] = False
                    response_payload["type"] = currentType
                    response_payload["source_id"] = normalized_metadata["source_id"]
                    response_payload["detail_url"] = normalized_metadata["detail_url"]
                    response_payload["classifier_confidence"] = classifier_confidence
                    response_payload["classifier_evidence"] = classifier_evidence
                    if http_status:
                        response_payload["http_status"] = http_status
                    logger.error(
                        f"[PARSER] /details -> 200 (partial) source={source} source_id={source_id} "
                        f"detail_url={detail_url or ''} reason={manual_reason}"
                    )
                    # Return 200 even if no video found, allowing metadata pickup.
                    # success: False + error: video_url_not_found will indicate failure to worker.
                    self._send_json(response_payload, 200)
                    return
                
                logger.info(f"[PARSER] Final video URL: {safe_truncate(video_url, 80)}... (type: {url_type})")
                logger.info(f"[PARSER] selected source url - type={url_type}, quality={source_quality or 'auto'}, url={safe_truncate(video_url, 120)}...")

                # /details returns metadata + source URL only. Downloading is
                # always delegated to /download so parser detail requests stay
                # fast and deterministic.
                response_payload = create_worker_payload(
                    source=source,
                    source_url=source_base_url,
                    page_url=details.get('video_page_url', detail_url or source_id),
                    video_url=video_url,
                    video_url_type=url_type,
                    metadata=normalized_metadata,
                    local_path="",  # No local_path yet - will be returned by /download
                    source_quality=source_quality,
                    available_qualities=available_qualities
                )
                
                # Add additional fields to indicate download is pending
                response_payload["success"] = True
                response_payload["video_found"] = True
                response_payload["download_needed"] = True  # Worker must call /download
                response_payload["download_completed"] = False
                response_payload["video_page_url"] = details.get('video_page_url', detail_url or source_id)
                response_payload["selected_quality"] = source_quality
                response_payload["selected_video_url"] = video_url
                response_payload["type"] = currentType
                response_payload["source_id"] = normalized_metadata["source_id"]
                response_payload["detail_url"] = normalized_metadata["detail_url"]
                response_payload["confidence"] = float(details.get("confidence") or 0.9)
                response_payload["classifier_confidence"] = classifier_confidence
                response_payload["classifier_evidence"] = classifier_evidence
                
                logger.info(f"[PARSER] /details returning metadata + source URL (download pending)")
                logger.info(f"[PARSER]   video_url: {video_url[:80]}...")
                logger.info(f"[PARSER]   quality: {source_quality or 'auto'}")
                logger.info(f"[PARSER]   worker must call /download to get local_path")
                
                self._send_json(response_payload)
                return
            
            except Exception as e:
                logger.error(f"Details error: {e}", exc_info=True)
                self._send_error(f"Failed to get details: {str(e)}", 500)
            return
        
        # NEW: /download endpoint - starts non-blocking background download
        elif path == "/download":
            source = query.get("source", [""])[0]
            video_url = query.get("video_url", [""])[0]
            job_id = query.get("job_id", [""])[0]
            output_name = query.get("output_name", [""])[0]
            quality = query.get("quality", [""])[0]
            selected_quality = query.get("selected_quality", [""])[0]
            referer = query.get("referer", [""])[0]
            force = query.get("force", [""])[0] == "1"
            # Defence in depth — the worker is supposed to unwrap embed
            # wrappers before calling /download, but if anything ever
            # passes an iframe URL here we'd silently 422 in the
            # downloader's URL validator. Apply the same unwrap that
            # _resolve_claimed_job_video uses on the queue path.
            video_url = _unwrap_embed_url(video_url)
            
            logger.info(f"[PARSER] /download called — job_id={job_id}")
            logger.info(f"[PARSER] new download started — job_id={job_id}, url={safe_truncate(video_url, 60)}")
            if selected_quality:
                logger.info(f"[download] job={job_id} source={source or 'unknown'} selected_quality={selected_quality}")
            if force and job_id:
                logger.info(f"[PARSER] force restart requested — job_id={job_id}")
                clear_active_download(job_id)
            
            if not video_url:
                self._send_error("download_url empty after parser resolve")
                return
            
            if job_id:
                output_name = build_job_output_name(job_id, output_name or "download")
            elif not output_name:
                output_name = build_job_output_name(job_id, "download")
            
            backend_job_id = job_id if job_id else ""
            
            # Check existing download state
            existing = get_active_download(job_id)
            if existing:
                if existing.status in ("starting", "downloading"):
                    logger.info(f"[PARSER] duplicate /download request reused existing download — job_id={job_id}, status={existing.status}")
                    self._send_json({
                        "success": True,
                        "status": "already_running",
                        "job_id": job_id,
                        "progress_percent": existing.progress_percent,
                        "downloaded_bytes": existing.downloaded_bytes,
                        "total_bytes": existing.total_bytes,
                        "message": "Download already in progress",
                    })
                    return
                elif existing.status == "completed" and existing.local_path:
                    logger.info(f"[PARSER] duplicate /download request returning existing result — job_id={job_id}, local_path={existing.local_path}")
                    self._send_json({
                        "success": True,
                        "status": "already_done",
                        "job_id": job_id,
                        "local_path": existing.local_path,
                        "file_size": existing.file_size,
                        "message": "Download already completed",
                    })
                    return
                elif existing.status == "failed":
                    # Allow retry after failure
                    logger.info(f"[PARSER] previous download failed, starting new — job_id={job_id}")
                    clear_active_download(job_id)
            
            # Create new download state
            state = DownloadState(
                job_id,
                video_url,
                output_name,
                source=source,
                detail_url=referer,
                selected_quality=selected_quality or quality,
            )
            set_active_download(job_id, state)
            logger.info(
                f"[PARSER] new background download started — job_id={job_id}, "
                f"source={source}, detail_url={safe_truncate(referer, 160)}, "
                f"video_url={safe_truncate(video_url, 160)}, "
                f"download_url={safe_truncate(video_url, 160)}, "
                f"selected_quality={selected_quality or quality or 'auto'}, "
                f"media_type={state.media_type}"
            )
            
            # Start download in background thread
            def background_download():
                try:
                    logger.info(f"[PARSER] background download worker starting — job_id={job_id}")
                    
                    # Progress callback to update state
                    def progress_callback(percent, downloaded, total, speed, eta):
                        state = get_active_download(job_id)
                        if state and state.status != "failed":
                            state.progress_percent = percent
                            state.downloaded_bytes = downloaded
                            state.total_bytes = total
                            state.speed_mbps = speed / (1024 * 1024) if speed > 0 else 0.0
                            state.eta_seconds = eta
                            state.file_size = downloaded
                            # Always update timestamp to indicate activity
                            state.last_progress_at = time.time()
                            state.status = "downloading"
                    
                    # PID callback to update state
                    def pid_callback(pid):
                        state = get_active_download(job_id)
                        if state:
                            state.pid = pid

                    def debug_callback(event, payload):
                        state = get_active_download(job_id)
                        if state:
                            state.apply_debug_event(event, payload or {})
                        if event == "command":
                            logger.info(
                                f"[download-debug] job_id={job_id} source={source or 'unknown'} "
                                f"detail_url={safe_truncate(referer, 180)} "
                                f"video_url={safe_truncate(video_url, 180)} "
                                f"download_url={safe_truncate(video_url, 180)} "
                                f"selected_quality={selected_quality or quality or 'auto'} "
                                f"media_type={(payload or {}).get('media_type', state.media_type if state else 'unknown')} "
                                f"temp_output_path={(payload or {}).get('output_path', '')} "
                                f"command={(payload or {}).get('command_string') or ' '.join(str(x) for x in (payload or {}).get('command', []))}"
                            )
                        elif event == "pid":
                            logger.info(f"[download-debug] job_id={job_id} downloader_pid={(payload or {}).get('pid', 0)}")
                        elif event == "exit":
                            logger.info(
                                f"[download-debug] job_id={job_id} exit_code={(payload or {}).get('exit_code')} "
                                f"stderr={safe_truncate(str((payload or {}).get('stderr') or ''), 1000)}"
                            )
                        elif event == "file_size":
                            logger.info(
                                f"[download-debug] job_id={job_id} temp_output_path={(payload or {}).get('output_path', '')} "
                                f"file_size={(payload or {}).get('file_size', 0)} "
                                f"last_progress_at={state.last_progress_at if state else 0}"
                            )

                    download_result = downloader_service.smart_download(
                        url=video_url,
                        output_name=output_name,
                        job_id=output_name,
                        backend_job_id=backend_job_id,
                        referer=referer if referer else None,
                        progress_callback=progress_callback,
                        pid_callback=pid_callback,
                        debug_callback=debug_callback,
                    )
                    
                    if not download_result.get("success"):
                        error_msg = download_result.get("error", "Download failed")
                        logger.error(f"[PARSER] background download failed — job_id={job_id}, error={error_msg}")
                        
                        # Auto-refresh expired URLs logic
                        if source and source in PARSERS and referer:
                            logger.info(f"[PARSER] Attempting to auto-refresh URL for failed download — job_id={job_id}, source={source}")
                            try:
                                # Signature-aware call: kinolar/kinochilar/uzmedia
                                # require source_id in addition to url; the
                                # other parsers take url only. Re-use the
                                # helper that already introspects this.
                                parser_inst = PARSERS[source]
                                # Build a kwargs dict the same way
                                # _get_details_from_parser does, then call.
                                import inspect as _inspect
                                _sig = _inspect.signature(parser_inst.get_details)
                                _kwargs = {}
                                if "source_id" in _sig.parameters:
                                    # Best-effort: pull source_id out of
                                    # the active DownloadState (the job
                                    # row's source_id isn't directly in
                                    # scope here).
                                    _state = get_active_download(job_id)
                                    _sid = getattr(_state, "backend_job_id", "") if _state else ""
                                    if _sid:
                                        _kwargs["source_id"] = _sid
                                fresh_details = parser_inst.get_details(referer, **_kwargs)
                                fresh_url = ""
                                target_quality = selected_quality or quality or ""
                                if fresh_details.video_urls:
                                    if target_quality:
                                        for v in fresh_details.video_urls:
                                            if target_quality.lower() in v.get("quality", "").lower():
                                                fresh_url = v.get("url")
                                                break
                                    if not fresh_url:
                                        fresh_url = fresh_details.video_urls[0].get("url")
                                # Unwrap embed wrappers — kinochilar /
                                # uzmedia / kinolar return playerjs.html
                                # iframe URLs which the downloader's
                                # validator would reject.
                                fresh_url = _unwrap_embed_url(fresh_url)
                                
                                if fresh_url and fresh_url != video_url:
                                    logger.info(f"[PARSER] Successfully fetched fresh URL for job_id={job_id}, retrying download...")
                                    state = get_active_download(job_id)
                                    if state:
                                        state.video_url = fresh_url
                                        state.download_url = fresh_url
                                        state.status = "starting"
                                    
                                    download_result = downloader_service.smart_download(
                                        url=fresh_url,
                                        output_name=output_name,
                                        job_id=output_name,
                                        backend_job_id=backend_job_id,
                                        referer=referer,
                                        progress_callback=progress_callback,
                                        pid_callback=pid_callback,
                                        debug_callback=debug_callback,
                                    )
                                    if not download_result.get("success"):
                                        error_msg = download_result.get("error", "Download failed after URL refresh")
                                        logger.error(f"[PARSER] background download failed again after refresh — job_id={job_id}, error={error_msg}")
                                else:
                                    logger.warning(f"[PARSER] No fresh URL found during auto-refresh for job_id={job_id}")
                            except Exception as e:
                                logger.error(f"[PARSER] Auto-refresh failed for job_id={job_id}: {e}")
                        
                        if not download_result.get("success"):
                            state = get_active_download(job_id)
                            if state:
                                state.status = "failed"
                                state.error = error_msg
                                state.done = True
                                state.exit_code = state.exit_code if state.exit_code is not None else -1
                            return
                    
                    local_path = resolve_downloaded_artifact(job_id, download_result.get("file_path", ""))
                    if not local_path:
                        raise Exception(f"Downloaded file does not exist: {download_result.get('file_path', '')}")
                    if local_path != download_result.get("file_path", ""):
                        logger.info(f"[AUTO_RECOVER] job={job_id} found file={local_path} -> repaired")
                    
                    file_size = os.path.getsize(local_path) if local_path else 0
                    if file_size == 0:
                        raise Exception(f"Downloaded file is empty: {local_path}")
                    
                    stream_type = download_result.get("type", "mp4")
                    
                    logger.info(f"[PARSER] background download completed — job_id={job_id}, local_path={local_path}, size={file_size}")
                    
                    # Update state to completed
                    state = get_active_download(job_id)
                    if state:
                        state.status = "completed"
                        state.done = True
                        state.progress_percent = 100
                        state.local_path = local_path
                        state.file_size = file_size
                        state.downloaded_bytes = file_size
                        state.total_bytes = file_size
                        state.file_size = file_size
                        state.last_progress_at = time.time()
                        state.media_type = stream_type
                        
                except Exception as e:
                    error_msg = str(e)
                    logger.error(f"[PARSER] background download error — job_id={job_id}, error={error_msg}")
                    state = get_active_download(job_id)
                    if state:
                        state.status = "failed"
                        state.error = error_msg
                        state.done = True
                        state.exit_code = state.exit_code if state.exit_code is not None else -1
            
            # Start background thread
            download_thread = threading.Thread(target=background_download, daemon=True)
            download_thread.start()
            
            # Return immediately
            logger.info(f"[PARSER] /download returning immediately — job_id={job_id}, status=started")
            self._send_json({
                "success": True,
                "status": "started",
                "job_id": job_id,
                "message": "Download started in background",
            })
            return
        
        # NEW: /progress endpoint - returns current download state
        elif path == "/progress":
            job_id = query.get("job_id", [""])[0]
            
            logger.info(f"[PARSER] /progress requested — job_id={job_id}")
            
            if not job_id:
                self._send_error("Missing 'job_id' parameter")
                return
            
            state = get_active_download(job_id)
            
            if not state:
                logger.info(f"[PARSER] /progress job_id not found — job_id={job_id}")
                self._send_json({
                    "success": False,
                    "status": "not_found",
                    "message": "No download found for this job_id",
                }, 404)
                return
            
            response = {
                "success": True,
                "status": state.status,
                "progress_percent": state.progress_percent,
                "downloaded_bytes": state.downloaded_bytes,
                "total_bytes": state.total_bytes,
                "speed_mbps": state.speed_mbps,
                "eta_seconds": state.eta_seconds,
                "local_path": state.local_path,
                "file_size": state.file_size,
                "error": state.error,
                "done": state.done,
                "pid": state.pid,
                "source": state.source,
                "detail_url": state.detail_url,
                "video_url": state.video_url,
                "download_url": state.download_url,
                "selected_quality": state.selected_quality,
                "media_type": state.media_type,
                "downloader_command": state.downloader_command,
                "downloader_command_string": state.downloader_command_string,
                "stdout_tail": state.stdout_tail,
                "stderr_tail": state.stderr_tail,
                "exit_code": state.exit_code,
                "temp_output_path": state.temp_output_path,
                "last_progress_at": state.last_progress_at,
            }
            
            logger.info(f"[PARSER] /progress response — job_id={job_id}, status={response['status']}, percent={response['progress_percent']}%")
            
            self._send_json(response)
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
            
            # Normalize plural UI values ("movies"/"serials") to the singular
            # form that catalog item `type` fields use ("movie"/"serial").
            # Without this, filtering against item["type"] always matches zero.
            raw_type_filter = type_filter
            if type_filter == "serials":
                type_filter = "serial"
            elif type_filter == "movies":
                type_filter = "movie"

            logger.info(f"[SERVER] Catalog request: source={source}, page={page}, limit={limit}, type_raw={raw_type_filter!r}, type_normalized={type_filter!r}, category_url={category_url!r}")
            if type_filter == "serial":
                logger.info(f"[SERVER] Catalog mode: SERIAL list requested for source={source}")

            if not source or source not in PARSERS:
                self._send_error(f"Invalid source for catalog. Available: {list(PARSERS.keys())}")
                return

            try:
                parser = PARSERS[source]

                # Check if parser has list_catalog method
                if hasattr(parser, 'list_catalog'):
                    catalog_result = parser.list_catalog(page=page, limit=limit, type_filter=type_filter, category_url=category_url)
                    logger.info(f"[SERVER] Catalog: source={source}, type={type_filter!r}, returned {len(catalog_result.get('items', []))} items")
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
            
            if type_filter == "serials":
                type_filter = "serial"
            elif type_filter == "movies":
                type_filter = "movie"
            logger.info(f"[SERVER] List request (alias for catalog): source={source}, page={page}, type={type_filter!r}")
            
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
            "uzmovi": f"https://uzmovi.net/page/{page}/",
            "freekino": f"https://freekino.net/page/{page}/",
            "asilmedia": f"http://asilmedia.org/page/{page}/",
            "kinolar": f"https://kinolar.tv/load/0-{page}",
            "kinochilar": f"https://kinochilar.com/page/{page}/",
            "uzmedia": f"https://uzmedia.tv/load/0-{page}",
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

        if path == "/serial/extract/start":
            try:
                body = self._read_json_body()
                serial_url = (body.get("url", "") or "").strip()
                source_hint = (body.get("source", "") or "").strip().lower()

                if not serial_url:
                    self._send_json({"ok": False, "error": "Missing 'url' field"}, status=400)
                    return

                provider = source_hint or _detect_serial_provider(serial_url)
                if provider not in SERIAL_PARSERS:
                    self._send_json({
                        "ok": False,
                        "error": f"Unsupported serial provider. Supported: {sorted(SERIAL_PARSERS.keys())}",
                    }, status=400)
                    return

                job_id = hashlib.sha1(f"{provider}:{serial_url}:{time.time()}:{random.random()}".encode("utf-8")).hexdigest()[:16]
                now = int(time.time())
                with _serial_jobs_lock:
                    _serial_extract_jobs[job_id] = {
                        "job_id": job_id,
                        "status": "queued",
                        "stage": "queued",
                        "provider": provider,
                        "url": serial_url,
                        "message": "Queued for extraction",
                        "episodes": [],
                        "expected_total": 0,
                        "discovered_count": 0,
                        "resolved_count": 0,
                        "missing_numbers": [],
                        "warnings": [],
                        "title": "",
                        "year": 0,
                        "poster": "",
                        "backdrop": "",
                        "description": "",
                        "error": "",
                        "created_at": now,
                        "updated_at": now,
                        "result": None,
                    }

                thread = threading.Thread(
                    target=_run_serial_extract_job,
                    args=(job_id, provider, serial_url),
                    daemon=True,
                )
                thread.start()
                self._send_json(
                    {
                        "ok": True,
                        "parser_job_id": job_id,
                        "job_id": job_id,
                        "status": "queued",
                        "provider": provider,
                        "message": "started",
                    },
                    status=202,
                )
            except Exception as exc:
                full_traceback = traceback.format_exc()
                logger.error(f"[SERIAL] async start failed: {exc}\n{full_traceback}")
                self._send_json({"ok": False, "error": str(exc)}, status=500)
            return
        
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
                    output_name = build_job_output_name(backend_job_id, title) if backend_job_id else build_job_output_name(output_name or title, "manual_import")
                    
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
                
                if backend_job_id:
                    output_name = build_job_output_name(backend_job_id, source_id or source or "download")
                elif not output_name:
                    # Generate default output name from source_id or URL
                    if source_id:
                        output_name = build_job_output_name(f"{source}_{source_id}", "download")
                    else:
                        url_hash = hashlib.md5(detail_url.encode()).hexdigest()[:8]
                        output_name = build_job_output_name(f"{source}_{url_hash}", "download")
                
                # Ensure output_name has proper extension
                if not output_name.endswith((".mp4", ".m3u8", ".mkv")):
                    output_name += ".mp4"
                
                logger.info(f"[SERVER] Download request: source={source}, id={source_id}, url={detail_url[:50] if detail_url else 'none'}...")

                # Job-scoped state for the auto-fix wrapper. These are read in
                # both success and failure paths below.
                _autofix_job_started_at = time.time()
                _autofix_recovery_attempts = 0
                _autofix_used_fallback_source = None
                _autofix_seen_urls: set = set()

                # Get parser and fetch details. If the parser fails (network reset,
                # site down, parsing exception), the auto-fix path below will try
                # iframe extraction / alternate sources rather than 500'ing.
                parser = PARSERS[source]
                details = None
                details_dict: dict = {}
                _get_details_err = None
                try:
                    details = self._get_details_from_parser(parser, source, source_id, detail_url)
                    if hasattr(details, 'to_dict'):
                        details_dict = details.to_dict()
                    elif isinstance(details, dict):
                        details_dict = details
                    else:
                        details_dict = {}
                except Exception as _gd_err:
                    _get_details_err = _gd_err
                    logger.warning(
                        f"[SERVER] AUTO-FIX: get_details failed on source={source} — "
                        f"falling through to recovery: {_gd_err}"
                    )
                    _autofix_recovery_attempts += 1

                # Extract best video URL
                video_url, url_type = self._extract_best_video_url(details, source)

                # === AUTO-FIX (URL topilmadi): iframe / playwright fallback ===
                if not video_url and detail_url:
                    logger.warning(
                        f"[SERVER] AUTO-FIX: primary extraction returned no playable URL — "
                        f"running iframe/playwright recovery"
                    )
                    _autofix_recovery_attempts += 1
                    recovered = recovery.find_iframe_candidates(parser, detail_url)
                    if not recovered:
                        recovered = recovery.try_playwright_extraction(parser, detail_url)
                    if recovered:
                        merged = list(details_dict.get("video_urls") or []) + recovered
                        details_dict["video_urls"] = merged
                        details = details_dict
                        video_url, url_type = self._extract_best_video_url(details, source)
                        if video_url:
                            logger.info(
                                f"[SERVER] AUTO-FIX: recovery produced playable URL "
                                f"type={url_type} url={video_url[:80]}"
                            )

                # === AUTO-FIX (URL topilmadi): try alternate source ===
                if not video_url:
                    title_for_search = (
                        details_dict.get("title_original")
                        or details_dict.get("title")
                        or details_dict.get("title_uz")
                        or ""
                    )
                    year_for_search = details_dict.get("year")
                    alt = recovery.try_alternate_sources(
                        title_for_search, year_for_search, source, PARSERS,
                    )
                    if alt:
                        alt_source, alt_details, _alt_vurls = alt
                        _autofix_used_fallback_source = alt_source
                        _autofix_recovery_attempts += 1
                        source = alt_source
                        parser = PARSERS[alt_source]
                        details_dict = alt_details
                        details = alt_details
                        video_url, url_type = self._extract_best_video_url(details, source)
                        if video_url:
                            logger.info(
                                f"[SERVER] AUTO-FIX: switched to alternate source={alt_source} "
                                f"url={video_url[:80]}"
                            )

                if not video_url:
                    logger.error(f"[SERVER] No playable video URL found for source '{source}' (after recovery)")
                    record_outcome(
                        job_id=body.get("job_id", "") or "-",
                        source=source,
                        outcome=OUTCOME_FAIL,
                        error_category=ERR_NO_VIDEO_URL,
                        error_message="no playable video URL found after recovery cascade",
                        detail_url=detail_url,
                        duration_seconds=time.time() - _autofix_job_started_at,
                        recovery_attempts=_autofix_recovery_attempts,
                        used_fallback_source=_autofix_used_fallback_source,
                    )
                    self._send_json({
                        "success": False,
                        "error": "No playable video URL found",
                        "source": source,
                        "details": details_dict,
                    }, 400)
                    return

                _autofix_seen_urls.add(video_url)
                
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
                    
                    # Generate internal job_id for downloader-local progress tracking.
                    internal_job_id = hashlib.md5(output_name.encode()).hexdigest()[:12]

                    if backend_job_id:
                        logger.info(f"[SERVER] Download progress will be reported directly by downloader_service for job_id={backend_job_id}")
                        self._report_progress_to_backend(backend_job_id, {
                            "stage": "download",
                            "status": "downloading",
                            "progress_percent": 0,
                            "message": "Starting download...",
                        })
                    else:
                        logger.warning(f"[SERVER] No backend_job_id provided, progress will not be reported!")
                    
                    # Now start the actual download (this is blocking).
                    # === AUTO-FIX WRAPPER ===
                    # smart_download already retries internal strategies, but it
                    # always retries the SAME URL. Many "stuck at 0%" and 403
                    # failures come from CDN URLs that expire between resolve
                    # and download. On those classes of errors we re-fetch
                    # details (or fall back to an alternate source) to grab a
                    # fresh URL, then call smart_download again.
                    MAX_RERESOLVES = 2
                    _reresolves_used = 0
                    _alt_source_used = bool(_autofix_used_fallback_source)
                    _last_download_err = None
                    result = None

                    while True:
                        logger.info(
                            f"[SERVER] Starting download: {output_name} "
                            f"(reresolves_used={_reresolves_used}, source={source})"
                        )
                        try:
                            result = downloader_service.smart_download(
                                url=video_url,
                                output_name=output_name,
                                job_id=internal_job_id,
                                backend_job_id=backend_job_id,
                                referer=referer if referer else None,
                            )
                            break  # success
                        except Exception as _dl_err:
                            _last_download_err = _dl_err
                            _err_msg = str(_dl_err)
                            _err_category = recovery.classify_download_error(_err_msg)
                            logger.warning(
                                f"[SERVER] AUTO-FIX: smart_download failed "
                                f"category={_err_category} err={_err_msg[:200]}"
                            )

                            # Option 1: re-resolve embed with the SAME source.
                            if (
                                _reresolves_used < MAX_RERESOLVES
                                and recovery.is_recoverable_via_reresolve(_err_category)
                            ):
                                _reresolves_used += 1
                                _autofix_recovery_attempts += 1
                                logger.info(
                                    f"[SERVER] AUTO-FIX: re-resolving embed on {source} "
                                    f"(attempt {_reresolves_used}/{MAX_RERESOLVES})"
                                )
                                fresh_cands = []
                                try:
                                    fresh = self._get_details_from_parser(parser, source, source_id, detail_url)
                                    fresh_d = (
                                        fresh.to_dict() if hasattr(fresh, "to_dict")
                                        else (fresh if isinstance(fresh, dict) else {})
                                    )
                                    fresh_cands = list(fresh_d.get("video_urls") or [])
                                except Exception as _re:
                                    logger.warning(f"[SERVER] AUTO-FIX: re-fetch details failed: {_re}")
                                if not fresh_cands:
                                    fresh_cands = recovery.find_iframe_candidates(parser, detail_url)
                                picked = recovery.next_unseen_candidate(fresh_cands, _autofix_seen_urls)
                                if picked:
                                    new_url, new_type, new_ref = picked
                                    video_url = new_url
                                    url_type = new_type
                                    if new_ref:
                                        referer = new_ref
                                    _autofix_seen_urls.add(video_url)
                                    logger.info(
                                        f"[SERVER] AUTO-FIX: switching to fresh URL "
                                        f"type={url_type} url={video_url[:80]}"
                                    )
                                    continue
                                logger.info("[SERVER] AUTO-FIX: no unseen candidate from re-resolve")

                            # Option 2: try an alternate source for the same title.
                            if (
                                not _alt_source_used
                                and recovery.is_recoverable_via_alternate_source(_err_category)
                            ):
                                title_for_search = (
                                    details_dict.get("title_original")
                                    or details_dict.get("title")
                                    or details_dict.get("title_uz")
                                    or ""
                                )
                                year_for_search = details_dict.get("year")
                                alt = recovery.try_alternate_sources(
                                    title_for_search, year_for_search, source, PARSERS,
                                )
                                if alt:
                                    alt_source, alt_details, _alt_vurls = alt
                                    _alt_source_used = True
                                    _autofix_recovery_attempts += 1
                                    _autofix_used_fallback_source = alt_source
                                    source = alt_source
                                    parser = PARSERS[alt_source]
                                    details_dict = alt_details
                                    details = alt_details
                                    new_vurl, new_vtype = self._extract_best_video_url(details, source)
                                    if new_vurl:
                                        video_url = new_vurl
                                        url_type = new_vtype
                                        referer = alt_details.get("video_page_url", "") or referer
                                        _autofix_seen_urls.add(video_url)
                                        logger.info(
                                            f"[SERVER] AUTO-FIX: switched to alternate source={alt_source} "
                                            f"url={video_url[:80]}"
                                        )
                                        continue

                            # No more recovery options left — propagate.
                            raise
                    
                    logger.info(f"[PARSER] Parallel download completed")
                    logger.info(f"[PARSER] Merged parts into {result['file_path']}")
                    logger.info(f"[PARSER] Removed temporary part files")
                    logger.info(f"[PARSER] Downloader reported backend completion and ready_to_process state")
                    
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
                    
                    record_outcome(
                        job_id=backend_job_id or internal_job_id or "-",
                        source=source,
                        outcome=OUTCOME_SUCCESS,
                        detail_url=detail_url,
                        duration_seconds=time.time() - _autofix_job_started_at,
                        recovery_attempts=_autofix_recovery_attempts,
                        used_fallback_source=_autofix_used_fallback_source,
                    )
                    if _autofix_used_fallback_source or _autofix_recovery_attempts:
                        response_data["auto_fix"] = {
                            "recovery_attempts": _autofix_recovery_attempts,
                            "used_fallback_source": _autofix_used_fallback_source,
                        }
                    self._send_json(response_data)

                except Exception as download_error:
                    logger.error(f"[SERVER] Download failed: {download_error}")
                    _err_cat = recovery.classify_download_error(str(download_error))
                    record_outcome(
                        job_id=backend_job_id or internal_job_id or "-",
                        source=source,
                        outcome=OUTCOME_FAIL,
                        error_category=_err_cat,
                        error_message=str(download_error),
                        detail_url=detail_url,
                        duration_seconds=time.time() - _autofix_job_started_at,
                        recovery_attempts=_autofix_recovery_attempts,
                        used_fallback_source=_autofix_used_fallback_source,
                    )
                    self._send_json({
                        "success": False,
                        "error": f"Download failed: {str(download_error)}",
                        "source": source,
                        "details": details_dict,
                        "selected_media": {
                            "url": video_url,
                            "type": url_type,
                        },
                        "auto_fix": {
                            "recovery_attempts": _autofix_recovery_attempts,
                            "used_fallback_source": _autofix_used_fallback_source,
                            "error_category": _err_cat,
                        },
                    }, 500)
                
            except json.JSONDecodeError as e:
                logger.error(f"[SERVER] Invalid JSON in request body: {e}")
                self._send_error(f"Invalid JSON in request body: {str(e)}", 400)
            except Exception as e:
                logger.error(f"[SERVER] Download endpoint error: {e}", exc_info=True)
                # Best-effort telemetry capture — these locals may not be bound
                # if the error fired before they were assigned.
                try:
                    record_outcome(
                        job_id=locals().get("backend_job_id", "") or locals().get("internal_job_id", "") or "-",
                        source=locals().get("source", "unknown") or "unknown",
                        outcome=OUTCOME_FAIL,
                        error_category=recovery.classify_download_error(str(e)),
                        error_message=str(e),
                        detail_url=locals().get("detail_url"),
                        duration_seconds=time.time() - locals().get("_autofix_job_started_at", time.time()),
                        recovery_attempts=locals().get("_autofix_recovery_attempts", 0),
                        used_fallback_source=locals().get("_autofix_used_fallback_source"),
                    )
                except Exception as _tel_err:
                    logger.warning(f"[SERVER] telemetry record_outcome failed: {_tel_err}")
                self._send_error(f"Download failed: {str(e)}", 500)
            return

        # /instagram/upload — upload a video clip as an Instagram Reel
        elif path == "/instagram/upload":
            try:
                query = parse_qs(parsed.query)
                body = self._read_json_body()
                username = body.get("username", "")
                password = body.get("password", "")
                video_url = body.get("video_url", "")
                caption = body.get("caption", "")
                publish_key = (body.get("publish_key") or query.get("publish_key", [""])[0] or "").strip()

                # Idempotency: if we already published for this publish_key,
                # return the saved success record without re-uploading.
                if publish_key:
                    existing = _ig_load_publish_success(publish_key)
                    if existing and existing.get("status") == "success":
                        logger.info(
                            f"[Instagram] idempotent hit publish_key={publish_key} "
                            f"media_id={existing.get('media_id')}"
                        )
                        self._send_json(existing)
                        return

                if not video_url:
                    self._send_error("video_url is required", 400)
                    return

                account_param = query.get("account", [""])[0].strip()
                requested_account = (
                    account_param
                    or body.get("account_name", "")
                    or body.get("account", "")
                    or "main"
                ).strip()

                logger.info(
                    f"[Instagram] upload requested account_param={account_param or '-'} "
                    f"requested_account={requested_account or '-'} url={video_url}"
                )

                try:
                    from instagrapi import Client  # noqa: F401
                except ImportError:
                    self._send_error("instagrapi not installed — run: pip install instagrapi", 500)
                    return

                import tempfile

                tmp_path = None
                try:
                    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
                        tmp_path = f.name
                    logger.info(f"[Instagram] download prepare received_url={video_url} final_resolved_url={video_url}")
                    _download_remote_video_to_path(video_url, tmp_path)
                    video_path = Path(tmp_path)
                    accounts_to_try = _ig_accounts_to_try(requested_account)
                    last_error = None

                    for index, account in enumerate(accounts_to_try):
                        account_config = _ig_get_account_config(
                            account,
                            body_username=username,
                            body_password=password,
                        )
                        logger.info(
                            f"[Instagram] resolved account={account_config['account']} "
                            f"username={account_config['username'] or '-'} "
                            f"session_file={account_config['session_file']} "
                            f"exists={account_config['session_file'].exists()}"
                        )
                        try:
                            result = _ig_upload_for_account(account_config, video_path, caption, publish_key=publish_key)
                            self._send_json(result)
                            return
                        except InstagramUploadError as exc:
                            last_error = exc
                            logger.error(
                                f"[Instagram] final fail account={exc.account} "
                                f"type={exc.error_type}: {exc.message}"
                            )
                            if index < len(accounts_to_try) - 1:
                                logger.warning(
                                    f"[Instagram] switching account {account_config['account']} -> "
                                    f"{accounts_to_try[index + 1]}"
                                )
                            else:
                                self._send_json({
                                    "status": "failed",
                                    "account": exc.account,
                                    "error_type": exc.error_type,
                                    "action_required": exc.action_required,
                                    "message": exc.message,
                                    "error": exc.message,
                                })
                                return
                    if last_error:
                        raise last_error
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
                    from google.auth.exceptions import RefreshError
                    from googleapiclient.discovery import build
                    from googleapiclient.http import MediaFileUpload
                    from googleapiclient.errors import HttpError
                except ImportError as _ie:
                    import sys as _sys
                    self._send_error(
                        f"Missing YouTube dependency: {_ie}. "
                        f"Run: {_sys.executable} -m pip install google-api-python-client google-auth google-auth-oauthlib google-auth-httplib2",
                        500,
                    )
                    return

                import tempfile, json as _json
                from datetime import datetime

                # Required scope for uploading; if the saved token has different scopes,
                # Google returns 403 on videos().insert() — so we always pin this.
                YT_UPLOAD_SCOPES = ["https://www.googleapis.com/auth/youtube.upload"]

                token_path = Path(__file__).parent / token_file
                if not token_path.exists():
                    self._send_error(f"token_file not found: {token_file}", 400)
                    return

                creds_data = _json.loads(token_path.read_text())

                # Parse stored expiry (ISO string) so the library knows token age
                expiry_dt = None
                if creds_data.get("expiry"):
                    try:
                        expiry_dt = datetime.fromisoformat(creds_data["expiry"].replace("Z", ""))
                    except Exception as _exp_err:
                        logger.warning(f"[YouTube] could not parse stored expiry: {_exp_err}")

                # Use the scopes saved at login time; fall back to upload scope if missing
                stored_scopes = creds_data.get("scopes") or YT_UPLOAD_SCOPES

                creds_kwargs = {
                    "token": creds_data.get("token"),
                    "refresh_token": creds_data.get("refresh_token"),
                    "token_uri": creds_data.get("token_uri", "https://oauth2.googleapis.com/token"),
                    "client_id": creds_data.get("client_id"),
                    "client_secret": creds_data.get("client_secret"),
                    "scopes": stored_scopes,
                }
                creds = Credentials(**creds_kwargs)
                if expiry_dt is not None:
                    creds.expiry = expiry_dt

                # Sanitized debug: never log the full bearer token
                _tok = creds_data.get("token") or ""
                _tok_preview = (_tok[:8] + "...") if _tok else "<empty>"
                logger.info(
                    f"[YouTube] account={account_name} scopes={stored_scopes} "
                    f"token_prefix=Bearer {_tok_preview} expired={creds.expired} "
                    f"has_refresh_token={bool(creds.refresh_token)}"
                )

                # Verify the upload scope is actually granted
                if "https://www.googleapis.com/auth/youtube.upload" not in stored_scopes:
                    logger.error(
                        f"[YouTube] account={account_name} missing youtube.upload scope; got {stored_scopes}"
                    )
                    self._send_json({
                        "status": "failed",
                        "error": "token expired or invalid",
                        "error_type": "token_invalid",
                        "detail": f"Token does not include youtube.upload scope. Re-run yt_login.py for account '{account_name}'.",
                    })
                    return

                if creds.expired and creds.refresh_token:
                    try:
                        creds.refresh(Request())
                    except RefreshError as _rerr:
                        logger.error(
                            f"[YouTube] token refresh failed account={account_name}: {_rerr}",
                            exc_info=True,
                        )
                        self._send_json({
                            "status": "failed",
                            "error": "token expired or invalid",
                            "error_type": "token_invalid",
                            "detail": f"Refresh failed: {_rerr}. Re-run yt_login.py for account '{account_name}'.",
                        })
                        return
                    creds_data["token"] = creds.token
                    if creds.expiry:
                        creds_data["expiry"] = creds.expiry.isoformat()
                    creds_data["scopes"] = stored_scopes
                    token_path.write_text(_json.dumps(creds_data, indent=2))
                    logger.info(f"[YouTube] token refreshed and persisted for account={account_name}")

                youtube = build("youtube", "v3", credentials=creds)

                tmp_path = None
                try:
                    with tempfile.NamedTemporaryFile(suffix=".mp4", delete=False) as f:
                        tmp_path = f.name
                    logger.info(f"[YouTube] downloading video to {tmp_path}")
                    _download_remote_video_to_path(video_url, tmp_path)

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
                    logger.info(
                        f"[YouTube] POST https://www.googleapis.com/upload/youtube/v3/videos "
                        f"(part=snippet,status) account={account_name}"
                    )
                    response = None
                    while response is None:
                        _, response = request.next_chunk()

                    video_id = response.get("id", "")
                    logger.info(f"[YouTube] upload success video_id={video_id} account={account_name}")
                    self._send_json({"status": "success", "video_id": video_id, "account": account_name, "platform": "youtube"})
                except HttpError as http_err:
                    status = getattr(getattr(http_err, "resp", None), "status", 0)
                    raw_content = b""
                    try:
                        raw_content = http_err.content or b""
                    except Exception:
                        pass
                    body_text = raw_content.decode("utf-8", errors="replace") if raw_content else ""
                    logger.error(
                        f"[YouTube] HttpError account={account_name} status={status} body={body_text[:1000]}"
                    )
                    if status in (401, 403):
                        self._send_json({
                            "status": "failed",
                            "error": "token expired or invalid",
                            "error_type": "token_invalid",
                            "http_status": status,
                            "detail": body_text[:500],
                        })
                    else:
                        self._send_json({
                            "status": "failed",
                            "error": str(http_err),
                            "error_type": "upload_failed",
                            "http_status": status,
                            "detail": body_text[:500],
                        })
                except Exception as e:
                    logger.error(f"[YouTube] upload error account={account_name}: {e}", exc_info=True)
                    self._send_json({"status": "failed", "error": str(e), "error_type": "upload_failed"})
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

                # ── Whisper speech analysis ───────────────────────────────────
                # Full-movie CPU transcription was taking 30–90+ min per job and
                # stalling the pipeline at "creating_movie". Two changes:
                #   1. Cache the loaded model on the handler class so the ~5s
                #      model load isn't repeated for every call (3 concurrent
                #      jobs each reloaded ~150MB into RAM).
                #   2. Cap audio extraction to the first AI_AUDIO_LIMIT_SEC
                #      seconds. Clip selection only needs *some* speech signal
                #      to bias toward dialogue-rich moments; we don't need a
                #      full transcript of a 2-hour film.
                tmp_audio = None
                AI_AUDIO_LIMIT_SEC = int(_os.environ.get("AI_AUDIO_LIMIT_SEC", "600"))
                try:
                    import whisper as _whisper
                    tmp_audio = _tmp.mktemp(suffix=".wav")
                    ffmpeg_cmd = ["ffmpeg", "-y", "-i", video_path,
                                  "-ac", "1", "-ar", "16000"]
                    if AI_AUDIO_LIMIT_SEC > 0:
                        ffmpeg_cmd += ["-t", str(AI_AUDIO_LIMIT_SEC)]
                    ffmpeg_cmd.append(tmp_audio)
                    _sp.run(ffmpeg_cmd, capture_output=True)
                    model = getattr(ParserHandler, "_whisper_model", None)
                    if model is None:
                        model = _whisper.load_model("base")
                        ParserHandler._whisper_model = model
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


def _cleanup_stale_state():
    """Remove leftover .tmp / .part files and stale progress JSON on startup."""
    import time as _time
    parser_root = Path(__file__).parent
    removed = 0

    # Stale download artifacts
    if os.path.isdir(DOWNLOAD_DIR):
        for entry in os.listdir(DOWNLOAD_DIR):
            if entry.endswith((".tmp", ".part", ".aria2", ".ytdl")):
                full = os.path.join(DOWNLOAD_DIR, entry)
                try:
                    if _time.time() - os.path.getmtime(full) > 600:  # >10 min old
                        os.remove(full)
                        removed += 1
                except OSError:
                    pass

    # Stale progress checkpoints (older than 1h)
    progress_dir = parser_root / "progress"
    if progress_dir.is_dir():
        cutoff = _time.time() - 3600
        for p in progress_dir.glob("*.json"):
            try:
                if p.stat().st_mtime < cutoff:
                    p.unlink()
                    removed += 1
            except OSError:
                pass

    if removed:
        logger.info(f"[STARTUP] Cleaned up {removed} stale download/progress file(s)")


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
    
    # Clean up stale state from previous runs:
    # - orphaned .tmp / .part / partial chunk files in DOWNLOAD_DIR
    # - stuck progress JSON files older than 1 hour
    try:
        _cleanup_stale_state()
    except Exception as _e:
        logger.warning(f"[STARTUP] Stale-state cleanup failed: {_e}")

    server_address = (host, port)
    # Set the server address string for use in handlers
    ParserHandler.server_address_str = f"http://{host}:{port}"
    httpd = ThreadedHTTPServer(server_address, ParserHandler)
    logger.info(f"Parser API server running on http://{host}:{port}")
    logger.info(f"Available sources: {AVAILABLE_SOURCES}")
    logger.info(f"Parser base URL for workers: {ParserHandler.server_address_str}")
    start_download_queue(ParserHandler.server_address_str)
    
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
