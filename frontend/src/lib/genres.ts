import { getHomepageData, getMovies } from "@/lib/api";
import { getSeries } from "@/lib/series-api";

export const DEFAULT_GENRES = [
  "action",
  "drama",
  "comedy",
  "animation",
  "anime",
  "dorama",
  "thriller",
  "horror",
  "romance",
  "sci-fi",
  "documentary",
  "crime",
  "fantasy",
];

export async function getPublicGenres(): Promise<string[]> {
  const genres = new Set<string>();

  for (const genre of DEFAULT_GENRES) {
    genres.add(genre);
  }

  try {
    const homepage = await getHomepageData();
    for (const genre of homepage.genres || []) {
      if (genre.slug) {
        genres.add(genre.slug.toLowerCase());
      }
    }
  } catch {
    // Ignore and fall back to defaults.
  }

  try {
    const movies = await getMovies({ page: 1, limit: 100 });
    for (const movie of movies.data || []) {
      for (const genre of movie.genre || []) {
        if (genre) {
          genres.add(genre.toLowerCase());
        }
      }
    }
  } catch {
    // Ignore and use collected values so far.
  }

  try {
    const series = await getSeries(1, 100);
    for (const item of series.data || []) {
      for (const genre of item.genre || []) {
        if (genre) {
          genres.add(genre.toLowerCase());
        }
      }
    }
  } catch {
    // Ignore and use collected values so far.
  }

  return Array.from(genres).sort();
}
