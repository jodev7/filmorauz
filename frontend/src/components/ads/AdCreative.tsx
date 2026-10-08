"use client";

import MediaImage from "@/components/MediaImage";
import { normalizeMediaUrl } from "@/lib/image-utils";

/** Fills its (relative, sized) parent with the ad image or muted video. */
export function AdMedia({ url, type }: { url: string; type: "image" | "video" }) {
  if (!url) return null;

  if (type === "video") {
    return (
      <video
        src={normalizeMediaUrl(url, "")}
        muted
        loop
        playsInline
        autoPlay
        preload="metadata"
        className="absolute inset-0 w-full h-full object-cover object-center"
      />
    );
  }

  return (
    <MediaImage
      src={normalizeMediaUrl(url, "/og-image.jpg")}
      alt=""
      className="absolute inset-0 h-full w-full object-cover object-center"
    />
  );
}

/** Small "Reklama" marker in a corner of a creative. */
export function AdLabel({ className = "top-1.5 left-1.5" }: { className?: string }) {
  return (
    <span
      className={`pointer-events-none absolute z-[1] rounded bg-black/70 px-1 py-0.5 text-[9px] uppercase tracking-wide text-gray-300 ${className}`}
    >
      Reklama
    </span>
  );
}

/**
 * The ad's call-to-action as a button on top of the creative. Purely visual:
 * the whole creative is already the click target. Renders nothing when the
 * ad has no call_to_action text.
 */
export function AdCta({
  text,
  size = "md",
  className = "bottom-2 right-2",
}: {
  text?: string;
  size?: "sm" | "md";
  className?: string;
}) {
  const label = text?.trim();
  if (!label) return null;
  return (
    <span
      className={`pointer-events-none absolute z-[1] inline-flex items-center gap-1 rounded-full bg-orange-500 font-semibold text-white shadow-lg shadow-black/40 ${
        size === "sm" ? "px-2.5 py-1 text-[11px]" : "px-3.5 py-1.5 text-xs sm:px-4 sm:py-2 sm:text-sm"
      } ${className}`}
    >
      {label}
      <span aria-hidden="true">→</span>
    </span>
  );
}
