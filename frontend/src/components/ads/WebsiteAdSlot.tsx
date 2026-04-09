"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { usePathname } from "next/navigation";
import { X } from "lucide-react";
import { Ad, getAdsForWebsite, recordAdImpression, recordAdClick } from "@/lib/api";
import { useAdSlot, IntrusiveAdType } from "./AdSlotContext";
import { shouldShowAds } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";

interface WebsiteAdSlotProps {
  placement: string;
  className?: string;
  popup?: boolean;
  intrusiveType?: IntrusiveAdType;
  variant?: "banner" | "inline" | "card";
}

const VARIANT_CONFIG = {
  banner: {
    container: "w-full max-w-3xl mx-auto",
    mediaClass: "aspect-[21/9] max-h-[120px] min-h-[80px]",
    contentClass: "p-2",
  },
  inline: {
    container: "w-full max-w-md mx-auto",
    mediaClass: "aspect-video max-h-[180px] min-h-[120px]",
    contentClass: "p-3",
  },
  card: {
    container: "w-full max-w-sm mx-auto",
    mediaClass: "aspect-[4/3] max-h-[200px] min-h-[120px]",
    contentClass: "p-3",
  },
};

export default function WebsiteAdSlot({ placement, className = "", popup = false, intrusiveType, variant = "inline" }: WebsiteAdSlotProps) {
  const [ads, setAds] = useState<Ad[]>([]);
  const [dismissed, setDismissed] = useState(false);
  const [current, setCurrent] = useState(0);
  const [isReady, setIsReady] = useState(false);
  const adsRef = useRef<Ad[]>([]);
  const fetchedRef = useRef(false);
  const impressedRef = useRef<Set<string>>(new Set());
  const lastImpressionRef = useRef<string>("");
  const { showIntrusiveAd, hideIntrusiveAd } = useAdSlot();
  const pathname = usePathname();
  const { user } = useAuth();

  const canShow = shouldShowAds(pathname, user);

  const fetchAds = useCallback(async () => {
    if (fetchedRef.current || !canShow) return;
    fetchedRef.current = true;
    const data = await getAdsForWebsite(placement);
    setAds(data);
    adsRef.current = data;
    setCurrent(0);
    impressedRef.current = new Set();
    setIsReady(true);
    if (popup && intrusiveType) {
      showIntrusiveAd(intrusiveType);
    }
  }, [placement, canShow, popup, intrusiveType, showIntrusiveAd]);

  useEffect(() => {
    fetchAds();
    return () => {
      if (popup && intrusiveType) {
        hideIntrusiveAd(intrusiveType);
      }
      fetchedRef.current = false;
    };
  }, [fetchAds, popup, intrusiveType, hideIntrusiveAd]);

  useEffect(() => {
    if (ads.length <= 1 || !isReady) return;
    const interval = setInterval(() => {
      setCurrent((prev) => (prev + 1) % adsRef.current.length);
    }, 15000);
    return () => clearInterval(interval);
  }, [ads.length, isReady]);

  useEffect(() => {
    if (ads.length === 0 || !isReady) return;
    const ad = ads[current % ads.length];
    const impressionKey = `${placement}:${ad.id}:${current}`;
    
    if (impressedRef.current.has(ad.id) || lastImpressionRef.current === impressionKey) {
      return;
    }
    
    lastImpressionRef.current = impressionKey;
    impressedRef.current.add(ad.id);
    recordAdImpression(ad.id).catch(() => {});
  }, [ads, current, isReady, placement]);

  if (!canShow || ads.length === 0 || dismissed) return null;

  const ad = ads[current % ads.length];
  const config = VARIANT_CONFIG[popup ? "card" : variant];

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  if (popup) {
    return (
      <div 
        className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
        onClick={(e) => { if (e.target === e.currentTarget) setDismissed(true); }}
      >
        <div 
          className="relative w-full max-w-sm bg-brand-card rounded-xl overflow-hidden shadow-2xl border border-brand-border"
          onClick={(e) => e.stopPropagation()}
        >
          <button
            onClick={() => setDismissed(true)}
            className="absolute top-2 right-2 z-10 p-1.5 rounded-full bg-black/50 text-white hover:bg-black/80 transition"
            aria-label="Close ad"
          >
            <X size={16} />
          </button>
          <div className="overflow-hidden">
            <AdMedia ad={ad} className="aspect-video max-h-48" variant="popup" />
          </div>
          <AdContent ad={ad} className="p-3" />
        </div>
      </div>
    );
  }

  return (
    <div 
      className={`${config.container} ${className}`}
      style={{ overflow: "hidden" }}
    >
      <div
        className="relative w-full bg-brand-card rounded-lg overflow-hidden border border-brand-border hover:border-brand-red/50 transition-colors cursor-pointer"
        onClick={handleClick}
      >
        <div className={`overflow-hidden ${config.mediaClass}`}>
          <AdMedia ad={ad} className="w-full h-full" variant={variant} />
        </div>
        <AdContent ad={ad} className={config.contentClass} />
        <span className="absolute top-1.5 right-1.5 text-[9px] text-gray-500 bg-black/60 px-1 py-0.5 rounded pointer-events-none uppercase tracking-wide">
          Ad
        </span>
      </div>
    </div>
  );
}

