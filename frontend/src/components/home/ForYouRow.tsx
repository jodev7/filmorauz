"use client";

import { useEffect, useState } from "react";
import { Sparkles } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { getForYou, Movie } from "@/lib/api";
import { localizeSingleGenre } from "@/lib/localization";
import MovieCarousel from "@/components/MovieCarousel";
import SectionHeader from "@/components/home/SectionHeader";

/**
 * "Siz uchun" — personal recommendations for logged-in users, based on what
 * they watched, liked, rated highly or saved. Hides itself for guests and
 * brand-new users with no history.
 */
export default function ForYouRow() {
  const { token, isLoading } = useAuth();
  const [movies, setMovies] = useState<Movie[]>([]);
  const [genres, setGenres] = useState<string[]>([]);

  useEffect(() => {
    if (isLoading || !token) {
      setMovies([]);
      return;
    }
    let active = true;
    getForYou(token)
      .then((r) => {
        if (!active) return;
        setMovies(r.data);
        setGenres(r.genres);
      })
      .catch(() => {});
    return () => {
      active = false;
    };
  }, [token, isLoading]);

  if (movies.length < 4) return null;

  return (
    <section className="max-w-7xl mx-auto px-4 py-6" aria-label="Siz uchun tavsiyalar">
      <SectionHeader title="Siz uchun" icon={Sparkles} accent="yellow" />
      {genres.length > 0 && (
        <p className="-mt-3 mb-4 text-xs text-gray-500">
          Sizga yoqqan janrlar asosida: {genres.slice(0, 3).map((g) => localizeSingleGenre(g)).join(", ")}
        </p>
      )}
      <MovieCarousel movies={movies} />
    </section>
  );
}
