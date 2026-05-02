import type { Episode, SeriesWithSeasons } from "@/lib/series-api";

export const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export function buildMoviePath(slug: string): string {
  return `/movies/${slug}`;
}

export function buildMovieUrl(slug: string): string {
  return `${SITE_URL}${buildMoviePath(slug)}`;
}

export function buildSeriesPath(slug: string): string {
  return `/series/${slug}`;
}

export function buildSeriesUrl(slug: string): string {
  return `${SITE_URL}${buildSeriesPath(slug)}`;
}

export function buildSeasonPath(seriesSlug: string, seasonNumber: number): string {
  return `/series/${seriesSlug}/season/${seasonNumber}`;
}

export function buildSeasonUrl(seriesSlug: string, seasonNumber: number): string {
  return `${SITE_URL}${buildSeasonPath(seriesSlug, seasonNumber)}`;
}

export function buildRawEpisodePath(episodeId: string): string {
  return `/episode/${episodeId}`;
}

export function buildRawEpisodeUrl(episodeId: string): string {
  return `${SITE_URL}${buildRawEpisodePath(episodeId)}`;
}

export function hasStableEpisodeSeoMetadata(input: {
  seriesSlug?: string;
  seasonNumber?: number;
  episodeNumber?: number;
}): boolean {
  return Boolean(
    input.seriesSlug &&
      Number.isInteger(input.seasonNumber) &&
      (input.seasonNumber as number) > 0 &&
      Number.isInteger(input.episodeNumber) &&
      (input.episodeNumber as number) > 0
  );
}

export function buildSeoEpisodePath(input: {
  seriesSlug: string;
  seasonNumber: number;
  episodeNumber: number;
}): string {
  return `/series/${input.seriesSlug}/season/${input.seasonNumber}/episode/${input.episodeNumber}`;
}

export function buildSeoEpisodeUrl(input: {
  seriesSlug: string;
  seasonNumber: number;
  episodeNumber: number;
}): string {
  return `${SITE_URL}${buildSeoEpisodePath(input)}`;
}

export function buildBestEpisodePath(input: {
  episodeId: string;
  seriesSlug?: string;
  seasonNumber?: number;
  episodeNumber?: number;
}): string {
  if (hasStableEpisodeSeoMetadata(input)) {
    return buildSeoEpisodePath({
      seriesSlug: input.seriesSlug!,
      seasonNumber: input.seasonNumber!,
      episodeNumber: input.episodeNumber!,
    });
  }
  return buildRawEpisodePath(input.episodeId);
}

export function buildBestEpisodeUrl(input: {
  episodeId: string;
  seriesSlug?: string;
  seasonNumber?: number;
  episodeNumber?: number;
}): string {
  return `${SITE_URL}${buildBestEpisodePath(input)}`;
}

export function findSeasonByNumber(seriesData: SeriesWithSeasons, seasonNumber: number) {
  return seriesData.seasons.find((item) => item.season.season_number === seasonNumber) || null;
}

export function findSeasonForEpisode(seriesData: SeriesWithSeasons, episodeId: string) {
  for (const season of seriesData.seasons) {
    const episode = season.episodes.find((item) => item.id === episodeId);
    if (episode) {
      return { season, episode };
    }
  }
  return null;
}

export function findEpisodeByNumber(seriesData: SeriesWithSeasons, seasonNumber: number, episodeNumber: number) {
  const season = findSeasonByNumber(seriesData, seasonNumber);
  if (!season) return null;
  const episode = season.episodes.find((item) => item.episode_number === episodeNumber) || null;
  if (!episode) return null;
  return { season, episode };
}

export function flattenSeriesEpisodes(seriesData: SeriesWithSeasons): Array<{
  episode: Episode;
  seasonNumber: number;
}> {
  return seriesData.seasons.flatMap((season) =>
    (season.episodes || []).map((episode) => ({
      episode,
      seasonNumber: season.season.season_number,
    }))
  );
}
