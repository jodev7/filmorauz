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

// Only NEXT_PUBLIC_* vars — otherwise server and client resolve differently
// and client-side navigation produces URLs that don't match what SSR rendered.
const CDN_FILE_BASE = (() => {
  const configured = process.env.NEXT_PUBLIC_CDN_BASE_URL?.trim();
  if (configured) {
    return configured.replace(/\/+$/, "");
  }
  return process.env.NODE_ENV === "production"
    ? "https://cdn.filmorauz.net/file/filmorauznet"
    : "";
})();

const MEDIA_ACCESS_MODE = (() => {
  const configured = process.env.NEXT_PUBLIC_MEDIA_ACCESS_MODE?.trim().toLowerCase();
  if (configured) return configured;
  // If a public CDN is configured, default to public mode.
  return CDN_FILE_BASE ? "public" : "protected";
})();

const CDN_MEDIA_BASE = (() => {
  const configured = process.env.NEXT_PUBLIC_CDN_MEDIA_BASE?.trim();
  if (configured) {
    return configured.replace(/\/+$/, "");
  }
  return process.env.NODE_ENV === "production" ? "https://cdn.filmorauz.net" : "";
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

// Canonical B2 paths use /images/<kind>/... — rewrite legacy short prefixes to match.
const LEGACY_TO_CANONICAL: Array<[string, string]> = [
  ["/avatars/", "/images/profile/"],
  ["/profile/", "/images/profile/"],
  ["/posters/", "/images/posters/"],
  ["/backdrops/", "/images/backdrops/"],
  ["/ads/", "/images/ads/"],
  ["/telegram-posts/", "/images/telegram-posts/"],
  ["/collections/", "/images/collections/"],
  ["/suggestions/", "/images/suggestions/"],
];

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

function rewriteLegacyMediaAliases(path: string): string {
  for (const [legacy, canonical] of LEGACY_TO_CANONICAL) {
    if (path.startsWith(legacy)) {
      return canonical + path.slice(legacy.length);
    }
  }
  return path;
}

// In public mode, drop any legacy ?token=... or signed-URL query from B2 URLs
// because public objects don't need auth and stale tokens make images 403.
function stripQueryInPublicMode(value: string): string {
  if (MEDIA_ACCESS_MODE !== "public") return value;
  const queryIdx = value.indexOf("?");
  return queryIdx === -1 ? value : value.slice(0, queryIdx);
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

// Hosts whose images can't be loaded directly by the browser (self-signed TLS
// like uzmedia.tv's CN=nohttps cert, or HTTP-only origins blocked as mixed
// content from an HTTPS page). Routed through the backend /api/proxy-image
// endpoint, which fetches them server-side and re-serves over HTTPS.
const PROXY_HOSTS = new Set([
  "uzmedia.tv",
  "www.uzmedia.tv",
]);

const API_BASE = (process.env.NEXT_PUBLIC_API_URL || "").replace(/\/+$/, "");

function maybeProxyExternalImage(value: string): string {
  if (!value.startsWith("http://") && !value.startsWith("https://")) return value;
  try {
    const u = new URL(value);
    const host = u.host.toLowerCase().split(":")[0];
    if (PROXY_HOSTS.has(host)) {
      // NEXT_PUBLIC_API_URL already includes the /api suffix in this project
      // (e.g. "https://api.filmorauz.net/api"). Fall back to a same-origin
      // /api path when the env var is absent (local dev with proxying).
      const base = API_BASE || "/api";
      return `${base}/proxy-image?url=${encodeURIComponent(value)}`;
    }
  } catch {
    /* not a parseable URL — leave it alone */
  }
  return value;
}

// Bare-path shortcut table: catches DB values stored without a leading slash
// (e.g. "posters/x.jpg", "avatars/x.jpg", "images/posters/x.jpg").
const BARE_PATH_REWRITES: Array<[string, string]> = [
  ["avatars/", "/images/profile/"],
  ["profile/", "/images/profile/"],
  ["posters/", "/images/posters/"],
  ["backdrops/", "/images/backdrops/"],
  ["ads/", "/images/ads/"],
  ["telegram-posts/", "/images/telegram-posts/"],
  ["collections/", "/images/collections/"],
  ["suggestions/", "/images/suggestions/"],
  ["images/", "/images/"],
  ["videos/", "/videos/"],
];

export function normalizeMediaUrl(
  src?: string | null,
  fallback: string = DEFAULT_POSTER_PLACEHOLDER
): string {
  const raw = src?.trim();
  if (!raw || isInvalidValue(raw)) {
    return fallback;
  }

  const value = stripQueryInPublicMode(raw);

  const absoluteMediaPath = stripAbsoluteMediaPrefix(value);
  if (absoluteMediaPath) {
    const normalizedMediaPath = rewriteLegacyMediaAliases(absoluteMediaPath.slice("/media".length));
    return maybeUseCDNMediaHost("/media" + normalizedMediaPath);
  }

  if (value.startsWith("/media/")) {
    const normalizedMediaPath = rewriteLegacyMediaAliases(value.slice("/media".length));
    return maybeUseCDNMediaHost("/media" + normalizedMediaPath);
  }

  // Bare paths (no leading slash) — some DB records store it this way.
  for (const [prefix, canonical] of BARE_PATH_REWRITES) {
    if (value.startsWith(prefix)) {
      const objectPath = canonical + value.slice(prefix.length);
      return maybeUseCDNMediaHost(MEDIA_ACCESS_MODE === "public" ? objectPath : "/media" + objectPath);
    }
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
    return maybeProxyExternalImage(value);
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
