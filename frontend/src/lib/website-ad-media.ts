import type { Ad } from "@/lib/api";

export type WebsiteAdVariant = "banner" | "inline" | "card" | "popup";

/** A creative is eligible only for its configured website slot. */
export function getWebsiteAdMedia(
  ad: Ad,
  variant: WebsiteAdVariant,
): { url: string; type: "image" | "video" } | null {
  if (variant === "banner") {
    // Old image-only ads have no structured slots; keep them in banners.
    const url = ad.banner_media_url || ad.image_url;
    return url ? { url, type: ad.banner_media_url ? ad.banner_media_type || "image" : "image" } : null;
  }
  if (variant === "inline" || variant === "card") {
    return ad.inline_media_url
      ? { url: ad.inline_media_url, type: ad.inline_media_type || "image" }
      : null;
  }
  return ad.popup_media_url
    ? { url: ad.popup_media_url, type: ad.popup_media_type || "image" }
    : null;
}
