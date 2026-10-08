"use client";

import { useEffect, useMemo, useRef } from "react";
import { usePathname } from "next/navigation";
import { recordAdClick } from "@/lib/api";
import { isAdsAllowedForRoute, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import { AdLabel, AdMedia } from "@/components/ads/AdCreative";
import { useIsPhone, useRotatedAd, useViewableImpression } from "@/components/ads/ad-hooks";
import { isBottomNavHidden } from "@/components/BottomNav";
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

interface FixedBottomAdProps {
  placement?: string;
}

const DEFAULT_PLACEMENT = "website_fixed_bottom";

/**
 * Full-width ad bar pinned to the bottom of the viewport on every public
 * page. It sits above the phone tab bar and has no close button.
 */
export default function FixedBottomAd({ placement = DEFAULT_PLACEMENT }: FixedBottomAdProps) {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const phone = useIsPhone();
  const boxRef = useRef<HTMLDivElement>(null);

  const placements = useMemo(
    () => [placement, "website", "global_fixed_bottom"],
    [placement],
  );

  const ads = useMemo(
    () => getMergedAds(placements).filter((a) => getWebsiteAdMedia(a, "fixed_bottom")),
    [getMergedAds, placements],
  );
  const ad = useRotatedAd(ads);

  // Skip fetching entirely on routes where this ad is never rendered. Also
  // skip for premium users; fetch only when we actually need data.
  const shouldFetch = !authLoading && !isUserPremium(user) && isAdsAllowedForRoute(pathname);

  useEffect(() => {
    if (!shouldFetch) return;
    ensurePlacements(placements).catch(() => {});
  }, [ensurePlacements, placements, shouldFetch]);

  const visible = shouldFetch && !!ad;
  useViewableImpression(boxRef, visible ? ad?.id : undefined, "fixed_bottom");

  if (!visible || !ad) return null;

  const media = getWebsiteAdMedia(ad, "fixed_bottom", phone)!;
  // 20:3 is the desktop creative, capped at 180px tall on wide screens (the
  // image is then cropped to its middle); the phone creative is 4:1.
  const ratio = media.mobile ? "aspect-[4/1]" : "aspect-[20/3] max-h-[180px]";
  // The phone tab bar (h-16 + safe area, below `md`) owns the very bottom.
  const bottom = isBottomNavHidden(pathname)
    ? "bottom-0"
    : "bottom-[calc(4rem+env(safe-area-inset-bottom))] md:bottom-0";

  const handleAdClick = () => {
    recordAdClick(ad.id, "fixed_bottom").catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <>
      {/* Reserves the bar's height at the end of the page so it never hides
          the footer. */}
      <div className={`w-full ${ratio}`} aria-hidden="true" />
      <div
        ref={boxRef}
        className={`fixed inset-x-0 z-[55] w-full cursor-pointer overflow-hidden border-t border-white/10 bg-brand-dark ${ratio} ${bottom}`}
        onClick={handleAdClick}
        role="link"
        aria-label={`Reklama: ${ad.title}`}
      >
        <AdMedia url={media.url} type={media.type} />
        <AdLabel className="bottom-1 left-1" />
      </div>
    </>
  );
}
