"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { X } from "lucide-react";
import { Ad, getAdsForWebsite, recordAdImpression, recordAdClick } from "@/lib/api";
import { pickWeightedRandomAd, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";

interface WebsiteAdSlotProps {
  placement: string;
  className?: string;
  popup?: boolean;
  variant?: "banner" | "inline" | "card";
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
}: WebsiteAdSlotProps) {
  const { user, isLoading: authLoading } = useAuth();
  const [ads, setAds] = useState<Ad[]>([]);
  const [dismissed, setDismissed] = useState(false);
  const [current, setCurrent] = useState(0);
  const [isReady, setIsReady] = useState(false);
  const adsRef = useRef<Ad[]>([]);
  const currentRef = useRef(0);
  const fetchedRef = useRef(false);
  const impressedRef = useRef<Set<string>>(new Set());
  const lastImpressionRef = useRef<string>("");

  const fetchAds = useCallback(async () => {
    if (fetchedRef.current) return;
    fetchedRef.current = true;
    try {
      const data = await getAdsForWebsite(placement);
      setAds(data);
      adsRef.current = data;
      setCurrent(0);
      currentRef.current = 0;
      impressedRef.current = new Set();
      setIsReady(true);
    } catch {
      fetchedRef.current = false;
    }
  }, [placement]);

  useEffect(() => {
    fetchAds();
    return () => {
      fetchedRef.current = false;
    };
  }, [fetchAds]);

  useEffect(() => {
    if (ads.length <= 1 || !isReady) return;
    const interval = setInterval(() => {
      const currentAdId = adsRef.current[currentRef.current]?.id;
      const picked = pickWeightedRandomAd(adsRef.current, currentAdId);
      const idx = adsRef.current.indexOf(picked);
      currentRef.current = idx;
      setCurrent(idx);
    }, 5000);
    return () => clearInterval(interval);
  }, [ads.length, isReady]);

  useEffect(() => {
    if (ads.length === 0 || !isReady) return;
    const ad = ads[current % ads.length];
    const impressionKey = `${placement}:${ad.id}:${current}`;
    if (impressedRef.current.has(ad.id) || lastImpressionRef.current === impressionKey) return;
    lastImpressionRef.current = impressionKey;
    impressedRef.current.add(ad.id);
    recordAdImpression(ad.id).catch(() => {});
  }, [ads, current, isReady, placement]);

  if (authLoading || isUserPremium(user)) return null;
  if (ads.length === 0 || dismissed) return null;

  const ad = ads[current % ads.length];

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  // ── Popup ──────────────────────────────────────────────────────────────────
  if (popup) {
    const media = resolveMedia(ad, "popup");
    return (
      <div
        className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
        onClick={(e) => { if (e.target === e.currentTarget) setDismissed(true); }}
      >
        <div
          className="relative w-full max-w-sm rounded-xl overflow-hidden shadow-2xl border border-brand-border"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            onClick={() => setDismissed(true)}
            className="absolute top-2 right-2 z-10 p-1.5 rounded-full bg-black/50 text-white hover:bg-black/80 transition"
            aria-label="Close ad"
          >
            <X size={14} />
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
  const media = resolveMedia(ad, variant);

  return (
    <div className={`w-full ${className}`}>
      <div
        className="relative w-full rounded-lg overflow-hidden border border-brand-border cursor-pointer hover:border-brand-red/50 transition-colors"
        onClick={handleClick}
      >
        <div className={`relative w-full ${mediaHeight} overflow-hidden`}>
          <AdMedia url={media.url} type={media.type} />
        </div>
        <span className="absolute top-1.5 right-1.5 text-[9px] text-gray-500 bg-black/60 px-1 py-0.5 rounded pointer-events-none uppercase tracking-wide">
          Ad
        </span>
      </div>
    </div>
  );
}

// ── Helpers ────────────────────────────────────────────────────────────────

function resolveMedia(
  ad: Ad,
  variant: "banner" | "inline" | "card" | "popup",
): { url: string; type: "image" | "video" } {
  // Priority chain per variant → final fallback: image_url
  let url: string | undefined;
  let type: "image" | "video" | undefined;

  if (variant === "banner") {
    url  = ad.banner_media_url;
    type = ad.banner_media_type;
  } else if (variant === "inline" || variant === "card") {
    url  = ad.inline_media_url  || ad.banner_media_url;
    type = ad.inline_media_type || ad.banner_media_type;
  } else if (variant === "popup") {
    url  = ad.popup_media_url   || ad.banner_media_url || ad.inline_media_url;
    type = ad.popup_media_type  || ad.banner_media_type || ad.inline_media_type;
  }

  return {
    url:  url  || ad.image_url || "",
    type: type || "image",
  };
}

function AdMedia({ url, type }: { url: string; type: "image" | "video" }) {
  if (!url) return null;

  if (type === "video") {
    return (
      <video
        src={url}
        muted
        loop
        playsInline
        autoPlay
        preload="metadata"
        className="absolute inset-0 w-full h-full object-cover object-center"
      />
    );
  }

  return (
    <img
      src={url}
      alt="Ad"
      className="absolute inset-0 w-full h-full object-cover object-center"
      loading="lazy"
    />
  );
}
