"use client";

import { useEffect, useState } from "react";
import { createPortal } from "react-dom";
import { useRouter } from "next/navigation";
import { useAuth } from "@/lib/auth-context";
import { createWatchRoom } from "@/lib/api";
import Link from "next/link";
import { Users, Loader2, X, Globe2, Lock, Crown } from "lucide-react";

type Props = {
  contentType: "movie" | "episode" | "series";
  contentID: string;
  className?: string;
};

// WatchTogetherButton opens a small modal where the host picks visibility
// (public / private) and member cap before the room is created. Free users
// are capped at 2 guests + host; premium at 20. The cap is also enforced
// server-side — the modal is just a friendlier UI than a 403 toast.
export default function WatchTogetherButton({ contentType, contentID, className }: Props) {
  const router = useRouter();
  const { user, token } = useAuth();
  const isPremiumActive = !!(user as { is_premium?: boolean } | null)?.is_premium;
  const [open, setOpen] = useState(false);
  const [busy, setBusy] = useState(false);
  const [visibility, setVisibility] = useState<"private" | "public">("private");
  const planCap = isPremiumActive ? 20 : 2;
  const [maxMembers, setMaxMembers] = useState<number>(planCap);
  const [error, setError] = useState("");
  // When the backend rejects with the daily-quota error, we show a
  // dedicated upsell popup instead of a generic toast — the goal is to
  // route the user to /premium, not to confuse them with an error.
  const [quotaPopup, setQuotaPopup] = useState(false);
  // Portal target — `document.body` is only available after mount, so we
  // gate render-to-portal behind a hydration flag.
  const [mounted, setMounted] = useState(false);
  useEffect(() => setMounted(true), []);

  const openModal = () => {
    if (!user || !token) {
      router.push("/?login=1");
      return;
    }
    setError("");
    setOpen(true);
  };

  const handleCreate = async () => {
    if (!token) return;
    setBusy(true);
    setError("");
    try {
      const room = await createWatchRoom(token, {
        content_type: contentType,
        content_id: contentID,
        visibility,
        max_members: maxMembers,
      });
      router.push(`/watch-room/${room.id}`);
    } catch (e) {
      const msg = e instanceof Error ? e.message : "Room yaratishda xato";
      if (msg.toLowerCase().includes("auth")) {
        router.push("/?login=1");
        return;
      }
      // Backend uses "daily room limit reached" verbatim — match the
      // English phrase from `watch_room_handler.go` so the upsell
      // popup fires for free users who've hit 3 rooms/day.
      const lower = msg.toLowerCase();
      if (lower.includes("daily room limit") || lower.includes("limit reached")) {
        setOpen(false);
        setQuotaPopup(true);
        setBusy(false);
        return;
      }
      setError(msg);
      setBusy(false);
    }
  };

  return (
    <>
      <button
        onClick={openModal}
        disabled={busy}
        className={
          className ||
          "px-4 py-2 bg-brand-card border border-brand-border hover:border-brand-red rounded-lg flex items-center gap-2 text-white transition-colors disabled:opacity-50"
        }
      >
        {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : <Users className="w-4 h-4" />}
        Birga ko&apos;rish
      </button>

      {open && mounted && createPortal(
        <div
          className="fixed inset-0 z-[10000] bg-black/70 flex items-center justify-center p-4"
          onClick={() => !busy && setOpen(false)}
        >
          <div
            className="bg-brand-card border border-brand-border rounded-xl w-full max-w-md overflow-hidden"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-4 py-3 border-b border-brand-border flex items-center justify-between">
              <h3 className="font-semibold">Birga ko&apos;rish room&apos;i</h3>
              <button onClick={() => !busy && setOpen(false)} className="text-gray-400 hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="p-4 space-y-4">
              <div>
                <p className="text-xs text-gray-400 mb-2">Ko&apos;rinish</p>
                <div className="grid grid-cols-2 gap-2">
                  <button
                    onClick={() => setVisibility("private")}
                    className={`p-3 rounded-lg border text-left transition-colors ${
                      visibility === "private"
                        ? "border-brand-red bg-brand-red/10"
                        : "border-brand-border hover:border-gray-500"
                    }`}
                  >
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <Lock className="w-4 h-4" /> Maxfiy
                    </div>
                    <p className="text-[11px] text-gray-400 mt-1">
                      Faqat taklif havolasi orqali
                    </p>
                  </button>
                  <button
                    onClick={() => setVisibility("public")}
                    className={`p-3 rounded-lg border text-left transition-colors ${
                      visibility === "public"
                        ? "border-brand-red bg-brand-red/10"
                        : "border-brand-border hover:border-gray-500"
                    }`}
                  >
                    <div className="flex items-center gap-2 text-sm font-medium">
                      <Globe2 className="w-4 h-4" /> Ochiq
                    </div>
                    <p className="text-[11px] text-gray-400 mt-1">
                      /rooms ro&apos;yxatida ko&apos;rinadi
                    </p>
                  </button>
                </div>
              </div>

              <div>
                <div className="flex items-center justify-between mb-2">
                  <p className="text-xs text-gray-400">A&apos;zolar soni (siz bilan birga)</p>
                  <span className="text-sm font-medium text-brand-red">{maxMembers}</span>
                </div>
                <input
                  type="range"
                  min={2}
                  max={planCap}
                  value={maxMembers}
                  onChange={(e) => setMaxMembers(Number(e.target.value))}
                  className="w-full accent-brand-red"
                />
                <div className="flex justify-between text-[10px] text-gray-500 mt-1">
                  <span>2 (minimum)</span>
                  <span>
                    {planCap} (maximum {isPremiumActive ? "premium" : "free"})
                  </span>
                </div>
                {!isPremiumActive && (
                  <p className="text-[11px] text-yellow-400/80 mt-2">
                    Ko&apos;proq a&apos;zo uchun PREMIUM olishingiz mumkin.
                  </p>
                )}
              </div>

              {error && <p className="text-xs text-red-400">{error}</p>}

              <button
                onClick={handleCreate}
                disabled={busy}
                className="w-full px-4 py-2 bg-brand-red rounded-lg font-medium flex items-center justify-center gap-2 disabled:opacity-50"
              >
                {busy ? <Loader2 className="w-4 h-4 animate-spin" /> : <Users className="w-4 h-4" />}
                Yaratish
              </button>
            </div>
          </div>
        </div>,
        document.body,
      )}

      {/* Daily-quota upsell popup — fires when the backend rejects with
          "daily room limit reached" (free tier only). Steers the user to
          /premium instead of just showing an error toast. */}
      {quotaPopup && mounted && createPortal(
        <div
          className="fixed inset-0 z-[10000] bg-black/75 flex items-center justify-center p-4"
          onClick={() => setQuotaPopup(false)}
        >
          <div
            className="bg-brand-card border border-brand-red rounded-2xl w-full max-w-md overflow-hidden shadow-2xl"
            onClick={(e) => e.stopPropagation()}
          >
            <div className="px-5 py-4 border-b border-brand-border flex items-center justify-between">
              <div className="flex items-center gap-2">
                <Crown className="w-5 h-5 text-yellow-400" />
                <h3 className="font-semibold text-white">Kunlik limit tugadi</h3>
              </div>
              <button onClick={() => setQuotaPopup(false)} className="text-gray-400 hover:text-white">
                <X className="w-5 h-5" />
              </button>
            </div>
            <div className="p-5 space-y-4">
              <p className="text-sm text-gray-300">
                Bugun siz allaqachon <span className="text-white font-semibold">3 ta room</span> ochib
                bo&apos;ldingiz. Bu — bepul rejaning kunlik limiti.
              </p>
              <div className="rounded-lg border border-yellow-500/30 bg-yellow-500/5 p-3 text-xs text-yellow-200">
                <p className="font-medium mb-1">PREMIUM bilan:</p>
                <ul className="space-y-1 text-yellow-100/90">
                  <li>• Cheksiz room ochish</li>
                  <li>• 1 roomda 20 tagacha a&apos;zo</li>
                  <li>• Reklamasiz, 720p/1080p sifat</li>
                </ul>
              </div>
              <div className="flex gap-2">
                <Link
                  href="/premium"
                  onClick={() => setQuotaPopup(false)}
                  className="flex-1 px-4 py-2.5 bg-gradient-to-r from-brand-red to-orange-600 rounded-lg text-white text-sm font-semibold flex items-center justify-center gap-2"
                >
                  <Crown className="w-4 h-4" /> PREMIUM olish
                </Link>
                <button
                  onClick={() => setQuotaPopup(false)}
                  className="px-4 py-2.5 bg-brand-dark border border-brand-border rounded-lg text-sm text-gray-300"
                >
                  Keyinroq
                </button>
              </div>
              <p className="text-[10px] text-gray-500 text-center">
                Limit har kuni yarim tunda yangilanadi.
              </p>
            </div>
          </div>
        </div>,
        document.body,
      )}
    </>
  );
}
