"use client";

import { useEffect, useState } from "react";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";

interface MediaImageProps {
  src?: string | null;
  alt: string;
  className?: string;
  fallbackSrc?: string;
  loading?: "eager" | "lazy";
  fetchPriority?: "high" | "low" | "auto";
  onError?: (event: React.SyntheticEvent<HTMLImageElement, Event>) => void;
}

export default function MediaImage({
  src,
  alt,
  className = "",
  fallbackSrc = DEFAULT_POSTER_PLACEHOLDER,
  loading = "lazy",
  fetchPriority = "auto",
  onError,
}: MediaImageProps) {
  const [currentSrc, setCurrentSrc] = useState(() => normalizeMediaUrl(src, fallbackSrc));

  useEffect(() => {
    setCurrentSrc(normalizeMediaUrl(src, fallbackSrc));
  }, [src, fallbackSrc]);

  const handleError = (event: React.SyntheticEvent<HTMLImageElement, Event>) => {
    const nextSrc = normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER);
    if (currentSrc !== nextSrc) {
      setCurrentSrc(nextSrc);
      event.currentTarget.src = nextSrc;
    }
    onError?.(event);
  };

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      src={currentSrc}
      alt={alt}
      loading={loading}
      fetchPriority={fetchPriority}
      onError={handleError}
      className={className}
    />
  );
}
