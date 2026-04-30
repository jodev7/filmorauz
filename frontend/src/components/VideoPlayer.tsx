"use client";

import { useState, useEffect, useRef, useCallback } from "react";
import Link from "next/link";
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
  Crown,
  Lock,
  RotateCcw,
  RotateCw,
  List,
} from "lucide-react";
import Hls from "hls.js";
import { VideoSourceType, Ad, getAdsByPlacement, recordAdImpression } from "@/lib/api";
import { PremiumBadge, isUserPremium } from "@/components/PremiumComponents";
import { useAuth } from "@/lib/auth-context";
import { logger } from "@/lib/logger";

const AD_INTERVAL_SECONDS = 600; // 10 minutes
const AD_DEFAULT_DURATION = 15;
const AD_MAX_PER_BREAK = 2;

function extractAdVideoUrl(ad: Ad): string | null {
  const candidates: Array<[string | undefined, string | undefined]> = [
    [ad.player_overlay_media_url, ad.player_overlay_media_type],
    [ad.banner_media_url, ad.banner_media_type],
    [ad.inline_media_url, ad.inline_media_type],
    [ad.telegram_media_url, ad.telegram_media_type],
  ];
  for (const [url, type] of candidates) {
    if (url && type === "video") return url;
  }
  return null;
}

