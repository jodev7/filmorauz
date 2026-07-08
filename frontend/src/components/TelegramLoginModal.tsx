"use client";

import { useState, useEffect, useRef } from "react";
import { useAuth } from "@/lib/auth-context";
import { Send, Loader2, CheckCircle, XCircle, X } from "lucide-react";

interface TelegramLoginModalProps {
  isOpen: boolean;
  onClose: () => void;
}

export default function TelegramLoginModal({ isOpen, onClose }: TelegramLoginModalProps) {
  const { startTelegramAuth, checkAuthStatus } = useAuth();
  const [authCode, setAuthCode] = useState<string | null>(null);
  const [botUrl, setBotUrl] = useState<string>("");
  const [status, setStatus] = useState<"idle" | "loading" | "polling" | "success" | "expired">("idle");
  const [error, setError] = useState<string>("");
  const pollingRef = useRef<NodeJS.Timeout | null>(null);
  const startTimeRef = useRef<number>(0);

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      if (pollingRef.current) {
        clearInterval(pollingRef.current);
      }
    };
  }, []);

  // Handle modal open - start login flow
  useEffect(() => {
    if (isOpen && status === "idle") {
      startLogin();
    }
  }, [isOpen, status]);

  const startLogin = async () => {
    setStatus("loading");
    setError("");

    try {
      const response = await startTelegramAuth();
      if (response) {
        setAuthCode(response.code);
        setBotUrl(response.bot_url);
        startPolling(response.code);
      } else {
        setError("Login boshlashda xatolik yuz berdi");
        setStatus("idle");
      }
    } catch (err) {
      setError("Server xatoligi");
      setStatus("idle");
    }
  };

  const startPolling = (code: string) => {
    setStatus("polling");
    startTimeRef.current = Date.now();
    const maxDuration = 5 * 60 * 1000; // 5 minutes max

    pollingRef.current = setInterval(async () => {
      // Check timeout
      if (Date.now() - startTimeRef.current > maxDuration) {
        if (pollingRef.current) {
          clearInterval(pollingRef.current);
        }
        setStatus("expired");
        return;
      }

      try {
        const success = await checkAuthStatus(code);
        if (success) {
          if (pollingRef.current) {
            clearInterval(pollingRef.current);
          }
          setStatus("success");
          // Full reload so server-rendered pages (movie/series detail,
          // profile, etc.) re-fetch with the auth cookie set. Without
          // this, only client-rendered widgets pick up the new session.
          setTimeout(() => {
            window.location.reload();
          }, 1500);
        }
        // If not success, continue polling
      } catch (err) {
        console.error("Polling error:", err);
      }
    }, 2000); // Poll every 2 seconds
  };

  const handleOpenTelegram = () => {
    if (botUrl) {
      window.open(botUrl, "_blank");
    }
  };

  const handleClose = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }
    setStatus("idle");
    setAuthCode(null);
    setBotUrl("");
    setError("");
    onClose();
  };

  const handleRetry = () => {
    if (pollingRef.current) {
      clearInterval(pollingRef.current);
    }
    setStatus("idle");
    setError("");
    startLogin();
  };

  if (!isOpen) return null;

  return (
    <div className="fixed inset-0 z-[100] flex items-center justify-center">
      {/* Backdrop */}
      <div
        className="absolute inset-0 bg-black/70 backdrop-blur-sm"
        onClick={handleClose}
      />

      {/* Modal */}
      <div className="relative glass-strong rounded-2xl p-6 w-full max-w-md mx-4 shadow-2xl">
        {/* Close button */}
        <button
          onClick={handleClose}
          className="absolute top-4 right-4 text-gray-500 hover:text-white transition-colors"
        >
          <X size={20} />
        </button>

        {/* Header */}
        <div className="text-center mb-6">
          <div className="inline-flex items-center justify-center w-16 h-16 rounded-full bg-brand-red/20 mb-4">
            <Send className="text-brand-red" size={32} />
          </div>
          <h2 className="text-xl font-bold text-white">Telegram orqali kirish</h2>
        </div>

        {/* Content based on status */}
        {status === "loading" && (
          <div className="text-center py-8">
            <Loader2 className="animate-spin text-brand-red mx-auto mb-4" size={32} />
            <p className="text-gray-400">Tayyorlanmoqda...</p>
          </div>
        )}

        {status === "polling" && (
          <div className="text-center py-4">
            <Loader2 className="animate-spin text-brand-red mx-auto mb-4" size={32} />
            <p className="text-gray-300 mb-2">
              Telegram botda tasdiqlang
            </p>
            <p className="text-gray-500 text-sm mb-4">
              Quyidagi tugmasini bosing va "/start" yuboring
            </p>

            {/* Telegram button */}
            <button
              onClick={handleOpenTelegram}
              className="inline-flex items-center gap-2 bg-[#0088cc] hover:bg-[#0077b3] text-white px-6 py-3 rounded-lg font-medium transition-colors"
            >
              <Send size={18} />
              Telegramga o'tish
            </button>

            <p className="text-gray-600 text-xs mt-4">
              Kutilmoqda... ({Math.floor((Date.now() - startTimeRef.current) / 1000)}s)
            </p>
          </div>
        )}

        {status === "success" && (
          <div className="text-center py-8">
            <CheckCircle className="text-green-500 mx-auto mb-4" size={48} />
            <p className="text-green-400 font-medium text-lg">
              Tizimga muvaffaqiyatli kirdingiz!
            </p>
            <p className="text-gray-500 text-sm mt-2 flex items-center justify-center gap-2">
              <Loader2 className="animate-spin" size={14} />
              Sahifa yangilanmoqda...
            </p>
          </div>
        )}

        {status === "expired" && (
          <div className="text-center py-8">
            <XCircle className="text-red-500 mx-auto mb-4" size={48} />
            <p className="text-red-400 mb-4">
              Login havolasi muddati tugadi
            </p>
            <button
              onClick={handleRetry}
              className="bg-brand-red hover:bg-brand-red/80 text-white px-6 py-2 rounded-lg font-medium transition-colors"
            >
              Qayta urinish
            </button>
          </div>
        )}

        {error && status === "idle" && (
          <div className="text-center py-8">
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
  );
}
