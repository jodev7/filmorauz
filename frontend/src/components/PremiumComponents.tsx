// Premium UI Components
// Shared premium visual components for consistent premium styling across the app

import { Crown } from "lucide-react";

// Premium Badge - elegant gold badge for premium content/users
export function PremiumBadge({ 
  size = "default",
  showCrown = false,
  className = "" 
}: { 
  size?: "sm" | "default" | "lg";
  showCrown?: boolean;
  className?: string;
}) {
  const sizeClasses = {
    sm: "text-xs px-1.5 py-0.5",
    default: "text-xs px-2 py-0.5",
    lg: "text-sm px-3 py-1",
  };

  return (
    <span 
      className={`
        inline-flex items-center gap-1 rounded-full font-medium
        bg-gradient-to-r from-yellow-500 to-amber-600 
        text-black shadow-[0_0_10px_rgba(234,179,8,0.3)]
        ${sizeClasses[size]}
        ${className}
      `}
    >
      {showCrown && <Crown size={size === "sm" ? 10 : size === "lg" ? 14 : 12} />}
      Premium
    </span>
  );
}

// Premium Avatar Ring - subtle gold ring around avatar for premium users
export function PremiumAvatarRing({ 
  children, 
  size = "default",
  className = "" 
}: { 
  children: React.ReactNode;
  size?: "sm" | "default" | "lg";
  className?: string;
}) {
  const sizeClasses = {
    sm: "ring-2 ring-offset-2 ring-offset-brand-dark",
    default: "ring-2 ring-offset-2 ring-offset-brand-dark",
    lg: "ring-2 ring-offset-2 ring-offset-brand-dark",
  };

  const ringClasses = {
    sm: "ring-yellow-500/70",
    default: "ring-yellow-500/70", 
    lg: "ring-yellow-500",
  };

  return (
    <div 
      className={`
        relative inline-flex rounded-full
        ${sizeClasses[size]} ${ringClasses[size]}
        ${className}
      `}
    >
      {children}
    </div>
  );
}

// Premium Glow Card - subtle glow effect for premium content cards
export function PremiumGlowCard({ 
  children, 
  className = "" 
}: { 
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <div 
      className={`
        relative rounded-xl overflow-hidden
        before:absolute before:inset-0 before:rounded-xl
        before:border before:border-yellow-500/30
        before:shadow-[0_0_20px_rgba(234,179,8,0.15)]
        ${className}
      `}
    >
      {children}
    </div>
  );
}

// Premium Lock Overlay - elegant locked state for premium content
export function PremiumLockOverlay({ 
  title,
  message,
  className = "" 
}: { 
  title?: string;
  message?: string;
  className?: string;
}) {
  return (
    <div 
      className={`
        flex flex-col items-center justify-center gap-4
        bg-brand-card/95 backdrop-blur-sm rounded-xl p-8
        border border-yellow-500/30
        shadow-[0_0_30px_rgba(234,179,8,0.1)]
        ${className}
      `}
    >
      <div className="w-16 h-16 rounded-full bg-gradient-to-br from-yellow-500 to-amber-600 flex items-center justify-center shadow-[0_0_20px_rgba(234,179,8,0.4)]">
        <Crown size={32} className="text-black" />
      </div>
      <div className="text-center">
        <h3 className="text-lg font-semibold text-white mb-1">
          {title || "Premium Content"}
        </h3>
        <p className="text-gray-400 text-sm">
          {message || "Upgrade to Premium to watch this content"}
        </p>
      </div>
    </div>
  );
}

// Premium Button - CTA button for premium actions
export function PremiumButton({ 
  children, 
  variant = "default",
  className = "",
  ...props 
}: { 
  children: React.ReactNode;
  variant?: "default" | "outline";
  className?: string;
  onClick?: () => void;
}) {
  const baseClasses = "inline-flex items-center justify-center gap-2 font-semibold px-6 py-3 rounded-xl transition-all";
  
  const variantClasses = {
    default: "bg-gradient-to-r from-yellow-500 to-amber-600 text-black hover:from-yellow-400 hover:to-amber-500 shadow-[0_0_15px_rgba(234,179,8,0.3)] hover:shadow-[0_0_25px_rgba(234,179,8,0.5)]",
    outline: "border-2 border-yellow-500 text-yellow-500 hover:bg-yellow-500/10",
  };

  return (
    <button 
      className={`${baseClasses} ${variantClasses[variant]} ${className}`}
      {...props}
    >
      {children}
    </button>
  );
}

// Utility function to check if a movie is premium
export function isMoviePremium(movie: { is_premium?: boolean }): boolean {
  return movie.is_premium === true;
}

// Unified user premium state interface for robust field checking
interface PremiumUserFields {
  is_premium?: boolean;
  is_premium_active?: boolean;
  premium?: boolean; // Alternative field name from some backends
  premium_active?: boolean; // Alternative field name
  premium_expires_at?: string | null;
  premium_started_at?: string | null;
  isPremium?: boolean; // CamelCase variant
}

