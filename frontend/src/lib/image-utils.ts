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

const MEDIA_ACCESS_MODE = (() => {
  const configured =
    process.env.NEXT_PUBLIC_MEDIA_ACCESS_MODE?.trim() ||
    process.env.MEDIA_ACCESS_MODE?.trim();
  return (configured || "protected").toLowerCase();
})();

const CDN_MEDIA_BASE = (() => {
  const configured = process.env.NEXT_PUBLIC_CDN_MEDIA_BASE?.trim();
  if (configured) {
    return configured.replace(/\/+$/, "");
  }
  return process.env.NODE_ENV === "production" ? "https://cdn.filmorauz.net" : "";
})();

const CDN_FILE_BASE = (() => {
  const configured =
    process.env.NEXT_PUBLIC_CDN_BASE_URL?.trim() ||
    process.env.CDN_BASE_URL?.trim();
  if (configured) {
    return configured.replace(/\/+$/, "");
  }
  return process.env.NODE_ENV === "production"
    ? "https://cdn.filmorauz.net/file/filmorauznet"
    : "";
})();

const B2_CDN_ABSOLUTE_PREFIXES = [
  "https://cdn.filmorauz.net/file/filmorauznet/",
  "https://f005.backblazeb2.com/file/filmorauznet/",
];
const B2_FILE_PATH_PREFIX = "/file/filmorauznet/";
const MEDIA_PATH_MARKER = "/media/";
const MEDIA_BUCKET_PREFIXES = [
  "/images/",
  "/videos/",
  "/movies/",
  "/series/",
  "/collections/",
  "/suggestions/",
];
const LEGACY_AVATAR_PREFIX = "/avatars/";
const LEGACY_PROFILE_PREFIX = "/profile/";
const CANONICAL_AVATAR_PREFIX = "/images/profile/";
const LEGACY_POSTER_PREFIX = "/posters/";
const CANONICAL_POSTER_PREFIX = "/images/posters/";
const LEGACY_BACKDROP_PREFIX = "/backdrops/";
const CANONICAL_BACKDROP_PREFIX = "/images/backdrops/";
const LEGACY_AD_PREFIX = "/ads/";
const CANONICAL_AD_PREFIX = "/images/ads/";
const LEGACY_TELEGRAM_POST_PREFIX = "/telegram-posts/";
const CANONICAL_TELEGRAM_POST_PREFIX = "/images/telegram-posts/";
const LEGACY_COLLECTIONS_PREFIX = "/collections/";
const CANONICAL_COLLECTIONS_PREFIX = "/images/collections/";
const LEGACY_SUGGESTIONS_PREFIX = "/suggestions/";
const CANONICAL_SUGGESTIONS_PREFIX = "/images/suggestions/";

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

function stripAbsoluteMediaPrefix(value: string): string | null {
  try {
    const parsed = new URL(value);
    if (!parsed.pathname.startsWith(MEDIA_PATH_MARKER)) {
      return null;
    }
    return parsed.pathname + parsed.search;
  } catch {
    return null;
  }
}

function rewriteMediaAlias(path: string, legacyPrefix: string, canonicalPrefix: string): string {
  if (path.startsWith(legacyPrefix)) {
    return canonicalPrefix + path.slice(legacyPrefix.length);
  }
  return path;
}

function rewriteLegacyMediaAliases(path: string): string {
  let normalized = path;
  normalized = rewriteMediaAlias(normalized, LEGACY_AVATAR_PREFIX, CANONICAL_AVATAR_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_PROFILE_PREFIX, CANONICAL_AVATAR_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_POSTER_PREFIX, CANONICAL_POSTER_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_BACKDROP_PREFIX, CANONICAL_BACKDROP_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_AD_PREFIX, CANONICAL_AD_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_TELEGRAM_POST_PREFIX, CANONICAL_TELEGRAM_POST_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_COLLECTIONS_PREFIX, CANONICAL_COLLECTIONS_PREFIX);
  normalized = rewriteMediaAlias(normalized, LEGACY_SUGGESTIONS_PREFIX, CANONICAL_SUGGESTIONS_PREFIX);
  return normalized;
}

function maybeUseCDNMediaHost(path: string): string {
  if (MEDIA_ACCESS_MODE === "public") {
    const objectPath = path.startsWith("/media/") ? path.slice("/media".length) : path;
    if (!CDN_FILE_BASE || !objectPath.startsWith("/")) {
      return objectPath || path;
    }
    return `${CDN_FILE_BASE}${objectPath}`;
  }
  if (!CDN_MEDIA_BASE || !path.startsWith("/media/")) {
    return path;
  }
  return `${CDN_MEDIA_BASE}${path}`;
}

export function normalizeMediaUrl(
  src?: string | null,
  fallback: string = DEFAULT_POSTER_PLACEHOLDER
): string {
  const value = src?.trim();
  if (!value || isInvalidValue(value)) {
    return fallback;
  }

  const absoluteMediaPath = stripAbsoluteMediaPrefix(value);
  if (absoluteMediaPath) {
    const normalizedMediaPath = rewriteLegacyMediaAliases(absoluteMediaPath.slice("/media".length));
    return maybeUseCDNMediaHost("/media" + normalizedMediaPath);
  }

  if (value.startsWith("/media/")) {
    const normalizedMediaPath = rewriteLegacyMediaAliases(value.slice("/media".length));
    return maybeUseCDNMediaHost("/media" + normalizedMediaPath);
  }

  // Bare "avatars/<file>" (no leading slash) — never appears from a URL parser,
  // but some older DB records store it this way.
  if (value.startsWith("avatars/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_AVATAR_PREFIX + value.slice("avatars/".length));
  }
  if (value.startsWith("profile/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_AVATAR_PREFIX + value.slice("profile/".length));
  }
  if (value.startsWith("posters/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_POSTER_PREFIX + value.slice("posters/".length));
  }
  if (value.startsWith("backdrops/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_BACKDROP_PREFIX + value.slice("backdrops/".length));
  }
  if (value.startsWith("ads/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_AD_PREFIX + value.slice("ads/".length));
  }
  if (value.startsWith("telegram-posts/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_TELEGRAM_POST_PREFIX + value.slice("telegram-posts/".length));
  }
  if (value.startsWith("collections/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_COLLECTIONS_PREFIX + value.slice("collections/".length));
  }
  if (value.startsWith("suggestions/")) {
    return maybeUseCDNMediaHost("/media" + CANONICAL_SUGGESTIONS_PREFIX + value.slice("suggestions/".length));
  }

  // Strip the B2 absolute host or /file/filmorauznet/ prefix so the rest of
  // the rules work on bucket-relative paths like "/posters/x.jpg".
  const stripped = stripB2Prefix(value);
  let path = stripped ?? value;

  if (
    !path.startsWith("/") &&
    !path.startsWith("http://") &&
    !path.startsWith("https://") &&
    !path.startsWith("data:") &&
    !path.startsWith("blob:")
  ) {
    path = "/" + path.replace(/^\/+/, "");
  }

  path = rewriteLegacyMediaAliases(path);

  for (const prefix of MEDIA_BUCKET_PREFIXES) {
    if (path.startsWith(prefix)) {
      return maybeUseCDNMediaHost(MEDIA_ACCESS_MODE === "public" ? path : "/media" + path);
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
  if (MEDIA_ACCESS_MODE === "public") return false;
  const value = src?.trim();
  if (!value) return false;
  if (value.startsWith("/media/")) return true;
  return stripAbsoluteMediaPrefix(value) !== null;
}
