"use client";

import { useEffect } from "react";
import {
  AlertTriangle,
  BadgeCheck,
  EyeOff,
  Flag,
  HeartHandshake,
  Link2Off,
  Lock,
  MessageSquareText,
  Repeat,
  ScrollText,
  ShieldAlert,
  X,
  type LucideIcon,
} from "lucide-react";

const RULES: { icon: LucideIcon; tone: string; title: string; text: string }[] = [
  {
    icon: HeartHandshake,
    tone: "bg-rose-500/15 text-rose-300",
    title: "Hurmat bilan yozing",
    text: "Haqorat, so'kinish, kamsitish va boshqa foydalanuvchilarni masxara qilish taqiqlanadi. Fikringizga qo'shilmaganlar bilan ham odob bilan bahslashing.",
  },
  {
    icon: EyeOff,
    tone: "bg-amber-500/15 text-amber-300",
    title: "Spoylerni belgilang",
    text: "Syujet, final yoki muhim burilishlar haqida yozsangiz, «Spoyler bor» belgisini qo'ying — izohingiz boshqalar uchun yashirin ko'rinadi.",
  },
  {
    icon: Link2Off,
    tone: "bg-sky-500/15 text-sky-300",
    title: "Reklama va havolalar yo'q",
    text: "Boshqa saytlar, Telegram kanallar, do'konlar va xizmatlar reklamasi, shuningdek begona havolalar avtomatik bloklanadi.",
  },
  {
    icon: MessageSquareText,
    tone: "bg-violet-500/15 text-violet-300",
    title: "Mavzuga oid bo'lsin",
    text: "Izoh shu kino, serial yoki qism haqida bo'lsin: taassurot, baho, savol yoki tavsiya. Boshqa mavzular uchun sahifa emas.",
  },
  {
    icon: Repeat,
    tone: "bg-emerald-500/15 text-emerald-300",
    title: "Spam va flood qilmang",
    text: "Bir xil izohni qayta-qayta yozish, faqat smayl yoki ma'nosiz belgilar yuborish ham spam hisoblanadi. Tizim juda tez-tez yozishni cheklaydi.",
  },
  {
    icon: ShieldAlert,
    tone: "bg-orange-500/15 text-orange-300",
    title: "Nizoli mavzular",
    text: "Siyosiy tashviqot, diniy va millatlararo adovatni qo'zg'atuvchi, zo'ravonlik yoki noqonuniy harakatlarga chaqiruvchi izohlar o'chiriladi.",
  },
  {
    icon: Lock,
    tone: "bg-slate-500/20 text-slate-300",
    title: "Shaxsiy ma'lumotlarni yozmang",
    text: "Telefon raqam, manzil, karta raqami yoki boshqalarning shaxsiy ma'lumotlarini joylamang — o'zingizni ham, boshqalarni ham himoya qiling.",
  },
  {
    icon: BadgeCheck,
    tone: "bg-teal-500/15 text-teal-300",
    title: "Qaroqchilikka yo'l yo'q",
    text: "Kinolarni yuklab olish havolalari yoki boshqa saytlardagi noqonuniy nusxalarni tarqatish mumkin emas.",
  },
];

/** "Izoh qoidalari" — comment guidelines shown in a modal. */
export default function CommentRulesModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-[200] flex items-end justify-center sm:items-center sm:p-4" role="dialog" aria-modal="true" aria-labelledby="comment-rules-title">
      <div className="absolute inset-0 animate-[fadeIn_.15s_ease-out] bg-black/75" onClick={onClose} />
      <div className="relative flex max-h-[90vh] w-full max-w-2xl animate-[sheetUp_.22s_ease-out] flex-col overflow-hidden rounded-t-3xl border border-white/10 bg-[#0f0f16] shadow-2xl sm:rounded-3xl">
        {/* Header */}
        <div className="relative overflow-hidden border-b border-white/5 px-5 pb-5 pt-6 sm:px-6">
          <div className="pointer-events-none absolute -right-10 -top-16 h-48 w-48 rounded-full bg-orange-500/20 blur-3xl" aria-hidden />
          <div className="pointer-events-none absolute -left-16 top-4 h-40 w-40 rounded-full bg-fuchsia-500/10 blur-3xl" aria-hidden />
          <div className="relative flex items-start gap-4">
            <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br from-orange-500 to-rose-600 shadow-lg shadow-orange-500/20">
              <ScrollText size={22} className="text-white" />
            </span>
            <div className="min-w-0 flex-1">
              <h2 id="comment-rules-title" className="font-display text-2xl tracking-wide text-white">
                Izoh qoidalari
              </h2>
              <p className="mt-0.5 text-sm text-gray-400">FilmoraUz — kino ixlosmandlari uchun do&apos;stona joy. Keling, uni birga shunday saqlaymiz.</p>
            </div>
            <button type="button" onClick={onClose} className="rounded-lg p-1.5 text-gray-400 hover:bg-white/10 hover:text-white" aria-label="Yopish">
              <X size={18} />
            </button>
          </div>
        </div>

        {/* Rules */}
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-5 py-5 sm:px-6">
          <ol className="grid gap-3 sm:grid-cols-2">
            {RULES.map((r, i) => {
              const Icon = r.icon;
              return (
                <li key={r.title} className="rounded-2xl border border-white/[0.07] bg-white/[0.02] p-4 transition hover:border-white/15">
                  <div className="mb-2 flex items-center gap-2.5">
                    <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${r.tone}`}>
                      <Icon size={17} />
                    </span>
                    <h3 className="font-semibold leading-tight text-white">
                      <span className="mr-1 text-gray-600">{i + 1}.</span>
                      {r.title}
                    </h3>
                  </div>
                  <p className="text-[13px] leading-relaxed text-gray-400">{r.text}</p>
                </li>
              );
            })}
          </ol>

          <div className="mt-4 rounded-2xl border border-amber-500/20 bg-amber-500/[0.06] p-4">
            <div className="mb-2 flex items-center gap-2 font-semibold text-amber-200">
              <AlertTriangle size={17} /> Qoidani buzsangiz
            </div>
            <ol className="space-y-1.5 text-[13px] text-gray-300">
              <li>
                <span className="font-semibold text-white">1.</span> Izoh yashiriladi yoki o&apos;chiriladi.
              </li>
              <li>
                <span className="font-semibold text-white">2.</span> Takrorlansa — izoh yozish vaqtincha cheklanadi.
              </li>
              <li>
                <span className="font-semibold text-white">3.</span> Jiddiy yoki muntazam buzilishda — akkaunt bloklanadi. Bloklanganlar apellyatsiya yuborishi mumkin.
              </li>
            </ol>
          </div>

          <p className="mt-4 flex items-start gap-2 text-xs text-gray-500">
            <Flag size={14} className="mt-0.5 shrink-0" />
            Qoidaga zid izohni ko&apos;rsangiz, uning yonidagi «Shikoyat» tugmasini bosing — moderatorlar tez orada ko&apos;rib chiqadi.
          </p>
        </div>

        <div className="border-t border-white/5 px-5 py-3 sm:px-6">
          <button
            type="button"
            onClick={onClose}
            className="w-full rounded-xl bg-orange-500 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 transition hover:bg-orange-400"
          >
            Tushunarli, qoidalarga amal qilaman
          </button>
        </div>
      </div>
    </div>
  );
}
