const API_URL = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";

export interface Series {
  id: string;
  slug: string;
  title: string;
  description: string;
  poster_url: string;
  backdrop_url: string;
  year: number;
  genre: string[];
  country: string;
  views: number;
  rating_avg: number;
  rating_count: number;
  is_premium: boolean;
  is_completed: boolean;
  created_at: string;
  updated_at: string;
}

export interface Season {
  id: string;
  series_id: string;
  season_number: number;
  title: string;
  poster_url: string;
  description: string;
  release_date: string;
  created_at: string;
  updated_at: string;
}

export interface Episode {
  id: string;
  series_id: string;
  season_id: string;
  episode_number: number;
  title: string;
  description: string;
  thumbnail_url: string;
  video_url: string;
  embed_url: string;
  duration: number;
  views: number;
  air_date: string;
  created_at: string;
  updated_at: string;
  series_slug?: string;
}

export interface SeasonWithEpisodes {
  season: Season;
  episodes: Episode[];
}

export interface SeriesWithSeasons {
  series: Series;
  seasons: SeasonWithEpisodes[];
}

export interface SeriesListResponse {
  data: Series[];
  page: number;
  limit: number;
}

// Helper function to build episode URL - use consistently across the app
export function buildEpisodeUrl(episodeId: string): string {
  return `/episode/${episodeId}`;
}

// Helper function to build series URL
export function buildSeriesUrl(seriesSlug: string): string {
  return `/series/${seriesSlug}`;
}

// Get all series
export async function getSeries(page: number = 1, limit: number = 20): Promise<SeriesListResponse> {
  const res = await fetch(`${API_URL}/series?page=${page}&limit=${limit}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch series");
  return res.json();
}

// Get series by slug
export async function getSeriesBySlug(slug: string): Promise<SeriesWithSeasons> {
  const res = await fetch(`${API_URL}/series/${slug}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch series");
  return res.json();
}

// Get episode by ID
export async function getEpisode(id: string): Promise<Episode> {
  const res = await fetch(`${API_URL}/episodes/${id}`, {
    cache: "no-store",
  });
  if (!res.ok) throw new Error("Failed to fetch episode");
  return res.json();
}