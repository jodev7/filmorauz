"use client";

import { Search } from "lucide-react";
import { openSearch } from "@/lib/search-overlay";

/** A search-box-looking button that opens the global search overlay. */
export default function SearchLauncher({ placeholder = "Kino, serial yoki aktyor qidiring…", className = "" }: { placeholder?: string; className?: string }) {
  return (
    <button
      type="button"
      onClick={() => openSearch()}
      className={`flex w-full items-center gap-3 rounded-2xl border border-white/10 bg-white/[0.04] px-4 py-3.5 text-left text-gray-400 transition-colors hover:border-orange-500/40 hover:text-gray-200 ${className}`}
    >
      <Search size={18} className="text-orange-400" />
      <span className="flex-1 truncate">{placeholder}</span>
      <kbd className="hidden rounded border border-white/15 px-1.5 text-[11px] text-gray-500 sm:block">/</kbd>
    </button>
  );
}
