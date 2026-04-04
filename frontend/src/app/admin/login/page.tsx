"use client";

import { useState, useEffect, useRef } from "react";
import { useRouter } from "next/navigation";
import { Film, Send, Loader2, CheckCircle, XCircle } from "lucide-react";
import { useAuth } from "@/lib/auth-context";

export default function AdminLoginPage() {
  const { isAuthenticated, user, startTelegramAuth, checkAuthStatus, logout } = useAuth();
  const router = useRouter();
  const [status, setStatus] = useState<"loading" | "polling" | "success" | "denied" | "expired" | "error">("loading");
  const [botUrl, setBotUrl] = useState("");
  const [error, setError] = useState("");
  const pollingRef = useRef<NodeJS.Timeout | null>(null);
  const startTimeRef = useRef<number>(0);
  const startedRef = useRef(false);

  // Redirect if already authenticated as admin
  useEffect(() => {
    if (isAuthenticated && user) {
      const isAdmin = user.role === "admin" || user.role === "superadmin";
      if (isAdmin) {
        router.replace("/admin/dashboard");
      } else {
        logout().then(() => setStatus("denied"));
      }
    }
  }, [isAuthenticated, user, router, logout]);

  useEffect(() => {
    if (startedRef.current) return;
    startedRef.current = true;
    startLogin();
    return () => {
      if (pollingRef.current) clearInterval(pollingRef.current);
    };
  }, []);

  const startLogin = async () => {
    setStatus("loading");
    setError("");

    try {
      const response = await startTelegramAuth();
      if (!response) {
        setError("Telegram login boshlanmadi. Qayta urinib ko'ring.");
        setStatus("error");
        return;
      }
      setBotUrl(response.bot_url);
      setStatus("polling");
      startTimeRef.current = Date.now();
      const maxDuration = 5 * 60 * 1000;

      pollingRef.current = setInterval(async () => {
        if (Date.now() - startTimeRef.current > maxDuration) {
          clearInterval(pollingRef.current!);
          setStatus("expired");
          return;
        }
        const success = await checkAuthStatus(response.code);
        if (success) {
          clearInterval(pollingRef.current!);
          // role check handled by the useEffect above once user is set
          setStatus("success");
        }
      }, 2000);
    } catch {
      setError("Server xatoligi yuz berdi.");
      setStatus("error");
    }
  };

  const handleRetry = () => {
    if (pollingRef.current) clearInterval(pollingRef.current);
    startedRef.current = false;
    startLogin();
  };

  return (
    <div className="min-h-screen bg-brand-dark flex items-center justify-center px-4">
      <div className="absolute inset-0 opacity-5 bg-[radial-gradient(circle_at_center,_#F97316_0%,_transparent_70%)]" />

      <div className="relative w-full max-w-sm">
        {/* Logo */}
        <div className="flex items-center justify-center gap-2 mb-10">
          <Film className="text-brand-red" size={28} />
          <span className="font-display text-3xl tracking-wider text-white">
            FILMORA<span className="text-brand-red">UZ</span>
          </span>
        </div>

        <div className="bg-brand-card border border-brand-border rounded-2xl p-8 text-center">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-brand-red/20 mb-4">
            <Send className="text-brand-red" size={32} />
          </div>
          <h1 className="text-xl font-semibold text-white mb-1">Admin Panel</h1>
          <p className="text-sm text-gray-500 mb-7">Telegram orqali kiring</p>

          {status === "loading" && (
            <div className="py-6">
              <Loader2 className="animate-spin text-brand-red mx-auto mb-3" size={32} />
              <p className="text-gray-400 text-sm">Tayyorlanmoqda...</p>
            </div>
          )}

          {status === "polling" && (
            <div className="py-4">
              <Loader2 className="animate-spin text-brand-red mx-auto mb-4" size={32} />
              <p className="text-gray-300 mb-2">Telegram botda tasdiqlang</p>
              <p className="text-gray-500 text-sm mb-5">
                Quyidagi tugmasini bosing va &ldquo;/start&rdquo; yuboring
              </p>
              <button
                onClick={() => botUrl && window.open(botUrl, "_blank")}
                className="inline-flex items-center gap-2 bg-[#0088cc] hover:bg-[#0077b3] text-white px-6 py-3 rounded-lg font-medium transition-colors"
              >
                <Send size={18} />
                Telegramga o&apos;tish
              </button>
            </div>
          )}

          {status === "success" && (
            <div className="py-6">
              <Loader2 className="animate-spin text-brand-red mx-auto mb-3" size={32} />
              <p className="text-gray-400 text-sm">Tekshirilmoqda...</p>
            </div>
          )}

          {status === "denied" && (
            <div className="py-6">
              <XCircle className="text-red-500 mx-auto mb-4" size={48} />
              <p className="text-red-400 font-medium mb-2">Ruxsat yo&apos;q</p>
              <p className="text-gray-500 text-sm mb-5">
                Bu panel faqat admin va superadmin foydalanuvchilar uchun.
              </p>
              <a
                href="/"
                className="inline-block bg-brand-card border border-brand-border hover:border-gray-500 text-gray-300 px-6 py-2 rounded-lg text-sm transition-colors"
              >
                Bosh sahifaga qaytish
              </a>
            </div>
          )}

          {status === "expired" && (
            <div className="py-6">
              <XCircle className="text-red-500 mx-auto mb-4" size={48} />
              <p className="text-red-400 mb-4">Login havolasi muddati tugadi</p>
              <button
                onClick={handleRetry}
                className="bg-brand-red hover:bg-brand-red/80 text-white px-6 py-2 rounded-lg font-medium transition-colors"
              >
                Qayta urinish
              </button>
            </div>
          )}

          {status === "error" && (
            <div className="py-6">
              <XCircle className="text-red-500 mx-auto mb-4" size={32} />
              <p className="text-red-400 mb-4">{error}</p>
              <button
                onClick={handleRetry}
                className="bg-brand-red hover:bg-brand-red/80 text-white px-6 py-2 rounded-lg font-medium transition-colors"
              >
                Qayta urinish
              </button>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}
