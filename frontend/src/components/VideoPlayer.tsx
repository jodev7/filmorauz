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

// PlayerJS-based HLS player with built-in quality selector (360p/480p/720p)
// Script is loaded once from CDN and cached on window.Playerjs
function HLSPlayer({ src, poster }: { src: string; title?: string; poster?: string }) {
  const PLAYER_ID = "filmorauz-hls-player";

  useEffect(() => {
    const baseUrl = src.replace(/\/master\.m3u8$/, "");
    const files = [
      { label: "720p", file: `${baseUrl}/720p/index.m3u8` },
      { label: "480p", file: `${baseUrl}/480p/index.m3u8` },
      { label: "360p", file: `${baseUrl}/360p/index.m3u8` },
    ];

    const init = () => {
      const el = document.getElementById(PLAYER_ID);
      if (el) el.innerHTML = "";
      console.log("[PlayerJS] init — qualities:", files.map(f => f.label).join(", "));
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
      new (window as any).Playerjs({
        id: PLAYER_ID,
        file: files,
        poster: poster || "",
        autoplay: 1,
      });
    };

    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    if ((window as any).Playerjs) {
      init();
      return;
    }

    const existing = document.getElementById("playerjs-cdn") as HTMLScriptElement | null;
    if (existing) {
      existing.addEventListener("load", init, { once: true });
      return;
    }

    const s = document.createElement("script");
    s.id = "playerjs-cdn";
    s.src = "https://playerjs.com/playerjs.js";
    s.async = true;
    s.addEventListener("load", init, { once: true });
    document.head.appendChild(s);
  }, [src, poster]);

  return (
    <div className="w-full aspect-video bg-black rounded-xl overflow-hidden">
      <div id={PLAYER_ID} style={{ width: "100%", height: "100%" }} />
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
