"""
Type Definitions for Watermark Removal Pipeline

Provides typed result objects and schemas for the watermark removal service.

Classes:
    WatermarkType: Enum for watermark types
    DetectionMethod: Enum for detection methods
    InpaintingMode: Enum for inpainting modes
    PipelineStage: Enum for pipeline stages
    RegionCandidate: Dataclass for detected watermark region
    MaskResult: Dataclass for mask generation result
    WatermarkDetectionResult: Dataclass for detection result
    WatermarkRemovalResult: Dataclass for pipeline result
    InpaintResult: Dataclass for inpainting result
    WatermarkRegion: Alias for RegionCandidate (for Go compatibility)
"""

from dataclasses import dataclass, field
from typing import List, Optional, Dict, Any
from enum import Enum


class WatermarkType(Enum):
    """Types of watermarks that can be detected."""
    CORNER_LOGO = "corner_logo"
    CORNER_TEXT = "corner_text"
    PERSISTENT_OVERLAY = "persistent_overlay"
    SEMI_TRANSPARENT = "semi_transparent"
    DYNAMIC = "dynamic"
    UNKNOWN = "unknown"


class DetectionMethod(Enum):
    """Detection methods used."""
    RULE_BASED = "rule_based"
    PERSISTENCE_ANALYSIS = "persistence_analysis"
    OCR = "ocr"
    YOLO = "yolo"
    COMBINED = "combined"


class InpaintingMode(Enum):
    """Inpainting modes."""
    FAST = "fast"
    PRO = "pro"


class PipelineStage(Enum):
    """Pipeline processing stages."""
    RECEIVED = "received_local_video"
    SAMPLING = "watermark_sampling_frames"
    DETECTING = "watermark_detecting"
    MASKS_CREATED = "watermark_masks_created"
    INPAINTING_FAST = "watermark_inpainting_fast"
    INPAINTING_PRO = "watermark_inpainting_pro"
    REBUILDING = "watermark_rebuilding_video"
    COMPLETE = "watermark_removal_complete"
    NOT_DETECTED = "watermark_not_detected"
    FALLBACK = "watermark_removal_fallback"
    CONTINUING = "continuing_ffmpeg_pipeline"


@dataclass
class RegionCandidate:
    """
    Represents a detected watermark region candidate.
    
    Attributes:
        x, y: Top-left corner coordinates
        width, height: Region dimensions
        confidence: Detection confidence (0.0-1.0)
        watermark_type: Type of watermark detected
        detection_method: Method used for detection
        location: String description (e.g., "top-left", "bottom-right")
        text: Detected text (for text watermarks)
        is_static: Whether the watermark is static (vs dynamic)
    """
    x: int = 0
    y: int = 0
    width: int = 0
    height: int = 0
    confidence: float = 0.0
    watermark_type: WatermarkType = WatermarkType.UNKNOWN
    detection_method: DetectionMethod = DetectionMethod.RULE_BASED
    location: str = ""
    text: str = ""
    is_static: bool = True
    metadata: Dict[str, Any] = field(default_factory=dict)
    
    @property
    def bbox(self) -> tuple:
        """Get bounding box as (x, y, w, h)."""
        return (self.x, self.y, self.width, self.height)
    
    @property
    def center(self) -> tuple:
        """Get center point of region."""
        return (self.x + self.width // 2, self.y + self.height // 2)
    
    @property
    def area(self) -> int:
        """Calculate area of the region."""
        return self.width * self.height
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "x": self.x,
            "y": self.y,
            "width": self.width,
            "height": self.height,
            "confidence": float(self.confidence),
            "watermark_type": self.watermark_type.value if isinstance(self.watermark_type, WatermarkType) else self.watermark_type,
            "detection_method": self.detection_method.value if isinstance(self.detection_method, DetectionMethod) else self.detection_method,
            "location": self.location,
            "text": self.text,
            "is_static": self.is_static,
            "metadata": self.metadata,
        }
    
    @classmethod
    def from_dict(cls, data: dict) -> 'RegionCandidate':
        """Create from dictionary."""
        wt = data.get("watermark_type", "unknown")
        if isinstance(wt, str):
            try:
                wt = WatermarkType(wt)
            except ValueError:
                wt = WatermarkType.UNKNOWN
        
        dm = data.get("detection_method", "rule_based")
        if isinstance(dm, str):
            try:
                dm = DetectionMethod(dm)
            except ValueError:
                dm = DetectionMethod.RULE_BASED
        
        return cls(
            x=data.get("x", 0),
            y=data.get("y", 0),
            width=data.get("width", 0),
            height=data.get("height", 0),
            confidence=data.get("confidence", 0.0),
            watermark_type=wt,
            detection_method=dm,
            location=data.get("location", ""),
            text=data.get("text", ""),
            is_static=data.get("is_static", True),
            metadata=data.get("metadata", {}),
        )


