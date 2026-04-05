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

import copy
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
from .inpaint import Inpainter, create_inpainter, OpenCVInpainter


logger = logging.getLogger(__name__)


class _RegionTracker:
    """
    Lightweight bounding-box tracker using sparse Lucas-Kanade optical flow.

    Tracks 5 anchor points per region (4 corners + centre) between full
    re-detection cycles. On each frame it estimates both the translation
    (median shift of inlier points) and a uniform scale change (median ratio
    of all pairwise distances before/after flow), then returns updated
    RegionCandidates with the new position and size.

    Scale is clamped to ±15 % per step to suppress noise; the bounding box
    is resized around its new centre after translation is applied.

    Falls back to the last known position when flow tracking fails (too few
    inliers, scene cut, black frame, etc.).

    Usage::
        tracker = _RegionTracker()
        tracker.init(gray_frame, regions)       # called after every re-detection
        regions = tracker.update(next_gray)     # called on every tracking frame
    """

    _LK_PARAMS = dict(
        winSize=(21, 21),
        maxLevel=2,
        criteria=(cv2.TERM_CRITERIA_EPS | cv2.TERM_CRITERIA_COUNT, 10, 0.03),
    )
    _MIN_INLIERS = 2  # minimum tracked points needed to trust the shift

    def __init__(self) -> None:
        self._prev_gray: Optional[np.ndarray] = None
        self._pts: List[Optional[np.ndarray]] = []   # shape (N,1,2) float32 per region
        self._regions: List[RegionCandidate] = []
        self._initialized = False

    @property
    def initialized(self) -> bool:
        return self._initialized

    def init(self, gray: np.ndarray, regions: List[RegionCandidate]) -> None:
        """Seed tracker from a fresh detection result."""
        self._prev_gray = gray.copy()
        self._regions = list(regions)
        self._pts = []
        for r in regions:
            cx = float(r.x + r.width // 2)
            cy = float(r.y + r.height // 2)
            pts = np.array([
                [[float(r.x),              float(r.y)]],
                [[float(r.x + r.width),    float(r.y)]],
                [[float(r.x),              float(r.y + r.height)]],
                [[float(r.x + r.width),    float(r.y + r.height)]],
                [[cx,                       cy]],
            ], dtype=np.float32)
            self._pts.append(pts)
        self._initialized = True

    def update(self, gray: np.ndarray) -> List[RegionCandidate]:
        """
        Track all regions to the new frame.

        Returns updated RegionCandidates (positions shifted by estimated
        translation). Regions where flow fails are returned at their last
        known position unchanged.
        """
        if not self._initialized or self._prev_gray is None:
            return self._regions

        updated_regions: List[RegionCandidate] = []
        updated_pts: List[Optional[np.ndarray]] = []

        for pts, region in zip(self._pts, self._regions):
            if pts is None or len(pts) < self._MIN_INLIERS:
                updated_regions.append(region)
                updated_pts.append(pts)
                continue

            next_pts, status, _ = cv2.calcOpticalFlowPyrLK(
                self._prev_gray, gray, pts, None, **self._LK_PARAMS
            )

            good_mask = status.flatten() == 1
            good_new = next_pts[good_mask].reshape(-1, 2)
            good_old = pts[good_mask].reshape(-1, 2)

            if len(good_new) < self._MIN_INLIERS:
                # Flow lost — keep last position
                updated_regions.append(region)
                updated_pts.append(pts)
                continue

            # ── Translation ───────────────────────────────────────────────
            # Median shift across all inlier points is robust to 1-2 outliers.
            shift = np.median(good_new - good_old, axis=0)
            dx = int(round(float(shift[0])))
            dy = int(round(float(shift[1])))

            # ── Scale ─────────────────────────────────────────────────────
            # Estimate uniform scale from all pairwise distances between the
            # N inlier points. With 5 anchors → up to 10 pairs — enough for
            # a stable median even when 1-2 pairs are degenerate (collinear).
            scale = 1.0
            n = len(good_new)
            if n >= 2:
                ratios = []
                for i in range(n):
                    for j in range(i + 1, n):
                        d_old = float(np.linalg.norm(good_old[i] - good_old[j]))
                        d_new = float(np.linalg.norm(good_new[i] - good_new[j]))
                        if d_old > 1.0:          # skip near-zero baselines
                            ratios.append(d_new / d_old)
                if ratios:
                    raw = float(np.median(ratios))
                    # Clamp: allow at most ±15 % change per tracking step to
                    # suppress noise; genuine watermark scale changes are slow.
                    scale = max(0.85, min(1.15, raw))

            # ── Apply translation + scale ─────────────────────────────────
            moved = copy.copy(region)
            # Translate first, then scale around the new centre.
            cx = (region.x + region.width  / 2) + dx
            cy = (region.y + region.height / 2) + dy
            new_w = max(10, int(round(region.width  * scale)))
            new_h = max(10, int(round(region.height * scale)))
            moved.x      = max(0, int(round(cx - new_w / 2)))
            moved.y      = max(0, int(round(cy - new_h / 2)))
            moved.width  = new_w
            moved.height = new_h
            updated_regions.append(moved)
            updated_pts.append(next_pts[good_mask].reshape(-1, 1, 2))

        self._prev_gray = gray.copy()
        self._regions = updated_regions
        self._pts = updated_pts
        return updated_regions


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
            
            # Stage 4+5: Per-frame adaptive inpainting + ffmpeg encoding.
            # Regions from sampled detection seed the initial mask; detection
            # re-runs every N frames so the mask adapts to moving watermarks.
            mode_stage = PipelineStage.INPAINTING_PRO if self.config.use_pro_mode else PipelineStage.INPAINTING_FAST
            self._log_stage(mode_stage)
            self._log_stage(PipelineStage.REBUILDING)

            success = self._process_video_stream(
                input_path, output_path, detection_result.regions, video_info
            )

            if not success or not os.path.exists(output_path):
                self.logger.error("[WATERMARK] Video stream processing failed, using original")
                return WatermarkRemovalResult(
                    success=True,
                    input_path=input_path,
                    output_path=input_path,
                    watermark_detected=True,
                    watermark_removed=False,
                    fallback_used=True,
                    warning="Video stream processing failed, using original",
                    regions=detection_result.regions,
                    stages=self._stages,
                    total_time=time.time() - start_time
                )

            # Validate output
            if not self._validate_video(output_path):
                self.logger.error("[WATERMARK] Output video is invalid")
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
    
    def _build_frame_mask(self, regions: List[RegionCandidate],
                          frame_shape: tuple) -> np.ndarray:
        """
        Build a quality-optimised binary mask from a list of RegionCandidates.

        Two-stage adaptive expansion per region:

        1. **Adaptive padding** — each bbox is expanded by
           ``max(5, min(20, int(shorter_side * 0.15)))`` pixels before being
           stamped into the mask.  This keeps small logos tight (≈5–8 px halo)
           while giving larger banners more room (≈12–20 px) to cover soft
           edges, glow, and anti-aliasing without eating into background.

        2. **Adaptive dilation** — a single morphological dilation pass uses a
           kernel sized to the *largest* region in the batch:
           - area ≥ 10 000 px² (≈100×100)  → 7×7 kernel
           - area ≥  2 500 px² (≈ 50× 50)  → 5×5 kernel
           - smaller                         → 3×3 kernel
           One pass is enough because the per-region padding already provides
           the bulk of the expansion; this pass smooths jagged contour edges.

        Returns a zeroed mask (no inpainting) when ``regions`` is empty.
        """
        h, w = frame_shape[:2]
        mask = np.zeros((h, w), dtype=np.uint8)

        for r in regions:
            # 15 % of the shorter side keeps the halo proportional to the logo.
            pad = max(5, min(20, int(min(r.width, r.height) * 0.15)))
            x1 = max(0, r.x - pad)
            y1 = max(0, r.y - pad)
            x2 = min(w, r.x + r.width  + pad)
            y2 = min(h, r.y + r.height + pad)
            mask[y1:y2, x1:x2] = 255

        if np.any(mask):
            max_area = max((r.width * r.height for r in regions), default=0)
            if max_area >= 10_000:
                ksize = 7
            elif max_area >= 2_500:
                ksize = 5
            else:
                ksize = 3
            kernel = np.ones((ksize, ksize), np.uint8)
            mask = cv2.dilate(mask, kernel, iterations=1)

        return mask

    def _process_video_stream(self, input_path: str, output_path: str,
                              initial_regions: List[RegionCandidate],
                              video_info: VideoInfo) -> bool:
        """
        Process the entire video frame-by-frame with per-frame adaptive masks.

        Strategy:
        - Regions detected on sampled frames seed the initial mask.
        - Every REDETECT_EVERY frames, CVWatermarkDetector re-runs on the live
          frame to pick up watermark movement, appearance, or disappearance.
        - Each re-detection updates `current_regions` and rebuilds the mask.
        - When re-detection finds nothing (black screen, scene cut), the previous
          mask is kept so momentary non-detection does not leave artifacts.
        - All N watermark regions from each detection are handled simultaneously.
        - Audio is merged from the original file after encoding.

        Args:
            input_path: Original video path
            output_path: Output video path (with audio)
            initial_regions: Watermark regions from sampled-frame detection
            video_info: VideoInfo with fps, dimensions, etc.

        Returns:
            True if output video was successfully created
        """
        import subprocess
        from .detector import CVWatermarkDetector

        # Full re-detection interval. Between re-detections the _RegionTracker
        # shifts bounding boxes via sparse optical flow — zero-lag, cheap.
        # 15 ≈ 0.5 s at 30 fps: responsive enough, rarely fires unnecessarily.
        REDETECT_EVERY = 15

        cap = cv2.VideoCapture(input_path)
        if not cap.isOpened():
            self.logger.error(f"[WATERMARK] Cannot open video: {input_path}")
            return False

        total_frames = int(cap.get(cv2.CAP_PROP_FRAME_COUNT))
        width = int(cap.get(cv2.CAP_PROP_FRAME_WIDTH))
        height = int(cap.get(cv2.CAP_PROP_FRAME_HEIGHT))
        fps = video_info.fps if video_info.fps > 0 else cap.get(cv2.CAP_PROP_FPS)
        frame_shape = (height, width, 3)

        self.logger.info(
            f"[WATERMARK] Stream processing: {total_frames} frames, "
            f"{width}x{height} @ {fps:.3f}fps, "
            f"redetect every frame"
        )

        # Seed state from sampled-frame detection
        current_regions: List[RegionCandidate] = list(initial_regions)
        current_mask = self._build_frame_mask(current_regions, frame_shape)

        self.logger.info(
            f"[WATERMARK] Initial mask: {len(current_regions)} region(s), "
            f"{int(np.sum(current_mask > 0))} px masked"
        )

        cv_detector = CVWatermarkDetector(self.config)
        tracker = _RegionTracker()
        mask_updates = 0
        track_updates = 0

        # Write inpainted frames to a no-audio temp file via ffmpeg pipe.
        temp_video = output_path + ".noaudio.mp4"
        ffmpeg_cmd = [
            "ffmpeg", "-y",
            "-f", "rawvideo",
            "-vcodec", "rawvideo",
            "-s", f"{width}x{height}",
            "-pix_fmt", "bgr24",
            "-r", str(fps),
            "-i", "pipe:0",
            "-c:v", "libx264",
            "-preset", "fast",
            "-crf", "18",
            "-pix_fmt", "yuv420p",
            temp_video,
        ]

        inpainter = OpenCVInpainter()
        frame_count = 0

        try:
            proc = subprocess.Popen(
                ffmpeg_cmd,
                stdin=subprocess.PIPE,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
            )

            while True:
                ret, frame = cap.read()
                if not ret:
                    break

                # ── Per-frame mask update ─────────────────────────────────────
                gray = cv2.cvtColor(frame, cv2.COLOR_BGR2GRAY)

                is_redetect = (frame_count == 0) or (frame_count % REDETECT_EVERY == 0)

                if is_redetect:
                    # Full corner detection to catch moved / newly appeared watermarks
                    ts = frame_count / fps if fps > 0 else 0.0
                    new_regions = cv_detector.detect_in_frame(frame, timestamp=ts)
                    new_regions = [
                        r for r in new_regions
                        if r.confidence >= self.config.confidence_threshold
                    ]

                    if new_regions:
                        prev_count = len(current_regions)
                        current_regions = new_regions
                        current_mask = self._build_frame_mask(new_regions, frame_shape)
                        tracker.init(gray, new_regions)   # reset tracker anchor
                        mask_updates += 1

                        if mask_updates <= 5 or mask_updates % 100 == 0:
                            self.logger.info(
                                f"[WATERMARK] Frame {frame_count}: re-detected "
                                f"({prev_count}→{len(new_regions)} region(s), "
                                f"{int(np.sum(current_mask > 0))} px, "
                                f"update #{mask_updates})"
                            )
                    else:
                        # No fresh detection — fall through to tracking below
                        if tracker.initialized:
                            current_regions = tracker.update(gray)
                            current_mask = self._build_frame_mask(current_regions, frame_shape)
                            track_updates += 1
                        # else: no detection ever — keep zero mask (pass-through)
                        self.logger.debug(
                            f"[WATERMARK] Frame {frame_count}: re-detect found nothing, "
                            f"tracking ({len(current_regions)} region(s))"
                        )
                else:
                    # Tracking frame — shift bounding boxes via optical flow
                    if tracker.initialized:
                        current_regions = tracker.update(gray)
                        current_mask = self._build_frame_mask(current_regions, frame_shape)
                        track_updates += 1
                    # else: initial regions not yet available (shouldn't happen)
                # ─────────────────────────────────────────────────────────────

                if np.any(current_mask):
                    inpainted = inpainter.inpaint(frame, current_mask)
                else:
                    inpainted = frame  # no watermark region active, pass through

                proc.stdin.write(inpainted.tobytes())
                frame_count += 1

                if frame_count % 500 == 0:
                    pct = (frame_count / total_frames * 100) if total_frames > 0 else 0
                    self.logger.info(
                        f"[WATERMARK] Progress: {frame_count}/{total_frames} frames "
                        f"({pct:.0f}%), {mask_updates} mask update(s) so far"
                    )

            proc.stdin.close()
            _, ff_stderr = proc.communicate()

            if proc.returncode != 0:
                self.logger.error(
                    f"[WATERMARK] ffmpeg encoding failed (rc={proc.returncode}): "
                    f"{ff_stderr.decode('utf-8', errors='replace')[-500:]}"
                )
                return False

        except Exception as e:
            self.logger.error(f"[WATERMARK] Stream processing error: {e}", exc_info=True)
            return False
        finally:
            cap.release()
            if os.path.exists(temp_video) and frame_count == 0:
                try:
                    os.remove(temp_video)
                except Exception:
                    pass

        self.logger.info(
            f"[WATERMARK] Encoded {frame_count} frames — "
            f"{mask_updates} re-detect update(s), "
            f"{track_updates} tracked frame(s)"
        )

        # Merge audio from original into the inpainted video.
        if self._merge_audio(temp_video, output_path, input_path):
            try:
                os.remove(temp_video)
            except Exception:
                pass
        elif os.path.exists(temp_video):
            self.logger.warning("[WATERMARK] Audio merge failed, using video-only output")
            try:
                os.rename(temp_video, output_path)
            except Exception as e:
                self.logger.error(f"[WATERMARK] Failed to rename temp video: {e}")
                return False

        return os.path.exists(output_path)

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
