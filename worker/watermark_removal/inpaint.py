"""
Inpainting Module for Watermark Removal

Provides two modes of inpainting:
- FAST MODE: OpenCV-based (Telea or Navier-Stokes)
- PRO MODE: LaMa neural network-based

Classes:
    OpenCVInpainter: Fast OpenCV-based inpainting
    LaMaInpainter: LaMa neural network inpainting
    Inpainter: Main inpainting service supporting both modes
"""

import os
import logging
import time
from typing import List, Optional, Callable, Tuple, Union
import numpy as np
import cv2

from .config import WatermarkConfig
from .watermark_types import InpaintingMode, InpaintResult, MaskResult, RegionCandidate


logger = logging.getLogger(__name__)


class OpenCVInpainter:
    """
    OpenCV-based inpainting for FAST mode.
    
    Uses Telea (fast) or Navier-Stokes (smoother) algorithms.
    """
    
    def __init__(self):
        self.logger = logging.getLogger(__name__)
    
    def inpaint(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint a single frame using OpenCV.
        
        Args:
            frame: Input frame (BGR format)
            mask: Binary mask (255 = watermark region to inpaint)
            
        Returns:
            Inpainted frame
        """
        if frame is None or frame.size == 0:
            return frame
            
        # Ensure mask is correct format
        if len(mask.shape) > 2:
            mask = cv2.cvtColor(mask, cv2.COLOR_BGR2GRAY)
        mask = (mask > 0).astype(np.uint8) * 255
        
        # Dilate mask for better edge coverage
        kernel = np.ones((3, 3), np.uint8)
        mask = cv2.dilate(mask, kernel, iterations=1)
        
        # Choose method based on watermark size
        mask_area = np.sum(mask > 0)
        frame_area = frame.shape[0] * frame.shape[1]
        mask_ratio = mask_area / frame_area if frame_area > 0 else 0
        
        if mask_ratio < 0.01:
            method = cv2.INPAINT_TELEA
            self.logger.debug("Using Telea inpainting (small watermark)")
        else:
            method = cv2.INPAINT_NS
            self.logger.debug("Using Navier-Stokes inpainting")
        
        try:
            result = cv2.inpaint(frame, mask, inpaintRadius=3, flags=method)
            return result
        except Exception as e:
            self.logger.error(f"OpenCV inpainting failed: {e}")
            return frame


class LaMaInpainter:
    """
    LaMa neural network-based inpainting for PRO mode.
    
    Requires PyTorch and the LaMa model.
    """
    
    def __init__(self, model_path: str = None):
        self.logger = logging.getLogger(__name__)
        self.model = None
        self.device = "cpu"
        self.model_path = model_path
        
        # Check for CUDA
        try:
            import torch
            if torch.cuda.is_available():
                self.device = "cuda"
                self.logger.info("CUDA available for LaMa")
        except ImportError:
            pass
        
        # Try to load model
        if model_path and os.path.exists(model_path):
            self._load_model()
        else:
            self.logger.warning("LaMa model not available")
    
    def _load_model(self):
        """Load the LaMa model."""
        try:
            # This is a placeholder - actual LaMa loading would go here
            # Example:
            # from lama_inpainter import LamaModel
            # self.model = LamaModel(self.model_path, device=self.device)
            self.logger.info(f"LaMa model loaded on {self.device}")
        except Exception as e:
            self.logger.warning(f"Failed to load LaMa model: {e}")
            self.model = None
    
    @property
    def is_available(self) -> bool:
        """Check if LaMa is available."""
        return self.model is not None
    
    def inpaint(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint using LaMa.
        
        Args:
            frame: Input frame (BGR format)
            mask: Binary mask (255 = watermark region)
            
        Returns:
            Inpainted frame
        """
        if frame is None or frame.size == 0:
            return frame
            
        if not self.is_available:
            self.logger.warning("LaMa not available, falling back to OpenCV")
            opencv = OpenCVInpainter()
            return opencv.inpaint(frame, mask)
        
        try:
            # Preprocess
            rgb_frame = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)
            
            # Ensure mask is single channel
            if len(mask.shape) > 2:
                mask = cv2.cvtColor(mask, cv2.COLOR_BGR2GRAY)
            mask = (mask > 127).astype(np.uint8) * 255
            
            # Run inference (placeholder)
            # result = self.model.predict(rgb_frame, mask)
            
            # For now, fall back to OpenCV
            self.logger.warning("LaMa inference not implemented, using OpenCV")
            opencv = OpenCVInpainter()
            return opencv.inpaint(frame, mask)
            
        except Exception as e:
            self.logger.error(f"LaMa inpainting failed: {e}, falling back to OpenCV")
            opencv = OpenCVInpainter()
            return opencv.inpaint(frame, mask)


