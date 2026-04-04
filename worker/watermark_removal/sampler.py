"""
Frame Sampler for Watermark Removal Pipeline

Extracts sample frames from video for watermark detection analysis.

Classes:
    VideoInfo: Video file information dataclass
    FrameSampler: Samples frames from video for analysis
"""

import os
import logging
import subprocess
from typing import List, Tuple, Optional
from dataclasses import dataclass
import numpy as np
import cv2

from .config import WatermarkConfig


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
    
    @property
    def resolution(self) -> Tuple[int, int]:
        return (self.width, self.height)


class FrameSampler:
    """
    Samples frames from video for watermark detection.
    
    Features:
    - Intelligent timestamp selection
    - Frame extraction with ffprobe/ffmpeg
    - Multiple sampling strategies
    - Progress tracking
    
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
                "-show_entries", 
                "stream=width,height,r_frame_rate,codec_name,nb_frames",
                "-show_entries", 
                "format=duration,size",
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
            
            # Check for audio
            audio_cmd = [
                "ffprobe", "-v", "error",
                "-select_streams", "a:0",
                "-show_entries", "stream=codec_name",
                "-of", "json",
                video_path
            ]
            audio_result = subprocess.run(audio_cmd, capture_output=True, text=True, timeout=10)
            has_audio = False
            
            if audio_result.returncode == 0:
                audio_info = json.loads(audio_result.stdout)
                has_audio = len(audio_info.get("streams", [])) > 0
            
            return VideoInfo(
                path=video_path,
                width=int(video_stream.get("width", 0)),
                height=int(video_stream.get("height", 0)),
                fps=fps,
                duration=float(info.get("format", {}).get("duration", 0)),
                total_frames=int(video_stream.get("nb_frames", 0)),
                has_audio=has_audio
            )
            
        except Exception as e:
            self.logger.error(f"Failed to get video info: {e}")
            return None
    
    def extract_frames(self, video_path: str, 
                      timestamps: List[float] = None,
                      max_frames: int = None) -> Tuple[List[np.ndarray], List[float]]:
        """
        Extract frames from video at specified timestamps.
        
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
                
                if ret and frame is not None and frame.size > 0:
                    frames.append(frame)
                    extracted_timestamps.append(ts)
        else:
            # Extract evenly distributed frames
            num_frames = max_frames or self.config.sample_count
            if num_frames > total_frames:
                num_frames = total_frames
            
            if num_frames <= 0:
                cap.release()
                return [], []
            
            step = total_frames / num_frames
            
            for i in range(num_frames):
                frame_idx = int(i * step)
                cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
                ret, frame = cap.read()
                
                if ret and frame is not None and frame.size > 0:
                    frames.append(frame)
                    extracted_timestamps.append(frame_idx / fps)
        
        cap.release()
        self.logger.info(f"Extracted {len(frames)} frames from {video_path}")
        
        return frames, extracted_timestamps
    
    def sample_frames(self, video_path: str) -> Tuple[List[np.ndarray], List[float]]:
        """
        Sample frames using intelligent timestamp selection.
        
        Strategy:
        - Skip first 10 seconds (likely intro/cut region)
        - Sample evenly across remaining video
        - Extra samples from later portions for persistent overlay detection
        
        Args:
            video_path: Path to video file
            
        Returns:
            Tuple of (frames, timestamps)
        """
        video_info = self.get_video_info(video_path)
        if not video_info:
            self.logger.error("Failed to get video info")
            return [], []
        
        duration = video_info.duration
        
        # Calculate number of samples
        num_samples = min(self.config.sample_count, 20)
        
        # Determine sample timestamps
        # Skip first 10 seconds (likely intro/cut region)
        start_time = min(10.0, duration * 0.1)
        end_time = duration - 5.0  # Leave margin at end
        
        if end_time <= start_time:
            start_time = 0
            end_time = duration
        
        timestamps = []
        if num_samples == 1:
            timestamps = [(start_time + end_time) / 2]
        else:
            for i in range(num_samples):
                t = start_time + (end_time - start_time) * i / (num_samples - 1)
                timestamps.append(t)
        
        # Extract frames
        frames, extracted_timestamps = self.extract_frames(video_path, timestamps)
        
        self.logger.info(f"Sampled {len(frames)} frames for analysis")
        self.logger.debug(f"Timestamps: {extracted_timestamps}")
        
        return frames, extracted_timestamps
    
    def extract_frame_at_timestamp(self, video_path: str, 
                                   timestamp: float) -> Tuple[Optional[np.ndarray], float]:
        """
        Extract a single frame at a specific timestamp.
        
        Args:
            video_path: Path to video file
            timestamp: Timestamp in seconds
            
        Returns:
            Tuple of (frame, actual_timestamp)
        """
        cap = cv2.VideoCapture(video_path)
        if not cap.isOpened():
            return None, 0.0
        
        fps = cap.get(cv2.CAP_PROP_FPS)
        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
        
        frame_idx = int(timestamp * fps)
        frame_idx = min(max(frame_idx, 0), total_frames - 1)
        
        cap.set(cv2.CAP_PROP_POS_FRAMES, frame_idx)
        ret, frame = cap.read()
        cap.release()
        
        if ret and frame is not None and frame.size > 0:
            return frame, frame_idx / fps
        return None, 0.0
    
    def save_frames(self, frames: List[np.ndarray], 
                    prefix: str = "frame") -> List[str]:
        """
        Save frames to temporary files.
        
        Args:
            frames: List of frames to save
            prefix: Filename prefix
            
        Returns:
            List of saved file paths
        """
        paths = []
        
        for i, frame in enumerate(frames):
            path = os.path.join(self.temp_dir, f"{prefix}_{i:04d}.jpg")
            cv2.imwrite(path, frame)
            paths.append(path)
        
        self.logger.info(f"Saved {len(paths)} frames to {self.temp_dir}")
        
        return paths
    
    def load_frames(self, paths: List[str]) -> List[np.ndarray]:
        """
        Load frames from files.
        
        Args:
            paths: List of file paths
            
        Returns:
            List of loaded frames
        """
        frames = []
        
        for path in paths:
            if os.path.exists(path):
                frame = cv2.imread(path)
                if frame is not None and frame.size > 0:
                    frames.append(frame)
        
        return frames
    
    def cleanup(self, paths: List[str]):
        """
        Remove temporary frame files.
        
        Args:
            paths: List of file paths to remove
        """
        for path in paths:
            try:
                if os.path.exists(path):
                    os.remove(path)
            except Exception as e:
                self.logger.warning(f"Failed to remove {path}: {e}")