function getAdMediaUrl(ad: Ad, variant: "banner" | "inline" | "card" | "popup" = "inline"): { url?: string; type?: "image" | "video" } {
  const mediaFields: Record<string, { url?: string; type?: "image" | "video" }> = {
    banner: { url: ad.banner_media_url, type: ad.banner_media_type },
    inline: { url: ad.inline_media_url, type: ad.inline_media_type },
    card: { url: ad.inline_media_url || ad.banner_media_url, type: ad.inline_media_type || ad.banner_media_type },
    popup: { url: ad.popup_media_url || ad.banner_media_url, type: ad.popup_media_type || ad.banner_media_type },
  };
  return mediaFields[variant] || { url: ad.banner_media_url, type: ad.banner_media_type };
}

function AdMedia({ ad, className = "", variant = "inline" }: { ad: Ad; className?: string; variant?: "banner" | "inline" | "card" | "popup" }) {
  const media = getAdMediaUrl(ad, variant);
  const url = media.url || ad.image_url;
  const type = media.type || "image";
  
  if (type === "video" || ad.video_url) {
    return (
      <video
        src={url}
        muted
        loop
        playsInline
        autoPlay
        preload="metadata"
        className={`block w-full h-full object-cover object-center ${className}`}
      />
    );
  }
  
  if (url) {
    return (
      <img
        src={url}
        alt={ad.title || "Ad"}
        className={`block w-full h-full object-cover object-center ${className}`}
        loading="lazy"
      />
    );
  }
  
  return null;
}

const SHOW_AD_TEXT = false;
const SHOW_AD_CTA = false;

function AdContent({ ad, className = "" }: { ad: Ad; className?: string }) {
  const hasMedia = ad.video_url || ad.image_url;
  const showText = SHOW_AD_TEXT && (ad.title || ad.description);
  const showCTA = SHOW_AD_CTA && ad.call_to_action;

  if (hasMedia) {
    return (
      <div className={`bg-brand-card flex flex-col ${showText || showCTA ? 'gap-1.5' : ''} ${className}`}>
        {showText && (
          <>
            <p className="text-white font-medium text-sm leading-tight line-clamp-1">{ad.title}</p>
            {ad.description && (
              <p className="text-gray-400 text-xs line-clamp-1 leading-snug">{ad.description}</p>
            )}
          </>
        )}
        {showCTA && (
          <span className="inline-block mt-0.5 px-2 py-0.5 bg-brand-red text-white text-xs rounded-full w-fit font-medium">
            {ad.call_to_action}
          </span>
        )}
      </div>
    );
  }

  return (
    <div className={`p-3 bg-brand-card flex flex-col ${showText || showCTA ? 'gap-1.5' : ''} ${className}`}>
      {showText && (
        <>
          <p className="text-white font-medium text-sm leading-tight line-clamp-2">{ad.title}</p>
          {ad.description && (
            <p className="text-gray-400 text-xs line-clamp-2 leading-snug">{ad.description}</p>
          )}
        </>
      )}
      {showCTA && (
        <span className="inline-block mt-0.5 px-2 py-0.5 bg-brand-red text-white text-xs rounded-full w-fit font-medium">
          {ad.call_to_action}
        </span>
      )}
    </div>
  );
}
