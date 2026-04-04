"""
Mask Generation Module

Generates binary masks for detected watermark regions.
Masks are expanded slightly to cover soft edges, glow, and shadows.

Features:
- Binary mask generation from bounding boxes
- Soft edge expansion using morphological operations
- Multiple mask support for multiple watermarks
- Adaptive mask refinement based on watermark characteristics
"""

import logging
from dataclasses import dataclass, field
from typing import List, Tuple, Optional
import numpy as np
import cv2

from .config import WatermarkConfig
from .detector import WatermarkRegion, WatermarkType


@dataclass
class Mask:
    """
    Represents a mask for watermark removal.
    
    Attributes:
        x, y: Top-left corner coordinates
        width, height: Mask dimensions
        mask_array: Binary mask data
        padding: Expansion padding applied
        source_region: Original WatermarkRegion this mask was generated from
    """
    x: int
    y: int
    width: int
    height: int
    mask_array: np.ndarray
    padding: int = 0
    source_region: Optional[WatermarkRegion] = None
    
    @property
    def bbox(self) -> Tuple[int, int, int, int]:
        """Get bounding box as (x, y, w, h)."""
        return (self.x, self.y, self.width, self.height)
    
    @property
    def mask_path(self) -> Optional[str]:
        """Path where mask was saved (if applicable)."""
        return getattr(self, '_path', None)
    
    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "x": self.x,
            "y": self.y,
            "width": self.width,
            "height": self.height,
            "padding": self.padding,
            "source_region": self.source_region.to_dict() if self.source_region else None,
        }


