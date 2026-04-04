"""
Configuration management for Watermark Removal Service

Provides configuration via environment variables with sensible defaults.

Environment Variables:
    WATERMARK_ENABLED: Enable/disable watermark removal (default: true)
    WATERMARK_MODE: Processing mode - 'fast' (OpenCV) or 'pro' (LaMa) (default: fast)
    WATERMARK_SAMPLE_COUNT: Number of frames to sample for detection (default: 10)
    WATERMARK_CONFIDENCE_THRESHOLD: Minimum confidence threshold 0.0-1.0 (default: 0.65)
    WATERMARK_MAX_REGIONS: Maximum watermark regions to detect (default: 3)
    WATERMARK_MASK_PADDING: Mask expansion padding in pixels (default: 8)
    WATERMARK_TEMP_DIR: Temporary directory for processing (default: /tmp/filmora_watermark)
    WATERMARK_KEEP_DEBUG_FRAMES: Keep debug frames for analysis (default: false)
    WATERMARK_PRO_FALLBACK_TO_FAST: Fallback from PRO to FAST mode (default: true)
    WATERMARK_USE_OCR: Enable OCR-based text detection (default: true)
    WATERMARK_USE_YOLO: Enable YOLO-based logo detection (default: false)
    WATERMARK_YOLO_MODEL: Path to YOLO model file (optional)
    WATERMARK_LAMA_MODEL: Path to LaMa model directory (optional)
    WATERMARK_NUM_WORKERS: Parallel processing workers (default: 4)
    WATERMARK_INPAINT_TIMEOUT: Inpainting timeout in seconds (default: 300)
    WATERMARK_LOG_LEVEL: Logging level - DEBUG, INFO, WARNING, ERROR (default: INFO)
    WATERMARK_SERVICE_URL: HTTP service URL for remote processing (optional)
    WATERMARK_PYTHON_PATH: Python interpreter path (default: python3)
"""

import os
from dataclasses import dataclass, field
from typing import Optional


@dataclass
class WatermarkConfig:
    """Configuration for watermark removal pipeline."""
    
    # Enable/disable watermark removal
    enabled: bool = True
    
    # Processing mode: 'fast' (OpenCV) or 'pro' (LaMa)
    mode: str = "fast"
    
    # Number of frames to sample for watermark detection
    sample_count: int = 10
    
    # Mask expansion padding in pixels
    mask_padding: int = 8
    
    # Minimum confidence threshold (0.0-1.0)
    confidence_threshold: float = 0.65
    
    # Maximum watermark regions to detect
    max_regions: int = 3
    
    # Temporary directory for processing
    temp_dir: str = "/tmp/filmora_watermark"
    
    # Timeout for inpainting (seconds)
    inpaint_timeout: int = 300
    
    # Parallel processing workers
    num_workers: int = 4
    
    # Edge region margins (percentage of frame dimension)
    edge_margin_x: float = 0.15
    edge_margin_y: float = 0.15
    
    # OCR enabled for text watermark detection
    ocr_enabled: bool = True
    
    # Use YOLO for logo detection (optional)
    use_yolo: bool = False
    
    # YOLO model path
    yolo_model_path: str = ""
    
    # LaMa model path (for PRO mode)
    lama_model_path: str = ""
    
    # PRO mode fallback to FAST
    pro_fallback_to_fast: bool = True
    
    # Keep debug frames
    keep_debug_frames: bool = False
    
    # Logging level
    log_level: str = "INFO"
    
    # HTTP service URL (optional - for remote processing)
    service_url: str = ""
    
    # Python interpreter path
    python_path: str = "python3"
    
    @property
    def use_pro_mode(self) -> bool:
        """Check if PRO mode is enabled."""
        return self.mode == "pro"
    
    @property
    def use_fast_mode(self) -> bool:
        """Check if FAST mode is enabled."""
        return self.mode == "fast"
    
    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "enabled": self.enabled,
            "mode": self.mode,
            "sample_count": self.sample_count,
            "mask_padding": self.mask_padding,
            "confidence_threshold": self.confidence_threshold,
            "max_regions": self.max_regions,
            "temp_dir": self.temp_dir,
            "inpaint_timeout": self.inpaint_timeout,
            "num_workers": self.num_workers,
            "edge_margin_x": self.edge_margin_x,
            "edge_margin_y": self.edge_margin_y,
            "ocr_enabled": self.ocr_enabled,
            "use_yolo": self.use_yolo,
            "yolo_model_path": self.yolo_model_path,
            "lama_model_path": self.lama_model_path,
            "pro_fallback_to_fast": self.pro_fallback_to_fast,
            "keep_debug_frames": self.keep_debug_frames,
            "log_level": self.log_level,
            "service_url": self.service_url,
            "python_path": self.python_path,
        }


def load_config() -> WatermarkConfig:
    """
    Load watermark removal configuration from environment variables.
    
    Returns:
        WatermarkConfig instance with values from environment or defaults
    """
    return WatermarkConfig(
        enabled=_env_bool("WATERMARK_ENABLED", True),
        mode=_env_str("WATERMARK_MODE", "fast").lower(),
        sample_count=_env_int("WATERMARK_SAMPLE_COUNT", 10),
        mask_padding=_env_int("WATERMARK_MASK_PADDING", 8),
        confidence_threshold=_env_float("WATERMARK_CONFIDENCE_THRESHOLD", 0.65),
        max_regions=_env_int("WATERMARK_MAX_REGIONS", 3),
        temp_dir=_env_str("WATERMARK_TEMP_DIR", "/tmp/filmora_watermark"),
        inpaint_timeout=_env_int("WATERMARK_INPAINT_TIMEOUT", 300),
        num_workers=_env_int("WATERMARK_NUM_WORKERS", 4),
        ocr_enabled=_env_bool("WATERMARK_USE_OCR", True),
        use_yolo=_env_bool("WATERMARK_USE_YOLO", False),
        yolo_model_path=_env_str("WATERMARK_YOLO_MODEL", ""),
        lama_model_path=_env_str("WATERMARK_LAMA_MODEL", ""),
        pro_fallback_to_fast=_env_bool("WATERMARK_PRO_FALLBACK_TO_FAST", True),
        keep_debug_frames=_env_bool("WATERMARK_KEEP_DEBUG_FRAMES", False),
        log_level=_env_str("WATERMARK_LOG_LEVEL", "INFO"),
        service_url=_env_str("WATERMARK_SERVICE_URL", ""),
        python_path=_env_str("WATERMARK_PYTHON_PATH", "python3"),
    )


def _env_bool(key: str, default: bool) -> bool:
    """Get boolean from environment variable."""
    value = os.environ.get(key, "").lower()
    if value == "":
        return default
    return value in ("true", "1", "yes", "on")


def _env_str(key: str, default: str) -> str:
    """Get string from environment variable."""
    return os.environ.get(key, default)


def _env_int(key: str, default: int) -> int:
    """Get integer from environment variable."""
    try:
        return int(os.environ.get(key, str(default)))
    except (ValueError, TypeError):
        return default


def _env_float(key: str, default: float) -> float:
    """Get float from environment variable."""
    try:
        return float(os.environ.get(key, str(default)))
    except (ValueError, TypeError):
        return default


# Default config instance for convenience
_default_config: Optional[WatermarkConfig] = None


def get_config() -> WatermarkConfig:
    """
    Get the default configuration instance.
    
    Loads from environment on first call, then caches.
    
    Returns:
        WatermarkConfig instance
    """
    global _default_config
    if _default_config is None:
        _default_config = load_config()
    return _default_config
