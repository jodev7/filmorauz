"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useRouter } from "next/navigation";
import { Search, Film, Tv, User as UserIcon, CornerDownLeft, Loader2, type LucideIcon } from "lucide-react";
import { adminGlobalSearch, AdminSearchResult } from "@/lib/api";
import type { AdminNavGroup } from "./admin-nav";

type PaletteItem = {
  key: string;
  label: string;
  subtitle?: string;
  icon: LucideIcon;
  href: string;
  section: string;
};

const KIND_ICON: Record<AdminSearchResult["kind"], LucideIcon> = {
  movie: Film,
  series: Tv,
  user: UserIcon,
};

function resultHref(r: AdminSearchResult): string {
  switch (r.kind) {
    case "movie":
      return `/admin/movies/${r.id}/edit`;
    case "series":
      return `/admin/series/${r.id}/edit`;
    case "user":
      // /admin/users search also matches a MongoDB id exactly.
      return `/admin/users?search=${encodeURIComponent(r.id)}`;
  }
}

function normalize(s: string): string {
  return s.toLowerCase().replace(/[ʻʼ‘’`']/g, "'");
}

/**
 * Ctrl+K / ⌘K palette: jump to any admin page, or find a movie, series or
 * user by name/code from anywhere in the admin panel.
 */
export default function CommandPalette({
  token,
  nav,
  open,
  onOpenChange,
}: {
  token: string | null;
  nav: AdminNavGroup[];
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const router = useRouter();
  const inputRef = useRef<HTMLInputElement>(null);
  const listRef = useRef<HTMLUListElement>(null);
  const [query, setQuery] = useState("");
  const [remote, setRemote] = useState<AdminSearchResult[]>([]);
  const [searching, setSearching] = useState(false);
  const [activeIndex, setActiveIndex] = useState(0);

  // Global shortcut.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        onOpenChange(!open);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onOpenChange]);

  // Reset on open, focus the input.
  useEffect(() => {
    if (!open) return;
    setQuery("");
    setRemote([]);
    setActiveIndex(0);
    const t = setTimeout(() => inputRef.current?.focus(), 0);
    return () => clearTimeout(t);
  }, [open]);

  // Debounced remote search; aborts superseded requests.
  useEffect(() => {
    const q = query.trim();
    if (!open || !token || q.length < 2) {
      setRemote([]);
      setSearching(false);
      return;
    }
    const ctrl = new AbortController();
    setSearching(true);
    const t = setTimeout(() => {
      adminGlobalSearch(token, q, ctrl.signal)
        .then((r) => setRemote(r))
        .catch(() => {})
        .finally(() => {
          if (!ctrl.signal.aborted) setSearching(false);
        });
    }, 250);
    return () => {
      clearTimeout(t);
      ctrl.abort();
    };
  }, [query, open, token]);

  const items: PaletteItem[] = useMemo(() => {
    const q = normalize(query.trim());
    const pages: PaletteItem[] = [];
    for (const g of nav) {
      for (const i of g.items) {
        const hay = normalize(`${i.label} ${g.label} ${i.keywords ?? ""}`);
        if (!q || hay.includes(q)) {
          pages.push({ key: `page:${i.href}`, label: i.label, subtitle: g.label, icon: i.icon, href: i.href, section: "Sahifalar" });
        }
      }
    }
    const found: PaletteItem[] = remote.map((r) => ({
      key: `${r.kind}:${r.id}`,
      label: r.title || r.id,
      subtitle: r.subtitle,
      icon: KIND_ICON[r.kind],
      href: resultHref(r),
      section: "Natijalar",
    }));
    return [...found, ...pages.slice(0, q ? 8 : 30)];
  }, [nav, query, remote]);

  useEffect(() => {
    setActiveIndex((i) => Math.min(i, Math.max(0, items.length - 1)));
  }, [items.length]);

  // Keep the highlighted row in view.
  useEffect(() => {
    listRef.current?.querySelector<HTMLElement>(`[data-index="${activeIndex}"]`)?.scrollIntoView({ block: "nearest" });
  }, [activeIndex]);

  const go = useCallback(
    (item: PaletteItem | undefined) => {
      if (!item) return;
      onOpenChange(false);
      router.push(item.href);
    },
    [onOpenChange, router]
  );

  if (!open) return null;

  const onInputKey = (e: React.KeyboardEvent) => {
    if (e.key === "ArrowDown") {
      e.preventDefault();
      setActiveIndex((i) => (items.length ? (i + 1) % items.length : 0));
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      setActiveIndex((i) => (items.length ? (i - 1 + items.length) % items.length : 0));
    } else if (e.key === "Enter") {
      e.preventDefault();
      go(items[activeIndex]);
    } else if (e.key === "Escape") {
      e.preventDefault();
      onOpenChange(false);
    }
  };

  let lastSection = "";

  return (
    <div
      className="fixed inset-0 z-[100] flex items-start justify-center bg-black/60 px-4 pt-[12vh]"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onOpenChange(false);
      }}
      role="dialog"
      aria-modal="true"
      aria-label="Admin qidiruv"
    >
      <div className="w-full max-w-xl overflow-hidden rounded-xl border border-brand-border bg-brand-card shadow-2xl">
        <div className="flex items-center gap-3 border-b border-brand-border px-4">
          <Search size={16} className="text-gray-500 shrink-0" aria-hidden />
          <input
            ref={inputRef}
            value={query}
            onChange={(e) => {
              setQuery(e.target.value);
              setActiveIndex(0);
            }}
            onKeyDown={onInputKey}
            placeholder="Sahifa, kino, serial yoki foydalanuvchi..."
            className="h-12 flex-1 bg-transparent text-sm text-white placeholder-gray-500 focus:outline-none"
            role="combobox"
            aria-expanded="true"
            aria-controls="admin-palette-list"
            aria-activedescendant={items[activeIndex] ? `palette-${activeIndex}` : undefined}
          />
          {searching && <Loader2 size={15} className="animate-spin text-gray-500" aria-label="Qidirilmoqda" />}
          <kbd className="hidden sm:inline rounded border border-brand-border px-1.5 py-0.5 text-[10px] text-gray-500">Esc</kbd>
        </div>
        <ul id="admin-palette-list" ref={listRef} role="listbox" className="max-h-[50vh] overflow-y-auto py-2">
          {items.length === 0 ? (
            <li className="px-4 py-6 text-center text-sm text-gray-500">
              {searching ? "Qidirilmoqda..." : "Hech narsa topilmadi"}
            </li>
          ) : (
            items.map((item, index) => {
              const Icon = item.icon;
              const header = item.section !== lastSection ? item.section : null;
              lastSection = item.section;
              const active = index === activeIndex;
              return (
                <li key={item.key} role="presentation">
                  {header && (
                    <p className="px-4 pb-1 pt-2 text-[10px] font-semibold uppercase tracking-wider text-gray-600">{header}</p>
                  )}
                  <button
                    id={`palette-${index}`}
                    data-index={index}
                    role="option"
                    aria-selected={active}
                    onMouseMove={() => setActiveIndex(index)}
                    onClick={() => go(item)}
                    className={`flex w-full items-center gap-3 px-4 py-2 text-left text-sm ${
                      active ? "bg-white/[0.06] text-white" : "text-gray-300"
                    }`}
                  >
                    <Icon size={15} className="shrink-0 text-gray-500" aria-hidden />
                    <span className="flex-1 min-w-0 truncate">{item.label}</span>
                    {item.subtitle && <span className="shrink-0 truncate text-xs text-gray-500 max-w-[45%]">{item.subtitle}</span>}
                    {active && <CornerDownLeft size={13} className="shrink-0 text-gray-500" aria-hidden />}
                  </button>
                </li>
              );
            })
          )}
        </ul>
        <div className="flex items-center gap-4 border-t border-brand-border px-4 py-2 text-[11px] text-gray-600">
          <span>↑↓ tanlash</span>
          <span>Enter ochish</span>
          <span className="ml-auto">Ctrl+K</span>
        </div>
      </div>
    </div>
  );
}
