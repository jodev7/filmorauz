"use client";

import { ReactNode, useCallback, useEffect, useState } from "react";
import { Check, Copy, Gift, Link2, Loader2, Share2, X } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createMovieShare, createSeriesShare, getMyReferral } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

const SITE_URL = (process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net").replace(/\/$/, "");

type Kind = "movie" | "series" | "episode";

// ─── Brand marks (inline so they don't depend on an icon pack) ───────────

const Svg = ({ children, viewBox = "0 0 24 24" }: { children: ReactNode; viewBox?: string }) => (
  <svg viewBox={viewBox} width="22" height="22" fill="currentColor" aria-hidden>
    {children}
  </svg>
);
const TelegramIcon = () => (
  <Svg>
    <path d="M21.94 4.3 18.7 19.6c-.24 1.07-.88 1.34-1.78.83l-4.93-3.63-2.38 2.29c-.26.26-.48.48-.99.48l.35-5.02 9.14-8.26c.4-.35-.09-.55-.62-.2L6.2 13.2l-4.87-1.52c-1.06-.33-1.08-1.06.22-1.57L20.6 2.77c.88-.33 1.65.2 1.34 1.53Z" />
  </Svg>
);
const XIcon = () => (
  <Svg>
    <path d="M17.75 3h3.07l-6.7 7.66L22 21h-6.17l-4.83-6.32L5.46 21H2.39l7.17-8.2L2 3h6.33l4.37 5.78L17.75 3Zm-1.08 16.18h1.7L7.4 4.73H5.58l11.09 14.45Z" />
  </Svg>
);
const FacebookIcon = () => (
  <Svg>
    <path d="M13.5 21v-7.5h2.53l.38-2.94H13.5V8.69c0-.85.24-1.43 1.46-1.43h1.56V4.63A21 21 0 0 0 14.25 4.5c-2.25 0-3.79 1.37-3.79 3.9v2.16H7.92v2.94h2.54V21h3.04Z" />
  </Svg>
);
const InstagramIcon = () => (
  <Svg>
    <path d="M12 7.35A4.65 4.65 0 1 0 12 16.65 4.65 4.65 0 0 0 12 7.35Zm0 7.67A3.02 3.02 0 1 1 12 8.98a3.02 3.02 0 0 1 0 6.04Zm5.92-7.86a1.08 1.08 0 1 1-2.17 0 1.08 1.08 0 0 1 2.17 0ZM21 8.26c-.07-1.46-.4-2.75-1.47-3.8C18.47 3.4 17.18 3.07 15.72 3c-1.5-.09-6-.09-7.5 0-1.45.07-2.74.4-3.8 1.46S3.07 6.8 3 8.26c-.09 1.5-.09 6 0 7.5.07 1.45.4 2.74 1.46 3.8 1.06 1.05 2.35 1.38 3.8 1.46 1.5.08 6 .08 7.5 0 1.46-.07 2.75-.4 3.8-1.46 1.06-1.06 1.39-2.35 1.47-3.8.08-1.5.08-6 0-7.5Zm-1.94 9.11a3.07 3.07 0 0 1-1.73 1.73c-1.2.48-4.04.37-5.36.37s-4.17.1-5.36-.37a3.07 3.07 0 0 1-1.73-1.73c-.48-1.2-.37-4.04-.37-5.36s-.1-4.17.37-5.36A3.07 3.07 0 0 1 6.61 4.9c1.2-.48 4.04-.37 5.36-.37s4.17-.1 5.36.37c.8.32 1.41.93 1.73 1.73.48 1.2.37 4.04.37 5.36s.11 4.17-.37 5.36Z" />
  </Svg>
);
const TikTokIcon = () => (
  <Svg>
    <path d="M16.6 5.82A4.28 4.28 0 0 1 15.54 3h-3.09v12.4a2.59 2.59 0 0 1-2.59 2.5 2.6 2.6 0 0 1-2.6-2.6 2.6 2.6 0 0 1 3.37-2.48V9.66a5.73 5.73 0 0 0-.77-.05 5.7 5.7 0 0 0-5.69 5.69A5.7 5.7 0 0 0 9.86 21a5.7 5.7 0 0 0 5.69-5.7V8.97a7.35 7.35 0 0 0 4.3 1.38V7.26a4.3 4.3 0 0 1-3.25-1.44Z" />
  </Svg>
);
const WhatsAppIcon = () => (
  <Svg>
    <path d="M19.05 4.91A9.82 9.82 0 0 0 12.04 2a9.91 9.91 0 0 0-8.59 14.86L2 22l5.25-1.38a9.9 9.9 0 0 0 4.79 1.22 9.92 9.92 0 0 0 7.01-16.93Zm-7.01 15.24a8.23 8.23 0 0 1-4.2-1.15l-.3-.18-3.12.82.83-3.04-.2-.31a8.24 8.24 0 1 1 6.99 3.86Zm4.52-6.17c-.25-.12-1.47-.72-1.69-.81-.23-.08-.39-.12-.56.13-.17.25-.64.8-.78.97-.15.17-.29.19-.54.06a6.75 6.75 0 0 1-3.37-2.94c-.25-.44.25-.41.72-1.36.08-.17.04-.31-.02-.43-.06-.13-.56-1.35-.77-1.84-.2-.48-.41-.42-.56-.42h-.48a.92.92 0 0 0-.66.31 2.79 2.79 0 0 0-.87 2.07 4.84 4.84 0 0 0 1.01 2.56 11.08 11.08 0 0 0 4.23 3.74c1.58.68 2.2.74 2.99.62.48-.07 1.47-.6 1.68-1.18.21-.58.21-1.08.14-1.18-.06-.11-.22-.17-.47-.3Z" />
  </Svg>
);

type Platform = {
  key: string;
  label: string;
  bg: string;
  icon: ReactNode;
  /** Web share intent; platforms without one (Instagram, TikTok) copy + open the app. */
  href?: (url: string, text: string) => string;
  appUrl?: string;
};

const PLATFORMS: Platform[] = [
  { key: "telegram", label: "Telegram", bg: "bg-[#229ED9]", icon: <TelegramIcon />, href: (u, t) => `https://t.me/share/url?url=${encodeURIComponent(u)}&text=${encodeURIComponent(t)}` },
  { key: "instagram", label: "Instagram", bg: "bg-gradient-to-br from-[#feda75] via-[#d62976] to-[#4f5bd5]", icon: <InstagramIcon />, appUrl: "https://www.instagram.com/" },
  { key: "tiktok", label: "TikTok", bg: "bg-black ring-1 ring-white/15", icon: <TikTokIcon />, appUrl: "https://www.tiktok.com/" },
  { key: "x", label: "X (Twitter)", bg: "bg-black ring-1 ring-white/15", icon: <XIcon />, href: (u, t) => `https://x.com/intent/post?url=${encodeURIComponent(u)}&text=${encodeURIComponent(t)}` },
  { key: "facebook", label: "Facebook", bg: "bg-[#1877F2]", icon: <FacebookIcon />, href: (u) => `https://www.facebook.com/sharer/sharer.php?u=${encodeURIComponent(u)}` },
  { key: "whatsapp", label: "WhatsApp", bg: "bg-[#25D366]", icon: <WhatsAppIcon />, href: (u, t) => `https://wa.me/?text=${encodeURIComponent(`${t}\n${u}`)}` },
];

async function copyText(text: string): Promise<boolean> {
  try {
    await navigator.clipboard.writeText(text);
    return true;
  } catch {
    try {
      const input = document.createElement("textarea");
      input.value = text;
      document.body.appendChild(input);
      input.select();
      document.execCommand("copy");
      document.body.removeChild(input);
      return true;
    } catch {
      return false;
    }
  }
}

function withRef(url: string, code?: string | null): string {
  if (!code) return url;
  try {
    const u = new URL(url);
    u.searchParams.set("ref", code);
    return u.toString();
  } catch {
    return url;
  }
}

/**
 * "Ulashish" button + share sheet for movies, series and episodes.
 * Movies/series get a tracked share link from the backend; signed-in users'
 * links also carry their referral code, so friends who sign up through it are
 * credited to them ("shaxsiy havola").
 */
export default function ShareSheet({
  kind,
  id,
  title,
  path,
  posterUrl,
  subtitle,
  className,
}: {
  kind: Kind;
  id: string;
  title: string;
  /** Canonical page path, e.g. /movies/slug — used when no tracked link can be made. */
  path: string;
  posterUrl?: string;
  subtitle?: string;
  className?: string;
}) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <button
        type="button"
        onClick={() => setOpen(true)}
        className={
          className ||
          "inline-flex items-center gap-2 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-white transition-colors glass-card hover:border-brand-red"
        }
      >
        <Share2 size={17} />
        Ulashish
      </button>
      {open && <ShareDialog kind={kind} id={id} title={title} path={path} posterUrl={posterUrl} subtitle={subtitle} onClose={() => setOpen(false)} />}
    </>
  );
}

