"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Folder, Plus, Trash2, Loader2, Upload, X } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminListContentFolders,
  adminCreateContentFolder,
  adminDeleteContentFolder,
  adminUploadContentPoster,
  ContentFolder,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";

export default function AdminContentPage() {
  const { token } = useAuth();
  const [folders, setFolders] = useState<ContentFolder[]>([]);
  const [loading, setLoading] = useState(true);
  const [showCreate, setShowCreate] = useState(false);

  // Create form
  const [title, setTitle] = useState("");
  const [description, setDescription] = useState("");
  const [posterURL, setPosterURL] = useState("");
  const [uploadingPoster, setUploadingPoster] = useState(false);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reload = async () => {
    if (!token) return;
    try {
      const data = await adminListContentFolders(token);
      setFolders(data);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Failed to load");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    void reload();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  const resetForm = () => {
    setTitle("");
    setDescription("");
    setPosterURL("");
    setError(null);
  };

  const handlePosterUpload = async (file: File) => {
    if (!token) return;
    setUploadingPoster(true);
    try {
      const res = await adminUploadContentPoster(token, file);
      setPosterURL(res.url);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Poster upload failed");
    } finally {
      setUploadingPoster(false);
    }
  };

  const handleCreate = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !title.trim()) return;
    setSaving(true);
    setError(null);
    try {
      await adminCreateContentFolder(token, {
        title: title.trim(),
        poster_url: posterURL || undefined,
        description: description.trim() || undefined,
      });
      setShowCreate(false);
      resetForm();
      await reload();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Create failed");
    } finally {
      setSaving(false);
    }
  };

  const handleDelete = async (folder: ContentFolder) => {
    if (!token) return;
    const ok = window.confirm(`"${folder.title}" papkasini va uning ichidagi barcha cliplarni o'chirishni tasdiqlaysizmi?`);
    if (!ok) return;
    try {
      await adminDeleteContentFolder(token, folder.id);
      await reload();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  };

  return (
    <div className="p-4 sm:p-8">
      <div className="flex items-center justify-between mb-6 sm:mb-8">
        <div>
          <h1 className="text-2xl font-bold text-white">Content</h1>
          <p className="text-sm text-gray-500 mt-1">
            CapCut'dan eksport qilingan kliplar va ularni Instagramga rejalashtirish.
          </p>
        </div>
        <button
          onClick={() => setShowCreate(true)}
          className="flex items-center gap-2 bg-brand-red hover:bg-red-600 text-white px-4 py-2 rounded-lg transition-colors"
        >
          <Plus size={18} />
          Yangi folder
        </button>
      </div>

      {loading ? (
        <div className="flex justify-center py-16">
          <Loader2 className="animate-spin text-brand-red" size={32} />
        </div>
      ) : folders.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl py-16 text-center">
          <Folder size={48} className="text-gray-600 mx-auto mb-3" />
          <p className="text-gray-500">Hali folder yo'q. Birinchi folderni yarating.</p>
        </div>
      ) : (
        <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-4">
          {folders.map((folder) => (
            <div
              key={folder.id}
              className="group relative overflow-hidden rounded-xl bg-brand-card border border-brand-border hover:border-brand-red/60 transition-colors"
            >
              <Link
                href={`/admin/content/${folder.id}`}
                className="block"
              >
                <div className="relative aspect-[2/3] bg-brand-dark overflow-hidden">
                  {folder.poster_url ? (
                    <MediaImage
                      src={normalizeMediaUrl(folder.poster_url)}
                      alt={folder.title}
                      className="absolute inset-0 h-full w-full object-cover transition-transform group-hover:scale-105"
                    />
                  ) : (
                    <div className="absolute inset-0 flex items-center justify-center text-gray-700">
                      <Folder size={48} />
                    </div>
                  )}
                  <div className="absolute inset-0 bg-gradient-to-t from-black/85 via-black/30 to-transparent" />
                  <div className="absolute top-2 right-2 rounded-full bg-black/70 px-2 py-0.5 text-[11px] text-white backdrop-blur-sm">
                    {folder.clips_count} clip
                  </div>
                  <div className="absolute inset-x-0 bottom-0 p-3">
                    <h3 className="text-base font-semibold text-white line-clamp-2">{folder.title}</h3>
                    {folder.description && (
                      <p className="mt-1 text-[11px] text-gray-300 line-clamp-2">{folder.description}</p>
                    )}
                  </div>
                </div>
              </Link>
              <button
                onClick={() => handleDelete(folder)}
                className="absolute top-2 left-2 p-1.5 rounded-full bg-black/70 text-gray-300 hover:text-red-400 opacity-0 group-hover:opacity-100 transition-opacity"
                title="O'chirish"
              >
                <Trash2 size={14} />
              </button>
            </div>
          ))}
        </div>
      )}

      {/* Create folder modal */}
      {showCreate && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
          <div className="w-full max-w-md rounded-xl bg-brand-card border border-brand-border p-6">
            <div className="flex items-center justify-between mb-5">
              <h2 className="text-lg font-semibold text-white">Yangi folder</h2>
              <button
                onClick={() => {
                  setShowCreate(false);
                  resetForm();
                }}
                className="text-gray-400 hover:text-white"
              >
                <X size={18} />
              </button>
            </div>

            {error && (
              <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/40 text-sm text-red-400">
                {error}
              </div>
            )}

            <form onSubmit={handleCreate} className="space-y-4">
              <div>
                <label className="block text-xs text-gray-400 mb-1">Nomi *</label>
                <input
                  type="text"
                  value={title}
                  onChange={(e) => setTitle(e.target.value)}
                  className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                  required
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Tavsif</label>
                <textarea
                  value={description}
                  onChange={(e) => setDescription(e.target.value)}
                  rows={2}
                  className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red resize-none"
                />
              </div>
              <div>
                <label className="block text-xs text-gray-400 mb-1">Poster</label>
                <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
                  {uploadingPoster ? <Loader2 size={16} className="animate-spin" /> : <Upload size={16} />}
                  <span>
                    {uploadingPoster
                      ? "Yuklanmoqda..."
                      : posterURL
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
                {posterURL && (
                  <MediaImage
                    src={normalizeMediaUrl(posterURL)}
                    alt="Poster"
                    className="mt-2 h-24 rounded object-cover border border-brand-border"
                  />
                )}
              </div>

              <div className="flex justify-end gap-2 pt-2">
                <button
                  type="button"
                  onClick={() => {
                    setShowCreate(false);
                    resetForm();
                  }}
                  className="px-4 py-2 text-sm text-gray-300 hover:text-white"
                >
                  Bekor qilish
                </button>
                <button
                  type="submit"
                  disabled={saving || !title.trim()}
                  className="flex items-center gap-2 bg-brand-red text-white px-4 py-2 rounded-lg hover:bg-red-600 disabled:opacity-50 text-sm"
                >
                  {saving && <Loader2 size={14} className="animate-spin" />}
                  Yaratish
                </button>
              </div>
            </form>
          </div>
        </div>
      )}
    </div>
  );
}
