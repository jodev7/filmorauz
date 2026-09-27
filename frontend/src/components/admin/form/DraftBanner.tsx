"use client";

import { History } from "lucide-react";

export default function DraftBanner({ savedAt, onRestore, onDiscard }: { savedAt: number; onRestore: () => void; onDiscard: () => void }) {
  const d = new Date(savedAt);
  const when = `${d.toLocaleDateString("ru-RU")} ${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
  return (
    <div className="flex flex-wrap items-center gap-3 rounded-xl border border-sky-500/30 bg-sky-500/[0.08] px-4 py-3 text-sm">
      <History size={16} className="text-sky-300" />
      <span className="flex-1 text-sky-100">Saqlanmagan qoralama topildi ({when}). Davom ettirasizmi?</span>
      <button type="button" onClick={onRestore} className="rounded-lg bg-sky-500 px-3 py-1.5 text-xs font-semibold text-white hover:bg-sky-400">
        Tiklash
      </button>
      <button type="button" onClick={onDiscard} className="rounded-lg px-3 py-1.5 text-xs text-sky-200 hover:bg-white/5">
        O&apos;chirish
      </button>
    </div>
  );
}
