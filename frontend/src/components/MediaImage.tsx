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
  const resolvedSrc = normalizeMediaUrl(src, fallbackSrc);
  const resolvedFallback = normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER);
  const [errored, setErrored] = useState(false);

  // Reset error state when the resolved URL changes so a new src gets a fresh load attempt.
  useEffect(() => {
    setErrored(false);
  }, [resolvedSrc]);

  const finalSrc = errored ? resolvedFallback : resolvedSrc;

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      key={finalSrc}
      src={finalSrc}
      alt={alt}
      loading={loading}
      fetchPriority={fetchPriority}
      onError={(event) => {
        if (!errored && finalSrc !== resolvedFallback) {
          setErrored(true);
        }
        onError?.(event);
      }}
      className={className}
    />
  );
}
