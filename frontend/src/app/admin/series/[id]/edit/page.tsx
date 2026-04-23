"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import Link from "next/link";
import {
  ArrowLeft,
  Save,
  Loader2,
  PlusCircle,
  Trash2,
  Tv,
  Play,
  ChevronDown,
  ChevronUp,
  Plus,
  X,
  Upload,
  CheckCircle,
  AlertCircle,
  GripVertical,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetSeries,
  adminUpdateSeries,
  adminCreateSeason,
  adminCreateEpisode,
  adminDeleteEpisode,
  adminUpdateEpisode,
  adminReorderEpisodes,
  adminMoveEpisodeToSeason,
  CreateSeriesData,
  UploadProgressInfo,
  UpdateEpisodeData,
} from "@/lib/api";
import { backendUploadMovieImage } from "@/lib/api";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  DragEndEvent,
} from "@dnd-kit/core";
import {
  SortableContext,
  verticalListSortingStrategy,
  useSortable,
  arrayMove,
} from "@dnd-kit/sortable";
import { CSS } from "@dnd-kit/utilities";

interface Season {
  id: string;
  series_id: string;
  season_number: number;
  title: string;
  poster_url: string;
  description: string;
}

interface Episode {
  id: string;
  series_id: string;
  season_id: string;
  episode_number: number;
  title: string;
  description: string;
  thumbnail_url: string;
  video_url: string;
  duration: number;
}

interface UploadState {
  status: "idle" | "uploading" | "success" | "error";
  message?: string;
  tempKey?: string;
  progress?: number;
  uploadedMB?: number;
  totalMB?: number;
  speedMBps?: number;
  etaSeconds?: number;
}

function formatSpeed(speedMBps?: number) {
  if (!speedMBps || !Number.isFinite(speedMBps) || speedMBps <= 0) return "";
  return `${speedMBps.toFixed(1)} MB/s`;
}

function formatETA(seconds?: number) {
  if (seconds === undefined || !Number.isFinite(seconds) || seconds <= 0) return "";
  if (seconds < 60) return `${seconds}s`;
  const minutes = Math.floor(seconds / 60);
  const remainingSeconds = seconds % 60;
  return `${minutes}m ${remainingSeconds}s`;
}

// Sortable Episode Item Component
function SortableEpisodeItem({
  episode,
  onDelete,
}: {
  episode: Episode;
  onDelete: (episodeId: string, episodeTitle: string) => void;
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
  } = useSortable({ id: episode.id });

  const style = {
    transform: CSS.Transform.toString(transform),
    transition,
  };

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className="flex items-center gap-3 p-3 border-b border-brand-border last:border-0 hover:bg-brand-dark/30 cursor-grab active:cursor-grabbing"
    >
      <GripVertical className="w-4 h-4 text-gray-500 flex-shrink-0" />
      {episode.thumbnail_url ? (
        <img src={episode.thumbnail_url} alt="" className="w-16 h-10 object-cover rounded" />
      ) : (
        <div className="w-16 h-10 bg-gray-700 rounded flex items-center justify-center">
          <Play className="w-4 h-4 text-gray-500" />
        </div>
      )}
      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium text-white truncate">
          {episode.episode_number}. {episode.title}
        </p>
        <p className="text-xs text-gray-400">
          {episode.duration ? `${episode.duration} min` : ""}
        </p>
      </div>
      <div className="flex items-center gap-2">
        <a
          href={episode.video_url || episode.thumbnail_url}
          target="_blank"
          rel="noopener noreferrer"
          className="text-brand-red hover:text-orange-400 text-sm"
          onClick={(e) => e.stopPropagation()}
        >
          Ko'rish
        </a>
        <button
          onClick={(e) => {
            e.stopPropagation();
            onDelete(episode.id, episode.title);
          }}
          className="p-1.5 text-gray-400 hover:text-red-400 hover:bg-red-500/10 rounded"
          title="O'chirish"
        >
          <Trash2 className="w-4 h-4" />
        </button>
      </div>
    </div>
  );
}


