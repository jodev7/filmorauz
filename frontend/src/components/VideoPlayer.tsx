"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import {
  Play,
  Pause,
  Volume2,
  VolumeX,
  Maximize,
  AlertTriangle,
  RefreshCw,
  Settings,
  ChevronDown,
} from "lucide-react";
import Hls from "hls.js";
import { VideoSourceType } from "@/lib/api";

interface Props {
  videoUrl: string;
  embedUrl?: string;
  sourceType?: VideoSourceType;
  title: string;
  posterUrl?: string;
}

// Detect if URL is an embed (YouTube, Vimeo, etc.)
function isEmbedUrl(url: string): boolean {
  return (
    url.includes("youtube.com") ||
    url.includes("youtu.be") ||
    url.includes("vimeo.com") ||
    url.includes("rutube.ru") ||
    url.includes("embed")
  );
}

// Convert YouTube watch URL to embed URL
function toEmbedUrl(url: string): string {
  const ytMatch = url.match(
    /(?:youtube\.com\/watch\?v=|youtu\.be\/)([a-zA-Z0-9_-]+)/
  );
  if (ytMatch) {
    return `https://www.youtube.com/embed/${ytMatch[1]}?autoplay=1&rel=0`;
  }
  return url;
}

// Restricted fallback component for when video playback is not allowed
function RestrictedFallback({ onRetry }: { onRetry?: () => void }) {
  return (
    <div className="w-full aspect-video bg-gray-900 rounded-xl flex flex-col items-center justify-center text-center p-8">
      <div className="w-16 h-16 bg-amber-500/20 rounded-full flex items-center justify-center mb-4">
        <AlertTriangle size={32} className="text-amber-500" />
      </div>
      <h3 className="text-xl font-semibold text-white mb-2">
        Video Source Restricted
      </h3>
      <p className="text-gray-400 max-w-md mb-6">
        This video source does not allow external playback. The content may only 
        be available on the original hosting platform.
      </p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="flex items-center gap-2 px-4 py-2 bg-brand-red hover:bg-orange-700 text-white rounded-lg transition-colors"
        >
          <RefreshCw size={16} />
          Try Again
        </button>
      )}
    </div>
  );
}

// Error fallback component
function ErrorFallback({ 
  message, 
  onRetry 
}: { 
  message?: string; 
  onRetry?: () => void 
}) {
  return (
    <div className="w-full aspect-video bg-gray-900 rounded-xl flex flex-col items-center justify-center text-center p-8">
      <div className="w-16 h-16 bg-red-500/20 rounded-full flex items-center justify-center mb-4">
        <AlertTriangle size={32} className="text-red-500" />
      </div>
      <h3 className="text-xl font-semibold text-white mb-2">
        Playback Error
      </h3>
      <p className="text-gray-400 max-w-md mb-6">
        {message || "Unable to load video. The source may be unavailable or blocked."}
      </p>
      {onRetry && (
        <button
          onClick={onRetry}
          className="flex items-center gap-2 px-4 py-2 bg-brand-red hover:bg-orange-700 text-white rounded-lg transition-colors"
        >
          <RefreshCw size={16} />
          Try Again
        </button>
      )}
    </div>
  );
}

// HTML5 Video player with error handling
function DirectVideoPlayer({ 
  src, 
  title, 
  poster 
}: { 
  src: string; 
  title: string; 
  poster?: string 
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);

  const handleError = () => {
    if (videoRef.current?.error) {
      const errorCode = videoRef.current.error.code;
      if (errorCode === 4) {
        // MEDIA_ERR_SRC_NOT_SUPPORTED - often 403 or blocked
        setError("This video source does not allow external playback (HTTP 403). Try using an iframe embed instead.");
      } else {
        setError("Unable to load video. The source may be unavailable.");
      }
    }
  };

  if (error) {
    return <ErrorFallback message={error} />;
  }

  return (
    <div className="relative w-full aspect-video bg-black rounded-xl overflow-hidden">
      <video
        ref={videoRef}
        src={src}
        title={title}
        controls
        autoPlay
        className="w-full h-full"
        poster={poster}
        onError={handleError}
        onLoadedData={() => setLoading(false)}
        onLoadStart={() => setLoading(true)}
      >
        Your browser does not support the video tag.
      </video>
      {loading && (
        <div className="absolute inset-0 flex items-center justify-center bg-black/50">
          <div className="w-10 h-10 border-2 border-white border-t-transparent rounded-full animate-spin" />
        </div>
      )}
    </div>
  );
}

