"use client";

import { useEffect, useRef, useState } from "react";
import { Link2, Lock, Unlock } from "lucide-react";
import { cleanSlugInput, slugify } from "@/lib/slugify";
import { inputCls } from "./ui";

/**
 * Slug that follows the title automatically until the admin edits it by
 * hand (or unlocks it). Existing content starts locked so a title tweak
 * never silently changes a live URL.
 */
export default function SlugInput({
  value,
  title,
  onChange,
  prefix,
  lockedInitially = false,
  id,
}: {
  value: string;
  title: string;
  onChange: (v: string) => void;
  prefix: string;
  lockedInitially?: boolean;
  id?: string;
}) {
  const [auto, setAuto] = useState(!lockedInitially && !value);
  const first = useRef(true);

  useEffect(() => {
    if (first.current) {
      first.current = false;
      return;
    }
    if (auto) onChange(slugify(title));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [title, auto]);

  return (
    <div>
      <div className="flex items-stretch overflow-hidden rounded-xl border border-white/10 bg-black/30 focus-within:border-orange-500 focus-within:ring-2 focus-within:ring-orange-500/20">
        <span className="hidden items-center gap-1 border-r border-white/10 px-3 text-xs text-gray-500 sm:flex">
          <Link2 size={13} /> {prefix}
        </span>
        <input
          id={id}
          value={value}
          onChange={(e) => {
            setAuto(false);
            onChange(cleanSlugInput(e.target.value));
          }}
          placeholder="avtomatik-yaratiladi"
          className={`${inputCls} rounded-none border-0 bg-transparent font-mono focus:ring-0`}
        />
        <button
          type="button"
          onClick={() => {
            const next = !auto;
            setAuto(next);
            if (next) onChange(slugify(title));
          }}
          title={auto ? "Nomdan avtomatik — qo'lda tahrirlash uchun bosing" : "Nomdan avtomatik yaratish"}
          className={`flex items-center gap-1 border-l border-white/10 px-3 text-xs ${auto ? "text-emerald-400" : "text-gray-500 hover:text-white"}`}
        >
          {auto ? <Unlock size={13} /> : <Lock size={13} />}
          <span className="hidden sm:inline">{auto ? "Avto" : "Qo'lda"}</span>
        </button>
      </div>
    </div>
  );
}
