"use client";

import { useEffect, useState } from "react";
import { useRouter, useParams } from "next/navigation";
import Link from "next/link";
import { ChevronLeft, Loader2 } from "lucide-react";
import MovieForm from "@/components/MovieForm";
import { useAuth } from "@/lib/auth-context";
import { adminUpdateMovie, adminGetMovies, Movie, MovieInput } from "@/lib/api";

function normalizeGenreValue(value: string): string {
  const trimmed = value.trim().toLowerCase();
  if (!trimmed) return "";
  const normalized = trimmed.replace(/[_\s]+/g, "-").replace(/-+/g, "-");
  if (normalized === "science-fiction" || normalized === "sciencefiction" || normalized === "scifi") {
    return "sci-fi";
  }
  return normalized;
}

export default function EditMoviePage() {
  const { token } = useAuth();
  const router = useRouter();
  const params = useParams();
  const id = params.id as string;

  const [movie, setMovie] = useState<Movie | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    adminGetMovies(token)
      .then((movies) => {
        const found = movies.find((m) => m.id === id);
        if (!found) {
          setError("Movie not found");
        } else {
          setMovie(found);
        }
      })
      .catch(() => setError("Failed to load movie"))
      .finally(() => setLoading(false));
  }, [token, id]);

  const handleSubmit = async (data: MovieInput) => {
    if (!token) throw new Error("Not authenticated");
    await adminUpdateMovie(token, id, data);
    router.push("/admin/movies");
  };

  if (loading) {
    return (
      <div className="p-8 flex items-center gap-2 text-gray-500">
        <Loader2 size={18} className="animate-spin" />
        Loading movie...
      </div>
    );
  }

  if (error || !movie) {
    return (
      <div className="p-8">
        <p className="text-red-400">{error || "Movie not found"}</p>
        <Link
          href="/admin/movies"
          className="text-sm text-brand-red hover:underline mt-2 block"
        >
          ← Back to movies
        </Link>
      </div>
    );
  }

  // Map movie to form initial data.
  // "ingestion" is a legacy source_type not shown in the form dropdown;
  // treat it as "direct_hls" since ingested movies stream via HLS.
  const sourceType = movie.source_type === "ingestion" ? "direct_hls" : movie.source_type;

  const initialData: Partial<MovieInput> = {
    title: movie.title,
    description: movie.description,
    poster_url: movie.poster_url,
    backdrop_url: movie.backdrop_url,
    year: movie.year,
    genre: Array.isArray(movie.genre)
      ? movie.genre.map((g) => normalizeGenreValue(g)).filter(Boolean)
      : [],
    country: movie.country,
    video_url: movie.video_url,
    embed_url: movie.embed_url,
    source_type: sourceType,
    duration: movie.duration,
    quality: movie.quality,
    is_premium: movie.is_premium ?? false,
    slug: movie.slug,
  };

  return (
    <div className="p-8">
      {/* Header */}
      <div className="mb-8">
        <Link
          href="/admin/movies"
          className="flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-4 transition-colors"
        >
          <ChevronLeft size={16} />
          Back to Movies
        </Link>
        <h1 className="text-2xl font-bold text-white">Edit Movie</h1>
        <p className="text-gray-500 text-sm mt-1 font-mono">{movie.slug}</p>
      </div>

      <MovieForm
        initialData={initialData}
        onSubmit={handleSubmit}
        submitLabel="Update Movie"
        token={token ?? undefined}
      />
    </div>
  );
}
