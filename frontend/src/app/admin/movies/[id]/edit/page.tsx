"use client";

import { useEffect, useState } from "react";
import { useRouter, useParams } from "next/navigation";
import Link from "next/link";
import MovieForm from "@/components/MovieForm";
import AdminPageHeader from "@/components/admin/form/PageHeader";
import { storageQualityList } from "@/components/admin/form/constants";
import { useToast } from "@/components/admin/Toast";
import { useAuth } from "@/lib/auth-context";
import { adminUpdateMovie, adminGetMovie, adminFetchMovieCredits, Movie, MovieInput } from "@/lib/api";

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
  const toast = useToast();
  const params = useParams();
  const id = params.id as string;

  const [movie, setMovie] = useState<Movie | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  useEffect(() => {
    if (!token) return;
    // Fetch this one movie by id (the old "load 500 and find" missed older titles).
    adminGetMovie(token, id)
      .then(setMovie)
      .catch((err) => setError(err instanceof Error && err.message === "not_found" ? "Kino topilmadi" : "Kinoni yuklab bo'lmadi"))
      .finally(() => setLoading(false));
  }, [token, id]);

  const handleSubmit = async (data: MovieInput) => {
    if (!token) throw new Error("Not authenticated");
    await adminUpdateMovie(token, id, data);
    toast.success("O'zgarishlar saqlandi");
  };

  if (loading) {
    return (
      <div className="p-4 sm:p-8">
        <div className="mb-6 h-8 w-64 animate-pulse rounded-lg bg-white/5" />
        <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_320px]">
          <div className="space-y-6">
            {[0, 1, 2].map((i) => (
              <div key={i} className="h-56 animate-pulse rounded-2xl bg-white/5" />
            ))}
          </div>
          <div className="h-80 animate-pulse rounded-2xl bg-white/5" />
        </div>
      </div>
    );
  }

  if (error || !movie) {
    return (
      <div className="p-8">
        <p className="text-red-400">{error || "Kino topilmadi"}</p>
        <Link href="/admin/movies" className="mt-2 block text-sm text-orange-400 hover:underline">
          ← Kinolarga qaytish
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
    cast: movie.cast ?? [],
    director: movie.director ?? "",
  };

  const statusBadge =
    movie.approval_status === "pending" ? (
      <span className="rounded-full bg-yellow-500/15 px-2 py-0.5 text-xs font-medium text-yellow-300">Kutmoqda</span>
    ) : movie.approval_status === "rejected" ? (
      <span className="rounded-full bg-red-500/15 px-2 py-0.5 text-xs font-medium text-red-300">Rad etilgan</span>
    ) : (
      <span className="rounded-full bg-emerald-500/15 px-2 py-0.5 text-xs font-medium text-emerald-300">Saytda</span>
    );

  return (
    <div className="p-4 sm:p-8">
      <AdminPageHeader
        backHref="/admin/movies"
        backLabel="Kinolar"
        title={movie.title}
        badges={statusBadge}
        subtitle={
          <span className="font-mono text-xs">
            {movie.code ? `#${movie.code} · ` : ""}/movies/{movie.slug}
          </span>
        }
      />
      <MovieForm
        initialData={initialData}
        onSubmit={handleSubmit}
        submitLabel="Saqlash"
        token={token ?? undefined}
        castDetails={movie.cast_details}
        onFetchCredits={() => adminFetchMovieCredits(token!, movie.id)}
        storageQualities={storageQualityList(movie.generated_qualities?.length ? movie.generated_qualities : movie.available_qualities)}
        previewHref={movie.approval_status === "approved" || !movie.approval_status ? `/movies/${movie.slug}` : undefined}
      />
    </div>
  );
}
