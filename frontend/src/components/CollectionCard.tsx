"use client";

import Link from "next/link";
import { Film, Tv } from "lucide-react";
import MediaImage from "@/components/MediaImage";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { Collection } from "@/lib/api";

interface Props {
  collection: Collection;
}

/**
 * CollectionCard — compact card used on the home page and /collections list.
 * Visually parallels MovieCard but links to /collections/[slug] instead of
 * inlining the contained movies (the detail page renders the full list).
 */
export default function CollectionCard({ collection }: Props) {
  const movieCount = collection.movies?.length || 0;
  const seriesCount = collection.series?.length || 0;
  const totalCount = movieCount + seriesCount;

  return (
    <Link
      href={`/collections/${collection.slug}`}
      className="group block overflow-hidden rounded-xl bg-brand-card border border-brand-border hover:border-brand-red/60 transition-colors"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-brand-dark">
        {collection.poster_url ? (
          <MediaImage
            src={normalizeMediaUrl(collection.poster_url)}
            alt={collection.title}
            className="absolute inset-0 h-full w-full object-cover transition-transform duration-300 group-hover:scale-105"
          />
        ) : (
          <div className="absolute inset-0 flex items-center justify-center text-gray-700">
            <Film size={48} />
          </div>
        )}

        <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/30 to-transparent" />

        <div className="absolute top-2 right-2 flex items-center gap-1 rounded-full bg-black/70 px-2 py-0.5 text-[11px] text-white backdrop-blur-sm">
          <span>{totalCount}</span>
          {seriesCount > 0 && movieCount === 0 ? (
            <Tv size={11} />
          ) : (
            <Film size={11} />
          )}
        </div>

        <div className="absolute inset-x-0 bottom-0 p-3">
          <h3 className="font-display text-base sm:text-lg leading-tight text-white tracking-wide line-clamp-2">
            {collection.title}
          </h3>
          {collection.description && (
            <p className="mt-1 text-[11px] text-gray-300 line-clamp-2">
              {collection.description}
            </p>
          )}
        </div>
      </div>
    </Link>
  );
}
