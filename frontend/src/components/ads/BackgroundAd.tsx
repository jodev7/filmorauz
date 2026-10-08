"use client";

import { useEffect, useMemo, useRef, useState } from "react";
import { usePathname } from "next/navigation";
import { recordAdImpression, recordAdClick } from "@/lib/api";
import { isAdsAllowedForRoute, isUserPremium } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { useAdSlot } from "@/components/ads/AdSlotContext";
import { normalizeMediaUrl } from "@/lib/image-utils";

const PLACEMENTS = ["website_background", "website"];

// Page content is capped at max-w-7xl (1440px, see tailwind.config). The
// gutters only become wide enough to carry a creative from this viewport up.
const CONTENT_WIDTH = 1440;
const MIN_VIEWPORT = 1600;
const WIDE_QUERY = `(min-width: ${MIN_VIEWPORT}px)`;

/**
 * Site "branding" ad: one wide image shown through the two empty gutters on
 * either side of the centered page. Both gutters are a single clickable ad.
 * Desktop-only — narrower viewports have no gutters, so nothing is fetched
 * and no impression is recorded there.
 */
export default function BackgroundAd() {
  const pathname = usePathname();
  const { user, isLoading: authLoading } = useAuth();
  const { ensurePlacements, getMergedAds } = useAdSlot();
  const [wide, setWide] = useState(false);
  const impressedRef = useRef<Set<string>>(new Set());

  useEffect(() => {
    const mq = window.matchMedia(WIDE_QUERY);
    const update = () => setWide(mq.matches);
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);

  const shouldShow =
    wide && !authLoading && !isUserPremium(user) && isAdsAllowedForRoute(pathname);

  useEffect(() => {
    if (!shouldShow) return;
    ensurePlacements(PLACEMENTS).catch(() => {});
  }, [ensurePlacements, shouldShow]);

  // Backend returns ads ordered by priority DESC, so the first one wins.
  const ad = useMemo(
    () => getMergedAds(PLACEMENTS).find((a) => a.background_media_url),
    [getMergedAds],
  );

  useEffect(() => {
    if (!shouldShow || !ad || impressedRef.current.has(ad.id)) return;
    impressedRef.current.add(ad.id);
    recordAdImpression(ad.id).catch(() => {});
  }, [ad, shouldShow]);

  if (!shouldShow || !ad) return null;

  const mediaUrl = normalizeMediaUrl(ad.background_media_url, "");
  if (!mediaUrl) return null;

  // `background-attachment: fixed` anchors the image to the viewport, so the
  // two gutters reveal the left and right parts of one continuous picture.
  const panelStyle: React.CSSProperties = {
    width: `calc((100% - ${CONTENT_WIDTH}px) / 2)`,
    backgroundImage: `url(${JSON.stringify(mediaUrl)})`,
    backgroundSize: "cover",
    backgroundPosition: "center top",
    backgroundAttachment: "fixed",
    backgroundRepeat: "no-repeat",
  };

  const panel = (side: "left" | "right") => (
    <a
      href={ad.target_url}
      target="_blank"
      rel="noopener noreferrer nofollow sponsored"
      onClick={() => recordAdClick(ad.id).catch(() => {})}
      aria-label={`Reklama: ${ad.title}`}
      tabIndex={side === "left" ? 0 : -1}
      className={`pointer-events-auto absolute inset-y-0 block ${side === "left" ? "left-0" : "right-0"}`}
      style={panelStyle}
    >
      {/* Soften the inner edge so the creative melts into the page. */}
      <span
        aria-hidden="true"
        className={`absolute inset-y-0 w-10 from-brand-dark to-transparent ${
          side === "left" ? "right-0 bg-gradient-to-l" : "left-0 bg-gradient-to-r"
        }`}
      />
      <span className="absolute bottom-2 left-2 rounded bg-black/60 px-1.5 py-0.5 text-[10px] uppercase tracking-wide text-white/70">
        Reklama
      </span>
    </a>
  );

  return (
    <div className="pointer-events-none fixed inset-0 z-[1]">
      {panel("left")}
      {panel("right")}
    </div>
  );
}
