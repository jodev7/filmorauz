"""
Parser failure / success telemetry.

In-memory only (lost on restart). Records the outcome of every ingest job so
operators can quickly see which sources are flaky and what the latest failures
were. Exposed via GET /parser/health on the parser HTTP server.

No external dependencies — safe to import everywhere.
"""
from __future__ import annotations

import threading
import time
from collections import deque
from dataclasses import dataclass, field
from typing import Deque, Dict, List, Optional


# Outcome categories used by callers.
OUTCOME_SUCCESS = "success"
OUTCOME_FAIL = "fail"

# Error categories — used when OUTCOME_FAIL. Stable strings; the health
# endpoint groups failures by these.
ERR_NO_VIDEO_URL = "no_video_url"           # parser returned empty / all rejected
ERR_VALIDATION = "validation_failed"        # _validate_download_target rejected the URL
ERR_CDN_EXPIRED = "cdn_expired"             # 403 / signed URL no longer valid
ERR_BLOCKED = "blocked_by_origin"           # 403 with bot-block markers
ERR_NOT_FOUND = "not_found"                 # 404
ERR_DOWNLOAD_STALL = "download_stall"       # watchdog killed for inactivity
ERR_NETWORK = "network_error"               # DNS / TCP / TLS error
ERR_HTML_RESPONSE = "html_response"         # selected URL pointed at HTML, not media
ERR_DOWNLOADER = "downloader_error"         # ffmpeg / m3u8DL-RE / aria2c crashed
ERR_UNKNOWN = "unknown"

RING_CAPACITY = 200


@dataclass
class JobOutcomeRecord:
    job_id: str
    source: str
    outcome: str                       # OUTCOME_SUCCESS or OUTCOME_FAIL
    error_category: Optional[str] = None
    error_message: Optional[str] = None
    detail_url: Optional[str] = None
    duration_seconds: float = 0.0
    recovery_attempts: int = 0          # how many re-resolves / fallbacks were used
    used_fallback_source: Optional[str] = None
    timestamp: float = field(default_factory=time.time)


class TelemetryStore:
    """Thread-safe ring buffer + per-source aggregates."""

    def __init__(self, capacity: int = RING_CAPACITY) -> None:
        self._lock = threading.Lock()
        self._ring: Deque[JobOutcomeRecord] = deque(maxlen=capacity)
        # source -> {"total": int, "success": int, "fail": int, "errors": {category: int}}
        self._per_source: Dict[str, Dict] = {}

    def record(self, rec: JobOutcomeRecord) -> None:
        with self._lock:
            self._ring.append(rec)
            agg = self._per_source.setdefault(
                rec.source,
                {"total": 0, "success": 0, "fail": 0, "errors": {}},
            )
            agg["total"] += 1
            if rec.outcome == OUTCOME_SUCCESS:
                agg["success"] += 1
            else:
                agg["fail"] += 1
                cat = rec.error_category or ERR_UNKNOWN
                agg["errors"][cat] = agg["errors"].get(cat, 0) + 1

    def health_snapshot(self, recent_n: int = 20) -> Dict:
        """Return a JSON-serialisable summary suitable for the /parser/health endpoint."""
        with self._lock:
            per_source = {}
            for src, agg in self._per_source.items():
                total = agg["total"] or 1
                per_source[src] = {
                    "total": agg["total"],
                    "success": agg["success"],
                    "fail": agg["fail"],
                    "success_rate": round(agg["success"] / total, 3),
                    "errors_by_category": dict(agg["errors"]),
                }
            recent = [
                {
                    "job_id": r.job_id,
                    "source": r.source,
                    "outcome": r.outcome,
                    "error_category": r.error_category,
                    "error_message": (r.error_message or "")[:300],
                    "detail_url": (r.detail_url or "")[:200],
                    "duration_seconds": round(r.duration_seconds, 2),
                    "recovery_attempts": r.recovery_attempts,
                    "used_fallback_source": r.used_fallback_source,
                    "timestamp": int(r.timestamp),
                }
                for r in list(self._ring)[-recent_n:]
            ]
            recent.reverse()  # newest first
            return {
                "captured_jobs": len(self._ring),
                "capacity": self._ring.maxlen,
                "per_source": per_source,
                "recent": recent,
            }

    def reset(self) -> None:
        with self._lock:
            self._ring.clear()
            self._per_source.clear()


# Global singleton — import this and call .record() / .health_snapshot()
TELEMETRY = TelemetryStore()


def record_outcome(
    *,
    job_id: str,
    source: str,
    outcome: str,
    error_category: Optional[str] = None,
    error_message: Optional[str] = None,
    detail_url: Optional[str] = None,
    duration_seconds: float = 0.0,
    recovery_attempts: int = 0,
    used_fallback_source: Optional[str] = None,
) -> None:
    """Convenience wrapper around TELEMETRY.record()."""
    TELEMETRY.record(JobOutcomeRecord(
        job_id=job_id or "-",
        source=source or "unknown",
        outcome=outcome,
        error_category=error_category,
        error_message=error_message,
        detail_url=detail_url,
        duration_seconds=duration_seconds,
        recovery_attempts=recovery_attempts,
        used_fallback_source=used_fallback_source,
    ))
