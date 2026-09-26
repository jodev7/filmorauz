/* FilmoraUz service worker.
 *
 * Deliberately conservative:
 *  - only same-origin GET requests are handled; the API, CDN video/HLS,
 *    images from other hosts, analytics etc. always go straight to network;
 *  - hashed build assets (/_next/static) and icons: cache-first;
 *  - page navigations: network-first, falling back to the last cached copy
 *    of that page, then to /offline.html.
 */
const VERSION = "v1";
const STATIC_CACHE = `filmora-static-${VERSION}`;
const PAGE_CACHE = `filmora-pages-${VERSION}`;
const OFFLINE_URL = "/offline.html";

self.addEventListener("install", (event) => {
  event.waitUntil(
    caches
      .open(STATIC_CACHE)
      .then((cache) => cache.addAll([OFFLINE_URL, "/icon-192.png", "/icon-512.png"]))
      .then(() => self.skipWaiting())
  );
});

self.addEventListener("activate", (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) =>
        Promise.all(
          keys
            .filter((k) => k.startsWith("filmora-") && k !== STATIC_CACHE && k !== PAGE_CACHE)
            .map((k) => caches.delete(k))
        )
      )
      .then(() => self.clients.claim())
  );
});

function isStaticAsset(url) {
  return url.pathname.startsWith("/_next/static/") || /\.(png|svg|ico|woff2?)$/.test(url.pathname);
}

self.addEventListener("fetch", (event) => {
  const req = event.request;
  if (req.method !== "GET") return;
  const url = new URL(req.url);
  if (url.origin !== self.location.origin) return; // API, CDN, video, analytics
  if (url.pathname.startsWith("/api/") || url.pathname.startsWith("/admin")) return;

  if (isStaticAsset(url)) {
    event.respondWith(
      caches.open(STATIC_CACHE).then(async (cache) => {
        const hit = await cache.match(req);
        if (hit) return hit;
        const res = await fetch(req);
        if (res.ok) cache.put(req, res.clone());
        return res;
      })
    );
    return;
  }

  if (req.mode === "navigate") {
    event.respondWith(
      (async () => {
        try {
          const res = await fetch(req);
          if (res.ok) {
            const cache = await caches.open(PAGE_CACHE);
            cache.put(req, res.clone());
          }
          return res;
        } catch {
          const cached = await caches.match(req);
          return cached || caches.match(OFFLINE_URL);
        }
      })()
    );
  }
});
