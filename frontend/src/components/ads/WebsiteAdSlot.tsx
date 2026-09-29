"use client";

import { useEffect, useState, useRef, useMemo } from "react";
import { usePathname } from "next/navigation";
import { X } from "lucide-react";
import { recordAdImpression, recordAdClick } from "@/lib/api";
import { isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import MediaImage from "@/components/MediaImage";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

interface WebsiteAdSlotProps {
  placement: string;
  className?: string;
  popup?: boolean;
  variant?: "banner" | "inline" | "card";
  lazy?: boolean;
}

const SLOT_HEIGHT: Record<string, string> = {
  banner: "h-[300px]",
  inline: "h-[400px]",
  card:   "h-[200px]",
};
export default function WebsiteAdSlot({
  placement,
  className = "",
  popup = false,
  variant = "inline",
  lazy = false,
}: WebsiteAdSlotProps) {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const [dismissed, setDismissed] = useState(false);
  const [current, setCurrent] = useState(0);
  const impressedRef = useRef<Set<string>>(new Set());
  const lastImpressionRef = useRef<string>("");
  const containerRef = useRef<HTMLDivElement>(null);
  const [shouldLoad, setShouldLoad] = useState(!lazy);

  const placements = useMemo(
    () => getSlotPlacements(placement, variant, popup),
    [placement, variant, popup],
  );

  // Ads always derive from the shared context — no local state, no stale
  // empty-array bug, no duplicate fetch loops.
  const ads = useMemo(
    () => getMergedAds(placements).filter((ad) => getWebsiteAdMedia(ad, popup ? "popup" : variant)),
    [getMergedAds, placements, popup, variant],
  );
  const isReady = ads.length > 0;

  // Viewport lazy-load trigger
  useEffect(() => {
    if (!lazy) return;
    const node = containerRef.current;
    if (!node) return;

    const observer = new IntersectionObserver(
      (entries) => {
        if (entries.some((entry) => entry.isIntersecting)) {
          setShouldLoad(true);
          observer.disconnect();
        }
      },
      { rootMargin: "300px 0px" },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, [lazy]);

  useEffect(() => {
    if (!shouldLoad) return;
    ensurePlacements(placements).catch(() => {});
  }, [ensurePlacements, placements, shouldLoad]);

  useEffect(() => {
    if (ads.length === 0 || !isReady) return;
    const ad = ads[current % ads.length];
    if (!ad) return;
    const impressionKey = `${placement}:${ad.id}:${current}`;
    if (impressedRef.current.has(ad.id) || lastImpressionRef.current === impressionKey) return;
    lastImpressionRef.current = impressionKey;
    impressedRef.current.add(ad.id);
    recordAdImpression(ad.id).catch(() => {});
  }, [ads, current, isReady, placement]);

  if (pathname.startsWith("/premium")) return <div ref={containerRef} />;
  if (authLoading || isUserPremium(user)) return null;
  if (!shouldLoad) return <div ref={containerRef} className={`w-full ${className}`} />;
  if (ads.length === 0 || dismissed) return <div ref={containerRef} className={`w-full ${className}`} />;

  const ad = ads[current % ads.length];

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  // ── Popup ──────────────────────────────────────────────────────────────────
  if (popup) {
    const media = getWebsiteAdMedia(ad, "popup")!;
    return (
      <div
        className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
        onClick={(e) => { if (e.target === e.currentTarget) setDismissed(true); }}
        role="dialog"
        aria-modal="true"
        aria-label="Reklama"
      >
        <div
          className="relative w-full max-w-sm rounded-xl overflow-hidden shadow-2xl border border-brand-border"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            onClick={() => setDismissed(true)}
            className="absolute top-2 right-2 z-10 p-1.5 rounded-full bg-black/50 text-white hover:bg-black/80 transition"
            aria-label="Reklamani yopish"
          >
            <X size={14} aria-hidden="true" />
          </button>
          <div className="relative w-full h-[500px] overflow-hidden cursor-pointer" onClick={handleClick}>
            <AdMedia url={media.url} type={media.type} />
          </div>
        </div>
      </div>
    );
  }

  // ── Banner / Inline ────────────────────────────────────────────────────────
  const mediaHeight = SLOT_HEIGHT[variant] ?? "h-[300px]";
  const media = getWebsiteAdMedia(ad, variant)!;

  return (
    <div ref={containerRef} className={`w-full ${className}`}>
      <div
        className="relative w-full rounded-lg overflow-hidden border border-brand-border cursor-pointer hover:border-brand-red/50 transition-colors"
        onClick={handleClick}
        role="link"
        aria-label="Reklamani ochish"
      >
        <div className={`relative w-full ${mediaHeight} overflow-hidden`}>
          <AdMedia url={media.url} type={media.type} />
        </div>
        <span className="absolute top-1.5 right-1.5 text-[9px] text-gray-300 bg-black/70 px-1 py-0.5 rounded pointer-events-none uppercase tracking-wide">
          Ad
        </span>
      </div>
    </div>
  );
}

function getSlotPlacements(
  placement: string,
  variant: "banner" | "inline" | "card",
  popup: boolean,
): string[] {
  if (popup) {
    return [placement, "website"];
  }

  const shared = ["website"];
  if (variant === "banner") {
    shared.push("global_banner");
  } else {
    shared.push("global_inline");
  }

  return [placement, ...shared];
}

// ── Helpers ────────────────────────────────────────────────────────────────

function AdMedia({ url, type }: { url: string; type: "image" | "video" }) {
  if (!url) return null;

  if (type === "video") {
    return (
      <video
        src={normalizeMediaUrl(url, "")}
        muted
        loop
        playsInline
        autoPlay
        preload="metadata"
        className="absolute inset-0 w-full h-full object-cover object-center"
      />
    );
  }

  const normalizedUrl = normalizeMediaUrl(url, "/og-image.jpg");
  return (
    <MediaImage
      src={normalizedUrl}
      alt=""
      className="absolute inset-0 h-full w-full object-cover object-center"
    />
  );
}
