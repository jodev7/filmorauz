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

export const DEFAULT_POSTER_PLACEHOLDER = "/placeholder-poster.png";
export const DEFAULT_AVATAR_PLACEHOLDER = "/placeholder-avatar.png";

export const SHIMMER_BLUR_DATA_URL = `data:image/svg+xml;charset=utf-8,${encodeURIComponent(shimmer)}`;

const B2_CDN_ABSOLUTE_PREFIXES = [
  "https://cdn.filmorauz.net/file/filmorauznet/",
  "https://f005.backblazeb2.com/file/filmorauznet/",
];
const B2_FILE_PATH_PREFIX = "/file/filmorauznet/";
const MEDIA_BUCKET_PREFIXES = ["/posters/", "/backdrops/", "/images/", "/ads/", "/telegram-posts/"];
const LEGACY_AVATAR_PREFIX = "/avatars/";
const CANONICAL_AVATAR_PREFIX = "/images/profile/";

function isInvalidValue(value: string): boolean {
  return !value || value === "null" || value === "undefined" || value === "-" || value === ".";
}

function stripB2Prefix(value: string): string | null {
  for (const prefix of B2_CDN_ABSOLUTE_PREFIXES) {
    if (value.startsWith(prefix)) {
      return "/" + value.slice(prefix.length).replace(/^\/+/, "");
    }
  }
  if (value.startsWith(B2_FILE_PATH_PREFIX)) {
    return "/" + value.slice(B2_FILE_PATH_PREFIX.length).replace(/^\/+/, "");
  }
  return null;
}

function rewriteAvatarAlias(path: string): string {
  if (path.startsWith(LEGACY_AVATAR_PREFIX)) {
    return CANONICAL_AVATAR_PREFIX + path.slice(LEGACY_AVATAR_PREFIX.length);
  }
  return path;
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

  // Bare "avatars/<file>" (no leading slash) — never appears from a URL parser,
  // but some older DB records store it this way.
  if (value.startsWith("avatars/")) {
    return "/media" + CANONICAL_AVATAR_PREFIX + value.slice("avatars/".length);
  }

  // Strip the B2 absolute host or /file/filmorauznet/ prefix so the rest of
  // the rules work on bucket-relative paths like "/posters/x.jpg".
  const stripped = stripB2Prefix(value);
  let path = stripped ?? value;

  path = rewriteAvatarAlias(path);

  for (const prefix of MEDIA_BUCKET_PREFIXES) {
    if (path.startsWith(prefix)) {
      return "/media" + path;
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