// Comment user fields interface (for comment author premium detection)
interface CommentUserFields {
  user_is_premium?: boolean;
  user_is_premium_active?: boolean;
  is_premium?: boolean;
  is_premium_active?: boolean;
  premium?: boolean;
  premium_active?: boolean;
}

// Simple resolver for premium status from any user object
// Handles multiple field naming conventions for robustness
export function resolveIsPremium(user: PremiumUserFields | CommentUserFields | null | undefined): boolean {
  if (!user) return false;
  
  // Check user_is_premium_active (common in comments API)
  if ('user_is_premium_active' in user && user.user_is_premium_active === true) return true;
  
  // Check is_premium_active (computed status from backend)
  if ('is_premium_active' in user && user.is_premium_active === true) return true;
  
  // Check alternative premium_active field
  if ('premium_active' in user && user.premium_active === true) return true;
  
  // Check raw is_premium field with expiry check
  if ('is_premium' in user && user.is_premium === true) {
    const expiresAt = ('premium_expires_at' in user ? user.premium_expires_at : null) as string | null;
    if (!expiresAt) return true;
    return new Date(expiresAt) > new Date();
  }
  
  // Check alternative premium field (snake_case)
  if ('premium' in user && user.premium === true) {
    const expiresAt = ('premium_expires_at' in user ? user.premium_expires_at : null) as string | null;
    if (!expiresAt) return true;
    return new Date(expiresAt) > new Date();
  }
  
  // Check user_is_premium (comments API variant)
  if ('user_is_premium' in user && user.user_is_premium === true) {
    const expiresAt = ('premium_expires_at' in user ? user.premium_expires_at : null) as string | null;
    if (!expiresAt) return true;
    return new Date(expiresAt) > new Date();
  }
  
  // Check isPremium (camelCase alternative)
  if ('isPremium' in user && user.isPremium === true) {
    const expiresAt = ('premium_expires_at' in user ? user.premium_expires_at : null) as string | null;
    if (!expiresAt) return true;
    return new Date(expiresAt) > new Date();
  }
  
  return false;
}

// Utility function to check if a user is premium active
// Checks multiple possible field names for robustness
export function isUserPremium(user: PremiumUserFields | null | undefined): boolean {
  if (!user) return false;
  
  // Priority 1: Check is_premium_active (computed status from backend)
  if (user.is_premium_active === true) return true;
  
  // Priority 2: Check alternative premium_active field
  if (user.premium_active === true) return true;
  
  // Priority 3: Check raw is_premium field
  if (user.is_premium === true) {
    // If is_premium is true, also check if it hasn't expired
    // If premium_expires_at is null, it's unlimited
    if (!user.premium_expires_at) return true;
    
    // Check if expiry is in the future
    const expiryDate = new Date(user.premium_expires_at);
    if (expiryDate > new Date()) return true;
  }
  
  // Priority 4: Check alternative premium field (snake_case)
  if (user.premium === true) {
    if (!user.premium_expires_at) return true;
    const expiryDate = new Date(user.premium_expires_at);
    if (expiryDate > new Date()) return true;
  }
  
  // Priority 5: Check isPremium (camelCase alternative)
  if (user.isPremium === true) {
    if (!user.premium_expires_at) return true;
    const expiryDate = new Date(user.premium_expires_at);
    if (expiryDate > new Date()) return true;
  }
  
  return false;
}

// Premium status result interface
export interface PremiumStatus {
  isPremium: boolean;
  startedAt: string | null;
  expiresAt: string | null;
  status: 'active' | 'expired' | 'inactive';
  daysRemaining: number | null;
}

// Resolves comprehensive premium status from user object
// Normalizes all possible field names and provides complete premium info
export function resolvePremiumStatus(user: PremiumUserFields | null | undefined): PremiumStatus {
  const isPremium = isUserPremium(user);
  
  const startedAt = user?.premium_started_at || null;
  const expiresAt = user?.premium_expires_at || null;
  
  let status: 'active' | 'expired' | 'inactive' = 'inactive';
  let daysRemaining: number | null = null;
  
  if (isPremium && expiresAt) {
    const expiryDate = new Date(expiresAt);
    const now = new Date();
    const diffTime = expiryDate.getTime() - now.getTime();
    const diffDays = Math.ceil(diffTime / (1000 * 60 * 60 * 24));
    
    if (diffDays > 0) {
      status = 'active';
      daysRemaining = diffDays;
    } else {
      status = 'expired';
    }
  } else if (isPremium && !expiresAt) {
    // Unlimited premium (no expiry)
    status = 'active';
  }
  
  return {
    isPremium,
    startedAt,
    expiresAt,
    status,
    daysRemaining,
  };
}
