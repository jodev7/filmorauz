"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { usePathname } from "next/navigation";
import { X } from "lucide-react";
import { Ad, getAdsForWebsite, recordAdImpression, recordAdClick } from "@/lib/api";
import { useAdSlot } from "./AdSlotContext";
import { shouldShowAds } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";

interface FixedBottomAdProps {
  placement: string;
  bottomOffset?: string;
}

export default function FixedBottomAd({ placement, bottomOffset = "0px" }: FixedBottomAdProps) {
  const [ads, setAds] = useState<Ad[]>([]);
  const [dismissed, setDismissed] = useState(false);
  const [current, setCurrent] = useState(0);
  const [isReady, setIsReady] = useState(false);
  const adsRef = useRef<Ad[]>([]);
  const fetchedRef = useRef(false);
  const impressedRef = useRef<Set<string>>(new Set());
  const lastImpressionRef = useRef<string>("");
  const { showIntrusiveAd, hideIntrusiveAd, isPopupOrPlayerVisible } = useAdSlot();
  const pathname = usePathname();
  const { user } = useAuth();

  const canShow = shouldShowAds(pathname, user) && !isPopupOrPlayerVisible;

  const fetchAds = useCallback(async () => {
    if (fetchedRef.current || !canShow) return;
    fetchedRef.current = true;
    const data = await getAdsForWebsite(placement);
    if (data.length > 0) {
      setAds(data);
      adsRef.current = data;
      setCurrent(0);
      impressedRef.current = new Set();
      setIsReady(true);
      showIntrusiveAd("fixed_bottom");
    }
  }, [placement, canShow, showIntrusiveAd]);

  useEffect(() => {
    fetchAds();
    return () => {
      hideIntrusiveAd("fixed_bottom");
      fetchedRef.current = false;
    };
  }, [fetchAds, hideIntrusiveAd]);

  useEffect(() => {
    if (ads.length <= 1) return;
    const interval = setInterval(() => {
      setCurrent((prev) => (prev + 1) % adsRef.current.length);
    }, 15000);
    return () => clearInterval(interval);
  }, [ads.length]);

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

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <div
      className="fixed left-0 right-0 z-40 bg-brand-card/95 backdrop-blur-sm border-t border-brand-border shadow-lg pointer-events-auto"
      style={{ bottom: bottomOffset }}
    >
      <div className="max-w-3xl mx-auto px-3 py-2 flex items-center gap-2">
        <button
          onClick={() => setDismissed(true)}
          className="flex-shrink-0 p-1 rounded-full hover:bg-brand-border transition"
          aria-label="Close ad"
        >
          <X size={14} className="text-gray-400" />
        </button>
        <div
          className="flex-1 cursor-pointer flex items-center justify-center min-w-0"
          onClick={handleClick}
        >
          {(() => {
            const url = ad.fixed_bottom_media_url || ad.image_url;
            const type = ad.fixed_bottom_media_type || "image";
            if (type === "video" || ad.video_url) {
              return (
                <video
                  src={url}
                  muted
                  loop
                  playsInline
                  autoPlay
                  preload="metadata"
                  className="h-8 w-auto max-w-full object-contain object-center"
                  style={{ maxHeight: "32px", maxWidth: "100%" }}
                />
              );
            }
            if (url) {
              return (
                <img
                  src={url}
                  alt={ad.title || "Ad"}
                  className="h-8 w-auto max-w-full object-contain object-center"
                  style={{ maxHeight: "32px", maxWidth: "100%" }}
                  loading="lazy"
                />
              );
            }
            return null;
          })()}
        </div>
        {ads.length > 1 && (
          <div className="flex gap-0.5">
            {ads.map((_, i) => (
              <div
                key={i}
                className={`w-1 h-1 rounded-full ${i === current % ads.length ? "bg-brand-red" : "bg-brand-border"}`}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  );
}