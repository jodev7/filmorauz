"use client";

import { useEffect, useRef } from "react";
import Link from "next/link";
import { Crown, Check } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { recordPremiumEvent } from "@/lib/api";

const BENEFITS = ["Premium kinolar va seriallar", "Reklamasiz, 1080p sifatda", "Telegram Stars bilan bir necha soniyada"];

/**
 * Shown instead of the player on premium-only titles. Sends the viewer to
 * the on-site /premium checkout (with a way back to this title) instead of
 * the bare bot link, and records lock_view / cta_click for the funnel.
 */
export default function PremiumUnlockCard({
  targetType,
  targetId,
  title,
  fromPath,
  priceHint = "oyiga 100 ⭐ dan",
}: {
  targetType: "movie" | "series" | "episode";
  targetId: string;
  title: string;
  fromPath: string;
  priceHint?: string;
}) {
  const { token } = useAuth();
  const viewed = useRef(false);

  useEffect(() => {
    if (viewed.current || !token) return;
    viewed.current = true;
    recordPremiumEvent(token, "lock_view", { type: targetType, id: targetId });
  }, [token, targetType, targetId]);

  const href = `/premium?from=${encodeURIComponent(fromPath)}&t=${encodeURIComponent(title)}#telegram-stars`;

  return (
    <div className="absolute inset-0 flex items-center justify-center p-4">
      <div className="w-full max-w-md rounded-2xl border border-yellow-500/30 bg-black/70 p-6 text-center shadow-[0_0_30px_rgba(234,179,8,0.12)] backdrop-blur">
        <div className="mx-auto mb-3 flex h-14 w-14 items-center justify-center rounded-full bg-gradient-to-br from-yellow-500 to-amber-600">
          <Crown size={28} className="text-black" />
        </div>
        <h3 className="text-lg font-semibold text-white">«{title}» — Premium kontent</h3>
        <ul className="mx-auto mt-3 inline-flex flex-col gap-1 text-left text-sm text-gray-300">
          {BENEFITS.map((b) => (
            <li key={b} className="flex items-center gap-2">
              <Check size={14} className="text-yellow-400" aria-hidden /> {b}
            </li>
          ))}
        </ul>
        <div className="mt-5">
          <Link
            href={href}
            onClick={() => recordPremiumEvent(token, "cta_click", { type: targetType, id: targetId })}
            className="inline-flex items-center gap-2 rounded-xl bg-gradient-to-r from-yellow-500 to-amber-600 px-6 py-3 font-semibold text-black shadow-[0_0_15px_rgba(234,179,8,0.3)] hover:from-yellow-400 hover:to-amber-500"
          >
            <Crown size={18} /> Premium olish
          </Link>
          <p className="mt-2 text-xs text-gray-400">{priceHint} · to&apos;lovdan so&apos;ng shu yerga qaytasiz</p>
        </div>
      </div>
    </div>
  );
}
