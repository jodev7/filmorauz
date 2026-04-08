"use client";

import { useState, useEffect, useRef } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import MovieCarousel from "@/components/MovieCarousel";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";
import Link from "next/link";
import { Heart, History, User as UserIcon, Crown, Calendar, Shield, Clock, Camera, Edit2, Check, X, Send, Hash, LayoutDashboard, BadgeCheck, Sparkles, RefreshCw, Zap, Palette, Eye, EyeOff, Lock, Star } from "lucide-react";
import { PremiumBadge, PremiumButton, PremiumAvatarRing, resolveIsPremium, resolvePremiumStatus } from "@/components/PremiumComponents";
import { getFavorites, getWatchHistory, getCurrentUser, updateProfile, uploadProfileImage, updateProfileStyle, updatePrivacySettings, ProfileStyle } from "@/lib/api";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { CurrentUser } from "@/lib/api";

// Subtle premium animations (luxury, not flashy)
const premiumAnimations = `
  @keyframes premiumGlowPulse {
    0%, 100% {
      opacity: 0.4;
      transform: scale(1);
      filter: blur(8px);
    }
    50% {
      opacity: 0.7;
      transform: scale(1.05);
      filter: blur(12px);
    }
  }
  
  @keyframes premiumBadgeShimmer {
    0% {
      background-position: -200% center;
    }
    100% {
      background-position: 200% center;
    }
  }
  
  @keyframes premiumProgressShine {
    0% {
      left: -100%;
      opacity: 0;
    }
    10% {
      opacity: 1;
    }
    90% {
      opacity: 1;
    }
    100% {
      left: 100%;
      opacity: 0;
    }
  }
  
  @keyframes premiumCrownFloat {
    0%, 100% {
      transform: translateY(0);
    }
    50% {
      transform: translateY(-2px);
    }
  }
  
  .premium-glow-pulse {
    animation: premiumGlowPulse 3s ease-in-out infinite;
    pointer-events: none;
  }
  
  .premium-badge-shimmer {
    background: linear-gradient(
      90deg,
      rgba(234, 179, 8, 1) 0%,
      rgba(255, 255, 255, 0.9) 50%,
      rgba(234, 179, 8, 1) 100%
    );
    background-size: 200% 100%;
    animation: premiumBadgeShimmer 4s ease-in-out infinite;
    animation-delay: 2s;
    pointer-events: none;
  }
  
  .premium-progress-shine {
    position: relative;
    overflow: hidden;
    pointer-events: none;
  }
  
  .premium-progress-shine::after {
    content: '';
    position: absolute;
    top: 0;
    left: -100%;
    width: 50%;
    height: 100%;
    background: linear-gradient(
      90deg,
      transparent,
      rgba(255, 255, 255, 0.3),
      transparent
    );
    animation: premiumProgressShine 3s ease-in-out infinite;
    animation-delay: 1s;
    pointer-events: none;
  }
  
  .premium-crown-float {
    animation: premiumCrownFloat 2.5s ease-in-out infinite;
    pointer-events: none;
  }
  
  .premium-card-hover {
    transition: transform 0.3s ease, box-shadow 0.3s ease;
    pointer-events: auto;
  }
  
  .premium-card-hover:hover {
    transform: translateY(-2px);
  }
  
  /* Ensure interactive elements are always on top */
  .premium-interactive {
    position: relative;
    z-index: 10;
    pointer-events: auto;
  }
  
  @media (prefers-reduced-motion: reduce) {
    .premium-glow-pulse,
    .premium-badge-shimmer,
    .premium-progress-shine::after,
    .premium-crown-float {
      animation: none;
    }
  }
`;

