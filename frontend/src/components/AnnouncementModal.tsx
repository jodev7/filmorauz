"use client";

import { useEffect } from "react";
import { AlertTriangle, ArrowRight, Crown, Gift, Megaphone, PartyPopper, Sparkles, X, type LucideIcon } from "lucide-react";

export type AnnouncementVariant = "info" | "update" | "promo" | "warning" | "celebration" | "premium";

export interface AnnouncementView {
  title: string;
  body?: string;
  link_url?: string;
  link_label?: string;
  variant?: string;
  image_url?: string;
  ends_at?: string;
  dismissible?: boolean;
}

interface Theme {
  label: string;
  icon: LucideIcon;
  /** icon circle + CTA gradient */
  gradient: string;
  /** soft glow behind the header */
  glow: string;
  ring: string;
  chip: string;
}

export const ANNOUNCEMENT_THEMES: Record<AnnouncementVariant, Theme> = {
  info: { label: "E'lon", icon: Megaphone, gradient: "from-sky-500 to-indigo-600", glow: "bg-sky-500/30", ring: "border-sky-400/30", chip: "bg-sky-500/15 text-sky-300" },
  update: { label: "Yangilik", icon: Sparkles, gradient: "from-violet-500 to-fuchsia-600", glow: "bg-violet-500/30", ring: "border-violet-400/30", chip: "bg-violet-500/15 text-violet-300" },
  promo: { label: "Aksiya", icon: Gift, gradient: "from-orange-500 to-rose-600", glow: "bg-orange-500/30", ring: "border-orange-400/30", chip: "bg-orange-500/15 text-orange-300" },
  warning: { label: "Diqqat", icon: AlertTriangle, gradient: "from-amber-500 to-red-600", glow: "bg-amber-500/25", ring: "border-amber-400/30", chip: "bg-amber-500/15 text-amber-300" },
  celebration: { label: "Bayram", icon: PartyPopper, gradient: "from-emerald-500 to-cyan-600", glow: "bg-emerald-500/25", ring: "border-emerald-400/30", chip: "bg-emerald-500/15 text-emerald-300" },
  premium: { label: "Premium", icon: Crown, gradient: "from-yellow-400 to-amber-600", glow: "bg-yellow-500/25", ring: "border-yellow-400/30", chip: "bg-yellow-500/15 text-yellow-300" },
};

export function announcementTheme(v?: string): Theme {
  return ANNOUNCEMENT_THEMES[(v as AnnouncementVariant) in ANNOUNCEMENT_THEMES ? (v as AnnouncementVariant) : "info"];
}

/** "3 kun 4 soat" / "5 soat 10 daqiqa" left until `iso`, only when < 7 days. */
function timeLeft(iso?: string): string {
  if (!iso) return "";
  const ms = new Date(iso).getTime() - Date.now();
  if (!(ms > 0) || ms > 7 * 24 * 3600e3) return "";
  const m = Math.floor(ms / 60000);
  const d = Math.floor(m / 1440);
  const h = Math.floor((m % 1440) / 60);
  if (d > 0) return `${d} kun${h ? ` ${h} soat` : ""}`;
  if (h > 0) return `${h} soat${m % 60 ? ` ${m % 60} daqiqa` : ""}`;
  return `${Math.max(1, m)} daqiqa`;
}

const CONFETTI = ["#f97316", "#22c55e", "#eab308", "#ec4899", "#38bdf8", "#a855f7"];

/**
 * The card itself. `overlay` wraps it in the full-screen backdrop (site);
 * without it the card renders inline (admin live preview).
 */
