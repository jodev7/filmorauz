"""
Watermark Detection Module

Provides watermark detection using multiple strategies:
1. Rule-based region search (corners, edges)
2. CV-based persistence analysis
3. OCR-based text detection
4. Optional YOLO-based detection

Detection produces bounding boxes with confidence scores.

Classes:
    CVWatermarkDetector: Computer vision-based watermark detector
    WatermarkDetector: Main detector combining multiple strategies
"""

import os
import logging
import subprocess
from typing import List, Tuple, Optional, Dict, Any
import numpy as np
import cv2

from .config import WatermarkConfig
from .watermark_types import (
    WatermarkType, DetectionMethod, 
    RegionCandidate, WatermarkDetectionResult, WatermarkRegion
)
from .ocr import OCRWatermarkDetector, create_ocr_detector


logger = logging.getLogger(__name__)


class CVWatermarkDetector:
    """
    Detects watermarks using computer vision techniques.
    
    Strategies:
    1. Rule-based corner/edge region analysis
    2. Frame difference for persistent overlay detection
    3. Edge detection and pattern analysis
    
    Args:
        config: WatermarkConfig instance
    """
    
    def __init__(self, config: WatermarkConfig):
        self.config = config
        self.logger = logging.getLogger(__name__)
    
    def detect_in_frame(self, frame: np.ndarray,
                       timestamp: float = 0.0) -> List[RegionCandidate]:
        """
        Detect watermarks in a single frame using CV techniques.
        
        Args:
            frame: Input frame (BGR format)
            timestamp: Video timestamp
            
        Returns:
            List of detected watermark regions
        """
        regions = []
        h, w = frame.shape[:2]
        
        # Define corner regions (percentage of frame dimensions)
        margin_x = int(w * self.config.edge_margin_x)
        margin_y = int(h * self.config.edge_margin_y)
        
        corners = {
            "top-left": (0, 0, margin_x, margin_y),
            "top-right": (w - margin_x, 0, margin_x, margin_y),
            "bottom-left": (0, h - margin_y, margin_x, margin_y),
            "bottom-right": (w - margin_x, h - margin_y, margin_x, margin_y),
        }
        
        for corner_name, (cx, cy, cw, ch) in corners.items():
            corner_regions = []
            
            # Extract corner region
            corner_frame = frame[cy:cy+ch, cx:cx+cw]
            if corner_frame.size == 0:
                continue
                
            gray = cv2.cvtColor(corner_frame, cv2.COLOR_BGR2GRAY)
            
            # Calculate statistics for watermark detection
            edge_density = self._calculate_edge_density(gray)
            has_text_like = self._detect_text_like_pattern(gray)
            has_logo_like = self._detect_logo_like_pattern(gray)
            
            # Calculate confidence
            confidence = self._calculate_confidence(
                edge_density, has_text_like, has_logo_like
            )
            
            if confidence >= self.config.confidence_threshold:
                # Estimate watermark bounds
                try:
                    binary = cv2.threshold(
                        gray, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU
                    )[1]
                    contours, _ = cv2.findContours(
                        binary, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
                    )
                    
                    if contours:
                        # Find bounding box of non-zero content
                        for contour in contours:
                            x_c, y_c, w_c, h_c = cv2.boundingRect(contour)
                            if w_c * h_c > 100:  # Filter noise
                                # Adjust coordinates to global frame
                                wx = cx + x_c
                                wy = cy + y_c
                                ww = w_c
                                wh = h_c
                                
                                region = RegionCandidate(
                                    x=wx,
                                    y=wy,
                                    width=ww,
                                    height=wh,
                                    confidence=confidence,
                                    watermark_type=(
                                        WatermarkType.CORNER_LOGO if has_logo_like 
                                        else WatermarkType.CORNER_TEXT
                                    ),
                                    detection_method=DetectionMethod.RULE_BASED,
                                    location=corner_name,
                                    is_static=True,
                                    metadata={
                                        "edge_density": float(edge_density),
                                        "has_text_like": has_text_like,
                                        "has_logo_like": has_logo_like,
                                        "timestamp": timestamp,
                                    }
                                )
                                corner_regions.append(region)
                except Exception as e:
                    self.logger.debug(f"Contour detection failed: {e}")
            
            # If watermark detected in this corner, add best one
            if corner_regions:
                best_region = max(corner_regions, key=lambda r: r.confidence)
                regions.append(best_region)
        
        return regions
    
    def detect_persistent_overlays(self, frames: List[np.ndarray],
                                   timestamps: List[float] = None) -> List[RegionCandidate]:
        """
        Detect persistent overlays by comparing multiple frames.
        
        A watermark that appears consistently across frames at the same
        location can be detected through frame differencing.
        
        Args:
            frames: List of frames to analyze
            timestamps: Optional timestamps
            
        Returns:
            List of detected persistent watermark regions
        """
        if len(frames) < 2:
            return []
        
        regions = []
        
        # Convert to grayscale
        gray_frames = []
        for f in frames:
            if f is not None and f.size > 0:
                gray_frames.append(cv2.cvtColor(f, cv2.COLOR_BGR2GRAY))
        
        if len(gray_frames) < 2:
            return []
        
        reference_frame = gray_frames[0]
        
        for i, gray in enumerate(gray_frames[1:], 1):
            # Calculate absolute difference
            diff = cv2.absdiff(reference_frame, gray)
            
            # Threshold to find unchanged regions (potential watermarks)
            unchanged_mask = diff < 20  # Low difference = unchanged
            
            # Find connected components in unchanged regions
            unchanged_uint8 = (unchanged_mask * 255).astype(np.uint8)
            
            try:
                contours, _ = cv2.findContours(
                    unchanged_uint8, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
                )
                
                for contour in contours:
                    x, y, w, h = cv2.boundingRect(contour)
                    area = w * h
                    
                    # Filter by size
                    if area < 1000 or area > 100000:
                        continue
                    
                    # Check if this region is consistent across frames
                    consistent_count = 1
                    for j, other_gray in enumerate(gray_frames):
                        if i == j:
                            continue
                        if other_gray is None:
                            continue
                        # Ensure we're within bounds
                        x1 = max(0, x)
                        y1 = max(0, y)
                        x2 = min(gray.shape[1], x + w)
                        y2 = min(gray.shape[0], y + h)
                        if x2 <= x1 or y2 <= y1:
                            continue
                            
                        region_diff = cv2.absdiff(
                            gray[y1:y2, x1:x2],
                            other_gray[y1:y2, x1:x2]
                        )
                        mean_diff = np.mean(region_diff)
                        if mean_diff < 15:
                            consistent_count += 1
                    
                    # If consistent across most frames, it's likely a watermark
                    consistency_ratio = consistent_count / len(gray_frames)
                    if consistency_ratio >= 0.6:
                        confidence = 0.5 + (consistency_ratio * 0.4)
                        
                        region = RegionCandidate(
                            x=x,
                            y=y,
                            width=w,
                            height=h,
                            confidence=confidence,
                            watermark_type=WatermarkType.PERSISTENT_OVERLAY,
                            detection_method=DetectionMethod.PERSISTENCE_ANALYSIS,
                            location=self._describe_location(x, y, w, h, frames[0].shape),
                            is_static=True,
                            metadata={
                                "consistency_ratio": consistency_ratio,
                                "consistent_frames": consistent_count,
                            }
                        )
                        regions.append(region)
            except Exception as e:
                self.logger.debug(f"Persistence analysis failed: {e}")
        
        return regions
    
    def _calculate_edge_density(self, gray: np.ndarray) -> float:
        """Calculate edge density in a region."""
        if gray.size == 0:
            return 0.0
        edges = cv2.Canny(gray, 50, 150)
        return np.sum(edges > 0) / edges.size
    
    def _detect_text_like_pattern(self, gray: np.ndarray) -> bool:
        """Detect if a region has text-like characteristics."""
        if gray.size == 0:
            return False
            
        # Vertical edge density
        sobel_v = cv2.Sobel(gray, cv2.CV_64F, 0, 1, ksize=3)
        v_edge_density = np.sum(np.abs(sobel_v) > 20) / sobel_v.size
        
        # Horizontal edge density
        sobel_h = cv2.Sobel(gray, cv2.CV_64F, 1, 0, ksize=3)
        h_edge_density = np.sum(np.abs(sobel_h) > 20) / sobel_h.size
        
        has_vertical = v_edge_density > 0.03
        has_horizontal = h_edge_density > 0.02
        
        # Check for rectangular structures
        try:
            binary = cv2.threshold(gray, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)[1]
            contours, _ = cv2.findContours(
                binary, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
            )
            
            rectangular_count = 0
            for cnt in contours:
                peri = cv2.arcLength(cnt, True)
                approx = cv2.approxPolyDP(cnt, 0.02 * peri, True)
                if len(approx) == 4:
                    bx, by, bw, bh = cv2.boundingRect(cnt)
                    aspect_ratio = bw / bh if bh > 0 else 0
                    if 0.5 < aspect_ratio < 5 and bw * bh > 50:
                        rectangular_count += 1
            
            return has_vertical and has_horizontal and rectangular_count >= 2
        except:
            return False
    
    def _detect_logo_like_pattern(self, gray: np.ndarray) -> bool:
        """Detect if a region has logo-like characteristics."""
        if gray.size == 0:
            return False
            
        edges = cv2.Canny(gray, 50, 150)
        edge_density = np.sum(edges > 0) / edges.size
        
        try:
            binary = cv2.threshold(gray, 0, 255, cv2.THRESH_BINARY + cv2.THRESH_OTSU)[1]
            contours, _ = cv2.findContours(
                binary, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
            )
            
            rect_count = 0
            for cnt in contours:
                peri = cv2.arcLength(cnt, True)
                approx = cv2.approxPolyDP(cnt, 0.02 * peri, True)
                if len(approx) == 4:
                    bx, by, bw, bh = cv2.boundingRect(cnt)
                    aspect_ratio = bw / bh if bh > 0 else 0
                    if 0.3 < aspect_ratio < 3.5 and bw * bh > 200:
                        rect_count += 1
            
            return edge_density > 0.08 and rect_count >= 1
        except:
            return False
    
    def _calculate_confidence(self, edge_density: float,
                              has_text_like: bool,
                              has_logo_like: bool) -> float:
        """Calculate watermark confidence score."""
        confidence = 0.0
        
        if edge_density > 0.05:
            confidence += 0.3
        if has_text_like:
            confidence += 0.35
        if has_logo_like:
            confidence += 0.35
        
        return confidence
    
    def _describe_location(self, x: int, y: int, w: int, h: int,
                          frame_shape: Tuple[int, int]) -> str:
        """Describe the location of a region."""
        fh, fw = frame_shape[:2]
        cx, cy = x + w // 2, y + h // 2
        
        if cx < fw * 0.33:
            h_pos = "left"
        elif cx > fw * 0.67:
            h_pos = "right"
        else:
            h_pos = "center"
        
        if cy < fh * 0.33:
            v_pos = "top"
        elif cy > fh * 0.67:
            v_pos = "bottom"
        else:
            v_pos = "middle"
        
        return f"{v_pos}-{h_pos}"


