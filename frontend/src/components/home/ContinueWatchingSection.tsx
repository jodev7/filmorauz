"use client";

import Image from "next/image";
import { useEffect, useRef, useState } from "react";
import Link from "next/link";
import { Play } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { ContinueWatchingItem, getContinueWatching } from "@/lib/api";
import { useI18n } from "@/lib/i18n";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeImageSrc, SHIMMER_BLUR_DATA_URL } from "@/lib/image-utils";

export default function ContinueWatchingSection() {
  const { t } = useI18n();
  const { token, isAuthenticated } = useAuth();
  const [items, setItems] = useState<ContinueWatchingItem[]>([]);
  const [loading, setLoading] = useState(true);
  // Guards against React StrictMode / re-render double-fetches in dev.
  const fetchedTokenRef = useRef<string | null>(null);

  useEffect(() => {
    if (!isAuthenticated || !token) {
      setLoading(false);
      return;
    }

    if (fetchedTokenRef.current === token) return;
    fetchedTokenRef.current = token;

    let cancelled = false;
    getContinueWatching(token)
      .then((data) => {
        if (!cancelled) setItems(data);
      })
      .catch(console.error)
      .finally(() => {
        if (!cancelled) setLoading(false);
      });

    return () => {
      cancelled = true;
    };
  }, [token, isAuthenticated]);

  if (loading || items.length === 0) {
    return null;
  }

  return (
    <section className="max-w-7xl mx-auto px-4 py-12">
      <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
        {t("home.continueWatching")}
      </h2>
      <div className="flex gap-3 overflow-x-auto pb-2 scrollbar-hide">
        {items.map((item) => (
          <Link
            key={item.movie_id}
            href={`/watch/${item.slug}`}
            className="shrink-0 w-[160px] sm:w-[180px] group"
          >
            <div className="relative aspect-[2/3] overflow-hidden rounded-lg bg-brand-border">
              <Image
                src={normalizeImageSrc(item.poster_url, DEFAULT_POSTER_PLACEHOLDER)}
                alt={item.title}
                fill
                sizes="(max-width: 640px) 160px, 180px"
                placeholder="blur"
                blurDataURL={SHIMMER_BLUR_DATA_URL}
                className="object-cover group-hover:scale-105 transition-transform duration-300"
              />
              {/* Play overlay */}
              <div className="absolute inset-0 bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                <div className="w-12 h-12 bg-brand-red rounded-full flex items-center justify-center">
                  <Play size={20} className="text-white ml-1" fill="white" aria-hidden="true" />
                </div>
              </div>
              {/* Progress bar */}
              <div className="absolute bottom-0 left-0 right-0 h-1 bg-black/50">
                <div
                  className="h-full bg-brand-red"
                  style={{ width: `${item.progress_percent}%` }}
                />
              </div>
              {/* Quality badge */}
              {item.duration_sec > 0 && (
                <span className="absolute top-2 right-2 bg-black/70 text-white text-xs px-2 py-0.5 rounded">
                  {Math.floor(item.last_position_sec / 60)}:{String(item.last_position_sec % 60).padStart(2, "0")} /
                  {Math.floor(item.duration_sec / 60)}:{String(item.duration_sec % 60).padStart(2, "0")}
                </span>
              )}
            </div>
            <p className="text-white text-sm mt-2 line-clamp-2 group-hover:text-brand-red transition-colors">
              {item.title}
            </p>
          </Link>
        ))}
      </div>
    </section>
  );
}