function ShareDialog({
  kind,
  id,
  title,
  path,
  posterUrl,
  subtitle,
  onClose,
}: {
  kind: Kind;
  id: string;
  title: string;
  path: string;
  posterUrl?: string;
  subtitle?: string;
  onClose: () => void;
}) {
  const { token } = useAuth();
  const canonical = `${SITE_URL}${path.startsWith("/") ? path : `/${path}`}`;
  const [url, setUrl] = useState<string>(canonical);
  const [preparing, setPreparing] = useState(true);
  const [personal, setPersonal] = useState(false);
  const [copied, setCopied] = useState(false);
  const [hint, setHint] = useState<string | null>(null);
  const [canNativeShare, setCanNativeShare] = useState(false);

  const emoji = kind === "movie" ? "🎬" : "📺";
  const text = `${emoji} ${title}${subtitle ? ` — ${subtitle}` : ""}\nFILMORAUZ'da o'zbek tilida tomosha qiling`;

  useEffect(() => {
    setCanNativeShare(typeof navigator !== "undefined" && typeof navigator.share === "function");
    let alive = true;
    (async () => {
      let link = canonical;
      try {
        if (kind === "movie") link = (await createMovieShare(token, id)).share_url || link;
        else if (kind === "series") link = (await createSeriesShare(token, id)).share_url || link;
      } catch {
        // tracked link is a nice-to-have; the canonical URL works too
      }
      let code: string | null = null;
      if (token) {
        try {
          code = (await getMyReferral(token)).code || null;
        } catch {
          code = null;
        }
      }
      if (!alive) return;
      setUrl(withRef(link, code));
      setPersonal(!!code);
      setPreparing(false);
    })();
    return () => {
      alive = false;
    };
  }, [kind, id, token, canonical]);

  useEffect(() => {
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    const onKey = (e: KeyboardEvent) => e.key === "Escape" && onClose();
    window.addEventListener("keydown", onKey);
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener("keydown", onKey);
    };
  }, [onClose]);

  const copy = useCallback(async () => {
    if (await copyText(url)) {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [url]);

  const nativeShare = useCallback(async () => {
    try {
      await navigator.share({ title, text, url });
      return true;
    } catch {
      return false;
    }
  }, [title, text, url]);

  const onPlatform = async (p: Platform) => {
    if (p.href) {
      window.open(p.href(url, text), "_blank", "noopener,noreferrer,width=640,height=640");
      return;
    }
    // Instagram / TikTok have no web share link: use the phone's share sheet
    // when there is one, otherwise copy the link and open the site.
    if (canNativeShare && (await nativeShare())) return;
    await copyText(`${text}\n${url}`);
    setHint(`Havola nusxalandi — ${p.label}'da story, post yoki xabarga joylashtiring`);
    setTimeout(() => setHint(null), 4000);
    if (p.appUrl) window.open(p.appUrl, "_blank", "noopener,noreferrer");
  };

  return (
    <div className="fixed inset-0 z-[200] flex items-end justify-center sm:items-center sm:p-4" role="dialog" aria-modal="true" aria-label="Ulashish">
      <div className="absolute inset-0 animate-[fadeIn_.15s_ease-out] bg-black/75" onClick={onClose} />
      <div className="relative w-full max-w-md animate-[sheetUp_.22s_ease-out] overflow-hidden rounded-t-3xl border border-white/10 bg-[#0f0f16] shadow-2xl sm:rounded-3xl">
        {/* Header with preview */}
        <div className="relative overflow-hidden border-b border-white/5 p-5">
          {posterUrl && (
            <div className="absolute inset-0 scale-110 bg-cover bg-center opacity-25 blur-2xl" style={{ backgroundImage: `url(${normalizeMediaUrl(posterUrl)})` }} aria-hidden />
          )}
          <div className="relative flex items-center gap-4">
            {posterUrl && (
              <MediaImage src={normalizeMediaUrl(posterUrl)} alt="" className="h-20 w-14 shrink-0 rounded-xl border border-white/10 object-cover shadow-lg" />
            )}
            <div className="min-w-0 flex-1">
              <p className="text-xs font-medium uppercase tracking-wider text-orange-300/90">Do&apos;stlarga ulashish</p>
              <h2 className="line-clamp-2 font-semibold leading-tight text-white">{title}</h2>
              {subtitle && <p className="truncate text-xs text-gray-400">{subtitle}</p>}
            </div>
            <button type="button" onClick={onClose} className="self-start rounded-lg p-1.5 text-gray-400 hover:bg-white/10 hover:text-white" aria-label="Yopish">
              <X size={18} />
            </button>
          </div>
        </div>

        <div className="space-y-5 p-5">
          {/* Platforms */}
          <div className="grid grid-cols-3 gap-3 sm:grid-cols-6">
            {PLATFORMS.map((p) => (
              <button
                key={p.key}
                type="button"
                disabled={preparing}
                onClick={() => void onPlatform(p)}
                className="group flex flex-col items-center gap-1.5 disabled:opacity-50"
                title={p.label}
              >
                <span className={`flex h-12 w-12 items-center justify-center rounded-2xl text-white shadow-lg transition group-hover:-translate-y-0.5 group-hover:scale-105 ${p.bg}`}>
                  {p.icon}
                </span>
                <span className="text-[11px] text-gray-400 group-hover:text-white">{p.label.replace(" (Twitter)", "")}</span>
              </button>
            ))}
          </div>

          {hint && <p className="rounded-xl bg-emerald-500/10 px-3 py-2 text-center text-xs text-emerald-300">{hint}</p>}

          {/* Personal link */}
          <div>
            <div className="mb-1.5 flex items-center justify-between gap-2">
              <span className="inline-flex items-center gap-1.5 text-xs font-medium text-gray-400">
                <Link2 size={13} /> {personal ? "Shaxsiy havolangiz" : "Havola"}
              </span>
              {personal && (
                <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-[10px] font-medium text-amber-300" title="Do'stingiz shu havola orqali ro'yxatdan o'tsa, sizga bonus beriladi">
                  <Gift size={11} /> Bonus havola
                </span>
              )}
            </div>
            <div className="flex items-center gap-2 rounded-2xl border border-white/10 bg-black/30 p-1.5 pl-3">
              {preparing ? (
                <span className="flex flex-1 items-center gap-2 text-sm text-gray-500">
                  <Loader2 size={14} className="animate-spin" /> Havola tayyorlanmoqda...
                </span>
              ) : (
                <input readOnly value={url} onFocus={(e) => e.currentTarget.select()} className="min-w-0 flex-1 bg-transparent text-sm text-gray-200 outline-none" aria-label="Ulashish havolasi" />
              )}
              <button
                type="button"
                onClick={() => void copy()}
                disabled={preparing}
                className={`inline-flex shrink-0 items-center gap-1.5 rounded-xl px-3.5 py-2 text-sm font-semibold text-white transition disabled:opacity-50 ${
                  copied ? "bg-emerald-600" : "bg-orange-500 hover:bg-orange-400"
                }`}
              >
                {copied ? <Check size={15} /> : <Copy size={15} />}
                {copied ? "Nusxalandi" : "Nusxalash"}
              </button>
            </div>
            {personal && <p className="mt-1.5 text-[11px] text-gray-500">Do&apos;stingiz shu havola orqali ro&apos;yxatdan o&apos;tsa, sizga Premium kunlar beriladi.</p>}
          </div>

          {canNativeShare && (
            <button
              type="button"
              disabled={preparing}
              onClick={() => void nativeShare()}
              className="flex w-full items-center justify-center gap-2 rounded-2xl border border-white/10 bg-white/[0.04] py-3 text-sm font-medium text-white hover:bg-white/[0.08] disabled:opacity-50"
            >
              <Share2 size={16} /> Boshqa ilovalar orqali
            </button>
          )}
        </div>
      </div>
    </div>
  );
}
