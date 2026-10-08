"use client";

import { RefObject, useEffect, useMemo, useRef, useState } from "react";
import { Ad, AdSlot, recordAdImpression } from "@/lib/api";
import { pickWeightedRandomAd } from "@/lib/ads-utils";

// Below Tailwind's `sm` — the widths where a slot's phone creative is used.
const PHONE_QUERY = "(max-width: 639px)";

/** True on phone-width viewports. Always false during SSR / first paint. */
export function useIsPhone(): boolean {
  const [phone, setPhone] = useState(false);
  useEffect(() => {
    const mq = window.matchMedia(PHONE_QUERY);
    const update = () => setPhone(mq.matches);
    update();
    mq.addEventListener("change", update);
    return () => mq.removeEventListener("change", update);
  }, []);
  return phone;
}

/**
 * Picks one ad out of the eligible list, weighted by priority, and keeps it
 * until the list itself changes. Several active ads therefore share a slot
 * across page views instead of the first one always winning.
 */
export function useRotatedAd(ads: Ad[]): Ad | null {
  const ids = ads.map((a) => a.id).join(",");
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => (ads.length ? pickWeightedRandomAd(ads) : null), [ids]);
}

// An impression counts once at least half of the creative has been on screen
// for a full second (the usual "viewable impression" rule).
const VIEWABLE_RATIO = 0.5;
const VIEWABLE_MS = 1000;

/**
 * Records one impression per ad per mount, and only when the creative was
 * really seen. Pass `adId = undefined` while nothing is rendered.
 */
export function useViewableImpression(
  ref: RefObject<Element | null>,
  adId: string | undefined,
  slot: AdSlot,
): void {
  const seen = useRef<Set<string>>(new Set());

  useEffect(() => {
    const node = ref.current;
    if (!node || !adId || seen.current.has(adId)) return;

    let timer: ReturnType<typeof setTimeout> | null = null;
    const cancel = () => {
      if (timer) clearTimeout(timer);
      timer = null;
    };
    const observer = new IntersectionObserver(
      ([entry]) => {
        const visible = entry.isIntersecting && entry.intersectionRatio >= VIEWABLE_RATIO;
        if (!visible) return cancel();
        if (timer) return;
        timer = setTimeout(() => {
          timer = null;
          if (document.visibilityState !== "visible" || seen.current.has(adId)) return;
          seen.current.add(adId);
          observer.disconnect();
          recordAdImpression(adId, slot).catch(() => {});
        }, VIEWABLE_MS);
      },
      { threshold: [0, VIEWABLE_RATIO] },
    );
    observer.observe(node);
    return () => {
      cancel();
      observer.disconnect();
    };
  }, [ref, adId, slot]);
}
