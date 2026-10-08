"use client";

import { useEffect, useState } from "react";
import { X } from "lucide-react";
import { Ad, getAdsForWebsite, recordAdClick, recordAdImpression } from "@/lib/api";
import { pickWeightedRandomAd } from "@/lib/ads-utils";
import { AdCta, AdLabel, AdMedia } from "@/components/ads/AdCreative";
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

// A pause shorter than this (a tap, a quick seek) never shows the ad.
const SHOW_AFTER_MS = 800;

/** Image creative for the pause card: the popup image, else inline, else banner. */
function pauseMedia(ad: Ad): string | null {
  for (const variant of ["popup", "inline", "banner"] as const) {
    const media = getWebsiteAdMedia(ad, variant);
    if (media && media.type === "image") return media.url;
  }
  return null;
}

/**
 * Image ad shown over the player while the viewer has paused the film. It
 * sits in the upper part of the player and only the card itself takes clicks,
 * so the play button and the control bar stay usable. `paused` must already
 * exclude ad breaks, scrubbing and the not-yet-started state.
 */
export default function PlayerPauseAd({ paused }: { paused: boolean }) {
  const [pool, setPool] = useState<Ad[] | null>(null);
  const [ad, setAd] = useState<Ad | null>(null);

  // Fetch lazily on the first pause; the result is reused for later pauses.
  useEffect(() => {
    if (!paused || pool) return;
    let cancelled = false;
    getAdsForWebsite("watch_player_pause")
      .then((ads) => { if (!cancelled) setPool(ads.filter((a) => pauseMedia(a))); })
      .catch(() => { if (!cancelled) setPool([]); });
    return () => { cancelled = true; };
  }, [paused, pool]);

  useEffect(() => {
    if (!paused || !pool?.length) {
      setAd(null);
      return;
    }
    const timer = setTimeout(() => {
      const picked = pickWeightedRandomAd(pool);
      setAd(picked);
      recordAdImpression(picked.id, "player_pause").catch(() => {});
    }, SHOW_AFTER_MS);
    return () => clearTimeout(timer);
  }, [paused, pool]);

  if (!ad) return null;
  const url = pauseMedia(ad);
  if (!url) return null;

  return (
    <div className="pointer-events-none absolute inset-0 z-20 flex items-start justify-center pt-[4%]">
      <div
        className="pointer-events-auto relative aspect-[3/2] h-[55%] cursor-pointer overflow-hidden rounded-xl border border-white/15 bg-black shadow-2xl"
        onClick={(e) => {
          e.stopPropagation();
          recordAdClick(ad.id, "player_pause").catch(() => {});
          window.open(ad.target_url, "_blank", "noopener,noreferrer");
        }}
        onDoubleClick={(e) => e.stopPropagation()}
        role="link"
        aria-label={`Reklama: ${ad.title}`}
      >
        <AdMedia url={url} type="image" />
        <AdLabel className="top-1.5 left-1.5" />
        <AdCta text={ad.call_to_action} size="sm" className="bottom-1.5 right-1.5" />
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            setAd(null);
          }}
          className="absolute right-1.5 top-1.5 z-[2] flex h-6 w-6 items-center justify-center rounded-full bg-black/70 text-white transition hover:bg-black"
          aria-label="Reklamani yopish"
        >
          <X size={13} aria-hidden="true" />
        </button>
      </div>
    </div>
  );
}
