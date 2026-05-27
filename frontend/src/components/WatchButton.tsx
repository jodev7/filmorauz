"use client";

import { Play, Crown } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { PremiumButton } from "@/components/PremiumComponents";
import { useWatchPlayer } from "@/lib/watch-player-context";

interface WatchButtonProps {
  movieSlug: string;
  movieTitle: string;
  isPremium: boolean;
}

export default function WatchButton({ isPremium }: WatchButtonProps) {
  const { user } = useAuth();
  const { openPlayer } = useWatchPlayer();

  // Check if user is premium active
  const isUserPremium = user?.is_premium_active === true;

  // If movie is premium and user doesn't have access, show upgrade CTA
  if (isPremium && !isUserPremium) {
    return (
      <PremiumButton onClick={() => window.open("/premium", "_blank")}>
        <Crown size={18} />
        Premium olish
      </PremiumButton>
    );
  }

  // Opens the player inline on the same page (no navigation) + scrolls to it.
  return (
    <button
      onClick={openPlayer}
      className="inline-flex items-center gap-2 sm:gap-3 bg-brand-red hover:bg-orange-700 text-white font-semibold px-6 sm:px-8 py-3 sm:py-4 rounded-xl transition-colors text-sm sm:text-base"
    >
      <Play size={20} className="sm:w-6 sm:h-6" fill="white" />
      Hozir tomosha qilish
    </button>
  );
}
