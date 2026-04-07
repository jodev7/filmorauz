"use client";

import { useState, useEffect, useRef } from "react";
import { Play, AlertTriangle, RefreshCw } from "lucide-react";
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

const SPEED_OPTIONS = [0.5, 0.75, 1, 1.25, 1.5, 2];

// HLS Player with hls.js, quality selector (360p/480p/720p) and speed control
function HLSPlayer({ src, title, poster }: { src: string; title: string; poster?: string }) {
  const videoRef = useRef<HTMLVideoElement>(null);
  const hlsRef = useRef<import("hls.js").default | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [levels, setLevels] = useState<{ height: number; index: number }[]>([]);
  const [currentLevel, setCurrentLevel] = useState<number>(-1); // -1 = auto
  const [speed, setSpeed] = useState<number>(1);
  const [settingsOpen, setSettingsOpen] = useState(false);

  useEffect(() => {
    const video = videoRef.current;
    if (!video) return;

    // Native HLS (Safari) — quality switching not exposed, but speed works
    if (video.canPlayType("application/vnd.apple.mpegurl")) {
      video.src = src;
      console.log("[Player] Native HLS (Safari) — speed control active");
      return;
    }

    import("hls.js").then((mod) => {
      const Hls = mod.default;
      if (!Hls.isSupported()) {
        setError("HLS is not supported in this browser.");
        return;
      }
      const hls = new Hls({ startLevel: -1 });
      hlsRef.current = hls;
      hls.loadSource(src);
      hls.attachMedia(video);
      hls.on(Hls.Events.MANIFEST_PARSED, (_evt, data) => {
        const lvls = data.levels.map((l, i) => ({ height: l.height, index: i }));
        setLevels(lvls);
        setCurrentLevel(-1);
        console.log("[Player] HLS manifest parsed — qualities:", lvls.map(l => `${l.height}p`).join(", "));
        video.play().catch(() => {});
      });
      hls.on(Hls.Events.ERROR, (_evt, data) => {
        if (data.fatal) setError("Video playback error. Please try again.");
      });
    });

    return () => {
      hlsRef.current?.destroy();
      hlsRef.current = null;
    };
  }, [src]);

  const handleQualityChange = (level: number) => {
    if (hlsRef.current) {
      hlsRef.current.currentLevel = level;
      setCurrentLevel(level);
    }
  };

  const handleSpeedChange = (s: number) => {
    setSpeed(s);
    if (videoRef.current) videoRef.current.playbackRate = s;
  };

  if (error) return <ErrorFallback message={error} />;

  const qualityLabel = currentLevel === -1 ? "Auto" : `${levels.find(l => l.index === currentLevel)?.height ?? ""}p`;

  return (
    <div className="relative w-full aspect-video bg-black rounded-xl overflow-hidden group">
      <video
        ref={videoRef}
        title={title}
        controls
        autoPlay
        className="w-full h-full"
        poster={poster}
      >
        Your browser does not support HLS streaming.
      </video>

      {/* Settings button — visible on hover */}
      <div className="absolute top-3 right-3 z-10 opacity-0 group-hover:opacity-100 transition-opacity">
        <button
          onClick={() => setSettingsOpen(o => !o)}
          className="bg-black/70 hover:bg-black/90 text-white text-xs px-3 py-1.5 rounded border border-white/20 flex items-center gap-1.5"
        >
          ⚙ Settings
        </button>

        {settingsOpen && (
          <div className="absolute right-0 mt-1 bg-black/90 border border-white/20 rounded-lg p-3 min-w-[160px] space-y-3 text-white text-xs">
            {/* Quality */}
            {levels.length > 0 && (
              <div>
                <p className="text-gray-400 mb-1 uppercase tracking-wide">Quality ({qualityLabel})</p>
                <div className="flex flex-col gap-1">
                  <button
                    onClick={() => handleQualityChange(-1)}
                    className={`text-left px-2 py-1 rounded hover:bg-white/10 ${currentLevel === -1 ? "text-brand-red font-semibold" : ""}`}
                  >Auto</button>
                  {levels.map(l => (
                    <button
                      key={l.index}
                      onClick={() => handleQualityChange(l.index)}
                      className={`text-left px-2 py-1 rounded hover:bg-white/10 ${currentLevel === l.index ? "text-brand-red font-semibold" : ""}`}
                    >{l.height}p</button>
                  ))}
              </div>
            </div>
            )}

            {/* Speed */}
            <div>
              <p className="text-gray-400 mb-1 uppercase tracking-wide">Speed ({speed}x)</p>
              <div className="flex flex-col gap-1">
                {SPEED_OPTIONS.map(s => (
                  <button
                    key={s}
                    onClick={() => handleSpeedChange(s)}
                    className={`text-left px-2 py-1 rounded hover:bg-white/10 ${speed === s ? "text-brand-red font-semibold" : ""}`}
                  >{s}x</button>
                ))}
              </div>
            </div>
          </div>
        )}
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
      return (
        <HLSPlayer 
          src={effectiveUrl} 
          title={title} 
          poster={posterUrl} 
        />
      );
    
    default:
      return <IframePlayer src={effectiveUrl} title={title} />;
  }
}
