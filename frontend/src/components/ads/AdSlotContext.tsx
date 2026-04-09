"use client";

import { createContext, useContext, useState, useCallback, useEffect, ReactNode } from "react";
import { usePathname } from "next/navigation";
import { shouldShowAds } from "@/lib/ads-utils";
import { useAuth } from "@/lib/auth-context";

export type IntrusiveAdType = "popup" | "fixed_bottom" | "large_inline" | "player_popup" | "player_overlay";

interface AdSlotContextType {
  visibleIntrusiveAds: Set<IntrusiveAdType>;
  showIntrusiveAd: (type: IntrusiveAdType) => boolean;
  hideIntrusiveAd: (type: IntrusiveAdType) => void;
  isIntrusiveAdVisible: (type: IntrusiveAdType) => boolean;
  isPopupOrPlayerVisible: boolean;
}

const AdSlotContext = createContext<AdSlotContextType | null>(null);

export function AdSlotProvider({ children }: { children: ReactNode }) {
  const [visibleIntrusiveAds, setVisibleIntrusiveAds] = useState<Set<IntrusiveAdType>>(new Set());
  const pathname = usePathname();
  const { user } = useAuth();

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

  return (
    <AdSlotContext.Provider value={{ visibleIntrusiveAds, showIntrusiveAd, hideIntrusiveAd, isIntrusiveAdVisible, isPopupOrPlayerVisible }}>
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
    };
  }
  return context;
}
