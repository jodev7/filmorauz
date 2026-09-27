"use client";

import { useState } from "react";
import { Plus, X } from "lucide-react";
import { localizeSingleGenre } from "@/lib/localization";
import { inputCls } from "./ui";

export const GENRE_OPTIONS = [
  "action", "adventure", "animation", "anime", "comedy", "crime", "documentary", "dorama",
  "drama", "family", "fantasy", "history", "horror", "musical", "mystery", "romance",
  "sci-fi", "sport", "thriller", "war", "western",
];

export function normalizeGenre(value: string): string {
  const n = value.trim().toLowerCase().replace(/[_\s]+/g, "-").replace(/-+/g, "-");
  if (n === "science-fiction" || n === "sciencefiction" || n === "scifi") return "sci-fi";
  return n;
}

/** Toggle chips for known genres + free-form custom ones. */
export default function GenrePicker({ value, onChange }: { value: string[]; onChange: (v: string[]) => void }) {
  const [custom, setCustom] = useState("");
  const selected = new Set(value.map(normalizeGenre));
  const toggle = (g: string) => onChange(selected.has(g) ? value.filter((x) => normalizeGenre(x) !== g) : [...value, g]);
  const add = () => {
    const g = normalizeGenre(custom);
    if (g && !selected.has(g)) onChange([...value, g]);
    setCustom("");
  };
  const extras = value.map(normalizeGenre).filter((g) => !GENRE_OPTIONS.includes(g));

  return (
    <div>
      <div className="flex flex-wrap gap-1.5">
        {GENRE_OPTIONS.map((g) => {
          const on = selected.has(g);
          return (
            <button
              key={g}
              type="button"
              aria-pressed={on}
              onClick={() => toggle(g)}
              className={`rounded-full border px-3 py-1.5 text-xs transition-colors ${
                on ? "border-orange-500 bg-orange-500 text-white" : "border-white/10 text-gray-400 hover:border-white/25 hover:text-white"
              }`}
            >
              {localizeSingleGenre(g)}
            </button>
          );
        })}
        {extras.map((g) => (
          <span key={g} className="inline-flex items-center gap-1 rounded-full border border-orange-500 bg-orange-500 px-3 py-1.5 text-xs text-white">
            {localizeSingleGenre(g)}
            <button type="button" onClick={() => toggle(g)} aria-label={`${g} ni olib tashlash`} className="opacity-80 hover:opacity-100">
              <X size={11} />
            </button>
          </span>
        ))}
      </div>
      <div className="mt-2 flex gap-2">
        <input
          value={custom}
          onChange={(e) => setCustom(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              add();
            }
          }}
          placeholder="Boshqa janr qo'shish…"
          className={`${inputCls} py-2`}
        />
        <button type="button" onClick={add} className="rounded-xl border border-white/10 px-3 text-gray-300 hover:bg-white/5" aria-label="Janr qo'shish">
          <Plus size={16} />
        </button>
      </div>
    </div>
  );
}
