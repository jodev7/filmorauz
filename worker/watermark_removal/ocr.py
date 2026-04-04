"""
OCR Module for Watermark Detection

Uses OCR to detect text-based watermarks in video frames.
Common text watermarks include site URLs, channel names, and branding.
"""

import os
import logging
from typing import List, Tuple, Optional, Dict, Any
import numpy as np
import cv2

from .config import WatermarkConfig
from .types import RegionCandidate, WatermarkType, DetectionMethod


logger = logging.getLogger(__name__)

# Try to import easyocr
try:
    import easyocr
    OCR_AVAILABLE = True
except ImportError:
    OCR_AVAILABLE = False
    logger.warning("easyocr not available - text watermark detection disabled")


class OCRWatermarkDetector:
    """
    Detects text watermarks using OCR.
    
    Scans edge/corner regions of frames for common watermark text patterns.
    
    Args:
        config: WatermarkConfig instance
    """
    
    # Common watermark text patterns
    WATERMARK_PATTERNS = [
        # Domain patterns
        r'\.(com|net|org|ru|uz|tk|ml|ga|cf|gq)',
        # Common streaming sites
        r'uzmovi', r'freekino', r'kinolar', r'asilmedia',
        r'kino', r'film', r'video', r'tv',
        # Common text markers
        r'©', r'™', r'®',
        # Short common words
        r'^(hd|sd|4k|hd\+|full|free|premium)$',
    ]
    
    def __init__(self, config: WatermarkConfig):
        self.config = config
        self.logger = logging.getLogger(__name__)
        
        # Initialize OCR reader if available and enabled
        self._reader = None
        if config.ocr_enabled and OCR_AVAILABLE:
            try:
                self._reader = easyocr.Reader(
                    ['en', 'ru', 'uz'],  # Languages
                    gpu=False,  # Use CPU by default
                    verbose=False
                )
                self.logger.info("OCR reader initialized successfully")
            except Exception as e:
                self.logger.warning(f"Failed to initialize OCR reader: {e}")
                self._reader = None
    
    @property
    def is_available(self) -> bool:
        """Check if OCR is available."""
        return self._reader is not None
    
    def detect_in_frame(self, frame: np.ndarray, 
                        frame_shape: Tuple[int, int],
                        timestamp: float = 0.0) -> List[RegionCandidate]:
        """
        Detect text watermarks in a single frame.
        
        Args:
            frame: Input frame (BGR format)
            frame_shape: Shape of the frame (H, W, C)
            timestamp: Video timestamp
            
        Returns:
            List of detected watermark regions
        """
        if not self.is_available:
            return []
        
        regions = []
        h, w = frame_shape[:2]
        
        # Define edge regions for OCR scanning
        margin_x = int(w * self.config.edge_margin_x)
        margin_y = int(h * self.config.edge_margin_y)
        
        regions_to_scan = [
            ("top-left", 0, 0, margin_x, margin_y),
            ("top-right", w - margin_x, 0, margin_x, margin_y),
            ("bottom-left", 0, h - margin_y, margin_x, margin_y),
            ("bottom-right", w - margin_x, h - margin_y, margin_x, margin_y),
            ("bottom-center", (w - margin_x) // 2, h - margin_y, margin_x, margin_y),
        ]
        
        for region_name, rx, ry, rw, rh in regions_to_scan:
            # Extract region
            region_frame = frame[ry:ry+rh, rx:rx+rw]
            
            try:
                # Run OCR
                results = self._reader.readtext(region_frame)
                
                for (bbox, text, ocr_confidence) in results:
                    # Filter low confidence OCR results
                    if ocr_confidence < 0.5:
                        continue
                    
                    # Extract bounding box coordinates
                    pts = np.array(bbox).astype(int)
                    x_min, y_min = pts.min(axis=0)
                    x_max, y_max = pts.max(axis=0)
                    
                    text = text.strip().lower()
                    
                    # Check for watermark text patterns
                    if self._is_watermark_text(text):
                        # Adjust coordinates to global frame
                        wx = rx + x_min
                        wy = ry + y_min
                        ww = x_max - x_min
                        wh = y_max - y_min
                        
                        # Calculate confidence
                        confidence = min(ocr_confidence * 0.9, 0.95)
                        
                        region = RegionCandidate(
                            x=wx,
                            y=wy,
                            width=ww,
                            height=wh,
                            confidence=confidence,
                            watermark_type=WatermarkType.CORNER_TEXT,
                            detection_method=DetectionMethod.OCR,
                            location=region_name,
                            text=text,
                            is_static=True,
                            metadata={
                                "ocr_confidence": float(ocr_confidence),
                            }
                        )
                        regions.append(region)
                        self.logger.info(f"OCR detected watermark text: '{text}' at {region_name}")
                
            except Exception as e:
                self.logger.debug(f"OCR failed for region {region_name}: {e}")
        
        return regions
    
    def detect_in_frames(self, frames: List[np.ndarray],
                        timestamps: List[float] = None) -> List[RegionCandidate]:
        """
        Detect text watermarks across multiple frames.
        
        Args:
            frames: List of frames to scan
            timestamps: Optional timestamps for frames
            
        Returns:
            List of detected watermark regions
        """
        if not self.is_available or not frames:
            return []
        
        all_regions = []
        
        # Limit OCR to first few frames for performance
        max_frames_for_ocr = min(5, len(frames))
        
        for i in range(max_frames_for_ocr):
            frame = frames[i]
            ts = timestamps[i] if i < len(timestamps) else 0.0
            
            regions = self.detect_in_frame(frame, frame.shape, ts)
            
            # Deduplicate by text content
            seen_texts = set()
            for region in regions:
                if region.text not in seen_texts:
                    all_regions.append(region)
                    seen_texts.add(region.text)
        
        return all_regions
    
    def _is_watermark_text(self, text: str) -> bool:
        """
        Check if detected text is likely a watermark.
        
        Args:
            text: Detected text
            
        Returns:
            True if text is likely a watermark
        """
        import re
        
        # Check against common watermark patterns
        for pattern in self.WATERMARK_PATTERNS:
            if re.search(pattern, text, re.IGNORECASE):
                return True
        
        # Check text length (watermarks are often short)
        if 3 <= len(text) <= 30:
            return True
        
        return False
    
    def get_text_in_region(self, frame: np.ndarray,
                          bbox: Tuple[int, int, int, int]) -> Optional[str]:
        """
        Get text content within a specific bounding box.
        
        Args:
            frame: Input frame
            bbox: Bounding box (x, y, w, h)
            
        Returns:
            Detected text or None
        """
        if not self.is_available:
            return None
        
        x, y, w, h = bbox
        region = frame[y:y+h, x:x+w]
        
        try:
            results = self._reader.readtext(region)
            if results:
                # Combine all detected text
                texts = [r[1].strip() for r in results]
                return ' '.join(texts)
        except Exception as e:
            self.logger.debug(f"OCR failed for region: {e}")
        
        return None


def create_ocr_detector(config: WatermarkConfig) -> OCRWatermarkDetector:
    """
    Factory function to create OCR detector.
    
    Args:
        config: WatermarkConfig instance
        
    Returns:
        OCRWatermarkDetector instance (may not be usable if OCR not installed)
    """
    return OCRWatermarkDetector(config)
