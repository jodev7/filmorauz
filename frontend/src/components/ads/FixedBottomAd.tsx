"use client";

import { useEffect, useState, useRef, useCallback } from "react";
import { usePathname } from "next/navigation";
import { Ad, getAdsForWebsite, recordAdImpression, recordAdClick } from "@/lib/api";
import { pickWeightedRandomAd, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";

interface FixedBottomAdProps {
  placement?: string;
  bottomOffset?: string;
}

const DEFAULT_PLACEMENT = "website_fixed_bottom";

export default function FixedBottomAd({
  placement = DEFAULT_PLACEMENT,
  bottomOffset = "0px",
}: FixedBottomAdProps) {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const [ads, setAds] = useState<Ad[]>([]);
  const [current, setCurrent] = useState(0);
  const [dismissed, setDismissed] = useState(false);
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
      const valid = data.filter(
        (a) => a.fixed_bottom_media_url || a.banner_media_url || a.image_url
      );
      setAds(valid);
      adsRef.current = valid;
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

  // Rotate with weighted random every 5 seconds when multiple ads exist
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

  // Record impression per unique ad shown
  useEffect(() => {
    if (ads.length === 0 || !isReady) return;
    const ad = ads[current % ads.length];
    const impressionKey = `${placement}:${ad.id}:${current}`;
    if (impressedRef.current.has(ad.id) || lastImpressionRef.current === impressionKey) return;
    lastImpressionRef.current = impressionKey;
    impressedRef.current.add(ad.id);
    recordAdImpression(ad.id).catch(() => {});
  }, [ads, current, isReady, placement]);

  if (pathname.startsWith("/admin")) return null;
  if (authLoading || isUserPremium(user)) return null;
  if (!isReady || ads.length === 0 || dismissed) return null;

  const ad = ads[current % ads.length];

  const mediaUrl = ad.fixed_bottom_media_url || ad.banner_media_url || ad.image_url || "";
  const isVideo =
    ad.fixed_bottom_media_type === "video" ||
    ad.banner_media_type === "video" ||
    mediaUrl.endsWith(".mp4") ||
    mediaUrl.endsWith(".webm");

  const handleAdClick = () => {
    recordAdClick(ad.id).catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <div
      className="fixed left-0 right-0 z-50 overflow-hidden cursor-pointer"
      style={{ bottom: bottomOffset, height: "180px" }}
      onClick={handleAdClick}
    >
      {isVideo ? (
        <video
          src={mediaUrl}
          className="w-full h-full object-cover"
          autoPlay
          muted
          loop
          playsInline
        />
      ) : (
        <img
          src={mediaUrl}
          alt="Advertisement"
          className="w-full h-full object-cover"
        />
      )}
    </div>
  );
}
