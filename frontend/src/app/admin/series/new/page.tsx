"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Save, Loader2, Film, Plus, X, Upload } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminCreateSeries, CreateSeriesData, uploadSeriesImage } from "@/lib/api";

// Genre options for series (lowercase, matching DB)
const GENRE_OPTIONS = [
  "action", "adventure", "animation", "comedy", "crime",
  "documentary", "drama", "fantasy", "horror", "mystery",
  "romance", "sci-fi", "thriller", "western",
];

export default function NewSeriesPage() {
  const { token } = useAuth();
  const router = useRouter();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [uploadingPoster, setUploadingPoster] = useState(false);
  const [uploadingBackdrop, setUploadingBackdrop] = useState(false);

  const [form, setForm] = useState<CreateSeriesData>({
    title: "",
    title_uz: "",
    title_ru: "",
    slug: "",
    description: "",
    description_uz: "",
    description_ru: "",
    poster_url: "",
    backdrop_url: "",
    year: new Date().getFullYear(),
    genre: [],
    country: "",
    is_premium: false,
  });

  const [genreInput, setGenreInput] = useState("");

  const addGenre = () => {
    const g = genreInput.trim().toLowerCase().replace(/\s+/g, "-");
    if (g && !(form.genre || []).includes(g)) {
      setForm({ ...form, genre: [...(form.genre || []), g] });
    }
    setGenreInput("");
  };

  const removeGenre = (g: string) => {
    setForm({ ...form, genre: (form.genre || []).filter((x) => x !== g) });
  };

  const toggleGenre = (g: string) => {
    if ((form.genre || []).includes(g)) {
      removeGenre(g);
    } else {
      setForm({ ...form, genre: [...(form.genre || []), g] });
    }
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    if (!form.title) {
      setError("Title is required");
      return;
    }

    // Generate slug from title if not provided
    if (!form.slug && form.title) {
      form.slug = form.title
        .toLowerCase()
        .replace(/[^a-z0-9]+/g, "-")
        .replace(/^-|-$/g, "");
    }

    setSaving(true);
    setError("");

    try {
      const normalizedGenres = Array.from(
        new Set((form.genre || []).map((g) => g.trim().toLowerCase().replace(/[_\s]+/g, "-")).filter(Boolean))
      );
      const series = await adminCreateSeries(token, { ...form, genre: normalizedGenres });
      router.push("/admin/series");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  const handleImageUpload = async (type: "poster" | "backdrop", file: File) => {
    if (!token) return;
    try {
      if (type === "poster") {
        setUploadingPoster(true);
      } else {
        setUploadingBackdrop(true);
      }
      const result = await uploadSeriesImage(token, file, type);
      setForm((prev) => ({
        ...prev,
        [type === "poster" ? "poster_url" : "backdrop_url"]: result.url,
      }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Upload failed");
    } finally {
      if (type === "poster") {
        setUploadingPoster(false);
      } else {
        setUploadingBackdrop(false);
      }
    }
  };

  return (
    <div className="p-4 sm:p-8 max-w-3xl mx-auto">
      {/* Header */}
      <div className="flex items-center gap-4 mb-6">
        <Link
          href="/admin/series"
          className="p-2 hover:bg-brand-card rounded-lg transition-colors"
        >
          <ArrowLeft className="w-5 h-5 text-gray-400" />
        </Link>
        <div>
          <h1 className="text-2xl font-bold text-white">Yangi serial</h1>
          <p className="text-gray-400 text-sm mt-1">
            Yangi serial qo'shish
          </p>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/50 border border-red-700 text-red-200 px-4 py-3 rounded-lg mb-6">
          {error}
        </div>
      )}

      <form onSubmit={handleSubmit} className="space-y-6">
        {/* Title */}
        <div>
          <label className="block text-sm font-medium text-gray-300 mb-2">
            Title *
          </label>
          <input
            type="text"
            value={form.title}
            onChange={(e) => setForm({ ...form, title: e.target.value })}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            placeholder="Serial nomi"
            required
          />
        </div>

        {/* Title translations */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Title (O'zbek)
            </label>
            <input
              type="text"
              value={form.title_uz || ""}
              onChange={(e) =>
                setForm({ ...form, title_uz: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="Serial nomi (o'zbekcha)"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Title (Русский)
            </label>
            <input
              type="text"
              value={form.title_ru || ""}
              onChange={(e) =>
                setForm({ ...form, title_ru: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="Название сериала"
            />
          </div>
        </div>

        {/* Slug */}
        <div>
          <label className="block text-sm font-medium text-gray-300 mb-2">
            Slug
          </label>
          <input
            type="text"
            value={form.slug || ""}
            onChange={(e) => setForm({ ...form, slug: e.target.value })}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            placeholder="serial-nomi"
          />
          <p className="text-gray-500 text-xs mt-1">
            Avtomatik yaratiladi, agar bo'sh qoldirilsa
          </p>
        </div>

        {/* Description */}
        <div>
          <label className="block text-sm font-medium text-gray-300 mb-2">
            Description
          </label>
          <textarea
            value={form.description}
            onChange={(e) =>
              setForm({ ...form, description: e.target.value })
            }
            rows={4}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            placeholder="Serial tavsifi"
          />
        </div>

        {/* Description translations */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Description (O'zbek)
            </label>
            <textarea
              value={form.description_uz || ""}
              onChange={(e) =>
                setForm({ ...form, description_uz: e.target.value })
              }
              rows={3}
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="Serial tavsifi (o'zbekcha)"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Description (Русский)
            </label>
            <textarea
              value={form.description_ru || ""}
              onChange={(e) =>
                setForm({ ...form, description_ru: e.target.value })
              }
              rows={3}
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="Описание сериала"
            />
          </div>
        </div>

        {/* Poster & Backdrop */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Poster URL
            </label>
            <label className="mb-2 flex items-center gap-2 cursor-pointer w-full bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
              {uploadingPoster ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
              <span>{uploadingPoster ? "Uploading..." : "Upload poster"}</span>
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) void handleImageUpload("poster", file);
                }}
              />
            </label>
            <input
              type="url"
              value={form.poster_url || ""}
              onChange={(e) =>
                setForm({ ...form, poster_url: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="https://..."
            />
            {form.poster_url ? (
              <img src={form.poster_url} alt="Poster preview" className="mt-2 h-24 rounded object-cover border border-brand-border" />
            ) : null}
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Backdrop URL
            </label>
            <label className="mb-2 flex items-center gap-2 cursor-pointer w-full bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
              {uploadingBackdrop ? <Loader2 className="w-4 h-4 animate-spin" /> : <Upload className="w-4 h-4" />}
              <span>{uploadingBackdrop ? "Uploading..." : "Upload backdrop"}</span>
              <input
                type="file"
                accept="image/jpeg,image/png,image/webp,image/gif"
                className="hidden"
                onChange={(e) => {
                  const file = e.target.files?.[0];
                  e.target.value = "";
                  if (file) void handleImageUpload("backdrop", file);
                }}
              />
            </label>
            <input
              type="url"
              value={form.backdrop_url || ""}
              onChange={(e) =>
                setForm({ ...form, backdrop_url: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="https://..."
            />
            {form.backdrop_url ? (
              <img src={form.backdrop_url} alt="Backdrop preview" className="mt-2 h-24 w-full rounded object-cover border border-brand-border" />
            ) : null}
          </div>
        </div>

        {/* Year & Country */}
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Year
            </label>
            <input
              type="number"
              value={form.year || ""}
              onChange={(e) =>
                setForm({ ...form, year: parseInt(e.target.value) || 2024 })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="2024"
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Country
            </label>
            <input
              type="text"
              value={form.country || ""}
              onChange={(e) =>
                setForm({ ...form, country: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="USA, UK, etc."
            />
          </div>
        </div>

        {/* Genres */}
        <div>
          <label className="block text-sm font-medium text-gray-300 mb-2">
            Janrlar
          </label>

          {/* Quick-select chips */}
          <div className="flex flex-wrap gap-2 mb-3">
            {GENRE_OPTIONS.map((g) => {
              const selected = (form.genre || []).includes(g);
              return (
                <button
                  key={g}
                  type="button"
                  onClick={() => toggleGenre(g)}
                  className={`text-xs px-3 py-1.5 rounded-full border transition-colors capitalize ${
                    selected
                      ? "bg-brand-red border-brand-red text-white"
                      : "border-brand-border text-gray-400 hover:border-gray-500 hover:text-white"
                  }`}
                >
                  {g}
                </button>
              );
            })}
          </div>

          {/* Custom genre input */}
          <div className="flex gap-2">
            <input
              type="text"
              value={genreInput}
              onChange={(e) => setGenreInput(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.preventDefault();
                  addGenre();
                }
              }}
              placeholder="Boshqa janr..."
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            />
            <button
              type="button"
              onClick={addGenre}
              className="px-4 py-3 bg-brand-border hover:bg-gray-600 text-white rounded-lg transition-colors"
            >
              <Plus size={18} />
            </button>
          </div>

          {/* Selected genres */}
          {(form.genre || []).length > 0 && (
            <div className="flex flex-wrap gap-2 mt-2">
              {(form.genre || []).map((g) => (
                <span
                  key={g}
                  className="flex items-center gap-1.5 text-xs bg-brand-red/20 text-brand-red border border-brand-red/30 px-2.5 py-1 rounded-full capitalize"
                >
                  {g}
                  <button
                    type="button"
                    onClick={() => removeGenre(g)}
                    className="hover:text-white"
                  >
                    <X size={12} />
                  </button>
                </span>
              ))}
            </div>
          )}
        </div>

        {/* Premium */}
        <div className="flex items-center gap-3">
          <input
            type="checkbox"
            id="is_premium"
            checked={form.is_premium}
            onChange={(e) =>
              setForm({ ...form, is_premium: e.target.checked })
            }
            className="w-5 h-5 rounded bg-brand-card border-brand-border text-brand-red focus:ring-brand-red"
          />
          <label htmlFor="is_premium" className="text-gray-300">
            Premium content
          </label>
        </div>

        {/* Submit */}
        <div className="flex gap-4 pt-4">
          <Link
            href="/admin/series"
            className="px-6 py-3 bg-brand-card border border-brand-border text-white rounded-lg font-medium hover:border-gray-500 transition-colors"
          >
            Bekor qilish
          </Link>
          <button
            type="submit"
            disabled={saving}
            className="flex-1 flex items-center justify-center gap-2 bg-brand-red hover:bg-orange-700 text-white px-6 py-3 rounded-lg font-medium transition-colors disabled:opacity-50"
          >
            {saving ? (
              <>
                <Loader2 className="w-5 h-5 animate-spin" />
                Saqlanmoqda...
              </>
            ) : (
              <>
                <Save className="w-5 h-5" />
                Saqlash
              </>
            )}
          </button>
        </div>
      </form>
    </div>
  );
}
