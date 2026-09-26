"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Bookmark, BellRing, X, Crown } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getWatchlist,
  getSeriesSubscriptions,
  removeFromWatchlist,
  unsubscribeSeries,
  LibraryItem,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

function itemHref(item: LibraryItem): string {
  return item.target_type === "series" ? `/series/${item.slug}` : `/movies/${item.slug}`;
}

function LibraryRow({
  items,
  onRemove,
  removeLabel,
}: {
  items: LibraryItem[];
  onRemove: (item: LibraryItem) => void;
  removeLabel: string;
}) {
  return (
    <div className="flex gap-3 overflow-x-auto pb-2 -mx-1 px-1 snap-x">
      {items.map((item) => (
        <div key={`${item.target_type}:${item.target_id}`} className="group relative w-32 sm:w-36 shrink-0 snap-start">
          <Link href={itemHref(item)} className="block">
            <div className="relative aspect-[2/3] overflow-hidden rounded-lg border border-white/5 bg-brand-card">
              <MediaImage
                src={normalizeMediaUrl(item.poster_url)}
                alt={item.title_uz || item.title}
                className="h-full w-full object-cover transition-transform group-hover:scale-105"
              />
              {item.is_premium && (
                <span className="absolute left-1.5 top-1.5 rounded bg-yellow-500/90 px-1.5 py-0.5 text-[10px] font-bold text-black inline-flex items-center gap-0.5">
                  <Crown size={10} /> PREMIUM
                </span>
              )}
              {item.target_type === "series" && (
                <span className="absolute bottom-1.5 left-1.5 rounded bg-black/70 px-1.5 py-0.5 text-[10px] text-white">Serial</span>
              )}
            </div>
            <p className="mt-1.5 truncate text-sm text-white">{item.title_uz || item.title}</p>
            {item.year ? <p className="text-xs text-gray-500">{item.year}</p> : null}
          </Link>
          <button
            onClick={() => onRemove(item)}
            className="absolute right-1.5 top-1.5 rounded-full bg-black/70 p-1 text-gray-300 opacity-100 sm:opacity-0 transition-opacity group-hover:opacity-100 hover:text-white"
            aria-label={`${item.title} — ${removeLabel}`}
            title={removeLabel}
          >
            <X size={14} />
          </button>
        </div>
      ))}
    </div>
  );
}

/** "Keyinroq ko'raman" and followed-series sections for the profile page. */
export default function UserLibrarySections() {
  const { token } = useAuth();
  const [watchlist, setWatchlist] = useState<LibraryItem[] | null>(null);
  const [subs, setSubs] = useState<LibraryItem[] | null>(null);

  useEffect(() => {
    if (!token) return;
    let cancelled = false;
    getWatchlist(token)
      .then((r) => !cancelled && setWatchlist(r.data || []))
      .catch(() => !cancelled && setWatchlist([]));
    getSeriesSubscriptions(token)
      .then((r) => !cancelled && setSubs(r.data || []))
      .catch(() => !cancelled && setSubs([]));
    return () => {
      cancelled = true;
    };
  }, [token]);

  if (!token) return null;

  const removeFromList = (item: LibraryItem) => {
    setWatchlist((prev) => (prev || []).filter((i) => !(i.target_id === item.target_id && i.target_type === item.target_type)));
    removeFromWatchlist(token, item.target_type, item.target_id).catch(() => {});
  };
  const unsubscribe = (item: LibraryItem) => {
    setSubs((prev) => (prev || []).filter((i) => i.target_id !== item.target_id));
    unsubscribeSeries(token, item.target_id).catch(() => {});
  };

  return (
    <>
      <section className="mb-10">
        <div className="flex items-center gap-3 mb-5">
          <div className="w-9 h-9 rounded-lg bg-emerald-500/10 border border-emerald-500/20 flex items-center justify-center">
            <Bookmark className="w-4 h-4 text-emerald-400" />
          </div>
          <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">KEYINROQ KO&apos;RAMAN</h2>
          {watchlist && watchlist.length > 0 && (
            <span className="px-2 py-0.5 bg-brand-border text-gray-400 text-xs rounded-full">{watchlist.length}</span>
          )}
        </div>
        {watchlist === null ? (
          <div className="h-48 rounded-xl glass-card animate-pulse" aria-hidden />
        ) : watchlist.length > 0 ? (
          <LibraryRow items={watchlist} onRemove={removeFromList} removeLabel="Ro'yxatdan olib tashlash" />
        ) : (
          <div className="py-10 text-center glass-card rounded-xl border border-white/5">
            <Bookmark className="w-8 h-8 text-gray-600 mx-auto mb-2" />
            <p className="text-gray-500 text-sm">Ro&apos;yxat hozircha bo&apos;sh</p>
            <p className="text-gray-600 text-xs mt-1">Kino yoki serial sahifasida &quot;Keyinroq ko&apos;raman&quot; tugmasini bosing</p>
          </div>
        )}
      </section>

      {subs && subs.length > 0 && (
        <section className="mb-10">
          <div className="flex items-center gap-3 mb-5">
            <div className="w-9 h-9 rounded-lg bg-purple-500/10 border border-purple-500/20 flex items-center justify-center">
              <BellRing className="w-4 h-4 text-purple-400" />
            </div>
            <h2 className="font-display text-xl sm:text-2xl text-white tracking-wide">OBUNA BO&apos;LGAN SERIALLAR</h2>
            <span className="px-2 py-0.5 bg-brand-border text-gray-400 text-xs rounded-full">{subs.length}</span>
          </div>
          <p className="-mt-3 mb-4 text-xs text-gray-500">Yangi qism chiqsa saytda va Telegram&apos;da xabar olasiz.</p>
          <LibraryRow items={subs} onRemove={unsubscribe} removeLabel="Obunani bekor qilish" />
        </section>
      )}
    </>
  );
}
