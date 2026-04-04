"""
Watermark Removal Pipeline

Main orchestrator for the watermark removal pipeline.
Coordinates all stages:
1. Frame Sampling
2. Watermark Detection
3. Mask Generation
4. Inpainting
5. Video Reconstruction

Classes:
    WatermarkRemovalPipeline: Main pipeline orchestrator
"""

import os
import logging
import time
import uuid
import shutil
from typing import List, Optional, Callable, Tuple
import numpy as np
import cv2

from .config import WatermarkConfig, load_config
from .watermark_types import (
    RegionCandidate, MaskResult, WatermarkType,
    WatermarkDetectionResult, WatermarkRemovalResult, PipelineStage
)
from .sampler import FrameSampler, VideoInfo
from .detector import WatermarkDetector
from .masks import MaskGenerator
from .inpaint import Inpainter, create_inpainter


logger = logging.getLogger(__name__)


class WatermarkRemovalPipeline:
    """
    Main pipeline for watermark removal.
    
    Coordinates all stages:
    1. Frame Sampling
    2. Watermark Detection
    3. Mask Generation
    4. Inpainting
    5. Video Reconstruction
    
    Args:
        config: WatermarkConfig instance (or None to load from env)
        temp_dir: Temporary directory for processing
        progress_callback: Optional callback for progress updates
    """
    
    def __init__(self, config: WatermarkConfig = None, 
                 temp_dir: str = None,
                 progress_callback: Callable[[str, int], None] = None):
        self.config = config or load_config()
        self.temp_dir = temp_dir or self.config.temp_dir
        self.progress_callback = progress_callback
        
        # Create components
        self.sampler = FrameSampler(self.config, self.temp_dir)
        self.detector = WatermarkDetector(self.config, self.temp_dir)
        self.mask_generator = MaskGenerator(self.config, self.temp_dir)
        self.inpainter = create_inpainter(self.config, self.temp_dir)
        
        # Ensure temp directory exists
        os.makedirs(self.temp_dir, exist_ok=True)
        
        self.logger = logging.getLogger(__name__)
        
        # Processing state
        self._stages: List[str] = []
    
    def process(self, input_path: str, 
                output_path: str = None) -> WatermarkRemovalResult:
        """
        Process a video file to remove watermarks.
        
        Args:
            input_path: Path to input video file
            output_path: Path for output video (optional, auto-generated)
            
        Returns:
            WatermarkRemovalResult with processing details
        """
        start_time = time.time()
        
        # Validate input
        if not os.path.exists(input_path):
            return WatermarkRemovalResult(
                success=False,
                input_path=input_path,
                error=f"Input file not found: {input_path}"
            )
        
        # Check if watermark removal is enabled
        if not self.config.enabled:
            self.logger.info("Watermark removal is disabled, skipping")
            return WatermarkRemovalResult(
                success=True,
                input_path=input_path,
                output_path=input_path,
                watermark_detected=False,
                warning="Watermark removal disabled",
                stages=self._stages
            )
        
        # Generate output path if not provided
        if not output_path:
            output_path = self._generate_output_path(input_path)
        
        self.logger.info(f"Starting watermark removal pipeline")
        self.logger.info(f"Input: {input_path}")
        self.logger.info(f"Output: {output_path}")
        
        # Create temp directory for this job
        job_temp_dir = os.path.join(self.temp_dir, f"watermark_{uuid.uuid4().hex[:8]}")
        os.makedirs(job_temp_dir, exist_ok=True)
        
        try:
            # Stage 1: Frame Sampling
            self._log_stage(PipelineStage.RECEIVED)
            
            # Get video info
            video_info = self.sampler.get_video_info(input_path)
            if not video_info:
                return WatermarkRemovalResult(
                    success=False,
                    input_path=input_path,
                    error="Failed to get video information"
                )
            
            # Sample frames for analysis
            self._log_stage(PipelineStage.SAMPLING)
            frames, timestamps = self.sampler.sample_frames(input_path)
            
            if not frames:
                self.logger.warning("No frames extracted, skipping watermark removal")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=False,
                    warning="No frames could be extracted",
                    stages=self._stages
                )
            
            # Stage 2: Watermark Detection
            self._log_stage(PipelineStage.DETECTING)
            detection_result = self.detector.detect(input_path, frames, timestamps)
            
            if not detection_result.detected:
                self.logger.info("No watermarks detected")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=False,
                    mode_used=self.config.mode,
                    stages=self._stages,
                    total_time=time.time() - start_time
                )
            
            self.logger.info(f"Detected {len(detection_result.regions)} watermark regions")
            
            # Stage 3: Mask Generation
            self._log_stage(PipelineStage.MASKS_CREATED)
            frame_shape = frames[0].shape
            masks = self.mask_generator.generate_masks(
                detection_result.regions, 
                frame_shape,
                frames
            )
            
            if not masks:
                self.logger.warning("No masks generated")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=True,
                    warning="Mask generation failed",
                    regions=detection_result.regions,
                    stages=self._stages,
                    total_time=time.time() - start_time
                )
            
            # Stage 4: Inpainting
            mode_stage = PipelineStage.INPAINTING_PRO if self.inpainter.mode.value == "pro" else PipelineStage.INPAINTING_FAST
            self._log_stage(mode_stage)
            
            # For efficiency, process frames in batches
            batch_size = min(len(frames), 50)
            inpainted_frames = []
            
            for i in range(0, len(frames), batch_size):
                batch = frames[i:i+batch_size]
                batch_masks = [masks[0]] * len(batch)  # Reuse first mask for static watermarks
                batch_inpainted, _ = self.inpainter.inpaint_frames(batch, batch_masks)
                inpainted_frames.extend(batch_inpainted)
                
                self.logger.info(
                    f"Inpainted frames {i+1}-{min(i+batch_size, len(frames))}/{len(frames)}"
                )
            
            # Stage 5: Video Reconstruction
            self._log_stage(PipelineStage.REBUILDING)
            success = self._write_video(
                inpainted_frames,
                output_path,
                fps=video_info.fps,
                original_video_path=input_path
            )
            
            if not success or not os.path.exists(output_path):
                self.logger.error("Video reconstruction failed")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=True,
                    watermark_removed=False,
                    fallback_used=True,
                    warning="Video reconstruction failed, using original",
                    regions=detection_result.regions,
                    stages=self._stages,
                    total_time=time.time() - start_time
                )
            
            # Validate output
            if not self._validate_video(output_path):
                self.logger.error("Output video is invalid")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=True,
                    watermark_removed=False,
                    fallback_used=True,
                    warning="Output validation failed, using original",
                    regions=detection_result.regions,
                    stages=self._stages,
                    total_time=time.time() - start_time
                )
            
            self._log_stage(PipelineStage.COMPLETE)
            
            self.logger.info(f"Watermark removal complete: {output_path}")
            
            return WatermarkRemovalResult(
                success=True,
                input_path=input_path,
                output_path=output_path,
                watermark_detected=True,
                watermark_removed=True,
                mode_used=self.config.mode,
                regions=detection_result.regions,
                fallback_used=False,
                stages=self._stages,
                total_time=time.time() - start_time
            )
            
        except Exception as e:
            self.logger.error(f"Pipeline error: {e}", exc_info=True)
            
            return WatermarkRemovalResult(
                success=True,
                input_path=input_path,
                output_path=input_path,
                watermark_detected=True,
                watermark_removed=False,
                fallback_used=True,
                warning=f"Pipeline error: {str(e)}",
                stages=self._stages,
                total_time=time.time() - start_time,
                error=str(e)
            )
        
        finally:
            # Cleanup temp directory
            try:
                if os.path.exists(job_temp_dir):
                    shutil.rmtree(job_temp_dir)
            except Exception as e:
                self.logger.warning(f"Failed to cleanup temp dir: {e}")
    
    def _log_stage(self, stage: PipelineStage):
        """Log current pipeline stage."""
        stage_str = stage.value
        self._stages.append(stage_str)
        self.logger.info(f"[WATERMARK] Stage: {stage_str}")
        
        if self.progress_callback:
            self.progress_callback(stage_str, 0)
    
    def _generate_output_path(self, input_path: str) -> str:
        """Generate output path for processed video."""
        base_dir = os.path.dirname(input_path)
        filename = os.path.basename(input_path)
        name, ext = os.path.splitext(filename)
        
        output_name = f"{name}_clean{ext}"
        output_path = os.path.join(base_dir, output_name)
        
        if os.path.exists(output_path):
            unique_id = uuid.uuid4().hex[:8]
            output_name = f"{name}_clean_{unique_id}{ext}"
            output_path = os.path.join(base_dir, output_name)
        
        return output_path
    
    def _write_video(self, frames: List[np.ndarray], output_path: str,
                    fps: float, original_video_path: str) -> bool:
        """Write frames to video file, preserving audio."""
        if not frames:
            return False
        
        # Filter out None frames
        valid_frames = [f for f in frames if f is not None and f.size > 0]
        if not valid_frames:
            return False
        
        h, w = valid_frames[0].shape[:2]
        os.makedirs(os.path.dirname(output_path) or '.', exist_ok=True)
        
        # Write video without audio first
        temp_video = output_path + ".temp.mp4"
        fourcc = cv2.VideoWriter_fourcc(*'mp4v')
        out = cv2.VideoWriter(temp_video, fourcc, fps, (w, h))
        
        if not out.isOpened():
            return False
        
        for frame in valid_frames:
            out.write(frame)
        out.release()
        
        # Merge audio from original
        if os.path.exists(original_video_path):
            success = self._merge_audio(temp_video, output_path, original_video_path)
            if success:
                os.remove(temp_video)
                return True
            else:
                os.rename(temp_video, output_path)
                return True
        else:
            os.rename(temp_video, output_path)
            return True
        
        return False
    
    def _merge_audio(self, video_without_audio: str, output_path: str, 
                    original_video_path: str) -> bool:
        """Merge audio from original video into processed video."""
        try:
            import subprocess
            cmd = [
                "ffmpeg", "-y",
                "-i", video_without_audio,
                "-i", original_video_path,
                "-c:v", "copy",
                "-c:a", "aac",
                "-b:a", "192k",
                "-map", "0:v:0",
                "-map", "1:a:0?",
                "-shortest",
                output_path
            ]
            
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=300)
            return result.returncode == 0
        except Exception as e:
            self.logger.warning(f"Audio merge failed: {e}")
            return False
    
    def _validate_video(self, video_path: str) -> bool:
        """Validate that a video file is valid."""
        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            return False
        
        ret, _ = cap.read()
        cap.release()
        
        return ret
    
    def detect_only(self, video_path: str) -> WatermarkDetectionResult:
        """
        Detect watermarks without removal.
        
        Args:
            video_path: Path to video file
            
        Returns:
            WatermarkDetectionResult with detected regions
        """
        frames, timestamps = self.sampler.sample_frames(video_path)
        return self.detector.detect(video_path, frames, timestamps)


# Convenience functions
def remove_watermark(input_path: str, 
                     output_path: str = None,
                     config: WatermarkConfig = None,
                     progress_callback: Callable[[str, int], None] = None) -> WatermarkRemovalResult:
    """
    Remove watermark from a video.
    
    Args:
        input_path: Path to input video
        output_path: Path for output video (optional)
        config: WatermarkConfig instance (optional)
        progress_callback: Progress callback function
        
    Returns:
        WatermarkRemovalResult
    """
    pipeline = WatermarkRemovalPipeline(
        config=config,
        progress_callback=progress_callback
    )
    return pipeline.process(input_path, output_path)


def detect_watermark(video_path: str, 
                    config: WatermarkConfig = None) -> WatermarkDetectionResult:
    """
    Detect watermark in a video without removal.
    
    Args:
        video_path: Path to video file
        config: WatermarkConfig instance (optional)
        
    Returns:
        WatermarkDetectionResult
    """
    pipeline = WatermarkRemovalPipeline(config=config)
    return pipeline.detect_only(video_path)