export default function UserPage() {
  const { user, isAuthenticated, token, refreshUser } = useAuth();
  const { t } = useI18n();
  const [favorites, setFavorites] = useState<any[]>([]);
  const [watchHistory, setWatchHistory] = useState<any[]>([]);
  const [recentWatched, setRecentWatched] = useState<any[]>([]);
  const [loading, setLoading] = useState(true);
  const [currentUser, setCurrentUser] = useState<CurrentUser | null>(null);

  // Edit states
  const [isEditingName, setIsEditingName] = useState(false);
  const [editedName, setEditedName] = useState("");
  const [isUploadingImage, setIsUploadingImage] = useState(false);
  const [isSavingName, setIsSavingName] = useState(false);
  
  // Premium profile customization states
  const [showStyleSettings, setShowStyleSettings] = useState(false);
  const [selectedFrame, setSelectedFrame] = useState("none");
  const [selectedTheme, setSelectedTheme] = useState("default");
  const [selectedGradient, setSelectedGradient] = useState("none");
  const [isSavingStyle, setIsSavingStyle] = useState(false);
  
  // Privacy settings states
  const [isPrivate, setIsPrivate] = useState(false);
  const [isSavingPrivacy, setIsSavingPrivacy] = useState(false);
  
  const fileInputRef = useRef<HTMLInputElement>(null);

  // Derived user state - normalize all role/premium fields
  // Prioritize 'user' from auth context (updated by refreshUser) over 'currentUser'
  const displayUser = user || currentUser;
  const displayIsPremium = resolveIsPremium(displayUser);
  const premiumStatus = resolvePremiumStatus(displayUser);
  const isAdmin = displayUser?.role === "admin" || displayUser?.role === "superadmin";
  const isSuperAdmin = displayUser?.role === "superadmin";

  useEffect(() => {
    if (isAuthenticated) {
      fetchData();
    } else {
      setLoading(false);
    }
  }, [isAuthenticated]);

  useEffect(() => {
    if (token) {
      getCurrentUser(token)
        .then((response) => {
          if (response.authenticated && response.user) {
            setCurrentUser(response.user);
            // Initialize profile style and privacy settings
            if (response.user.profile_style) {
              setSelectedFrame(response.user.profile_style.frame || "none");
              setSelectedTheme(response.user.profile_style.theme || "default");
              setSelectedGradient(response.user.profile_style.gradient || "none");
            }
            if (response.user.privacy) {
              setIsPrivate(response.user.privacy.is_private || false);
            }
          }
        })
        .catch(console.error);
    }
  }, [token]);

  async function fetchData() {
    try {
      const [favRes, histRes] = await Promise.all([
        getFavorites(token || ""),
        getWatchHistory(token || ""),
      ]);
      
      setFavorites(favRes || []);
      
      const history = histRes || [];
      setWatchHistory(history);
      
      // Recent watched - last 6 items from history
      setRecentWatched(history.slice(0, 6));
    } catch (error) {
      console.error("Error fetching user data:", error);
    }
    setLoading(false);
  }

  // Get display name with proper priority
  const getDisplayName = (user: CurrentUser | null): string => {
    if (!user) return "Foydalanuvchi";
    if (user.display_name) return user.display_name;
    if (user.first_name) {
      if (user.last_name) return `${user.first_name} ${user.last_name}`;
      return user.first_name;
    }
    if (user.username) return `@${user.username}`;
    return "Foydalanuvchi";
  };

  // Get profile image URL - check multiple fields for robustness
  const getProfileImageUrl = (user: CurrentUser | null): string | undefined => {
    if (!user) return undefined;
    return user.profile_image_url || user.photo_url || undefined;
  };

  // Get avatar initial for fallback
  const getAvatarInitial = (user: CurrentUser | null): string => {
    const name = getDisplayName(user);
    return name.charAt(0).toUpperCase() || "U";
  };

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

  // Format datetime for last login
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

  // Get profile style configuration
  const getProfileStyle = () => {
    const style = displayUser?.profile_style;
    return {
      frame: style?.frame || "none",
      theme: style?.theme || "default",
      gradient: style?.gradient || "none",
    };
  };

  // Apply profile style based on customization settings
  const profileStyle = getProfileStyle();
  const isGoldFrame = profileStyle.frame === "gold" && displayIsPremium;
  const isGoldDarkTheme = profileStyle.theme === "gold_dark" && displayIsPremium;
  const isGoldGradient = profileStyle.gradient === "gold" && displayIsPremium;
  const isPurpleGradient = profileStyle.gradient === "purple" && displayIsPremium;

  // Get dynamic profile card classes
  const getProfileCardClasses = () => {
    if (!displayIsPremium) return 'bg-brand-card border-brand-border';
    if (isGoldDarkTheme) return 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/40 shadow-[0_0_60px_rgba(234,179,8,0.15)]';
    if (isPurpleGradient) return 'bg-gradient-to-br from-[#12121A] via-[#15152A] to-[#1a1535] border-purple-500/40 shadow-[0_0_60px_rgba(168,85,247,0.15)]';
    return 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/30 shadow-[0_0_40px_rgba(234,179,8,0.1)]';
  };

  // Get avatar border classes
  const getAvatarBorderClasses = () => {
    if (!displayIsPremium) return 'bg-brand-border';
    if (isGoldFrame || isGoldDarkTheme || isGoldGradient) {
      return 'bg-gradient-to-br from-yellow-500 via-amber-500 to-orange-500 shadow-[0_0_30px_rgba(234,179,8,0.5)]';
    }
    if (isPurpleGradient) {
      return 'bg-gradient-to-br from-purple-500 via-violet-500 to-purple-600 shadow-[0_0_30px_rgba(168,85,247,0.5)]';
    }
    return 'bg-brand-border';
  };

  // Handle name edit
  const handleStartEditName = () => {
    setEditedName(getDisplayName(currentUser || user));
    setIsEditingName(true);
  };

  const handleSaveName = async () => {
    if (!token || !editedName.trim()) return;
    setIsSavingName(true);
    try {
      await updateProfile(token, editedName.trim());
      await refreshUser();
      if (currentUser) {
        setCurrentUser({ ...currentUser, display_name: editedName.trim() });
      }
      setIsEditingName(false);
    } catch (error) {
      console.error("Failed to update name:", error);
    }
    setIsSavingName(false);
  };

  const handleCancelEditName = () => {
    setIsEditingName(false);
    setEditedName("");
  };

  // Handle profile style save
  const handleSaveProfileStyle = async () => {
    if (!token) return;
    setIsSavingStyle(true);
    try {
      const style: ProfileStyle = {
        frame: selectedFrame,
        theme: selectedTheme,
        gradient: selectedGradient,
      };
      await updateProfileStyle(token, style);
      await refreshUser();
      if (currentUser) {
        setCurrentUser({ ...currentUser, profile_style: style });
      }
      setShowStyleSettings(false);
    } catch (error) {
      console.error("Failed to save profile style:", error);
      alert("Profil stilini saqlashda xatolik yuz berdi");
    }
    setIsSavingStyle(false);
  };

  // Handle privacy settings save
  const handleSavePrivacy = async (newIsPrivate: boolean) => {
    if (!token) return;
    setIsSavingPrivacy(true);
    try {
      await updatePrivacySettings(token, { is_private: newIsPrivate });
      await refreshUser();
      if (currentUser) {
        setCurrentUser({ ...currentUser, privacy: { is_private: newIsPrivate } });
      }
    } catch (error) {
      console.error("Failed to save privacy settings:", error);
      alert("Maxfiylik sozlamalarini saqlashda xatolik yuz berdi");
    }
    setIsSavingPrivacy(false);
  };

  // Handle profile image upload
  const handleImageUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file || !token) return;

    setIsUploadingImage(true);
    try {
      const imageUrl = await uploadProfileImage(token, file);
      await refreshUser();
      if (currentUser) {
        setCurrentUser({ ...currentUser, profile_image_url: imageUrl });
      }
    } catch (error) {
      console.error("Failed to upload image:", error);
    }
    setIsUploadingImage(false);
    if (fileInputRef.current) {
      fileInputRef.current.value = "";
    }
  };

  if (!isAuthenticated) {
    return (
      <>
        <Navbar />
        <style dangerouslySetInnerHTML={{ __html: premiumAnimations }} />
        <main className="min-h-screen pt-20 sm:pt-24">
          <div className="max-w-7xl mx-auto px-4">
            <div className="py-24 text-center">
              <div className="inline-flex items-center justify-center w-20 h-20 rounded-full bg-brand-card border border-brand-border mb-6">
                <UserIcon className="w-10 h-10 text-gray-500" />
              </div>
              <h1 className="font-display text-3xl sm:text-4xl text-white mb-4 tracking-wide">
                {t("auth.loginRequired")}
              </h1>
              <p className="text-gray-500 mb-8 max-w-md mx-auto">
                Sevimli filmlaringizni saqlash va tomosha tarixingizni ko&apos;rish uchun tizimga kiring.
              </p>
              <Link 
                href="/" 
                className="inline-flex items-center gap-2 px-6 py-3 bg-brand-red text-white rounded-lg hover:bg-orange-600 transition-colors font-medium"
              >
                Bosh sahifaga
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
      <style dangerouslySetInnerHTML={{ __html: premiumAnimations }} />
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-3xl mx-auto px-4">
          
          {/* Centered profile header */}
          <div className={`relative rounded-2xl border p-8 mb-8 overflow-hidden text-center premium-card-hover ${
            displayIsPremium 
              ? 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/40 shadow-[0_0_60px_rgba(234,179,8,0.15)]' 
              : 'bg-brand-card border-brand-border'
          }`}>
            {/* Premium decorative glow effects - pointer-events: none to not block clicks */}
            {displayIsPremium && (
              <>
                <div className="absolute -top-20 -right-20 w-60 h-60 bg-yellow-500/15 rounded-full blur-3xl premium-glow-pulse" style={{ pointerEvents: 'none' }} />
                <div className="absolute -bottom-20 -left-20 w-40 h-40 bg-amber-500/10 rounded-full blur-3xl premium-glow-pulse" style={{ animationDelay: '0.5s', pointerEvents: 'none' }} />
                <div className="absolute top-1/2 left-1/2 -translate-x-1/2 -translate-y-1/2 w-[300px] h-[200px] bg-gradient-to-t from-yellow-500/5 to-transparent rounded-full blur-2xl" style={{ pointerEvents: 'none' }} />
              </>
            )}

            {/* Premium user label */}
            {displayIsPremium && (
              <div className="mb-4 relative z-10">
                <span className="inline-flex items-center gap-2 px-4 py-1.5 bg-gradient-to-r from-yellow-500/20 to-amber-500/20 border border-yellow-500/40 rounded-full text-yellow-400 text-xs font-semibold shadow-[0_0_20px_rgba(234,179,8,0.3)]">
                  <Crown size={14} className="text-yellow-400 premium-crown-float" />
                  👑 PREMIUM FOYDALANUVCHI
                </span>
              </div>
            )}

            {/* Centered avatar */}
            <div className="relative inline-block mb-4">
              {/* Premium animated glow ring - pointer-events: none via CSS */}
              {displayIsPremium && (
                <div className="absolute inset-0 rounded-full premium-glow-pulse" style={{ pointerEvents: 'none' }}>
                  <div className="w-full h-full rounded-full bg-gradient-to-br from-yellow-500 via-amber-500 to-yellow-600 opacity-50 blur-sm" />
                </div>
              )}
              
              <div className={`relative rounded-full p-1 z-10 ${
                displayIsPremium 
                  ? 'bg-gradient-to-br from-yellow-500 via-amber-500 to-orange-500 shadow-[0_0_30px_rgba(234,179,8,0.5)]' 
                  : 'bg-brand-border'
              }`}>
                <div className={`w-28 h-28 sm:w-32 sm:h-32 rounded-full flex items-center justify-center text-white text-5xl sm:text-6xl font-bold overflow-hidden ${
                  displayIsPremium 
                    ? 'bg-gradient-to-br from-[#1a1520] to-[#12121A] border-2 border-yellow-500/30' 
                    : 'bg-gradient-to-br from-brand-red/80 to-orange-600/80'
                }`}>
                  {getProfileImageUrl(displayUser) ? (
                    <img 
                      src={getProfileImageUrl(displayUser)} 
                      alt="Profile" 
                      className="w-full h-full object-cover" 
                    />
                  ) : (
                    getAvatarInitial(displayUser)
                  )}
                </div>
              </div>
              
              {/* Camera edit button - ensure it's above decorative layers */}
              <button
                onClick={() => fileInputRef.current?.click()}
                disabled={isUploadingImage}
                className={`absolute bottom-1 right-1 p-2 rounded-full transition-all z-20 ${
                  displayIsPremium 
                    ? 'bg-gradient-to-r from-yellow-500 to-amber-600 hover:from-yellow-400 hover:to-amber-500 shadow-[0_0_15px_rgba(234,179,8,0.6)] hover:shadow-[0_0_20px_rgba(234,179,8,0.8)]' 
                    : 'bg-brand-red hover:bg-orange-600 shadow-lg'
                } ${isUploadingImage ? 'opacity-50 cursor-not-allowed' : ''}`}
                title="Rasmni o&apos;zgartirish"
              >
                {isUploadingImage ? (
                  <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                ) : (
                  <Camera size={16} className="text-white" />
                )}
              </button>
              <input
                ref={fileInputRef}
                type="file"
                accept="image/*"
                onChange={handleImageUpload}
                className="hidden"
              />

              {/* Premium crown indicator on avatar - z-index above decorative layers */}
              {displayIsPremium && (
                <div className="absolute -top-2 -right-2 bg-gradient-to-r from-yellow-500 to-amber-600 rounded-full p-2 shadow-[0_0_20px_rgba(234,179,8,0.6)] premium-crown-float z-20" style={{ pointerEvents: 'none' }}>
                  <Crown size={16} className="text-black" />
                </div>
              )}
            </div>

            {/* Display name with edit */}
            <div className="mb-3">
              {isEditingName ? (
                <div className="flex items-center justify-center gap-2 relative z-10">
                  <input
                    type="text"
                    value={editedName}
                    onChange={(e) => setEditedName(e.target.value)}
                    className="bg-brand-dark border border-brand-border rounded-lg px-3 py-1.5 text-white text-xl font-body text-center focus:outline-none focus:border-brand-red max-w-[220px]"
                    placeholder="Ismingizni kiriting"
                  />
                  <button
                    onClick={handleSaveName}
                    disabled={isSavingName || !editedName.trim()}
                    className="p-2 rounded-lg bg-green-600 hover:bg-green-500 transition-colors disabled:opacity-50 relative z-10"
                    title="Saqlash"
                  >
                    {isSavingName ? (
                      <div className="w-4 h-4 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    ) : (
                      <Check size={16} className="text-white" />
                    )}
                  </button>
                  <button
                    onClick={handleCancelEditName}
                    disabled={isSavingName}
                    className="p-2 rounded-lg bg-gray-600 hover:bg-gray-500 transition-colors disabled:opacity-50 relative z-10"
                    title="Bekor qilish"
                  >
                    <X size={16} className="text-white" />
                  </button>
                </div>
              ) : (
                <div className="flex items-center justify-center gap-2">
                  <h2 className={`font-body text-2xl sm:text-3xl ${displayIsPremium ? 'text-yellow-50' : 'text-white'}`}>
                    {getDisplayName(displayUser)}
                  </h2>
                  <button
                    onClick={handleStartEditName}
                    className={`p-1.5 rounded-lg transition-colors z-10 ${
                      displayIsPremium 
                        ? 'bg-yellow-500/20 border border-yellow-500/30 hover:border-yellow-500/50 text-yellow-400 hover:text-yellow-300' 
                        : 'bg-brand-dark/50 border border-brand-border/50 hover:border-brand-red/50 text-gray-400 hover:text-white'
                    }`}
                    title="Ismni o&apos;zgartirish"
                  >
                    <Edit2 size={14} />
                  </button>
                </div>
              )}
            </div>

            {/* Badges row - Premium and Admin badges with enhanced styling */}
            <div className="flex items-center justify-center gap-2 mb-3 flex-wrap">
              {displayIsPremium && (
                <span className="inline-flex items-center gap-1.5 px-4 py-1.5 bg-gradient-to-r from-yellow-500 to-amber-600 text-black text-xs font-bold rounded-full shadow-[0_0_20px_rgba(234,179,8,0.5)] premium-badge-shimmer">
                  <Crown size={12} className="text-black" />
                  PREMIUM
                </span>
              )}
              {isAdmin && (
                <span className={`inline-flex items-center gap-1.5 px-3 py-1 text-xs font-semibold rounded-full border ${
                  isSuperAdmin 
                    ? 'bg-purple-500/20 text-purple-400 border-purple-500/40 shadow-[0_0_15px_rgba(168,85,247,0.4)]' 
                    : 'bg-brand-red/20 text-brand-red border-brand-red/40 shadow-[0_0_10px_rgba(239,68,68,0.3)]'
                }`}>
                  {isSuperAdmin ? <BadgeCheck size={12} /> : <Shield size={12} />}
                  {isSuperAdmin ? "SuperAdmin" : "Admin"}
                </span>
              )}
            </div>

            {/* Admin Dashboard Button - prominently placed for admin users */}
            {isAdmin && (
              <div className="mb-4 relative z-10">
                <Link
                  href="/admin/dashboard"
                  className={`inline-flex items-center gap-2 px-5 py-2.5 rounded-lg border font-medium text-sm transition-all ${
                    isSuperAdmin
                      ? 'bg-purple-500/10 text-purple-400 border-purple-500/40 hover:bg-purple-500/20 hover:border-purple-500/60 shadow-[0_0_15px_rgba(168,85,247,0.2)]'
                      : 'bg-brand-red/10 text-brand-red border-brand-red/30 hover:bg-brand-red/20 hover:border-brand-red/50 shadow-[0_0_15px_rgba(239,68,68,0.2)]'
                  }`}
                >
                  <LayoutDashboard size={16} />
                  Admin panel
                </Link>
              </div>
            )}

            {/* Telegram username row */}
            {displayUser?.username && (
              <div className="flex items-center justify-center gap-2 text-sm text-gray-400 mb-1">
                <Send size={14} className="text-blue-400 flex-shrink-0" />
                <span>@{displayUser.username}</span>
              </div>
            )}

            {/* Telegram ID row */}
            {displayUser?.telegram_id && (
              <div className="flex items-center justify-center gap-2 text-sm text-gray-400 mb-4">
                <Hash size={14} className="text-cyan-400 flex-shrink-0" />
                <span>ID: {displayUser.telegram_id}</span>
              </div>
            )}

            {/* Quick stats */}
            <div className="flex items-center justify-center gap-4">
              <div className="flex items-center gap-2 px-3 py-1.5 bg-brand-dark/50 rounded-lg border border-brand-border/50">
                <Heart size={14} className="text-brand-red" />
                <span className="text-sm text-gray-300">{favorites.length} Sevimli</span>
              </div>
              <div className="flex items-center gap-2 px-3 py-1.5 bg-brand-dark/50 rounded-lg border border-brand-border/50">
                <History size={14} className="text-blue-500" />
                <span className="text-sm text-gray-300">{watchHistory.length} Tarix</span>
              </div>
            </div>
          </div>

          {/* Detailed profile info card */}
          <div className={`rounded-2xl border p-6 mb-8 ${
            displayIsPremium 
              ? 'bg-gradient-to-br from-[#12121A] via-[#151520] to-[#1a1525] border-yellow-500/20' 
              : 'bg-brand-card border-brand-border'
          }`}>
            <h3 className="font-display text-lg text-white mb-4 flex items-center gap-2">
              <Shield size={16} className={displayIsPremium ? 'text-yellow-500' : 'text-gray-400'} />
              Hisob ma&apos;lumotlari
            </h3>
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              
              {/* Rol */}
              <div className={`p-3 rounded-xl border ${
                isAdmin 
                  ? isSuperAdmin 
                    ? 'bg-purple-500/10 border-purple-500/30' 
                    : 'bg-brand-red/10 border-brand-red/30'
                  : 'bg-brand-dark/50 border-brand-border/50'
              }`}>
                <div className={`flex items-center gap-2 text-xs uppercase tracking-wide mb-1 ${
                  isAdmin ? (isSuperAdmin ? 'text-purple-400' : 'text-brand-red') : 'text-gray-500'
                }`}>
                  {isAdmin ? (
                    isSuperAdmin ? <BadgeCheck size={12} /> : <Shield size={12} />
                  ) : (
                    <Shield size={12} />
                  )}
                  Rol
                </div>
                <div className={`font-medium ${
                  isAdmin ? (isSuperAdmin ? 'text-purple-300' : 'text-brand-red/80') : 'text-white'
                }`}>
                  {formatRole(displayUser?.role || "user")}
                </div>
              </div>

              {/* Ro'yxatdan o'tgan */}
              <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                  <Calendar size={12} />
                  Ro&apos;yxatdan o&apos;tgan
                </div>
                <div className="text-white font-medium">
                  {formatDate(displayUser?.created_at)}
                </div>
              </div>

              {/* Oxirgi kirish */}
              <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                  <Clock size={12} />
                  Oxirgi kirish
                </div>
                <div className="text-white font-medium">
                  {formatDateTime(displayUser?.last_login_at)}
                </div>
              </div>

              {/* Telegram */}
              {displayUser?.username && (
                <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                  <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                    <Send size={12} />
                    Telegram
                  </div>
                  <div className="text-white font-medium">
                    @{displayUser.username}
                  </div>
                </div>
              )}

              {/* Telegram ID */}
              <div className="p-3 bg-brand-dark/50 rounded-xl border border-brand-border/50">
                <div className="flex items-center gap-2 text-gray-500 text-xs uppercase tracking-wide mb-1">
                  <Hash size={12} />
                  ID
                </div>
                <div className="text-white font-mono text-sm">
                  {displayUser?.telegram_id || "N/A"}
                </div>
              </div>

              {/* Premium holati */}
              {premiumStatus.isPremium ? (
                <>
                  {/* Premium status with progress bar */}
                  <div className="col-span-1 sm:col-span-2 p-4 bg-gradient-to-r from-yellow-500/15 via-amber-500/10 to-orange-500/15 rounded-xl border border-yellow-500/40 shadow-[0_0_20px_rgba(234,179,8,0.1)]">
                    <div className="flex items-center justify-between mb-3">
                      <div className="flex items-center gap-2">
                        <Crown size={16} className="text-yellow-500" />
                        <span className="text-yellow-400 text-sm font-semibold">Premium holati</span>
                      </div>
                      <div className="flex items-center gap-2">
                        <Zap size={14} className="text-yellow-500/70" />
                        <span className="text-yellow-500 text-xs">
                          {premiumStatus.daysRemaining !== null ? `${premiumStatus.daysRemaining} kun qoldi` : 'Cheksiz'}
                        </span>
                      </div>
                    </div>
                    
                    {/* Progress bar */}
                    {premiumStatus.daysRemaining !== null && premiumStatus.startedAt && premiumStatus.expiresAt ? (
                      <div className="relative">
                        {/* Calculate total subscription days */}
                        {(() => {
                          const start = new Date(premiumStatus.startedAt);
                          const end = new Date(premiumStatus.expiresAt);
                          const totalDays = Math.ceil((end.getTime() - start.getTime()) / (1000 * 60 * 60 * 24));
                          const remaining = premiumStatus.daysRemaining;
                          const progressPercent = Math.max(5, Math.min(100, (remaining / totalDays) * 100));
                          return (
                            <>
                              <div className="h-3 bg-brand-dark/80 rounded-full overflow-hidden border border-yellow-500/20 premium-progress-shine">
                                <div
                                  className="h-full bg-gradient-to-r from-yellow-500 via-amber-500 to-orange-500 rounded-full transition-all duration-500"
                                  style={{ width: `${progressPercent}%` }}
                                />
                              </div>
                              <div className="flex justify-between mt-1">
                                <span className="text-xs text-yellow-500/60">{formatDate(premiumStatus.startedAt)}</span>
                                <span className="text-xs text-yellow-500/60">{formatDate(premiumStatus.expiresAt)}</span>
                              </div>
                            </>
                          );
                        })()}
                      </div>
                    ) : (
                      <div className="flex items-center gap-2 text-yellow-400">
                        <span className="w-3 h-3 rounded-full bg-yellow-500 animate-pulse"></span>
                        <span className="font-medium">Premium faol</span>
                        <span className="text-yellow-500/70">(Cheksiz)</span>
                      </div>
                    )}
                    
                    {/* Renew button */}
                    <div className="mt-4">
                      <Link href="/premium">
                        <button className="w-full bg-gradient-to-r from-yellow-500 to-amber-600 hover:from-yellow-400 hover:to-amber-500 text-black font-semibold px-4 py-2.5 rounded-lg transition-all shadow-[0_0_20px_rgba(234,179,8,0.3)] hover:shadow-[0_0_30px_rgba(234,179,8,0.5)] flex items-center justify-center gap-2">
                          <RefreshCw size={16} className="text-black" />
                          Yangilash
                        </button>
                      </Link>
                    </div>
                  </div>
                  
                  {/* Premium boshlangan */}
                  <div className="p-3 bg-gradient-to-br from-yellow-500/10 to-amber-500/5 rounded-xl border border-yellow-500/30">
                    <div className="flex items-center gap-2 text-yellow-500/80 text-xs uppercase tracking-wide mb-1">
                      <Calendar size={12} />
                      Boshlangan
                    </div>
                    <div className="text-white font-medium">
                      {premiumStatus.startedAt ? formatDateTime(premiumStatus.startedAt) : "N/A"}
                    </div>
                  </div>
                  
                  {/* Premium tugaydi */}
                  <div className="p-3 bg-gradient-to-br from-yellow-500/10 to-amber-500/5 rounded-xl border border-yellow-500/30">
                    <div className="flex items-center gap-2 text-yellow-500/80 text-xs uppercase tracking-wide mb-1">
                      <Clock size={12} />
                      Tugaydi
                    </div>
                    <div className="text-white font-medium">
                      {premiumStatus.expiresAt ? formatDateTime(premiumStatus.expiresAt) : "Cheksiz"}
                    </div>
                  </div>
                </>
              ) : (
                <div className="p-3 bg-gradient-to-r from-brand-red/10 to-orange-600/10 rounded-xl border border-brand-red/30">
                  <div className="flex items-center gap-2 text-gray-400 text-xs uppercase tracking-wide mb-1">
                    <Crown size={12} />
                    Premium holati
                  </div>
                  <div className="text-gray-400 font-medium mb-3">
                    Oddiy foydalanuvchi
                  </div>
                  <Link href="/premium" className="block">
                    <button className="w-full bg-gradient-to-r from-yellow-500 to-amber-600 hover:from-yellow-400 hover:to-amber-500 text-black font-semibold px-4 py-2.5 rounded-lg transition-all shadow-[0_0_15px_rgba(234,179,8,0.3)] flex items-center justify-center gap-2">
                      <Crown size={16} className="text-black" />
                      Premium olish
                    </button>
                  </Link>
                </div>
              )}
            </div>
          </div>

          {/* Premium Settings - Consolidated section for premium users */}
          {displayIsPremium && (
            <div className={`rounded-2xl border p-6 mb-8 ${getProfileCardClasses()}`}>
              <div className="flex items-center justify-between mb-4">
                <h3 className="font-display text-lg text-white flex items-center gap-2">
                  <Sparkles size={18} className="text-yellow-500" />
                  Premium sozlamalar
                </h3>
                <button
                  onClick={() => setShowStyleSettings(!showStyleSettings)}
                  className="text-sm text-yellow-500 hover:text-yellow-400 transition-colors"
                >
                  {showStyleSettings ? "Yashirish" : "Sozlash"}
                </button>
              </div>
              
              {showStyleSettings && (
                <div className="space-y-6">
                  {/* Profile Customization Section */}
                  <div className="space-y-4">
                    <h4 className="text-sm font-medium text-gray-300 flex items-center gap-2">
                      <Palette size={14} className="text-yellow-500" />
                      Profil ko&apos;rinishi
                    </h4>
                    
                    {/* Frame selection */}
                    <div>
                      <label className="block text-xs text-gray-400 mb-2">Ramka</label>
                      <div className="flex gap-3">
                        {["none", "gold"].map((frame) => (
                          <button
                            key={frame}
                            onClick={() => setSelectedFrame(frame)}
                            className={`px-4 py-2 rounded-lg border transition-all ${
                              selectedFrame === frame
                                ? 'bg-yellow-500/20 border-yellow-500/50 text-yellow-400'
                                : 'bg-brand-dark/50 border-brand-border/50 text-gray-400 hover:border-yellow-500/30'
                            }`}
                          >
                            {frame === "none" ? "Yo'q" : "🔶 Oltin"}
                          </button>
                        ))}
                      </div>
                    </div>
                    
                    {/* Theme selection */}
                    <div>
                      <label className="block text-xs text-gray-400 mb-2">Tema</label>
                      <div className="flex gap-3">
                        {[
                          { value: "default", label: "Odatiy" },
                          { value: "gold_dark", label: "🌟 Oltin qorong'i" },
                        ].map((theme) => (
                          <button
                            key={theme.value}
                            onClick={() => setSelectedTheme(theme.value)}
                            className={`px-4 py-2 rounded-lg border transition-all ${
                              selectedTheme === theme.value
                                ? 'bg-yellow-500/20 border-yellow-500/50 text-yellow-400'
                                : 'bg-brand-dark/50 border-brand-border/50 text-gray-400 hover:border-yellow-500/30'
                            }`}
                          >
                            {theme.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    
                    {/* Gradient selection */}
                    <div>
                      <label className="block text-xs text-gray-400 mb-2">Gradient</label>
                      <div className="flex gap-3">
                        {[
                          { value: "none", label: "Yo'q" },
                          { value: "gold", label: "🌟 Oltin" },
                          { value: "purple", label: "💜 Binafsha" },
                        ].map((grad) => (
                          <button
                            key={grad.value}
                            onClick={() => setSelectedGradient(grad.value)}
                            className={`px-4 py-2 rounded-lg border transition-all ${
                              selectedGradient === grad.value
                                ? grad.value === "purple"
                                  ? 'bg-purple-500/20 border-purple-500/50 text-purple-400'
                                  : 'bg-yellow-500/20 border-yellow-500/50 text-yellow-400'
                                : 'bg-brand-dark/50 border-brand-border/50 text-gray-400 hover:border-yellow-500/30'
                            }`}
                          >
                            {grad.label}
                          </button>
                        ))}
                      </div>
                    </div>
                    
                    <button
                      onClick={handleSaveProfileStyle}
                      disabled={isSavingStyle}
                      className="w-full bg-gradient-to-r from-yellow-500 to-amber-600 hover:from-yellow-400 hover:to-amber-500 text-black font-semibold px-4 py-2.5 rounded-lg transition-all shadow-[0_0_15px_rgba(234,179,8,0.3)] disabled:opacity-50 flex items-center justify-center gap-2"
                    >
                      {isSavingStyle ? (
                        <>
                          <div className="w-4 h-4 border-2 border-black border-t-transparent rounded-full animate-spin" />
                          Saqlanmoqda...
                        </>
                      ) : (
                        <>
                          <Check size={16} />
                          Saqlash
                        </>
                      )}
                    </button>
                  </div>
                  
                  {/* Privacy Section */}
                  <div className="pt-4 border-t border-brand-border/30">
                    <h4 className="text-sm font-medium text-gray-300 flex items-center gap-2 mb-4">
                      <Lock size={14} className="text-yellow-500" />
                      Maxfiylik
                    </h4>
                    
                    <div className="flex items-center justify-between">
                      <div className="flex items-center gap-3">
                        {isPrivate ? (
                          <EyeOff size={20} className="text-yellow-500" />
                        ) : (
                          <Eye size={20} className="text-gray-400" />
                        )}
                        <div>
                          <p className="text-white font-medium">Profilni yashirish</p>
                          <p className="text-gray-400 text-sm">Boshqa foydalanuvchilar faqat asosiy ma&apos;lumotlarni ko&apos;radi</p>
                        </div>
                      </div>
                      <button
                        onClick={async () => {
                          const newValue = !isPrivate;
                          setIsPrivate(newValue);
                          await handleSavePrivacy(newValue);
                        }}
                        disabled={isSavingPrivacy}
                        className={`relative w-12 h-6 rounded-full transition-colors ${
                          isPrivate ? 'bg-yellow-500' : 'bg-gray-600'
                        }`}
                      >
                        <div
                          className={`absolute top-1 w-4 h-4 bg-white rounded-full transition-transform ${
                            isPrivate ? 'translate-x-7' : 'translate-x-1'
                          }`}
                        />
                      </button>
                    </div>
                  </div>
                </div>
              )}
            </div>
          )}

          {/* Loading state */}
          {loading && (
            <div className="flex justify-center py-16">
              <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
            </div>
          )}

          {!loading && (
            <>
              {/* Recent watched section - Carousel */}
              {recentWatched.length > 0 && (
                <section className="mb-10">
                  <div className="flex items-center gap-3 mb-5">
                    <div className={`w-9 h-9 rounded-lg flex items-center justify-center ${
                      displayIsPremium 
                        ? 'bg-yellow-500/10 border border-yellow-500/20' 
                        : 'bg-blue-500/10 border border-blue-500/20'
                    }`}>
                      <Clock className={`w-4 h-4 ${displayIsPremium ? 'text-yellow-500' : 'text-blue-500'}`} />
                    </div>
                    <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">
                      SO&apos;NGGI KO&apos;RILGANLAR
                    </h2>
                    <span className="px-2 py-0.5 bg-brand-border text-gray-400 text-xs rounded-full">
                      {recentWatched.length}
                    </span>
                  </div>
                  
                  <MovieCarousel 
                    movies={recentWatched.map((item) => item.movie || item)} 
                  />
                </section>
              )}

              {/* Favorites section - Carousel */}
              <section className="mb-10">
                <div className="flex items-center gap-3 mb-5">
                  <div className={`w-9 h-9 rounded-lg flex items-center justify-center ${
                    displayIsPremium 
                      ? 'bg-yellow-500/10 border border-yellow-500/20' 
                      : 'bg-brand-red/10 border border-brand-red/20'
                  }`}>
                    <Heart className={`w-4 h-4 ${displayIsPremium ? 'text-yellow-500' : 'text-brand-red'}`} />
                  </div>
                  <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">
                    SEVIMLILAR
                  </h2>
                  {favorites.length > 0 && (
                    <span className="px-2 py-0.5 bg-brand-border text-gray-400 text-xs rounded-full">
                      {favorites.length}
                    </span>
                  )}
                </div>
                
                {favorites.length > 0 ? (
                  <MovieCarousel movies={favorites} />
                ) : (
                  <div className="py-10 text-center bg-brand-card/50 rounded-xl border border-brand-border/50">
                    <Heart className="w-8 h-8 text-gray-600 mx-auto mb-2" />
                    <p className="text-gray-500 text-sm">{t("user.noFavorites")}</p>
                    <p className="text-gray-600 text-xs mt-1">
                      Kinolar sahifasida belgisini bosing
                    </p>
                  </div>
                )}
              </section>

              {/* Watch History section - Carousel */}
              <section className="mb-10">
                <div className="flex items-center gap-3 mb-5">
                  <div className="w-9 h-9 rounded-lg bg-blue-500/10 border border-blue-500/20 flex items-center justify-center">
                    <History className="w-4 h-4 text-blue-500" />
                  </div>
                  <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">
                    KO&apos;RISH TARIXI
                  </h2>
                  {watchHistory.length > 0 && (
                    <span className="px-2 py-0.5 bg-brand-border text-gray-400 text-xs rounded-full">
                      {watchHistory.length}
                    </span>
                  )}
                </div>
                
                {watchHistory.length > 0 ? (
                  <MovieCarousel 
                    movies={watchHistory.map((item) => item.movie || item)} 
                  />
                ) : (
                  <div className="py-10 text-center bg-brand-card/50 rounded-xl border border-brand-border/50">
                    <History className="w-8 h-8 text-gray-600 mx-auto mb-2" />
                    <p className="text-gray-500 text-sm">{t("user.noWatchHistory")}</p>
                    <p className="text-gray-600 text-xs mt-1">
                      Haligacha hech qanday kino ko&apos;rmagansiz
                    </p>
                  </div>
                )}
              </section>
            </>
          )}

          {/* Bottom padding */}
          <div className="pb-12" />

          {/* Profile page banner ad */}
          <div className="max-w-5xl mx-auto px-4 pb-8">
            <WebsiteAdSlot placement="profile_page_banner" />
          </div>
        </div>
      </main>
      <Footer />
    </>
  );
}
