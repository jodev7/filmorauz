"use client";

import type { Ad } from "@/lib/api";

/**
 * Picks a weighted random ad using the `priority` field as weight (default 1).
 * Avoids repeating `currentAdId` when alternatives exist — tries twice before
 * falling back to any other ad.
 */
export function pickWeightedRandomAd(ads: Ad[], currentAdId?: string): Ad {
  if (ads.length === 1) return ads[0];

  const weights = ads.map((a) => Math.max(a.priority ?? 1, 1));
  const total = weights.reduce((sum, w) => sum + w, 0);

  const pick = (): Ad => {
    let rand = Math.random() * total;
    for (let i = 0; i < ads.length; i++) {
      rand -= weights[i];
      if (rand <= 0) return ads[i];
    }
    return ads[ads.length - 1];
  };

  // Try twice to avoid back-to-back repeat
  const first = pick();
  if (first.id !== currentAdId) return first;
  const second = pick();
  if (second.id !== currentAdId) return second;

  // Last resort: any ad that isn't current, or first
  return ads.find((a) => a.id !== currentAdId) ?? ads[0];
}

export function isAdsAllowedForRoute(pathname: string): boolean {
  if (!pathname) return true;
  
  const lowerPath = pathname.toLowerCase();
  
  if (lowerPath.startsWith("/admin")) return false;
  if (lowerPath.startsWith("/premium")) return false;
  
  return true;
}

export function isUserPremium(user: { is_premium?: boolean; is_premium_active?: boolean } | null | undefined): boolean {
  if (!user) return false;
  return user.is_premium === true || user.is_premium_active === true;
}

export function shouldShowAds(pathname: string, user: { is_premium?: boolean; is_premium_active?: boolean } | null | undefined): boolean {
  if (!isAdsAllowedForRoute(pathname)) return false;
  if (isUserPremium(user)) return false;
  return true;
}

export const AD_MEDIA_RULES = {
  website: {
    banner: {
      aspectRatio: "21/9",
      maxHeight: "120px",
      objectFit: "cover" as const,
      objectPosition: "center" as const,
      minWidth: 300,
      recommendedRatio: "16:9 to 21:9",
    },
    inline: {
      aspectRatio: "16/9",
      maxHeight: "180px",
      objectFit: "cover" as const,
      objectPosition: "center" as const,
      minWidth: 200,
      recommendedRatio: "16:9",
    },
    card: {
      aspectRatio: "4/3",
      maxHeight: "200px",
      objectFit: "cover" as const,
      objectPosition: "center" as const,
      minWidth: 150,
      recommendedRatio: "4:3 or 16:9",
    },
    fixedBottom: {
      height: "32px",
      maxHeight: "32px",
      objectFit: "contain" as const,
      objectPosition: "center" as const,
      minWidth: 100,
      recommendedRatio: "any horizontal",
    },
  },
  telegram: {
    recommendedRatio: "any (photo or video supported)",
    minWidth: 150,
  },
};

export function getRecommendedMediaRules(platform: "website" | "telegram", slotType?: string) {
  if (platform === "telegram") {
    return AD_MEDIA_RULES.telegram;
  }
  if (platform === "website" && slotType) {
    return AD_MEDIA_RULES.website[slotType as keyof typeof AD_MEDIA_RULES.website] || AD_MEDIA_RULES.website.banner;
  }
  return AD_MEDIA_RULES.website.banner;
}
