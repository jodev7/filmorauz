"use client";

import Link from "next/link";
import { Play, Crown } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { PremiumButton } from "@/components/PremiumComponents";

interface WatchButtonProps {
  movieSlug: string;
  movieTitle: string;
  isPremium: boolean;
}

export default function WatchButton({ movieSlug, movieTitle, isPremium }: WatchButtonProps) {
  const { user, isAuthenticated, isLoading } = useAuth();
  
  // Check if user is premium active
  const isUserPremium = user?.is_premium_active === true;
  
  // If movie is premium and user doesn't have access, show upgrade CTA
  if (isPremium && !isUserPremium) {
    return (
      <PremiumButton
        onClick={() => window.open('http://localhost:3000/premium', '_blank')}
      >
        <Crown size={18} />
        Upgrade to Watch
      </PremiumButton>
    );
  }
  
  // Otherwise show normal watch button
  return (
    <Link
      href={`/watch/${movieSlug}`}
      className="inline-flex items-center gap-2 sm:gap-3 bg-brand-red hover:bg-orange-700 text-white font-semibold px-6 sm:px-8 py-3 sm:py-4 rounded-xl transition-colors text-sm sm:text-base"
    >
      <Play size={20} className="sm:w-6 sm:h-6" fill="white" />
      Watch Now
    </Link>
  );
}
