"use client";

import { useEffect, useState } from "react";
import { useParams } from "next/navigation";
import {
  Loader2,
  Save,
  PlusCircle,
  Trash2,
  Tv,
  Play,
  ChevronDown,
  ChevronUp,
  Plus,
  X,
  Upload,
  GripVertical,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetSeries,
  adminUpdateSeries,
  adminCreateSeason,
  adminUpdateSeason,
  adminDeleteSeason,
  adminCreateEpisode,
  adminDeleteEpisode,
  adminReorderEpisodes,
  adminMoveEpisodeToSeason,
  CreateSeriesData,
  UploadProgressInfo,
  directB2Upload,
  createEpisodeDirectUpload,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import AdminPageHeader from "@/components/admin/form/PageHeader";
import SeriesInfoForm from "@/components/admin/SeriesInfoForm";
import SeriesCastPanel from "@/components/admin/SeriesCastPanel";
import type { CastMember } from "@/lib/api";
import { storageQualityList } from "@/components/admin/form/constants";
import { useToast } from "@/components/admin/Toast";
import {
  DndContext,
  closestCenter,
  PointerSensor,
  useSensor,
  useSensors,
  DragEndEvent,
  useDroppable,
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
  generated_qualities?: string[];
  available_qualities?: string[];
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
        <MediaImage src={normalizeMediaUrl(episode.thumbnail_url)} alt="" className="w-16 h-10 object-cover rounded" />
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

function SeasonDropZone({
  seasonId,
  children,
}: {
  seasonId: string;
  children: React.ReactNode;
}) {
  const { setNodeRef, isOver } = useDroppable({
    id: `season-drop-${seasonId}`,
  });

  return (
    <div
      ref={setNodeRef}
      className={`${isOver ? "bg-brand-red/5" : ""}`}
    >
      {children}
    </div>
  );
}


interface SeasonWithEpisodes {
  season: Season;
  episodes: Episode[];
}

// Stored lowercase English (matches DB). Displayed with `capitalize` CSS.

export default function EditSeriesPage() {
  const { token } = useAuth();
  const toast = useToast();
  const [seriesCredits, setSeriesCredits] = useState<{ cast: string[]; director: string; directorPhoto: string; details: CastMember[] }>({ cast: [], director: "", directorPhoto: "", details: [] });
  const params = useParams();
  const id = params.id as string;

  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [seriesCode, setSeriesCode] = useState("");

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
    quality: "1080p",
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
  // Direct-upload state for the new episode's video (resumable B2 upload,
  // mirrors Add Movie). When tempUrl is set, submitting goes through the
  // transcode pipeline instead of saving a raw video_url.
  const [episodeUpload, setEpisodeUpload] = useState<{
    status: "idle" | "uploading" | "done" | "error";
    progress: number;
    uploadedMB?: number;
    tempUrl?: string;
    tempKey?: string;
    fileName?: string;
    message?: string;
  }>({ status: "idle", progress: 0 });

  const resetEpisodeUpload = () => setEpisodeUpload({ status: "idle", progress: 0 });

  const handleEpisodeVideoUpload = async (file: File) => {
    if (!token) return;
    setEpisodeUpload({ status: "uploading", progress: 0, fileName: file.name });
    try {
      const result = await directB2Upload(token, file, "video", (p: UploadProgressInfo) => {
        setEpisodeUpload((prev) => ({
          ...prev,
          status: "uploading",
          progress: p.progress ?? prev.progress,
          uploadedMB: p.uploadedMB ?? prev.uploadedMB,
        }));
      });
      setEpisodeUpload({ status: "done", progress: 100, tempUrl: result.url, tempKey: result.file_key, fileName: file.name });
    } catch (err) {
      setEpisodeUpload({ status: "error", progress: 0, message: err instanceof Error ? err.message : "Upload failed" });
    }
  };

  const [addingSeason, setAddingSeason] = useState(false);
  const [addingEpisode, setAddingEpisode] = useState(false);
  const [savingSeasonId, setSavingSeasonId] = useState<string | null>(null);
  const [deletingSeasonId, setDeletingSeasonId] = useState<string | null>(null);

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
          setSeriesCode(series.code || "");
          setSeriesCredits({ cast: series.cast || [], director: series.director || "", directorPhoto: series.director_profile_url || "", details: series.cast_details || [] });
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
            quality: series.quality || "",
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

  const saveSeriesInfo = async (data: CreateSeriesData) => {
    if (!token || !id) throw new Error("Tizimga qayta kiring");
    await adminUpdateSeries(token, id, data);
    setForm(data);
    toast.success("Serial ma'lumotlari saqlandi");
  };

  // Next free numbers so adding seasons/episodes needs no typing.
  const nextSeasonNumber = () => Math.max(0, ...seasons.map((x) => x.season.season_number || 0)) + 1;
  const nextEpisodeNumber = (seasonId: string) => {
    const eps = seasons.find((x) => x.season.id === seasonId)?.episodes ?? [];
    return Math.max(0, ...eps.map((e) => e.episode_number || 0)) + 1;
  };
  const openAddSeason = () => {
    const n = nextSeasonNumber();
    setNewSeason({ season_number: n, title: `${n}-fasl`, poster_url: "", description: "" });
    setShowAddSeason(true);
  };
  const openAddEpisode = (seasonId: string) => {
    const n = nextEpisodeNumber(seasonId);
    setNewEpisode({ episode_number: n, title: `${n}-qism`, description: "", thumbnail_url: "", video_url: "", duration: 0 });
    resetEpisodeUpload();
    setActiveSeasonId(seasonId);
    setExpandedSeasons((prev) => new Set(prev).add(seasonId));
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

    if (episodeUpload.status === "uploading") {
      alert("Video hali yuklanmoqda, kuting...");
      return;
    }
    setAddingEpisode(true);
    try {
      let episode: Episode;
      if (episodeUpload.tempUrl) {
        // Direct-upload path: the worker transcodes the uploaded file to HLS
        // and links it to the episode (video_url filled asynchronously).
        const res = await createEpisodeDirectUpload(token, {
          season_id: activeSeasonId,
          episode_number: newEpisode.episode_number,
          title: newEpisode.title,
          temp_file_url: episodeUpload.tempUrl,
          temp_file_key: episodeUpload.tempKey,
          duration: newEpisode.duration,
          quality: form.quality,
        });
        episode = res.episode as unknown as Episode;
      } else {
        // Legacy path: a ready video_url was pasted.
        episode = await adminCreateEpisode(token, activeSeasonId, newEpisode);
      }
      setSeasons((prev) =>
        prev.map((s) =>
          s.season.id === activeSeasonId
            ? { ...s, episodes: [...s.episodes, episode] }
            : s
        )
      );
      // Keep the form open and bump the episode number so adding many
      // episodes in a row (e.g. 100) is fast — no need to reopen each time.
      const nextNumber = newEpisode.episode_number + 1;
      setNewEpisode({
        episode_number: nextNumber,
        title: `${nextNumber}-qism`,
        description: "",
        thumbnail_url: "",
        video_url: "",
        duration: 0,
      });
      resetEpisodeUpload();
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

  const renumberEpisodes = (episodes: Episode[], seasonId?: string): Episode[] =>
    episodes.map((ep, index) => ({
      ...ep,
      season_id: seasonId ?? ep.season_id,
      episode_number: index + 1,
    }));

  const handleSaveSeason = async (seasonId: string) => {
    if (!token) return;

    const seasonState = seasons.find((s) => s.season.id === seasonId);
    if (!seasonState) return;

    setSavingSeasonId(seasonId);
    try {
      const updatedSeason = await adminUpdateSeason(token, seasonId, {
        title: seasonState.season.title,
        poster_url: seasonState.season.poster_url,
        description: seasonState.season.description,
      });
      setSeasons((prev) =>
        prev.map((s) =>
          s.season.id === seasonId ? { ...s, season: updatedSeason } : s
        )
      );
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to update season");
    } finally {
      setSavingSeasonId(null);
    }
  };

  const handleDeleteSeason = async (seasonId: string) => {
    if (!token) return;

    const seasonState = seasons.find((s) => s.season.id === seasonId);
    if (!seasonState) return;

    if (seasonState.episodes.length > 0) {
      alert("Bu season ichida epizodlar bor. Avval epizodlarni ko'chiring yoki o'chiring.");
      return;
    }

    if (!confirm(`"${seasonState.season.title || `Season ${seasonState.season.season_number}`}" seasonini o'chirmoqchimisiz?`)) {
      return;
    }

    setDeletingSeasonId(seasonId);
    try {
      await adminDeleteSeason(token, seasonId);
      setSeasons((prev) => prev.filter((s) => s.season.id !== seasonId));
      setExpandedSeasons((prev) => {
        const next = new Set(prev);
        next.delete(seasonId);
        return next;
      });
      if (activeSeasonId === seasonId) {
        setActiveSeasonId(null);
      }
    } catch (err) {
      alert(err instanceof Error ? err.message : "Failed to delete season");
    } finally {
      setDeletingSeasonId(null);
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
      if (seasons[i].episodes.some((ep) => ep.id === overId) || overId === `season-drop-${seasons[i].season.id}`) {
        targetSeasonIndex = i;
      }
    }

    if (!activeEpisode || sourceSeasonIndex === -1 || targetSeasonIndex === -1) return;

    const sourceSeason = seasons[sourceSeasonIndex];
    const targetSeason = seasons[targetSeasonIndex];

    // If moving within the same season
    if (sourceSeasonIndex === targetSeasonIndex) {
      const oldIndex = sourceSeason.episodes.findIndex((ep) => ep.id === activeId);
      const newIndex =
        overId === `season-drop-${sourceSeason.season.id}`
          ? sourceSeason.episodes.length - 1
          : sourceSeason.episodes.findIndex((ep) => ep.id === overId);

      if (oldIndex === -1 || newIndex === -1 || oldIndex === newIndex) return;

      const newEpisodes = renumberEpisodes(arrayMove(sourceSeason.episodes, oldIndex, newIndex));

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
    const sourceEpisodesWithoutActive = sourceSeason.episodes.filter((ep) => ep.id !== activeId);
    const targetInsertIndex =
      overId === `season-drop-${targetSeason.season.id}`
        ? targetSeason.episodes.length
        : Math.max(0, targetSeason.episodes.findIndex((ep) => ep.id === overId));
    const targetEpisodesWithMoved = [...targetSeason.episodes];
    targetEpisodesWithMoved.splice(targetInsertIndex, 0, {
      ...activeEpisode,
      season_id: targetSeason.season.id,
    });

    const updatedSourceEpisodes = renumberEpisodes(sourceEpisodesWithoutActive, sourceSeason.season.id);
    const updatedTargetEpisodes = renumberEpisodes(targetEpisodesWithMoved, targetSeason.season.id);
    const movedEpisode = updatedTargetEpisodes.find((ep) => ep.id === activeId);
    if (!movedEpisode) return;

    // Optimistically update UI
    setSeasons((prev) => {
      const newSeasons = [...prev];
      newSeasons[sourceSeasonIndex] = {
        ...newSeasons[sourceSeasonIndex],
        episodes: updatedSourceEpisodes,
      };
      newSeasons[targetSeasonIndex] = {
        ...newSeasons[targetSeasonIndex],
        episodes: updatedTargetEpisodes,
      };
      return newSeasons;
    });

    // Persist move + both season orderings
    try {
      await adminMoveEpisodeToSeason(token, activeId, targetSeason.season.id, movedEpisode.episode_number);
      await adminReorderEpisodes(token, targetSeason.season.id, updatedTargetEpisodes.map((ep) => ep.id));
      await adminReorderEpisodes(token, sourceSeason.season.id, updatedSourceEpisodes.map((ep) => ep.id));
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

  if (loading) {
    return (
      <div className="flex items-center justify-center min-h-[400px]">
        <Loader2 className="w-8 h-8 text-brand-red animate-spin" />
      </div>
    );
  }

  const episodesCount = seasons.reduce((n, x) => n + x.episodes.length, 0);

  return (
    <div className="p-4 sm:p-8">
      <AdminPageHeader
        backHref="/admin/series"
        backLabel="Seriallar"
        title={form.title || "Serial"}
        subtitle={<span className="font-mono text-xs">{seriesCode ? `#${seriesCode} · ` : ""}/series/{form.slug}</span>}
        actions={
          <a href="#seasons" className="rounded-xl border border-white/10 px-3 py-2 text-sm text-gray-300 hover:bg-white/5">
            Fasllar va qismlar ↓
          </a>
        }
      />

      {error && <div className="mb-6 rounded-xl border border-red-500/30 bg-red-500/10 px-4 py-3 text-sm text-red-300">{error}</div>}

      <SeriesInfoForm
        mode="edit"
        token={token}
        initial={form}
        onSubmit={saveSeriesInfo}
        previewHref={form.slug ? `/series/${form.slug}` : undefined}
        code={seriesCode}
        seasonsCount={seasons.length}
        episodesCount={episodesCount}
        storageQualities={storageQualityList(
          ...seasons.flatMap((s) => s.episodes.map((e) => (e.generated_qualities?.length ? e.generated_qualities : e.available_qualities)))
        )}
      />

      <SeriesCastPanel
        seriesId={id}
        token={token}
        initialCast={seriesCredits.cast}
        initialDirector={seriesCredits.director}
        initialDetails={seriesCredits.details}
        initialDirectorPhoto={seriesCredits.directorPhoto}
        title={form.title}
        year={form.year}
        onMessage={(kind, text) => (kind === "success" ? toast.success(text) : toast.error(text))}
      />

      <div id="seasons" className="mt-8 scroll-mt-24 rounded-2xl border border-white/10 bg-[#12121a] p-5 sm:p-6">
        {/* Seasons & Episodes */}
        <div>
          <div className="flex items-center justify-between mb-4">
            <div>
              <h2 className="font-semibold text-white">Fasllar va qismlar</h2>
              <p className="text-xs text-gray-500">{seasons.length} fasl · {episodesCount} qism · qismlarni sudrab tartiblash mumkin</p>
            </div>
            <button
              onClick={() => (showAddSeason ? setShowAddSeason(false) : openAddSeason())}
              className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400"
            >
              <PlusCircle className="w-4 h-4" />
              Fasl qo&apos;shish
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
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
          >
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
                        <MediaImage src={normalizeMediaUrl(s.season.poster_url)} alt="" className="w-12 h-16 object-cover rounded" />
                      ) : (
                        <div className="w-12 h-16 bg-gray-700 rounded flex items-center justify-center">
                          <Tv className="w-6 h-6 text-gray-500" />
                        </div>
                      )}
                      <div>
                        <div className="flex items-center gap-2">
                          <input
                            type="text"
                            value={s.season.title}
                            onClick={(e) => e.stopPropagation()}
                            onChange={(e) =>
                              setSeasons((prev) =>
                                prev.map((seasonState) =>
                                  seasonState.season.id === s.season.id
                                    ? {
                                        ...seasonState,
                                        season: { ...seasonState.season, title: e.target.value },
                                      }
                                    : seasonState
                                )
                              )
                            }
                            className="bg-transparent border border-brand-border rounded px-2 py-1 text-sm font-medium text-white focus:outline-none focus:border-brand-red"
                            placeholder={`Season ${s.season.season_number}`}
                          />
                          <button
                            onClick={(e) => {
                              e.stopPropagation();
                              void handleSaveSeason(s.season.id);
                            }}
                            disabled={savingSeasonId === s.season.id}
                            className="p-1.5 text-brand-red hover:text-orange-400 disabled:opacity-50"
                            title="Seasonni saqlash"
                          >
                            {savingSeasonId === s.season.id ? (
                              <Loader2 className="w-4 h-4 animate-spin" />
                            ) : (
                              <Save className="w-4 h-4" />
                            )}
                          </button>
                        </div>
                        <p className="text-sm text-gray-400">{s.episodes.length} epizod</p>
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          void handleDeleteSeason(s.season.id);
                        }}
                        disabled={deletingSeasonId === s.season.id}
                        className="p-2 text-gray-400 hover:text-red-400 disabled:opacity-50"
                        title={s.episodes.length > 0 ? "Avval season ichidagi epizodlarni ko'chiring yoki o'chiring" : "Seasonni o'chirish"}
                      >
                        {deletingSeasonId === s.season.id ? (
                          <Loader2 className="w-5 h-5 animate-spin" />
                        ) : (
                          <Trash2 className="w-5 h-5" />
                        )}
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation();
                          openAddEpisode(s.season.id);
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
                    <SeasonDropZone seasonId={s.season.id}>
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
                            disabled={episodeUpload.status === "uploading" || episodeUpload.status === "done"}
                            className="w-full px-3 py-2 bg-brand-card border border-brand-border rounded-lg text-white text-sm disabled:opacity-50"
                          />

                          {/* — yoki — direct video upload (Add Movie kabi) */}
                          <div className="flex items-center gap-2 text-[11px] text-gray-500">
                            <span className="flex-1 h-px bg-brand-border" /> yoki video faylni yuklang <span className="flex-1 h-px bg-brand-border" />
                          </div>
                          {episodeUpload.status === "idle" && (
                            <label className="flex items-center justify-center gap-2 px-3 py-2 bg-brand-card border border-dashed border-brand-border rounded-lg text-sm text-gray-300 cursor-pointer hover:border-brand-red">
                              <Upload className="w-4 h-4" />
                              <span>Video tanlash (mp4/mov/webm)</span>
                              <input
                                type="file"
                                accept="video/mp4,video/webm,video/ogg,video/quicktime"
                                className="hidden"
                                onChange={(e) => {
                                  const f = e.target.files?.[0];
                                  if (f) void handleEpisodeVideoUpload(f);
                                  e.target.value = "";
                                }}
                              />
                            </label>
                          )}
                          {episodeUpload.status === "uploading" && (
                            <div className="space-y-1">
                              <div className="flex justify-between text-[11px] text-gray-400">
                                <span className="truncate">{episodeUpload.fileName}</span>
                                <span>{episodeUpload.progress}%{episodeUpload.uploadedMB ? ` · ${episodeUpload.uploadedMB.toFixed(0)} MB` : ""}</span>
                              </div>
                              <div className="h-1.5 bg-brand-border rounded-full overflow-hidden">
                                <div className="h-full bg-brand-red transition-all" style={{ width: `${episodeUpload.progress}%` }} />
                              </div>
                            </div>
                          )}
                          {episodeUpload.status === "done" && (
                            <div className="flex items-center justify-between gap-2 px-3 py-2 bg-green-500/10 border border-green-500/30 rounded-lg text-xs text-green-300">
                              <span className="truncate">✓ Yuklandi: {episodeUpload.fileName} (transkod qilinadi)</span>
                              <button type="button" onClick={resetEpisodeUpload} className="text-gray-400 hover:text-white shrink-0">
                                <X className="w-4 h-4" />
                              </button>
                            </div>
                          )}
                          {episodeUpload.status === "error" && (
                            <div className="px-3 py-2 bg-red-500/10 border border-red-500/30 rounded-lg text-xs text-red-300">
                              {episodeUpload.message || "Upload failed"} ·{" "}
                              <button type="button" onClick={resetEpisodeUpload} className="underline">qayta urinish</button>
                            </div>
                          )}

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
                              onClick={() => { setActiveSeasonId(null); resetEpisodeUpload(); }}
                              className="flex-1 px-3 py-2 bg-brand-card border border-brand-border text-gray-400 rounded-lg text-sm"
                            >
                              Bekor
                            </button>
                            <button
                              type="submit"
                              disabled={addingEpisode || episodeUpload.status === "uploading"}
                              className="flex-1 px-3 py-2 bg-brand-red text-white rounded-lg text-sm disabled:opacity-50"
                            >
                              {addingEpisode ? "..." : episodeUpload.status === "uploading" ? "Yuklanmoqda..." : "Qo'shish"}
                            </button>
                          </div>
                        </form>
                      )}

                      {/* Episodes */}
                      <SortableContext
                        items={s.episodes.map((ep) => ep.id)}
                        strategy={verticalListSortingStrategy}
                      >
                        <div className="max-h-64 overflow-y-auto min-h-16">
                          {s.episodes.length === 0 ? (
                            <p className="text-gray-500 text-center py-4 text-sm">Epizodlar yo'q. Epizodni bu yerga tashlash mumkin.</p>
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
                    </div>
                    </SeasonDropZone>
                  )}
                </div>
              ))
            )}
          </div>
          </DndContext>
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
