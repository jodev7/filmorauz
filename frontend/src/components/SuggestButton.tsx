"use client";

import { Lightbulb } from "lucide-react";
import { openSuggestion } from "@/lib/suggestion-modal";

/** "Saytga qo'shishni so'rash" CTA (e.g. empty search results). */
export default function SuggestButton({ title, className = "" }: { title?: string; className?: string }) {
  return (
    <button
      type="button"
      onClick={() => openSuggestion({ title })}
      className={`inline-flex items-center gap-2 rounded-xl bg-gradient-to-r from-yellow-500 to-orange-600 px-5 py-3 text-sm font-semibold text-white shadow-lg shadow-orange-500/20 ${className}`}
    >
      <Lightbulb size={16} /> Saytga qo&apos;shishni so&apos;rash
    </button>
  );
}
