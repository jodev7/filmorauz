"use client";

import Image from "next/image";
import { useEffect, useState } from "react";
import {
  DEFAULT_POSTER_PLACEHOLDER,
  isProtectedMediaUrl,
  normalizeMediaUrl,
  SHIMMER_BLUR_DATA_URL,
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

// Default dimensions for a 2/3 poster card. Used as width/height attrs so the
// browser can reserve space before the image loads (no CLS), while CSS continues
// to scale it to fill the aspect-ratio container.
const DEFAULT_POSTER_WIDTH = 240;
const DEFAULT_POSTER_HEIGHT = 360;

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
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);
  const [currentSrc, setCurrentSrc] = useState(() => normalizeMediaUrl(src, fallbackSrc));

  // Reset loading state when src changes
  useEffect(() => {
    setIsLoading(true);
    setHasError(false);
    setCurrentSrc(normalizeMediaUrl(src, fallbackSrc));
  }, [src, fallbackSrc]);

  const handleLoad = () => {
    setIsLoading(false);
  };

  const handleError = (e: React.SyntheticEvent<HTMLImageElement, Event>) => {
    const nextSrc = normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER);
    const target = e.target as HTMLImageElement;
    if (currentSrc !== nextSrc) {
      setCurrentSrc(nextSrc);
      setHasError(false);
      setIsLoading(true);
    } else {
      setIsLoading(false);
      setHasError(true);
    }
    target.src = nextSrc;
    onError?.(e);
  };

  const w = width ?? DEFAULT_POSTER_WIDTH;
  const h = height ?? DEFAULT_POSTER_HEIGHT;

  // /media/ paths are served by the Cloudflare Worker (signed, auth-gated for
  // some subpaths); skipping the Next.js optimizer avoids /_next/image fetches
  // hitting the worker without the user's cookie and returning 400.
  const unoptimized = isProtectedMediaUrl(currentSrc);

  return (
    <div className={`relative overflow-hidden ${className}`} style={{ aspectRatio }}>
      {/* Skeleton/Placeholder */}
      {showSkeleton && isLoading && !hasError && (
        <div className="absolute inset-0 bg-gradient-to-br from-gray-800 to-gray-900 animate-pulse">
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/5 to-transparent animate-shimmer" />
        </div>
      )}

      {/* Image */}
      <Image
        src={currentSrc}
        alt={alt}
        fill
        sizes={sizes || "(max-width: 640px) 50vw, (max-width: 1024px) 33vw, 240px"}
        priority={priority}
        placeholder="blur"
        blurDataURL={SHIMMER_BLUR_DATA_URL}
        unoptimized={unoptimized}
        className={`object-cover transition-opacity duration-300 ${
          isLoading ? "opacity-0" : "opacity-100"
        }`}
        onLoad={handleLoad}
        onError={handleError}
      />
    </div>
  );
}
