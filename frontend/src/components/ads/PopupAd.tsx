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
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

// "homepage_popup" is kept so ads targeted at the old home-only slot still run.
const PLACEMENTS = ["website_popup", "homepage_popup", "website"];

// A popup may open on any page, but not more often than this per tab.
const MIN_INTERVAL_MS = 3 * 60 * 1000;
const LAST_SHOWN_KEY = "ad_popup_shown_at";

// Player pages already carry a pre-roll and a pause ad; a modal on top of the
// player would only get in the way of pressing Play.
function isPlayerRoute(pathname: string): boolean {
  return (
    pathname.startsWith("/watch/") ||
    pathname.startsWith("/watch-room/") ||
    pathname.startsWith("/episode/") ||
    /^\/series\/[^/]+\/season\/\d+\/episode\//.test(pathname)
  );
}

function lastShownAt(): number {
  try {
    return Number(sessionStorage.getItem(LAST_SHOWN_KEY)) || 0;
  } catch {
    return 0;
  }
}

/** Site-wide modal ad, mounted once in the root layout. */
export default function PopupAd() {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const phone = useIsPhone();
  const [openOn, setOpenOn] = useState<string | null>(null);
  const boxRef = useRef<HTMLDivElement>(null);

  const allowed =
    !authLoading && !isUserPremium(user) && isAdsAllowedForRoute(pathname) && !isPlayerRoute(pathname);

  useEffect(() => {
    if (!allowed) return;
    ensurePlacements(PLACEMENTS).catch(() => {});
  }, [ensurePlacements, allowed]);

  const ads = useMemo(
    () => getMergedAds(PLACEMENTS).filter((a) => getWebsiteAdMedia(a, "popup")),
    [getMergedAds],
  );
  const ad = useRotatedAd(ads);

  // Open on this page when the frequency cap allows it.
  useEffect(() => {
    if (!allowed || !ad) return;
    if (Date.now() - lastShownAt() < MIN_INTERVAL_MS) return;
    try {
      sessionStorage.setItem(LAST_SHOWN_KEY, String(Date.now()));
    } catch {
      // Storage blocked: the popup still opens, just without the cap.
    }
    setOpenOn(pathname);
  }, [allowed, ad, pathname]);

  const open = allowed && !!ad && openOn === pathname;
  useViewableImpression(boxRef, open ? ad?.id : undefined, "popup");

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpenOn(null);
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open]);

  if (!open || !ad) return null;

  const media = getWebsiteAdMedia(ad, "popup", phone)!;
  // Desktop creative is 3:2 (900×600); the phone creative is 3:4 portrait.
  const box = media.mobile ? "max-w-[340px]" : "max-w-[600px]";
  const ratio = media.mobile ? "aspect-[3/4]" : "aspect-[3/2]";

  const handleClick = () => {
    recordAdClick(ad.id, "popup").catch(() => {});
    window.open(ad.target_url, "_blank", "noopener,noreferrer");
  };

  return (
    <div
      className="fixed inset-0 z-[60] flex items-center justify-center bg-black/60 backdrop-blur-sm p-4"
      onClick={(e) => { if (e.target === e.currentTarget) setOpenOn(null); }}
      role="dialog"
      aria-modal="true"
      aria-label="Reklama"
    >
      <div
        ref={boxRef}
        className={`relative w-full ${box} rounded-xl overflow-hidden shadow-2xl border border-brand-border`}
      >
        <button
          onClick={() => setOpenOn(null)}
          className="absolute top-2 right-2 z-10 p-1.5 rounded-full bg-black/60 text-white hover:bg-black/80 transition"
          aria-label="Reklamani yopish"
        >
          <X size={16} aria-hidden="true" />
        </button>
        <div
          className={`relative w-full ${ratio} max-h-[80vh] overflow-hidden cursor-pointer`}
          onClick={handleClick}
          role="link"
          aria-label={`Reklama: ${ad.title}`}
        >
          <AdMedia url={media.url} type={media.type} />
          <AdLabel className="bottom-2 left-2" />
          <AdCta text={ad.call_to_action} className="bottom-3 right-3" />
        </div>
      </div>
    </div>
  );
}
