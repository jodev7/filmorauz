import { notFound } from "next/navigation";
import type { Episode, SeriesWithSeasons } from "@/lib/series-api";
import { getEpisode, getSeriesBySlug } from "@/lib/series-api";
import {
  buildBestEpisodePath,
  buildBestEpisodeUrl,
  buildSeasonUrl,
  buildSeriesUrl,
  findEpisodeByNumber,
  findSeasonForEpisode,
  flattenSeriesEpisodes,
} from "@/lib/content-routes";
import { buildContentDescription, buildContentKeywords, buildContentTitle, pickSeoImage } from "@/lib/seo";

export type EpisodePageData = {
  episode: Episode;
  series: SeriesWithSeasons;
  seasonNumber: number;
  canonicalUrl: string;
  rawUrl: string;
  title: string;
  description: string;
  keywords: string[];
  imageUrl: string;
  shouldIndexRawRoute: boolean;
  previousEpisode: { id: string; title: string; episode_number: number; href: string } | null;
  nextEpisode: { id: string; title: string; episode_number: number; href: string } | null;
};

function buildEpisodeNav(seriesData: SeriesWithSeasons, currentEpisodeId: string) {
  const orderedEpisodes = flattenSeriesEpisodes(seriesData);
  const index = orderedEpisodes.findIndex((item) => item.episode.id === currentEpisodeId);
  const makeNav = (item?: { episode: Episode; seasonNumber: number }) =>
    item
      ? {
          id: item.episode.id,
          title: item.episode.title,
          episode_number: item.episode.episode_number,
          href: buildBestEpisodePath({
            episodeId: item.episode.id,
            seriesSlug: seriesData.series.slug,
            seasonNumber: item.seasonNumber,
            episodeNumber: item.episode.episode_number,
          }),
        }
      : null;

  return {
    previousEpisode: makeNav(index > 0 ? orderedEpisodes[index - 1] : undefined),
    nextEpisode: makeNav(index >= 0 ? orderedEpisodes[index + 1] : undefined),
  };
}

function buildPageData(seriesData: SeriesWithSeasons, episode: Episode, seasonNumber: number): EpisodePageData {
  const title = buildContentTitle(episode.title || `${seriesData.series.title} ${episode.episode_number}-qism`);
  const description = buildContentDescription(
    `${seriesData.series.title} ${seasonNumber}-mavsum ${episode.episode_number}-qism`,
    seriesData.series.year
  );
  const canonicalUrl = buildBestEpisodeUrl({
    episodeId: episode.id,
    seriesSlug: seriesData.series.slug,
    seasonNumber,
    episodeNumber: episode.episode_number,
  });
  const rawUrl = buildBestEpisodeUrl({ episodeId: episode.id });
  const { previousEpisode, nextEpisode } = buildEpisodeNav(seriesData, episode.id);

  return {
    episode,
    series: seriesData,
    seasonNumber,
    canonicalUrl,
    rawUrl,
    title,
    description,
    keywords: buildContentKeywords({
      title: episode.title || `${seriesData.series.title} ${episode.episode_number}-qism`,
      uzbekTitle: episode.title || undefined,
      genres: seriesData.series.genre,
      extra: [
        seriesData.series.title,
        `${seriesData.series.title} ${seasonNumber}-mavsum`,
        `${seriesData.series.title} ${episode.episode_number}-qism`,
      ],
    }),
    imageUrl: pickSeoImage(
      episode.thumbnail_url,
      seriesData.series.backdrop_url,
      seriesData.series.poster_url
    ),
    shouldIndexRawRoute: canonicalUrl === rawUrl,
    previousEpisode,
    nextEpisode,
  };
}

export async function getEpisodePageDataById(id: string): Promise<EpisodePageData> {
  const response = await getEpisode(id);
  const episode = response.episode;
  if (!episode?.series_slug) {
    notFound();
  }

  const seriesData = await getSeriesBySlug(episode.series_slug);
  const matched = findSeasonForEpisode(seriesData, episode.id);
  if (!matched) {
    notFound();
  }

  return buildPageData(seriesData, matched.episode, matched.season.season.season_number);
}

export async function getEpisodePageDataBySeoPath(
  seriesSlug: string,
  seasonNumber: number,
  episodeNumber: number
): Promise<EpisodePageData> {
  const seriesData = await getSeriesBySlug(seriesSlug);
  const matched = findEpisodeByNumber(seriesData, seasonNumber, episodeNumber);
  if (!matched) {
    notFound();
  }

  return buildPageData(seriesData, matched.episode, matched.season.season.season_number);
}

export function buildEpisodeJsonLd(data: EpisodePageData) {
  return {
    "@context": "https://schema.org",
    "@type": "TVEpisode",
    name: data.episode.title,
    description: data.episode.description || data.description,
    url: data.canonicalUrl,
    image: data.imageUrl,
    episodeNumber: data.episode.episode_number,
    partOfSeason: {
      "@type": "TVSeason",
      seasonNumber: data.seasonNumber,
      url: buildSeasonUrl(data.series.series.slug, data.seasonNumber),
    },
    partOfSeries: {
      "@type": "TVSeries",
      name: data.series.series.title,
      url: buildSeriesUrl(data.series.series.slug),
    },
    datePublished: data.episode.air_date || undefined,
  };
}
