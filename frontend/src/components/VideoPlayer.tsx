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
  onPlayIntent?: () => void;
  forceStart?: boolean;
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
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = Math.floor(s % 60);
  if (h > 0) {
    return `${h}:${m.toString().padStart(2, "0")}:${sec.toString().padStart(2, "0")}`;
  }
  return `${m}:${sec.toString().padStart(2, "0")}`;
}

function HLSPlayer({ src, poster, autoPlay: shouldAutoPlay }: { src: string; poster?: string; autoPlay?: boolean }) {
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

    console.log("[HLSPlayer] initializing with src:", src);

    if (!src) {
      setError("Video manbasi mavjud emas.");
      return;
    }

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
        if (shouldAutoPlay) {
          video.play().catch(() => {});
        }
      });

      // Track network error retries so we don't loop forever on a dead URL.
      // hls.js fires repeated fatal NETWORK_ERROR events on a 404/CORS manifest;
      // we give it two automatic startLoad() attempts, then surface a real message.
      let networkRetries = 0;
      const MAX_NETWORK_RETRIES = 2;

      hls.on(Hls.Events.ERROR, (_, data) => {
        console.error("[HLSPlayer] error event:", {
          type: data.type,
          details: data.details,
          fatal: data.fatal,
          url: (data as unknown as { url?: string }).url,
          responseCode: (data as unknown as { response?: { code?: number } }).response?.code,
          src,
        });
        if (!data.fatal) return;

        const respCode = (data as unknown as { response?: { code?: number } }).response?.code;

        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            if (networkRetries < MAX_NETWORK_RETRIES) {
              networkRetries += 1;
              console.warn(`[HLSPlayer] network error — retry ${networkRetries}/${MAX_NETWORK_RETRIES}`);
              hls.startLoad();
              return;
            }
            if (respCode === 404) {
              setError("Video fayli topilmadi (404).");
            } else if (respCode === 403) {
              setError("Video kirish taqiqlangan (403).");
            } else if (data.details && (data.details as string).includes("CORS")) {
              setError("Videoyuklanmadi (CORS xatolik). Site admin bilan bog'laning.");
            } else {
              setError("Video yuklanmadi (tarmoq xatosi).");
            }
            hls.destroy();
            break;
          case Hls.ErrorTypes.MEDIA_ERROR:
            console.warn("[HLSPlayer] media error — attempting recover");
            hls.recoverMediaError();
            break;
          default:
            setError("Video yuklanmadi. Sahifani yangilang.");
            hls.destroy();
            break;
        }
      });

      return () => {
        hls.destroy();
        hlsRef.current = null;
      };
    } else if (video.canPlayType("application/vnd.apple.mpegurl")) {
      // Native HLS (Safari)
      video.src = src;
      const onVideoError = () => {
        const code = video.error?.code;
        console.error("[HLSPlayer] native video error:", code, video.error?.message, "src:", src);
        if (code === 4) {
          setError("Video manbasi qo'llab-quvvatlanmaydi yoki topilmadi.");
        } else {
          setError("Video yuklanmadi. Sahifani yangilang.");
        }
      };
      video.addEventListener("error", onVideoError);
      return () => {
        video.removeEventListener("error", onVideoError);
        video.removeAttribute("src");
        video.load();
      };
    } else {
      setError("Brauzeringiz HLS formatini qo'llab-quvvatlamaydi.");
    }
  }, [src]);

  const handleQualityChange = useCallback((index: number) => {
    setSelectedQuality(index);
    setShowQualityMenu(false);
    const hls = hlsRef.current;
    const video = videoRef.current;
    if (!hls) return;
    hls.currentLevel = index;
    // Seek to current position to flush buffered old-quality data and reload at new level
    if (video) {
      const t = video.currentTime;
      video.currentTime = t;
    }
  }, []);

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

  const isMobile = typeof window !== "undefined" && /Android|webOS|iPhone|iPad|iPod|BlackBerry|IEMobile|Opera Mini/i.test(navigator.userAgent);

  const toggleFullscreen = () => {
    const video = videoRef.current;
    const container = containerRef.current;
    if (!container && !video) return;
    
    // iOS Safari: use video element's native fullscreen
    if (video) {
      const videoEl = video as HTMLVideoElement & {
        webkitEnterFullscreen?: () => void;
        webkitExitFullscreen?: () => void;
      };
      if (document.fullscreenElement || (document as Document & { webkitFullscreenElement?: Element }).webkitFullscreenElement) {
        // Exit fullscreen
        if (document.exitFullscreen) {
          document.exitFullscreen().catch(() => {});
        }
      } else {
        // Try video element's webkit fullscreen first (works on iOS Safari)
        if (videoEl.webkitEnterFullscreen) {
          videoEl.webkitEnterFullscreen();
          return;
        }
        // Try container fullscreen
        const el = container as HTMLDivElement & { webkitRequestFullscreen?: () => Promise<void> };
        if (el.requestFullscreen) {
          el.requestFullscreen().catch(() => {});
        } else if (el.webkitRequestFullscreen) {
          el.webkitRequestFullscreen().catch(() => {});
        }
      }
    }
  };

  // Check if fullscreen is active (handles webkit and iOS video fullscreen)
  const isFullscreenActive = () => {
    const video = videoRef.current;
    const doc = document as Document & { webkitFullscreenElement?: Element; webkitIsFullscreen?: boolean };
    const videoEl = video as HTMLVideoElement & { webkitDisplayingFullscreen?: boolean };
    return !!(
      document.fullscreenElement ||
      doc.webkitFullscreenElement ||
      document.fullscreen ||
      doc.webkitIsFullscreen ||
      videoEl?.webkitDisplayingFullscreen
    );
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
            {/* base track — full width always visible */}
            <div className="absolute inset-0 bg-white/10 rounded-full" />
            {/* buffered */}
            <div
              className="absolute top-0 left-0 h-full bg-white/25 rounded-full pointer-events-none"
              style={{ width: duration ? `${Math.min((buffered / duration) * 100, 100)}%` : "0%" }}
            />
            {/* played */}
            <div
              className="absolute top-0 left-0 h-full bg-brand-red rounded-full pointer-events-none"
              style={{ width: duration ? `${Math.min((currentTime / duration) * 100, 100)}%` : "0%" }}
            />
            {/* invisible range input — on top for interaction */}
            <input
              type="range"
              min={0}
              max={duration || 0}
              step={0.1}
              value={currentTime}
              onChange={seek}
              className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-10"
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
              {/* Hide volume slider on mobile - browsers don't support real volume control */}
              {!isMobile && (
                <input
                  type="range"
                  min={0}
                  max={1}
                  step={0.05}
                  value={muted ? 0 : volume}
                  onChange={changeVolume}
                  className="w-16 h-1 accent-brand-red cursor-pointer hidden sm:block"
                />
              )}
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
                        onClick={() => handleQualityChange(q.index)}
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
  posterUrl,
  onPlayIntent,
  forceStart,
}: Props) {
  const [started, setStarted] = useState(false);
  const [hasPlaybackError, setHasPlaybackError] = useState(false);

  useEffect(() => {
    if (forceStart && !started) setStarted(true);
  }, [forceStart, started]);

  // If the backend hasn't set a canonical source_type but the video_url looks
  // like an HLS master playlist, treat it as direct_hls so the HLS player is used.
  // This protects against legacy/ingestion records that missed source_type mapping.
  const looksLikeHls = (url: string): boolean =>
    /\.m3u8($|\?)/i.test(url) || url.includes("/master.m3u8");

  const looksLikeMp4 = (url: string): boolean => /\.mp4($|\?)/i.test(url);

  let effectiveSourceType: VideoSourceType = sourceType;
  if (
    (sourceType === "iframe_embed" || sourceType === ("ingestion" as VideoSourceType)) &&
    videoUrl &&
    !isEmbedUrl(videoUrl)
  ) {
    if (looksLikeHls(videoUrl)) {
      effectiveSourceType = "direct_hls";
    } else if (looksLikeMp4(videoUrl)) {
      effectiveSourceType = "direct_mp4";
    }
  }

  // Determine which URL to use based on source type
  const getEffectiveUrl = (): string | null => {
    if (effectiveSourceType === "iframe_embed" && embedUrl) {
      return embedUrl;
    }
    if (effectiveSourceType === "iframe_embed" && videoUrl) {
      // Fallback to videoUrl if no embedUrl provided
      if (isEmbedUrl(videoUrl)) {
        return toEmbedUrl(videoUrl);
      }
      return null;
    }
    if (effectiveSourceType === "direct_mp4" || effectiveSourceType === "direct_hls") {
      return videoUrl;
    }
    return null;
  };

  const effectiveUrl = getEffectiveUrl();

  useEffect(() => {
    console.log("[VideoPlayer] source fields:", {
      sourceType,
      effectiveSourceType,
      videoUrl,
      embedUrl,
      effectiveUrl,
    });
  }, [sourceType, effectiveSourceType, videoUrl, embedUrl, effectiveUrl]);

  const handlePlay = () => {
    if (onPlayIntent) {
      onPlayIntent();
    } else {
      setStarted(true);
    }
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
  switch (effectiveSourceType) {
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
      return <HLSPlayer src={effectiveUrl} poster={posterUrl} autoPlay={forceStart} />;

    default:
      return <IframePlayer src={effectiveUrl} title={title} />;
  }
}
