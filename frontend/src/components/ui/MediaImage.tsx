"use client";

import type { CSSProperties, ImgHTMLAttributes, SyntheticEvent } from "react";
import { memo, useEffect, useMemo, useState } from "react";
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

function MediaImageImpl({
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
  // Memoize on the raw src prop, not the resolved value, so we don't re-run
  // normalizeMediaUrl on every render. The output is otherwise identical for
  // identical input, but skipping it keeps the dep array stable and avoids
  // any chance of an effect retriggering from a transient string identity.
  const resolvedUrl = useMemo(() => normalizeMediaUrl(src, ""), [src]);
  const resolvedFallback = useMemo(
    () => normalizeMediaUrl(fallbackSrc, DEFAULT_POSTER_PLACEHOLDER),
    [fallbackSrc],
  );

  const [failed, setFailed] = useState(false);
  // Treat empty URLs as already "loaded" so we don't sit on opacity-0
  // forever waiting for an onLoad that will never fire.
  const [loaded, setLoaded] = useState(() => !resolvedUrl);

  // Reset transient state ONLY when the resolved URL actually changes. No
  // pathname, no Date.now, no random — anything that doesn't change here
  // must not cause the image to fade out.
  useEffect(() => {
    setFailed(false);
    setLoaded(!resolvedUrl);
  }, [resolvedUrl]);

  const finalSrc = failed ? resolvedFallback : resolvedUrl;
  const loadingClassName = loaded
    ? "opacity-100"
    : "opacity-0 animate-pulse bg-gradient-to-br from-gray-800 to-gray-900";

  return (
    // eslint-disable-next-line @next/next/no-img-element
    <img
      key={resolvedUrl}
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

// Wrap in memo so identical-prop re-renders from above don't cascade into
// the image element. Without this, every parent re-render rebuilds the
// className string and the inner <img> can churn.
const MediaImage = memo(MediaImageImpl);
export default MediaImage;
