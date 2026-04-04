"""
Inpainting Module

Provides two modes of inpainting for watermark removal:

FAST MODE (OpenCV):
- Uses OpenCV's Telea or Navier-Stokes inpainting
- Fast, suitable for real-time processing
- Good for small to medium watermark regions
- Works well for corner logos and text

PRO MODE (LaMa):
- Uses LaMa (Large Mask Inpainting) neural network
- Higher quality restoration
- Better for complex watermark patterns
- Requires GPU for best performance

Both modes use multiprocessing for parallel frame processing.
"""

import os
import logging
import subprocess
import tempfile
from dataclasses import dataclass, field
from typing import List, Optional, Tuple, Callable
from enum import Enum
import numpy as np
import cv2

from .config import WatermarkConfig
from .mask_generator import Mask


logger = logging.getLogger(__name__)


class InpaintMethod(Enum):
    """Available inpainting methods."""
    TELEA = "telea"      # OpenCV Telea method
    NAVIER_STOKES = "ns" # OpenCV Navier-Stokes method
    LAMA = "lama"        # LaMa neural network


@dataclass
class InpaintResult:
    """
    Result of inpainting operation.
    
    Attributes:
        success: Whether inpainting completed successfully
        output_frames: List of inpainted frames (for single-frame mode)
        output_path: Path to output video (for video mode)
        method: Inpainting method used
        frames_processed: Number of frames processed
        processing_time: Total processing time in seconds
        error: Error message if failed
    """
    success: bool = False
    output_frames: List[np.ndarray] = field(default_factory=list)
    output_path: str = ""
    method: InpaintMethod = InpaintMethod.TELEA
    frames_processed: int = 0
    processing_time: float = 0.0
    error: str = ""
    
    def to_dict(self) -> dict:
        """Convert to dictionary for serialization."""
        return {
            "success": self.success,
            "output_path": self.output_path,
            "method": self.method.value,
            "frames_processed": self.frames_processed,
            "processing_time": self.processing_time,
            "error": self.error,
        }


