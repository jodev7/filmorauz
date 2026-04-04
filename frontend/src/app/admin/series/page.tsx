"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import {
  PlusCircle,
  Pencil,
  Trash2,
  Search,
  Loader2,
  Film,
  Tv,
  Star,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetSeries,
  adminDeleteSeries,
  AdminSeries,
} from "@/lib/api";

export default function AdminSeriesPage() {
  const { token } = useAuth();
  const [series, setSeries] = useState<AdminSeries[]>([]);
  const [filtered, setFiltered] = useState<AdminSeries[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [deleting, setDeleting] = useState<string | null>(null);

  const fetchSeries = async () => {
    if (!token) return;
    try {
      const data = await adminGetSeries(token);
      setSeries(data || []);
      setFiltered(data || []);
    } catch (err) {
      console.error(err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchSeries();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [token]);

  // Client-side filter
  useEffect(() => {
    if (!search.trim()) {
      setFiltered(series);
    } else {
      const q = search.toLowerCase();
      setFiltered(
        series.filter(
          (s) =>
            s.title.toLowerCase().includes(q) ||
            s.slug.toLowerCase().includes(q) ||
            s.genre?.some((g) => g.toLowerCase().includes(q))
        )
      );
    }
  }, [search, series]);

  const handleDelete = async (s: AdminSeries) => {
    if (
      !confirm(
        `Haqiqatan ham "${s.title}" serialni o'chirmoqchimisiz? Bu amalni bekor qilib bo'lmaydi.`
      )
    )
      return;

    setDeleting(s.id);
    try {
      await adminDeleteSeries(token!, s.id);
      setSeries((prev) => prev.filter((item) => item.id !== s.id));
    } catch (err) {
      alert(err instanceof Error ? err.message : "O'chirishda xato");
    } finally {
      setDeleting(null);
    }
  };

  return (
    <div className="p-4 sm:p-8">
      {/* Header */}
      <div className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4 mb-6">
        <div>
          <h1 className="text-2xl font-bold text-white">Seriallar</h1>
          <p className="text-gray-400 text-sm mt-1">
            {filtered.length} ta serial
          </p>
        </div>
        <Link
          href="/admin/series/new"
          className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white px-4 py-2 rounded-lg font-medium transition-colors"
        >
          <PlusCircle className="w-5 h-5" />
          Yangi serial
        </Link>
      </div>

      {/* Search */}
      <div className="relative mb-6">
        <Search className="absolute left-3 top-1/2 -translate-y-1/2 w-5 h-5 text-gray-400" />
        <input
          type="text"
          placeholder="Seriallarni qidirish..."
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          className="w-full pl-10 pr-4 py-3 bg-brand-card border border-brand-border rounded-lg text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
        />
      </div>

      {/* Loading */}
      {loading && (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-8 h-8 text-brand-red animate-spin" />
        </div>
      )}

      {/* Empty */}
      {!loading && filtered.length === 0 && (
        <div className="text-center py-12">
          <Film className="w-16 h-16 text-gray-600 mx-auto mb-4" />
          <p className="text-gray-400 text-lg">Seriallar topilmadi</p>
          <Link
            href="/admin/series/new"
            className="inline-block mt-4 text-brand-red hover:underline"
          >
            Yangi serial qo'shing
          </Link>
        </div>
      )}

      {/* Table */}
      {!loading && filtered.length > 0 && (
        <div className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
          <div className="overflow-x-auto">
            <table className="w-full">
              <thead className="bg-brand-dark/50">
                <tr>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Poster
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Title
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Year
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Genre
                  </th>
                  <th className="text-left text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Status
                  </th>
                  <th className="text-right text-xs font-medium text-gray-400 uppercase tracking-wider px-4 py-3">
                    Actions
                  </th>
                </tr>
              </thead>
              <tbody className="divide-y divide-brand-border">
                {filtered.map((s) => (
                  <tr key={s.id} className="hover:bg-brand-dark/30">
                    <td className="px-4 py-3">
                      <div className="w-10 h-14 bg-gray-800 rounded overflow-hidden">
                        {s.poster_url ? (
                          <img
                            src={s.poster_url}
                            alt={s.title}
                            className="w-full h-full object-cover"
                          />
                        ) : (
                          <div className="w-full h-full flex items-center justify-center">
                            <Film className="w-5 h-5 text-gray-600" />
                          </div>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      <div>
                        <p className="font-medium text-white">{s.title}</p>
                        <p className="text-sm text-gray-500">/{s.slug}</p>
                      </div>
                    </td>
                    <td className="px-4 py-3 text-gray-300">
                      {s.year}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex flex-wrap gap-1">
                        {s.genre?.slice(0, 2).map((g, i) => (
                          <span
                            key={i}
                            className="px-2 py-0.5 bg-brand-dark text-gray-300 text-xs rounded"
                          >
                            {g}
                          </span>
                        ))}
                        {s.genre && s.genre.length > 2 && (
                          <span className="text-gray-500 text-xs">+{s.genre.length - 2}</span>
                        )}
                      </div>
                    </td>
                    <td className="px-4 py-3">
                      {s.is_premium ? (
                        <span className="inline-flex items-center gap-1 px-2 py-1 bg-yellow-900/50 text-yellow-400 text-xs rounded">
                          <Star className="w-3 h-3" />
                          Premium
                        </span>
                      ) : (
                        <span className="text-gray-500 text-xs">Free</span>
                      )}
                    </td>
                    <td className="px-4 py-3">
                      <div className="flex items-center justify-end gap-2">
                        <Link
                          href={`/admin/series/${s.id}/edit`}
                          className="p-2 text-gray-400 hover:text-white hover:bg-brand-dark rounded-lg transition-colors"
                          title="Tahrirlash"
                        >
                          <Pencil className="w-4 h-4" />
                        </Link>
                        <Link
                          href={`/series/${s.slug}`}
                          target="_blank"
                          className="p-2 text-gray-400 hover:text-white hover:bg-brand-dark rounded-lg transition-colors"
                          title="Ko'rish"
                        >
                          <Tv className="w-4 h-4" />
                        </Link>
                        <button
                          onClick={() => handleDelete(s)}
                          disabled={deleting === s.id}
                          className="p-2 text-gray-400 hover:text-red-400 hover:bg-brand-dark rounded-lg transition-colors disabled:opacity-50"
                          title="O'chirish"
                        >
                          {deleting === s.id ? (
                            <Loader2 className="w-4 h-4 animate-spin" />
                          ) : (
                            <Trash2 className="w-4 h-4" />
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