interface Props {
  videoUrl: string;
  premiumStreamUrl?: string;
  embedUrl?: string;
  sourceType?: VideoSourceType;
  title: string;
  posterUrl?: string;
  onPlayIntent?: () => void;
  forceStart?: boolean;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onEnded?: () => void;
  // New UI props (all optional — older callers keep working unchanged)
  headerTitle?: string;
  headerSubtitle?: string;
  seriesButtonUrl?: string;
  seriesButtonLabel?: string;
  previewImageUrl?: string;
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
  poster,
  onTimeUpdate,
  onEnded,
}: { 
  src: string; 
  title: string; 
  poster?: string;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onEnded?: () => void;
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
        onTimeUpdate={() => {
          if (!videoRef.current || !onTimeUpdate) return;
          onTimeUpdate(videoRef.current.currentTime, videoRef.current.duration || 0);
        }}
        onEnded={onEnded}
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
const SEEK_STEPS = [5, 10, 15, 30];
const SEEK_STEP_STORAGE_KEY = "filmorauz:player:seekStep";

function loadSeekStep(): number {
  if (typeof window === "undefined") return 10;
  const v = Number(window.localStorage.getItem(SEEK_STEP_STORAGE_KEY));
  return SEEK_STEPS.includes(v) ? v : 10;
}

interface QualityLevel {
  index: number; // hls.js level index, -1 = Auto
  label: string;
  height: number;
  locked?: boolean;
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

function HLSPlayer({
  src,
  poster,
  autoPlay: shouldAutoPlay,
  isPremiumUser,
  onTimeUpdate,
  onEnded,
  headerTitle,
  headerSubtitle,
  seriesButtonUrl,
  seriesButtonLabel,
  previewImageUrl,
}: {
  src: string;
  poster?: string;
  autoPlay?: boolean;
  isPremiumUser: boolean;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onEnded?: () => void;
  headerTitle?: string;
  headerSubtitle?: string;
  seriesButtonUrl?: string;
  seriesButtonLabel?: string;
  previewImageUrl?: string;
}) {
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
  const [showPremiumPrompt, setShowPremiumPrompt] = useState(false);
  const [seekStep, setSeekStep] = useState(10);
  const [showSeekStepMenu, setShowSeekStepMenu] = useState(false);

  // Progress hover preview
  const progressContainerRef = useRef<HTMLDivElement>(null);
  const [hoverTime, setHoverTime] = useState<number | null>(null);
  const [hoverX, setHoverX] = useState(0);
  const [progressWidth, setProgressWidth] = useState(0);

  // In-player ad state
  const adVideoRef = useRef<HTMLVideoElement>(null);
  const [adQueue, setAdQueue] = useState<Array<{ ad: Ad; url: string }>>([]);
  const [adIndex, setAdIndex] = useState(0);
  const [adRemaining, setAdRemaining] = useState(AD_DEFAULT_DURATION);
  const [adActive, setAdActive] = useState(false);
  const [adFetching, setAdFetching] = useState(false);
  const nextAdAtRef = useRef<number | null>(null);
  const adInitializedRef = useRef(false);
  const resumeTimeRef = useRef(0);

  useEffect(() => {
    setSeekStep(loadSeekStep());
  }, []);

  const seekBy = useCallback((delta: number) => {
    const video = videoRef.current;
    if (!video) return;
    const dur = video.duration;
    const next = video.currentTime + delta;
    const clamped = Math.max(0, isFinite(dur) && dur > 0 ? Math.min(next, dur) : next);
    video.currentTime = clamped;
  }, []);

  const handleSeekStepChange = useCallback((step: number) => {
    setSeekStep(step);
    setShowSeekStepMenu(false);
    if (typeof window !== "undefined") {
      window.localStorage.setItem(SEEK_STEP_STORAGE_KEY, String(step));
    }
  }, []);

  const triggerAdBreak = useCallback(async () => {
    if (isPremiumUser || adActive || adFetching) return;
    const video = videoRef.current;
    if (!video) return;
    setAdFetching(true);
    try {
      const ads = await getAdsByPlacement("player");
      const queue = ads
        .map((ad) => {
          const url = extractAdVideoUrl(ad);
          return url ? { ad, url } : null;
        })
        .filter((x): x is { ad: Ad; url: string } => x !== null)
        .slice(0, AD_MAX_PER_BREAK);
      if (queue.length === 0) {
        // no ads — push next window forward so we don't spam refetch
        nextAdAtRef.current = video.currentTime + AD_INTERVAL_SECONDS;
        return;
      }
      resumeTimeRef.current = video.currentTime;
      video.pause();
      setAdQueue(queue);
      setAdIndex(0);
      setAdRemaining(AD_DEFAULT_DURATION);
      setAdActive(true);
      queue.forEach((item) => recordAdImpression(item.ad.id).catch(() => {}));
    } finally {
      setAdFetching(false);
    }
  }, [isPremiumUser, adActive, adFetching]);

  const finishAdBreak = useCallback(() => {
    setAdActive(false);
    setAdQueue([]);
    setAdIndex(0);
    const video = videoRef.current;
    if (video) {
      nextAdAtRef.current = video.currentTime + AD_INTERVAL_SECONDS;
      video.play().catch(() => {});
    }
  }, []);

  const resetControlsTimer = useCallback(() => {
    setShowControls(true);
    if (controlsTimer.current) clearTimeout(controlsTimer.current);
    controlsTimer.current = setTimeout(() => {
      // Keep controls visible while paused — only auto-hide during playback.
      const v = videoRef.current;
      if (v && !v.paused) setShowControls(false);
    }, 3000);
  }, []);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    setError(null);
    setQualities([]);
    setSelectedQuality(-1);
    setAdActive(false);
    setAdQueue([]);
    setAdIndex(0);
    nextAdAtRef.current = null;
    adInitializedRef.current = false;

    logger.debug("[HLSPlayer] initializing");

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
        let maxFreeLevelIndex = -1;
        let maxFreeLevelHeight = -1;
        data.levels
          .map((l, i) => ({ index: i, height: l.height }))
          .sort((a, b) => b.height - a.height)
          .forEach(({ index, height }) => {
            if (!isPremiumUser && height <= 480 && height > maxFreeLevelHeight) {
              maxFreeLevelHeight = height;
              maxFreeLevelIndex = index;
            }
            if (!seen.has(height)) {
              seen.add(height);
              levels.push({
                index,
                label: `${height}p`,
                height,
                locked: !isPremiumUser && height > 480,
              });
            }
          });
        if (!isPremiumUser && maxFreeLevelIndex >= 0) {
          hls.autoLevelCapping = maxFreeLevelIndex;
          hls.nextLevel = maxFreeLevelIndex;
        }
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
        logger.error("[HLSPlayer] error", data.type, data.details, data.fatal);
        if (!data.fatal) return;

        const respCode = (data as unknown as { response?: { code?: number } }).response?.code;

        switch (data.type) {
          case Hls.ErrorTypes.NETWORK_ERROR:
            if (networkRetries < MAX_NETWORK_RETRIES) {
              networkRetries += 1;
              logger.warn("[HLSPlayer] network error retry", networkRetries);
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
            logger.warn("[HLSPlayer] media error — recovering");
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
        logger.error("[HLSPlayer] native video error", code);
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
  }, [src, isPremiumUser, shouldAutoPlay]);

  const handleQualityChange = useCallback((quality: QualityLevel) => {
    setShowQualityMenu(false);
    if (quality.locked) {
      setShowPremiumPrompt(true);
      return;
    }
    setSelectedQuality(quality.index);
    const hls = hlsRef.current;
    const video = videoRef.current;
    if (!hls) return;
    hls.currentLevel = quality.index;
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

  // Keyboard: space (play/pause), ArrowLeft/ArrowRight (seek)
  useEffect(() => {
    const onKeyDown = (e: KeyboardEvent) => {
      const target = e.target as HTMLElement;
      if (
        target.tagName === "INPUT" ||
        target.tagName === "TEXTAREA" ||
        target.tagName === "SELECT" ||
        target.isContentEditable
      ) return;
      if (adActive) return;
      const video = videoRef.current;
      if (!video) return;
      if (e.code === "Space") {
        e.preventDefault();
        if (video.paused) video.play();
        else video.pause();
      } else if (e.code === "ArrowRight") {
        e.preventDefault();
        seekBy(seekStep);
        resetControlsTimer();
      } else if (e.code === "ArrowLeft") {
        e.preventDefault();
        seekBy(-seekStep);
        resetControlsTimer();
      }
    };
    document.addEventListener("keydown", onKeyDown);
    return () => document.removeEventListener("keydown", onKeyDown);
  }, [seekStep, seekBy, resetControlsTimer, adActive]);

  // Video event listeners
  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;
    const onPlay = () => setPlaying(true);
    const onPause = () => {
      setPlaying(false);
      setShowControls(true);
    };
    const onVideoTimeUpdate = () => {
      setCurrentTime(video.currentTime);
      onTimeUpdate?.(video.currentTime, video.duration || 0);
      if (video.buffered.length > 0) {
        setBuffered(video.buffered.end(video.buffered.length - 1));
      }
      if (
        !isPremiumUser &&
        !adActive &&
        nextAdAtRef.current !== null &&
        video.currentTime >= nextAdAtRef.current &&
        video.duration > 0 &&
        video.currentTime < video.duration - 1
      ) {
        nextAdAtRef.current = video.currentTime + AD_INTERVAL_SECONDS;
        triggerAdBreak();
      }
    };
    const onDurationChange = () => {
      setDuration(video.duration);
      if (!adInitializedRef.current && isFinite(video.duration) && video.duration > 0) {
        adInitializedRef.current = true;
        nextAdAtRef.current =
          video.duration < AD_INTERVAL_SECONDS ? video.duration * 0.5 : AD_INTERVAL_SECONDS;
      }
    };
    const onVolumeChange = () => {
      setVolume(video.volume);
      setMuted(video.muted);
    };
    const onVideoEnded = () => {
      setPlaying(false);
      onEnded?.();
    };
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", onPause);
    video.addEventListener("timeupdate", onVideoTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("volumechange", onVolumeChange);
    video.addEventListener("ended", onVideoEnded);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", onPause);
      video.removeEventListener("timeupdate", onVideoTimeUpdate);
      video.removeEventListener("durationchange", onDurationChange);
      video.removeEventListener("volumechange", onVolumeChange);
      video.removeEventListener("ended", onVideoEnded);
    };
  }, [onEnded, onTimeUpdate, isPremiumUser, adActive, triggerAdBreak]);

  const togglePlay = () => {
    if (adActive) return;
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) video.play();
    else video.pause();
  };

  const seek = (e: React.ChangeEvent<HTMLInputElement>) => {
    if (adActive) return;
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
      className={`relative w-full aspect-video overflow-hidden rounded-xl select-none ${
        isPremiumUser
          ? "bg-black ring-1 ring-yellow-500/30 shadow-[0_0_30px_rgba(234,179,8,0.12)]"
          : "bg-black"
      }`}
      onMouseMove={resetControlsTimer}
      onMouseLeave={() => setShowControls(false)}
      onMouseEnter={() => setShowControls(true)}
      onClick={() => { togglePlay(); resetControlsTimer(); }}
    >
      <video
        ref={videoRef}
        className="w-full h-full"
        poster={poster}
        playsInline
      />

      {adActive && adQueue[adIndex] && (
        <div className="absolute inset-0 z-30 bg-black animate-in fade-in duration-200">
          <video
            ref={adVideoRef}
            key={`${adQueue[adIndex].ad.id}-${adIndex}`}
            src={adQueue[adIndex].url}
            className="w-full h-full"
            autoPlay
            playsInline
            onLoadedMetadata={(e) => {
              const d = e.currentTarget.duration;
              setAdRemaining(
                isFinite(d) && d > 0 ? Math.ceil(Math.min(d, AD_DEFAULT_DURATION)) : AD_DEFAULT_DURATION
              );
            }}
            onTimeUpdate={(e) => {
              const v = e.currentTarget;
              const cap = Math.min(v.duration || AD_DEFAULT_DURATION, AD_DEFAULT_DURATION);
              const remaining = Math.max(0, Math.ceil(cap - v.currentTime));
              setAdRemaining(remaining);
              if (v.currentTime >= AD_DEFAULT_DURATION) {
                v.pause();
                if (adIndex + 1 < adQueue.length) {
                  setAdIndex(adIndex + 1);
                  setAdRemaining(AD_DEFAULT_DURATION);
                } else {
                  finishAdBreak();
                }
              }
            }}
            onEnded={() => {
              if (adIndex + 1 < adQueue.length) {
                setAdIndex(adIndex + 1);
                setAdRemaining(AD_DEFAULT_DURATION);
              } else {
                finishAdBreak();
              }
            }}
          />
          <div className="pointer-events-none absolute left-3 top-3 rounded bg-black/70 px-2 py-1 text-[11px] font-medium uppercase tracking-wide text-white">
            Reklama
          </div>
          <div className="pointer-events-none absolute left-3 bottom-3 rounded bg-black/70 px-2 py-1 text-xs text-white tabular-nums">
            Reklama tugashiga: {adRemaining}s
          </div>
          <Link
            href="/premium"
            className="absolute right-3 bottom-3 rounded-full bg-white/95 px-4 py-2 text-xs font-semibold text-black transition hover:bg-white"
          >
            Reklamani o&apos;chirish
          </Link>
        </div>
      )}

      <div className="pointer-events-none absolute left-3 top-3 z-10 flex items-center gap-2">
        {isPremiumUser && <PremiumBadge size="sm" showCrown />}
        {isPremiumUser && (
          <span className="rounded-full border border-yellow-500/25 bg-black/55 px-2.5 py-1 text-[11px] font-medium text-yellow-300 backdrop-blur-sm">
            Premium stream
          </span>
        )}
      </div>

      {/* Top title / series-button overlay — fades with controls, hidden during ads */}
      {(headerTitle || seriesButtonUrl) && (
        <div
          className={`pointer-events-none absolute inset-x-0 top-0 z-10 flex items-start justify-between gap-3 px-4 pt-4 pb-8 transition-opacity duration-200 bg-gradient-to-b from-black/70 via-black/30 to-transparent ${
            showControls && !adActive ? "opacity-100" : "opacity-0"
          }`}
        >
          <div className="min-w-0 pl-12 sm:pl-16 max-w-[70%]">
            {headerTitle && (
              <h2 className="truncate text-sm sm:text-base font-semibold text-white drop-shadow-md">
                {headerTitle}
              </h2>
            )}
            {headerSubtitle && (
              <p className="truncate text-xs text-gray-300/90 drop-shadow">{headerSubtitle}</p>
            )}
          </div>
          {seriesButtonUrl && (
            <Link
              href={seriesButtonUrl}
              onClick={(e) => e.stopPropagation()}
              className="pointer-events-auto inline-flex items-center gap-1.5 rounded-full bg-black/60 hover:bg-black/80 backdrop-blur-sm border border-white/15 px-3 py-1.5 text-xs font-medium text-white transition-colors"
            >
              <List size={14} />
              <span className="hidden sm:inline">{seriesButtonLabel || "Sezonlar va qismlar"}</span>
              <span className="sm:hidden">Qismlar</span>
            </Link>
          )}
        </div>
      )}

      {showPremiumPrompt && (
        <div className="absolute inset-0 z-20 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-sm rounded-2xl border border-yellow-500/30 bg-brand-card/95 p-5 text-center shadow-[0_0_30px_rgba(234,179,8,0.12)] backdrop-blur-sm">
            <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-yellow-500 to-amber-600">
              <Crown size={26} className="text-black" />
            </div>
            <h3 className="mb-2 text-lg font-semibold text-white">Premium sifat</h3>
            <p className="mb-4 text-sm text-gray-300">
              720p va 1080p faqat Premium foydalanuvchilar uchun mavjud.
            </p>
            <div className="flex flex-col gap-3 sm:flex-row">
              <button
                onClick={() => setShowPremiumPrompt(false)}
                className="inline-flex flex-1 items-center justify-center rounded-xl border border-brand-border bg-brand-dark px-4 py-3 text-sm font-medium text-white transition-colors hover:border-brand-red"
              >
                Yopish
              </button>
              <Link
                href="/premium"
                className="inline-flex flex-1 items-center justify-center rounded-xl bg-gradient-to-r from-yellow-500 to-amber-600 px-4 py-3 text-sm font-semibold text-black transition-opacity hover:opacity-90"
              >
                Premium olish
              </Link>
            </div>
          </div>
        </div>
      )}

      {/* Controls overlay — transparent area passes clicks through to container toggle */}
      <div
        className={`absolute inset-0 flex flex-col justify-end transition-opacity duration-200 ${
          showControls && !adActive ? "opacity-100" : "opacity-0 pointer-events-none"
        }`}
      >
        {/* Gradient */}
        <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-transparent to-transparent pointer-events-none" />

        {/* Controls bar — stopPropagation here so only bar clicks don't toggle play */}
        <div
          className={`relative px-3 pb-3 space-y-1.5 ${
            isPremiumUser ? "rounded-b-xl bg-gradient-to-t from-black/35 to-transparent" : ""
          }`}
          onClick={(e) => e.stopPropagation()}
        >
          {/* Progress bar with hover preview */}
          <div
            ref={progressContainerRef}
            className="relative h-1.5 group/progress hover:h-2 transition-all"
            onMouseMove={(e) => {
              if (adActive) return;
              const rect = e.currentTarget.getBoundingClientRect();
              const x = Math.max(0, Math.min(e.clientX - rect.left, rect.width));
              setHoverX(x);
              setProgressWidth(rect.width);
              if (duration > 0) setHoverTime((x / rect.width) * duration);
            }}
            onMouseLeave={() => setHoverTime(null)}
          >
            {/* Hover preview — thumbnail + time. Positioned above the bar, follows mouse. */}
            {hoverTime !== null && duration > 0 && !adActive && (() => {
              const PREVIEW_W = 160;
              const half = PREVIEW_W / 2;
              const left = Math.max(half, Math.min(hoverX, progressWidth - half));
              return (
                <div
                  className="pointer-events-none absolute -translate-x-1/2 bottom-full mb-3 flex flex-col items-center"
                  style={{ left }}
                >
                  <div
                    className="overflow-hidden rounded-md border border-white/15 bg-black shadow-xl"
                    style={{ width: PREVIEW_W, height: PREVIEW_W * 9 / 16 }}
                  >
                    {previewImageUrl || poster ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={previewImageUrl || poster}
                        alt=""
                        className="w-full h-full object-cover"
                        draggable={false}
                      />
                    ) : (
                      <div className="w-full h-full bg-gray-800" />
                    )}
                  </div>
                  <span className="mt-1 rounded bg-black/85 px-2 py-0.5 text-[11px] font-medium text-white tabular-nums">
                    {formatTime(hoverTime)}
                  </span>
                </div>
              );
            })()}

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
            {/* hover indicator dot */}
            {hoverTime !== null && duration > 0 && !adActive && (
              <div
                className="pointer-events-none absolute top-1/2 -translate-y-1/2 h-3 w-3 rounded-full bg-white shadow"
                style={{ left: hoverX - 6 }}
              />
            )}
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

            {/* Rewind */}
            <button
              onClick={() => { seekBy(-seekStep); resetControlsTimer(); }}
              className="flex items-center gap-0.5 text-white hover:text-brand-red transition-colors"
              aria-label={`Rewind ${seekStep} seconds`}
              title={`Rewind ${seekStep}s`}
            >
              <RotateCcw size={18} />
              <span className="text-[10px] tabular-nums">{seekStep}s</span>
            </button>

            {/* Forward */}
            <button
              onClick={() => { seekBy(seekStep); resetControlsTimer(); }}
              className="flex items-center gap-0.5 text-white hover:text-brand-red transition-colors"
              aria-label={`Forward ${seekStep} seconds`}
              title={`Forward ${seekStep}s`}
            >
              <span className="text-[10px] tabular-nums">{seekStep}s</span>
              <RotateCw size={18} />
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

            {/* Seek step selector */}
            <div className="relative">
              <button
                onClick={() => { setShowSeekStepMenu((v) => !v); setShowSpeedMenu(false); setShowQualityMenu(false); }}
                className="flex items-center gap-1 text-white text-xs hover:text-brand-red transition-colors"
                title="Seek step"
              >
                <RotateCw size={14} />
                {seekStep}s
                <ChevronDown size={12} />
              </button>
              {showSeekStepMenu && (
                <div className="absolute bottom-full right-0 mb-2 bg-black/90 border border-white/10 rounded-lg overflow-hidden text-xs min-w-[80px]">
                  {SEEK_STEPS.map((s) => (
                    <button
                      key={s}
                      onClick={() => handleSeekStepChange(s)}
                      className={`block w-full px-3 py-1.5 text-left hover:bg-white/10 transition-colors ${
                        s === seekStep ? "text-brand-red font-semibold" : "text-white"
                      }`}
                    >
                      {s}s
                    </button>
                  ))}
                </div>
              )}
            </div>

            {/* Speed selector */}
            <div className="relative">
              <button
                onClick={() => { setShowSpeedMenu((v) => !v); setShowQualityMenu(false); setShowSeekStepMenu(false); }}
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
                  onClick={() => { setShowQualityMenu((v) => !v); setShowSpeedMenu(false); setShowSeekStepMenu(false); }}
                  className="flex items-center gap-1 text-white text-xs hover:text-brand-red transition-colors"
                >
                  {qualityLabel}
                  <ChevronDown size={12} />
                </button>
                {showQualityMenu && (
                  <div className="absolute bottom-full right-0 mb-2 min-w-[140px] overflow-hidden rounded-lg border border-white/10 bg-black/90 text-xs">
                    {qualities.map((q) => (
                      <button
                        key={q.index}
                        onClick={() => handleQualityChange(q)}
                        className={`flex w-full items-center justify-between px-3 py-1.5 text-left transition-colors hover:bg-white/10 ${
                          q.index === selectedQuality ? "text-brand-red font-semibold" : "text-white"
                        }`}
                      >
                        <span>{q.locked ? `${q.label} 🔒 Premium` : q.label}</span>
                        {q.locked && <Lock size={12} className="text-yellow-400" />}
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
  premiumStreamUrl,
  embedUrl,
  sourceType = "iframe_embed",
  title,
  posterUrl,
  onPlayIntent,
  forceStart,
  onTimeUpdate,
  onEnded,
  headerTitle,
  headerSubtitle,
  seriesButtonUrl,
  seriesButtonLabel,
  previewImageUrl,
}: Props) {
  const { user } = useAuth();
  const [started, setStarted] = useState(false);
  const [hasPlaybackError, setHasPlaybackError] = useState(false);
  const isPremiumViewer = isUserPremium(user);

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
  const selectedVideoUrl =
    sourceType === "direct_hls" || sourceType === "direct_mp4"
      ? (isPremiumViewer ? premiumStreamUrl || videoUrl : videoUrl)
      : videoUrl;
  if (
    (sourceType === "iframe_embed" || sourceType === ("ingestion" as VideoSourceType)) &&
    selectedVideoUrl &&
    !isEmbedUrl(selectedVideoUrl)
  ) {
    if (looksLikeHls(selectedVideoUrl)) {
      effectiveSourceType = "direct_hls";
    } else if (looksLikeMp4(selectedVideoUrl)) {
      effectiveSourceType = "direct_mp4";
    }
  }

  // Determine which URL to use based on source type
  const getEffectiveUrl = (): string | null => {
    if (effectiveSourceType === "iframe_embed" && embedUrl) {
      return embedUrl;
    }
    if (effectiveSourceType === "iframe_embed" && selectedVideoUrl) {
      // Fallback to videoUrl if no embedUrl provided
      if (isEmbedUrl(selectedVideoUrl)) {
        return toEmbedUrl(selectedVideoUrl);
      }
      return null;
    }
    if (effectiveSourceType === "direct_mp4" || effectiveSourceType === "direct_hls") {
      return selectedVideoUrl;
    }
    return null;
  };

  const effectiveUrl = getEffectiveUrl();


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
          <p className="text-gray-300 text-sm mt-1">Tomosha qilish uchun bosing</p>
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
          onTimeUpdate={onTimeUpdate}
          onEnded={onEnded}
        />
      );

    case "direct_hls":
      return (
        <HLSPlayer
          src={effectiveUrl}
          poster={posterUrl}
          autoPlay={forceStart}
          isPremiumUser={isPremiumViewer}
          onTimeUpdate={onTimeUpdate}
          onEnded={onEnded}
          headerTitle={headerTitle}
          headerSubtitle={headerSubtitle}
          seriesButtonUrl={seriesButtonUrl}
          seriesButtonLabel={seriesButtonLabel}
          previewImageUrl={previewImageUrl}
        />
      );

    default:
      return <IframePlayer src={effectiveUrl} title={title} />;
  }
}