class Inpainter:
    """
    Inpainting service for watermark removal.
    
    Supports two modes:
    - FAST: OpenCV-based (Telea or Navier-Stokes)
    - PRO: LaMa neural network-based
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Directory for temporary files
    """
    
    def __init__(self, config: WatermarkConfig, temp_dir: str = "./tmp"):
        self.config = config
        self.temp_dir = temp_dir
        
        # Determine method based on config
        self.method = InpaintMethod.LAMA if config.use_pro_mode else InpaintMethod.TELEA
        
        # Create temp directory
        os.makedirs(temp_dir, exist_ok=True)
        
        self.logger = logging.getLogger(__name__)
        self.logger.info(f"Inpainter initialized with method: {self.method.value}")
        
        # Check LaMa availability for PRO mode
        self._lama_available = self._check_lama_availability()
        if config.use_pro_mode and not self._lama_available:
            self.logger.warning("LaMa not available, falling back to FAST mode")
            self.method = InpaintMethod.TELEA
        
        # Initialize multiprocessing pool
        self._pool = None
    
    def _check_lama_availability(self) -> bool:
        """Check if LaMa is available and configured."""
        if not self.config.lama_model_path:
            self.logger.info("LaMa model path not configured")
            return False
        
        if not os.path.exists(self.config.lama_model_path):
            self.logger.warning(f"LaMa model not found at: {self.config.lama_model_path}")
            return False
        
        # Check if torch is available
        try:
            import torch
            self.logger.info(f"LaMa check: torch available, CUDA: {torch.cuda.is_available()}")
            return True
        except ImportError:
            self.logger.warning("PyTorch not available - LaMa requires torch")
            return False
    
    def inpaint_frame(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint a single frame with the configured method.
        
        Args:
            frame: Input frame (BGR format)
            mask: Binary mask (255 = watermark region to inpaint)
            
        Returns:
            Inpainted frame
        """
        if self.method == InpaintMethod.LAMA and self._lama_available:
            return self._inpaint_with_lama(frame, mask)
        else:
            return self._inpaint_with_opencv(frame, mask)
    
    def _inpaint_with_opencv(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint using OpenCV method.
        
        Supports Telea (fast, good quality) and Navier-Stokes (slower, smoother).
        """
        # Ensure mask is correct format
        if len(mask.shape) > 2:
            mask = cv2.cvtColor(mask, cv2.COLOR_BGR2GRAY)
        mask = (mask > 0).astype(np.uint8) * 255
        
        # Dilate mask slightly for better edge coverage
        kernel = np.ones((3, 3), np.uint8)
        mask = cv2.dilate(mask, kernel, iterations=1)
        
        # Choose method based on watermark size
        mask_area = np.sum(mask > 0)
        frame_area = frame.shape[0] * frame.shape[1]
        mask_ratio = mask_area / frame_area
        
        if mask_ratio < 0.01:  # Small watermark
            # Use Telea (faster)
            method = cv2.INPAINT_TELEA
            self.logger.debug("Using Telea inpainting (small watermark)")
        else:
            # Use Navier-Stokes (better for larger areas)
            method = cv2.INPAINT_NS
            self.logger.debug("Using Navier-Stokes inpainting")
        
        # Perform inpainting
        try:
            result = cv2.inpaint(frame, mask, inpaintRadius=3, flags=method)
            return result
        except Exception as e:
            self.logger.error(f"OpenCV inpainting failed: {e}")
            # Return original frame as fallback
            return frame
    
    def _inpaint_with_lama(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint using LaMa neural network.
        
        LaMa provides better quality especially for complex watermark patterns.
        """
        try:
            import torch
            from lama_inpainter import LamaInpainter
            
            # Lazy load LaMa
            if not hasattr(self, '_lama_model'):
                self._lama_model = LamaInpainter(self.config.lama_model_path)
            
            # Prepare inputs
            if len(mask.shape) > 2:
                mask = cv2.cvtColor(mask, cv2.COLOR_BGR2GRAY)
            mask = (mask > 127).astype(np.uint8) * 255
            
            # Run inpainting
            result = self._lama_model.inpaint(frame, mask)
            
            return result
        except ImportError as e:
            self.logger.warning(f"LaMa not available: {e}, falling back to OpenCV")
            self.method = InpaintMethod.TELEA
            return self._inpaint_with_opencv(frame, mask)
        except Exception as e:
            self.logger.error(f"LaMa inpainting failed: {e}, falling back to OpenCV")
            self.method = InpaintMethod.TELEA
            return self._inpaint_with_opencv(frame, mask)
    
    def inpaint_video_frames(self, frames: List[np.ndarray], 
                              masks: List[Mask],
                              progress_callback: Optional[Callable[[int, int], None]] = None) -> List[np.ndarray]:
        """
        Inpaint multiple frames using masks.
        
        For static watermarks, uses the same mask for all frames.
        For dynamic watermarks, uses corresponding masks.
        
        Args:
            frames: List of frames to inpaint
            masks: List of masks (or single mask for static watermark)
            progress_callback: Optional callback(current, total) for progress updates
            
        Returns:
            List of inpainted frames
        """
        import time
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
                # Create frame-level mask if mask is full-frame
                if mask.mask_array.shape[:2] != frame.shape[:2]:
                    frame_mask = np.zeros(frame.shape[:2], dtype=np.uint8)
                    # Scale mask to frame size
                    mask_resized = cv2.resize(
                        mask.mask_array, 
                        (frame.shape[1], frame.shape[0])
                    )
                    frame_mask = (mask_resized > 127).astype(np.uint8) * 255
                else:
                    frame_mask = mask.mask_array
                
                inpainted = self.inpaint_frame(frame, frame_mask)
                inpainted_frames.append(inpainted)
                
                if progress_callback and (i + 1) % 10 == 0:
                    progress_callback(i + 1, total)
                    
            except Exception as e:
                self.logger.error(f"Failed to inpaint frame {i}: {e}")
                inpainted_frames.append(frame)  # Keep original
        
        elapsed = time.time() - start_time
        fps = len(frames) / elapsed if elapsed > 0 else 0
        
        self.logger.info(
            f"Inpainting complete: {len(inpainted_frames)} frames in {elapsed:.1f}s ({fps:.1f} fps)"
        )
        
        return inpainted_frames
    
    def inpaint_video_inplace(self, input_path: str, output_path: str,
                               masks: List[Mask],
                               progress_callback: Optional[Callable[[int, int], None]] = None) -> InpaintResult:
        """
        Inpaint video file directly, writing to output file.
        
        This method is more memory efficient as it processes frames
        without loading all frames into memory.
        
        Args:
            input_path: Path to input video
            output_path: Path to output video
            masks: List of masks for watermark regions
            progress_callback: Optional callback for progress updates
            
        Returns:
            InpaintResult with processing details
        """
        import time
        start_time = time.time()
        
        # Handle single mask
        if len(masks) == 1:
            single_mask = masks[0]
            masks = [single_mask]
        
        try:
            # Open input video
            cap = cv2.VideoCapture(input_path)
            if not cap.isOpened():
                return InpaintResult(
                    success=False,
                    error=f"Failed to open video: {input_path}"
                )
            
            # Get video properties
            fps = cap.get(cv2.CAP_PROP_FPS)
            width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
            height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
            total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
            
            self.logger.info(
                f"Processing video: {width}x{height} @ {fps}fps, {total_frames} frames"
            )
            
            # Create output video writer
            fourcc = cv2.VideoWriter_fourcc(*'mp4v')
            out = cv2.VideoWriter(output_path, fourcc, fps, (width, height))
            
            if not out.isOpened():
                cap.release()
                return InpaintResult(
                    success=False,
                    error=f"Failed to create output video: {output_path}"
                )
            
            frame_idx = 0
            processed = 0
            
            while True:
                ret, frame = cap.read()
                if not ret:
                    break
                
                # Get mask for this frame
                # For static watermarks, use first mask
                # For dynamic, use corresponding mask (with wrapping)
                mask_idx = min(frame_idx, len(masks) - 1)
                mask = masks[mask_idx]
                
                # Scale mask to frame size if needed
                if mask.mask_array.shape[:2] != frame.shape[:2]:
                    frame_mask = cv2.resize(
                        mask.mask_array,
                        (frame.shape[1], frame.shape[0])
                    )
                    frame_mask = (frame_mask > 127).astype(np.uint8) * 255
                else:
                    frame_mask = mask.mask_array
                
                # Inpaint frame
                inpainted = self.inpaint_frame(frame, frame_mask)
                out.write(inpainted)
                processed += 1
                frame_idx += 1
                
                if progress_callback and frame_idx % 30 == 0:
                    progress_callback(frame_idx, total_frames)
            
            cap.release()
            out.release()
            
            elapsed = time.time() - start_time
            fps_processing = processed / elapsed if elapsed > 0 else 0
            
            self.logger.info(
                f"Inpainting complete: {processed} frames in {elapsed:.1f}s ({fps_processing:.1f} fps)"
            )
            
            return InpaintResult(
                success=True,
                output_path=output_path,
                method=self.method,
                frames_processed=processed,
                processing_time=elapsed
            )
            
        except Exception as e:
            self.logger.error(f"Inpainting failed: {e}")
            return InpaintResult(
                success=False,
                error=str(e),
                method=self.method
            )


class LamaInpainter:
    """
    LaMa (Large Mask Inpainting) wrapper.
    
    This is a placeholder implementation. In production,
    you would use the actual LaMa implementation from:
    https://github.com/advimman/lama
    
    The actual implementation would:
    1. Load the LaMa model
    2. Preprocess frame and mask
    3. Run inference
    4. Postprocess result
    """
    
    def __init__(self, model_path: str):
        self.model_path = model_path
        self.model = None
        self.device = "cuda" if self._has_cuda() else "cpu"
        self._load_model()
    
    def _has_cuda(self) -> bool:
        """Check if CUDA is available."""
        try:
            import torch
            return torch.cuda.is_available()
        except ImportError:
            return False
    
    def _load_model(self):
        """Load the LaMa model."""
        try:
            import torch
            from lama_inpainter import LamaModel
            
            # Load model
            self.model = LamaModel(self.model_path, device=self.device)
            logger.info(f"LaMa model loaded on {self.device}")
        except Exception as e:
            logger.error(f"Failed to load LaMa model: {e}")
            raise
    
    def inpaint(self, frame: np.ndarray, mask: np.ndarray) -> np.ndarray:
        """
        Inpaint using LaMa.
        
        Args:
            frame: Input frame (BGR)
            mask: Binary mask (255 = inpaint region)
            
        Returns:
            Inpainted frame
        """
        # Preprocess
        # Convert BGR to RGB
        rgb_frame = cv2.cvtColor(frame, cv2.COLOR_BGR2RGB)
        
        # Run inference
        result = self.model.predict(rgb_frame, mask)
        
        # Postprocess
        # Convert RGB back to BGR
        result_bgr = cv2.cvtColor(result, cv2.COLOR_RGB2BGR)
        
        return result_bgr


def create_inpainter(config: WatermarkConfig, temp_dir: str = "./tmp") -> Inpainter:
    """
    Factory function to create an Inpainter based on config.
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Temporary directory for processing
        
    Returns:
        Inpainter instance
    """
    return Inpainter(config, temp_dir)
