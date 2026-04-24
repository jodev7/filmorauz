"use client";

import { useState } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { ArrowLeft, Save, Loader2, Upload } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import { createCollection, CollectionInput, uploadCollectionPoster } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

export default function NewCollectionPage() {
  const { token } = useAuth();
  const router = useRouter();
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [uploadingPoster, setUploadingPoster] = useState(false);

  const [form, setForm] = useState<CollectionInput>({
    title: "",
    slug: "",
    description: "",
    poster_url: "",
    is_published: true, // Default to published for easier testing
    is_featured: false,
    sort_order: 0,
    movie_ids: [],
  });

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;

    setError(null);
    setSaving(true);
    try {
      await createCollection(token, form);
      router.push("/admin/collections");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

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
    setError(null);
    setUploadingPoster(true);
    try {
      const result = await uploadCollectionPoster(token, file);
      setForm((prev) => ({ ...prev, poster_url: result.url }));
    } catch (err) {
      setError(err instanceof Error ? err.message : "Poster upload failed");
    } finally {
      setUploadingPoster(false);
    }
  };

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
        <h1 className="text-2xl font-bold text-white">Yangi kolleksiya</h1>
      </div>

      {/* Error Message */}
      {error && (
        <div className="mb-6 p-4 bg-red-500/10 border border-red-500 rounded-lg text-red-400">
          {error}
        </div>
      )}

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

        {/* Poster URL */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Poster URL</label>
          <label className="mb-2 flex items-center gap-2 cursor-pointer w-full bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
            {uploadingPoster ? <Loader2 size={16} className="animate-spin" /> : <Upload size={16} />}
            <span>{uploadingPoster ? "Uploading..." : "Upload poster"}</span>
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
          <input
            type="url"
            value={form.poster_url}
            onChange={(e) => setForm({ ...form, poster_url: e.target.value })}
            className="w-full px-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
            placeholder="https://..."
          />
          {form.poster_url ? (
            <img src={normalizeMediaUrl(form.poster_url)} alt="Poster preview" className="mt-2 h-24 rounded object-cover border border-brand-border" />
          ) : null}
        </div>

        {/* Sort Order */}
        <div>
          <label className="block text-gray-400 text-sm mb-2">Tartib raqami</label>
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
