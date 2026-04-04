"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  PlusCircle,
  Pencil,
  Trash2,
  ExternalLink,
  Search,
  Loader2,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminGetMovies, adminDeleteMovie, Movie } from "@/lib/api";

export default function AdminMoviesPage() {
  const { token } = useAuth();
  const [movies, setMovies] = useState<Movie[]>([]);
  const [filtered, setFiltered] = useState<Movie[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);

  const fetchMovies = async () => {
    if (!token) return;
    try {
      const data = await adminGetMovies(token);
      setMovies(data || []);
      setFiltered(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMovies();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  // Client-side filter
  useEffect(() => {
    if (!search.trim()) {
      setFiltered(movies);
    } else {
      const q = search.toLowerCase();
      setFiltered(
        movies.filter(
          (m) =>
            m.title.toLowerCase().includes(q) ||
            (m.code && String(m.code).includes(q)) ||
            m.genre?.some((g) => g.toLowerCase().includes(q))
        )
      );
    }
  }, [search, movies]);

  const handleDelete = async (movie: Movie) => {
    if (
      !confirm(
        `Haqiqatan ham "${movie.title}" filmni o'chirmoqchimisiz? Bu amalni bekor qilib bo'lmaydi.`
      )
    )
      return;

    setDeleting(movie.id);
    try {
      await adminDeleteMovie(token!, movie.id);
      setMovies((prev) => prev.filter((m) => m.id !== movie.id));
    } catch (err) {
      alert(err instanceof Error ? err.message : "O'chirishda xato");
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="p-4 sm:p-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-white">Kinolar</h1>
          <p className="text-gray-500 text-sm mt-1">
            {movies.length} ta kino mavjud
          </p>
        </div>
        <Link
          href="/admin/movies/new"
          className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-medium px-4 sm:px-5 py-2 sm:py-2.5 rounded-lg transition-colors text-sm"
        >
          <PlusCircle size={16} />
          <span className="hidden sm:inline">Kino qo'shish</span>
          <span className="sm:hidden">Qo'shish</span>
        </Link>
      </div>

      {/* Search */}
      <div className="relative mb-6 max-w-sm">
        <Search
          size={15}
          className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-500"
        />
        <input
          type="text"
          placeholder="Kinolarni qidiring..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full bg-brand-card border border-brand-border rounded-lg pl-9 pr-4 py-2.5 text-sm text-white placeholder-gray-600 focus:outline-none focus:border-brand-red transition-colors"
        />
      </div>

      {/* Table */}
      {loading ? (
        <div className="flex items-center gap-2 text-gray-500 py-12 justify-center">
          <Loader2 size={18} className="animate-spin" />
          Kinolar yuklanmoqda...
        </div>
      ) : filtered.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl p-8 sm:p-12 text-center">
          <p className="text-gray-500">
            {search ? "Qidiruvga mos kinolar yo'q." : "Hali kinolar yo'q."}
          </p>
          {!search && (
            <Link
              href="/admin/movies/new"
              className="mt-3 inline-block text-sm text-brand-red hover:underline"
            >
              Birinchi kinoni qo'shing →
            </Link>
          )}
        </div>
      ) : (
        <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full text-sm">
              <thead>
                <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                  <th className="text-left px-3 sm:px-5 py-3">Kino</th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden lg:table-cell">
                    Janr
                  </th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">
                    Yil
                  </th>
                  <th className="text-left px-3 sm:px-5 py-3 hidden md:table-cell">
                    Sifat
                  </th>
                  <th className="text-right px-3 sm:px-5 py-3">Amallar</th>
                </tr>
              </thead>
              <tbody>
                {filtered.map((movie) => (
                  <tr
                    key={movie.id}
                    className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors"
                  >
                    {/* Poster + title */}
                    <td className="px-3 sm:px-5 py-3">
                      <div className="flex items-center gap-2 sm:gap-3">
                        {/* eslint-disable-next-line @next/next/no-img-element */}
                        <img
                          src={movie.poster_url}
                          alt={movie.title}
                          className="w-8 h-12 sm:w-9 sm:h-14 object-cover rounded shrink-0"
                          onError={(e) => {
                            (e.target as HTMLImageElement).style.display =
                              "none";
                          }}
                        />
                        <div className="min-w-0">
                          <p className="text-white font-medium truncate max-w-[150px] sm:max-w-[200px]">
                            {movie.code && (
                              <span className="text-gray-500 font-mono text-xs mr-1">
                                #{movie.code}
                              </span>
                            )}
                            {movie.title}
                          </p>
                          <p className="text-gray-600 text-xs font-mono truncate max-w-[150px] sm:max-w-[200px]">
                            {movie.slug}
                          </p>
                        </div>
                      </div>
                    </td>

                    {/* Genre */}
                    <td className="px-3 sm:px-5 py-3 hidden lg:table-cell">
                      <div className="flex flex-wrap gap-1">
                        {movie.genre?.slice(0, 2).map((g) => (
                          <span
                            key={g}
                            className="text-xs bg-brand-border text-gray-400 px-2 py-0.5 rounded-full"
                          >
                            {g}
                          </span>
                        ))}
                      </div>
                    </td>

                    {/* Year */}
                    <td className="px-3 sm:px-5 py-3 text-gray-400 hidden md:table-cell">
                      {movie.year}
                    </td>

                    {/* Quality */}
                    <td className="px-3 sm:px-5 py-3 hidden md:table-cell">
                      {movie.quality && (
                        <span className="text-xs text-brand-red border border-brand-red/40 px-2 py-0.5 rounded font-semibold">
                          {movie.quality}
                        </span>
                      )}
                    </td>

                    {/* Actions */}
                    <td className="px-3 sm:px-5 py-3">
                      <div className="flex items-center justify-end gap-1">
                        {/* View on site */}
                        <Link
                          href={`/movies/${movie.slug}`}
                          target="_blank"
                          title="Saytda ko'rish"
                          className="p-2 text-gray-500 hover:text-gray-300 rounded-lg hover:bg-brand-border transition-colors"
                        >
                          <ExternalLink size={14} />
                        </Link>

                        {/* Edit */}
                        <Link
                          href={`/admin/movies/${movie.id}/edit`}
                          title="Tahrirlash"
                          className="p-2 text-gray-400 hover:text-white rounded-lg hover:bg-brand-border transition-colors"
                        >
                          <Pencil size={14} />
                        </Link>

                        {/* Delete */}
                        <button
                          onClick={() => handleDelete(movie)}
                          disabled={deleting === movie.id}
                          title="O'chirish"
                          className="p-2 text-gray-500 hover:text-red-400 rounded-lg hover:bg-red-400/10 transition-colors disabled:opacity-50"
                        >
                          {deleting === movie.id ? (
                            <Loader2 size={14} className="animate-spin" />
                          ) : (
                            <Trash2 size={14} />
                          )}
                        </button>
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </div>
  );
}