interface SeasonWithEpisodes {
  season: Season;
  episodes: Episode[];
}

// Stored lowercase English (matches DB). Displayed with `capitalize` CSS.
const GENRE_OPTIONS = [
  "action", "adventure", "animation", "comedy", "crime",
  "documentary", "drama", "fantasy", "horror", "mystery",
  "romance", "sci-fi", "thriller", "western",
];

export default function EditSeriesPage() {
  const { token } = useAuth();
  const params = useParams();
  const id = params.id as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  // Series data
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

  // Upload states for poster and backdrop
  const [uploads, setUploads] = useState<{
    poster: UploadState;
    backdrop: UploadState;
  }>({
    poster: { status: "idle" },
    backdrop: { status: "idle" },
  });

  // Seasons and episodes
  const [seasons, setSeasons] = useState<SeasonWithEpisodes[]>([]);
  const [expandedSeasons, setExpandedSeasons] = useState<Set<string>>(new Set());

  // Add season form
  const [showAddSeason, setShowAddSeason] = useState(false);
  const [newSeason, setNewSeason] = useState({
    season_number: 1,
    title: "",
    poster_url: "",
    description: "",
  });

  // Add episode form
  const [activeSeasonId, setActiveSeasonId] = useState<string | null>(null);
  
  // Delete confirmation modal
  const [deleteModal, setDeleteModal] = useState<{show: boolean; episodeId: string | null; episodeTitle: string}>({
    show: false,
    episodeId: null,
    episodeTitle: "",
  });
  const [deletingEpisode, setDeletingEpisode] = useState(false);

  const [newEpisode, setNewEpisode] = useState({
    episode_number: 1,
    title: "",
    description: "",
    thumbnail_url: "",
    video_url: "",
    duration: 0,
  });

  const [addingSeason, setAddingSeason] = useState(false);
  const [addingEpisode, setAddingEpisode] = useState(false);

  // Drag-and-drop state
  const [activeDragId, setActiveDragId] = useState<string | null>(null);
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    })
  );

  // Load series data
  useEffect(() => {
    async function loadData() {
      if (!token || !id) return;
      try {
        // Load series
        const seriesList = await adminGetSeries(token);
        const series = seriesList.find((s) => s.id === id);
        if (series) {
          setForm({
            title: series.title,
            title_uz: series.title_uz || "",
            title_ru: series.title_ru || "",
            slug: series.slug,
            description: series.description,
            description_uz: series.description_uz || "",
            description_ru: series.description_ru || "",
            poster_url: series.poster_url,
            backdrop_url: series.backdrop_url,
            year: series.year,
            genre: (series.genre || []).map((g) => g.toLowerCase().replace(/[_\s]+/g, "-")),
            country: series.country,
            is_premium: series.is_premium,
          });
        }

        // Load seasons with episodes using public API
        const seasonsRes = await fetch(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api"}/series-by-id/${id}/seasons`);
        if (seasonsRes.ok) {
          const seasonsData = await seasonsRes.json();
          const seasonsList: SeasonWithEpisodes[] = [];
          
          for (const s of seasonsData.data || []) {
            const epRes = await fetch(`${process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api"}/seasons/${s.id}/episodes`);
            const epData = epRes.ok ? await epRes.json() : { data: [] };
            seasonsList.push({
              season: s,
              episodes: epData.data || [],
            });
          }
          setSeasons(seasonsList);
        }
      } catch (err) {
        console.error(err);
        setError("Failed to load series");
      } finally {
        setLoading(false);
      }
    }
    loadData();
  }, [token, id]);

  const handleSaveSeries = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !id) return;

    if (!form.title) {
      setError("Title is required");
      return;
    }

    setSaving(true);
    setError("");

    try {
      const normalizedGenres = Array.from(
        new Set((form.genre || []).map((g) => g.trim().toLowerCase().replace(/[_\s]+/g, "-")).filter(Boolean))
      );
      await adminUpdateSeries(token, id, { ...form, genre: normalizedGenres });
      alert("Series saved!");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik yuz berdi");
    } finally {
      setSaving(false);
    }
  };

  const handleUpload = async (type: "poster" | "backdrop", file: File) => {
    if (!token) {
      setUploads(prev => ({
        ...prev,
        [type]: { status: "error", message: "No auth token available" }
      }));
      return;
    }

    setUploads(prev => ({
      ...prev,
      [type]: { status: "uploading", progress: 0 }
    }));

    const onUploadProgress = (uploadProgress: UploadProgressInfo) => {
      setUploads(prev => ({
        ...prev,
        [type]: {
          status: "uploading",
          progress: uploadProgress.progress,
          uploadedMB: uploadProgress.uploadedMB,
          totalMB: uploadProgress.total ? uploadProgress.total / 1024 / 1024 : undefined,
          speedMBps: uploadProgress.speedMBps,
          etaSeconds: uploadProgress.etaSeconds,
        }
      }));
    };

    try {
      const result = await backendUploadMovieImage(token, file, type, onUploadProgress);

      setUploads(prev => ({
        ...prev,
        [type]: { status: "success", message: result.url, tempKey: result.file_key, progress: 100 }
      }));

      if (type === "poster") {
        setForm(prev => ({ ...prev, poster_url: result.url }));
      } else {
        setForm(prev => ({ ...prev, backdrop_url: result.url }));
      }
    } catch (err) {
      setUploads(prev => ({
        ...prev,
        [type]: {
          status: "error",
          message: err instanceof Error ? err.message : "Upload failed"
        }
      }));
    }
  };

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

  const handleAddSeason = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !id) return;

    setAddingSeason(true);
    try {
      const season = await adminCreateSeason(token, id, newSeason);
      setSeasons((prev) => [
        ...prev,
        { season: season, episodes: [] },
      ]);
      setNewSeason({ season_number: 1, title: "", poster_url: "", description: "" });
      setShowAddSeason(false);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to add season");
    } finally {
      setAddingSeason(false);
    }
  };

  const handleAddEpisode = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !activeSeasonId) return;

    setAddingEpisode(true);
    try {
      const episode = await adminCreateEpisode(token, activeSeasonId, newEpisode);
      setSeasons((prev) =>
        prev.map((s) =>
          s.season.id === activeSeasonId
            ? { ...s, episodes: [...s.episodes, episode] }
            : s
        )
      );
      setNewEpisode({
        episode_number: 1,
        title: "",
        description: "",
        thumbnail_url: "",
        video_url: "",
        duration: 0,
      });
      setActiveSeasonId(null);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to add episode");
    } finally {
      setAddingEpisode(false);
    }
  };

  const handleDeleteEpisode = async () => {
    if (!token || !deleteModal.episodeId) return;

    setDeletingEpisode(true);
    try {
      await adminDeleteEpisode(token, deleteModal.episodeId);
      // Remove episode from the list
      setSeasons((prev) =>
        prev.map((s) => ({
          ...s,
          episodes: s.episodes.filter((ep) => ep.id !== deleteModal.episodeId),
        }))
      );
      setDeleteModal({ show: false, episodeId: null, episodeTitle: "" });
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete episode");
    } finally {
      setDeletingEpisode(false);
    }
  };

  // Drag-and-drop handlers
  const handleDragStart = (event: any) => {
    setActiveDragId(event.active.id);
  };

  const handleDragEnd = async (event: DragEndEvent) => {
    const { active, over } = event;
    setActiveDragId(null);

    if (!over || !token) return;

    const activeId = active.id as string;
    const overId = over.id as string;

    if (activeId === overId) return;

    // Find source and target seasons
    let sourceSeasonIndex = -1;
    let targetSeasonIndex = -1;
    let activeEpisode: Episode | null = null;

    for (let i = 0; i < seasons.length; i++) {
      const epIndex = seasons[i].episodes.findIndex((ep) => ep.id === activeId);
      if (epIndex !== -1) {
        sourceSeasonIndex = i;
        activeEpisode = seasons[i].episodes[epIndex];
      }
      if (seasons[i].episodes.some((ep) => ep.id === overId)) {
        targetSeasonIndex = i;
      }
    }

    if (!activeEpisode || sourceSeasonIndex === -1 || targetSeasonIndex === -1) return;

    const sourceSeason = seasons[sourceSeasonIndex];
    const targetSeason = seasons[targetSeasonIndex];

    // If moving within the same season
    if (sourceSeasonIndex === targetSeasonIndex) {
      const oldIndex = sourceSeason.episodes.findIndex((ep) => ep.id === activeId);
      const newIndex = sourceSeason.episodes.findIndex((ep) => ep.id === overId);

      if (oldIndex === newIndex) return;

      const newEpisodes = arrayMove(sourceSeason.episodes, oldIndex, newIndex);

      // Optimistically update UI
      setSeasons((prev) =>
        prev.map((s, idx) =>
          idx === sourceSeasonIndex ? { ...s, episodes: newEpisodes } : s
        )
      );

      // Send reorder request
      try {
        await adminReorderEpisodes(token, sourceSeason.season.id, newEpisodes.map((ep) => ep.id));
      } catch (err) {
        alert(err instanceof Error ? err.message : "Failed to reorder episodes");
        // Reload data on error
        window.location.reload();
      }
      return;
    }

    // Moving between seasons
    const newEpisodeNumber = targetSeason.episodes.length + 1;

    // Optimistically update UI
    setSeasons((prev) => {
      const newSeasons = [...prev];
      // Remove from source
      newSeasons[sourceSeasonIndex] = {
        ...newSeasons[sourceSeasonIndex],
        episodes: newSeasons[sourceSeasonIndex].episodes.filter((ep) => ep.id !== activeId),
      };
      // Add to target
      const updatedEpisode = { ...activeEpisode!, season_id: targetSeason.season.id, episode_number: newEpisodeNumber };
      newSeasons[targetSeasonIndex] = {
        ...newSeasons[targetSeasonIndex],
        episodes: [...newSeasons[targetSeasonIndex].episodes, updatedEpisode],
      };
      return newSeasons;
    });

    // Send move request
    try {
      await adminMoveEpisodeToSeason(token, activeId, targetSeason.season.id, newEpisodeNumber);
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to move episode");
      // Reload data on error
      window.location.reload();
    }
  };

  const toggleSeason = (seasonId: string) => {
    setExpandedSeasons((prev) => {
      const next = new Set(prev);
      if (next.has(seasonId)) {
        next.delete(seasonId);
      } else {
        next.add(seasonId);
      }
      return next;
    });
  };

  const renderUploadProgress = (state: UploadState, type: string) => {
    const progress = state.progress ?? 0;
    const speed = formatSpeed(state.speedMBps);
    const eta = formatETA(state.etaSeconds);

    return (
      <div className="upload-progress">
        <Loader2 size={20} className="animate-spin" />
        <div className="progress-content">
          <span>
            {state.progress !== undefined
              ? `Uploading ${type}: ${progress}%`
              : `Uploading ${type}: ${(state.uploadedMB ?? 0).toFixed(1)} MB`}
          </span>
          {state.progress !== undefined && (
            <div className="progress-bar">
              <div className="progress-fill" style={{ width: `${progress}%` }} />
            </div>
          )}
          <div className="progress-meta">
            {speed && <span>Speed: {speed}</span>}
            {eta && <span>ETA: {eta}</span>}
            {state.progress === undefined && state.uploadedMB !== undefined && (
              <span>Uploaded: {state.uploadedMB.toFixed(1)} MB</span>
            )}
          </div>
        </div>
      </div>
    );
  };

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 text-brand-red animate-spin" />
      </div>
    );
  }

  return (
    <div className="p-4 sm:p-8 max-w-5xl mx-auto">
      {/* Header */}
      <div className="flex items-center gap-4 mb-6">
        <Link
          href="/admin/series"
          className="p-2 hover:bg-brand-card rounded-lg transition-colors"
        >
          <ArrowLeft className="w-5 h-5 text-gray-400" />
        </Link>
        <div>
          <h1 className="text-2xl font-bold text-white">Tahrirlash</h1>
          <p className="text-gray-400 text-sm mt-1">{form.title}</p>
        </div>
      </div>

      {error && (
        <div className="bg-red-900/50 border border-red-700 text-red-200 px-4 py-3 rounded-lg mb-6">
          {error}
        </div>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-8">
        {/* Left: Series Info */}
        <div>
          <h2 className="text-lg font-semibold text-white mb-4">Series ma'lumotlari</h2>
          <form onSubmit={handleSaveSeries} className="space-y-4">
            {/* Title */}
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">
                Title *
              </label>
              <input
                type="text"
                value={form.title}
                onChange={(e) => setForm({ ...form, title: e.target.value })}
                className="w-full px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
                required
              />
            </div>

            {/* Slug */}
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">
                Slug <span className="text-gray-500">(lowercase, hyphens)</span>
              </label>
              <input
                type="text"
                value={form.slug}
                onChange={(e) => setForm({ ...form, slug: e.target.value.toLowerCase().replace(/\s+/g, "-").replace(/[^a-z0-9-]/g, "") })}
                className="w-full px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white font-mono focus:outline-none focus:border-brand-red"
              />
            </div>

            {/* Description */}
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-1">Description</label>
              <textarea
                value={form.description}
                onChange={(e) => setForm({ ...form, description: e.target.value })}
                rows={3}
                className="w-full px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
              />
            </div>

            {/* Poster & Backdrop - Movie-Style Uploads */}
            <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">Poster</label>
                <div className="upload-box">
                  {uploads.poster.status === "uploading" && (
                    renderUploadProgress(uploads.poster, "poster")
                  )}
                  {uploads.poster.status === "success" && (
                    <div className="upload-status success">
                      <CheckCircle size={20} />
                      <span>Uploaded successfully</span>
                    </div>
                  )}
                  {uploads.poster.status === "error" && (
                    <div className="upload-status error">
                      <AlertCircle size={20} />
                      <span>{uploads.poster.message}</span>
                    </div>
                  )}
                  {uploads.poster.status !== "uploading" && (
                    <>
                      <input
                        type="file"
                        accept="image/jpeg,image/png,image/webp"
                        onChange={(e) => {
                          const file = e.target.files?.[0];
                          if (file) handleUpload("poster", file);
                        }}
                        className="hidden"
                        id="series-poster-upload"
                      />
                      <label htmlFor="series-poster-upload" className="upload-label">
                        <Upload size={20} />
                        <span>Choose poster image (jpg, png, webp)</span>
                      </label>
                    </>
                  )}
                </div>
                {form.poster_url && (
                  <img
                    src={form.poster_url}
                    alt="Poster preview"
                    className="mt-2 h-24 rounded object-cover border border-brand-border"
                    onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
                  />
                )}
              </div>

              <div>
                <label className="block text-sm font-medium text-gray-300 mb-2">Backdrop</label>
                <div className="upload-box">
                  {uploads.backdrop.status === "uploading" && (
                    renderUploadProgress(uploads.backdrop, "backdrop")
                  )}
                  {uploads.backdrop.status === "success" && (
                    <div className="upload-status success">
                      <CheckCircle size={20} />
                      <span>Uploaded successfully</span>
                    </div>
                  )}
                  {uploads.backdrop.status === "error" && (
                    <div className="upload-status error">
                      <AlertCircle size={20} />
                      <span>{uploads.backdrop.message}</span>
                    </div>
                  )}
                  {uploads.backdrop.status !== "uploading" && (
                    <>
                      <input
                        type="file"
                        accept="image/jpeg,image/png,image/webp"
                        onChange={(e) => {
                          const file = e.target.files?.[0];
                          if (file) handleUpload("backdrop", file);
                        }}
                        className="hidden"
                        id="series-backdrop-upload"
                      />
                      <label htmlFor="series-backdrop-upload" className="upload-label">
                        <Upload size={20} />
                        <span>Choose backdrop image (jpg, png, webp)</span>
                      </label>
                    </>
                  )}
                </div>
                {form.backdrop_url && (
                  <img
                    src={form.backdrop_url}
                    alt="Backdrop preview"
                    className="mt-2 h-24 rounded object-cover border border-brand-border w-full"
                    onError={(e) => { (e.target as HTMLImageElement).style.display = "none"; }}
                  />
                )}
              </div>
            </div>

            {/* Year & Country */}
            <div className="grid grid-cols-2 gap-4">
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Year</label>
                <input
                  type="number"
                  value={form.year}
                  onChange={(e) => setForm({ ...form, year: parseInt(e.target.value) || 2024 })}
                  className="w-full px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
                />
              </div>
              <div>
                <label className="block text-sm font-medium text-gray-300 mb-1">Country</label>
                <input
                  type="text"
                  value={form.country}
                  onChange={(e) => setForm({ ...form, country: e.target.value })}
                  className="w-full px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white focus:outline-none focus:border-brand-red"
                />
              </div>
            </div>

            {/* Genres */}
            <div>
              <label className="block text-sm font-medium text-gray-300 mb-2">Janrlar</label>

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
                  className="flex-1 px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
                />
                <button
                  type="button"
                  onClick={addGenre}
                  className="px-3 py-2 bg-brand-border hover:bg-gray-600 text-white rounded-lg transition-colors"
                >
                  <Plus size={16} />
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
                        <X size={11} />
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
                onChange={(e) => setForm({ ...form, is_premium: e.target.checked })}
                className="w-4 h-4 rounded bg-brand-card border-brand-border text-brand-red"
              />
              <label htmlFor="is_premium" className="text-gray-300">Premium content</label>
            </div>

            {/* Save Button */}
            <button
              type="submit"
              disabled={saving}
              className="w-full flex items-center justify-center gap-2 bg-brand-red hover:bg-orange-700 text-white px-6 py-3 rounded-lg font-medium transition-colors disabled:opacity-50"
            >
              {saving ? <Loader2 className="w-5 h-5 animate-spin" /> : <Save className="w-5 h-5" />}
              Saqlash
            </button>
          </form>
        </div>

        {/* Right: Seasons & Episodes */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <h2 className="text-lg font-semibold text-white">Seasonlar va Epizodlar</h2>
            <button
              onClick={() => setShowAddSeason(!showAddSeason)}
              className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400"
            >
              <PlusCircle className="w-4 h-4" />
              Season qo'shish
            </button>
          </div>

          {/* Add Season Form */}
          {showAddSeason && (
            <form onSubmit={handleAddSeason} className="bg-brand-card border border-brand-border rounded-lg p-4 mb-4 space-y-3">
              <h3 className="text-sm font-medium text-white">Yangi season</h3>
              <div className="grid grid-cols-2 gap-3">
                <input
                  type="number"
                  placeholder="Season number"
                  value={newSeason.season_number}
                  onChange={(e) => setNewSeason({ ...newSeason, season_number: parseInt(e.target.value) || 1 })}
                  className="px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm"
                  required
                />
                <input
                  type="text"
                  placeholder="Title"
                  value={newSeason.title}
                  onChange={(e) => setNewSeason({ ...newSeason, title: e.target.value })}
                  className="px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm"
                  required
                />
              </div>
              <input
                type="url"
                placeholder="Poster URL"
                value={newSeason.poster_url}
                onChange={(e) => setNewSeason({ ...newSeason, poster_url: e.target.value })}
                className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm"
              />
              <div className="flex gap-2">
                <button
                  type="button"
                  onClick={() => setShowAddSeason(false)}
                  className="flex-1 px-3 py-2 bg-brand-dark border border-brand-border text-gray-400 rounded-lg text-sm"
                >
                  Bekor
                </button>
                <button
                  type="submit"
                  disabled={addingSeason}
                  className="flex-1 px-3 py-2 bg-brand-red text-white rounded-lg text-sm disabled:opacity-50"
                >
                  {addingSeason ? "..." : "Qo'shish"}
                </button>
              </div>
            </form>
          )}

          {/* Seasons List */}
          <div className="space-y-3">
            {seasons.length === 0 ? (
              <p className="text-gray-500 text-center py-8">Hozircha seasonlar yo'q</p>
            ) : (
              seasons.map((s) => (
                <div key={s.season.id} className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
                  {/* Season Header */}
                  <div
                    className="flex items-center justify-between p-4 cursor-pointer hover:bg-brand-dark/50"
                    onClick={() => toggleSeason(s.season.id)}
                  >
                    <div className="flex items-center gap-3">
                      {s.season.poster_url ? (
                        <img src={s.season.poster_url} alt="" className="w-12 h-16 object-cover rounded" />
                      ) : (
                        <div className="w-12 h-16 bg-gray-700 rounded flex items-center justify-center">
                          <Tv className="w-6 h-6 text-gray-500" />
                        </div>
                      )}
                      <div>
                        <h3 className="font-medium text-white">
                          {s.season.title || `Season ${s.season.season_number}`}
                        </h3>
                        <p className="text-sm text-gray-400">{s.episodes.length} epizod</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          setActiveSeasonId(s.season.id);
                        }}
                        className="p-2 text-brand-red hover:text-orange-400"
                      >
                        <PlusCircle className="w-5 h-5" />
                      </button>
                      {expandedSeasons.has(s.season.id) ? (
                        <ChevronUp className="w-5 h-5 text-gray-400" />
                      ) : (
                        <ChevronDown className="w-5 h-5 text-gray-400" />
                      )}
                    </div>
                  </div>

                  {/* Episodes List */}
                  {expandedSeasons.has(s.season.id) && (
                    <div className="border-t border-brand-border">
                      {/* Add Episode Form */}
                      {activeSeasonId === s.season.id && (
                        <form onSubmit={handleAddEpisode} className="p-4 bg-brand-dark/50 border-b border-brand-border space-y-3">
                          <h4 className="text-sm font-medium text-white">Yangi epizod</h4>
                          <div className="grid grid-cols-2 gap-3">
                            <input
                              type="number"
                              placeholder="Epizod #"
                              value={newEpisode.episode_number}
                              onChange={(e) => setNewEpisode({ ...newEpisode, episode_number: parseInt(e.target.value) || 1 })}
                              className="px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm"
                              required
                            />
                            <input
                              type="text"
                              placeholder="Title"
                              value={newEpisode.title}
                              onChange={(e) => setNewEpisode({ ...newEpisode, title: e.target.value })}
                              className="px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm"
                              required
                            />
                          </div>
                          <input
                            type="url"
                            placeholder="Video URL"
                            value={newEpisode.video_url}
                            onChange={(e) => setNewEpisode({ ...newEpisode, video_url: e.target.value })}
                            className="w-full px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm"
                          />
                          <input
                            type="number"
                            placeholder="Duration (min)"
                            value={newEpisode.duration}
                            onChange={(e) => setNewEpisode({ ...newEpisode, duration: parseInt(e.target.value) || 0 })}
                            className="w-full px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm"
                          />
                          <div className="flex gap-2">
                            <button
                              type="button"
                              onClick={() => setActiveSeasonId(null)}
                              className="flex-1 px-3 py-2 bg-brand-card border border-brand-border text-gray-400 rounded-lg text-sm"
                            >
                              Bekor
                            </button>
                            <button
                              type="submit"
                              disabled={addingEpisode}
                              className="flex-1 px-3 py-2 bg-brand-red text-white rounded-lg text-sm disabled:opacity-50"
                            >
                              {addingEpisode ? "..." : "Qo'shish"}
                            </button>
                          </div>
                        </form>
                      )}

                      {/* Episodes */}
                      <DndContext
                        sensors={sensors}
                        collisionDetection={closestCenter}
                        onDragStart={handleDragStart}
                        onDragEnd={handleDragEnd}
                      >
                        <SortableContext
                          items={s.episodes.map((ep) => ep.id)}
                          strategy={verticalListSortingStrategy}
                        >
                          <div className="max-h-64 overflow-y-auto">
                            {s.episodes.length === 0 ? (
                              <p className="text-gray-500 text-center py-4 text-sm">Epizodlar yo'q</p>
                            ) : (
                              s.episodes.map((ep) => (
                                <SortableEpisodeItem
                                  key={ep.id}
                                  episode={ep}
                                  onDelete={(episodeId, episodeTitle) => setDeleteModal({ show: true, episodeId, episodeTitle })}
                                />
                              ))
                            )}
                          </div>
                        </SortableContext>
                      </DndContext>
                    </div>
                  )}
                </div>
              ))
            )}
          </div>
        </div>
      </div>

      {/* Delete Confirmation Modal */}
      {deleteModal.show && (
        <div className="fixed inset-0 bg-black/70 flex items-center justify-center z-50 p-4">
          <div className="bg-brand-card border border-brand-border rounded-lg max-w-md w-full p-6">
            <h3 className="text-lg font-semibold text-white mb-4">Epizodni o'chirish</h3>
            <p className="text-gray-300 mb-6">
              "{deleteModal.episodeTitle}" epizodni o'chirmoqchimisiz? Bu amalni ortga qaytarib bo'lmaydi.
            </p>
            <div className="flex gap-3">
              <button
                onClick={() => setDeleteModal({ show: false, episodeId: null, episodeTitle: "" })}
                className="flex-1 px-4 py-2 bg-brand-dark border border-brand-border text-gray-300 rounded-lg hover:bg-brand-card"
              >
                Bekor qilish
              </button>
              <button
                onClick={handleDeleteEpisode}
                disabled={deletingEpisode}
                className="flex-1 px-4 py-2 bg-red-600 text-white rounded-lg hover:bg-red-700 disabled:opacity-50"
              >
                {deletingEpisode ? "..." : "O'chirish"}
              </button>
            </div>
          </div>
        </div>
      )}

      <style jsx global>{`
        .upload-box {
          border: 2px dashed #1e1e2e;
          border-radius: 0.5rem;
          padding: 1rem;
          text-align: center;
          background: #0a0a0f;
          min-height: 80px;
          display: flex;
          align-items: center;
          justify-content: center;
        }
        .upload-box:hover {
          border-color: #e63946;
        }
        .upload-label {
          cursor: pointer;
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 0.5rem;
          color: #9ca3af;
          font-size: 0.875rem;
        }
        .upload-label:hover {
          color: white;
        }
        .upload-status {
          display: flex;
          align-items: center;
          gap: 0.5rem;
          color: #9ca3af;
          font-size: 0.875rem;
        }
        .upload-status.success {
          color: #10b981;
        }
        .upload-status.error {
          color: #ef4444;
        }
        .upload-progress {
          display: flex;
          align-items: center;
          gap: 0.75rem;
          color: #9ca3af;
          font-size: 0.875rem;
          width: 100%;
        }
        .progress-content {
          flex: 1;
          text-align: left;
        }
        .progress-bar {
          width: 100%;
          height: 0.5rem;
          background: #111827;
          border-radius: 9999px;
          overflow: hidden;
          margin-top: 0.5rem;
        }
        .progress-fill {
          height: 100%;
          background: #e63946;
          transition: width 0.2s ease;
        }
        .progress-meta {
          display: flex;
          gap: 0.75rem;
          flex-wrap: wrap;
          font-size: 0.75rem;
          color: #6b7280;
          margin-top: 0.5rem;
        }
      `}</style>
    </div>
  );
}