// Iframe embed player
function IframePlayer({ src, title }: { src: string; title: string }) {
  return (
    <div className="relative w-full aspect-video bg-black rounded-xl overflow-hidden">
      <iframe
        src={src}
        title={title}
        allow="accelerometer; autoplay; clipboard-write; encrypted-media; gyroscope; picture-in-picture; fullscreen"
        allowFullScreen
        className="w-full h-full"
      />
    </div>
  );
}

// ─── Custom HLS Player ────────────────────────────────────────────────────────

const SPEEDS = [0.5, 1, 1.25, 1.5, 2];

interface QualityLevel {
  index: number; // hls.js level index, -1 = Auto
  label: string;
  height: number;
}

function formatTime(s: number): string {
  if (!isFinite(s)) return "0:00";
  const m = Math.floor(s / 60);
  const sec = Math.floor(s % 60);
  return `${m}:${sec.toString().padStart(2, "0")}`;
}

function HLSPlayer({ src, poster }: { src: string; poster?: string }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const controlsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const [buffered, setBuffered] = useState(0);
  const [error, setError] = useState<string | null>(null);

  const [qualities, setQualities] = useState<QualityLevel[]>([]);
  const [selectedQuality, setSelectedQuality] = useState(-1); // -1 = Auto
  const [selectedSpeed, setSelectedSpeed] = useState(1);
  const [showQualityMenu, setShowQualityMenu] = useState(false);
  const [showSpeedMenu, setShowSpeedMenu] = useState(false);

  // future preroll hook — insert ad logic here before play starts
  // future midroll hook — check currentTime intervals for mid-roll triggers
  // future overlay ad layer — render over player container

  const resetControlsTimer = useCallback(() => {
    setShowControls(true);
    if (controlsTimer.current) clearTimeout(controlsTimer.current);
    controlsTimer.current = setTimeout(() => setShowControls(false), 3000);
  }, []);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    setError(null);
    setQualities([]);
    setSelectedQuality(-1);

    if (Hls.isSupported()) {
      const hls = new Hls({ startLevel: -1 });
      hlsRef.current = hls;
      hls.loadSource(src);
      hls.attachMedia(video);

      hls.on(Hls.Events.MANIFEST_PARSED, (_, data) => {
        // Build deduplicated quality list sorted by height desc.
        const seen = new Set<number>();
        const levels: QualityLevel[] = [{ index: -1, label: "Auto", height: -1 }];
        data.levels
          .map((l, i) => ({ index: i, height: l.height }))
          .sort((a, b) => b.height - a.height)
          .forEach(({ index, height }) => {
            if (!seen.has(height)) {
              seen.add(height);
              levels.push({ index, label: `${height}p`, height });
            }
          });
        setQualities(levels);
      });

      hls.on(Hls.Events.ERROR, (_, data) => {
        if (data.fatal) {
          console.error("[HLS] fatal error:", data.type, data.details);
          setError("Video yuklanmadi. Sahifani yangilang.");
          hls.destroy();
        }
      });

      return () => {
        hls.destroy();
        hlsRef.current = null;
      };
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      // Native HLS (Safari)
      video.src = src;
    } else {
      setError("Brauzeringiz HLS formatini qo'llab-quvvatlamaydi.");
    }
  }, [src]);

  // Sync selected quality to hls.js
  useEffect(() => {
    if (hlsRef.current) {
      hlsRef.current.currentLevel = selectedQuality;
    }
  }, [selectedQuality]);

  // Sync speed
  useEffect(() => {
    if (videoRef.current) {
      videoRef.current.playbackRate = selectedSpeed;
    }
  }, [selectedSpeed]);

  // Space key: toggle play/pause — ignore when focus is on a text input
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      if (e.code !== "Space") return;
      const target = e.target as HTMLElement;
      if (
        target.tagName === "INPUT" ||
        target.tagName === "TEXTAREA" ||
        target.tagName === "SELECT" ||
        target.isContentEditable
      ) return;
      e.preventDefault();
      const video = videoRef.current;
      if (!video) return;
      if (video.paused) video.play();
      else video.pause();
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, []);

  // Video event listeners
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const onPlay = () => setPlaying(true);
    const onPause = () => setPlaying(false);
    const onTimeUpdate = () => {
      setCurrentTime(video.currentTime);
      if (video.buffered.length > 0) {
        setBuffered(video.buffered.end(video.buffered.length - 1));
      }
    };
    const onDurationChange = () => setDuration(video.duration);
    const onVolumeChange = () => {
      setVolume(video.volume);
      setMuted(video.muted);
    };
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("volumechange", onVolumeChange);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("timeupdate", onTimeUpdate);
      video.removeEventListener("durationchange", onDurationChange);
      video.removeEventListener("volumechange", onVolumeChange);
    };
  }, []);

  const togglePlay = () => {
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) video.play();
    else video.pause();
  };

  const seek = (e: React.ChangeEvent<HTMLInputElement>) => {
    const video = videoRef.current;
    if (!video) return;
    video.currentTime = Number(e.target.value);
  };

  const changeVolume = (e: React.ChangeEvent<HTMLInputElement>) => {
    const video = videoRef.current;
    if (!video) return;
    video.volume = Number(e.target.value);
    video.muted = false;
  };

  const toggleMute = () => {
    if (videoRef.current) videoRef.current.muted = !videoRef.current.muted;
  };

  const toggleFullscreen = () => {
    const el = containerRef.current;
    if (!el) return;
    if (!document.fullscreenElement) el.requestFullscreen();
    else document.exitFullscreen();
  };

  const qualityLabel = qualities.find((q) => q.index === selectedQuality)?.label ?? "Auto";

  if (error) {
    return (
      <div className="w-full aspect-video bg-gray-900 rounded-xl flex items-center justify-center">
        <p className="text-red-400 text-sm">{error}</p>
      </div>
    );
  }

  return (
    <div
      ref={containerRef}
      className="relative w-full aspect-video bg-black rounded-xl overflow-hidden select-none"
      onMouseMove={resetControlsTimer}
      onMouseLeave={() => setShowControls(false)}
      onMouseEnter={() => setShowControls(true)}
      onClick={() => { togglePlay(); resetControlsTimer(); }}
    >
      {/* future overlay ad layer */}

      <video
        ref={videoRef}
        className="w-full h-full"
        poster={poster}
        playsInline
      />

      {/* Controls overlay — transparent area passes clicks through to container toggle */}
      <div
        className={`absolute inset-0 flex flex-col justify-end transition-opacity duration-200 ${
          showControls ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
      >
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent pointer-events-none" />

        {/* Controls bar — stopPropagation here so only bar clicks don't toggle play */}
        <div className="relative px-3 pb-3 space-y-1.5" onClick={(e) => e.stopPropagation()}>
          {/* Progress bar */}
          <div className="relative h-1 group/progress">
            {/* buffered */}
            <div
              className="absolute top-0 left-0 h-full bg-white/20 rounded-full"
              style={{ width: duration ? `${(buffered / duration) * 100}%` : "0%" }}
            />
            <input
              type="range"
              min={0}
              max={duration || 0}
              step={0.1}
              value={currentTime}
              onChange={seek}
              className="absolute inset-0 w-full h-full opacity-0 cursor-pointer"
            />
            {/* played */}
            <div
              className="absolute top-0 left-0 h-full bg-brand-red rounded-full pointer-events-none"
              style={{ width: duration ? `${(currentTime / duration) * 100}%` : "0%" }}
            />
          </div>

          {/* Buttons row */}
          <div className="flex items-center gap-3">
            {/* Play/Pause */}
            <button onClick={togglePlay} className="text-white hover:text-brand-red transition-colors">
              {playing ? <Pause size={20} fill="white" /> : <Play size={20} fill="white" />}
            </button>

            {/* Volume */}
            <div className="flex items-center gap-1.5">
              <button onClick={toggleMute} className="text-white hover:text-brand-red transition-colors">
                {muted || volume === 0 ? <VolumeX size={18} /> : <Volume2 size={18} />}
              </button>
              <input
                type="range"
                min={0}
                max={1}
                step={0.05}
                value={muted ? 0 : volume}
                onChange={changeVolume}
                className="w-16 h-1 accent-brand-red cursor-pointer"
              />
            </div>

            {/* Time */}
            <span className="text-white text-xs tabular-nums">
              {formatTime(currentTime)} / {formatTime(duration)}
            </span>

            <div className="flex-1" />

            {/* Speed selector */}
            <div className="relative">
              <button
                onClick={() => { setShowSpeedMenu((v) => !v); setShowQualityMenu(false); }}
                className="flex items-center gap-1 text-white text-xs hover:text-brand-red transition-colors"
              >
                <Settings size={14} />
                {selectedSpeed}x
                <ChevronDown size={12} />
              </button>
              {showSpeedMenu && (
                <div className="absolute bottom-full right-0 mb-2 bg-black/90 border border-white/10 rounded-lg overflow-hidden text-xs min-w-[80px]">
                  {SPEEDS.map((s) => (
                    <button
                      key={s}
                      onClick={() => { setSelectedSpeed(s); setShowSpeedMenu(false); }}
                      className={`block w-full px-3 py-1.5 text-left hover:bg-white/10 transition-colors ${
                        s === selectedSpeed ? "text-brand-red font-semibold" : "text-white"
                      }`}
                    >
                      {s}x
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Quality selector */}
            {qualities.length > 1 && (
              <div className="relative">
                <button
                  onClick={() => { setShowQualityMenu((v) => !v); setShowSpeedMenu(false); }}
                  className="flex items-center gap-1 text-white text-xs hover:text-brand-red transition-colors"
                >
                  {qualityLabel}
                  <ChevronDown size={12} />
                </button>
                {showQualityMenu && (
                  <div className="absolute bottom-full right-0 mb-2 bg-black/90 border border-white/10 rounded-lg overflow-hidden text-xs min-w-[80px]">
                    {qualities.map((q) => (
                      <button
                        key={q.index}
                        onClick={() => { setSelectedQuality(q.index); setShowQualityMenu(false); }}
                        className={`block w-full px-3 py-1.5 text-left hover:bg-white/10 transition-colors ${
                          q.index === selectedQuality ? "text-brand-red font-semibold" : "text-white"
                        }`}
                      >
                        {q.label}
                      </button>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Fullscreen */}
            <button onClick={toggleFullscreen} className="text-white hover:text-brand-red transition-colors">
              <Maximize size={18} />
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}

export default function VideoPlayer({ 
  videoUrl, 
  embedUrl, 
  sourceType = "iframe_embed",
  title, 
  posterUrl 
}: Props) {
  const [started, setStarted] = useState(false);
  const [hasPlaybackError, setHasPlaybackError] = useState(false);

  // Determine which URL to use based on source type
  const getEffectiveUrl = (): string | null => {
    if (sourceType === "iframe_embed" && embedUrl) {
      return embedUrl;
    }
    if (sourceType === "iframe_embed" && videoUrl) {
      // Fallback to videoUrl if no embedUrl provided
      if (isEmbedUrl(videoUrl)) {
        return toEmbedUrl(videoUrl);
      }
      return null;
    }
    if (sourceType === "direct_mp4" || sourceType === "direct_hls") {
      return videoUrl;
    }
    return null;
  };

  const effectiveUrl = getEffectiveUrl();

  const handlePlay = () => {
    setStarted(true);
  };

  const handleRetry = () => {
    setHasPlaybackError(false);
    setStarted(false);
  };

  // External restricted - show restricted message
  if (sourceType === "external_restricted") {
    return <RestrictedFallback onRetry={handleRetry} />;
  }

  // No valid URL provided
  if (!effectiveUrl) {
    return (
      <ErrorFallback 
        message="No video source available. Please check the video configuration." 
      />
    );
  }

  // Not started - show poster/thumbnail with play button
  if (!started && !hasPlaybackError) {
    return (
      <div
        className="relative w-full aspect-video bg-black rounded-xl overflow-hidden cursor-pointer group"
        onClick={handlePlay}
      >
        {posterUrl && (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            src={posterUrl}
            alt={title}
            className="w-full h-full object-cover opacity-60 group-hover:opacity-50 transition-opacity"
          />
        )}
        <div className="absolute inset-0 flex items-center justify-center">
          <div className="w-20 h-20 bg-brand-red rounded-full flex items-center justify-center shadow-2xl group-hover:scale-110 transition-transform">
            <Play size={32} className="text-white ml-2" fill="white" />
          </div>
        </div>
        <div className="absolute bottom-4 left-4 right-4 text-center">
          <p className="text-white font-display text-xl tracking-wide drop-shadow-lg">
            {title}
          </p>
          <p className="text-gray-300 text-sm mt-1">Click to play</p>
        </div>
      </div>
    );
  }

  // Has playback error
  if (hasPlaybackError) {
    return <ErrorFallback onRetry={handleRetry} />;
  }

  // Render based on source type
  switch (sourceType) {
    case "iframe_embed":
      return <IframePlayer src={effectiveUrl} title={title} />;
    
    case "direct_mp4":
      return (
        <DirectVideoPlayer 
          src={effectiveUrl} 
          title={title} 
          poster={posterUrl} 
        />
      );
    
    case "direct_hls":
      return <HLSPlayer src={effectiveUrl} poster={posterUrl} />;
    
    default:
      return <IframePlayer src={effectiveUrl} title={title} />;
  }
}
