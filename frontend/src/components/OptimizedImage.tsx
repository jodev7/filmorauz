"use client";

import { useEffect, useState } from "react";
import {
  DEFAULT_POSTER_PLACEHOLDER,
  normalizeMediaUrl,
} from "@/lib/image-utils";

interface OptimizedImageProps {
  src: string;
  alt: string;
  className?: string;
  priority?: boolean;
  showSkeleton?: boolean;
  aspectRatio?: string;
  width?: number;
  height?: number;
  sizes?: string;
  onError?: (e: React.SyntheticEvent<HTMLImageElement, Event>) => void;
  fallbackSrc?: string;
}

export default function OptimizedImage({
  src,
  alt,
  className = "",
  priority = false,
  showSkeleton = true,
  aspectRatio = "2/3",
  width,
  height,
  sizes,
  onError,
  fallbackSrc = DEFAULT_POSTER_PLACEHOLDER,
}: OptimizedImageProps) {
  const resolvedSrc = normalizeMediaUrl(src, fallbackSrc);
  const resolvedFallback = normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER);
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);

  // Resolved URL changed (route navigation, updated props) — reset loading state
  // and let the browser re-fetch. The key={finalSrc} below force-remounts the <img>.
  useEffect(() => {
    setIsLoading(true);
    setHasError(false);
  }, [resolvedSrc]);

  const finalSrc = hasError ? resolvedFallback : resolvedSrc;

  const handleLoad = () => {
    setIsLoading(false);
  };

  const handleError = (e: React.SyntheticEvent<HTMLImageElement, Event>) => {
    if (!hasError && resolvedSrc !== resolvedFallback) {
      setHasError(true);
      setIsLoading(true);
    } else {
      setIsLoading(false);
    }
    onError?.(e);
  };

  return (
    <div className={`relative overflow-hidden ${className}`} style={{ aspectRatio }}>
      {/* Skeleton/Placeholder */}
      {showSkeleton && isLoading && !hasError && (
        <div className="absolute inset-0 bg-gradient-to-br from-gray-800 to-gray-900 animate-pulse">
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/5 to-transparent animate-shimmer" />
        </div>
      )}

      {/* Image */}
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        key={finalSrc}
        src={finalSrc}
        alt={alt}
        loading={priority ? "eager" : "lazy"}
        fetchPriority={priority ? "high" : "auto"}
        width={width}
        height={height}
        sizes={sizes}
        className={`object-cover transition-opacity duration-300 ${
          isLoading ? "opacity-0" : "opacity-100"
        }`}
        style={{ position: "absolute", inset: 0, width: "100%", height: "100%" }}
        onLoad={handleLoad}
        onError={handleError}
      />
    </div>
  );
}
