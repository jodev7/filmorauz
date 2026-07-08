"use client";

import { memo } from "react";
import Link from "next/link";
import { Play, Star, Tv } from "lucide-react";
import MediaImage from "@/components/MediaImage";
import MediaTitle from "@/components/MediaTitle";
import type { Series } from "@/lib/series-api";
import { localizeSingleGenre } from "@/lib/localization";

interface SeriesCardProps {
  series: Series;
}

function areSeriesCardPropsEqual(prev: SeriesCardProps, next: SeriesCardProps) {
  return (
    prev.series.id === next.series.id &&
    prev.series.slug === next.series.slug &&
    prev.series.title === next.series.title &&
    prev.series.poster_url === next.series.poster_url &&
    prev.series.year === next.series.year &&
    prev.series.rating_avg === next.series.rating_avg &&
    prev.series.is_premium === next.series.is_premium &&
    prev.series.quality === next.series.quality &&
    (prev.series.genre?.[0] || "") === (next.series.genre?.[0] || "")
  );
}

function SeriesCardImpl({ series }: SeriesCardProps) {
  return (
    <Link
      href={`/series/${series.slug}`}
      className="series-card group relative glass-card rounded-2xl overflow-hidden transition-all duration-300 hover:-translate-y-1 hover:shadow-xl hover:border-orange-500/30 block w-full h-full"
    >
      <div className="relative aspect-[2/3] overflow-hidden bg-white/5">
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
          <div className="flex flex-col gap-1.5">
            <span className="bg-brand-red/80 text-white text-[10px] sm:text-xs px-1.5 py-0.5 rounded flex items-center gap-1 w-fit">
              <Tv className="w-3 h-3" />
              Serial
            </span>
          </div>

          <div className="flex flex-col gap-1.5 items-end">
            {series.is_premium && (
              <span className="bg-yellow-500/90 text-white text-[10px] px-1.5 py-0.5 rounded flex items-center gap-1">
                <Star className="w-3 h-3 fill-white" />
              </span>
            )}
            {series.quality && (
              <span className={`text-[10px] font-bold px-1.5 py-0.5 rounded ${
                series.is_premium 
                  ? "bg-yellow-500/90 text-black" 
                  : "bg-brand-red text-white"
              }`}>
                {series.quality}
              </span>
            )}
          </div>
        </div>
      </div>

      <div className="p-3">
        <h3 className="font-medium text-white text-sm truncate group-hover:text-brand-red transition-colors">
          <MediaTitle title={series.title} />
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

const SeriesCard = memo(SeriesCardImpl, areSeriesCardPropsEqual);
SeriesCard.displayName = "SeriesCard";

export default SeriesCard;
