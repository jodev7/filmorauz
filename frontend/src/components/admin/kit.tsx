"use client";

import { ReactNode, useEffect, useState } from "react";
import { ChevronLeft, ChevronRight, Search, X, type LucideIcon } from "lucide-react";

// Shared building blocks for admin list pages (users, bans, comments, ads...).

export type Tone = "gray" | "green" | "yellow" | "red" | "blue" | "orange" | "violet" | "sky";

const TONE_CHIP: Record<Tone, string> = {
  gray: "bg-white/5 text-gray-400",
  green: "bg-emerald-500/15 text-emerald-300",
  yellow: "bg-amber-500/15 text-amber-300",
  red: "bg-red-500/15 text-red-300",
  blue: "bg-blue-500/15 text-blue-300",
  orange: "bg-orange-500/15 text-orange-300",
  violet: "bg-violet-500/15 text-violet-300",
  sky: "bg-sky-500/15 text-sky-300",
};
const TONE_DOT: Record<Tone, string> = {
  gray: "bg-gray-500",
  green: "bg-emerald-400",
  yellow: "bg-amber-400",
  red: "bg-red-400",
  blue: "bg-blue-400",
  orange: "bg-orange-400",
  violet: "bg-violet-400",
  sky: "bg-sky-400",
};
const TONE_ICON: Record<Tone, string> = {
  gray: "bg-white/5 text-gray-300",
  green: "bg-emerald-500/15 text-emerald-400",
  yellow: "bg-amber-500/15 text-amber-400",
  red: "bg-red-500/15 text-red-400",
  blue: "bg-blue-500/15 text-blue-400",
  orange: "bg-orange-500/15 text-orange-400",
  violet: "bg-violet-500/15 text-violet-400",
  sky: "bg-sky-500/15 text-sky-400",
};

