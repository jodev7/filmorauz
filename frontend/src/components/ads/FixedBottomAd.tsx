"use client";

import { useEffect, useState, useRef, useMemo } from "react";
import { usePathname } from "next/navigation";
import { Ad, recordAdImpression, recordAdClick } from "@/lib/api";
import { isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import MediaImage from "@/components/MediaImage";
import { normalizeMediaUrl } from "@/lib/image-utils";

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
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const [current, setCurrent] = useState(0);
  const [dismissed, setDismissed] = useState(false);
  const impressedRef = useRef<Set<string>>(new Set());
  const lastImpressionRef = useRef<string>("");

  const placements = useMemo(
    () => [placement, "website", "global_fixed_bottom"],
    [placement],
  );

  const allAds = useMemo(() => getMergedAds(placements), [getMergedAds, placements]);
  const ads = useMemo(
    () => allAds.filter((a) => a.fixed_bottom_media_url || a.banner_media_url || a.image_url),
    [allAds],
  );
  const isReady = ads.length > 0;

  // Skip fetching entirely on routes where this ad is never rendered. Also
  // skip for premium users; fetch only when we actually need data.
  const shouldFetch =
    !authLoading &&
    !isUserPremium(user) &&
    !pathname.startsWith("/admin") &&
    !pathname.startsWith("/premium");

  useEffect(() => {
    if (!shouldFetch) return;
    ensurePlacements(placements).catch(() => {});
  }, [ensurePlacements, placements, shouldFetch]);

  // Record impression per unique ad shown
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

  if (!shouldFetch) return null;
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
      className="fixed left-0 right-0 z-50 overflow-hidden cursor-pointer relative"
      style={{ bottom: bottomOffset, height: "180px" }}
      onClick={handleAdClick}
      role="link"
      aria-label="Reklama"
    >
      {isVideo ? (
        <video
          src={normalizeMediaUrl(mediaUrl, "")}
          className="w-full h-full object-cover"
          autoPlay
          muted
          loop
          playsInline
          preload="metadata"
        />
      ) : (
        (() => {
          const normalizedMediaUrl = normalizeMediaUrl(mediaUrl, "/og-image.jpg");
          return (
            <MediaImage
              src={normalizedMediaUrl}
              alt=""
              className="absolute inset-0 h-full w-full object-cover"
            />
          );
        })()
      )}
    </div>
  );
}
