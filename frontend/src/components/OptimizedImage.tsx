"use client";

import { memo, useEffect, useMemo, useState } from "react";
import {
  DEFAULT_POSTER_PLACEHOLDER,
  normalizeMediaUrl,
} from "@/lib/image-utils";
import MediaImage, { isImageUrlLoaded, markImageUrlLoaded } from "@/components/ui/MediaImage";

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

function OptimizedImageImpl({
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
  const resolvedImageUrl = useMemo(() => normalizeMediaUrl(src, ""), [src]);
  const imageStyle = useMemo(
    () => ({ position: "absolute", inset: 0, width: "100%", height: "100%" } as const),
    [],
  );

  // Skeleton visibility tracks the inner image's load lifecycle. Reset only
  // when the resolved URL actually changes — never on parent re-renders that
  // pass an identical src.
  const [showOverlay, setShowOverlay] = useState(() => !isImageUrlLoaded(resolvedImageUrl));

  useEffect(() => {
    setShowOverlay(!isImageUrlLoaded(resolvedImageUrl));
  }, [resolvedImageUrl]);

  const handleLoad = () => {
    markImageUrlLoaded(resolvedImageUrl);
    setShowOverlay(false);
  };
  const handleError = (e: React.SyntheticEvent<HTMLImageElement, Event>) => {
    // Drop the skeleton on error so we don't sit on it forever — MediaImage
    // will swap to the fallback src internally.
    setShowOverlay(false);
    onError?.(e);
  };

  return (
    <div className={`relative overflow-hidden ${className}`} style={{ aspectRatio }}>
      {showSkeleton && showOverlay && (
        <div className="absolute inset-0 bg-gradient-to-br from-gray-800 to-gray-900 animate-pulse">
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/5 to-transparent animate-shimmer" />
        </div>
      )}

      <MediaImage
        src={resolvedImageUrl}
        alt={alt}
        loading={priority ? "eager" : "lazy"}
        fetchPriority={priority ? "high" : "auto"}
        width={width}
        height={height}
        sizes={sizes}
        fallbackSrc={fallbackSrc}
        className="object-cover"
        style={imageStyle}
        onLoad={handleLoad}
        onError={handleError}
      />
    </div>
  );
}

// Memoize so a parent re-render with the same src does not retrigger the
// skeleton overlay or churn the underlying <img>. Without this, identical
// re-renders can flash the skeleton back over a fully-loaded poster.
const OptimizedImage = memo(OptimizedImageImpl, (prev, next) => (
  prev.src === next.src &&
  prev.alt === next.alt &&
  prev.className === next.className &&
  prev.priority === next.priority &&
  prev.showSkeleton === next.showSkeleton &&
  prev.aspectRatio === next.aspectRatio &&
  prev.width === next.width &&
  prev.height === next.height &&
  prev.sizes === next.sizes &&
  prev.fallbackSrc === next.fallbackSrc
));
export default OptimizedImage;