class WatermarkDetector:
    """
    Main watermark detector combining multiple detection strategies.
    
    Detection strategies:
    1. Rule-based corner/edge region analysis (CV)
    2. Frame difference for persistent overlay detection
    3. OCR-based text detection
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Directory for temporary files
    """
    
    def __init__(self, config: WatermarkConfig, temp_dir: str = "./tmp"):
        self.config = config
        self.temp_dir = temp_dir
        self.logger = logging.getLogger(__name__)
        
        # Initialize sub-detectors
        self._cv_detector = CVWatermarkDetector(config)
        self._ocr_detector = create_ocr_detector(config)
    
    def detect(self, video_path: str,
               frames: List[np.ndarray] = None,
               timestamps: List[float] = None) -> WatermarkDetectionResult:
        """
        Detect watermarks in a video.
        
        Args:
            video_path: Path to video file
            frames: Optional pre-extracted frames
            timestamps: Optional timestamps for frames
            
        Returns:
            WatermarkDetectionResult with detected regions
        """
        import time
        start_time = time.time()
        
        self.logger.info(f"Starting watermark detection for: {video_path}")
        
        # Get video info
        video_info = self._get_video_info(video_path)
        if not video_info:
            return WatermarkDetectionResult(
                error="Failed to get video information"
            )
        
        duration = video_info.get("duration", 0)
        resolution = video_info.get("resolution", (0, 0))
        
        # Extract frames if not provided
        if frames is None:
            from .sampler import FrameSampler
            sampler = FrameSampler(self.config, self.temp_dir)
            frames, timestamps = sampler.sample_frames(video_path)
        
        total_frames = len(frames)
        if total_frames == 0:
            return WatermarkDetectionResult(
                error="No frames extracted for analysis",
                video_duration=duration,
                video_resolution=resolution
            )
        
        self.logger.info(f"Analyzing {total_frames} frames")
        
        # Detect using multiple strategies
        all_regions = []
        
        # Strategy 1: CV-based corner detection
        corner_count = 0
        for i, frame in enumerate(frames):
            if frame is None or frame.size == 0:
                continue
            ts = timestamps[i] if i < len(timestamps) else 0.0
            corner_regions = self._cv_detector.detect_in_frame(frame, ts)
            all_regions.extend(corner_regions)
            corner_count += len(corner_regions)
        
        self.logger.info(f"CV detection found {corner_count} regions")
        
        # Strategy 2: Persistence analysis (if multiple frames)
        if len(frames) >= 3:
            persistent_regions = self._cv_detector.detect_persistent_overlays(frames, timestamps)
            all_regions.extend(persistent_regions)
            self.logger.info(f"Persistence analysis found {len(persistent_regions)} regions")
        
        # Strategy 3: OCR-based detection
        if self._ocr_detector is not None and self._ocr_detector.is_available:
            ocr_regions = self._ocr_detector.detect_in_frames(frames, timestamps)
            all_regions.extend(ocr_regions)
            self.logger.info(f"OCR detection found {len(ocr_regions)} regions")
        
        # Filter and merge overlapping regions
        filtered_regions = self._filter_regions(all_regions)
        
        # Limit to max regions
        if len(filtered_regions) > self.config.max_regions:
            filtered_regions = sorted(
                filtered_regions, 
                key=lambda r: r.confidence, 
                reverse=True
            )[:self.config.max_regions]
        
        self.logger.info(f"Final detection: {len(filtered_regions)} watermark regions")
        
        return WatermarkDetectionResult(
            detected=len(filtered_regions) > 0,
            regions=filtered_regions,
            total_frames_analyzed=total_frames,
            video_duration=duration,
            video_resolution=resolution,
            processing_time=time.time() - start_time
        )
    
    def _get_video_info(self, video_path: str) -> Optional[dict]:
        """Get video information using ffprobe."""
        try:
            cmd = [
                "ffprobe", "-v", "error",
                "-show_entries", "stream=width,height,duration",
                "-show_entries", "format=duration",
                "-of", "json",
                video_path
            ]
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            
            if result.returncode != 0:
                return None
            
            import json
            info = json.loads(result.stdout)
            
            streams = info.get("streams", [{}])
            video_stream = next((s for s in streams if s.get("codec_type") == "video"), {})
            
            width = int(video_stream.get("width", 0))
            height = int(video_stream.get("height", 0))
            duration = float(info.get("format", {}).get("duration", 0))
            
            return {
                "duration": duration,
                "resolution": (width, height)
            }
        except Exception as e:
            self.logger.error(f"Failed to get video info: {e}")
            return None
    
    def _filter_regions(self, regions: List[RegionCandidate]) -> List[RegionCandidate]:
        """Filter and merge overlapping regions."""
        if not regions:
            return []
        
        # Sort by confidence (highest first)
        sorted_regions = sorted(regions, key=lambda r: r.confidence, reverse=True)
        
        filtered = []
        for region in sorted_regions:
            should_keep = True
            for kept in filtered:
                if self._regions_overlap(region, kept, iou_threshold=0.3):
                    if region.confidence > kept.confidence + 0.1:
                        filtered.remove(kept)
                    else:
                        should_keep = False
                    break
            
            if should_keep:
                filtered.append(region)
        
        # Apply confidence threshold
        filtered = [r for r in filtered if r.confidence >= self.config.confidence_threshold]
        
        return filtered
    
    def _regions_overlap(self, r1: RegionCandidate, r2: RegionCandidate,
                        iou_threshold: float = 0.5) -> bool:
        """Check if two regions overlap significantly."""
        # Calculate intersection
        x1 = max(r1.x, r2.x)
        y1 = max(r1.y, r2.y)
        x2 = min(r1.x + r1.width, r2.x + r2.width)
        y2 = min(r1.y + r1.height, r2.y + r2.height)
        
        if x2 <= x1 or y2 <= y1:
            return False
        
        # Calculate union
        intersection = (x2 - x1) * (y2 - y1)
        union = (r1.width * r1.height) + (r2.width * r2.height) - intersection
        
        if union <= 0:
            return False
        
        # Calculate IoU
        iou = intersection / union
        return iou > iou_threshold
