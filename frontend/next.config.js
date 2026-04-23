/** @type {import('next').NextConfig} */
const nextConfig = {
  // Gzip/Brotli responses at the Node layer so bundles and API proxies stream smaller.
  compress: true,
  // Strip "Powered by Next.js" noise from every response.
  poweredByHeader: false,
  // Generate stable asset ids so CDNs get clean cache keys between builds that
  // don't actually change content.
  generateEtags: true,
  // Cut CSS bytes modestly across the tree.
  productionBrowserSourceMaps: false,
  reactStrictMode: true,

  images: {
    remotePatterns: [
      { protocol: "https", hostname: "**" },
    ],
    // Long cache for the Next/Image optimizer output (used where we switch to
    // next/image). Poster/backdrop CDN URLs are served via plain <img> and
    // caching is controlled by the origin/CDN, not this value.
    minimumCacheTTL: 60 * 60 * 24 * 30, // 30 days
    formats: ["image/avif", "image/webp"],
    deviceSizes: [360, 480, 640, 750, 828, 1080, 1200, 1536, 1920],
    imageSizes: [64, 96, 128, 180, 240, 320, 384],
  },

  async headers() {
    const oneYear = "public, max-age=31536000, immutable";
    return [
      // Next.js build output is content-hashed — safe to cache forever.
      {
        source: "/_next/static/:path*",
        headers: [{ key: "Cache-Control", value: oneYear }],
      },
      // Static public assets (icons, fonts, og images).
      {
        source: "/:path*.(ico|png|jpg|jpeg|webp|avif|svg|gif|woff|woff2|ttf|otf)",
        headers: [{ key: "Cache-Control", value: oneYear }],
      },
      // Next.js image optimizer output — long TTL, revalidate if origin changes.
      {
        source: "/_next/image:path*",
        headers: [
          { key: "Cache-Control", value: "public, max-age=2592000, stale-while-revalidate=604800" },
        ],
      },
    ];
  },
};

module.exports = nextConfig;
