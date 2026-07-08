"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { Shield, Clock, User, AlertTriangle, LogOut, Lock, Timer, Send, CheckCircle, XCircle, MessageSquare, ChevronDown, ChevronUp } from "lucide-react";
import { createAppeal, getMyAppeals, BanAppeal } from "@/lib/api";

interface BanInfo {
  is_banned: boolean;
  reason?: string;
  banned_at?: string;
  banned_until?: string | null;
  banned_by_username?: string;
}

export default function BannedPage() {
  const { user, isLoading, isBanned, banInfo, token, logout } = useAuth();
  const router = useRouter();
  const [loading, setLoading] = useState(true);
  const [remainingTime, setRemainingTime] = useState<string | null>(null);
  const [showAppealForm, setShowAppealForm] = useState(false);
  const [appealMessage, setAppealMessage] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [submitError, setSubmitError] = useState<string | null>(null);
  const [submitSuccess, setSubmitSuccess] = useState(false);
  const [myAppeals, setMyAppeals] = useState<BanAppeal[]>([]);
  const [loadingAppeals, setLoadingAppeals] = useState(false);
  const [hasPendingAppeal, setHasPendingAppeal] = useState(false);
  const [appealExpanded, setAppealExpanded] = useState(false);

  useEffect(() => {
    if (isLoading) return;

    // If no token, redirect to home
    if (!token) {
      router.push("/");
      return;
    }

    // If user is not banned, redirect to home
    if (!isBanned) {
      router.push("/");
      return;
    }

    setLoading(false);
  }, [isLoading, token, isBanned, router]);

  // Load user's appeals
  useEffect(() => {
    if (!token || loading) return;

    const loadAppeals = async () => {
      setLoadingAppeals(true);
      try {
        const data = await getMyAppeals(token);
        setMyAppeals(data.appeals);
        setHasPendingAppeal(data.appeals.some(a => a.status === "pending"));
      } catch (error) {
        console.error("Failed to load appeals:", error);
      } finally {
        setLoadingAppeals(false);
      }
    };

    loadAppeals();
  }, [token, loading]);

  // Calculate remaining time for temporary bans
  useEffect(() => {
    if (!banInfo?.banned_until || banInfo?.is_banned === false) {
      setRemainingTime(null);
      return;
    }

    const updateRemainingTime = () => {
      const now = new Date().getTime();
      const until = new Date(banInfo.banned_until!).getTime();
      const diff = until - now;

      if (diff <= 0) {
        setRemainingTime(null);
        return;
      }

      const days = Math.floor(diff / (1000 * 60 * 60 * 24));
      const hours = Math.floor((diff % (1000 * 60 * 60 * 24)) / (1000 * 60 * 60));
      const minutes = Math.floor((diff % (1000 * 60 * 60)) / (1000 * 60));

      if (days > 0) {
        setRemainingTime(`${days} kun ${hours} soat`);
      } else if (hours > 0) {
        setRemainingTime(`${hours} soat ${minutes} daqiqa`);
      } else {
        setRemainingTime(`${minutes} daqiqa`);
      }
    };

    updateRemainingTime();
    const interval = setInterval(updateRemainingTime, 60000); // Update every minute

    return () => clearInterval(interval);
  }, [banInfo]);

  const formatDate = (dateString: string) => {
    if (!dateString) return "—";
    const date = new Date(dateString);
    return date.toLocaleDateString("uz-UZ", {
      year: "numeric",
      month: "long",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const isPermanentBan = !banInfo?.banned_until;

  const handleSubmitAppeal = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || appealMessage.trim().length < 10) return;

    setSubmitting(true);
    setSubmitError(null);

    try {
      await createAppeal(token, { message: appealMessage.trim() });
      setSubmitSuccess(true);
      setAppealMessage("");
      setShowAppealForm(false);
      
      // Reload appeals
      const data = await getMyAppeals(token);
      setMyAppeals(data.appeals);
      setHasPendingAppeal(data.appeals.some(a => a.status === "pending"));
      
      // Clear success message after 3 seconds
      setTimeout(() => setSubmitSuccess(false), 3000);
    } catch (error: any) {
      setSubmitError(error.message || "Apellyatsiya yuborishda xatolik");
    } finally {
      setSubmitting(false);
    }
  };

  const handleLogout = async () => {
    await logout();
    router.push("/");
  };

  if (isLoading || loading) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <div className="animate-spin w-10 h-10 border-3 border-brand-red border-t-transparent rounded-full" />
      </div>
    );
  }

  if (!isBanned || !banInfo) {
    return null;
  }

  return (
    <div className="min-h-screen bg-brand-dark flex items-center justify-center p-4 relative overflow-hidden">
      {/* Background Effects */}
      <div className="absolute inset-0 bg-gradient-to-br from-red-900/20 via-brand-dark to-brand-dark" />
      <div className="absolute top-1/4 left-1/4 w-96 h-96 bg-red-500/10 rounded-full blur-3xl" />
      <div className="absolute bottom-1/4 right-1/4 w-64 h-64 bg-orange-500/10 rounded-full blur-3xl" />
      
      <div className="relative z-10 max-w-lg w-full">
        <div className="bg-gradient-to-b from-brand-card to-brand-dark border border-red-500/30 rounded-3xl p-8 shadow-2xl shadow-red-500/10">
          {/* Lock Icon */}
          <div className="flex justify-center mb-6">
            <div className="relative">
              <div className="absolute inset-0 bg-red-500/30 rounded-full blur-xl" />
              <div className="relative w-20 h-20 rounded-full bg-gradient-to-br from-red-500/20 to-orange-500/20 border border-red-500/30 flex items-center justify-center">
                <Lock className="w-10 h-10 text-red-400" />
              </div>
            </div>
          </div>

          {/* Title */}
          <div className="text-center mb-8">
            <h1 className="font-display text-4xl sm:text-5xl text-white mb-3 tracking-wide">
              SIZ BAN OLGANSIZ
            </h1>
            <div className="h-1 w-20 bg-gradient-to-r from-red-500 to-orange-500 mx-auto rounded-full" />
          </div>

          {/* Status Message */}
          <p className="text-center text-gray-400 mb-8 text-lg">
            {isPermanentBan
              ? "Hisobingiz doimiy ravishda bloklangan."
              : "Hisobingiz vaqtincha bloklangan."}
          </p>

          {/* Remaining Time (for temporary bans) */}
          {remainingTime && !isPermanentBan && (
            <div className="mb-6 p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-2xl">
              <div className="flex items-center gap-3">
                <Timer className="w-6 h-6 text-yellow-400" />
                <div>
                  <p className="text-yellow-400 text-sm font-medium">Qolgan vaqt</p>
                  <p className="text-white font-bold text-lg">{remainingTime}</p>
                </div>
              </div>
            </div>
          )}

          {/* Ban Details */}
          <div className="space-y-3">
            {/* Reason */}
            <div className="flex items-start gap-3 p-4 bg-brand-dark/50 rounded-xl border border-white/5">
              <div className="w-10 h-10 rounded-lg bg-red-500/20 flex items-center justify-center flex-shrink-0">
                <Shield className="w-5 h-5 text-red-400" />
              </div>
              <div>
                <p className="text-gray-500 text-xs uppercase tracking-wide mb-1">
                  Sabab
                </p>
                <p className="text-white font-medium text-lg">
                  {banInfo.reason || "Sabab ko'rsatilmagan"}
                </p>
              </div>
            </div>

            {/* Banned By */}
            <div className="flex items-start gap-3 p-4 bg-brand-dark/50 rounded-xl border border-white/5">
              <div className="w-10 h-10 rounded-lg bg-orange-500/20 flex items-center justify-center flex-shrink-0">
                <User className="w-5 h-5 text-orange-400" />
              </div>
              <div>
                <p className="text-gray-500 text-xs uppercase tracking-wide mb-1">
                  Admin
                </p>
                <p className="text-white font-medium text-lg">
                  {banInfo.banned_by_username || "Noma'lum"}
                </p>
              </div>
            </div>

            {/* Banned At */}
            <div className="flex items-start gap-3 p-4 bg-brand-dark/50 rounded-xl border border-white/5">
              <div className="w-10 h-10 rounded-lg bg-blue-500/20 flex items-center justify-center flex-shrink-0">
                <Clock className="w-5 h-5 text-blue-400" />
              </div>
              <div>
                <p className="text-gray-500 text-xs uppercase tracking-wide mb-1">
                  Ban boshlangan
                </p>
                <p className="text-white font-medium">
                  {banInfo.banned_at ? formatDate(banInfo.banned_at) : "—"}
                </p>
              </div>
            </div>

            {/* Expiry */}
            <div className="flex items-start gap-3 p-4 bg-brand-dark/50 rounded-xl border border-white/5">
              <div className={`w-10 h-10 rounded-lg flex items-center justify-center flex-shrink-0 ${
                isPermanentBan ? "bg-red-500/20" : "bg-yellow-500/20"
              }`}>
                <AlertTriangle className={`w-5 h-5 ${isPermanentBan ? "text-red-400" : "text-yellow-400"}`} />
              </div>
              <div>
                <p className="text-gray-500 text-xs uppercase tracking-wide mb-1">
                  {isPermanentBan ? "Holati" : "Tugash vaqti"}
                </p>
                <p className={`font-medium ${isPermanentBan ? "text-red-400 text-lg" : "text-white"}`}>
                  {isPermanentBan ? (
                    "Doimiy bloklangan"
                  ) : banInfo.banned_until ? (
                    formatDate(banInfo.banned_until)
                  ) : (
                    "—"
                  )}
                </p>
              </div>
            </div>
          </div>

          {/* Appeal Section */}
          <div className="mt-8">
            {/* Submit Success Message */}
            {submitSuccess && (
              <div className="mb-4 p-4 bg-green-500/20 border border-green-500/30 rounded-xl">
                <div className="flex items-center gap-3">
                  <CheckCircle className="w-5 h-5 text-green-400" />
                  <p className="text-green-400 font-medium">Apellyatsiya muvaffaqiyatli yuborildi!</p>
                </div>
              </div>
            )}

            {/* My Appeals */}
            {myAppeals.length > 0 && (
              <div className="mb-4">
                <button
                  onClick={() => setAppealExpanded(!appealExpanded)}
                  className="w-full flex items-center justify-between p-3 bg-brand-dark/50 rounded-xl border border-white/5 hover:border-brand-red/30 transition-colors"
                >
                  <div className="flex items-center gap-2">
                    <MessageSquare className="w-5 h-5 text-brand-red" />
                    <span className="text-white font-medium"> mening apellyatsiyalarim ({myAppeals.length})</span>
                  </div>
                  {appealExpanded ? (
                    <ChevronUp className="w-5 h-5 text-gray-400" />
                  ) : (
                    <ChevronDown className="w-5 h-5 text-gray-400" />
                  )}
                </button>
                
                {appealExpanded && (
                  <div className="mt-2 space-y-2">
                    {myAppeals.map((appeal) => (
                      <div
                        key={appeal.id}
                        className="p-3 bg-brand-dark/30 rounded-lg border border-white/5"
                      >
                        <div className="flex items-center justify-between mb-2">
                          <span className={`text-sm font-medium px-2 py-1 rounded ${
                            appeal.status === "pending" ? "bg-yellow-500/20 text-yellow-400" :
                            appeal.status === "approved" ? "bg-green-500/20 text-green-400" :
                            "bg-red-500/20 text-red-400"
                          }`}>
                            {appeal.status === "pending" ? "Ko'rib chiqilmoqda" :
                             appeal.status === "approved" ? "Qabul qilindi" : "Rad etildi"}
                          </span>
                          <span className="text-xs text-gray-500">
                            {formatDate(appeal.created_at)}
                          </span>
                        </div>
                        <p className="text-gray-300 text-sm mb-2">{appeal.message}</p>
                        {appeal.admin_note && (
                          <div className="mt-2 p-2 glass-card rounded border border-white/5">
                            <p className="text-xs text-gray-500 mb-1">Admin izohi:</p>
                            <p className="text-sm text-gray-400">{appeal.admin_note}</p>
                          </div>
                        )}
                        {appeal.reviewed_at && (
                          <p className="text-xs text-gray-500 mt-2">
                            Ko'rib chiqildi: {formatDate(appeal.reviewed_at)}
                            {appeal.reviewed_by_username && ` (${appeal.reviewed_by_username})`}
                          </p>
                        )}
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Pending Appeal Notice */}
            {hasPendingAppeal && !showAppealForm && (
              <div className="p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-xl">
                <div className="flex items-center gap-3">
                  <Clock className="w-5 h-5 text-yellow-400" />
                  <p className="text-yellow-400 font-medium">
                    Sizning apellyatsiyangiz ko'rib chiqilmoqda
                  </p>
                </div>
              </div>
            )}

            {/* Submit New Appeal Button */}
            {!showAppealForm && !hasPendingAppeal && (
              <button
                onClick={() => setShowAppealForm(true)}
                className="w-full flex items-center justify-center gap-2 py-3 px-4 bg-brand-red hover:bg-red-600 text-white rounded-xl transition-colors font-medium"
              >
                <Send className="w-5 h-5" />
                <span>Apellyatsiya yuborish</span>
              </button>
            )}

            {/* Appeal Form */}
            {showAppealForm && !hasPendingAppeal && (
              <form onSubmit={handleSubmitAppeal} className="space-y-4">
                <div>
                  <label className="block text-sm text-gray-400 mb-2">
                    Apellyatsiya xabari
                  </label>
                  <textarea
                    value={appealMessage}
                    onChange={(e) => setAppealMessage(e.target.value)}
                    placeholder="Nega ban bekor qilinishini tushuntiring... (kamida 10 belgi)"
                    className="w-full p-3 bg-brand-dark border border-white/10 rounded-xl text-white placeholder-gray-500 focus:outline-none focus:border-brand-red resize-none"
                    rows={4}
                    minLength={10}
                    maxLength={2000}
                  />
                  <p className="text-xs text-gray-500 mt-1">
                    {appealMessage.length}/2000 belgi
                  </p>
                </div>

                {submitError && (
                  <div className="p-3 bg-red-500/20 border border-red-500/30 rounded-lg">
                    <div className="flex items-center gap-2">
                      <XCircle className="w-4 h-4 text-red-400" />
                      <p className="text-red-400 text-sm">{submitError}</p>
                    </div>
                  </div>
                )}

                <div className="flex gap-3">
                  <button
                    type="submit"
                    disabled={submitting || appealMessage.trim().length < 10}
                    className="flex-1 flex items-center justify-center gap-2 py-3 px-4 bg-brand-red hover:bg-red-600 disabled:bg-gray-600 disabled:cursor-not-allowed text-white rounded-xl transition-colors font-medium"
                  >
                    {submitting ? (
                      <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
                    ) : (
                      <>
                        <Send className="w-5 h-5" />
                        <span>Yuborish</span>
                      </>
                    )}
                  </button>
                  <button
                    type="button"
                    onClick={() => {
                      setShowAppealForm(false);
                      setAppealMessage("");
                      setSubmitError(null);
                    }}
                    className="py-3 px-4 bg-brand-dark hover:bg-brand-border/50 text-gray-400 rounded-xl transition-colors font-medium"
                  >
                    Bekor qilish
                  </button>
                </div>
              </form>
            )}
          </div>

          {/* Contact Info */}
          <div className="mt-6 p-4 bg-yellow-500/10 border border-yellow-500/30 rounded-2xl">
            <p className="text-yellow-400 text-sm text-center">
              Agar bu xato deb hisoblasangiz, admin bilan bog'laning.
            </p>
          </div>

          {/* Logout Button */}
          <button
            onClick={handleLogout}
            className="mt-6 w-full flex items-center justify-center gap-2 py-3 px-4 bg-brand-dark hover:bg-brand-border/50 text-gray-400 hover:text-white rounded-xl transition-colors border border-white/10 font-medium"
          >
            <LogOut size={18} />
            <span>Chiqish</span>
          </button>
        </div>
      </div>
    </div>
  );
}
