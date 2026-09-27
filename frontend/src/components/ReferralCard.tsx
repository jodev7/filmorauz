"use client";

import { useEffect, useState } from "react";
import { Gift, Copy, Check, Send } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getMyReferral, MyReferral } from "@/lib/api";

/** Profile card: personal invite link + progress for the referral program. */
export default function ReferralCard() {
  const { token } = useAuth();
  const [data, setData] = useState<MyReferral | null>(null);
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    if (!token) return;
    getMyReferral(token).then(setData).catch(() => {});
  }, [token]);

  if (!data) return null;

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(data.link);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      /* clipboard blocked */
    }
  };

  const shareText = `FilmoraUz'da kinolarni o'zbek tilida bepul ko'raman — qo'shil! Havola orqali kirsang, sovg'a sifatida ${data.welcome_days} kun premium olasan.`;
  const telegramShare = `https://t.me/share/url?url=${encodeURIComponent(data.link)}&text=${encodeURIComponent(shareText)}`;

  return (
    <section className="mt-8 mb-10 rounded-2xl border border-yellow-500/25 bg-gradient-to-br from-yellow-500/10 to-transparent p-5">
      <div className="mb-3 flex items-center gap-3">
        <div className="flex h-9 w-9 items-center justify-center rounded-lg border border-yellow-500/30 bg-yellow-500/10">
          <Gift className="h-4 w-4 text-yellow-400" />
        </div>
        <div>
          <h2 className="font-display text-xl text-white tracking-wide">DO&apos;STLARNI TAKLIF QILING</h2>
          <p className="text-xs text-gray-400">
            Har bir do&apos;stingiz tomosha qilishni boshlasa — sizga {data.days_per_friend} kun, unga {data.welcome_days} kun premium.
          </p>
        </div>
      </div>

      <div className="flex flex-col gap-2 sm:flex-row">
        <input
          readOnly
          value={data.link}
          onFocus={(e) => e.currentTarget.select()}
          className="flex-1 rounded-lg border border-white/10 bg-black/40 px-3 py-2 font-mono text-sm text-white"
          aria-label="Taklif havolasi"
        />
        <div className="flex gap-2">
          <button onClick={copy} className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 px-3 py-2 text-sm text-gray-200 hover:border-white/30">
            {copied ? <Check size={15} className="text-emerald-400" /> : <Copy size={15} />}
            {copied ? "Nusxalandi" : "Nusxalash"}
          </button>
          <a
            href={telegramShare}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center gap-1.5 rounded-lg bg-[#229ED9] px-3 py-2 text-sm font-medium text-white hover:bg-[#1c8cc2]"
          >
            <Send size={15} /> Telegram
          </a>
        </div>
      </div>

      <dl className="mt-4 grid grid-cols-3 gap-3 text-center">
        <div className="rounded-lg bg-black/30 p-2">
          <dt className="text-[11px] text-gray-500">Taklif qilingan</dt>
          <dd className="text-lg font-bold text-white tabular-nums">{data.invited}</dd>
        </div>
        <div className="rounded-lg bg-black/30 p-2">
          <dt className="text-[11px] text-gray-500">Faol do&apos;stlar</dt>
          <dd className="text-lg font-bold text-white tabular-nums">{data.rewarded}</dd>
        </div>
        <div className="rounded-lg bg-black/30 p-2">
          <dt className="text-[11px] text-gray-500">Olingan kunlar</dt>
          <dd className="text-lg font-bold text-yellow-400 tabular-nums">{data.reward_days}</dd>
        </div>
      </dl>
    </section>
  );
}
