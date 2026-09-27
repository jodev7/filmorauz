"use client";

import { Crown, Eye } from "lucide-react";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { localizeSingleGenre } from "@/lib/localization";
import MediaImage from "@/components/ui/MediaImage";

/** How the title will look in the catalogue — updates as the form changes. */
export default function ContentPreviewCard({
  title,
  year,
  genres,
  posterUrl,
  backdropUrl,
  quality,
  isPremium,
  kindLabel,
  href,
  checklist,
}: {
  title: string;
  year?: number;
  genres: string[];
  posterUrl?: string;
  backdropUrl?: string;
  quality?: string;
  isPremium?: boolean;
  kindLabel: string;
  href?: string;
  checklist: { label: string; ok: boolean }[];
}) {
  const done = checklist.filter((c) => c.ok).length;
  return (
    <div className="overflow-hidden rounded-2xl border border-white/10 bg-[#12121a]">
      <div className="relative aspect-video bg-black/40">
        {backdropUrl ? (
          <MediaImage src={normalizeMediaUrl(backdropUrl)} alt="" className="absolute inset-0 h-full w-full object-cover opacity-70" />
        ) : null}
        <div className="absolute inset-0 bg-gradient-to-t from-[#12121a] via-[#12121a]/40 to-transparent" />
        <div className="absolute bottom-3 left-3 right-3 flex items-end gap-3">
          <div className="relative aspect-[2/3] w-20 shrink-0 overflow-hidden rounded-lg border border-white/10 bg-white/5 shadow-xl">
            {posterUrl ? <MediaImage src={normalizeMediaUrl(posterUrl)} alt="" className="h-full w-full object-cover" /> : null}
            {quality && <span className="absolute right-1 top-1 rounded bg-orange-500 px-1 text-[9px] font-bold text-white">{quality}</span>}
          </div>
          <div className="min-w-0 pb-1">
            <p className="text-[10px] uppercase tracking-wider text-gray-400">{kindLabel}</p>
            <p className="line-clamp-2 font-display text-xl leading-tight tracking-wide text-white">{title || "Nomi…"}</p>
            <p className="mt-1 flex flex-wrap items-center gap-x-2 text-[11px] text-gray-400">
              {year ? <span>{year}</span> : null}
              {genres.slice(0, 3).map((g) => (
                <span key={g}>{localizeSingleGenre(g)}</span>
              ))}
              {isPremium && (
                <span className="inline-flex items-center gap-0.5 text-yellow-400">
                  <Crown size={10} /> Premium
                </span>
              )}
            </p>
          </div>
        </div>
      </div>
      <div className="p-4">
        <div className="mb-2 flex items-center justify-between text-xs">
          <span className="font-medium text-gray-300">To&apos;ldirilganlik</span>
          <span className={done === checklist.length ? "text-emerald-400" : "text-gray-500"}>
            {done}/{checklist.length}
          </span>
        </div>
        <div className="mb-3 h-1.5 overflow-hidden rounded-full bg-white/10">
          <div className="h-full rounded-full bg-emerald-500 transition-[width]" style={{ width: `${(done / Math.max(1, checklist.length)) * 100}%` }} />
        </div>
        <ul className="space-y-1.5">
          {checklist.map((c) => (
            <li key={c.label} className={`flex items-center gap-2 text-xs ${c.ok ? "text-gray-400" : "text-amber-300"}`}>
              <span className={`flex h-4 w-4 items-center justify-center rounded-full text-[10px] ${c.ok ? "bg-emerald-500/20 text-emerald-400" : "bg-amber-500/15"}`}>
                {c.ok ? "✓" : "!"}
              </span>
              {c.label}
            </li>
          ))}
        </ul>
        {href && (
          <a href={href} target="_blank" rel="noreferrer" className="mt-4 inline-flex items-center gap-1.5 text-xs text-orange-400 hover:underline">
            <Eye size={13} /> Saytda ko&apos;rish
          </a>
        )}
      </div>
    </div>
  );
}
