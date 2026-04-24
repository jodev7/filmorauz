import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import { Metadata } from "next";
import Link from "next/link";
import { cookies } from "next/headers";
import { User as UserIcon, Crown, Shield, Send, Hash, LayoutDashboard, BadgeCheck, Sparkles, Clock, Calendar } from "lucide-react";
import { PremiumAvatarRing, resolveIsPremium, resolvePremiumStatus } from "@/components/PremiumComponents";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getPublicUser, getCurrentUser, PublicUserProfile } from "@/lib/api";
import { DEFAULT_AVATAR_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";

interface PageProps {
  params: { id: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  return {
    title: "Foydalanuvchi — FILMORAUZ",
    description: "Foydalanuvchi profil sahifasi",
  };
}

// Format date for display - Uzbek format
const formatDate = (dateString?: string | null): string => {
  if (!dateString) return "N/A";
  try {
    const date = new Date(dateString);
    const day = date.getDate().toString().padStart(2, '0');
    const month = date.getMonth() + 1;
    const monthNames: Record<number, string> = {
      1: 'Yan', 2: 'Fev', 3: 'Mar', 4: 'Apr', 5: 'May', 6: 'Iyun',
      7: 'Iyul', 8: 'Avg', 9: 'Sen', 10: 'Okt', 11: 'Noy', 12: 'Dek'
    };
    const monthName = monthNames[month] || month.toString();
    const year = date.getFullYear();
    return `${day} ${monthName}, ${year}`;
  } catch {
    return "N/A";
  }
};

// Format datetime for display - Uzbek format with time
const formatDateTime = (dateString?: string | null): string => {
  if (!dateString) return "N/A";
  try {
    const date = new Date(dateString);
    const day = date.getDate().toString().padStart(2, '0');
    const month = date.getMonth() + 1;
    const monthNames: Record<number, string> = {
      1: 'Yan', 2: 'Fev', 3: 'Mar', 4: 'Apr', 5: 'May', 6: 'Iyun',
      7: 'Iyul', 8: 'Avg', 9: 'Sen', 10: 'Okt', 11: 'Noy', 12: 'Dek'
    };
    const monthName = monthNames[month] || month.toString();
    const year = date.getFullYear();
    const hours = date.getHours().toString().padStart(2, '0');
    const minutes = date.getMinutes().toString().padStart(2, '0');
    return `${day} ${monthName}, ${year} ${hours}:${minutes}`;
  } catch {
    return "N/A";
  }
};

// Format role for display
const formatRole = (role: string) => {
  if (role === "admin") return "Moderator";
  if (role === "superadmin") return "SuperAdmin";
  return "Foydalanuvchi";
};

// Get display name with proper priority
const getDisplayName = (profile: PublicUserProfile | null): string => {
  if (!profile) return "Foydalanuvchi";
  if (profile.display_name) return profile.display_name;
  if (profile.username) return `@${profile.username}`;
  return "Foydalanuvchi";
};

// Get avatar initial for fallback
const getAvatarInitial = (profile: PublicUserProfile | null): string => {
  const name = getDisplayName(profile);
  return name.charAt(0).toUpperCase() || "U";
};

export default async function UserProfilePage({ params }: PageProps) {
  const { id } = params;

  // Read auth token once — used for both viewer identification and profile fetch
  const cookieStore = await cookies();
  const authToken = cookieStore.get("auth_token")?.value;

  // Get current viewer's information for permission checks
  let viewerUser = null;
  let viewerIsAdmin = false;
  let viewerIsSuperAdmin = false;
  let viewerIsOwner = false;

  try {
    if (authToken) {
      const authResponse = await getCurrentUser(authToken);
      if (authResponse.authenticated && authResponse.user) {
        viewerUser = authResponse.user;
        viewerIsAdmin = viewerUser.role === "admin" || viewerUser.role === "superadmin";
        viewerIsSuperAdmin = viewerUser.role === "superadmin";
        viewerIsOwner = viewerUser.id === id;
      }
    }
  } catch {
    // viewer identification is best-effort
  }

  // Fetch user profile — pass token so the backend can enforce privacy correctly.
  // If the profile is private and the requester is not the owner/admin, the API
  // returns 403 and getPublicUser throws Error("Profile hidden").
  let profile: PublicUserProfile | null = null;
  let profileHidden = false;
  try {
    profile = await getPublicUser(id, authToken);
  } catch (error: unknown) {
    if (error instanceof Error && error.message === "Profile hidden") {
      profileHidden = true;
    }
  }

  // Profile owner's properties (for badges and profile display)
  const isPremiumUser = resolveIsPremium(profile);
  const premiumStatus = resolvePremiumStatus(profile as any);
  const profileIsAdmin = profile?.role === "admin" || profile?.role === "superadmin";
  const profileIsSuperAdmin = profile?.role === "superadmin";
  
  // BUG FIX: Admin panel button should depend on VIEWER's role, not profile owner's role
  // Only show admin panel button if current logged-in user is admin/superadmin
  const canViewAdminPanel = viewerIsAdmin;

  // Check if profile is private - only owner or admin can view full profile
  const isPrivateProfile = profile?.is_private === true;
  const canViewFullProfile = viewerIsOwner || viewerIsAdmin;

  // Show hidden-profile UI when:
  // - backend returned 403 (profileHidden = true), OR
  // - profile loaded with is_private flag but viewer is not authorized (client-side fallback)
  if (profileHidden || (profile && isPrivateProfile && !canViewFullProfile)) {
    return (
      <>
        <Navbar />
        <main className="min-h-screen pt-20 sm:pt-24">
          <div className="max-w-3xl mx-auto px-4">
            <div className="flex flex-col items-center justify-center py-16">
              <div className="w-20 h-20 rounded-full bg-gray-700/50 flex items-center justify-center mb-6">
                <Shield size={40} className="text-gray-500" />
              </div>
              <h1 className="font-display text-2xl sm:text-3xl text-white mb-3 text-center">
                🔒 XUSUSIY PROFIL
              </h1>
              <p className="text-gray-400 text-center max-w-md mb-2">
                Bu foydalanuvchi o&apos;z profilini yashirgan
              </p>
              <p className="text-gray-500 text-sm text-center max-w-md mb-8">
                Siz ushbu profilni ko&apos;ra olmaysiz
              </p>
              <Link
                href="/"
                className="inline-flex items-center gap-2 px-6 py-3 bg-brand-red text-white rounded-lg hover:bg-orange-600 transition-colors font-medium"
              >
                Bosh sahifaga qaytish
              </Link>
            </div>
          </div>
        </main>
        <Footer />
      </>
    );
  }

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-3xl mx-auto px-4">
          
          {/* Centered profile header */}
          <div className={`relative rounded-2xl border p-8 mb-8 overflow-hidden text-center ${
            isPremiumUser 
              ? 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/30 shadow-[0_0_40px_rgba(234,179,8,0.1)]' 
              : 'bg-brand-card border-brand-border'
          }`}>
            {/* Premium decorative glow */}
            {isPremiumUser && (
              <div className="absolute -top-20 -right-20 w-40 h-40 bg-yellow-500/10 rounded-full blur-3xl" />
            )}

            {/* Centered avatar */}
            <div className="relative inline-block mb-4">
              <PremiumAvatarRing size="lg" className={isPremiumUser ? '' : 'ring-transparent ring-offset-0'}>
                <div className={`w-28 h-28 sm:w-32 sm:h-32 rounded-full flex items-center justify-center text-white text-5xl sm:text-6xl font-bold overflow-hidden ${
                  isPremiumUser 
                    ? 'bg-gradient-to-br from-yellow-500/20 to-amber-600/20 border-2 border-yellow-500/50' 
                    : 'bg-gradient-to-br from-brand-red/80 to-orange-600/80'
                }`}>
                  {profile?.avatar_url ? (() => {
                    const avatarSrc = normalizeMediaUrl(profile.avatar_url, DEFAULT_AVATAR_PLACEHOLDER);
                    return (
                    <img
                      key={avatarSrc}
                      src={avatarSrc}
                      alt="Profile"
                      className="w-full h-full object-cover"
                    />
                    );
                  })() : (
                    getAvatarInitial(profile)
                  )}
                </div>
              </PremiumAvatarRing>
              
              {/* Premium crown indicator on avatar */}
              {isPremiumUser && (
                <div className="absolute -top-1 -right-1 bg-gradient-to-r from-yellow-500 to-amber-600 rounded-full p-1.5 shadow-[0_0_15px_rgba(234,179,8,0.5)]">
                  <Crown size={14} className="text-black" />
                </div>
              )}
            </div>

            {/* Display name */}
            <h2 className="font-body text-2xl sm:text-3xl text-white mb-3">
              {getDisplayName(profile)}
            </h2>

            {/* Badges row - Premium and Admin badges (based on profile OWNER's properties) */}
            <div className="flex items-center justify-center gap-2 mb-4 flex-wrap">
              {isPremiumUser && (
                <span className="inline-flex items-center gap-1.5 px-3 py-1 bg-gradient-to-r from-yellow-500 to-amber-600 text-black text-xs font-semibold rounded-full shadow-[0_0_15px_rgba(234,179,8,0.4)]">
                  <Sparkles size={12} className="text-black" />
                  Premium
                </span>
              )}
              {profileIsAdmin && (
                <span className={`inline-flex items-center gap-1.5 px-3 py-1 text-xs font-semibold rounded-full border ${
                  profileIsSuperAdmin 
                    ? 'bg-purple-500/20 text-purple-400 border-purple-500/40 shadow-[0_0_10px_rgba(168,85,247,0.3)]' 
                    : 'bg-brand-red/20 text-brand-red border-brand-red/40 shadow-[0_0_10px_rgba(239,68,68,0.2)]'
                }`}>
                  {profileIsSuperAdmin ? <BadgeCheck size={12} /> : <Shield size={12} />}
                  {profileIsSuperAdmin ? "SuperAdmin" : "Admin"}
                </span>
              )}
            </div>

            {/* Telegram username row */}
            {profile?.username && (
              <div className="flex items-center justify-center gap-2 text-sm text-gray-400 mb-1">
                <Send size={14} className="text-blue-400 flex-shrink-0" />
                <span>@{profile.username}</span>
              </div>
            )}

            {/* Admin dashboard link - FIX: Only show if CURRENT VIEWER is admin/superadmin, not if profile owner is admin */}
            {canViewAdminPanel && (
              <div className="mt-4">
                <Link
                  href="/admin/dashboard"
                  className={`inline-flex items-center gap-2 px-5 py-2.5 rounded-lg border font-medium text-sm transition-all ${
                    viewerIsSuperAdmin
                      ? 'bg-purple-500/10 text-purple-400 border-purple-500/40 hover:bg-purple-500/20 hover:border-purple-500/60'
                      : 'bg-brand-red/10 text-brand-red border-brand-red/30 hover:bg-brand-red/20 hover:border-brand-red/50'
                  }`}
                >
                  <LayoutDashboard size={16} />
                  Admin panel
                </Link>
              </div>
            )}
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="user_profile_page_banner" variant="banner" />
          </div>

          {/* Profile details card - only show if profile is not private or viewer can see it */}
          {profile && (!isPrivateProfile || canViewFullProfile) && (
            <div className={`rounded-2xl border p-6 mb-8 ${
              isPremiumUser 
                ? 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/20' 
                : 'bg-brand-card border-brand-border'
            }`}>
              <h3 className="font-display text-lg text-white mb-4 flex items-center gap-2">
                <Shield size={16} className={isPremiumUser ? 'text-yellow-500' : 'text-gray-400'} />
                Foydalanuvchi ma&apos;lumotlari
              </h3>
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                {/* Rol (based on profile OWNER's role) */}
                <div className={`p-3 rounded-xl border ${
                  profileIsAdmin 
                    ? profileIsSuperAdmin 
                      ? 'bg-purple-500/10 border-purple-500/30' 
                      : 'bg-brand-red/10 border-brand-red/30'
                    : 'bg-brand-dark/50 border-brand-border/50'
                }`}>
                  <div className={`flex items-center gap-2 text-xs uppercase tracking-wide mb-1 ${
                    profileIsAdmin ? (profileIsSuperAdmin ? 'text-purple-400' : 'text-brand-red') : 'text-gray-500'
                  }`}>
                    {profileIsAdmin ? (
                      profileIsSuperAdmin ? <BadgeCheck size={12} /> : <Shield size={12} />
                    ) : (
                      <Shield size={12} />
                    )}
                    Rol
                  </div>
                  <div className={`font-medium ${
                    profileIsAdmin ? (profileIsSuperAdmin ? 'text-purple-300' : 'text-brand-red/80') : 'text-white'
                  }`}>
                    {formatRole(profile.role)}
                  </div>
                </div>

                {/* Ro'yxatdan o'tgan */}
                <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                  <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                    <UserIcon size={12} />
                    Ro&apos;yxatdan o&apos;tgan
                  </div>
                  <div className="text-white font-medium">
                    {formatDate(profile.created_at)}
                  </div>
                </div>

                {/* Telegram */}
                {profile.username && (
                  <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                    <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                      <Send size={12} />
                      Telegram
                    </div>
                    <div className="text-white font-medium">
                      @{profile.username}
                    </div>
                  </div>
                )}

                {/* ID */}
                <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                  <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                    <Hash size={12} />
                    ID
                  </div>
                  <div className="text-white font-mono text-sm">
                    {profile.id || id}
                  </div>
                </div>

                {/* Premium holati */}
                {premiumStatus.isPremium ? (
                  <>
                    <div className="p-3 bg-gradient-to-r from-yellow-500/10 to-amber-600/10 rounded-xl border border-yellow-500/30">
                      <div className="flex items-center gap-2 text-yellow-500 text-xs uppercase tracking-wide mb-1">
                        <Crown size={12} />
                        Premium holati
                      </div>
                      <div className="text-white font-medium flex items-center gap-2">
                        <span className="w-2 h-2 rounded-full bg-yellow-500 animate-pulse"></span>
                        Premium faol
                        {premiumStatus.daysRemaining !== null && (
                          <span className="text-yellow-500/70 text-xs">
                            ({premiumStatus.daysRemaining} kun qoldi)
                          </span>
                        )}
                      </div>
                    </div>
                    
                    {/* BUG FIX: Premium boshlangan - Only visible to profile OWNER or admin/superadmin */}
                    {(viewerIsOwner || viewerIsAdmin) && (
                      <>
                        <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                          <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                            <Calendar size={12} />
                            Boshlangan
                          </div>
                          <div className="text-white font-medium">
                            {premiumStatus.startedAt ? formatDateTime(premiumStatus.startedAt) : "N/A"}
                          </div>
                        </div>
                        
                        {/* Premium tugaydi */}
                        <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                          <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                            <Clock size={12} />
                            Tugaydi
                          </div>
                          <div className="text-white font-medium">
                            {premiumStatus.expiresAt ? formatDateTime(premiumStatus.expiresAt) : "Cheksiz"}
                          </div>
                        </div>
                      </>
                    )}
                  </>
                ) : (
                  <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                    <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                      <Crown size={12} />
                      Premium holati
                    </div>
                    <div className="text-gray-400 font-medium">
                      Oddiy foydalanuvchi
                    </div>
                  </div>
                )}
              </div>
            </div>
          )}

          {/* Back link */}
          <div className="pb-8 text-center">
            <Link 
              href="/" 
              className="inline-flex items-center gap-2 px-4 py-2 text-gray-400 hover:text-white transition-colors"
            >
              <span className="text-brand-red">←</span> Bosh sahifaga qaytish
            </Link>
          </div>

          {/* Bottom padding */}
          <div className="pb-12" />
        </div>
      </main>
      <Footer />
    </>
  );
}
