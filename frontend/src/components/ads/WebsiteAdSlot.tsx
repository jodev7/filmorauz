"use client";

import { useEffect, useState, useRef, useMemo } from "react";
import { usePathname } from "next/navigation";
import { recordAdClick } from "@/lib/api";
import { isAdsAllowedForRoute, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import { AdCta, AdLabel, AdMedia } from "@/components/ads/AdCreative";
import AdBannerCarousel from "@/components/ads/AdBannerCarousel";
import { useIsPhone, useRotatedAd, useViewableImpression } from "@/components/ads/ad-hooks";
import type { BannerPlace } from "@/lib/api";
import { bannersForPlace, getWebsiteAdMedia } from "@/lib/website-ad-media";

interface WebsiteAdSlotProps {
  placement: string;
  className?: string;
  variant?: "banner" | "inline" | "card";
  /** Which banner carousel this is (admin "Banner karusel"). Banner variant only. */
  bannerPlace?: BannerPlace;
  lazy?: boolean;
  /** Render nothing (not even the empty wrapper) while there is no ad. Not for `lazy` slots. */
  hideEmpty?: boolean;
}

// Slots keep the creative's own ratio (the sizes the admin editor asks for) so
// the whole banner stays visible on narrow screens; the height cap keeps the
// previous desktop size on containers wider than the creative.
const SLOT_HEIGHT: Record<string, string> = {
  banner: "aspect-[4/1] max-h-[300px]",
  inline: "aspect-[3/1] max-h-[400px]",
  card:   "h-[200px]",
};
// Ratios of the optional phone creatives (see WEBSITE_SLOTS in AdEditor).
const MOBILE_SLOT_HEIGHT: Record<string, string> = {
  banner: "aspect-[2/1]",
  inline: "aspect-[4/3]",
  card:   "aspect-[4/3]",
};

/**
 * In-page ad. `banner` slots are a carousel of every eligible ad; `inline`
 * slots show one ad picked per page view. Renders an empty box without ads.
 */
export default function WebsiteAdSlot({
  placement,
  className = "",
  variant = "inline",
  bannerPlace = "top",
  lazy = false,
  hideEmpty = false,
}: WebsiteAdSlotProps) {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const phone = useIsPhone();
  const containerRef = useRef<HTMLDivElement>(null);
  const creativeRef = useRef<HTMLDivElement>(null);
  const [shouldLoad, setShouldLoad] = useState(!lazy);

  const placements = useMemo(
    () => getSlotPlacements(placement, variant),
    [placement, variant],
  );

  // Ads always derive from the shared context — no local state, no stale
  // empty-array bug, no duplicate fetch loops.
  const ads = useMemo(() => {
    const merged = getMergedAds(placements);
    return variant === "banner"
      ? bannersForPlace(merged, bannerPlace)
      : merged.filter((ad) => getWebsiteAdMedia(ad, variant));
  }, [getMergedAds, placements, variant, bannerPlace]);
  const ad = useRotatedAd(ads);

  const allowed = !authLoading && !isUserPremium(user) && isAdsAllowedForRoute(pathname);

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
    if (!shouldLoad || !allowed) return;
    ensurePlacements(placements).catch(() => {});
  }, [ensurePlacements, placements, shouldLoad, allowed]);

  const shown = allowed && shouldLoad ? ad : null;
  const carousel = variant === "banner";
  // The carousel counts impressions per slide itself.
  useViewableImpression(creativeRef, carousel ? undefined : shown?.id, "inline");

  // The wrapper stays mounted (it carries the caller's spacing and is the
  // lazy-load sentinel) but collapses while there is nothing to show.
  if (!shown && hideEmpty) return null;
  if (!shown) return <div ref={containerRef} className={allowed ? `w-full ${className}` : undefined} />;

  if (carousel) {
    return (
      <div ref={containerRef} className={`w-full ${className}`}>
        <AdBannerCarousel ads={ads} phone={phone} />
      </div>
    );
  }

  const media = getWebsiteAdMedia(shown, variant, phone)!;
  const mediaHeight = (media.mobile ? MOBILE_SLOT_HEIGHT : SLOT_HEIGHT)[variant] ?? SLOT_HEIGHT.banner;

  const handleClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    recordAdClick(shown.id, "inline").catch(() => {});
    window.open(shown.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <div ref={containerRef} className={`w-full ${className}`}>
      <div
        ref={creativeRef}
        className="relative w-full rounded-lg overflow-hidden border border-brand-border cursor-pointer hover:border-brand-red/50 transition-colors"
        onClick={handleClick}
        role="link"
        aria-label={`Reklama: ${shown.title}`}
      >
        <div className={`relative w-full ${mediaHeight} overflow-hidden`}>
          <AdMedia url={media.url} type={media.type} />
        </div>
        <AdLabel className="top-1.5 right-1.5" />
        <AdCta text={shown.call_to_action} />
      </div>
    </div>
  );
}

function getSlotPlacements(
  placement: string,
  variant: "banner" | "inline" | "card",
): string[] {
  const shared = ["website"];
  if (variant === "banner") {
    shared.push("global_banner");
  } else {
    shared.push("global_inline");
  }

  return [placement, ...shared];
}