# Alias for Go compatibility
WatermarkRegion = RegionCandidate


@dataclass
class WatermarkDetectionResult:
    """
    Result of watermark detection on a video.
    
    Attributes:
        detected: Whether watermarks were detected
        regions: List of detected watermark regions
        total_frames_analyzed: Number of frames analyzed
        video_duration: Video duration in seconds
        video_resolution: (width, height) tuple
        processing_time: Time taken for detection in seconds
    """
    detected: bool = False
    regions: List[RegionCandidate] = field(default_factory=list)
    total_frames_analyzed: int = 0
    video_duration: float = 0.0
    video_resolution: tuple = (0, 0)
    processing_time: float = 0.0
    error: str = ""
    
    @property
    def primary_region(self) -> Optional[RegionCandidate]:
        """Get the highest confidence region."""
        if not self.regions:
            return None
        return max(self.regions, key=lambda r: r.confidence)
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "detected": self.detected,
            "regions": [r.to_dict() for r in self.regions],
            "total_frames_analyzed": self.total_frames_analyzed,
            "video_duration": float(self.video_duration),
            "video_resolution": self.video_resolution,
            "processing_time": self.processing_time,
            "error": self.error,
        }


@dataclass
class WatermarkRemovalResult:
    """
    Result of watermark removal pipeline.
    
    Attributes:
        success: Whether pipeline completed successfully
        input_path: Original video path
        output_path: Clean video path (or original if no watermark)
        watermark_detected: Whether watermark was detected
        watermark_removed: Whether watermark was successfully removed
        mode_used: Processing mode used ("fast" or "pro")
        regions: Detected watermark regions
        fallback_used: Whether fallback was used (original video)
        warning: Warning message if any
        stages: List of completed stages
        total_time: Total processing time in seconds
        error: Error message if failed
    """
    success: bool = False
    input_path: str = ""
    output_path: str = ""
    watermark_detected: bool = False
    watermark_removed: bool = False
    mode_used: Optional[str] = None
    regions: List[RegionCandidate] = field(default_factory=list)
    fallback_used: bool = False
    warning: Optional[str] = None
    stages: List[str] = field(default_factory=list)
    total_time: float = 0.0
    error: str = ""
    
    @property
    def has_clean_output(self) -> bool:
        """Check if clean output video is available."""
        return self.success and self.output_path and self.output_path != self.input_path
    
    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization."""
        return {
            "success": self.success,
            "input_path": self.input_path,
            "output_path": self.output_path,
            "watermark_detected": self.watermark_detected,
            "watermark_removed": self.watermark_removed,
            "mode_used": self.mode_used,
            "regions": [r.to_dict() for r in self.regions],
            "fallback_used": self.fallback_used,
            "warning": self.warning,
            "stages": self.stages,
            "total_time": self.total_time,
            "error": self.error,
        }


@dataclass
class MaskResult:
    """
    Result of mask generation.
    
    Attributes:
        x, y: Top-left corner coordinates
        width, height: Mask dimensions
        padding: Expansion padding applied
        coverage: Percentage of mask that is non-zero
        source_region: Original region this mask was generated from
    """
    x: int = 0
    y: int = 0
    width: int = 0
    height: int = 0
    padding: int = 0
    coverage: float = 0.0
    source_region: Optional[RegionCandidate] = None
    
    @property
    def bbox(self) -> tuple:
        """Get bounding box as (x, y, w, h)."""
        return (self.x, self.y, self.width, self.height)
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "x": self.x,
            "y": self.y,
            "width": self.width,
            "height": self.height,
            "padding": self.padding,
            "coverage": self.coverage,
            "source_region": self.source_region.to_dict() if self.source_region else None,
        }


@dataclass
class InpaintResult:
    """
    Result of inpainting operation.
    
    Attributes:
        success: Whether inpainting completed successfully
        frames_processed: Number of frames processed
        processing_time: Total processing time in seconds
        mode: Inpainting mode used
        error: Error message if failed
    """
    success: bool = False
    frames_processed: int = 0
    processing_time: float = 0.0
    mode: InpaintingMode = InpaintingMode.FAST
    error: str = ""
    
    def to_dict(self) -> dict:
        """Convert to dictionary."""
        return {
            "success": self.success,
            "frames_processed": self.frames_processed,
            "processing_time": self.processing_time,
            "mode": self.mode.value if isinstance(self.mode, InpaintingMode) else self.mode,
            "error": self.error,
        }
