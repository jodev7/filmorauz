"use client";

import type { CSSProperties, ImgHTMLAttributes, SyntheticEvent } from "react";
import { useEffect, useState } from "react";
import { usePathname } from "next/navigation";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";

interface MediaImageProps
  extends Omit<ImgHTMLAttributes<HTMLImageElement>, "src" | "alt" | "onError" | "onLoad"> {
  src?: string | null;
  alt: string;
  fallbackSrc?: string;
  onError?: (event: SyntheticEvent<HTMLImageElement, Event>) => void;
  onLoad?: (event: SyntheticEvent<HTMLImageElement, Event>) => void;
  style?: CSSProperties;
}

export default function MediaImage({
  src,
  alt,
  className = "",
  fallbackSrc = DEFAULT_POSTER_PLACEHOLDER,
  loading = "lazy",
  fetchPriority = "auto",
  onLoad,
  onError,
  ...imgProps
}: MediaImageProps) {
  const pathname = usePathname() || "";
  const resolvedUrl = normalizeMediaUrl(src, "");
  const resolvedFallback = normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER);
  const [failed, setFailed] = useState(false);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    setFailed(false);
    setLoaded(!resolvedUrl);
  }, [pathname, resolvedUrl]);

  const finalSrc = failed ? resolvedFallback : resolvedUrl;
  const loadingClassName = loaded
    ? "opacity-100"
    : "opacity-0 animate-pulse bg-gradient-to-br from-gray-800 to-gray-900";

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      key={`${pathname}-${resolvedUrl}`}
      src={finalSrc || undefined}
      alt={alt}
      loading={loading}
      fetchPriority={fetchPriority}
      decoding="async"
      onLoad={(event) => {
        setLoaded(true);
        onLoad?.(event);
      }}
      onError={(event) => {
        if (!failed && resolvedUrl && resolvedUrl !== resolvedFallback) {
          setFailed(true);
          setLoaded(false);
        } else {
          setLoaded(true);
        }
        onError?.(event);
      }}
      className={`${loadingClassName} transition-opacity duration-300 ${className}`.trim()}
      {...imgProps}
    />
  );
}
