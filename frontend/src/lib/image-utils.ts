const shimmer = `
  <svg width="1200" height="675" viewBox="0 0 1200 675" xmlns="http://www.w3.org/2000/svg">
    <defs>
      <linearGradient id="g">
        <stop stop-color="#151520" offset="20%" />
        <stop stop-color="#20202f" offset="50%" />
        <stop stop-color="#151520" offset="70%" />
      </linearGradient>
    </defs>
    <rect width="1200" height="675" fill="#151520" />
    <rect id="r" width="1200" height="675" fill="url(#g)" />
    <animate xlink:href="#r" attributeName="x" from="-1200" to="1200" dur="1.6s" repeatCount="indefinite" />
  </svg>
`;

export const DEFAULT_POSTER_PLACEHOLDER = "/placeholder-poster.jpg";

export const SHIMMER_BLUR_DATA_URL = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(shimmer)}`;

const B2_CDN_ABSOLUTE_PREFIXES = [
  "https://cdn.filmorauz.net/file/filmorauznet/",
  "https://f005.backblazeb2.com/file/filmorauznet/",
];
const B2_FILE_PATH_PREFIX = "/file/filmorauznet/";
const MEDIA_BUCKET_PREFIXES = ["/posters/", "/backdrops/", "/images/", "/ads/"];

function isInvalidValue(value: string): boolean {
  return !value || value === "null" || value === "undefined" || value === "-" || value === ".";
}

export function normalizeMediaUrl(
  src?: string | null,
  fallback: string = DEFAULT_POSTER_PLACEHOLDER
): string {
  const value = src?.trim();
  if (!value || isInvalidValue(value)) {
    return fallback;
  }

  if (value.startsWith("/media/")) {
    return value;
  }

  for (const prefix of B2_CDN_ABSOLUTE_PREFIXES) {
    if (value.startsWith(prefix)) {
      return "/media/" + value.slice(prefix.length).replace(/^\/+/, "");
    }
  }

  if (value.startsWith(B2_FILE_PATH_PREFIX)) {
    return "/media/" + value.slice(B2_FILE_PATH_PREFIX.length).replace(/^\/+/, "");
  }

  for (const prefix of MEDIA_BUCKET_PREFIXES) {
    if (value.startsWith(prefix)) {
      return "/media" + value;
    }
  }

  if (value.startsWith("//")) {
    return `https:${value}`;
  }

  if (
    value.startsWith("/") ||
    value.startsWith("http://") ||
    value.startsWith("https://") ||
    value.startsWith("data:") ||
    value.startsWith("blob:")
  ) {
    return value;
  }

  return fallback;
}

export function normalizeImageSrc(
  src?: string | null,
  fallback: string = DEFAULT_POSTER_PLACEHOLDER
): string {
  return normalizeMediaUrl(src, fallback);
}

export function isProtectedMediaUrl(src?: string | null): boolean {
  const value = src?.trim();
  return !!value && value.startsWith("/media/");
}
