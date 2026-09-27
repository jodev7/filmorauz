"use client";

import { useState } from "react";
import { X } from "lucide-react";

/**
 * Tag input: Enter or comma adds, Backspace on empty removes the last one,
 * pasting "a, b, c" adds all. Optional one-click suggestions.
 */
export default function ChipsInput({
  value,
  onChange,
  placeholder,
  suggestions = [],
  max = 30,
  id,
}: {
  value: string[];
  onChange: (v: string[]) => void;
  placeholder?: string;
  suggestions?: string[];
  max?: number;
  id?: string;
}) {
  const [text, setText] = useState("");
  const lower = new Set(value.map((v) => v.toLowerCase()));

  const addMany = (raw: string) => {
    const items = raw
      .split(/[,\n;]/)
      .map((s) => s.replace(/\s+/g, " ").trim())
      .filter(Boolean);
    if (!items.length) return;
    const next = [...value];
    for (const it of items) {
      if (!next.some((x) => x.toLowerCase() === it.toLowerCase()) && next.length < max) next.push(it);
    }
    onChange(next);
    setText("");
  };

  return (
    <div>
      <div className="flex min-h-[46px] flex-wrap items-center gap-1.5 rounded-xl border border-white/10 bg-black/30 px-2 py-1.5 focus-within:border-orange-500 focus-within:ring-2 focus-within:ring-orange-500/20">
        {value.map((v) => (
          <span key={v} className="inline-flex items-center gap-1 rounded-lg bg-white/10 px-2 py-1 text-xs text-white">
            {v}
            <button type="button" onClick={() => onChange(value.filter((x) => x !== v))} className="text-gray-400 hover:text-white" aria-label={`${v} ni olib tashlash`}>
              <X size={11} />
            </button>
          </span>
        ))}
        <input
          id={id}
          value={text}
          onChange={(e) => {
            const v = e.target.value;
            if (v.includes(",")) addMany(v);
            else setText(v);
          }}
          onKeyDown={(e) => {
            if (e.key === "Enter") {
              e.preventDefault();
              addMany(text);
            } else if (e.key === "Backspace" && !text && value.length) {
              onChange(value.slice(0, -1));
            }
          }}
          onBlur={() => addMany(text)}
          onPaste={(e) => {
            const t = e.clipboardData.getData("text");
            if (/[,\n;]/.test(t)) {
              e.preventDefault();
              addMany(t);
            }
          }}
          placeholder={value.length ? "" : placeholder}
          className="min-w-[120px] flex-1 bg-transparent px-1.5 py-1 text-sm text-white placeholder-gray-600 focus:outline-none"
        />
      </div>
      {suggestions.filter((s) => !lower.has(s.toLowerCase())).length > 0 && (
        <div className="mt-2 flex flex-wrap gap-1.5">
          {suggestions
            .filter((s) => !lower.has(s.toLowerCase()))
            .map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => addMany(s)}
                className="rounded-full border border-dashed border-white/15 px-2.5 py-1 text-[11px] text-gray-400 hover:border-orange-500/50 hover:text-white"
              >
                + {s}
              </button>
            ))}
        </div>
      )}
    </div>
  );
}
