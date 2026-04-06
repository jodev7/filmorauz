"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { Film, ExternalLink, Play, Clock, Calendar, Loader2 } from "lucide-react";
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
}

export default function AdminClipsPage() {
  const { token } = useAuth();
  const [clips, setClips] = useState<Clip[]>([]);
  const [loading, setLoading] = useState(true);

  const fetchClips = async () => {
    if (!token) return;
    try {
      const res = await fetch(`${process.env.NEXT_PUBLIC_API_URL || ''}/api/admin/clips?limit=100`, {
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
  };

  useEffect(() => {
    fetchClips();
  }, [token]);

  const formatDuration = (seconds: number) => {
    const mins = Math.floor(seconds / 60);
    const secs = seconds % 60;
    return `${mins}:${secs.toString().padStart(2, "0")}`;
  };

  const formatDate = (dateStr: string) => {
    const date = new Date(dateStr);
    return date.toLocaleDateString("uz-UZ", {
      year: "numeric",
      month: "short",
      day: "numeric",
      hour: "2-digit",
      minute: "2-digit",
    });
  };

  const groupClipsByMovie = (clips: Clip[]) => {
    const grouped: Record<string, Clip[]> = {};
    clips.forEach((clip) => {
      const key = clip.movie_id;
      if (!grouped[key]) {
        grouped[key] = [];
      }
      grouped[key].push(clip);
    });
    return grouped;
  };

  const groupedClips = groupClipsByMovie(clips);

  return (
    <div className="p-4 sm:p-8">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-white">Klip</h1>
          <p className="text-gray-500 text-sm mt-1">
            {clips.length} ta kip mavjud
          </p>
        </div>
      </div>

      {loading ? (
        <div className="flex items-center gap-2 text-gray-500 py-12 justify-center">
          <Loader2 size={18} className="animate-spin" />
          Klippler yuklanmoqda...
        </div>
      ) : clips.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl p-8 sm:p-12 text-center">
          <Film className="mx-auto text-gray-600 mb-4" size={48} />
          <p className="text-gray-500">Hali kip yaratilmagan.</p>
          <p className="text-gray-600 text-sm mt-2">
            Kinolar qoshilganda avtomatik ravishda kip yaratiladi.
          </p>
        </div>
      ) : (
        <div className="space-y-8">
          {Object.entries(groupedClips).map(([movieId, movieClips]) => (
            <div key={movieId} className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="px-4 sm:px-6 py-4 border-b border-brand-border bg-brand-dark/50">
                <div className="flex items-center justify-between">
                  <div>
                    <Link
                      href={`/admin/movies`}
                      className="text-white font-medium hover:text-brand-red transition-colors"
                    >
                      {movieClips[0].movie_title}
                    </Link>
                    <div className="flex items-center gap-3 mt-1 text-sm text-gray-500">
                      <span className="font-mono">#{movieClips[0].movie_code}</span>
                      <span>{movieClips.length} ta kip</span>
                      <span>{formatDate(movieClips[0].created_at)}</span>
                    </div>
                  </div>
                  <Link
                    href={`/movies/${movieClips[0].movie_slug}`}
                    target="_blank"
                    className="text-gray-500 hover:text-white transition-colors"
                    title="Kinoni sahifasida korish"
                  >
                    <ExternalLink size={18} />
                  </Link>
                </div>
              </div>

              <div className="overflow-x-auto">
                <table className="w-full text-sm">
                  <thead>
                    <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                      <th className="text-left px-4 py-3">#</th>
                      <th className="text-left px-4 py-3">Fayl</th>
                      <th className="text-left px-4 py-3">Davomiyligi</th>
                      <th className="text-left px-4 py-3">Saxnalash</th>
                      <th className="text-left px-4 py-3">Havola</th>
                    </tr>
                  </thead>
                  <tbody>
                    {movieClips
                      .sort((a, b) => a.sequence - b.sequence)
                      .map((clip) => (
                        <tr
                          key={clip.id}
                          className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors"
                        >
                          <td className="px-4 py-3 text-gray-500">
                            {clip.sequence}
                          </td>
                          <td className="px-4 py-3">
                            <span className="text-gray-300 font-mono text-xs">
                              {clip.filename}
                            </span>
                          </td>
                          <td className="px-4 py-3">
                            <div className="flex items-center gap-1.5 text-gray-400">
                              <Clock size={14} />
                              <span>{formatDuration(clip.duration)}</span>
                            </div>
                          </td>
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
                          <td className="px-4 py-3">
                            <a
                              href={clip.url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="text-brand-red hover:text-orange-400 transition-colors"
                              title="Yangi tabda ochish"
                            >
                              <ExternalLink size={16} />
                            </a>
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
    </div>
  );
}
