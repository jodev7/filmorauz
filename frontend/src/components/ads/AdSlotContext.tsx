"use client";

import { createContext, useContext, useState, useCallback, ReactNode, useRef, useMemo, useEffect } from "react";
import { usePathname } from "next/navigation";
import { shouldShowAds } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";
import { Ad, getAdsBatch } from "@/lib/api";

export type IntrusiveAdType = "popup" | "fixed_bottom" | "large_inline" | "player_popup" | "player_overlay";

interface AdSlotContextType {
  visibleIntrusiveAds: Set<IntrusiveAdType>;
  showIntrusiveAd: (type: IntrusiveAdType) => boolean;
  hideIntrusiveAd: (type: IntrusiveAdType) => void;
  isIntrusiveAdVisible: (type: IntrusiveAdType) => boolean;
  isPopupOrPlayerVisible: boolean;
  adsByPlacement: Record<string, Ad[]>;
  ensurePlacements: (placements: string[]) => Promise<void>;
  getMergedAds: (placements: string[]) => Ad[];
}

const AdSlotContext = createContext<AdSlotContextType | null>(null);

export function AdSlotProvider({ children }: { children: ReactNode }) {
  const [visibleIntrusiveAds, setVisibleIntrusiveAds] = useState<Set<IntrusiveAdType>>(new Set());
  const [adsByPlacement, setAdsByPlacement] = useState<Record<string, Ad[]>>({});
  const pathname = usePathname();
  const { user } = useAuth();
  const requestedPlacementsRef = useRef<Set<string>>(new Set());

  const showIntrusiveAd = useCallback((type: IntrusiveAdType): boolean => {
    if (!shouldShowAds(pathname, user)) return false;
    
    let allowed = false;
    setVisibleIntrusiveAds((prev) => {
      const newSet = new Set(prev);
      if (type === "popup" || type === "player_popup") {
        if (prev.has("popup") || prev.has("player_popup") || prev.has("fixed_bottom")) {
          return prev;
        }
        newSet.add(type);
        allowed = true;
      } else if (type === "fixed_bottom") {
        if (prev.has("popup") || prev.has("player_popup") || prev.has("fixed_bottom")) {
          return prev;
        }
        newSet.add(type);
        allowed = true;
      } else if (type === "large_inline" || type === "player_overlay") {
        newSet.add(type);
        allowed = true;
      }
      return newSet;
    });
    return allowed;
  }, [pathname, user]);

  const hideIntrusiveAd = useCallback((type: IntrusiveAdType) => {
    setVisibleIntrusiveAds((prev) => {
      const newSet = new Set(prev);
      newSet.delete(type);
      return newSet;
    });
  }, []);

  const isIntrusiveAdVisible = useCallback((type: IntrusiveAdType): boolean => {
    return visibleIntrusiveAds.has(type);
  }, [visibleIntrusiveAds]);

  const isPopupOrPlayerVisible = visibleIntrusiveAds.has("popup") || visibleIntrusiveAds.has("player_popup");

  const ensurePlacements = useCallback(async (placements: string[]) => {
    const uniquePlacements = Array.from(new Set(placements.filter(Boolean)));
    const missing = uniquePlacements.filter((placement) => !requestedPlacementsRef.current.has(placement));
    if (missing.length === 0) return;

    missing.forEach((placement) => requestedPlacementsRef.current.add(placement));
    const data = await getAdsBatch(missing);
    setAdsByPlacement((prev) => ({ ...prev, ...(data.placements || {}) }));
  }, []);

  const getMergedAds = useCallback((placements: string[]) => {
    const seen = new Set<string>();
    const merged: Ad[] = [];

    placements.forEach((placement) => {
      (adsByPlacement[placement] || []).forEach((ad) => {
        if (seen.has(ad.id)) return;
        seen.add(ad.id);
        merged.push(ad);
      });
    });

    return merged;
  }, [adsByPlacement]);

  const homepagePrefetchPlacements = useMemo(() => {
    if (pathname !== "/") return [];
    return [
      "homepage_top_banner",
      "homepage_popup",
      "website_fixed_bottom",
      "website",
      "global_banner",
      "global_fixed_bottom",
    ];
  }, [pathname]);

  useEffect(() => {
    if (homepagePrefetchPlacements.length === 0) return;
    ensurePlacements(homepagePrefetchPlacements).catch(() => {});
  }, [ensurePlacements, homepagePrefetchPlacements]);

  return (
    <AdSlotContext.Provider value={{ visibleIntrusiveAds, showIntrusiveAd, hideIntrusiveAd, isIntrusiveAdVisible, isPopupOrPlayerVisible, adsByPlacement, ensurePlacements, getMergedAds }}>
      {children}
    </AdSlotContext.Provider>
  );
}

export function useAdSlot() {
  const context = useContext(AdSlotContext);
  if (!context) {
    return {
      visibleIntrusiveAds: new Set<IntrusiveAdType>(),
      showIntrusiveAd: () => true,
      hideIntrusiveAd: () => {},
      isIntrusiveAdVisible: () => false,
      isPopupOrPlayerVisible: false,
      adsByPlacement: {},
      ensurePlacements: async () => {},
      getMergedAds: () => [],
    };
  }
  return context;
}
