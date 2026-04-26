"use client";

import { memo } from "react";
import Link from "next/link";
import { Play, Star, Tv } from "lucide-react";
import MediaImage from "@/components/MediaImage";
import type { Series } from "@/lib/series-api";
import { localizeSingleGenre } from "@/lib/localization";

interface SeriesCardProps {
  series: Series;
}

function SeriesCardImpl({ series }: SeriesCardProps) {
  return (
    <Link
      href={`/series/${series.slug}`}
      className="series-card group relative bg-brand-card rounded-xl overflow-hidden border border-brand-border transition-all duration-300 hover:scale-[1.02] hover:shadow-2xl block shrink-0 w-[140px] sm:w-[160px]"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-brand-border">
        <MediaImage
          src={series.poster_url}
          alt={series.title}
          className="absolute inset-0 h-full w-full object-cover transition-transform duration-500 group-hover:scale-105"
        />

        {series.is_premium && (
          <div className="absolute inset-0 bg-gradient-to-t from-black/80 via-black/20 to-transparent opacity-60" />
        )}

        <div className="absolute inset-0 bg-black/60 opacity-0 group-hover:opacity-100 transition-opacity duration-300 flex items-center justify-center">
          <div className={`w-12 h-12 sm:w-14 sm:h-14 rounded-full flex items-center justify-center shadow-lg ${
            series.is_premium
              ? "bg-gradient-to-br from-yellow-500 to-amber-600"
              : "bg-brand-red"
          }`}>
            <Play size={20} className="text-white ml-1" fill="white" />
          </div>
        </div>

        <div className="absolute top-2 left-2 right-2 flex justify-between items-start z-10">
          <span className="bg-brand-red/80 text-white text-xs px-1.5 py-0.5 rounded flex items-center gap-1">
            <Tv className="w-3 h-3" />
            Serial
          </span>
        </div>

        {series.is_premium && (
          <div className="absolute top-2 right-2">
            <span className="bg-yellow-500/80 text-white text-xs px-1.5 py-0.5 rounded flex items-center gap-1">
              <Star className="w-3 h-3 fill-white" />
            </span>
          </div>
        )}
      </div>

      <div className="p-3">
        <h3 className="font-medium text-white text-sm truncate group-hover:text-brand-red transition-colors">
          {series.title}
        </h3>
        <div className="flex items-center gap-2 mt-1 text-xs text-gray-400 flex-wrap">
          <span>{series.year}</span>
          {series.genre && series.genre.length > 0 && (
            <>
              <span>•</span>
              <span className="truncate">{localizeSingleGenre(series.genre[0])}</span>
            </>
          )}
          {series.rating_avg > 0 && (
            <>
              <span>•</span>
              <span className="flex items-center gap-0.5 text-yellow-400">
                <Star size={11} className="fill-yellow-400" />
                {series.rating_avg.toFixed(1)}
              </span>
            </>
          )}
        </div>
      </div>
    </Link>
  );
}

const SeriesCard = memo(SeriesCardImpl);
SeriesCard.displayName = "SeriesCard";

export default SeriesCard;
