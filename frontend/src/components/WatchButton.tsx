"use client";

import { useState } from "react";
import { Play, Crown, Send } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { PremiumButton } from "@/components/PremiumComponents";
import { useWatchPlayer } from "@/lib/watch-player-context";
import TelegramLoginModal from "@/components/TelegramLoginModal";

interface WatchButtonProps {
  movieSlug: string;
  movieTitle: string;
  isPremium: boolean;
}

const buttonClass =
  "inline-flex items-center gap-2 sm:gap-3 bg-brand-red hover:bg-orange-700 text-white font-semibold px-6 sm:px-8 py-3 sm:py-4 rounded-xl transition-colors text-sm sm:text-base";

export default function WatchButton({ isPremium }: WatchButtonProps) {
  const { user, isAuthenticated, isLoading } = useAuth();
  const { openPlayer } = useWatchPlayer();
  const [loginModalOpen, setLoginModalOpen] = useState(false);

  // Auth is still resolving — render an inert watch button so logged-in users
  // don't see a "Kirish" flash before their session loads.
  if (isLoading) {
    return (
      <button disabled className={`${buttonClass} opacity-60 cursor-wait`}>
        <Play size={20} className="sm:w-6 sm:h-6" fill="white" />
        Hozir tomosha qilish
      </button>
    );
  }

  // Watching requires an account — guests get a login CTA instead.
  if (!isAuthenticated) {
    return (
      <>
        <button onClick={() => setLoginModalOpen(true)} className={buttonClass}>
          <Send size={20} className="sm:w-6 sm:h-6" />
          Kirish
        </button>
        <TelegramLoginModal isOpen={loginModalOpen} onClose={() => setLoginModalOpen(false)} />
      </>
    );
  }

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
    <button onClick={openPlayer} className={buttonClass}>
      <Play size={20} className="sm:w-6 sm:h-6" fill="white" />
      Hozir tomosha qilish
    </button>
  );
}
