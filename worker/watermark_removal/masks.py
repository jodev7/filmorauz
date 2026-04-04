"""
Mask Generation Module

Generates binary masks for detected watermark regions.
Masks are expanded slightly to cover soft edges, glow, and shadows.

Classes:
    MaskResult: Dataclass for mask generation result
    MaskGenerator: Generates masks for watermark removal
"""

import os
import logging
from typing import List, Tuple, Optional
import numpy as np
import cv2

from .config import WatermarkConfig
from .watermark_types import RegionCandidate, MaskResult


logger = logging.getLogger(__name__)


class MaskGenerator:
    """
    Generates masks for watermark removal.
    
    Features:
    - Binary mask generation from bounding boxes
    - Soft edge expansion using morphological operations
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
        os.makedirs(temp_dir, exist_ok=True)
    
    def generate_mask(self, region: RegionCandidate,
                     frame_shape: Tuple[int, int],
                     frame: np.ndarray = None) -> MaskResult:
        """
        Generate a mask for a single watermark region.
        
        Args:
            region: Watermark region
            frame_shape: Shape of the video frame (H, W, C)
            frame: Optional frame for adaptive refinement
            
        Returns:
            MaskResult with mask information
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
        
        # Refine mask if frame provided
        if frame is not None and frame.size > 0:
            base_mask = self._refine_mask(base_mask, region, frame)
        
        # Apply morphological operations for soft edges
        refined_mask = self._apply_soft_edges(base_mask, region)
        
        # Calculate coverage
        coverage = np.sum(refined_mask > 0) / refined_mask.size
        
        return MaskResult(
            x=x,
            y=y,
            width=ew,
            height=eh,
            padding=padding,
            coverage=coverage,
            source_region=region
        )
    
    def generate_masks(self, regions: List[RegionCandidate],
                       frame_shape: Tuple[int, int],
                       frames: List[np.ndarray] = None) -> List[MaskResult]:
        """
        Generate masks for multiple watermark regions.
        
        Args:
            regions: List of detected watermark regions
            frame_shape: Shape of the video frame
            frames: Optional frames for adaptive mask refinement
            
        Returns:
            List of MaskResult objects
        """
        if not regions:
            self.logger.info("No regions provided for mask generation")
            return []
        
        self.logger.info(f"Generating masks for {len(regions)} regions")
        
        # Use first frame for refinement if available
        frame = frames[0] if frames and len(frames) > 0 and frames[0] is not None else None
        
        masks = []
        for region in regions:
            mask = self.generate_mask(region, frame_shape, frame)
            masks.append(mask)
            
            self.logger.info(
                f"Generated mask: bbox={mask.bbox}, padding={mask.padding}, "
                f"coverage={mask.coverage:.2%}"
            )
        
        self.logger.info(f"Generated {len(masks)} masks")
        return masks
    
    def _refine_mask(self, mask: np.ndarray, region: RegionCandidate,
                     frame: np.ndarray) -> np.ndarray:
        """
        Refine mask based on actual watermark content in frame.
        
        Uses edge detection and intensity analysis to better define
        the actual watermark boundaries.
        """
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
        if region_frame.size == 0:
            return mask
            
        gray = cv2.cvtColor(region_frame, cv2.COLOR_BGR2GRAY)
        
        # Calculate adaptive threshold
        try:
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
                if area < 50:
                    continue
                
                peri = cv2.arcLength(contour, True)
                approx = cv2.approxPolyDP(contour, 0.02 * peri, True)
                
                cv2.fillPoly(refined, [approx + np.array([[x1, y1]])], 255)
            
            # If no contours found, use original mask
            if np.sum(refined > 0) < 100:
                return mask
            
            # Dilate slightly
            kernel = np.ones((3, 3), np.uint8)
            refined = cv2.dilate(refined, kernel, iterations=1)
            
            return refined
        except Exception as e:
            self.logger.debug(f"Mask refinement failed: {e}")
            return mask
    
    def _apply_soft_edges(self, mask: np.ndarray, region: RegionCandidate) -> np.ndarray:
        """
        Apply soft edges to mask for better inpainting results.
        
        Uses morphological operations to create smooth transitions.
        """
        from .watermark_types import WatermarkType
        
        wt = region.watermark_type
        if isinstance(wt, str):
            try:
                wt = WatermarkType(wt)
            except ValueError:
                wt = WatermarkType.UNKNOWN
        
        if wt == WatermarkType.CORNER_TEXT:
            kernel = np.ones((3, 3), np.uint8)
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
            mask = cv2.morphologyEx(mask, cv2.MORPH_OPEN, kernel)
        elif wt == WatermarkType.CORNER_LOGO:
            kernel = cv2.getStructuringElement(cv2.MORPH_ELLIPSE, (5, 5))
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
        else:
            kernel = np.ones((3, 3), np.uint8)
            mask = cv2.morphologyEx(mask, cv2.MORPH_CLOSE, kernel)
        
        return mask
    
    def merge_masks(self, masks: List[MaskResult],
                   frame_shape: Tuple[int, int]) -> np.ndarray:
        """
        Merge multiple masks into a single combined mask.
        
        Args:
            masks: List of masks to merge
            frame_shape: Shape of the video frame
            
        Returns:
            Combined mask array
        """
        h, w = frame_shape[:2]
        combined = np.zeros((h, w), dtype=np.uint8)
        
        for mask in masks:
            # Create mask array from MaskResult
            mask_array = np.zeros((h, w), dtype=np.uint8)
            mask_array[mask.y:mask.y+mask.height, mask.x:mask.x+mask.width] = 255
            combined = cv2.bitwise_or(combined, mask_array)
        
        return combined
    
    def save_mask(self, mask: np.ndarray, path: str) -> bool:
        """
        Save a mask to file.
        
        Args:
            mask: Mask array
            path: Output file path
            
        Returns:
            True if successful
        """
        try:
            cv2.imwrite(path, mask)
            self.logger.info(f"Saved mask to: {path}")
            return True
        except Exception as e:
            self.logger.error(f"Failed to save mask: {e}")
            return False
    
    def load_mask(self, path: str) -> Optional[np.ndarray]:
        """
        Load a mask from file.
        
        Args:
            path: Path to mask file
            
        Returns:
            Mask array or None
        """
        try:
            mask = cv2.imread(path, cv2.IMREAD_GRAYSCALE)
            return mask
        except Exception as e:
            self.logger.error(f"Failed to load mask: {e}")
            return None
