"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { ChevronLeft, ChevronRight } from "lucide-react";
import { Ad, recordAdClick, recordAdImpression } from "@/lib/api";
import { AdCta, AdLabel, AdMedia } from "@/components/ads/AdCreative";
import { getWebsiteAdMedia } from "@/lib/website-ad-media";

// How long one banner stays before the next one slides in.
const SLIDE_MS = 7000;
// Same "viewable" rule as useViewableImpression: half on screen for a second.
const VIEWABLE_RATIO = 0.5;
const VIEWABLE_MS = 1000;

/**
 * Banner slot as a carousel: every eligible ad is one slide with its own
 * link and its own impression/click counters. Slides advance on their own,
 * by swipe on touch screens (native scroll-snap) and by arrows on desktop.
 * With a single ad it is just that banner — no arrows, dots or timer.
 */
export default function AdBannerCarousel({ ads, phone }: { ads: Ad[]; phone: boolean }) {
  const rootRef = useRef<HTMLDivElement>(null);
  const trackRef = useRef<HTMLDivElement>(null);
  const seen = useRef<Set<string>>(new Set());
  const [index, setIndex] = useState(0);
  const [inView, setInView] = useState(false);
  const [hovered, setHovered] = useState(false);
  const count = ads.length;

  const slides = ads.map((ad) => ({ ad, media: getWebsiteAdMedia(ad, "banner", phone)! }));
  // One phone creative (2:1) makes the whole carousel 2:1 on phones; wide
  // desktop creatives in it are then shown whole instead of cropped.
  const tall = slides.some((s) => s.media.mobile);
  const ratio = tall ? "aspect-[2/1]" : "aspect-[4/1] max-h-[300px]";

  const goTo = useCallback(
    (target: number) => {
      const track = trackRef.current;
      if (!track || count === 0) return;
      const next = ((target % count) + count) % count;
      track.scrollTo({ left: next * track.clientWidth, behavior: "smooth" });
    },
    [count],
  );

  // The list can shrink (ad expired, phone/desktop switch): stay in range.
  useEffect(() => {
    if (index >= count) setIndex(0);
  }, [index, count]);

  useEffect(() => {
    const node = rootRef.current;
    if (!node) return;
    const observer = new IntersectionObserver(
      ([entry]) => setInView(entry.isIntersecting && entry.intersectionRatio >= VIEWABLE_RATIO),
      { threshold: [0, VIEWABLE_RATIO] },
    );
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  // Auto-advance. Keyed on `index`, so a manual swipe or arrow click restarts
  // the 7 seconds for the banner the viewer just moved to.
  useEffect(() => {
    if (count < 2 || !inView || hovered) return;
    const timer = setTimeout(() => {
      if (document.visibilityState === "visible") goTo(index + 1);
    }, SLIDE_MS);
    return () => clearTimeout(timer);
  }, [count, inView, hovered, index, goTo]);

  // One impression per banner, counted when that slide was actually shown.
  const currentId = ads[index]?.id;
  useEffect(() => {
    if (!inView || !currentId || seen.current.has(currentId)) return;
    const timer = setTimeout(() => {
      if (document.visibilityState !== "visible" || seen.current.has(currentId)) return;
      seen.current.add(currentId);
      recordAdImpression(currentId, "banner").catch(() => {});
    }, VIEWABLE_MS);
    return () => clearTimeout(timer);
  }, [inView, currentId]);

  if (count === 0) return null;

  const arrow =
    "absolute top-1/2 z-[2] hidden h-9 w-9 -translate-y-1/2 items-center justify-center rounded-full bg-black/60 text-white transition hover:bg-black/80 sm:flex";

  return (
    <div
      ref={rootRef}
      className="group relative w-full overflow-hidden rounded-lg border border-brand-border"
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      role="region"
      aria-roledescription="carousel"
      aria-label="Reklama bannerlari"
    >
      <div
        ref={trackRef}
        className={`flex w-full snap-x snap-mandatory overflow-x-auto scrollbar-hide ${ratio}`}
        onScroll={(e) => {
          const track = e.currentTarget;
          if (!track.clientWidth) return;
          const next = Math.round(track.scrollLeft / track.clientWidth);
          if (next !== index && next >= 0 && next < count) setIndex(next);
        }}
      >
        {slides.map(({ ad, media }) => (
          <div
            key={ad.id}
            className="relative h-full w-full shrink-0 cursor-pointer snap-center overflow-hidden bg-brand-dark"
            onClick={() => {
              recordAdClick(ad.id, "banner").catch(() => {});
              window.open(ad.target_url, "_blank", "noopener,noreferrer");
            }}
            role="link"
            aria-label={`Reklama: ${ad.title}`}
          >
            <AdMedia url={media.url} type={media.type} fit={tall && !media.mobile ? "contain" : "cover"} />
            <AdCta text={ad.call_to_action} className={count > 1 ? "bottom-2 right-2 sm:right-14" : "bottom-2 right-2"} />
          </div>
        ))}
      </div>

      <AdLabel className="top-1.5 right-1.5" />

      {count > 1 && (
        <>
          <button type="button" onClick={() => goTo(index - 1)} className={`${arrow} left-2`} aria-label="Oldingi banner">
            <ChevronLeft size={20} aria-hidden="true" />
          </button>
          <button type="button" onClick={() => goTo(index + 1)} className={`${arrow} right-2`} aria-label="Keyingi banner">
            <ChevronRight size={20} aria-hidden="true" />
          </button>
          <div className="absolute bottom-1.5 left-1/2 z-[2] flex -translate-x-1/2 gap-1.5">
            {slides.map(({ ad }, i) => (
              <button
                key={ad.id}
                type="button"
                onClick={() => goTo(i)}
                className={`h-1.5 rounded-full transition-all ${i === index ? "w-5 bg-white" : "w-1.5 bg-white/50 hover:bg-white/80"}`}
                aria-label={`${i + 1}-banner`}
                aria-current={i === index}
              />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
