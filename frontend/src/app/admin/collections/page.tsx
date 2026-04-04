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
  Eye,
  EyeOff,
  Star,
  StarOff,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getAdminCollections,
  deleteCollection,
  CollectionInput,
} from "@/lib/api";

export default function AdminCollectionsPage() {
  const { token } = useAuth();
  const [collections, setCollections] = useState<CollectionInput[]>([]);
  const [filtered, setFiltered] = useState<CollectionInput[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);

  const fetchCollections = async () => {
    if (!token) return;
    try {
      const data = await getAdminCollections(token);
      setCollections(data || []);
      setFiltered(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchCollections();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  // Client-side filter
  useEffect(() => {
    if (!search.trim()) {
      setFiltered(collections);
    } else {
      const q = search.toLowerCase();
      setFiltered(
        collections.filter(
          (c) =>
            c.title.toLowerCase().includes(q) ||
            c.slug.toLowerCase().includes(q)
        )
      );
    }
  }, [search, collections]);

  const handleDelete = async (collection: CollectionInput) => {
    if (
      !confirm(
        `Haqiqatan ham "${collection.title}" kolleksiyani o'chirmoqchimisiz? Bu amalni bekor qilib bo'lmaydi.`
      )
    )
      return;

    setDeleting(collection.id || "");
    try {
      await deleteCollection(token!, collection.id!);
      setCollections((prev) => prev.filter((c) => c.id !== collection.id));
    } catch (err) {
      alert(err instanceof Error ? err.message : "O'chirishda xato");
    } finally {
      setDeleting(null);
    }
  };

  // Helper to get movie count from movie_ids
  const getMovieCount = (collection: CollectionInput) => {
    return collection.movie_ids?.length || 0;
  };

  return (
    <div className="p-4 sm:p-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-8">
        <div>
          <h1 className="text-2xl font-bold text-white">Kolleksiyalar</h1>
          <p className="text-gray-400 mt-1">
            Boshqaruv: {filtered.length} kolleksiya
          </p>
        </div>
        <Link
          href="/admin/collections/new"
          className="flex items-center gap-2 bg-brand-red text-white px-4 py-2 rounded-lg hover:bg-red-600 transition-colors"
        >
          <PlusCircle size={20} />
          Yangi kolleksiya
        </Link>
      </div>

      {/* Search */}
      <div className="relative mb-6">
        <Search
          className="absolute left-3 top-1/2 -translate-y-1/2 text-gray-400"
          size={20}
        />
        <input
          type="text"
          placeholder="Qidirish..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-10 pr-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
        />
      </div>

      {/* Table */}
      {loading ? (
        <div className="flex justify-center py-12">
          <Loader2 className="animate-spin text-brand-red" size={32} />
        </div>
      ) : filtered.length === 0 ? (
        <div className="text-center py-12">
          <p className="text-gray-500">Kolleksiyalar topilmadi</p>
        </div>
      ) : (
        <div className="bg-brand-card rounded-xl border border-brand-border overflow-hidden">
          <table className="w-full">
            <thead>
              <tr className="border-b border-brand-border">
                <th className="text-left px-4 py-3 text-gray-400 font-medium text-sm">
                  Nomi
                </th>
                <th className="text-left px-4 py-3 text-gray-400 font-medium text-sm">
                  Slug
                </th>
                <th className="text-center px-4 py-3 text-gray-400 font-medium text-sm">
                  Kinolar
                </th>
                <th className="text-center px-4 py-3 text-gray-400 font-medium text-sm">
                  Holat
                </th>
                <th className="text-center px-4 py-3 text-gray-400 font-medium text-sm">
                  Tanlangan
                </th>
                <th className="text-right px-4 py-3 text-gray-400 font-medium text-sm">
                  Amallar
                </th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((collection) => (
                <tr
                  key={collection.id}
                  className="border-b border-brand-border last:border-0 hover:bg-brand-border/30"
                >
                  <td className="px-4 py-4">
                    <div className="font-medium text-white">{collection.title}</div>
                    {collection.description && (
                      <div className="text-gray-500 text-sm mt-0.5 line-clamp-1">
                        {collection.description}
                      </div>
                    )}
                  </td>
                  <td className="px-4 py-4">
                    <code className="text-gray-400 text-sm">{collection.slug}</code>
                  </td>
                  <td className="px-4 py-4 text-center text-gray-300">
                    {getMovieCount(collection)}
                  </td>
                  <td className="px-4 py-4 text-center">
                    {collection.is_published ? (
                      <span className="inline-flex items-center gap-1 text-green-400 text-sm">
                        <Eye size={16} /> Chop etilgan
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-gray-500 text-sm">
                        <EyeOff size={16} /> Noshir
                      </span>
                    )}
                  </td>
                  <td className="px-4 py-4 text-center">
                    {collection.is_featured ? (
                      <Star className="inline text-yellow-400" size={16} />
                    ) : (
                      <StarOff className="inline text-gray-600" size={16} />
                    )}
                  </td>
                  <td className="px-4 py-4">
                    <div className="flex items-center justify-end gap-2">
                      <Link
                        href={`/admin/collections/${collection.id}/edit`}
                        className="p-2 text-gray-400 hover:text-white hover:bg-brand-border rounded-lg transition-colors"
                        title="Tahrirlash"
                      >
                        <Pencil size={18} />
                      </Link>
                      <Link
                        href={`/collections/${collection.slug}`}
                        target="_blank"
                        className="p-2 text-gray-400 hover:text-brand-red hover:bg-brand-border rounded-lg transition-colors"
                        title="Ko'rish"
                      >
                        <ExternalLink size={18} />
                      </Link>
                      <button
                        onClick={() => handleDelete(collection)}
                        disabled={deleting === collection.id}
                        className="p-2 text-gray-400 hover:text-red-500 hover:bg-brand-border rounded-lg transition-colors disabled:opacity-50"
                        title="O'chirish"
                      >
                        {deleting === collection.id ? (
                          <Loader2 size={18} className="animate-spin" />
                        ) : (
                          <Trash2 size={18} />
                        )}
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
