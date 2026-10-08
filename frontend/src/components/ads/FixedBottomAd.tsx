"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { X } from "lucide-react";
import { recordAdClick } from "@/lib/api";
import { isAdsAllowedForRoute, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import { AdCta, AdLabel, AdMedia } from "@/components/ads/AdCreative";
import { useIsPhone, useRotatedAd, useViewableImpression } from "@/components/ads/ad-hooks";
import { isBottomNavHidden } from "@/components/BottomNav";
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

interface FixedBottomAdProps {
  placement?: string;
}

const DEFAULT_PLACEMENT = "website_fixed_bottom";

/**
 * Anchor ad pinned to the bottom of the viewport on every public page. Kept
 * to standard anchor sizes (600×90 on desktop, full width on phones) so it
 * never covers a meaningful part of the page, and it sits above the phone tab
 * bar. Closing it hides it until the next page.
 */
export default function FixedBottomAd({ placement = DEFAULT_PLACEMENT }: FixedBottomAdProps) {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const phone = useIsPhone();
  const [dismissedOn, setDismissedOn] = useState<string | null>(null);
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

  const visible = shouldFetch && !!ad && dismissedOn !== pathname;
  useViewableImpression(boxRef, visible ? ad?.id : undefined, "fixed_bottom");

  if (!visible || !ad) return null;

  const media = getWebsiteAdMedia(ad, "fixed_bottom", phone)!;
  // 20:3 is the desktop creative (600×90 here); the phone creative is 4:1.
  const ratio = media.mobile ? "aspect-[4/1]" : "aspect-[20/3]";
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
      <div className={`mx-auto w-full max-w-[600px] ${ratio}`} aria-hidden="true" />
      <div className={`pointer-events-none fixed inset-x-0 z-[55] flex justify-center ${bottom}`}>
        <div
          ref={boxRef}
          className={`pointer-events-auto relative w-full max-w-[600px] cursor-pointer overflow-hidden border-t border-white/10 bg-brand-dark shadow-[0_-8px_30px_rgba(0,0,0,0.5)] sm:rounded-t-xl sm:border-x ${ratio}`}
          onClick={handleAdClick}
          role="link"
          aria-label={`Reklama: ${ad.title}`}
        >
          <AdMedia url={media.url} type={media.type} />
          <AdLabel className="bottom-1 left-1" />
          <AdCta text={ad.call_to_action} size="sm" className="bottom-1.5 right-1.5" />
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              setDismissedOn(pathname);
            }}
            className="absolute right-1 top-1 z-[2] flex h-6 w-6 items-center justify-center rounded-full bg-black/70 text-white transition hover:bg-black"
            aria-label="Reklamani yopish"
          >
            <X size={13} aria-hidden="true" />
          </button>
        </div>
      </div>
    </>
  );
}
