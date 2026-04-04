"""
Watermark Removal Service for FilmoraUz Worker

A modular AI-based watermark detection and removal pipeline for video processing.

Modules:
- config: Configuration management
- types: Type definitions and result objects
- sampler: Frame extraction and sampling
- detector: Watermark detection (CV + OCR)
- ocr: OCR-based text watermark detection
- masks: Mask generation
- inpaint: Inpainting (FAST/PRO modes)
- pipeline: Main pipeline orchestrator
- service: HTTP service for Go integration
- video_processor: Video frame extraction and reconstruction

Usage:
    from watermark_removal import WatermarkRemovalPipeline, load_config
    
    config = load_config()
    pipeline = WatermarkRemovalPipeline(config=config)
    result = pipeline.process("/path/to/video.mp4")
"""

from .config import WatermarkConfig, load_config, get_config
from .watermark_types import (
    WatermarkType,
    DetectionMethod,
    InpaintingMode,
    PipelineStage,
    RegionCandidate,
    WatermarkRegion,  # Alias for Go compatibility
    MaskResult,
    WatermarkDetectionResult,
    WatermarkRemovalResult,
    InpaintResult,
)
from .pipeline import WatermarkRemovalPipeline, remove_watermark, detect_watermark
from .sampler import FrameSampler, VideoInfo
from .detector import WatermarkDetector, CVWatermarkDetector
from .ocr import OCRWatermarkDetector, create_ocr_detector
from .masks import MaskGenerator
from .inpaint import Inpainter, create_inpainter, OpenCVInpainter, LaMaInpainter

__version__ = "1.0.0"
__all__ = [
    # Config
    "WatermarkConfig",
    "load_config",
    "get_config",
    # Types
    "WatermarkType",
    "DetectionMethod",
    "InpaintingMode",
    "PipelineStage",
    "RegionCandidate",
    "WatermarkRegion",  # Alias for Go compatibility
    "MaskResult",
    "WatermarkDetectionResult",
    "WatermarkRemovalResult",
    "InpaintResult",
    # Main pipeline
    "WatermarkRemovalPipeline",
    "remove_watermark",
    "detect_watermark",
    # Components
    "FrameSampler",
    "VideoInfo",
    "WatermarkDetector",
    "CVWatermarkDetector",
    "OCRWatermarkDetector",
    "create_ocr_detector",
    "MaskGenerator",
    "Inpainter",
    "create_inpainter",
    "OpenCVInpainter",
    "LaMaInpainter",
]
