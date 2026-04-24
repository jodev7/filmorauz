"use client";

import { SyntheticEvent, useState } from "react";
import Link from "next/link";
import { ChevronDown, ChevronUp, Play, Clock } from "lucide-react";
import { SeasonWithEpisodes, Episode, buildEpisodeUrl } from "@/lib/series-api";
import { formatDuration } from "@/lib/movie-utils";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";

interface SeasonListProps {
  seasons: SeasonWithEpisodes[];
  currentEpisodeId?: string;
  seriesBackdropUrl?: string;
  seriesPosterUrl?: string;
}

function isKnownBrokenEpisodeThumbnail(url?: string | null): boolean {
  const value = url?.trim().toLowerCase();
  if (!value) return true;
  return (
    value.endsWith("/images/ogimage.png") ||
    value.endsWith("/media/images/ogimage.png") ||
    value.endsWith("/ogimage.png")
  );
}

function getEpisodeCardThumbnail(
  episode: Episode,
  seriesBackdropUrl?: string,
  seriesPosterUrl?: string
): string {
  const candidates = [
    isKnownBrokenEpisodeThumbnail(episode.thumbnail_url) ? "" : episode.thumbnail_url,
    seriesBackdropUrl,
    seriesPosterUrl,
    DEFAULT_POSTER_PLACEHOLDER,
  ];

  for (const candidate of candidates) {
    const normalized = normalizeMediaUrl(candidate, "");
    if (normalized) {
      return normalized;
    }
  }

  return DEFAULT_POSTER_PLACEHOLDER;
}

function handleProtectedThumbnailError(event: SyntheticEvent<HTMLImageElement>) {
  const target = event.currentTarget;
  if (target.src.endsWith(DEFAULT_POSTER_PLACEHOLDER)) {
    return;
  }
  target.src = DEFAULT_POSTER_PLACEHOLDER;
}