class MaskGenerator:
    """
    Generates masks for watermark removal.
    
    Features:
    - Bounding box to binary mask conversion
    - Soft edge expansion for better inpainting
    - Multiple mask support
    - Mask refinement based on watermark characteristics
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Directory for temporary files
    """
    
    def __init__(self, config: WatermarkConfig, temp_dir: str = "./tmp"):
        self.config = config
        self.temp_dir = temp_dir
        self.logger = logging.getLogger(__name__)
        
        # Create temp directory if needed
        import os
        os.makedirs(temp_dir, exist_ok=True)
    
    def generate_masks(self, regions: List[WatermarkRegion], 
                       frame_shape: Tuple[int, int],
                       frames: List[np.ndarray] = None) -> List[Mask]:
        """
        Generate masks for detected watermark regions.
        
        Args:
            regions: List of detected WatermarkRegion
            frame_shape: Shape of the video frame (H, W, C)
            frames: Optional frames for adaptive mask refinement
            
        Returns:
            List of Mask objects
        """
        if not regions:
            self.logger.info("No regions provided for mask generation")
            return []
        
        self.logger.info(f"Generating masks for {len(regions)} regions")
        
        masks = []
        for region in regions:
            mask = self._generate_single_mask(region, frame_shape, frames)
            if mask is not None:
                masks.append(mask)
                self.logger.info(
                    f"Generated mask: bbox={mask.bbox}, padding={mask.padding}, "
                    f"coverage={np.sum(mask.mask_array > 0) / mask.mask_array.size:.2%}"
                )
        
        self.logger.info(f"Generated {len(masks)} masks")
        return masks
    
    def _generate_single_mask(self, region: WatermarkRegion,
                               frame_shape: Tuple[int, int],
                               frames: List[np.ndarray] = None) -> Optional[Mask]:
        """
        Generate a mask for a single watermark region.
        
        Applies padding and optional refinement based on watermark characteristics.
        """
        h, w = frame_shape[:2]
        padding = self.config.mask_padding
        
        # Calculate expanded bounding box
        x = max(0, region.x - padding)
        y = max(0, region.y - padding)
        ew = min(w - x, region.width + 2 * padding)
        eh = min(h - y, region.height + 2 * padding)
        
        # Create base mask (all zeros)
        base_mask = np.zeros((h, w), dtype=np.uint8)
        
        # Fill the region with white (255)
        base_mask[y:y+eh, x:x+ew] = 255
        
        # Refine mask based on watermark type and frame analysis
        if frames is not None and len(frames) > 0:
            base_mask = self._refine_mask(base_mask, region, frames)
        
        # Apply morphological operations for soft edges
        refined_mask = self._apply_soft_edges(base_mask, region)
        
        return Mask(
            x=x,
            y=y,
            width=ew,
            height=eh,
            mask_array=refined_mask,
            padding=padding,
            source_region=region
        )
    
    def _refine_mask(self, mask: np.ndarray, region: WatermarkRegion,
                      frames: List[np.ndarray]) -> np.ndarray:
        """
        Refine mask based on actual watermark content in frames.
        
        Uses edge detection and intensity analysis to better define
        the actual watermark boundaries vs. padding.
        """
        # Use the first frame for analysis (frames should be consistent for static watermarks)
        frame = frames[0]
        
        # Extract region with some margin
        x, y = region.x, region.y
        w, h = region.width, region.height
        margin = self.config.mask_padding
        
        x1 = max(0, x - margin)
        y1 = max(0, y - margin)
        x2 = min(frame.shape[1], x + w + margin)
        y2 = min(frame.shape[0], y + h + margin)
        
        # Extract the region
        region_frame = frame[y1:y2, x1:x2]
        gray = cv2.cvtColor(region_frame, cv2.COLOR_BGR2GRAY)
        
        # Calculate adaptive threshold to find actual watermark edges
        adaptive_thresh = cv2.adaptiveThreshold(
            gray, 255, cv2.ADAPTIVE_THRESH_GAUSSIAN_C, 
            cv2.THRESH_BINARY, 11, 2
        )
        
        # Find contours in the region
        contours, _ = cv2.findContours(
            adaptive_thresh, cv2.RETR_EXTERNAL, cv2.CHAIN_APPROX_SIMPLE
        )
        
        # Create refined mask
        refined = np.zeros_like(mask)
        
        for contour in contours:
            area = cv2.contourArea(contour)
            if area < 50:  # Filter noise
                continue
            
            # Get approximate polygon
            peri = cv2.arcLength(contour, True)
            approx = cv2.approxPolyDP(contour, 0.02 * peri, True)
            
            # Create filled polygon
            cv2.fillPoly(refined, [approx + np.array([[x1, y1]])], 255)
        
        # If no contours found, use original mask
        if np.sum(refined > 0) < 100:
            return mask
        
        # Dilate slightly to ensure coverage
        kernel = np.ones((3, 3), np.uint8)
        refined = cv2.dilate(refined, kernel, iterations=1)
        
        return refined
    
    def _apply_soft_edges(self, mask: np.ndarray, region: WatermarkRegion) -> np.ndarray:
        """
        Apply soft edges to mask for better inpainting results.
        
        Uses morphological operations to create smooth transitions
        at mask boundaries.
        """
        # For text-like watermarks, use lighter smoothing
        if region.watermark_type == WatermarkType.CORNER_TEXT:
            # Light morphological operations
            kernel = np.ones((3, 3), np.uint8)
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
            mask = cv2.morphologyEx(mask, cv2.MORPH_OPEN, kernel)
        
        elif region.watermark_type == WatermarkType.CORNER_LOGO:
            # Logo watermarks may need more aggressive smoothing
            # but keep edges relatively sharp
            kernel = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (5, 5))
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
        
        else:
            # Default: moderate smoothing
            kernel = np.ones((3, 3), np.uint8)
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
        
        return mask
    
    def save_mask(self, mask: Mask, path: str) -> bool:
        """
        Save a mask to file.
        
        Args:
            mask: Mask to save
            path: Output file path
            
        Returns:
            True if successful, False otherwise
        """
        try:
            # Ensure mask is uint8
            mask_array = mask.mask_array.astype(np.uint8)
            
            # Save as PNG (lossless)
            cv2.imwrite(path, mask_array)
            
            # Store path in mask object
            mask._path = path
            
            self.logger.info(f"Saved mask to: {path}")
            return True
        except Exception as e:
            self.logger.error(f"Failed to save mask: {e}")
            return False
    
    def load_mask(self, path: str) -> Optional[Mask]:
        """
        Load a mask from file.
        
        Args:
            path: Path to mask file
            
        Returns:
            Mask object or None if failed
        """
        try:
            mask_array = cv2.imread(path, cv2.IMREAD_GRAYSCALE)
            if mask_array is None:
                return None
            
            h, w = mask_array.shape[:2]
            
            # Find bounding box of non-zero pixels
            rows = np.any(mask_array > 0, axis=1)
            cols = np.any(mask_array > 0, axis=0)
            if not rows.any() or not cols.any():
                return None
            
            y1, y2 = np.where(rows)[0][[0, -1]]
            x1, x2 = np.where(cols)[0][[0, -1]]
            
            return Mask(
                x=int(x1),
                y=int(y1),
                width=int(x2 - x1 + 1),
                height=int(y2 - y1 + 1),
                mask_array=mask_array,
                padding=0
            )
        except Exception as e:
            self.logger.error(f"Failed to load mask: {e}")
            return None
    
    def merge_masks(self, masks: List[Mask], frame_shape: Tuple[int, int]) -> Mask:
        """
        Merge multiple masks into a single mask.
        
        Args:
            masks: List of masks to merge
            frame_shape: Shape of the video frame
            
        Returns:
            Merged Mask object
        """
        if not masks:
            h, w = frame_shape[:2]
            return Mask(
                x=0, y=0, width=w, height=h,
                mask_array=np.zeros((h, w), dtype=np.uint8)
            )
        
        if len(masks) == 1:
            return masks[0]
        
        # Create combined mask
        h, w = frame_shape[:2]
        combined = np.zeros((h, w), dtype=np.uint8)
        
        for mask in masks:
            combined = cv2.bitwise_or(combined, mask.mask_array)
        
        # Find bounding box
        rows = np.any(combined > 0, axis=1)
        cols = np.any(combined > 0, axis=0)
        
        if rows.any() and cols.any():
            y1, y2 = np.where(rows)[0][[0, -1]]
            x1, x2 = np.where(cols)[0][[0, -1]]
            x, y, width, height = x1, y1, x2 - x1 + 1, y2 - y1 + 1
        else:
            x, y, width, height = 0, 0, w, h
        
        return Mask(
            x=x, y=y, width=width, height=height,
            mask_array=combined,
            padding=0
        )
    
    def create_frame_mask(self, masks: List[Mask], frame_shape: Tuple[int, int]) -> np.ndarray:
        """
        Create a combined mask for an entire frame.
        
        Args:
            masks: List of masks
            frame_shape: Shape of the frame
            
        Returns:
            Combined mask array
        """
        h, w = frame_shape[:2]
        frame_mask = np.zeros((h, w), dtype=np.uint8)
        
        for mask in masks:
            frame_mask = cv2.bitwise_or(frame_mask, mask.mask_array)
        
        return frame_mask
