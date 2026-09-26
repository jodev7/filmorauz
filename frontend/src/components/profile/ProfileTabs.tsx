"use client";

import { useRef } from "react";
import { LayoutGrid, Bookmark, ListVideo, History, Settings } from "lucide-react";

export type ProfileTab = "overview" | "library" | "lists" | "history" | "settings";
export const PROFILE_TABS: ProfileTab[] = ["overview", "library", "lists", "history", "settings"];

const TABS: { key: ProfileTab; label: string; icon: React.ElementType }[] = [
  { key: "overview", label: "Umumiy", icon: LayoutGrid },
  { key: "library", label: "Kutubxona", icon: Bookmark },
  { key: "lists", label: "Ro'yxatlar", icon: ListVideo },
  { key: "history", label: "Tarix", icon: History },
  { key: "settings", label: "Sozlamalar", icon: Settings },
];

/** Sticky, horizontally scrollable tab bar for the profile page. */
export default function ProfileTabs({
  active,
  onChange,
  counts,
}: {
  active: ProfileTab;
  onChange: (t: ProfileTab) => void;
  counts?: { history?: number; favorites?: number };
}) {
  const anchor = useRef<HTMLDivElement>(null);
  const select = (t: ProfileTab) => {
    onChange(t);
    // When scrolled past the bar, jump back to the top of the tab content.
    const a = anchor.current;
    if (!a) return;
    const top = a.getBoundingClientRect().top + window.scrollY - 80;
    if (window.scrollY > top) window.scrollTo({ top: Math.max(0, top), behavior: "smooth" });
  };
  return (
    <>
    <div ref={anchor} aria-hidden="true" />
    <div className="sticky top-[72px] z-30 -mx-4 border-b border-white/10 bg-brand-dark/85 px-4 backdrop-blur sm:top-[88px]">
      <div role="tablist" aria-label="Profil bo'limlari" className="scrollbar-hide flex gap-1 overflow-x-auto">
        {TABS.map(({ key, label, icon: Icon }) => {
          const selected = key === active;
          const count = key === "history" ? counts?.history : key === "library" ? counts?.favorites : undefined;
          return (
            <button
              key={key}
              role="tab"
              aria-selected={selected}
              onClick={() => select(key)}
              className={`relative flex shrink-0 items-center gap-2 px-4 py-3.5 text-sm font-medium transition-colors ${
                selected ? "text-white" : "text-gray-400 hover:text-gray-200"
              }`}
            >
              <Icon size={16} className={selected ? "text-orange-400" : ""} />
              {label}
              {count ? <span className="rounded-full bg-white/10 px-1.5 text-[11px] text-gray-300">{count}</span> : null}
              {selected && <span className="absolute inset-x-3 bottom-0 h-0.5 rounded-full bg-orange-500" />}
            </button>
          );
        })}
      </div>
    </div>
    </>
  );
}
