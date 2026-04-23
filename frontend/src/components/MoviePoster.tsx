"use client";

import OptimizedImage from "./OptimizedImage";

interface Props {
  src: string;
  alt: string;
  className?: string;
  priority?: boolean;
  showSkeleton?: boolean;
  aspectRatio?: string;
}

export default function MoviePoster({
  src,
  alt,
  className = "",
  priority = false,
  showSkeleton = true,
  aspectRatio = "2/3"
}: Props) {
  return (
    <OptimizedImage
      src={src}
      alt={alt}
      className={className}
      priority={priority}
      showSkeleton={showSkeleton}
      aspectRatio={aspectRatio}
      onError={(e) => {
        (e.target as HTMLImageElement).src = "/placeholder-poster.jpg";
      }}
    />
  );
}