export default function AnnouncementModal({
  a,
  overlay = true,
  onClose,
  onLink,
  counter,
}: {
  a: AnnouncementView;
  overlay?: boolean;
  onClose?: () => void;
  onLink?: () => void;
  /** e.g. "1/3" when several announcements are queued */
  counter?: string;
}) {
  const theme = announcementTheme(a.variant);
  const Icon = theme.icon;
  const dismissible = a.dismissible !== false;
  const left = a.variant === "promo" || a.variant === "premium" ? timeLeft(a.ends_at) : "";
  const hasLink = !!a.link_url;

  useEffect(() => {
    if (!overlay) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && dismissible && onClose?.();
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [overlay, dismissible, onClose]);

  const card = (
    <div
      role={overlay ? "dialog" : undefined}
      aria-modal={overlay ? true : undefined}
      aria-labelledby={overlay ? "announcement-title" : undefined}
      className={`ann-pop relative w-full max-w-md overflow-hidden rounded-3xl border ${theme.ring} bg-[#111119] shadow-[0_30px_80px_-20px_rgba(0,0,0,0.8)]`}
    >
      {/* Header: cover image, or a glowing icon */}
      {a.image_url ? (
        <div className="relative aspect-[16/9] w-full overflow-hidden">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img src={a.image_url} alt="" className="h-full w-full object-cover" />
          <div className="absolute inset-0 bg-gradient-to-t from-[#111119] via-[#111119]/30 to-transparent" />
        </div>
      ) : (
        <div className="relative flex h-36 items-center justify-center">
          <div className={`absolute -top-16 left-1/2 h-48 w-72 -translate-x-1/2 rounded-full blur-3xl ${theme.glow}`} />
          <div className="absolute inset-0 bg-[radial-gradient(circle_at_center,rgba(255,255,255,0.06)_1px,transparent_1px)] [background-size:14px_14px] [mask-image:radial-gradient(ellipse_at_center,black,transparent_70%)]" />
          {a.variant === "celebration" &&
            Array.from({ length: 14 }).map((_, i) => (
              <span
                key={i}
                className="ann-fall absolute top-0 h-2 w-1.5 rounded-sm"
                style={{ left: `${(i * 7.3) % 100}%`, background: CONFETTI[i % CONFETTI.length], animationDelay: `${(i % 7) * 0.45}s` }}
                aria-hidden="true"
              />
            ))}
          <span className={`ann-float relative flex h-20 w-20 items-center justify-center rounded-3xl bg-gradient-to-br ${theme.gradient} shadow-2xl ring-8 ring-white/[0.04]`}>
            <Icon size={36} className="text-white drop-shadow" strokeWidth={2.2} />
          </span>
        </div>
      )}

      {dismissible && onClose && (
        <button
          type="button"
          onClick={onClose}
          className="absolute right-3 top-3 z-10 flex h-9 w-9 items-center justify-center rounded-full bg-black/40 text-white/80 backdrop-blur transition hover:bg-black/70 hover:text-white"
          aria-label="Yopish"
        >
          <X size={18} />
        </button>
      )}

      <div className={`px-6 pb-6 ${a.image_url ? "-mt-6 relative" : "pt-1"} text-center`}>
        <div className="flex items-center justify-center gap-2">
          <span className={`inline-flex items-center gap-1 rounded-full px-2.5 py-1 text-[11px] font-semibold uppercase tracking-wider ${theme.chip}`}>
            <theme.icon size={12} />
            {theme.label}
          </span>
          {counter && <span className="rounded-full bg-white/5 px-2 py-1 text-[11px] text-gray-400">{counter}</span>}
        </div>
        <h2 id={overlay ? "announcement-title" : undefined} className="mt-3 font-display text-2xl leading-tight tracking-wide text-white sm:text-[1.7rem]">
          {a.title || "Sarlavha"}
        </h2>
        {a.body && <p className="mx-auto mt-3 max-w-sm whitespace-pre-line text-sm leading-relaxed text-gray-300 sm:text-[15px]">{a.body}</p>}
        {left && (
          <p className="mt-4 inline-flex items-center gap-1.5 rounded-full border border-white/10 bg-white/[0.03] px-3 py-1 text-xs text-gray-300">
            <span className="h-1.5 w-1.5 animate-pulse rounded-full bg-rose-400" />
            Tugashiga {left} qoldi
          </p>
        )}

        <div className="mt-6 flex flex-col gap-2.5">
          {hasLink && (
            <a
              href={a.link_url}
              target="_blank"
              rel="noopener noreferrer"
              onClick={onLink}
              className={`group relative inline-flex items-center justify-center gap-2 overflow-hidden rounded-2xl bg-gradient-to-r ${theme.gradient} px-5 py-3 text-sm font-semibold text-white shadow-lg transition hover:brightness-110`}
            >
              <span className="ann-shine pointer-events-none absolute inset-y-0 left-0 w-1/3 bg-white/25 blur-sm" aria-hidden="true" />
              {a.link_label || "Batafsil"}
              <ArrowRight size={16} className="transition-transform group-hover:translate-x-0.5" />
            </a>
          )}
          {dismissible && (
            <button
              type="button"
              onClick={onClose}
              className={`rounded-2xl px-5 py-3 text-sm font-medium transition ${
                hasLink ? "text-gray-400 hover:bg-white/5 hover:text-white" : `bg-gradient-to-r ${theme.gradient} text-white shadow-lg hover:brightness-110`
              }`}
            >
              {hasLink ? "Keyinroq" : "Tushunarli"}
            </button>
          )}
        </div>
      </div>
    </div>
  );

  if (!overlay) return card;
  return (
    <div className="fixed inset-0 z-[200] flex items-end justify-center p-3 sm:items-center sm:p-4">
      <div className="absolute inset-0 animate-[fadeIn_.2s_ease-out] bg-black/75 backdrop-blur-md" onClick={() => dismissible && onClose?.()} aria-hidden="true" />
      {card}
    </div>
  );
}
