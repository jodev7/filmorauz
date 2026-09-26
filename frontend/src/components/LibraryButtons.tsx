"use client";

import { useEffect, useState } from "react";
import { Bookmark, BookmarkCheck, Bell, BellRing, Loader2 } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  addToWatchlist,
  removeFromWatchlist,
  getLibraryStatus,
  subscribeSeries,
  unsubscribeSeries,
  LibraryTargetType,
} from "@/lib/api";
import TelegramLoginModal from "@/components/TelegramLoginModal";
import AddToListButton from "@/components/AddToListButton";

const baseBtn =
  "inline-flex items-center gap-1.5 px-4 py-2.5 rounded-xl text-sm transition-colors disabled:opacity-60";
const offBtn = "glass-card border border-white/10 text-gray-300 hover:border-brand-red hover:text-white";
const onBtn = "bg-white text-black border border-white";

/**
 * "Keyinroq ko'raman" toggle for movies and series, plus — for series — a
 * "Yangi qismlar haqida xabar berish" subscription toggle. Logged-out users
 * get the Telegram login modal instead of a dead button.
 */
export default function LibraryButtons({
  targetType,
  targetId,
  title,
}: {
  targetType: LibraryTargetType;
  targetId: string;
  title?: string;
}) {
  const { isAuthenticated, token } = useAuth();
  const [inList, setInList] = useState(false);
  const [subscribed, setSubscribed] = useState(false);
  const [loaded, setLoaded] = useState(false);
  const [busy, setBusy] = useState<"list" | "sub" | null>(null);
  const [loginOpen, setLoginOpen] = useState(false);
  const [hint, setHint] = useState<string | null>(null);

  useEffect(() => {
    if (!isAuthenticated || !token) {
      setLoaded(true);
      return;
    }
    let cancelled = false;
    getLibraryStatus(token, targetType, targetId)
      .then((s) => {
        if (cancelled) return;
        setInList(s.in_watchlist);
        setSubscribed(!!s.subscribed);
      })
      .catch(() => {})
      .finally(() => {
        if (!cancelled) setLoaded(true);
      });
    return () => {
      cancelled = true;
    };
  }, [isAuthenticated, token, targetType, targetId]);

  const flash = (text: string) => {
    setHint(text);
    setTimeout(() => setHint(null), 2500);
  };

  const toggleList = async () => {
    if (!isAuthenticated || !token) return setLoginOpen(true);
    setBusy("list");
    const next = !inList;
    setInList(next); // optimistic
    try {
      if (next) await addToWatchlist(token, targetType, targetId);
      else await removeFromWatchlist(token, targetType, targetId);
      flash(next ? "Keyinroq ko'raman ro'yxatiga qo'shildi" : "Ro'yxatdan olib tashlandi");
    } catch {
      setInList(!next);
      flash("Xatolik, qayta urinib ko'ring");
    } finally {
      setBusy(null);
    }
  };

  const toggleSub = async () => {
    if (!isAuthenticated || !token) return setLoginOpen(true);
    setBusy("sub");
    const next = !subscribed;
    setSubscribed(next);
    try {
      if (next) await subscribeSeries(token, targetId);
      else await unsubscribeSeries(token, targetId);
      flash(next ? "Yangi qism chiqsa saytda va Telegram'da xabar beramiz" : "Obuna bekor qilindi");
    } catch {
      setSubscribed(!next);
      flash("Xatolik, qayta urinib ko'ring");
    } finally {
      setBusy(null);
    }
  };

  return (
    <div className="relative flex flex-wrap items-center gap-2">
      <button
        onClick={toggleList}
        disabled={!loaded || busy === "list"}
        aria-pressed={inList}
        title={title ? `${title} — keyinroq ko'rish` : undefined}
        className={`${baseBtn} ${inList ? onBtn : offBtn}`}
      >
        {busy === "list" ? (
          <Loader2 size={16} className="animate-spin" />
        ) : inList ? (
          <BookmarkCheck size={16} />
        ) : (
          <Bookmark size={16} />
        )}
        {inList ? "Ro'yxatda" : "Keyinroq ko'raman"}
      </button>

      <AddToListButton
        targetType={targetType}
        targetId={targetId}
        className={`${baseBtn} ${offBtn}`}
        onNeedLogin={() => setLoginOpen(true)}
        onFlash={flash}
      />

      {targetType === "series" && (
        <button
          onClick={toggleSub}
          disabled={!loaded || busy === "sub"}
          aria-pressed={subscribed}
          className={`${baseBtn} ${subscribed ? onBtn : offBtn}`}
        >
          {busy === "sub" ? (
            <Loader2 size={16} className="animate-spin" />
          ) : subscribed ? (
            <BellRing size={16} />
          ) : (
            <Bell size={16} />
          )}
          {subscribed ? "Obuna bo'lgansiz" : "Yangi qismlarga obuna"}
        </button>
      )}

      {hint && (
        <span role="status" className="w-full text-xs text-gray-400">
          {hint}
        </span>
      )}

      <TelegramLoginModal isOpen={loginOpen} onClose={() => setLoginOpen(false)} />
    </div>
  );
}