export function PageHead({
  icon: Icon,
  gradient = "from-orange-500 to-rose-600",
  title,
  subtitle,
  actions,
}: {
  icon: LucideIcon;
  gradient?: string;
  title: string;
  subtitle?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <header className="mb-6 flex flex-wrap items-start justify-between gap-3">
      <div className="flex min-w-0 items-start gap-3">
        <span className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-2xl bg-gradient-to-br ${gradient} shadow-lg shadow-black/30`}>
          <Icon size={20} className="text-white" />
        </span>
        <div className="min-w-0">
          <h1 className="text-2xl font-bold text-white">{title}</h1>
          {subtitle && <p className="text-sm text-gray-500">{subtitle}</p>}
        </div>
      </div>
      {actions && <div className="flex flex-wrap items-center gap-2">{actions}</div>}
    </header>
  );
}

export function PrimaryButton({ children, onClick, disabled, type = "button" }: { children: ReactNode; onClick?: () => void; disabled?: boolean; type?: "button" | "submit" }) {
  return (
    <button
      type={type}
      onClick={onClick}
      disabled={disabled}
      className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 transition hover:bg-orange-400 disabled:cursor-not-allowed disabled:opacity-50"
    >
      {children}
    </button>
  );
}

export function GhostButton({ children, onClick, disabled }: { children: ReactNode; onClick?: () => void; disabled?: boolean }) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      className="inline-flex items-center gap-2 rounded-xl border border-white/10 bg-white/[0.03] px-3.5 py-2.5 text-sm text-gray-300 transition hover:bg-white/[0.07] hover:text-white disabled:opacity-50"
    >
      {children}
    </button>
  );
}

export function StatTile({
  label,
  value,
  icon: Icon,
  tone = "gray",
  hint,
  active,
  onClick,
}: {
  label: string;
  value: ReactNode;
  icon: LucideIcon;
  tone?: Tone;
  hint?: ReactNode;
  active?: boolean;
  onClick?: () => void;
}) {
  const Tag = onClick ? "button" : "div";
  return (
    <Tag
      {...(onClick ? { type: "button" as const, onClick } : {})}
      className={`flex items-center gap-3 rounded-2xl border p-4 text-left transition ${
        active ? "border-orange-500/50 bg-orange-500/[0.06]" : "border-white/10 bg-[#12121a]"
      } ${onClick ? "hover:border-white/20" : ""}`}
    >
      <span className={`flex h-10 w-10 shrink-0 items-center justify-center rounded-xl ${TONE_ICON[tone]}`}>
        <Icon size={18} />
      </span>
      <span className="min-w-0">
        <span className="block truncate text-xs text-gray-400">{label}</span>
        <span className="block text-xl font-bold tabular-nums text-white">{value}</span>
        {hint && <span className="block truncate text-[11px] text-gray-500">{hint}</span>}
      </span>
    </Tag>
  );
}

export function Tabs<T extends string>({
  value,
  onChange,
  items,
}: {
  value: T;
  onChange: (v: T) => void;
  items: { key: T; label: string; count?: number }[];
}) {
  return (
    <div className="scrollbar-hide flex gap-1 overflow-x-auto rounded-xl border border-white/10 bg-black/20 p-1">
      {items.map((it) => (
        <button
          key={it.key}
          type="button"
          onClick={() => onChange(it.key)}
          className={`whitespace-nowrap rounded-lg px-3 py-1.5 text-xs font-medium transition ${
            value === it.key ? "bg-white/10 text-white" : "text-gray-400 hover:text-white"
          }`}
        >
          {it.label}
          {it.count !== undefined && <span className="ml-1.5 text-gray-500">{it.count}</span>}
        </button>
      ))}
    </div>
  );
}

export function SearchBox({ value, onChange, placeholder }: { value: string; onChange: (v: string) => void; placeholder: string }) {
  return (
    <div className="relative min-w-0 flex-1">
      <Search size={16} className="pointer-events-none absolute left-3.5 top-1/2 -translate-y-1/2 text-gray-500" />
      <input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
        className="w-full rounded-xl border border-white/10 bg-black/20 py-2.5 pl-10 pr-9 text-sm text-white placeholder-gray-600 focus:border-orange-500 focus:outline-none focus:ring-2 focus:ring-orange-500/20"
      />
      {value && (
        <button type="button" onClick={() => onChange("")} className="absolute right-2.5 top-1/2 -translate-y-1/2 rounded p-1 text-gray-500 hover:text-white" aria-label="Tozalash">
          <X size={14} />
        </button>
      )}
    </div>
  );
}

export function Chip({ tone = "gray", dot, icon: Icon, children, title }: { tone?: Tone; dot?: boolean; icon?: LucideIcon; children: ReactNode; title?: string }) {
  return (
    <span title={title} className={`inline-flex items-center gap-1.5 whitespace-nowrap rounded-full px-2 py-0.5 text-[11px] font-medium ${TONE_CHIP[tone]}`}>
      {dot && <span className={`h-1.5 w-1.5 rounded-full ${TONE_DOT[tone]}`} />}
      {Icon && <Icon size={11} />}
      {children}
    </span>
  );
}

export function IconBtn({
  label,
  onClick,
  tone = "gray",
  disabled,
  children,
}: {
  label: string;
  onClick?: () => void;
  tone?: "gray" | "green" | "red" | "yellow" | "blue";
  disabled?: boolean;
  children: ReactNode;
}) {
  const hover = {
    gray: "hover:bg-white/5 hover:text-white",
    green: "hover:bg-emerald-500/10 hover:text-emerald-300",
    red: "hover:bg-red-500/10 hover:text-red-300",
    yellow: "hover:bg-amber-500/10 hover:text-amber-300",
    blue: "hover:bg-sky-500/10 hover:text-sky-300",
  }[tone];
  return (
    <button type="button" onClick={onClick} disabled={disabled} title={label} aria-label={label} className={`rounded-lg p-2 text-gray-400 transition disabled:opacity-40 ${hover}`}>
      {children}
    </button>
  );
}

const AVATAR_BG = ["from-orange-500 to-rose-600", "from-sky-500 to-indigo-600", "from-emerald-500 to-teal-700", "from-fuchsia-500 to-purple-700", "from-amber-500 to-orange-700"];

export function Avatar({ name, src, size = 36 }: { name: string; src?: string; size?: number }) {
  let h = 0;
  for (const c of name || "?") h = (h * 31 + c.charCodeAt(0)) >>> 0;
  const initials = (name || "?")
    .replace(/^@/, "")
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase())
    .join("");
  return (
    <span
      className={`relative flex shrink-0 items-center justify-center overflow-hidden rounded-full bg-gradient-to-br ${AVATAR_BG[h % AVATAR_BG.length]} font-semibold text-white`}
      style={{ width: size, height: size, fontSize: size * 0.38 }}
      aria-hidden="true"
    >
      {initials || "?"}
      {src && (
        // eslint-disable-next-line @next/next/no-img-element
        <img src={src} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
      )}
    </span>
  );
}

export function EmptyState({ icon: Icon, title, text, action }: { icon: LucideIcon; title: string; text?: string; action?: ReactNode }) {
  return (
    <div className="flex flex-col items-center rounded-2xl border border-dashed border-white/10 bg-[#12121a] px-6 py-14 text-center">
      <span className="flex h-14 w-14 items-center justify-center rounded-2xl bg-white/5">
        <Icon size={24} className="text-gray-500" />
      </span>
      <p className="mt-4 font-medium text-white">{title}</p>
      {text && <p className="mt-1 max-w-sm text-sm text-gray-500">{text}</p>}
      {action && <div className="mt-5">{action}</div>}
    </div>
  );
}

export function SkeletonList({ rows = 5, height = 72 }: { rows?: number; height?: number }) {
  return (
    <div className="space-y-2.5">
      {Array.from({ length: rows }).map((_, i) => (
        <div key={i} className="animate-pulse rounded-2xl border border-white/5 bg-white/[0.03]" style={{ height }} />
      ))}
    </div>
  );
}

export function Pager({ page, totalPages, total, unit, onChange }: { page: number; totalPages: number; total?: number; unit?: string; onChange: (p: number) => void }) {
  if (totalPages <= 1) return total !== undefined && total > 0 ? <p className="mt-4 text-sm text-gray-500">Jami: {total.toLocaleString()} {unit}</p> : null;
  return (
    <div className="mt-4 flex items-center justify-between gap-3">
      <p className="text-sm text-gray-500">{total !== undefined && `Jami: ${total.toLocaleString()} ${unit ?? ""}`}</p>
      <div className="flex items-center gap-1.5">
        <button type="button" onClick={() => onChange(Math.max(1, page - 1))} disabled={page <= 1} className="rounded-lg border border-white/10 p-2 text-gray-300 hover:bg-white/5 disabled:opacity-40" aria-label="Oldingi">
          <ChevronLeft size={16} />
        </button>
        <span className="min-w-[64px] text-center text-sm tabular-nums text-gray-300">
          {page} / {totalPages}
        </span>
        <button type="button" onClick={() => onChange(Math.min(totalPages, page + 1))} disabled={page >= totalPages} className="rounded-lg border border-white/10 p-2 text-gray-300 hover:bg-white/5 disabled:opacity-40" aria-label="Keyingi">
          <ChevronRight size={16} />
        </button>
      </div>
    </div>
  );
}

/**
 * Lightweight modal: no backdrop-filter (blur over a big page is what made
 * the old admin modals janky), Escape closes, body doesn't scroll behind it.
 */
export function Modal({
  open,
  onClose,
  title,
  subtitle,
  icon: Icon,
  iconTone = "orange",
  size = "md",
  children,
  footer,
  busy,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  subtitle?: ReactNode;
  icon?: LucideIcon;
  iconTone?: Tone;
  size?: "sm" | "md" | "lg" | "xl";
  children: ReactNode;
  footer?: ReactNode;
  busy?: boolean;
}) {
  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && !busy && onClose();
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [open, busy, onClose]);

  if (!open) return null;
  const width = { sm: "max-w-sm", md: "max-w-lg", lg: "max-w-2xl", xl: "max-w-4xl" }[size];
  return (
    <div className="fixed inset-0 z-[150] flex items-end justify-center sm:items-center sm:p-4" role="dialog" aria-modal="true">
      <div className="absolute inset-0 animate-[fadeIn_.15s_ease-out] bg-black/75" onClick={() => !busy && onClose()} />
      <div className={`relative flex max-h-[92vh] w-full ${width} animate-[sheetUp_.22s_ease-out] flex-col overflow-hidden rounded-t-3xl border border-white/10 bg-[#0f0f16] shadow-2xl sm:rounded-3xl`}>
        <div className="flex items-start justify-between gap-3 border-b border-white/5 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            {Icon && (
              <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${TONE_ICON[iconTone]}`}>
                <Icon size={17} />
              </span>
            )}
            <div className="min-w-0">
              <h2 className="truncate font-semibold text-white">{title}</h2>
              {subtitle && <p className="truncate text-xs text-gray-500">{subtitle}</p>}
            </div>
          </div>
          <button type="button" onClick={onClose} disabled={busy} className="rounded-lg p-1.5 text-gray-500 hover:bg-white/5 hover:text-white" aria-label="Yopish">
            <X size={18} />
          </button>
        </div>
        <div className="min-h-0 flex-1 overflow-y-auto overscroll-contain p-5">{children}</div>
        {footer && <div className="flex items-center justify-end gap-2 border-t border-white/5 px-5 py-3">{footer}</div>}
      </div>
    </div>
  );
}

/** Debounced copy of a value (search inputs). */
export function useDebounced<T>(value: T, ms = 300): T {
  const [v, setV] = useState(value);
  useEffect(() => {
    const t = setTimeout(() => setV(value), ms);
    return () => clearTimeout(t);
  }, [value, ms]);
  return v;
}

const MONTHS = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avg", "sen", "okt", "noy", "dek"];
/** "27-sen 2026, 14:05" (year only when not the current one). */
export function fmtDate(iso?: string | null, withTime = true): string {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d.getTime()) || d.getFullYear() < 2000) return "—";
  const pad = (n: number) => String(n).padStart(2, "0");
  const year = d.getFullYear() !== new Date().getFullYear() ? ` ${d.getFullYear()}` : "";
  return `${d.getDate()}-${MONTHS[d.getMonth()]}${year}${withTime ? `, ${pad(d.getHours())}:${pad(d.getMinutes())}` : ""}`;
}

/** "5 daqiqa oldin", "3 kun oldin". */
export function timeAgo(iso?: string | null): string {
  if (!iso) return "—";
  const ms = Date.now() - new Date(iso).getTime();
  if (isNaN(ms)) return "—";
  const m = Math.floor(ms / 60000);
  if (m < 1) return "hozirgina";
  if (m < 60) return `${m} daqiqa oldin`;
  const h = Math.floor(m / 60);
  if (h < 24) return `${h} soat oldin`;
  const d = Math.floor(h / 24);
  if (d < 30) return `${d} kun oldin`;
  return fmtDate(iso, false);
}

/** <img> that falls back to an icon tile when the URL is empty or broken. */
export function Thumb({ src, alt = "", className, icon: Icon }: { src?: string; alt?: string; className: string; icon: LucideIcon }) {
  const [failed, setFailed] = useState(false);
  if (!src || failed) {
    return (
      <span className={`${className} flex items-center justify-center bg-white/5`}>
        <Icon size={16} className="text-gray-600" />
      </span>
    );
  }
  // eslint-disable-next-line @next/next/no-img-element
  return <img src={src} alt={alt} loading="lazy" onError={() => setFailed(true)} className={`${className} object-cover`} />;
}
