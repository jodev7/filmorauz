"""
Video Processor Module

Handles video reconstruction after inpainting.
- Preserves original audio stream
- Ensures A/V sync
- Outputs clean intermediate video
- Supports parallel frame processing for performance
"""

import os
import logging
import subprocess
import tempfile
from dataclasses import dataclass, field
from typing import List, Optional, Callable, Tuple
import numpy as np
import cv2

from .config import WatermarkConfig
from .masks import MaskGenerator
from .watermark_types import MaskResult, RegionCandidate


logger = logging.getLogger(__name__)


@dataclass
class VideoInfo:
    """Video file information."""
    path: str
    width: int
    height: int
    fps: float
    duration: float
    total_frames: int
    has_audio: bool = True
    audio_codec: str = ""
    video_codec: str = ""
    
    def to_dict(self) -> dict:
        return {
            "path": self.path,
            "width": self.width,
            "height": self.height,
            "fps": self.fps,
            "duration": self.duration,
            "total_frames": self.total_frames,
            "has_audio": self.has_audio,
            "audio_codec": self.audio_codec,
            "video_codec": self.video_codec,
        }


@dataclass
class VideoProcessResult:
    """
    Result of video processing operation.
    
    Attributes:
        success: Whether processing completed successfully
        output_path: Path to output video
        input_path: Path to input video
        duration: Processing duration in seconds
        frames_processed: Number of frames processed
        error: Error message if failed
    """
    success: bool = False
    output_path: str = ""
    input_path: str = ""
    duration: float = 0.0
    frames_processed: int = 0
    error: str = ""
    
    def to_dict(self) -> dict:
        return {
            "success": self.success,
            "output_path": self.output_path,
            "input_path": self.input_path,
            "duration": self.duration,
            "frames_processed": self.frames_processed,
            "error": self.error,
        }


