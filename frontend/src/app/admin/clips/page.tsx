"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  Film,
  ExternalLink,
  Clock,
  Loader2,
  Upload,
  CheckCircle,
  XCircle,
  MinusCircle,
  X,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";

interface Clip {
  id: string;
  movie_id: string;
  movie_title: string;
  movie_slug: string;
  movie_code: string;
  filename: string;
  path: string;
  url: string;
  duration: number;
  sequence: number;
  storage_type: string;
  created_at: string;
  // Instagram tracking
  uploaded_to_instagram: boolean;
  instagram_upload_count: number;
  last_instagram_upload_at?: string;
  last_instagram_upload_status: string; // "success" | "failed" | ""
}

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";

function UploadStatusBadge({ clip }: { clip: Clip }) {
  if (clip.instagram_upload_count === 0 || !clip.last_instagram_upload_status) {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-gray-800 text-gray-400 border border-gray-700">
        <MinusCircle size={11} />
        Not uploaded
      </span>
    );
  }
  if (clip.last_instagram_upload_status === "success") {
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-500/15 text-green-400 border border-green-500/30">
        <CheckCircle size={11} />
        Uploaded
      </span>
    );
  }
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-red-500/15 text-red-400 border border-red-500/30">
      <XCircle size={11} />
      Failed
    </span>
  );
}