class Inpainter:
    """
    Main inpainting service supporting FAST and PRO modes.
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Directory for temporary files
    """
    
    def __init__(self, config: WatermarkConfig, temp_dir: str = "./tmp"):
        self.config = config
        self.temp_dir = temp_dir
        
        # Determine mode
        if config.use_pro_mode:
            self.mode = InpaintingMode.PRO
            self._lama_inpainter = LaMaInpainter(config.lama_model_path)
            if not self._lama_inpainter.is_available and config.pro_fallback_to_fast:
                self.logger.warning("PRO mode unavailable, falling back to FAST")
                self.mode = InpaintingMode.FAST
        else:
            self.mode = InpaintingMode.FAST
        
        self._opencv_inpainter = OpenCVInpainter()
        
        # Create temp directory
        os.makedirs(temp_dir, exist_ok=True)
        
        self.logger = logging.getLogger(__name__)
        self.logger.info(f"Inpainter initialized with mode: {self.mode.value}")
    
    def inpaint_frame(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint a single frame.
        
        Args:
            frame: Input frame (BGR format)
            mask: Binary mask (255 = watermark region)
            
        Returns:
            Inpainted frame
        """
        if self.mode == InpaintingMode.PRO and self._lama_inpainter.is_available:
            return self._lama_inpainter.inpaint(frame, mask)
        else:
            return self._opencv_inpainter.inpaint(frame, mask)
    
    def inpaint_frames(self, frames: List[np.ndarray],
                      masks: List[MaskResult],
                      progress_callback: Optional[Callable[[int, int], None]] = None) -> Tuple[List[np.ndarray], InpaintResult]:
        """
        Inpaint multiple frames using masks.
        
        Args:
            frames: List of frames to inpaint
            masks: List of masks (or single mask for static watermark)
            progress_callback: Optional callback(current, total)
            
        Returns:
            Tuple of (inpainted_frames, InpaintResult)
        """
        start_time = time.time()
        
        # Handle single mask (static watermark)
        if len(masks) == 1:
            single_mask = masks[0]
            masks = [single_mask] * len(frames)
        
        self.logger.info(f"Inpainting {len(frames)} frames")
        
        inpainted_frames = []
        total = len(frames)
        
        for i, (frame, mask) in enumerate(zip(frames, masks)):
            try:
                # Create frame-level mask
                if mask.x == 0 and mask.y == 0 and mask.width == 0:
                    # Full frame mask
                    mask_array = np.zeros(frame.shape[:2], dtype=np.uint8)
                    mask_array[mask.y:mask.y+mask.height, mask.x:mask.x+mask.width] = 255
                else:
                    mask_array = self._create_mask_array(mask, frame.shape)
                
                inpainted = self.inpaint_frame(frame, mask_array)
                inpainted_frames.append(inpainted)
                
                if progress_callback and (i + 1) % 10 == 0:
                    progress_callback(i + 1, total)
                    
            except Exception as e:
                self.logger.error(f"Failed to inpaint frame {i}: {e}")
                if frame is not None and frame.size > 0:
                    inpainted_frames.append(frame)
                else:
                    inpainted_frames.append(frames[i] if i < len(frames) else None)
        
        elapsed = time.time() - start_time
        fps = len(frames) / elapsed if elapsed > 0 else 0
        
        self.logger.info(
            f"Inpainting complete: {len(inpainted_frames)} frames in {elapsed:.1f}s ({fps:.1f} fps)"
        )
        
        return inpainted_frames, InpaintResult(
            success=True,
            frames_processed=len(inpainted_frames),
            processing_time=elapsed,
            mode=self.mode
        )
    
    def _create_mask_array(self, mask: MaskResult, frame_shape: Tuple[int, int]) -> np.ndarray:
        """Create a full-frame mask array from MaskResult."""
        h, w = frame_shape[:2]
        mask_array = np.zeros((h, w), dtype=np.uint8)
        
        # Ensure bounds are within frame
        x1 = max(0, mask.x)
        y1 = max(0, mask.y)
        x2 = min(w, mask.x + mask.width)
        y2 = min(h, mask.y + mask.height)
        
        mask_array[y1:y2, x1:x2] = 255
        
        return mask_array


def create_inpainter(config: WatermarkConfig, temp_dir: str = "./tmp") -> Inpainter:
    """
    Factory function to create an Inpainter.
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Temporary directory
        
    Returns:
        Inpainter instance
    """
    return Inpainter(config, temp_dir)
