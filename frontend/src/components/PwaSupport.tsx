"use client";

import { useEffect, useState } from "react";
import { Download, X, Share } from "lucide-react";

type InstallPromptEvent = Event & { prompt: () => Promise<void>; userChoice: Promise<{ outcome: "accepted" | "dismissed" }> };

const DISMISS_KEY = "filmora_pwa_dismissed_at";
const DISMISS_DAYS = 14;

function recentlyDismissed(): boolean {
  try {
    const at = Number(localStorage.getItem(DISMISS_KEY));
    return at > 0 && Date.now() - at < DISMISS_DAYS * 24 * 60 * 60 * 1000;
  } catch {
    return false;
  }
}

/**
 * Registers the service worker (production only) and offers a small
 * "Ilovani o'rnatish" banner on phones: the native prompt on Android/Chrome,
 * a short "Ulashish → Bosh ekranga qo'shish" hint on iPhone Safari.
 */
export default function PwaSupport() {
  const [promptEvent, setPromptEvent] = useState<InstallPromptEvent | null>(null);
  const [showIosHint, setShowIosHint] = useState(false);

  useEffect(() => {
    if (process.env.NODE_ENV === "production" && "serviceWorker" in navigator) {
      navigator.serviceWorker.register("/sw.js").catch(() => {});
    }

    const standalone =
      window.matchMedia("(display-mode: standalone)").matches ||
      (navigator as Navigator & { standalone?: boolean }).standalone === true;
    const isMobile = /Android|iPhone|iPad|iPod/i.test(navigator.userAgent);
    if (standalone || !isMobile || recentlyDismissed()) return;
    if (window.location.pathname.startsWith("/admin") || window.location.pathname.startsWith("/watch")) return;

    const onPrompt = (e: Event) => {
      e.preventDefault();
      setPromptEvent(e as InstallPromptEvent);
    };
    window.addEventListener("beforeinstallprompt", onPrompt);

    const isIosSafari = /iPhone|iPad|iPod/i.test(navigator.userAgent) && /Safari/i.test(navigator.userAgent) && !/CriOS|FxiOS|Telegram|Instagram/i.test(navigator.userAgent);
    let t: ReturnType<typeof setTimeout> | undefined;
    if (isIosSafari) t = setTimeout(() => setShowIosHint(true), 20000); // after some engagement

    return () => {
      window.removeEventListener("beforeinstallprompt", onPrompt);
      if (t) clearTimeout(t);
    };
  }, []);

  const dismiss = () => {
    try {
      localStorage.setItem(DISMISS_KEY, String(Date.now()));
    } catch {
      /* ignore */
    }
    setPromptEvent(null);
    setShowIosHint(false);
  };

  const install = async () => {
    if (!promptEvent) return;
    await promptEvent.prompt();
    await promptEvent.userChoice.catch(() => null);
    dismiss();
  };

  if (!promptEvent && !showIosHint) return null;

  return (
    <div className="fixed inset-x-3 bottom-20 z-[60] mx-auto flex max-w-md items-center gap-3 rounded-2xl border border-white/10 bg-brand-card/95 p-3 shadow-2xl backdrop-blur sm:bottom-6" role="dialog" aria-label="Ilovani o'rnatish">
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img src="/icon-192.png" alt="" className="h-11 w-11 rounded-xl" />
      <div className="min-w-0 flex-1">
        <p className="text-sm font-semibold text-white">FilmoraUz ilovasini o&apos;rnating</p>
        {promptEvent ? (
          <p className="text-xs text-gray-400">Bosh ekrandan bir bosishda oching</p>
        ) : (
          <p className="text-xs text-gray-400">
            <Share size={11} className="inline -mt-0.5" /> Ulashish → &quot;Bosh ekranga qo&apos;shish&quot;
          </p>
        )}
      </div>
      {promptEvent && (
        <button onClick={install} className="inline-flex items-center gap-1.5 rounded-lg bg-brand-red px-3 py-2 text-sm font-semibold text-white">
          <Download size={15} /> O&apos;rnatish
        </button>
      )}
      <button onClick={dismiss} className="p-1 text-gray-500 hover:text-white" aria-label="Yopish">
        <X size={16} />
      </button>
    </div>
  );
}
