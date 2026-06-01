"use client";

import { useState, useEffect, useRef, useCallback, useMemo } from "react";
import Link from "next/link";
import {
  Play,
  Pause,
  Volume2,
  VolumeX,
  Maximize,
  PictureInPicture2,
  AlertTriangle,
  RefreshCw,
  Settings,
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
  initialSeekTime?: number;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onPause?: (currentTime: number, duration: number) => void;
  onInitialSeekResolved?: (applied: boolean) => void;
  onEnded?: () => void;
  // New UI props (all optional — older callers keep working unchanged)
  headerTitle?: string;
  headerSubtitle?: string;
  seriesButtonUrl?: string;
  seriesButtonLabel?: string;
  onSeriesButtonClick?: () => void;
  thumbnailsVttUrl?: string;
  thumbnailsSpriteUrl?: string;
  thumbnailsBaseUrl?: string;
  thumbnailInterval?: number;
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
  initialSeekTime,
  onTimeUpdate,
  onPause,
  onInitialSeekResolved,
  onEnded,
}: { 
  src: string; 
  title: string; 
  poster?: string;
  initialSeekTime?: number;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onPause?: (currentTime: number, duration: number) => void;
  onInitialSeekResolved?: (applied: boolean) => void;
  onEnded?: () => void;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const hasAppliedInitialSeek = useRef(false);
  const isDev = process.env.NODE_ENV !== "production";

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

  useEffect(() => {
    hasAppliedInitialSeek.current = false;
  }, [src, initialSeekTime]);

  useEffect(() => {
    const video = videoRef.current;
    if (!video || hasAppliedInitialSeek.current) return;
    if (!initialSeekTime || initialSeekTime <= 0) {
      onInitialSeekResolved?.(false);
      if (isDev) {
        console.log("[watch-progress] seek skipped", {
          player: "direct",
          reason: "no_initial_seek_time",
        });
      }
      return;
    }
    if (!isFinite(video.duration) || video.duration <= 0 || video.readyState < 1) {
      return;
    }
    video.currentTime = initialSeekTime;
    hasAppliedInitialSeek.current = true;
    onInitialSeekResolved?.(true);
    if (isDev) {
      console.log("[watch-progress] seek applied", {
        player: "direct",
        current_time: initialSeekTime,
        duration: video.duration,
      });
    }
  }, [initialSeekTime, isDev, onInitialSeekResolved]);

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
        onLoadedMetadata={() => {
          if (!videoRef.current || hasAppliedInitialSeek.current) return;
          if (initialSeekTime && initialSeekTime > 0) {
            videoRef.current.currentTime = initialSeekTime;
            onInitialSeekResolved?.(true);
            if (isDev) {
              console.log("[watch-progress] seek applied", {
                player: "direct",
                current_time: initialSeekTime,
                duration: videoRef.current.duration,
                trigger: "loadedmetadata",
              });
            }
          } else {
            onInitialSeekResolved?.(false);
            if (isDev) {
              console.log("[watch-progress] seek skipped", {
                player: "direct",
                reason: "no_initial_seek_time",
                trigger: "loadedmetadata",
              });
            }
          }
          hasAppliedInitialSeek.current = true;
        }}
        onLoadStart={() => setLoading(true)}
        onTimeUpdate={() => {
          if (!videoRef.current || !onTimeUpdate) return;
          onTimeUpdate(videoRef.current.currentTime, videoRef.current.duration || 0);
        }}
        onPause={() => {
          if (!videoRef.current || !onPause) return;
          onPause(videoRef.current.currentTime, videoRef.current.duration || 0);
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

// Parse a WebVTT file emitted by the worker's thumbnail pipeline. Each cue
// looks like:
//   00:00:00.000 --> 00:00:10.000
//   sprite.webp#xywh=0,0,160,90
// Returns sorted cues with absolute sprite URLs and parsed xywh.
function parseThumbnailVtt(
  text: string,
  vttUrl: string
): { start: number; end: number; src: string; x: number; y: number; w: number; h: number }[] {
  const cues: { start: number; end: number; src: string; x: number; y: number; w: number; h: number }[] = [];
  const lines = text.split(/\r?\n/);
  const ts = (s: string) => {
    const m = s.match(/(\d+):(\d+):(\d+)(?:\.(\d+))?/);
    if (!m) return NaN;
    return +m[1] * 3600 + +m[2] * 60 + +m[3] + (m[4] ? +`0.${m[4]}` : 0);
  };
  for (let i = 0; i < lines.length; i++) {
    const arrow = lines[i].indexOf("-->");
    if (arrow < 0) continue;
    const start = ts(lines[i].slice(0, arrow));
    const end = ts(lines[i].slice(arrow + 3));
    const target = (lines[i + 1] || "").trim();
    if (!target || isNaN(start) || isNaN(end)) continue;
    const [path, frag = ""] = target.split("#");
    const m = frag.match(/xywh=(\d+),(\d+),(\d+),(\d+)/);
    if (!m) continue;
    const src = new URL(path, vttUrl).toString();
    cues.push({ start, end, src, x: +m[1], y: +m[2], w: +m[3], h: +m[4] });
    i++;
  }
  return cues.sort((a, b) => a.start - b.start);
}

function HLSPlayer({
  src,
  poster,
  autoPlay: shouldAutoPlay,
  isPremiumUser,
  initialSeekTime,
  onTimeUpdate,
  onPause,
  onInitialSeekResolved,
  onEnded,
  headerTitle,
  headerSubtitle,
  seriesButtonUrl,
  seriesButtonLabel,
  onSeriesButtonClick,
  thumbnailsVttUrl,
  thumbnailsSpriteUrl,
  thumbnailsBaseUrl,
  thumbnailInterval,
}: {
  src: string;
  poster?: string;
  autoPlay?: boolean;
  isPremiumUser: boolean;
  initialSeekTime?: number;
  onTimeUpdate?: (currentTime: number, duration: number) => void;
  onPause?: (currentTime: number, duration: number) => void;
  onInitialSeekResolved?: (applied: boolean) => void;
  onEnded?: () => void;
  headerTitle?: string;
  headerSubtitle?: string;
  seriesButtonUrl?: string;
  seriesButtonLabel?: string;
  onSeriesButtonClick?: () => void;
  thumbnailsVttUrl?: string;
  thumbnailsSpriteUrl?: string;
  thumbnailsBaseUrl?: string;
  thumbnailInterval?: number;
}) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<Hls | null>(null);
  const containerRef = useRef<HTMLDivElement>(null);
  const controlsTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const clickTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  const [playing, setPlaying] = useState(false);
  const [currentTime, setCurrentTime] = useState(0);
  const [duration, setDuration] = useState(0);
  const [volume, setVolume] = useState(1);
  const [muted, setMuted] = useState(false);
  const [showControls, setShowControls] = useState(true);
  const [buffered, setBuffered] = useState(0);
  const [error, setError] = useState<string | null>(null);
  // Scrubbing state for the progress bar. The range input is controlled by
  // `currentTime`, which only advances on `timeupdate`; during an HLS seek
  // that event lags, so a naive drag snaps the thumb back to the old position
  // and the bar feels un-draggable (it only "jumps" on release). While the
  // user drags we show `scrubValue` and hold the commit until pointer-up.
  const [scrubbing, setScrubbing] = useState(false);
  const [scrubValue, setScrubValue] = useState(0);

  const [qualities, setQualities] = useState<QualityLevel[]>([]);
  const [selectedQuality, setSelectedQuality] = useState(-1); // -1 = Auto
  const [selectedSpeed, setSelectedSpeed] = useState(1);
  const [showPremiumPrompt, setShowPremiumPrompt] = useState(false);
  const [seekStep, setSeekStep] = useState(10);
  const [showSettings, setShowSettings] = useState(false);
  // Picture-in-Picture: lets the video keep playing in a floating OS-level
  // window after the user switches tab/app (YouTube-style). `isPiP` drives
  // the toggle icon; support is feature-detected so the button hides on
  // browsers without PiP (notably iPhone Safari, which has no <video> PiP —
  // it only offers native fullscreen → control-center PiP).
  const [isPiP, setIsPiP] = useState(false);
  const [pipSupported, setPipSupported] = useState(false);
  const [settingsPane, setSettingsPane] = useState<"root" | "quality" | "speed" | "seek">("root");

  // Progress hover preview
  const progressContainerRef = useRef<HTMLDivElement>(null);
  const settingsRef = useRef<HTMLDivElement>(null);
  const settingsButtonRef = useRef<HTMLButtonElement>(null);

  useEffect(() => {
    const handleClickOutside = (event: MouseEvent | TouchEvent) => {
      if (
        showSettings &&
        settingsRef.current &&
        !settingsRef.current.contains(event.target as Node) &&
        settingsButtonRef.current &&
        !settingsButtonRef.current.contains(event.target as Node)
      ) {
        setShowSettings(false);
      }
    };

    const handleEscKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setShowSettings(false);
      }
    };

    document.addEventListener("mousedown", handleClickOutside);
    document.addEventListener("touchstart", handleClickOutside);
    document.addEventListener("keydown", handleEscKey);

    return () => {
      document.removeEventListener("mousedown", handleClickOutside);
      document.removeEventListener("touchstart", handleClickOutside);
      document.removeEventListener("keydown", handleEscKey);
    };
  }, [showSettings]);

  const [hoverTime, setHoverTime] = useState<number | null>(null);
  const [hoverX, setHoverX] = useState(0);
  const [progressWidth, setProgressWidth] = useState(0);
  const hoverRaf = useRef<number | null>(null);

  // Lazy-loaded scrub-preview cues parsed from the VTT file. Only fetched the
  // first time the user hovers the progress bar — avoids a network hit for
  // visitors who never scrub.
  type ThumbCue = { start: number; end: number; src: string; x: number; y: number; w: number; h: number };
  const [thumbCues, setThumbCues] = useState<ThumbCue[] | null>(null);
  const thumbFetchStarted = useRef(false);
  // Thumbnails: prefer the explicit props, but fall back to deriving the
  // thumbnail dir from the HLS src (…/index.m3u8 → …/thumbnails/). The worker
  // uploads preview.vtt + sprite.webp + frame_NNNN.webp there, so scrub
  // previews work even when the movie/episode row predates the thumbnail-URL
  // fields (or they were never persisted).
  const derivedThumbBase = useMemo(() => {
    if (thumbnailsBaseUrl) return thumbnailsBaseUrl;
    if (src && /\/(index|master)\.m3u8(\?.*)?$/.test(src)) {
      return src.replace(/\/(index|master)\.m3u8(\?.*)?$/, "/thumbnails/");
    }
    return "";
  }, [thumbnailsBaseUrl, src]);
  const effVttUrl = thumbnailsVttUrl || (derivedThumbBase ? `${derivedThumbBase}preview.vtt` : "");
  const effSpriteUrl = thumbnailsSpriteUrl || (derivedThumbBase ? `${derivedThumbBase}sprite.webp` : "");
  const effInterval = thumbnailInterval && thumbnailInterval > 0 ? thumbnailInterval : 5;
  const ensureThumbCues = useCallback(() => {
    if (thumbFetchStarted.current) return;
    thumbFetchStarted.current = true;
    // Preload the sprite in parallel with VTT — first hover usually means the
    // user will keep moving, so warming the image cache cuts the visible
    // flash on the first cue render. img.decode() resolves once decoded.
    if (effSpriteUrl) {
      const img = new Image();
      img.src = effSpriteUrl;
    }
    if (!effVttUrl) return;
    fetch(effVttUrl)
      .then((r) => (r.ok ? r.text() : Promise.reject(r.status)))
      .then((text) => setThumbCues(parseThumbnailVtt(text, effVttUrl)))
      .catch(() => { /* fall back to time-only tooltip */ });
  }, [effVttUrl, effSpriteUrl]);
  const activeCue = (() => {
    if (hoverTime === null) return null;
    if (thumbCues && thumbCues.length) {
      // Binary-search-friendly scan: cues are sorted by start time.
      for (let i = 0; i < thumbCues.length; i++) {
        const c = thumbCues[i];
        if (hoverTime >= c.start && hoverTime < c.end) return c;
      }
      return thumbCues[thumbCues.length - 1];
    }
    if (derivedThumbBase) {
      const idx = Math.floor(hoverTime / effInterval) + 1;
      const padded = String(idx).padStart(4, "0");
      return { start: 0, end: 0, src: `${derivedThumbBase}frame_${padded}.webp`, x: 0, y: 0, w: 0, h: 0 };
    }
    return null;
  })();

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
  const hasAppliedInitialSeek = useRef(false);
  const isDev = process.env.NODE_ENV !== "production";

  useEffect(() => {
    setSeekStep(loadSeekStep());
  }, []);

  // Try to apply the saved-progress seek. Safe to call repeatedly: it bails if
  // already applied, if the player isn't ready, or if the saved time is outside
  // the resume window. Each branch logs once via a ref.
  const tryApplyInitialSeek = useCallback((trigger: string) => {
    const video = videoRef.current;
    if (!video || hasAppliedInitialSeek.current) return;
    if (!initialSeekTime || initialSeekTime <= 10) {
      // Not a definitive "no resume" yet — saved progress may still be loading.
      // Only resolve when we know there's nothing to seek to AND metadata exists.
      if (isFinite(video.duration) && video.duration > 0 && (initialSeekTime ?? 0) <= 0) {
        hasAppliedInitialSeek.current = true;
        onInitialSeekResolved?.(false);
        if (isDev) {
          console.log("[player] resume skipped reason=no_saved_time trigger=" + trigger);
        }
      }
      return;
    }
    if (!isFinite(video.duration) || video.duration <= 0 || video.readyState < 1) {
      return; // wait for metadata
    }
    if (initialSeekTime > video.duration - 30) {
      hasAppliedInitialSeek.current = true;
      onInitialSeekResolved?.(false);
      if (isDev) {
        console.log(`[player] resume skipped reason=too_close_to_end time=${initialSeekTime} duration=${video.duration}`);
      }
      return;
    }
    video.currentTime = initialSeekTime;
    hasAppliedInitialSeek.current = true;
    onInitialSeekResolved?.(true);
    if (isDev) {
      console.log(`[player] resume seek applied time=${initialSeekTime} trigger=${trigger}`);
    }
  }, [initialSeekTime, isDev, onInitialSeekResolved]);

  // Keep the latest tryApplyInitialSeek reachable from listeners that were
  // registered once (HLS MANIFEST_PARSED, native loadedmetadata) — those
  // closures capture the function at registration time, but we need them to
  // see the updated `initialSeekTime` once the GET /watch/progress fetch
  // resolves after the source effect already ran.
  const trySeekRef = useRef(tryApplyInitialSeek);
  useEffect(() => {
    trySeekRef.current = tryApplyInitialSeek;
    tryApplyInitialSeek("effect");
  }, [tryApplyInitialSeek]);

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
    setSettingsPane("root");
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
    hasAppliedInitialSeek.current = false;

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
        // Try the saved-progress seek as soon as the manifest is ready —
        // duration is usually available here, before durationchange fires.
        trySeekRef.current("manifest_parsed");
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
      const onNativeLoadedMeta = () => trySeekRef.current("loadedmetadata_native");
      video.addEventListener("loadedmetadata", onNativeLoadedMeta);
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
        video.removeEventListener("loadedmetadata", onNativeLoadedMeta);
        video.removeAttribute("src");
        video.load();
      };
    } else {
      setError("Brauzeringiz HLS formatini qo'llab-quvvatlamaydi.");
    }
  }, [src, isPremiumUser, shouldAutoPlay]);

  const handleQualityChange = useCallback((quality: QualityLevel) => {
    setSettingsPane("root");
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
      } else if (e.code === "ArrowUp") {
        e.preventDefault();
        const next = Math.min(1, (video.muted ? 0 : video.volume) + 0.05);
        video.muted = false;
        video.volume = next;
        resetControlsTimer();
      } else if (e.code === "ArrowDown") {
        e.preventDefault();
        const next = Math.max(0, (video.muted ? 0 : video.volume) - 0.05);
        video.volume = next;
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
    const handlePause = () => {
      setPlaying(false);
      setShowControls(true);
      onPause?.(video.currentTime, video.duration || 0);
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
      trySeekRef.current("durationchange");
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
    const onLoadedMeta = () => trySeekRef.current("loadedmetadata");
    video.addEventListener("play", onPlay);
    video.addEventListener("pause", handlePause);
    video.addEventListener("timeupdate", onVideoTimeUpdate);
    video.addEventListener("durationchange", onDurationChange);
    video.addEventListener("loadedmetadata", onLoadedMeta);
    video.addEventListener("volumechange", onVolumeChange);
    video.addEventListener("ended", onVideoEnded);
    return () => {
      video.removeEventListener("play", onPlay);
      video.removeEventListener("pause", handlePause);
      video.removeEventListener("timeupdate", onVideoTimeUpdate);
      video.removeEventListener("durationchange", onDurationChange);
      video.removeEventListener("loadedmetadata", onLoadedMeta);
      video.removeEventListener("volumechange", onVolumeChange);
      video.removeEventListener("ended", onVideoEnded);
    };
  }, [onEnded, onTimeUpdate, onPause, isPremiumUser, adActive, triggerAdBreak]);

  const togglePlay = () => {
    if (adActive) return;
    const video = videoRef.current;
    if (!video) return;
    if (video.paused) video.play();
    else video.pause();
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

  // Feature-detect PiP once and keep `isPiP` in sync with the OS window,
  // which the user can close from the floating window itself (not just our
  // button) — so we listen to enter/leave events rather than trusting state.
  useEffect(() => {
    const video = videoRef.current;
    const doc = document as Document & { pictureInPictureEnabled?: boolean };
    const supported =
      !!doc.pictureInPictureEnabled &&
      !!video &&
      typeof (video as HTMLVideoElement & { requestPictureInPicture?: () => Promise<unknown> })
        .requestPictureInPicture === "function";
    setPipSupported(supported);
    if (!video) return;
    const onEnter = () => setIsPiP(true);
    const onLeave = () => setIsPiP(false);
    video.addEventListener("enterpictureinpicture", onEnter);
    video.addEventListener("leavepictureinpicture", onLeave);
    return () => {
      video.removeEventListener("enterpictureinpicture", onEnter);
      video.removeEventListener("leavepictureinpicture", onLeave);
    };
  }, [src]);

  // Screen Wake Lock: keep the phone awake while playing. A custom
  // (non-fullscreen) player doesn't inhibit the OS idle timer, so the
  // screen sleeps after a minute without a touch mid-playback. Re-acquire
  // on tab return (the OS releases it on hide). No-ops where unsupported.
  useEffect(() => {
    const nav = navigator as Navigator & {
      wakeLock?: { request: (t: "screen") => Promise<{ release: () => Promise<void> }> };
    };
    if (!nav.wakeLock) return;
    let sentinel: { release: () => Promise<void> } | null = null;
    let cancelled = false;
    const acquire = async () => {
      if (sentinel) return;
      try {
        sentinel = await nav.wakeLock!.request("screen");
        if (cancelled) {
          sentinel.release().catch(() => {});
          sentinel = null;
        }
      } catch {
        /* not allowed — ignore */
      }
    };
    const release = () => {
      sentinel?.release().catch(() => {});
      sentinel = null;
    };
    if (playing) acquire();
    else release();
    const onVis = () => {
      if (!document.hidden && playing) acquire();
    };
    document.addEventListener("visibilitychange", onVis);
    return () => {
      cancelled = true;
      document.removeEventListener("visibilitychange", onVis);
      release();
    };
  }, [playing]);

  // Auto-PiP: when the user switches tab/app while the video is playing,
  // pop it into the floating window automatically (and restore on return).
  // Browsers only permit the programmatic request from a visibilitychange
  // handler when the page already has a user-gesture history and media is
  // active, so this silently no-ops where disallowed.
  useEffect(() => {
    if (!pipSupported) return;
    const doc = document as Document & {
      pictureInPictureElement?: Element;
      exitPictureInPicture?: () => Promise<void>;
    };
    const onVisibility = () => {
      const video = videoRef.current as
        | (HTMLVideoElement & { requestPictureInPicture?: () => Promise<unknown> })
        | null;
      if (!video) return;
      if (document.hidden) {
        if (!video.paused && !doc.pictureInPictureElement) {
          video.requestPictureInPicture?.().catch(() => {});
        }
      } else if (doc.pictureInPictureElement) {
        doc.exitPictureInPicture?.().catch(() => {});
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    return () => document.removeEventListener("visibilitychange", onVisibility);
  }, [pipSupported]);

  const togglePictureInPicture = async () => {
    const video = videoRef.current as
      | (HTMLVideoElement & { requestPictureInPicture?: () => Promise<unknown> })
      | null;
    const doc = document as Document & {
      pictureInPictureElement?: Element;
      exitPictureInPicture?: () => Promise<void>;
    };
    if (!video) return;
    try {
      if (doc.pictureInPictureElement) {
        await doc.exitPictureInPicture?.();
      } else if (video.requestPictureInPicture) {
        await video.requestPictureInPicture();
      }
    } catch {
      /* user gesture / not-allowed — ignore, button stays inert */
    }
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
      onClick={() => {
        resetControlsTimer();
        // On touch devices there's no double-tap-to-fullscreen (it kept firing
        // by accident while users tapped the seek buttons), so toggle play
        // immediately instead of debouncing against a pending dblclick.
        if (isMobile) {
          togglePlay();
          return;
        }
        if (clickTimer.current) clearTimeout(clickTimer.current);
        clickTimer.current = setTimeout(() => {
          clickTimer.current = null;
          togglePlay();
        }, 250);
      }}
      onDoubleClick={(e) => {
        if (isMobile) return; // fullscreen via the dedicated button on mobile
        e.preventDefault();
        if (clickTimer.current) {
          clearTimeout(clickTimer.current);
          clickTimer.current = null;
        }
        toggleFullscreen();
      }}
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
      {(headerTitle || seriesButtonUrl || onSeriesButtonClick) && (
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
          {(seriesButtonUrl || onSeriesButtonClick) && (
            onSeriesButtonClick ? (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onSeriesButtonClick();
                }}
                className="pointer-events-auto inline-flex items-center gap-1.5 rounded-full bg-black/60 hover:bg-black/80 backdrop-blur-sm border border-white/15 px-3 py-1.5 text-xs font-medium text-white transition-colors"
              >
                <List size={14} />
                <span className="hidden sm:inline">{seriesButtonLabel || "Sezonlar va qismlar"}</span>
                <span className="sm:hidden">Qismlar</span>
              </button>
            ) : (
              <Link
                href={seriesButtonUrl!}
                onClick={(e) => e.stopPropagation()}
                className="pointer-events-auto inline-flex items-center gap-1.5 rounded-full bg-black/60 hover:bg-black/80 backdrop-blur-sm border border-white/15 px-3 py-1.5 text-xs font-medium text-white transition-colors"
              >
                <List size={14} />
                <span className="hidden sm:inline">{seriesButtonLabel || "Sezonlar va qismlar"}</span>
                <span className="sm:hidden">Qismlar</span>
              </Link>
            )
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

      {/* Center playback controls — YouTube-style: rewind / play / forward.
          stopPropagation so the click doesn't fall through to the container's
          play-toggle handler. */}
      <div
        className={`pointer-events-none absolute inset-0 z-10 flex items-center justify-center gap-8 sm:gap-14 transition-opacity duration-200 ${
          showControls && !adActive ? "opacity-100" : "opacity-0"
        }`}
      >
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); seekBy(-seekStep); resetControlsTimer(); }}
          className="pointer-events-auto flex h-12 w-12 items-center justify-center rounded-full bg-black/45 text-white transition hover:bg-black/65 sm:h-14 sm:w-14"
          aria-label={`Rewind ${seekStep} seconds`}
          title={`Rewind ${seekStep}s`}
        >
          <RotateCcw size={24} />
        </button>
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); togglePlay(); resetControlsTimer(); }}
          className="pointer-events-auto flex h-16 w-16 items-center justify-center rounded-full bg-black/55 text-white transition hover:bg-black/75 sm:h-20 sm:w-20"
          aria-label={playing ? "Pause" : "Play"}
        >
          {playing ? <Pause size={32} fill="white" /> : <Play size={32} fill="white" className="ml-1" />}
        </button>
        <button
          type="button"
          onClick={(e) => { e.stopPropagation(); seekBy(seekStep); resetControlsTimer(); }}
          className="pointer-events-auto flex h-12 w-12 items-center justify-center rounded-full bg-black/45 text-white transition hover:bg-black/65 sm:h-14 sm:w-14"
          aria-label={`Forward ${seekStep} seconds`}
          title={`Forward ${seekStep}s`}
        >
          <RotateCw size={24} />
        </button>
      </div>

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
          {/* Progress bar with hover time tooltip */}
          <div
            ref={progressContainerRef}
            className="relative h-1.5 group/progress hover:h-2 transition-all"
            onMouseEnter={ensureThumbCues}
            onMouseMove={(e) => {
              if (adActive) return;
              // Throttle via rAF — coalesces multiple mousemove events into a
              // single paint, keeps state churn ≤16ms regardless of input rate.
              const rect = e.currentTarget.getBoundingClientRect();
              const clientX = e.clientX;
              if (hoverRaf.current !== null) return;
              hoverRaf.current = requestAnimationFrame(() => {
                hoverRaf.current = null;
                const x = Math.max(0, Math.min(clientX - rect.left, rect.width));
                setHoverX(x);
                setProgressWidth(rect.width);
                if (duration > 0) setHoverTime((x / rect.width) * duration);
              });
            }}
            onMouseLeave={() => {
              if (hoverRaf.current !== null) {
                cancelAnimationFrame(hoverRaf.current);
                hoverRaf.current = null;
              }
              setHoverTime(null);
            }}
          >
            {/* Hover tooltip — thumbnail + time (sprite-backed when VTT cues
                are available, single-image fallback when only the base URL is
                known, time-only otherwise). Lazy: cues fetch on first hover. */}
            {hoverTime !== null && duration > 0 && !adActive && (() => {
              const hasThumb = !!activeCue;
              const PREVIEW_W = hasThumb ? 160 : 56;
              const half = PREVIEW_W / 2;
              const left = Math.max(half, Math.min(hoverX, progressWidth - half));
              const isSprite = !!activeCue && activeCue.w > 0;
              return (
                <div
                  className="pointer-events-none absolute -translate-x-1/2 bottom-full mb-3 flex flex-col items-center gap-1"
                  style={{ left }}
                >
                  {hasThumb && (
                    isSprite ? (
                      <div
                        className="rounded overflow-hidden ring-1 ring-white/20 shadow-lg bg-black"
                        style={{
                          width: activeCue!.w,
                          height: activeCue!.h,
                          backgroundImage: `url(${activeCue!.src})`,
                          backgroundPosition: `-${activeCue!.x}px -${activeCue!.y}px`,
                          backgroundRepeat: "no-repeat",
                        }}
                      />
                    ) : (
                      <img
                        src={activeCue!.src}
                        alt=""
                        loading="lazy"
                        decoding="async"
                        className="rounded ring-1 ring-white/20 shadow-lg bg-black"
                        style={{ width: 160, height: 90 }}
                        onError={(e) => { (e.currentTarget as HTMLImageElement).style.visibility = "hidden"; }}
                      />
                    )
                  )}
                  <span className="block min-w-14 rounded bg-black/85 px-2 py-1 text-center text-[11px] font-medium text-white tabular-nums shadow-lg">
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
            {/* played — follow the drag position while scrubbing so the bar
                tracks the finger/cursor instead of the (lagging) video time */}
            <div
              className="absolute top-0 left-0 h-full bg-brand-red rounded-full pointer-events-none"
              style={{ width: duration ? `${Math.min(((scrubbing ? scrubValue : currentTime) / duration) * 100, 100)}%` : "0%" }}
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
              value={scrubbing ? scrubValue : currentTime}
              onPointerDown={() => {
                if (adActive) return;
                setScrubValue(videoRef.current?.currentTime ?? currentTime);
                setScrubbing(true);
              }}
              onChange={(e) => {
                if (adActive) return;
                // Track the drag locally; commit to the video on pointer-up so
                // HLS isn't hammered with a seek on every intermediate value.
                setScrubValue(Number(e.target.value));
              }}
              onPointerUp={() => {
                if (!scrubbing) return;
                const video = videoRef.current;
                if (video) video.currentTime = scrubValue;
                setScrubbing(false);
              }}
              onPointerCancel={() => setScrubbing(false)}
              className="absolute inset-0 w-full h-full opacity-0 cursor-pointer z-10"
            />
          </div>

          {/* Buttons row — clean: volume / time on the left, settings + fullscreen on the right.
              Play/Pause and seek are in the center overlay above. */}
          <div className="flex items-center gap-3">
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

            {/* Settings — single gear with submenu pattern (root → quality / speed / seek). */}
            <div className="relative">
              <button
                ref={settingsButtonRef}
                onClick={() => {
                  setShowSettings((v) => !v);
                  setSettingsPane("root");
                }}
                className="flex items-center gap-1 text-white hover:text-brand-red transition-colors"
                aria-label="Settings"
                title="Settings"
              >
                <Settings size={18} />
              </button>
              {showSettings && (
                <div ref={settingsRef} className="absolute bottom-full right-0 z-50 mb-2 min-w-[200px] overflow-hidden rounded-lg border border-white/10 bg-black/95 text-xs shadow-xl">
                  {settingsPane === "root" && (
                    <div className="py-1">
                      {qualities.length > 1 && (
                        <button
                          onClick={() => setSettingsPane("quality")}
                          className="flex w-full items-center justify-between px-3 py-2 text-left text-white hover:bg-white/10"
                        >
                          <span>Sifat</span>
                          <span className="text-white/60">{qualityLabel}</span>
                        </button>
                      )}
                      <button
                        onClick={() => setSettingsPane("speed")}
                        className="flex w-full items-center justify-between px-3 py-2 text-left text-white hover:bg-white/10"
                      >
                        <span>Tezlik</span>
                        <span className="text-white/60">{selectedSpeed}x</span>
                      </button>
                      <button
                        onClick={() => setSettingsPane("seek")}
                        className="flex w-full items-center justify-between px-3 py-2 text-left text-white hover:bg-white/10"
                      >
                        <span>Olg‘a/orqa qadami</span>
                        <span className="text-white/60">{seekStep}s</span>
                      </button>
                    </div>
                  )}

                  {settingsPane === "quality" && (
                    <div className="py-1">
                      <button
                        onClick={() => setSettingsPane("root")}
                        className="block w-full px-3 py-2 text-left text-white/70 hover:bg-white/10"
                      >
                        ‹ Sifat
                      </button>
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

                  {settingsPane === "speed" && (
                    <div className="py-1">
                      <button
                        onClick={() => setSettingsPane("root")}
                        className="block w-full px-3 py-2 text-left text-white/70 hover:bg-white/10"
                      >
                        ‹ Tezlik
                      </button>
                      {SPEEDS.map((s) => (
                        <button
                          key={s}
                          onClick={() => { setSelectedSpeed(s); setSettingsPane("root"); }}
                          className={`block w-full px-3 py-1.5 text-left hover:bg-white/10 transition-colors ${
                            s === selectedSpeed ? "text-brand-red font-semibold" : "text-white"
                          }`}
                        >
                          {s}x
                        </button>
                      ))}
                    </div>
                  )}

                  {settingsPane === "seek" && (
                    <div className="py-1">
                      <button
                        onClick={() => setSettingsPane("root")}
                        className="block w-full px-3 py-2 text-left text-white/70 hover:bg-white/10"
                      >
                        ‹ Olg‘a/orqa qadami
                      </button>
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
              )}
            </div>

            {/* Picture-in-Picture — keeps the video playing in a floating
                OS window when the user leaves the tab/app. Hidden where
                unsupported (e.g. iPhone Safari). */}
            {pipSupported && (
              <button
                onClick={togglePictureInPicture}
                className={`transition-colors ${isPiP ? "text-brand-red" : "text-white hover:text-brand-red"}`}
                aria-label="Suzuvchi oyna (Picture-in-Picture)"
                title="Suzuvchi oynada ko'rish"
              >
                <PictureInPicture2 size={18} />
              </button>
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
  initialSeekTime,
  onTimeUpdate,
  onPause,
  onInitialSeekResolved,
  onEnded,
  headerTitle,
  headerSubtitle,
  seriesButtonUrl,
  seriesButtonLabel,
  onSeriesButtonClick,
  thumbnailsVttUrl,
  thumbnailsSpriteUrl,
  thumbnailsBaseUrl,
  thumbnailInterval,
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
          initialSeekTime={initialSeekTime}
          onTimeUpdate={onTimeUpdate}
          onPause={onPause}
          onInitialSeekResolved={onInitialSeekResolved}
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
          initialSeekTime={initialSeekTime}
          onTimeUpdate={onTimeUpdate}
          onPause={onPause}
          onInitialSeekResolved={onInitialSeekResolved}
          onEnded={onEnded}
          headerTitle={headerTitle}
          headerSubtitle={headerSubtitle}
          seriesButtonUrl={seriesButtonUrl}
          seriesButtonLabel={seriesButtonLabel}
          onSeriesButtonClick={onSeriesButtonClick}
          thumbnailsVttUrl={thumbnailsVttUrl}
          thumbnailsSpriteUrl={thumbnailsSpriteUrl}
          thumbnailsBaseUrl={thumbnailsBaseUrl}
          thumbnailInterval={thumbnailInterval}
        />
      );

    default:
      return <IframePlayer src={effectiveUrl} title={title} />;
  }
}
