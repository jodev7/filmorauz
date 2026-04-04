"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Save, Loader2, Film } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { adminCreateSeries, CreateSeriesData } from "@/lib/api";

export default function NewSeriesPage() {
  const { token } = useAuth();
  const router = useRouter();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

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
    genres: [],
    country: "",
    is_premium: false,
  });

  const [genreInput, setGenreInput] = useState("");

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

    // Add genres
    if (genreInput.trim()) {
      const genres = genreInput
        .split(",")
        .map((g) => g.trim())
        .filter((g) => g);
      form.genres = genres;
    }

    setSaving(true);
    setError("");

    try {
      const series = await adminCreateSeries(token, form);
      router.push("/admin/series");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
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
            <input
              type="url"
              value={form.poster_url || ""}
              onChange={(e) =>
                setForm({ ...form, poster_url: e.target.value })
              }
              className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              placeholder="https://..."
            />
          </div>
          <div>
            <label className="block text-sm font-medium text-gray-300 mb-2">
              Backdrop URL
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
            Genres (comma separated)
          </label>
          <input
            type="text"
            value={genreInput}
            onChange={(e) => setGenreInput(e.target.value)}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            placeholder="Drama, Mystery, Comedy"
          />
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