export default function SeasonList({
  seasons,
  currentEpisodeId,
  seriesBackdropUrl,
  seriesPosterUrl,
}: SeasonListProps) {
  // Defensive: ensure seasons is always an array
  const safeSeasons = Array.isArray(seasons) ? seasons : [];

  // First season is open by default
  const [openSeasons, setOpenSeasons] = useState<Set<string>>(
    new Set(safeSeasons.length > 0 ? [safeSeasons[0].season.id] : [])
  );

  const toggleSeason = (seasonId: string) => {
    const newOpen = new Set(openSeasons);
    if (newOpen.has(seasonId)) {
      newOpen.delete(seasonId);
    } else {
      newOpen.add(seasonId);
    }
    setOpenSeasons(newOpen);
  };

  // Calculate totals with defensive handling for episodes
  const totalEpisodes = safeSeasons.reduce((acc, s) => {
    const episodes = Array.isArray(s.episodes) ? s.episodes : [];
    return acc + episodes.length;
  }, 0);

  return (
    <div className="space-y-4">
      {/* Header with totals */}
      <div className="flex items-center gap-4 text-gray-400 text-sm mb-6">
        <span className="bg-brand-card px-3 py-1 rounded-full border border-brand-border">
          {safeSeasons.length} {safeSeasons.length === 1 ? "fasl" : "fasl"}
        </span>
        <span className="bg-brand-card px-3 py-1 rounded-full border border-brand-border">
          {totalEpisodes} {totalEpisodes === 1 ? "epizod" : "epizod"}
        </span>
      </div>

      {/* Season list */}
      {safeSeasons.map((seasonWithEpisodes) => {
        const { season, episodes: seasonEpisodes } = seasonWithEpisodes;
        const episodes = Array.isArray(seasonEpisodes) ? seasonEpisodes : [];
        const isOpen = openSeasons.has(season.id);
        const isCurrentSeason = currentEpisodeId && episodes.some(ep => ep.id === currentEpisodeId);

        return (
          <div 
            key={season.id} 
            className={`bg-brand-card rounded-lg border overflow-hidden transition-colors ${
              isCurrentSeason ? "border-brand-red" : "border-brand-border"
            }`}
          >
            {/* Season header - clickable */}
            <button
              onClick={() => toggleSeason(season.id)}
              className="w-full flex items-center justify-between p-4 hover:bg-brand-dark/30 transition-colors"
            >
              <div className="flex items-center gap-3">
                <div className={`w-10 h-10 rounded-full flex items-center justify-center ${
                  isCurrentSeason ? "bg-brand-red" : "bg-brand-dark"
                }`}>
                  <span className="text-white font-bold">{season.season_number}</span>
                </div>
                <div className="text-left">
                  <h3 className="text-lg font-semibold text-white">
                    {season.title || `Fasl ${season.season_number}`}
                  </h3>
                  <div className="text-sm text-gray-400">
                    {episodes.length} epizod
                  </div>
                </div>
              </div>
              <div className="flex items-center gap-2">
                {isCurrentSeason && (
                  <span className="text-brand-red text-sm">Hozir tomosha qilish</span>
                )}
                {isOpen ? (
                  <ChevronUp className="text-gray-400" size={20} />
                ) : (
                  <ChevronDown className="text-gray-400" size={20} />
                )}
              </div>
            </button>

            {/* Episodes grid - collapsible */}
            {isOpen && (
              <div className="border-t border-brand-border">
                <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-2 p-4">
                  {episodes.map((episode) => {
                    const isActive = episode.id === currentEpisodeId;
                    const thumbnailUrl = getEpisodeCardThumbnail(
                      episode,
                      seriesBackdropUrl,
                      seriesPosterUrl
                    );
                    const hasThumbnail = Boolean(thumbnailUrl);

                    return (
                      <Link
                        key={episode.id}
                        href={buildEpisodeUrl(episode.id)}
                        className={`group block rounded-lg overflow-hidden transition-all ${
                          isActive 
                            ? "bg-brand-red/20 border border-brand-red" 
                            : "bg-brand-dark hover:bg-brand-dark/80 border border-transparent hover:border-brand-border"
                        }`}
                      >
                        {/* Thumbnail */}
                        <div className="relative aspect-video bg-gray-800">
                          {hasThumbnail ? (
                            // eslint-disable-next-line @next/next/no-img-element
                            <img
                              src={thumbnailUrl}
                              alt={episode.title}
                              loading="lazy"
                              onError={handleProtectedThumbnailError}
                              className="absolute inset-0 h-full w-full object-cover transition-transform group-hover:scale-105"
                            />
                          ) : (
                            <div className="w-full h-full flex items-center justify-center">
                              <Play className="text-gray-600" size={24} />
                            </div>
                          )}
                          
                          {/* Play overlay on hover */}
                          <div className="absolute inset-0 bg-black/40 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center">
                            <div className="w-10 h-10 rounded-full bg-brand-red flex items-center justify-center">
                              <Play size={16} className="text-white ml-0.5" fill="white" />
                            </div>
                          </div>

                          {/* Episode number badge */}
                          <div className="absolute top-2 left-2">
                            <span className="bg-black/70 text-white text-xs px-2 py-0.5 rounded">
                              Ep {episode.episode_number}
                            </span>
                          </div>

                          {/* Duration */}
                          {episode.duration > 0 && (
                            <div className="absolute bottom-2 right-2">
                              <span className="bg-black/70 text-white text-xs px-1.5 py-0.5 rounded flex items-center gap-1">
                                <Clock size={10} />
                                {formatDuration(episode.duration)}
                              </span>
                            </div>
                          )}
                        </div>

                        {/* Info */}
                        <div className="p-2">
                          <div className={`text-sm font-medium truncate ${
                            isActive ? "text-brand-red" : "text-white group-hover:text-brand-red"
                          }`}>
                            {episode.title}
                          </div>
                        </div>
                      </Link>
                    );
                  })}
                </div>
              </div>
            )}
          </div>
        );
      })}
    </div>
  );
}