function formatDuration(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

function formatDate(dateStr?: string) {
  if (!dateStr) return null;
  return new Date(dateStr).toLocaleDateString("uz-UZ", {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function groupByMovie(clips: Clip[]): Record<string, Clip[]> {
  const grouped: Record<string, Clip[]> = {};
  for (const clip of clips) {
    if (!grouped[clip.movie_id]) grouped[clip.movie_id] = [];
    grouped[clip.movie_id].push(clip);
  }
  return grouped;
}

interface AccountResult {
  account: string;
  status: "success" | "failed";
  error?: string;
}

interface UploadModal {
  clip: Clip;
  selectedAccounts: string[];
  results: AccountResult[] | null;
}

export default function AdminClipsPage() {
  const { token } = useAuth();
  const [clips, setClips] = useState<Clip[]>([]);
  const [loading, setLoading] = useState(true);
  const [igAccounts, setIgAccounts] = useState<string[]>([]);
  const [uploading, setUploading] = useState<Record<string, boolean>>({});
  const [modal, setModal] = useState<UploadModal | null>(null);

  const fetchClips = useCallback(async () => {
    if (!token) return;
    try {
      const res = await fetch(`${API}/admin/clips?limit=100`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setClips(data.data || []);
      }
    } catch (err) {
      console.error("Failed to fetch clips:", err);
    } finally {
      setLoading(false);
    }
  }, [token]);

  const fetchAccounts = useCallback(async () => {
    if (!token) return;
    try {
      const res = await fetch(`${API}/admin/instagram/accounts`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setIgAccounts(data.accounts || []);
      }
    } catch (err) {
      console.error("Failed to fetch Instagram accounts:", err);
    }
  }, [token]);

  useEffect(() => {
    fetchClips();
    fetchAccounts();
  }, [fetchClips, fetchAccounts]);

  const openUploadModal = (clip: Clip) => {
    setModal({ clip, selectedAccounts: igAccounts.slice(0, 1), results: null });
  };

  const toggleAccount = (name: string) => {
    if (!modal) return;
    setModal((prev) => {
      if (!prev) return prev;
      const selected = prev.selectedAccounts.includes(name)
        ? prev.selectedAccounts.filter((a) => a !== name)
        : [...prev.selectedAccounts, name];
      return { ...prev, selectedAccounts: selected };
    });
  };

  const handleUpload = async () => {
    if (!token || !modal || modal.selectedAccounts.length === 0) return;
    const { clip, selectedAccounts } = modal;
    setUploading((prev) => ({ ...prev, [clip.id]: true }));
    try {
      const res = await fetch(`${API}/admin/clips/${clip.id}/instagram`, {
        method: "POST",
        headers: {
          Authorization: `Bearer ${token}`,
          "Content-Type": "application/json",
        },
        body: JSON.stringify({ account_names: selectedAccounts }),
      });
      const data = await res.json();
      setModal((prev) => prev ? { ...prev, results: data.results || [] } : prev);
      await fetchClips();
    } catch (err) {
      console.error("Upload error:", err);
    } finally {
      setUploading((prev) => ({ ...prev, [clip.id]: false }));
    }
  };

  const groupedClips = groupByMovie(clips);

  return (
    <div className="p-4 sm:p-8">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-white">Kliplar</h1>
          <p className="text-gray-500 text-sm mt-1">{clips.length} ta klip mavjud</p>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-gray-500 py-12 justify-center">
          <Loader2 size={18} className="animate-spin" />
          Kliplar yuklanmoqda...
        </div>
      ) : clips.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl p-8 sm:p-12 text-center">
          <Film className="mx-auto text-gray-600 mb-4" size={48} />
          <p className="text-gray-500">Hali klip yaratilmagan.</p>
          <p className="text-gray-600 text-sm mt-2">
            Kinolar qo&apos;shilganda avtomatik ravishda klip yaratiladi.
          </p>
        </div>
      ) : (
        <div className="space-y-8">
          {Object.entries(groupedClips).map(([movieId, movieClips]) => (
            <div
              key={movieId}
              className="bg-brand-card border border-brand-border rounded-xl overflow-hidden"
            >
              {/* Movie header */}
              <div className="px-4 sm:px-6 py-4 border-b border-brand-border bg-brand-dark/50">
                <div className="flex items-center justify-between">
                  <div>
                    <Link
                      href="/admin/movies"
                      className="text-white font-medium hover:text-brand-red transition-colors"
                    >
                      {movieClips[0].movie_title}
                    </Link>
                    <div className="flex items-center gap-3 mt-1 text-sm text-gray-500">
                      <span className="font-mono">#{movieClips[0].movie_code}</span>
                      <span>{movieClips.length} ta klip</span>
                      <span>{formatDate(movieClips[0].created_at)}</span>
                    </div>
                  </div>
                  <Link
                    href={`/movies/${movieClips[0].movie_slug}`}
                    target="_blank"
                    className="text-gray-500 hover:text-white transition-colors"
                    title="Kinoni sahifasida ko'rish"
                  >
                    <ExternalLink size={18} />
                  </Link>
                </div>
              </div>

              {/* Clips table */}
              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                      <th className="text-left px-4 py-3">#</th>
                      <th className="text-left px-4 py-3">Fayl</th>
                      <th className="text-left px-4 py-3">Davom</th>
                      <th className="text-left px-4 py-3">Saxnalash</th>
                      <th className="text-left px-4 py-3">Instagram</th>
                      <th className="text-left px-4 py-3">Yuklashlar</th>
                      <th className="text-right px-4 py-3">Amallar</th>
                    </tr>
                  </thead>
                  <tbody>
                    {[...movieClips]
                      .sort((a, b) => a.sequence - b.sequence)
                      .map((clip) => (
                        <tr
                          key={clip.id}
                          className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors"
                        >
                          {/* Sequence */}
                          <td className="px-4 py-3 text-gray-500">{clip.sequence}</td>

                          {/* Filename */}
                          <td className="px-4 py-3">
                            <span className="text-gray-300 font-mono text-xs">
                              {clip.filename}
                            </span>
                          </td>

                          {/* Duration */}
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-1.5 text-gray-400">
                              <Clock size={14} />
                              <span>{formatDuration(clip.duration)}</span>
                            </div>
                          </td>

                          {/* Storage type */}
                          <td className="px-4 py-3">
                            <span
                              className={`text-xs px-2 py-0.5 rounded ${
                                clip.storage_type === "b2"
                                  ? "bg-blue-500/20 text-blue-400"
                                  : "bg-green-500/20 text-green-400"
                              }`}
                            >
                              {clip.storage_type === "b2" ? "B2/CDN" : "Local"}
                            </span>
                          </td>

                          {/* Instagram upload status */}
                          <td className="px-4 py-3">
                            <UploadStatusBadge clip={clip} />
                          </td>

                          {/* Upload count + last time */}
                          <td className="px-4 py-3">
                            {clip.instagram_upload_count > 0 ? (
                              <div className="flex flex-col gap-0.5">
                                <span className="text-xs text-gray-300 font-medium">
                                  {clip.instagram_upload_count}× yuklangan
                                </span>
                                {clip.last_instagram_upload_at && (
                                  <span className="text-xs text-gray-600">
                                    {formatDate(clip.last_instagram_upload_at)}
                                  </span>
                                )}
                              </div>
                            ) : (
                              <span className="text-xs text-gray-600">—</span>
                            )}
                          </td>

                          {/* Actions */}
                          <td className="px-4 py-3">
                            <div className="flex items-center justify-end gap-2">
                              {/* View clip */}
                              <a
                                href={clip.url}
                                target="_blank"
                                rel="noopener noreferrer"
                                className="text-gray-500 hover:text-gray-300 transition-colors"
                                title="Klipni ochish"
                              >
                                <ExternalLink size={14} />
                              </a>

                              {/* Instagram upload */}
                              <button
                                onClick={() => openUploadModal(clip)}
                                disabled={uploading[clip.id]}
                                title="Instagramga yuklash"
                                className={`inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors ${
                                  clip.last_instagram_upload_status === "success"
                                    ? "bg-green-500/15 text-green-400 border border-green-500/30 hover:bg-green-500/25"
                                    : clip.last_instagram_upload_status === "failed"
                                    ? "bg-red-500/15 text-red-400 border border-red-500/30 hover:bg-red-500/25"
                                    : "bg-brand-border text-gray-300 border border-brand-border hover:bg-white/10"
                                } disabled:opacity-50`}
                              >
                                {uploading[clip.id] ? (
                                  <Loader2 size={12} className="animate-spin" />
                                ) : (
                                  <Upload size={12} />
                                )}
                                {uploading[clip.id] ? "Yuklanmoqda..." : "Upload"}
                              </button>
                            </div>
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
            </div>
          ))}
        </div>
      )}

      {/* Instagram account picker modal */}
      {modal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div className="bg-brand-card border border-brand-border rounded-xl w-full max-w-sm shadow-2xl">
            {/* Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b border-brand-border">
              <div>
                <h2 className="text-white font-semibold text-sm">Instagramga yuklash</h2>
                <p className="text-gray-500 text-xs mt-0.5 truncate max-w-[220px]">
                  {modal.clip.movie_title} — klip #{modal.clip.sequence}
                </p>
              </div>
              <button
                onClick={() => setModal(null)}
                className="text-gray-500 hover:text-white transition-colors"
              >
                <X size={18} />
              </button>
            </div>

            {/* Results view */}
            {modal.results ? (
              <div className="px-5 py-4 space-y-2">
                <p className="text-gray-400 text-xs mb-3">Yuklash natijalari:</p>
                {modal.results.map((r) => (
                  <div key={r.account} className="flex items-center justify-between">
                    <span className="text-gray-300 text-sm font-mono">{r.account}</span>
                    {r.status === "success" ? (
                      <span className="inline-flex items-center gap-1 text-xs text-green-400">
                        <CheckCircle size={13} /> Muvaffaqiyatli
                      </span>
                    ) : (
                      <span className="inline-flex items-center gap-1 text-xs text-red-400" title={r.error}>
                        <XCircle size={13} /> Xato
                      </span>
                    )}
                  </div>
                ))}
                <div className="pt-3">
                  <button
                    onClick={() => setModal(null)}
                    className="w-full py-2 rounded-lg bg-brand-border text-gray-300 text-sm hover:bg-white/10 transition-colors"
                  >
                    Yopish
                  </button>
                </div>
              </div>
            ) : (
              /* Account picker view */
              <div className="px-5 py-4 space-y-3">
                {igAccounts.length === 0 ? (
                  <p className="text-gray-500 text-sm text-center py-4">
                    Instagram akkauntlar sozlanmagan.<br />
                    <span className="text-xs text-gray-600">INSTAGRAM_ACCOUNTS_JSON env o&apos;rnatilmagan.</span>
                  </p>
                ) : (
                  <>
                    <p className="text-gray-400 text-xs">Akkaunt(lar)ni tanlang:</p>
                    <div className="space-y-2">
                      {igAccounts.map((name) => {
                        const checked = modal.selectedAccounts.includes(name);
                        return (
                          <label
                            key={name}
                            className="flex items-center gap-3 px-3 py-2.5 rounded-lg border border-brand-border hover:border-gray-500 cursor-pointer transition-colors"
                          >
                            <input
                              type="checkbox"
                              checked={checked}
                              onChange={() => toggleAccount(name)}
                              className="accent-brand-red w-4 h-4 cursor-pointer"
                            />
                            <span className="text-gray-200 text-sm font-mono">{name}</span>
                          </label>
                        );
                      })}
                    </div>
                  </>
                )}

                <div className="flex gap-2 pt-1">
                  <button
                    onClick={() => setModal(null)}
                    className="flex-1 py-2 rounded-lg border border-brand-border text-gray-400 text-sm hover:bg-white/5 transition-colors"
                  >
                    Bekor
                  </button>
                  <button
                    onClick={handleUpload}
                    disabled={
                      modal.selectedAccounts.length === 0 ||
                      uploading[modal.clip.id] ||
                      igAccounts.length === 0
                    }
                    className="flex-1 py-2 rounded-lg bg-brand-red text-white text-sm font-medium hover:bg-brand-red/80 transition-colors disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-2"
                  >
                    {uploading[modal.clip.id] ? (
                      <>
                        <Loader2 size={14} className="animate-spin" />
                        Yuklanmoqda...
                      </>
                    ) : (
                      <>
                        <Upload size={14} />
                        Yuklash
                      </>
                    )}
                  </button>
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
