"use client";

import { useMemo, useState } from "react";
import Link from "next/link";
import { History, Trash2, Loader2, Check } from "lucide-react";
import { clearWatchHistory, deleteHistoryEntry, WatchHistoryItem } from "@/lib/api";
import OptimizedImage from "@/components/OptimizedImage";

function hrefFor(item: WatchHistoryItem): string {
  if (item.target_type === "episode" || item.type === "episode") {
    return `/episode/${item.episode_id || item.target_id}`;
  }
  return item.slug ? `/movies/${item.slug}` : "#";
}

function titleFor(item: WatchHistoryItem): string {
  if (item.series_title) {
    const code = item.season_number && item.episode_number ? ` · ${item.season_number}-fasl ${item.episode_number}-qism` : "";
    return `${item.series_title}${code}`;
  }
  return item.title;
}

function dayLabel(iso: string): string {
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "Avvalroq";
  const today = new Date();
  const start = (x: Date) => new Date(x.getFullYear(), x.getMonth(), x.getDate()).getTime();
  const diff = Math.round((start(today) - start(d)) / 86400000);
  if (diff <= 0) return "Bugun";
  if (diff === 1) return "Kecha";
  if (diff < 7) return "Shu hafta";
  if (diff < 31) return "Shu oy";
  return "Avvalroq";
}

interface Props {
  token: string;
  items: WatchHistoryItem[];
  onChange: (items: WatchHistoryItem[]) => void;
}

/** Watch history grouped by day, with per-item delete and "clear all". */
export default function WatchHistoryList({ token, items, onChange }: Props) {
  const [busy, setBusy] = useState<string | null>(null);
  const [confirmClear, setConfirmClear] = useState(false);
  const [showAll, setShowAll] = useState(false);

  const visible = showAll ? items : items.slice(0, 24);
  const groups = useMemo(() => {
    const out: { label: string; items: WatchHistoryItem[] }[] = [];
    for (const it of visible) {
      const label = dayLabel(it.watched_at);
      const last = out[out.length - 1];
      if (last && last.label === label) last.items.push(it);
      else out.push({ label, items: [it] });
    }
    return out;
  }, [visible]);

  const remove = async (item: WatchHistoryItem) => {
    const type = item.target_type === "episode" ? "episode" : "movie";
    const id = type === "episode" ? item.episode_id || item.target_id : item.movie_id || item.target_id;
    const key = item.record_id || item.id;
    setBusy(key);
    try {
      await deleteHistoryEntry(token, type, id);
      onChange(items.filter((i) => (i.record_id || i.id) !== key));
    } catch {
      // keep the row
    } finally {
      setBusy(null);
    }
  };

  const clearAll = async () => {
    setBusy("all");
    try {
      await clearWatchHistory(token);
      onChange([]);
      setConfirmClear(false);
    } catch {
      // keep
    } finally {
      setBusy(null);
    }
  };

  if (items.length === 0) {
    return (
      <div className="py-10 text-center glass-card rounded-xl border border-white/5">
        <History className="w-8 h-8 text-gray-600 mx-auto mb-2" />
        <p className="text-gray-500 text-sm">Ko&apos;rish tarixi bo&apos;sh</p>
        <p className="text-gray-600 text-xs mt-1">Tomosha qilgan kinolaringiz shu yerda chiqadi</p>
      </div>
    );
  }

  return (
    <div>
      <div className="mb-4 flex flex-wrap items-center justify-between gap-2">
        <p className="text-sm text-gray-500">{items.length} ta yozuv</p>
        {confirmClear ? (
          <div className="flex items-center gap-2 text-sm">
            <span className="text-gray-300">Butun tarix o&apos;chirilsinmi?</span>
            <button
              onClick={clearAll}
              disabled={busy === "all"}
              className="inline-flex items-center gap-1 rounded-lg bg-red-600 px-3 py-1.5 text-white hover:bg-red-500 disabled:opacity-50"
            >
              {busy === "all" && <Loader2 size={13} className="animate-spin" />} Ha, o&apos;chirish
            </button>
            <button onClick={() => setConfirmClear(false)} className="rounded-lg px-3 py-1.5 text-gray-400 hover:text-white">
              Bekor
            </button>
          </div>
        ) : (
          <button
            onClick={() => setConfirmClear(true)}
            className="inline-flex items-center gap-1.5 rounded-lg border border-white/10 px-3 py-1.5 text-sm text-gray-400 hover:border-red-500/40 hover:text-red-400"
          >
            <Trash2 size={14} /> Tarixni tozalash
          </button>
        )}
      </div>

      <div className="space-y-6">
        {groups.map((g) => (
          <div key={g.label}>
            <h3 className="mb-2 text-xs font-semibold uppercase tracking-wider text-gray-500">{g.label}</h3>
            <ul className="divide-y divide-white/5 overflow-hidden rounded-xl border border-white/5 glass-card">
              {g.items.map((item) => {
                const key = item.record_id || item.id;
                const pct = Math.round(item.progress_percent || 0);
                const done = item.completed || pct >= 90;
                return (
                  <li key={key} className="flex items-center gap-3 p-2.5 sm:p-3">
                    <Link href={hrefFor(item)} className="relative block w-12 shrink-0 overflow-hidden rounded-md sm:w-14">
                      <OptimizedImage src={item.poster_url} alt={item.title} aspectRatio="2/3" />
                    </Link>
                    <div className="min-w-0 flex-1">
                      <Link href={hrefFor(item)} className="line-clamp-1 text-sm font-medium text-white hover:text-brand-red">
                        {titleFor(item)}
                      </Link>
                      <div className="mt-1 flex items-center gap-2 text-xs text-gray-500">
                        {done ? (
                          <span className="inline-flex items-center gap-1 text-emerald-400">
                            <Check size={12} /> Ko&apos;rildi
                          </span>
                        ) : pct > 0 ? (
                          <>
                            <span className="h-1 w-20 overflow-hidden rounded-full bg-white/10">
                              <span className="block h-full bg-brand-red" style={{ width: `${pct}%` }} />
                            </span>
                            {pct}%
                          </>
                        ) : (
                          <span>Ochilgan</span>
                        )}
                        {item.year ? <span>· {item.year}</span> : null}
                      </div>
                    </div>
                    <button
                      onClick={() => remove(item)}
                      disabled={busy === key}
                      className="rounded-lg p-2 text-gray-500 hover:bg-red-500/10 hover:text-red-400 disabled:opacity-50"
                      aria-label={`${titleFor(item)} ni tarixdan o'chirish`}
                      title="Tarixdan o'chirish"
                    >
                      {busy === key ? <Loader2 size={15} className="animate-spin" /> : <Trash2 size={15} />}
                    </button>
                  </li>
                );
              })}
            </ul>
          </div>
        ))}
      </div>

      {items.length > visible.length && (
        <button
          onClick={() => setShowAll(true)}
          className="mt-4 w-full rounded-xl border border-white/10 py-2.5 text-sm text-gray-300 hover:bg-white/5"
        >
          Hammasini ko&apos;rsatish ({items.length})
        </button>
      )}
    </div>
  );
}
