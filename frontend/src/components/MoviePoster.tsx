"use client";

import OptimizedImage from "./OptimizedImage";
import { DEFAULT_POSTER_PLACEHOLDER } from "@/lib/image-utils";

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
      fallbackSrc={DEFAULT_POSTER_PLACEHOLDER}
    />
  );
}
