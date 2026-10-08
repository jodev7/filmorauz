import type { Ad, BannerPlace } from "@/lib/api";

export type WebsiteAdVariant = "banner" | "inline" | "card" | "popup" | "fixed_bottom";

export interface WebsiteAdMedia {
  url: string;
  type: "image" | "video";
  /** True when the phone-sized creative was picked (it has its own ratio). */
  mobile: boolean;
}

/**
 * A creative is eligible only for its configured website slot. On phones the
 * slot's optional mobile image wins; without one the desktop creative is used.
 */
export function getWebsiteAdMedia(
  ad: Ad,
  variant: WebsiteAdVariant,
  mobile = false,
): WebsiteAdMedia | null {
  const phone = (url?: string): WebsiteAdMedia | null =>
    mobile && url ? { url, type: "image", mobile: true } : null;

  if (variant === "banner") {
    // Old image-only ads have no structured slots; keep them in banners.
    const url = ad.banner_media_url || ad.image_url;
    if (!url) return null;
    return (
      phone(ad.banner_mobile_media_url) || {
        url,
        type: ad.banner_media_url ? ad.banner_media_type || "image" : "image",
        mobile: false,
      }
    );
  }
  if (variant === "inline" || variant === "card") {
    if (!ad.inline_media_url) return null;
    return (
      phone(ad.inline_mobile_media_url) || {
        url: ad.inline_media_url,
        type: ad.inline_media_type || "image",
        mobile: false,
      }
    );
  }
  if (variant === "fixed_bottom") {
    if (!ad.fixed_bottom_media_url) return null;
    return (
      phone(ad.fixed_bottom_mobile_media_url) || {
        url: ad.fixed_bottom_media_url,
        type: ad.fixed_bottom_media_type || "image",
        mobile: false,
      }
    );
  }
  if (!ad.popup_media_url) return null;
  return (
    phone(ad.popup_mobile_media_url) || {
      url: ad.popup_media_url,
      type: ad.popup_media_type || "image",
      mobile: false,
    }
  );
}

/** True when the ad's banner belongs to this carousel (no places = all of them). */
export function isBannerInPlace(ad: Ad, place: BannerPlace): boolean {
  const places = ad.banner_places || [];
  return places.length === 0 || places.includes(place);
}

/**
 * Slides of one banner carousel: ads with a banner creative assigned to the
 * place, ordered by banner_order (ties keep the incoming priority order).
 */
export function bannersForPlace(ads: Ad[], place: BannerPlace): Ad[] {
  return ads
    .filter((ad) => getWebsiteAdMedia(ad, "banner") && isBannerInPlace(ad, place))
    .map((ad, i) => ({ ad, i }))
    .sort((a, b) => (a.ad.banner_order || 0) - (b.ad.banner_order || 0) || a.i - b.i)
    .map(({ ad }) => ad);
}
