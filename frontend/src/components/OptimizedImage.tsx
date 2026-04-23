"use client";

import { useState, useEffect } from "react";

interface OptimizedImageProps {
  src: string;
  alt: string;
  className?: string;
  priority?: boolean;
  showSkeleton?: boolean;
  aspectRatio?: string;
  onError?: (e: React.SyntheticEvent<HTMLImageElement, Event>) => void;
}

export default function OptimizedImage({
  src,
  alt,
  className = "",
  priority = false,
  showSkeleton = true,
  aspectRatio = "2/3",
  onError
}: OptimizedImageProps) {
  const [isLoading, setIsLoading] = useState(true);
  const [hasError, setHasError] = useState(false);

  // Reset loading state when src changes
  useEffect(() => {
    setIsLoading(true);
    setHasError(false);
  }, [src]);

  const handleLoad = () => {
    setIsLoading(false);
  };

  const handleError = (e: React.SyntheticEvent<HTMLImageElement, Event>) => {
    setIsLoading(false);
    setHasError(true);
    onError?.(e);
  };

  return (
    <div className={`relative overflow-hidden ${className}`}>
      {/* Skeleton/Placeholder */}
      {showSkeleton && isLoading && !hasError && (
        <div
          className={`absolute inset-0 bg-gradient-to-br from-gray-800 to-gray-900 animate-pulse`}
          style={{ aspectRatio }}
        >
          <div className="absolute inset-0 bg-gradient-to-r from-transparent via-white/5 to-transparent animate-shimmer" />
        </div>
      )}

      {/* Image */}
      <img
        src={src}
        alt={alt}
        className={`w-full h-full object-cover transition-opacity duration-300 ${
          isLoading ? 'opacity-0' : 'opacity-100'
        }`}
        loading={priority ? "eager" : "lazy"}
        fetchPriority={priority ? "high" : "auto"}
        decoding="async"
        onLoad={handleLoad}
        onError={handleError}
      />
    </div>
  );
}

// Shimmer animation for loading state
const shimmerStyles = `
  @keyframes shimmer {
    0% {
      transform: translateX(-100%);
    }
    100% {
      transform: translateX(100%);
    }
  }
  .animate-shimmer {
    animation: shimmer 1.5s infinite;
  }
`;

// Inject shimmer styles on component mount
if (typeof document !== 'undefined') {
  const styleId = 'optimized-image-styles';
  if (!document.getElementById(styleId)) {
    const style = document.createElement('style');
    style.id = styleId;
    style.textContent = shimmerStyles;
    document.head.appendChild(style);
  }
}