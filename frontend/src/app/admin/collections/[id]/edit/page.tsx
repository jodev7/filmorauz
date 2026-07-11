"use client";

import { useEffect, useMemo, useState } from "react";
import { useRouter, useParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Save,
  Loader2,
  Upload,
  X,
  Search,
  Plus,
  GripVertical,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getAdminCollectionById,
  createCollection,
  updateCollection,
  adminGetMovies,
  adminGetSeries,
  CollectionInput,
  Movie,
  AdminSeries,
  uploadCollectionPoster,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { localizeSingleGenre } from "@/lib/localization";
import MediaImage from "@/components/ui/MediaImage";

// Catalog items (Movie / AdminSeries) share the fields we filter on.
type Filterable = { title: string; year?: number; genre?: string[]; country?: string };

// Country strings in the DB are often multi-valued ("Canada,Ispaniya,Meksika"
// or "Daniya, Belgiya"), so split on comma/slash and trim.
function splitCountries(raw?: string): string[] {
  return (raw || "")
    .split(/[,/]/)
    .map((c) => c.trim())
    .filter(Boolean);
}

function distinctCountries(items: Filterable[]): string[] {
  const set = new Set<string>();
  items.forEach((it) => splitCountries(it.country).forEach((c) => set.add(c)));
  return Array.from(set).sort((a, b) => a.localeCompare(b));
}

function distinctYears(items: Filterable[]): number[] {
  const set = new Set<number>();
  items.forEach((it) => { if (it.year && it.year > 0) set.add(it.year); });
  return Array.from(set).sort((a, b) => b - a);
}

function distinctGenres(items: Filterable[]): string[] {
  const set = new Set<string>();
  items.forEach((it) => (it.genre || []).forEach((g) => { const t = (g || "").trim(); if (t) set.add(t); }));
  return Array.from(set).sort((a, b) => localizeSingleGenre(a).localeCompare(localizeSingleGenre(b)));
}

type CatalogFilters = { search: string; country: string; year: string; genre: string };

function matchesFilters(item: Filterable, f: CatalogFilters): boolean {
  if (f.search.trim() && !item.title.toLowerCase().includes(f.search.trim().toLowerCase())) return false;
  if (f.country && !splitCountries(item.country).includes(f.country)) return false;
  if (f.year && item.year !== Number(f.year)) return false;
  if (f.genre && !(item.genre || []).includes(f.genre)) return false;
  return true;
}

// How many candidates to show in the picker list after filtering.
const PICKER_LIMIT = 50;

export default function EditCollectionPage() {
  const { token } = useAuth();
  const router = useRouter();
  const params = useParams();
  const isNew = params.id === "new";
  const collectionId = isNew ? null : (params.id as string);

  const [loading, setLoading] = useState(!isNew);
  const [saving, setSaving] = useState(false);
  const [uploadingPoster, setUploadingPoster] = useState(false);
  const [movies, setMovies] = useState<Movie[]>([]);
  const [movieSearch, setMovieSearch] = useState("");
  const [movieCountry, setMovieCountry] = useState("");
  const [movieYear, setMovieYear] = useState("");
  const [movieGenre, setMovieGenre] = useState("");
  const [filteredMovies, setFilteredMovies] = useState<Movie[]>([]);
  const [showMovieSearch, setShowMovieSearch] = useState(false);

  const [seriesList, setSeriesList] = useState<AdminSeries[]>([]);
  const [seriesSearch, setSeriesSearch] = useState("");
  const [seriesCountry, setSeriesCountry] = useState("");
  const [seriesYear, setSeriesYear] = useState("");
  const [seriesGenre, setSeriesGenre] = useState("");
  const [filteredSeries, setFilteredSeries] = useState<AdminSeries[]>([]);
  const [showSeriesSearch, setShowSeriesSearch] = useState(false);

  // Filter option lists derived from the loaded catalog (stay in sync w/ data).
  const movieCountries = useMemo(() => distinctCountries(movies), [movies]);
  const movieYears = useMemo(() => distinctYears(movies), [movies]);
  const movieGenres = useMemo(() => distinctGenres(movies), [movies]);
  const seriesCountries = useMemo(() => distinctCountries(seriesList), [seriesList]);
  const seriesYears = useMemo(() => distinctYears(seriesList), [seriesList]);
  const seriesGenres = useMemo(() => distinctGenres(seriesList), [seriesList]);

  const [form, setForm] = useState<CollectionInput>({
    title: "",
    slug: "",
    description: "",
    poster_url: "",
    is_published: false,
    is_featured: false,
    sort_order: 0,
    movie_ids: [],
    series_ids: [],
  });

  // Fetch collection if editing
  useEffect(() => {
    if (!isNew && token && collectionId) {
      getAdminCollectionById(token, collectionId)
        .then((data) => {
          if (data) {
            setForm({
              title: data.title,
              slug: data.slug,
              description: data.description || "",
              poster_url: data.poster_url || "",
              is_published: data.is_published || false,
              is_featured: data.is_featured || false,
              sort_order: data.sort_order || 0,
              movie_ids: data.movie_ids || [],
              series_ids: data.series_ids || [],
            });
          }
        })
        .catch(console.error)
        .finally(() => setLoading(false));
    }
  }, [isNew, token, collectionId]);

  // Fetch movies + series for selection
  useEffect(() => {
    if (!token) return;
    adminGetMovies(token)
      .then((data) => setMovies(data || []))
      .catch(console.error);
    adminGetSeries(token)
      .then((data) => setSeriesList(data || []))
      .catch(console.error);
  }, [token]);

  // Filter movies by search + country + year + genre
  useEffect(() => {
    const f: CatalogFilters = { search: movieSearch, country: movieCountry, year: movieYear, genre: movieGenre };
    setFilteredMovies(movies.filter((m) => matchesFilters(m, f)).slice(0, PICKER_LIMIT));
  }, [movieSearch, movieCountry, movieYear, movieGenre, movies]);

  // Filter series by search + country + year + genre
  useEffect(() => {
    const f: CatalogFilters = { search: seriesSearch, country: seriesCountry, year: seriesYear, genre: seriesGenre };
    setFilteredSeries(seriesList.filter((s) => matchesFilters(s, f)).slice(0, PICKER_LIMIT));
  }, [seriesSearch, seriesCountry, seriesYear, seriesGenre, seriesList]);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    setSaving(true);
    try {
      if (isNew) {
        await createCollection(token, form);
      } else {
        await updateCollection(token, collectionId!, form);
      }
      router.push("/admin/collections");
    } catch (err) {
      alert(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  const addMovie = (movieId: string) => {
    if (form.movie_ids?.includes(movieId)) return;
    setForm({
      ...form,
      movie_ids: [...(form.movie_ids || []), movieId],
    });
  };

  const removeMovie = (movieId: string) => {
    setForm({
      ...form,
      movie_ids: (form.movie_ids || []).filter((id) => id !== movieId),
    });
  };

  const moveMovie = (index: number, direction: "up" | "down") => {
    const newIds = [...(form.movie_ids || [])];
    const newIndex = direction === "up" ? index - 1 : index + 1;
    if (newIndex < 0 || newIndex >= newIds.length) return;
    [newIds[index], newIds[newIndex]] = [newIds[newIndex], newIds[index]];
    setForm({ ...form, movie_ids: newIds });
  };

  const selectedMovies = movies.filter((m) =>
    form.movie_ids?.includes(m.id)
  );

  const addSeries = (seriesId: string) => {
    if (form.series_ids?.includes(seriesId)) return;
    setForm({
      ...form,
      series_ids: [...(form.series_ids || []), seriesId],
    });
  };

  const removeSeries = (seriesId: string) => {
    setForm({
      ...form,
      series_ids: (form.series_ids || []).filter((id) => id !== seriesId),
    });
  };

  const moveSeries = (index: number, direction: "up" | "down") => {
    const newIds = [...(form.series_ids || [])];
    const newIndex = direction === "up" ? index - 1 : index + 1;
    if (newIndex < 0 || newIndex >= newIds.length) return;
    [newIds[index], newIds[newIndex]] = [newIds[newIndex], newIds[index]];
    setForm({ ...form, series_ids: newIds });
  };

  const selectedSeries = seriesList.filter((s) =>
    form.series_ids?.includes(s.id)
  );

  // Generate slug from title
  const handleTitleChange = (title: string) => {
    const slug = title
      .toLowerCase()
      .replace(/[^a-z0-9]+/g, "-")
      .replace(/^-|-$/g, "");
    setForm({ ...form, title, slug: form.slug || slug });
  };

  const handlePosterUpload = async (file: File) => {
    if (!token) return;
    setUploadingPoster(true);
    try {
      const result = await uploadCollectionPoster(token, file);
      setForm((prev) => ({ ...prev, poster_url: result.url }));
    } catch (err) {
      alert(err instanceof Error ? err.message : "Poster upload failed");
    } finally {
      setUploadingPoster(false);
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center py-12">
        <Loader2 className="animate-spin text-brand-red" size={32} />
      </div>
    );
  }

  return (
    <div className="p-4 sm:p-8 max-w-3xl">
      {/* Header */}
      <div className="flex items-center gap-4 mb-8">
        <Link
          href="/admin/collections"
          className="p-2 text-gray-400 hover:text-white hover:bg-brand-border rounded-lg transition-colors"
        >
          <ArrowLeft size={20} />
        </Link>
        <h1 className="text-2xl font-bold text-white">
          {isNew ? "Yangi kolleksiya" : "Kolleksiyani tahrirlash"}
        </h1>
      </div>

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Title */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Nomi *</label>
          <input
            type="text"
            value={form.title}
            onChange={(e) => handleTitleChange(e.target.value)}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            required
          />
        </div>

        {/* Slug */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Slug *</label>
          <input
            type="text"
            value={form.slug}
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            required
          />
        </div>

        {/* Description */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Tavsif</label>
          <textarea
            value={form.description}
            onChange={(e) => setForm({ ...form, description: e.target.value })}
            rows={3}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red resize-none"
          />
        </div>

        {/* Poster */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Poster</label>
          <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
            {uploadingPoster ? <Loader2 size={16} className="animate-spin" /> : <Upload size={16} />}
            <span>
              {uploadingPoster
                ? "Yuklanmoqda..."
                : form.poster_url
                ? "Posterni almashtirish"
                : "Poster yuklash"}
            </span>
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,image/gif"
              className="hidden"
              onChange={(e) => {
                const file = e.target.files?.[0];
                e.target.value = "";
                if (file) void handlePosterUpload(file);
              }}
            />
          </label>
          {form.poster_url ? (
            <MediaImage src={normalizeMediaUrl(form.poster_url)} alt="Poster preview" className="mt-2 h-24 rounded object-cover border border-brand-border" />
          ) : null}
        </div>

        {/* Sort Order */}
        <div>
          <label className="block text-gray-400 text-sm mb-2"> Tartib raqami</label>
          <input
            type="number"
            value={form.sort_order}
            onChange={(e) =>
              setForm({ ...form, sort_order: parseInt(e.target.value) || 0 })
            }
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
          />
        </div>

        {/* Toggles */}
        <div className="flex gap-6">
          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={form.is_published}
              onChange={(e) =>
                setForm({ ...form, is_published: e.target.checked })
              }
              className="w-5 h-5 rounded border-brand-border bg-brand-card text-brand-red focus:ring-brand-red"
            />
            <span className="text-white">Chop etilgan</span>
          </label>

          <label className="flex items-center gap-3 cursor-pointer">
            <input
              type="checkbox"
              checked={form.is_featured}
              onChange={(e) =>
                setForm({ ...form, is_featured: e.target.checked })
              }
              className="w-5 h-5 rounded border-brand-border bg-brand-card text-brand-red focus:ring-brand-red"
            />
            <span className="text-white">Tanlangan</span>
          </label>
        </div>

        {/* Movies */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-gray-400 text-sm">Kinolar</label>
            <button
              type="button"
              onClick={() => setShowMovieSearch(!showMovieSearch)}
              className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400"
            >
              <Plus size={16} />
              Qo'shish
            </button>
          </div>

          {/* Movie Search */}
          {showMovieSearch && (
            <div className="mb-4 p-4 bg-brand-card border border-brand-border rounded-lg">
              <div className="relative">
                <Search
                  className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                  size={18}
                />
                <input
                  type="text"
                  placeholder="Qidirish..."
                  value={movieSearch}
                  onChange={(e) => setMovieSearch(e.target.value)}
                  className="w-full pl-10 pr-4 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
              {/* Filters: country / year / genre */}
              <div className="mt-2 grid grid-cols-2 sm:grid-cols-3 gap-2">
                <select
                  value={movieCountry}
                  onChange={(e) => setMovieCountry(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha davlatlar</option>
                  {movieCountries.map((c) => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
                <select
                  value={movieYear}
                  onChange={(e) => setMovieYear(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha yillar</option>
                  {movieYears.map((y) => (
                    <option key={y} value={y}>{y}</option>
                  ))}
                </select>
                <select
                  value={movieGenre}
                  onChange={(e) => setMovieGenre(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha janrlar</option>
                  {movieGenres.map((g) => (
                    <option key={g} value={g}>{localizeSingleGenre(g)}</option>
                  ))}
                </select>
              </div>
              {(movieCountry || movieYear || movieGenre || movieSearch) && (
                <div className="mt-2 flex items-center justify-between text-xs text-gray-500">
                  <span>{filteredMovies.filter((m) => !form.movie_ids?.includes(m.id)).length} ta topildi</span>
                  <button
                    type="button"
                    onClick={() => { setMovieSearch(""); setMovieCountry(""); setMovieYear(""); setMovieGenre(""); }}
                    className="text-brand-red hover:text-orange-400"
                  >
                    Tozalash
                  </button>
                </div>
              )}
              <div className="mt-2 max-h-48 overflow-y-auto space-y-1">
                {filteredMovies
                  .filter((m) => !form.movie_ids?.includes(m.id))
                  .map((movie) => (
                    <button
                      key={movie.id}
                      type="button"
                      onClick={() => addMovie(movie.id)}
                      className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-brand-border rounded flex items-center justify-between gap-2"
                    >
                      <span className="truncate">
                        {movie.title}
                        {movie.year ? <span className="text-gray-500 ml-1">({movie.year})</span> : null}
                        {movie.country ? <span className="text-gray-600 ml-1">· {movie.country}</span> : null}
                      </span>
                      <Plus size={14} className="flex-shrink-0" />
                    </button>
                  ))}
                {filteredMovies.filter((m) => !form.movie_ids?.includes(m.id)).length === 0 && (
                  <p className="text-gray-500 text-sm px-3 py-2">Hech narsa topilmadi</p>
                )}
              </div>
            </div>
          )}

          {/* Selected Movies */}
          {selectedMovies.length > 0 ? (
            <div className="space-y-2">
              {selectedMovies.map((movie, index) => (
                <div
                  key={movie.id}
                  className="flex items-center gap-3 p-3 bg-brand-card border border-brand-border rounded-lg"
                >
                  <GripVertical className="text-gray-600 cursor-move" size={16} />
                  <div className="flex-1">
                    <span className="text-white text-sm">{movie.title}</span>
                    <span className="text-gray-500 text-xs ml-2">({movie.year})</span>
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => moveMovie(index, "up")}
                      disabled={index === 0}
                      className="p-1 text-gray-400 hover:text-white disabled:opacity-30"
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      onClick={() => moveMovie(index, "down")}
                      disabled={index === selectedMovies.length - 1}
                      className="p-1 text-gray-400 hover:text-white disabled:opacity-30"
                    >
                      ↓
                    </button>
                    <button
                      type="button"
                      onClick={() => removeMovie(movie.id)}
                      className="p-1 text-gray-400 hover:text-red-500"
                    >
                      <X size={16} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-gray-500 text-sm">Kinolar tanlanmagan</p>
          )}
        </div>

        {/* Series */}
        <div>
          <div className="flex items-center justify-between mb-2">
            <label className="text-gray-400 text-sm">Seriallar</label>
            <button
              type="button"
              onClick={() => setShowSeriesSearch(!showSeriesSearch)}
              className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400"
            >
              <Plus size={16} />
              Qo'shish
            </button>
          </div>

          {showSeriesSearch && (
            <div className="mb-4 p-4 bg-brand-card border border-brand-border rounded-lg">
              <div className="relative">
                <Search
                  className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
                  size={18}
                />
                <input
                  type="text"
                  placeholder="Serial qidirish..."
                  value={seriesSearch}
                  onChange={(e) => setSeriesSearch(e.target.value)}
                  className="w-full pl-10 pr-4 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                />
              </div>
              {/* Filters: country / year / genre */}
              <div className="mt-2 grid grid-cols-2 sm:grid-cols-3 gap-2">
                <select
                  value={seriesCountry}
                  onChange={(e) => setSeriesCountry(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha davlatlar</option>
                  {seriesCountries.map((c) => (
                    <option key={c} value={c}>{c}</option>
                  ))}
                </select>
                <select
                  value={seriesYear}
                  onChange={(e) => setSeriesYear(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha yillar</option>
                  {seriesYears.map((y) => (
                    <option key={y} value={y}>{y}</option>
                  ))}
                </select>
                <select
                  value={seriesGenre}
                  onChange={(e) => setSeriesGenre(e.target.value)}
                  className="px-2 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                >
                  <option value="">Barcha janrlar</option>
                  {seriesGenres.map((g) => (
                    <option key={g} value={g}>{localizeSingleGenre(g)}</option>
                  ))}
                </select>
              </div>
              {(seriesCountry || seriesYear || seriesGenre || seriesSearch) && (
                <div className="mt-2 flex items-center justify-between text-xs text-gray-500">
                  <span>{filteredSeries.filter((s) => !form.series_ids?.includes(s.id)).length} ta topildi</span>
                  <button
                    type="button"
                    onClick={() => { setSeriesSearch(""); setSeriesCountry(""); setSeriesYear(""); setSeriesGenre(""); }}
                    className="text-brand-red hover:text-orange-400"
                  >
                    Tozalash
                  </button>
                </div>
              )}
              <div className="mt-2 max-h-48 overflow-y-auto space-y-1">
                {filteredSeries
                  .filter((s) => !form.series_ids?.includes(s.id))
                  .map((series) => (
                    <button
                      key={series.id}
                      type="button"
                      onClick={() => addSeries(series.id)}
                      className="w-full text-left px-3 py-2 text-sm text-gray-300 hover:bg-brand-border rounded flex items-center justify-between gap-2"
                    >
                      <span className="truncate">
                        {series.title}
                        {series.year ? <span className="text-gray-500 ml-1">({series.year})</span> : null}
                        {series.country ? <span className="text-gray-600 ml-1">· {series.country}</span> : null}
                      </span>
                      <Plus size={14} className="flex-shrink-0" />
                    </button>
                  ))}
                {filteredSeries.filter((s) => !form.series_ids?.includes(s.id)).length === 0 && (
                  <p className="text-gray-500 text-sm px-3 py-2">Hech narsa topilmadi</p>
                )}
              </div>
            </div>
          )}

          {selectedSeries.length > 0 ? (
            <div className="space-y-2">
              {selectedSeries.map((series, index) => (
                <div
                  key={series.id}
                  className="flex items-center gap-3 p-3 bg-brand-card border border-brand-border rounded-lg"
                >
                  <GripVertical className="text-gray-600 cursor-move" size={16} />
                  <div className="flex-1">
                    <span className="text-white text-sm">{series.title}</span>
                    {series.year ? (
                      <span className="text-gray-500 text-xs ml-2">({series.year})</span>
                    ) : null}
                  </div>
                  <div className="flex items-center gap-1">
                    <button
                      type="button"
                      onClick={() => moveSeries(index, "up")}
                      disabled={index === 0}
                      className="p-1 text-gray-400 hover:text-white disabled:opacity-30"
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      onClick={() => moveSeries(index, "down")}
                      disabled={index === selectedSeries.length - 1}
                      className="p-1 text-gray-400 hover:text-white disabled:opacity-30"
                    >
                      ↓
                    </button>
                    <button
                      type="button"
                      onClick={() => removeSeries(series.id)}
                      className="p-1 text-gray-400 hover:text-red-500"
                    >
                      <X size={16} />
                    </button>
                  </div>
                </div>
              ))}
            </div>
          ) : (
            <p className="text-gray-500 text-sm">Seriallar tanlanmagan</p>
          )}
        </div>

        {/* Submit */}
        <div className="flex justify-end gap-4 pt-4">
          <Link
            href="/admin/collections"
            className="px-6 py-3 text-gray-400 hover:text-white transition-colors"
          >
            Bekor qilish
          </Link>
          <button
            type="submit"
            disabled={saving}
            className="flex items-center gap-2 bg-brand-red text-white px-6 py-3 rounded-lg hover:bg-red-600 transition-colors disabled:opacity-50"
          >
            {saving ? (
              <Loader2 size={20} className="animate-spin" />
            ) : (
              <Save size={20} />
            )}
            Saqlash
          </button>
        </div>
      </form>
    </div>
  );
}