class VideoProcessor:
    """
    Handles video frame extraction, processing, and reconstruction.
    
    Features:
    - Frame extraction with configurable sampling
    - Audio stream preservation
    - Parallel processing for performance
    - Progress reporting
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Directory for temporary files
    """
    
    def __init__(self, config: WatermarkConfig, temp_dir: str = "./tmp"):
        self.config = config
        self.temp_dir = temp_dir
        self.logger = logging.getLogger(__name__)
        
        # Create temp directory
        os.makedirs(temp_dir, exist_ok=True)
    
    def get_video_info(self, video_path: str) -> Optional[VideoInfo]:
        """
        Get video file information using ffprobe.
        
        Args:
            video_path: Path to video file
            
        Returns:
            VideoInfo or None if failed
        """
        try:
            cmd = [
                "ffprobe", "-v", "error",
                "-select_streams", "v:0",
                "-show_entries", "stream=width,height,r_frame_rate,codec_name,duration,nb_frames",
                "-show_entries", "format=duration",
                "-of", "json",
                video_path
            ]
            
            result = subprocess.run(cmd, capture_output=True, text=True, timeout=30)
            if result.returncode != 0:
                self.logger.error(f"ffprobe failed: {result.stderr}")
                return None
            
            import json
            info = json.loads(result.stdout)
            
            streams = info.get("streams", [{}])
            video_stream = streams[0] if streams else {}
            
            # Parse frame rate
            fps_str = video_stream.get("r_frame_rate", "30/1")
            if '/' in fps_str:
                num, den = fps_str.split('/')
                fps = float(num) / float(den) if float(den) != 0 else 30.0
            else:
                fps = float(fps_str)
            
            # Get audio info
            audio_cmd = [
                "ffprobe", "-v", "error",
                "-select_streams", "a:0",
                "-show_entries", "stream=codec_name",
                "-of", "json",
                video_path
            ]
            audio_result = subprocess.run(audio_cmd, capture_output=True, text=True, timeout=10)
            has_audio = False
            audio_codec = ""
            
            if audio_result.returncode == 0:
                audio_info = json.loads(audio_result.stdout)
                audio_streams = audio_info.get("streams", [])
                if audio_streams:
                    has_audio = True
                    audio_codec = audio_streams[0].get("codec_name", "")
            
            return VideoInfo(
                path=video_path,
                width=int(video_stream.get("width", 0)),
                height=int(video_stream.get("height", 0)),
                fps=fps,
                duration=float(info.get("format", {}).get("duration", 0)),
                total_frames=int(video_stream.get("nb_frames", 0)),
                has_audio=has_audio,
                audio_codec=audio_codec,
                video_codec=video_stream.get("codec_name", "")
            )
            
        except Exception as e:
            self.logger.error(f"Failed to get video info: {e}")
            return None
    
    def extract_frames(self, video_path: str, 
                       timestamps: List[float] = None,
                       max_frames: int = None) -> Tuple[List[np.ndarray], List[float]]:
        """
        Extract frames from video.
        
        Args:
            video_path: Path to video file
            timestamps: Specific timestamps to extract (optional)
            max_frames: Maximum frames to extract (optional)
            
        Returns:
            Tuple of (frames, timestamps)
        """
        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            self.logger.error(f"Failed to open video: {video_path}")
            return [], []
        
        fps = cap.get(cv2.CAP_PROP_FPS)
        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
        duration = total_frames / fps if fps > 0 else 0
        
        frames = []
        extracted_timestamps = []
        
        if timestamps:
            # Extract specific timestamps
            for ts in timestamps:
                frame_idx = int(ts * fps)
                frame_idx = min(frame_idx, total_frames - 1)
                
                cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
                ret, frame = cap.read()
                
                if ret:
                    frames.append(frame)
                    extracted_timestamps.append(ts)
        else:
            # Extract evenly distributed frames
            num_frames = max_frames or self.config.sample_count
            if num_frames > total_frames:
                num_frames = total_frames
            
            step = total_frames / num_frames
            
            for i in range(num_frames):
                frame_idx = int(i * step)
                cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
                ret, frame = cap.read()
                
                if ret:
                    frames.append(frame)
                    extracted_timestamps.append(frame_idx / fps)
        
        cap.release()
        self.logger.info(f"Extracted {len(frames)} frames from {video_path}")
        
        return frames, extracted_timestamps
    
    def write_video(self, frames: List[np.ndarray], output_path: str,
                    fps: float = 30.0, 
                    original_video_path: str = None,
                    progress_callback: Optional[Callable[[int, int], None]] = None) -> bool:
        """
        Write frames to video file, optionally preserving audio from original.
        
        Args:
            frames: List of frames to write
            output_path: Output video path
            fps: Frames per second for output
            original_video_path: Original video for audio extraction
            progress_callback: Progress callback
            
        Returns:
            True if successful
        """
        if not frames:
            self.logger.error("No frames to write")
            return False
        
        h, w = frames[0].shape[:2]
        
        # Create output directory
        os.makedirs(os.path.dirname(output_path) or '.', exist_ok=True)
        
        # Write video without audio first
        temp_video = output_path + ".temp.mp4"
        fourcc = cv2.VideoWriter_fourcc(*'mp4v')
        out = cv2.VideoWriter(temp_video, fourcc, fps, (w, h))
        
        if not out.isOpened():
            self.logger.error(f"Failed to create video writer for {temp_video}")
            return False
        
        total = len(frames)
        for i, frame in enumerate(frames):
            out.write(frame)
            if progress_callback and (i + 1) % 30 == 0:
                progress_callback(i + 1, total)
        
        out.release()
        
        # If original video provided, copy audio stream
        if original_video_path and os.path.exists(original_video_path):
            success = self._merge_audio(temp_video, output_path, original_video_path)
            if success:
                os.remove(temp_video)
                return True
            else:
                # Fallback: use temp video without audio
                os.rename(temp_video, output_path)
                return True
        else:
            # No audio to merge
            os.rename(temp_video, output_path)
            return True
        
        return False
    
    def _merge_audio(self, video_without_audio: str, output_path: str, 
                    original_video_path: str) -> bool:
        """
        Merge audio from original video into processed video.
        
        Args:
            video_without_audio: Path to processed video without audio
            output_path: Final output path
            original_video_path: Original video with audio
            
        Returns:
            True if successful
        """
        try:
            # Use ffmpeg to copy audio stream
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
            
            result = subprocess.run(
                cmd, 
                capture_output=True, 
                text=True,
                timeout=300
            )
            
            if result.returncode != 0:
                self.logger.warning(f"Audio merge failed: {result.stderr}")
                return False
            
            return True
            
        except Exception as e:
            self.logger.error(f"Failed to merge audio: {e}")
            return False
    
    def process_frames_parallel(self, frames: List[np.ndarray], 
                               process_func: callable,
                               num_workers: int = None) -> List[np.ndarray]:
        """
        Process frames in parallel using multiprocessing.
        
        Args:
            frames: List of frames to process
            process_func: Function to apply to each frame
            num_workers: Number of worker processes
            
        Returns:
            List of processed frames
        """
        from concurrent.futures import ProcessPoolExecutor, as_completed
        
        num_workers = num_workers or self.config.num_workers
        processed = []
        
        # For small number of frames, process sequentially
        if len(frames) < 10:
            return [process_func(f) for f in frames]
        
        try:
            with ProcessPoolExecutor(max_workers=num_workers) as executor:
                # Submit all frames
                future_to_idx = {
                    executor.submit(process_func, frame): idx 
                    for idx, frame in enumerate(frames)
                }
                
                # Collect results
                results = [None] * len(frames)
                for future in as_completed(future_to_idx):
                    idx = future_to_idx[future]
                    try:
                        results[idx] = future.result()
                    except Exception as e:
                        self.logger.error(f"Frame {idx} processing failed: {e}")
                        results[idx] = frames[idx]  # Keep original
                
                processed = results
                
        except Exception as e:
            self.logger.error(f"Parallel processing failed: {e}")
            # Fallback to sequential
            processed = [process_func(f) for f in frames]
        
        return processed
    
    def validate_video(self, video_path: str) -> bool:
        """
        Validate that a video file is valid and playable.
        
        Args:
            video_path: Path to video file
            
        Returns:
            True if valid
        """
        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            return False
        
        # Try to read a frame
        ret, _ = cap.read()
        cap.release()
        
        return ret
    
    def get_duration(self, video_path: str) -> float:
        """
        Get video duration in seconds.
        
        Args:
            video_path: Path to video file
            
        Returns:
            Duration in seconds, or 0 if failed
        """
        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            return 0.0
        
        fps = cap.get(cv2.CAP_PROP_FPS)
        frame_count = cap.get(cv2.CAP_PROP_FRAME_COUNT)
        cap.release()
        
        if fps > 0:
            return frame_count / fps
        return 0.0


def create_video_processor(config: WatermarkConfig, temp_dir: str = "./tmp") -> VideoProcessor:
    """
    Factory function to create a VideoProcessor.
    
    Args:
        config: WatermarkConfig instance
        temp_dir: Temporary directory
        
    Returns:
        VideoProcessor instance
    """
    return VideoProcessor(config, temp_dir)
