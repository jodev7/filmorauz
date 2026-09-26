"use client";

import { useState } from "react";
import Link from "next/link";
import { Film, Star, Tv, X } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { removeFromList, UserListDetail } from "@/lib/api";
import OptimizedImage from "@/components/OptimizedImage";

/** Items of a shared list; the owner can remove items in place. */
export default function ListItemsGrid({ list }: { list: UserListDetail }) {
  const { token, user } = useAuth();
  const [items, setItems] = useState(list.items);
  const isOwner = !!user && user.id === list.owner_id;

  const remove = async (type: "movie" | "series", id: string) => {
    if (!token) return;
    const prev = items;
    setItems((cur) => cur.filter((i) => !(i.target_type === type && i.target_id === id)));
    try {
      await removeFromList(token, list.id, type, id);
    } catch {
      setItems(prev);
    }
  };

  if (items.length === 0) {
    return (
      <div className="glass-card rounded-xl border border-white/5 py-14 text-center text-gray-400">
        Bu ro&apos;yxat hozircha bo&apos;sh.
        {isOwner && <p className="mt-1 text-sm text-gray-500">Kino sahifasidagi &quot;Ro&apos;yxatga&quot; tugmasi orqali qo&apos;shing.</p>}
      </div>
    );
  }

  return (
    <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:grid-cols-4 lg:grid-cols-6">
      {items.map((it) => {
        const href = it.target_type === "series" ? `/series/${it.slug}` : `/movies/${it.slug}`;
        return (
          <div key={`${it.target_type}-${it.target_id}`} className="group relative">
            <Link href={href} className="block">
              <div className="relative overflow-hidden rounded-xl border border-white/10 bg-white/5 transition-colors group-hover:border-orange-500/40">
                <OptimizedImage src={it.poster_url} alt={it.title} aspectRatio="2/3" className="transition-transform duration-500 group-hover:scale-105" />
                <span className="absolute left-2 top-2 inline-flex items-center gap-1 rounded bg-black/70 px-1.5 py-0.5 text-[10px] text-white">
                  {it.target_type === "series" ? <Tv size={10} /> : <Film size={10} />}
                  {it.target_type === "series" ? "Serial" : "Kino"}
                </span>
              </div>
              <p className="mt-2 line-clamp-1 text-sm font-medium text-white group-hover:text-orange-400">{it.title_uz || it.title}</p>
              <p className="flex items-center gap-2 text-xs text-gray-500">
                {it.year ? <span>{it.year}</span> : null}
                {it.rating_avg > 0 && (
                  <span className="inline-flex items-center gap-0.5 text-yellow-400">
                    <Star size={10} className="fill-yellow-400" /> {it.rating_avg.toFixed(1)}
                  </span>
                )}
              </p>
            </Link>
            {isOwner && (
              <button
                onClick={() => remove(it.target_type, it.target_id)}
                className="absolute right-2 top-2 flex h-7 w-7 items-center justify-center rounded-full bg-black/75 text-white opacity-100 hover:bg-red-600 sm:opacity-0 sm:group-hover:opacity-100"
                aria-label={`${it.title} ni ro'yxatdan olib tashlash`}
              >
                <X size={14} />
              </button>
            )}
          </div>
        );
      })}
    </div>
  );
}
