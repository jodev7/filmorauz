"use client";

import React, { useEffect, useState, useCallback } from "react";
import {
  Film,
  ExternalLink,
  Download,
  Clock,
  Loader2,
  Upload,
  CheckCircle,
  XCircle,
  X,
  Calendar,
  CalendarClock,
  Pencil,
  Trash2,
  Instagram,
  Youtube,
  Music2,
  ChevronDown,
  ChevronRight,
  ChevronsDownUp,
  ChevronsUpDown,
  type LucideIcon,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";

// ─── Constants ───────────────────────────────────────────────────────────────

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";
const TASHKENT_TZ = "Asia/Tashkent";
const CDN_BASE_URL =
  (process.env.NEXT_PUBLIC_CDN_BASE_URL || "").trim().replace(/\/+$/, "") ||
  "https://cdn.filmorauz.net/file/filmorauznet";

// ─── Time helpers ─────────────────────────────────────────────────────────────

function getNowTashkent(): string {
  return new Intl.DateTimeFormat("sv-SE", {
    timeZone: TASHKENT_TZ,
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  })
    .format(new Date())
    .replace(" ", "T");
}

function tashkentLocalToISO(localStr: string): string {
  return new Date(localStr + ":00+05:00").toISOString();
}

function formatTashkent(isoStr?: string | null): string {
  if (!isoStr) return "—";
  return new Intl.DateTimeFormat("uz-UZ", {
    timeZone: TASHKENT_TZ,
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
  }).format(new Date(isoStr));
}

// ─── Types ───────────────────────────────────────────────────────────────────

type Platform = "instagram" | "youtube" | "tiktok";

interface Clip {
  id: string;
  content_kind?: string;
  movie_id: string;
  movie_title: string;
  movie_slug: string;
  movie_code: string;
  series_id?: string;
  series_title?: string;
  series_slug?: string;
  season_id?: string;
  season_number?: number;
  episode_id?: string;
  episode_number?: number;
  title?: string;
  clip_index?: number;
  source_type?: string;
  filename: string;
  path: string;
  url: string;
  video_url?: string;
  file_url?: string;
  public_url?: string;
  cdn_url?: string;
  duration: number;
  sequence: number;
  storage_type: string;
  created_at: string;
  uploaded_to_instagram: boolean;
  instagram_upload_count: number;
  last_instagram_upload_at?: string;
  last_instagram_upload_status: string;
}

interface AllAccounts {
  instagram: string[];
  youtube: string[];
  tiktok: string[];
}

interface PublishJob {
  id: string;
  clip_id: string;
  clip_url: string;
  movie_title: string;
  movie_slug: string;
  movie_code: string;
  platform: Platform;
  account_name: string;
  scheduled_for: string;
  status: "pending" | "processing" | "success" | "failed";
  created_at: string;
  executed_at?: string;
  error?: string;
}

interface SelectedJob {
  platform: Platform;
  account_name: string;
}

type UploadMode = "now" | "scheduled";

interface JobResult {
  platform: Platform;
  account_name: string;
  status: "success" | "failed";
  error?: string;
}

interface PublishModal {
  clip: Clip;
  selectedJobs: SelectedJob[];
  mode: UploadMode;
  scheduledFor: string;
  results: JobResult[] | null;
  scheduledCreated: boolean;
}

interface EpisodeClipGroup {
  key: string;
  title: string;
  episodeNumber: number;
  clips: Clip[];
}

interface SeasonClipGroup {
  key: string;
  title: string;
  seasonNumber: number;
  episodes: EpisodeClipGroup[];
}

interface ContentClipGroup {
  key: string;
  kind: "movie" | "series";
  title: string;
  slug: string;
  code?: string;
  clips: Clip[];
  seasons?: SeasonClipGroup[];
}

interface PaginationControlsProps {
  page: number;
  totalPages: number;
  onPrev: () => void;
  onNext: () => void;
}

// ─── Platform config ──────────────────────────────────────────────────────────

const PLATFORM_META: Record<
  Platform,
  { label: string; color: string; bgColor: string; borderColor: string; Icon: LucideIcon }
> = {
  instagram: {
    label: "Instagram",
    color: "text-pink-400",
    bgColor: "bg-pink-500/10",
    borderColor: "border-pink-500/30",
    Icon: Instagram,
  },
  youtube: {
    label: "YouTube",
    color: "text-red-400",
    bgColor: "bg-red-500/10",
    borderColor: "border-red-500/30",
    Icon: Youtube,
  },
  tiktok: {
    label: "TikTok",
    color: "text-sky-400",
    bgColor: "bg-sky-500/10",
    borderColor: "border-sky-500/30",
    Icon: Music2,
  },
};

const ALL_PLATFORMS: Platform[] = ["instagram", "youtube", "tiktok"];

// ─── Sub-components ──────────────────────────────────────────────────────────

function PlatformUploadStatuses({
  clip,
  clipJobs,
}: {
  clip: Clip;
  clipJobs: PublishJob[];
}) {
  const rows: React.ReactNode[] = [];

  // Instagram: driven by clip fields
  if (clip.instagram_upload_count > 0 || clip.last_instagram_upload_status) {
    const meta = PLATFORM_META.instagram;
    const { Icon } = meta;
    const ok = clip.last_instagram_upload_status === "success";
    const failed = clip.last_instagram_upload_status === "failed";
    rows.push(
      <div key="instagram" className="flex items-center gap-1.5">
        <Icon size={11} className={meta.color} />
        {ok ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-green-400">
            <CheckCircle size={10} />
            {clip.instagram_upload_count}× yuklandi
            {clip.last_instagram_upload_at && (
              <span className="text-gray-600 text-[10px]">
                · {formatTashkent(clip.last_instagram_upload_at)}
              </span>
            )}
          </span>
        ) : failed ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-red-400">
            <XCircle size={10} />
            Xato
          </span>
        ) : (
          <span className="text-[11px] text-gray-500">—</span>
        )}
      </div>
    );
  }

  // YouTube + TikTok: driven by publishJobs
  for (const platform of ["youtube", "tiktok"] as const) {
    const jobs = clipJobs.filter((j) => j.platform === platform);
    if (jobs.length === 0) continue;
    const meta = PLATFORM_META[platform];
    const { Icon } = meta;
    const successJobs = jobs.filter((j) => j.status === "success");
    const pendingJobs = jobs.filter((j) => j.status === "pending" || j.status === "processing");
    const failedJobs = jobs.filter((j) => j.status === "failed");
    const lastSuccess = successJobs.sort((a, b) =>
      (b.executed_at ?? b.created_at).localeCompare(a.executed_at ?? a.created_at)
    )[0];
    rows.push(
      <div key={platform} className="flex items-center gap-1.5">
        <Icon size={11} className={meta.color} />
        {successJobs.length > 0 ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-green-400">
            <CheckCircle size={10} />
            {successJobs.length}× yuklandi
            {lastSuccess?.executed_at && (
              <span className="text-gray-600 text-[10px]">
                · {formatTashkent(lastSuccess.executed_at)}
              </span>
            )}
          </span>
        ) : pendingJobs.length > 0 ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-blue-400">
            <CalendarClock size={10} />
            {formatTashkent(pendingJobs[0].scheduled_for)}
          </span>
        ) : failedJobs.length > 0 ? (
          <span className="inline-flex items-center gap-1 text-[11px] text-red-400">
            <XCircle size={10} />
            Xato
          </span>
        ) : null}
      </div>
    );
  }

  if (rows.length === 0) {
    return <span className="text-xs text-gray-600">—</span>;
  }
  return <div className="flex flex-col gap-1">{rows}</div>;
}

function JobStatusBadge({ status }: { status: PublishJob["status"] }) {
  if (status === "pending")
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-blue-500/15 text-blue-400 border border-blue-500/30">
        <Clock size={11} />
        Kutilmoqda
      </span>
    );
  if (status === "processing")
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-yellow-500/15 text-yellow-400 border border-yellow-500/30">
        <Loader2 size={11} className="animate-spin" />
        Bajarilmoqda
      </span>
    );
  if (status === "success")
    return (
      <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-green-500/15 text-green-400 border border-green-500/30">
        <CheckCircle size={11} />
        Muvaffaqiyatli
      </span>
    );
  return (
    <span className="inline-flex items-center gap-1 text-xs px-2 py-0.5 rounded-full bg-red-500/15 text-red-400 border border-red-500/30">
      <XCircle size={11} />
      Xato
    </span>
  );
}

function PlatformBadge({ platform }: { platform: Platform }) {
  const meta = PLATFORM_META[platform];
  const { Icon } = meta;
  return (
    <span
      className={`inline-flex items-center gap-1 text-[10px] px-1.5 py-0.5 rounded ${meta.bgColor} ${meta.color} ${meta.borderColor} border`}
    >
      <Icon size={9} />
      {meta.label}
    </span>
  );
}

function PaginationControls({ page, totalPages, onPrev, onNext }: PaginationControlsProps) {
  return (
    <div className="flex items-center justify-end gap-2">
      <button
        onClick={onPrev}
        disabled={page <= 1}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-brand-border text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Prev
      </button>
      <span className="text-xs text-gray-500 min-w-[88px] text-center">
        Page {totalPages === 0 ? 0 : page} of {totalPages}
      </span>
      <button
        onClick={onNext}
        disabled={page >= totalPages}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-brand-border text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Next
      </button>
    </div>
  );
}

function formatDuration(seconds: number) {
  const m = Math.floor(seconds / 60);
  const s = seconds % 60;
  return `${m}:${s.toString().padStart(2, "0")}`;
}

function padEpisodeNumber(value?: number) {
  return String(value ?? 0).padStart(2, "0");
}

function getClipDisplayTitle(clip: Clip): string {
  if (clip.movie_title?.trim()) return clip.movie_title;
  if (clip.title?.trim()) return clip.title;
  if (clip.series_title?.trim()) {
    return `${clip.series_title} S${padEpisodeNumber(clip.season_number)}E${padEpisodeNumber(clip.episode_number)}`;
  }
  return "Untitled clip";
}

function getClipSequence(clip: Clip): number {
  return clip.clip_index || clip.sequence || 0;
}

// Returns true if value is a non-empty, non-zero MongoDB ObjectID string.
// `primitive.ObjectID` json-marshals zero values as "000000000000000000000000",
// which would otherwise be truthy and misclassify movie clips as series clips.
function hasId(value?: string): boolean {
  if (!value) return false;
  const trimmed = value.trim();
  if (!trimmed) return false;
  if (/^0+$/.test(trimmed)) return false;
  return true;
}

function normalizeClipType(clip: Clip): "movie" | "series_episode" {
  if (clip.source_type === "series_episode") return "series_episode";
  if (clip.source_type === "movie") return "movie";
  if (clip.content_kind === "series") return "series_episode";
  if (clip.content_kind === "movie") return "movie";
  if (hasId(clip.episode_id) || hasId(clip.series_id)) return "series_episode";
  if (hasId(clip.movie_id)) return "movie";
  return "movie";
}

function getClipKind(clip: Clip): "movie" | "series" {
  return normalizeClipType(clip) === "series_episode" ? "series" : "movie";
}

function buildClipGroups(clips: Clip[]): ContentClipGroup[] {
  const movieGroups = new Map<string, ContentClipGroup>();
  const seriesGroups = new Map<string, {
    key: string;
    title: string;
    slug: string;
    clips: Clip[];
    seasons: Map<number, {
      key: string;
      title: string;
      seasonNumber: number;
      episodes: Map<string, EpisodeClipGroup>;
    }>;
  }>();

  for (const clip of clips) {
    if (getClipKind(clip) === "movie") {
      const key = hasId(clip.movie_id)
        ? `movie:${clip.movie_id}`
        : clip.movie_slug
          ? `movie:slug:${clip.movie_slug}`
          : `movie:clip:${clip.id}`;
      if (!movieGroups.has(key)) {
        movieGroups.set(key, {
          key,
          kind: "movie",
          title: clip.movie_title?.trim() || clip.title?.trim() || "Untitled movie",
          slug: clip.movie_slug || "",
          code: clip.movie_code || "",
          clips: [],
        });
      }
      movieGroups.get(key)!.clips.push(clip);
      continue;
    }

    const seriesKey = hasId(clip.series_id)
      ? `series:${clip.series_id}`
      : clip.series_slug
        ? `series:slug:${clip.series_slug}`
        : `series:clip:${clip.id}`;
    if (!seriesGroups.has(seriesKey)) {
      seriesGroups.set(seriesKey, {
        key: seriesKey,
        title: clip.series_title?.trim() || clip.title?.trim() || "Untitled series",
        slug: clip.series_slug || "",
        clips: [],
        seasons: new Map(),
      });
    }
    const seriesGroup = seriesGroups.get(seriesKey)!;
    seriesGroup.clips.push(clip);

    const seasonNumber = clip.season_number || 0;
    if (!seriesGroup.seasons.has(seasonNumber)) {
      seriesGroup.seasons.set(seasonNumber, {
        key: `${seriesKey}:season:${seasonNumber}`,
        title: `Season ${seasonNumber}`,
        seasonNumber,
        episodes: new Map(),
      });
    }
    const seasonGroup = seriesGroup.seasons.get(seasonNumber)!;
    const episodeKey = clip.episode_id || `${seasonGroup.key}:episode:${clip.episode_number || 0}`;
    if (!seasonGroup.episodes.has(episodeKey)) {
      seasonGroup.episodes.set(episodeKey, {
        key: episodeKey,
        title: clip.title || `${seriesGroup.title} S${padEpisodeNumber(seasonNumber)}E${padEpisodeNumber(clip.episode_number)}`,
        episodeNumber: clip.episode_number || 0,
        clips: [],
      });
    }
    seasonGroup.episodes.get(episodeKey)!.clips.push(clip);
  }

  const movieResults = Array.from(movieGroups.values());
  const seriesResults: ContentClipGroup[] = Array.from(seriesGroups.values()).map((group) => ({
    key: group.key,
    kind: "series",
    title: group.title,
    slug: group.slug,
    clips: group.clips,
    seasons: Array.from(group.seasons.values())
      .sort((a, b) => a.seasonNumber - b.seasonNumber)
      .map((season) => ({
        key: season.key,
        title: season.title,
        seasonNumber: season.seasonNumber,
        episodes: Array.from(season.episodes.values())
          .sort((a, b) => a.episodeNumber - b.episodeNumber)
          .map((episode) => ({
            ...episode,
            clips: [...episode.clips].sort((a, b) => getClipSequence(a) - getClipSequence(b)),
          })),
      })),
  }));

  return [...movieResults, ...seriesResults];
}

function resolveClipDownloadUrl(clip: Clip): string {
  const candidates = [
    clip.url,
    clip.video_url,
    clip.file_url,
    clip.public_url,
    clip.cdn_url,
  ];

  for (const candidate of candidates) {
    const value = candidate?.trim();
    if (!value) continue;
    if (value.startsWith("http://") || value.startsWith("https://")) {
      if (value.includes("/media/")) {
        const mediaIndex = value.indexOf("/media/");
        const mediaPath = value.slice(mediaIndex + "/media".length);
        if (mediaPath.startsWith("/videos/")) {
          return `${CDN_BASE_URL}${mediaPath}`;
        }
      }
      return value;
    }
    if (value.startsWith("/file/filmorauznet/")) {
      return `${CDN_BASE_URL}/${value.slice("/file/filmorauznet/".length).replace(/^\/+/, "")}`;
    }
    if (value.startsWith("/media/videos/")) {
      return `${CDN_BASE_URL}${value.slice("/media".length)}`;
    }
    if (value.startsWith("/videos/")) {
      return `${CDN_BASE_URL}${value}`;
    }
    if (value.startsWith("videos/")) {
      return `${CDN_BASE_URL}/${value}`;
    }
  }

  return "";
}

function resolveClipOpenUrl(clip: Clip): string {
  return resolveClipDownloadUrl(clip);
}

// ─── Main page ────────────────────────────────────────────────────────────────

export default function AdminClipsPage() {
  const { token } = useAuth();
  const clipsPageLimit = 10;
  const jobsPageLimit = 10;
  const [clips, setClips] = useState<Clip[]>([]);
  const [clipsPage, setClipsPage] = useState(1);
  const [loading, setLoading] = useState(true);
  const [allAccounts, setAllAccounts] = useState<AllAccounts>({ instagram: [], youtube: [], tiktok: [] });
  const [publishJobs, setPublishJobs] = useState<PublishJob[]>([]);
  const [jobsPage, setJobsPage] = useState(1);
  const [jobsTotal, setJobsTotal] = useState(0);
  const [uploading, setUploading] = useState<Record<string, boolean>>({});
  const [modal, setModal] = useState<PublishModal | null>(null);
  const [editingJob, setEditingJob] = useState<{ id: string; value: string } | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [downloading, setDownloading] = useState<Record<string, boolean>>({});

  const toggleGroup = (groupKey: string) =>
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      next.has(groupKey) ? next.delete(groupKey) : next.add(groupKey);
      return next;
    });

  const expandAll = () =>
    setExpandedGroups(new Set(buildClipGroups(clips).map((group) => group.key)));

  const collapseAll = () => setExpandedGroups(new Set());

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
    } catch {
      // silently ignore
    } finally {
      setLoading(false);
    }
  }, [token]);

  const fetchAccounts = useCallback(async () => {
    if (!token) return;
    try {
      const res = await fetch(`${API}/admin/publish/accounts`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setAllAccounts({
          instagram: data.instagram || [],
          youtube: data.youtube || [],
          tiktok: data.tiktok || [],
        });
      }
    } catch {
      // silently ignore
    }
  }, [token]);

  const fetchJobs = useCallback(async () => {
    if (!token) return;
    try {
      const offset = (jobsPage - 1) * jobsPageLimit;
      const res = await fetch(`${API}/admin/publish/jobs?limit=${jobsPageLimit}&offset=${offset}`, {
        headers: { Authorization: `Bearer ${token}` },
      });
      if (res.ok) {
        const data = await res.json();
        setPublishJobs(data.items || data.data || []);
        setJobsTotal(data.total || 0);
      }
    } catch {
      // silently ignore
    }
  }, [token, jobsPage]);

  useEffect(() => {
    fetchClips();
    fetchAccounts();
    fetchJobs();
  }, [fetchClips, fetchAccounts, fetchJobs]);

  useEffect(() => {
    const totalGroups = buildClipGroups(clips).length;
    const totalPages = Math.max(1, Math.ceil(totalGroups / clipsPageLimit));
    if (clipsPage > totalPages) {
      setClipsPage(totalPages);
    }
  }, [clips, clipsPage]);

  useEffect(() => {
    const totalPages = Math.max(1, Math.ceil(jobsTotal / jobsPageLimit));
    if (jobsPage > totalPages) {
      setJobsPage(totalPages);
    }
  }, [jobsPage, jobsTotal]);

  const openModal = (clip: Clip) => {
    // Pre-select first account for each platform that has accounts configured
    const defaultJobs: SelectedJob[] = [];
    if (allAccounts.instagram.length > 0)
      defaultJobs.push({ platform: "instagram", account_name: allAccounts.instagram[0] });
    setModal({
      clip,
      selectedJobs: defaultJobs,
      mode: "now",
      scheduledFor: getNowTashkent(),
      results: null,
      scheduledCreated: false,
    });
  };

  const hasAnyAccount = allAccounts.instagram.length > 0 || allAccounts.youtube.length > 0 || allAccounts.tiktok.length > 0;

  // Toggle a specific platform+account combo in selectedJobs
  const toggleJob = (platform: Platform, accountName: string) => {
    setModal((prev) => {
      if (!prev) return prev;
      const key = `${platform}:${accountName}`;
      const exists = prev.selectedJobs.some((j) => `${j.platform}:${j.account_name}` === key);
      const updated = exists
        ? prev.selectedJobs.filter((j) => `${j.platform}:${j.account_name}` !== key)
        : [...prev.selectedJobs, { platform, account_name: accountName }];
      return { ...prev, selectedJobs: updated };
    });
  };

  // Upload now
  const handleUploadNow = async () => {
    if (!token || !modal || modal.selectedJobs.length === 0) return;
    const { clip, selectedJobs } = modal;
    setUploading((prev) => ({ ...prev, [clip.id]: true }));
    try {
      const res = await fetch(`${API}/admin/clips/${clip.id}/publish/now`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ jobs: selectedJobs }),
      });
      const data = await res.json();
      setModal((prev) => (prev ? { ...prev, results: data.results || [] } : prev));
      await fetchClips();
    } catch {
      // silently ignore
    } finally {
      setUploading((prev) => ({ ...prev, [clip.id]: false }));
    }
  };

  // Schedule upload
  const handleSchedule = async () => {
    if (!token || !modal || modal.selectedJobs.length === 0) return;
    const { clip, selectedJobs, scheduledFor } = modal;
    setUploading((prev) => ({ ...prev, [clip.id]: true }));
    try {
      const res = await fetch(`${API}/admin/clips/${clip.id}/publish/schedule`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({
          jobs: selectedJobs,
          scheduled_for: tashkentLocalToISO(scheduledFor),
        }),
      });
      const data = await res.json();
      if (res.ok && data.count > 0) {
        setModal((prev) => (prev ? { ...prev, scheduledCreated: true } : prev));
        await fetchJobs();
      } else {
        setModal((prev) =>
          prev
            ? {
                ...prev,
                results: [
                  {
                    platform: "instagram",
                    account_name: "system",
                    status: "failed",
                    error: data.error || "Rejalash muvaffaqiyatsiz",
                  },
                ],
              }
            : prev
        );
      }
    } catch {
      // silently ignore
    } finally {
      setUploading((prev) => ({ ...prev, [clip.id]: false }));
    }
  };

  const handleCancelJob = async (id: string) => {
    if (!token) return;
    setCancellingId(id);
    try {
      await fetch(`${API}/admin/publish/jobs/${id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      await fetchJobs();
    } finally {
      setCancellingId(null);
    }
  };

  const handleSaveEditTime = async (id: string, newLocalTime: string) => {
    if (!token) return;
    try {
      const res = await fetch(`${API}/admin/publish/jobs/${id}`, {
        method: "PATCH",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ scheduled_for: tashkentLocalToISO(newLocalTime) }),
      });
      if (res.ok) {
        setEditingJob(null);
        await fetchJobs();
      }
    } catch {
      // silently ignore
    }
  };

  const handleDownloadClip = (clip: Clip) => {
    if (!token) return;
    setDownloading((prev) => ({ ...prev, [clip.id]: true }));

    fetch(`${API}/admin/clips/${clip.id}/download`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then(async (res) => {
        if (!res.ok) {
          throw new Error(`download failed: ${res.status}`);
        }
        const blob = await res.blob();
        const objectUrl = window.URL.createObjectURL(blob);
        const link = document.createElement("a");
        link.href = objectUrl;
        link.download = clip.filename || "clip.mp4";
        document.body.appendChild(link);
        link.click();
        document.body.removeChild(link);
        window.URL.revokeObjectURL(objectUrl);
      })
      .catch(() => {
        // silently ignore
      })
      .finally(() => {
        setDownloading((prev) => ({ ...prev, [clip.id]: false }));
      });
  };

  const groupedClips = buildClipGroups(clips);
  const clipsTotalPages = Math.max(1, Math.ceil(groupedClips.length / clipsPageLimit));
  const pagedGroupedClips = groupedClips.slice(
    (clipsPage - 1) * clipsPageLimit,
    clipsPage * clipsPageLimit,
  );
  const pendingJobs = publishJobs.filter((j) => j.status === "pending" || j.status === "processing");
  const doneJobs = publishJobs.filter((j) => j.status === "success" || j.status === "failed");
  const jobsTotalPages = Math.max(1, Math.ceil(jobsTotal / jobsPageLimit));

  return (
    <div className="p-4 sm:p-8">
      <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-4 mb-6 sm:mb-8">
        <div>
          <h1 className="text-xl sm:text-2xl font-bold text-white">Kliplar</h1>
          <p className="text-gray-500 text-sm mt-1">
            {groupedClips.length} ta kontent · {clips.length} ta klip
          </p>
        </div>
        {clips.length > 0 && (
          <div className="flex items-center gap-2">
            <button
              onClick={expandAll}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-brand-border text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
            >
              <ChevronsUpDown size={13} />
              Barchasini ochish
            </button>
            <button
              onClick={collapseAll}
              className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-brand-border text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
            >
              <ChevronsDownUp size={13} />
              Barchasini yopish
            </button>
          </div>
        )}
      </div>

      {/* ── Clips table ─────────────────────────────────────────── */}
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
            Kino yoki serial episode tayyor bo&apos;lganda klip avtomatik yaratiladi.
          </p>
        </div>
      ) : (
        <div className="space-y-8">
          <div className="flex justify-end">
            <PaginationControls
              page={clipsPage}
              totalPages={clipsTotalPages}
              onPrev={() => setClipsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setClipsPage((prev) => Math.min(clipsTotalPages, prev + 1))}
            />
          </div>
          {pagedGroupedClips.map((group) => {
            const isExpanded = expandedGroups.has(group.key);
            const groupPublishJobs = publishJobs.filter((j) =>
              group.clips.some((c) => c.id === j.clip_id)
            );
            const igUploaded = group.clips.filter((c) => c.instagram_upload_count > 0).length;
            const ytUploaded = new Set(
              groupPublishJobs
                .filter((j) => j.platform === "youtube" && j.status === "success")
                .map((j) => j.clip_id)
            ).size;
            const ttUploaded = new Set(
              groupPublishJobs
                .filter((j) => j.platform === "tiktok" && j.status === "success")
                .map((j) => j.clip_id)
            ).size;
            const lastUpload = [
              ...group.clips.map((c) => c.last_instagram_upload_at).filter(Boolean),
              ...groupPublishJobs
                .filter((j) => j.status === "success" && j.executed_at)
                .map((j) => j.executed_at!),
            ].sort().at(-1);
            const headerHref = group.slug
              ? group.kind === "movie"
                ? `/movies/${group.slug}`
                : `/series/${group.slug}`
              : "";

            return (
              <div
                key={group.key}
                className="bg-brand-card border border-brand-border rounded-xl overflow-hidden"
              >
                <button
                  onClick={() => toggleGroup(group.key)}
                  className="w-full px-4 sm:px-6 py-4 border-b border-brand-border bg-brand-dark/50 hover:bg-brand-dark/80 transition-colors text-left"
                >
                  <div className="flex items-center gap-3">
                    <span className="text-gray-500 flex-shrink-0">
                      {isExpanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}
                    </span>
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-3 flex-wrap">
                        <span className="text-white font-medium truncate">
                          {group.title}
                        </span>
                        {group.kind === "movie" && group.code && (
                          <span className="text-xs font-mono text-gray-500">
                            #{group.code}
                          </span>
                        )}
                        {group.kind === "movie" ? (
                          <span className="text-xs text-emerald-300/80">Movie clips</span>
                        ) : (
                          <span className="text-xs text-amber-300/80">Serial episode clips</span>
                        )}
                        <span className="text-xs text-gray-600">
                          {group.clips.length} ta klip
                        </span>
                      </div>
                      <div className="flex items-center gap-3 mt-1.5 flex-wrap">
                        {igUploaded > 0 && (
                          <span className="inline-flex items-center gap-1 text-[11px] text-pink-400">
                            <Instagram size={10} />
                            {igUploaded}/{group.clips.length}
                          </span>
                        )}
                        {ytUploaded > 0 && (
                          <span className="inline-flex items-center gap-1 text-[11px] text-red-400">
                            <Youtube size={10} />
                            {ytUploaded}/{group.clips.length}
                          </span>
                        )}
                        {ttUploaded > 0 && (
                          <span className="inline-flex items-center gap-1 text-[11px] text-sky-400">
                            <Music2 size={10} />
                            {ttUploaded}/{group.clips.length}
                          </span>
                        )}
                        {igUploaded === 0 && ytUploaded === 0 && ttUploaded === 0 && (
                          <span className="text-[11px] text-gray-600">Yuklanmagan</span>
                        )}
                        {lastUpload && (
                          <span className="text-[11px] text-gray-600">
                            · {formatTashkent(lastUpload)}
                          </span>
                        )}
                      </div>
                    </div>
                    {headerHref && (
                      <a
                        href={headerHref}
                        target="_blank"
                        rel="noopener noreferrer"
                        onClick={(e) => e.stopPropagation()}
                        className="text-gray-600 hover:text-white transition-colors flex-shrink-0"
                      >
                        <ExternalLink size={15} />
                      </a>
                    )}
                  </div>
                </button>

                {isExpanded && (
                  <div className="overflow-x-auto">
                    {group.kind === "movie" ? (
                      <table className="w-full text-sm">
                        <thead>
                          <tr className="border-b border-brand-border text-gray-500 text-xs uppercase tracking-wider">
                            <th className="text-left px-4 py-3">#</th>
                            <th className="text-left px-4 py-3">Fayl</th>
                            <th className="text-left px-4 py-3">Davom</th>
                            <th className="text-left px-4 py-3">Saxlash</th>
                            <th className="text-left px-4 py-3">Platformlar</th>
                            <th className="text-right px-4 py-3">Amallar</th>
                          </tr>
                        </thead>
                        <tbody>
                          {[...group.clips]
                            .sort((a, b) => getClipSequence(a) - getClipSequence(b))
                            .map((clip) => {
                              const clipJobs = publishJobs.filter((j) => j.clip_id === clip.id);
                              return (
                                <tr
                                  key={clip.id}
                                  className="border-b border-brand-border/50 last:border-0 hover:bg-brand-border/20 transition-colors"
                                >
                                  <td className="px-4 py-3 text-gray-500">{getClipSequence(clip)}</td>
                                  <td className="px-4 py-3">
                                    <span className="text-gray-300 font-mono text-xs">{clip.filename}</span>
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
                                    <PlatformUploadStatuses clip={clip} clipJobs={clipJobs} />
                                  </td>
                                  <td className="px-4 py-3">
                                    <div className="flex items-center justify-end gap-2">
                                      <a
                                        href={resolveClipOpenUrl(clip)}
                                        target="_blank"
                                        rel="noopener noreferrer"
                                        className="text-gray-500 hover:text-gray-300 transition-colors"
                                        title="Klipni ochish"
                                      >
                                        <ExternalLink size={14} />
                                      </a>
                                      <button
                                        onClick={() => handleDownloadClip(clip)}
                                        disabled={!token || downloading[clip.id]}
                                        title="Klipni yuklab olish"
                                        className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-brand-border text-gray-300 border border-brand-border hover:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed"
                                      >
                                        {downloading[clip.id] ? (
                                          <Loader2 size={12} className="animate-spin" />
                                        ) : (
                                          <Download size={12} />
                                        )}
                                        {downloading[clip.id] ? "..." : "Download"}
                                      </button>
                                      <button
                                        onClick={() => openModal(clip)}
                                        disabled={uploading[clip.id]}
                                        title="Ijtimoiy tarmoqlarga yuklash"
                                        className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-brand-border text-gray-300 border border-brand-border hover:bg-white/10 disabled:opacity-50"
                                      >
                                        {uploading[clip.id] ? (
                                          <Loader2 size={12} className="animate-spin" />
                                        ) : (
                                          <Upload size={12} />
                                        )}
                                        {uploading[clip.id] ? "..." : "Publish"}
                                      </button>
                                    </div>
                                  </td>
                                </tr>
                              );
                            })}
                        </tbody>
                      </table>
                    ) : (
                      <div className="divide-y divide-brand-border/50">
                        {group.seasons?.map((season) => (
                          <div key={season.key}>
                            <div className="px-4 sm:px-6 py-3 bg-white/[0.03] border-b border-brand-border/50">
                              <p className="text-sm font-medium text-gray-200">{season.title}</p>
                            </div>
                            <div className="divide-y divide-brand-border/30">
                              {season.episodes.map((episode) => (
                                <div key={episode.key}>
                                  <div className="px-4 sm:px-6 py-3 bg-white/[0.02]">
                                    <p className="text-sm text-white">{episode.title}</p>
                                    <p className="text-xs text-gray-500 mt-1">
                                      {episode.clips.length} ta klip
                                    </p>
                                  </div>
                                  <table className="w-full text-sm">
                                    <thead>
                                      <tr className="border-y border-brand-border/50 text-gray-500 text-xs uppercase tracking-wider">
                                        <th className="text-left px-4 py-3">#</th>
                                        <th className="text-left px-4 py-3">Fayl</th>
                                        <th className="text-left px-4 py-3">Davom</th>
                                        <th className="text-left px-4 py-3">Saxlash</th>
                                        <th className="text-left px-4 py-3">Platformlar</th>
                                        <th className="text-right px-4 py-3">Amallar</th>
                                      </tr>
                                    </thead>
                                    <tbody>
                                      {episode.clips.map((clip) => {
                                        const clipJobs = publishJobs.filter((j) => j.clip_id === clip.id);
                                        return (
                                          <tr
                                            key={clip.id}
                                            className="border-b border-brand-border/30 last:border-0 hover:bg-brand-border/20 transition-colors"
                                          >
                                            <td className="px-4 py-3 text-gray-500">{getClipSequence(clip)}</td>
                                            <td className="px-4 py-3">
                                              <span className="text-gray-300 font-mono text-xs">{clip.filename}</span>
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
                                              <PlatformUploadStatuses clip={clip} clipJobs={clipJobs} />
                                            </td>
                                            <td className="px-4 py-3">
                                              <div className="flex items-center justify-end gap-2">
                                                <a
                                                  href={resolveClipOpenUrl(clip)}
                                                  target="_blank"
                                                  rel="noopener noreferrer"
                                                  className="text-gray-500 hover:text-gray-300 transition-colors"
                                                  title="Klipni ochish"
                                                >
                                                  <ExternalLink size={14} />
                                                </a>
                                                <button
                                                  onClick={() => handleDownloadClip(clip)}
                                                  disabled={!token || downloading[clip.id]}
                                                  title="Klipni yuklab olish"
                                                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-brand-border text-gray-300 border border-brand-border hover:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed"
                                                >
                                                  {downloading[clip.id] ? (
                                                    <Loader2 size={12} className="animate-spin" />
                                                  ) : (
                                                    <Download size={12} />
                                                  )}
                                                  {downloading[clip.id] ? "..." : "Download"}
                                                </button>
                                                <button
                                                  onClick={() => openModal(clip)}
                                                  disabled={uploading[clip.id]}
                                                  title="Ijtimoiy tarmoqlarga yuklash"
                                                  className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-brand-border text-gray-300 border border-brand-border hover:bg-white/10 disabled:opacity-50"
                                                >
                                                  {uploading[clip.id] ? (
                                                    <Loader2 size={12} className="animate-spin" />
                                                  ) : (
                                                    <Upload size={12} />
                                                  )}
                                                  {uploading[clip.id] ? "..." : "Publish"}
                                                </button>
                                              </div>
                                            </td>
                                          </tr>
                                        );
                                      })}
                                    </tbody>
                                  </table>
                                </div>
                              ))}
                            </div>
                          </div>
                        ))}
                      </div>
                    )}
                  </div>
                )}
              </div>
            );
          })}
          <div className="flex justify-end">
            <PaginationControls
              page={clipsPage}
              totalPages={clipsTotalPages}
              onPrev={() => setClipsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setClipsPage((prev) => Math.min(clipsTotalPages, prev + 1))}
            />
          </div>
        </div>
      )}

      {/* ── Scheduled publish jobs ───────────────────────────────── */}
      {jobsTotal > 0 && (
        <div className="mt-10">
          <div className="flex flex-col sm:flex-row sm:items-center justify-between gap-3 mb-4">
            <h2 className="text-lg font-bold text-white flex items-center gap-2">
              <CalendarClock size={20} className="text-blue-400" />
              Rejalashtirilgan yuklamalar
            </h2>
            <PaginationControls
              page={jobsPage}
              totalPages={jobsTotalPages}
              onPrev={() => setJobsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setJobsPage((prev) => Math.min(jobsTotalPages, prev + 1))}
            />
          </div>

          {/* Pending / processing */}
          {pendingJobs.length > 0 && (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden mb-4">
              <div className="px-4 py-3 border-b border-brand-border bg-brand-dark/50">
                <span className="text-sm text-gray-400">Kutilayotgan</span>
              </div>
              <div className="divide-y divide-brand-border/50">
                {pendingJobs.map((j) => (
                  <div
                    key={j.id}
                    className="px-4 py-3 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="text-white text-sm font-medium truncate">{j.movie_title}</p>
                        <PlatformBadge platform={j.platform} />
                      </div>
                      <div className="flex flex-wrap items-center gap-2 mt-1 text-xs text-gray-500">
                        <span className="font-mono">{j.account_name}</span>
                        <span>·</span>
                        <span className="text-blue-400 font-medium">
                          {formatTashkent(j.scheduled_for)}
                        </span>
                      </div>
                    </div>
                    <div className="flex items-center gap-2 shrink-0">
                      <JobStatusBadge status={j.status} />
                      {j.status === "pending" && (
                        <>
                          {editingJob?.id === j.id ? (
                            <div className="flex items-center gap-1">
                              <input
                                type="datetime-local"
                                value={editingJob.value}
                                onChange={(e) =>
                                  setEditingJob({ id: j.id, value: e.target.value })
                                }
                                className="bg-brand-dark border border-brand-border rounded px-2 py-1 text-xs text-white"
                              />
                              <button
                                onClick={() => handleSaveEditTime(j.id, editingJob.value)}
                                className="text-green-400 hover:text-green-300 transition-colors px-1"
                                title="Saqlash"
                              >
                                <CheckCircle size={15} />
                              </button>
                              <button
                                onClick={() => setEditingJob(null)}
                                className="text-gray-500 hover:text-gray-300 transition-colors px-1"
                                title="Bekor"
                              >
                                <X size={15} />
                              </button>
                            </div>
                          ) : (
                            <button
                              onClick={() => {
                                const localVal = new Intl.DateTimeFormat("sv-SE", {
                                  timeZone: TASHKENT_TZ,
                                  year: "numeric",
                                  month: "2-digit",
                                  day: "2-digit",
                                  hour: "2-digit",
                                  minute: "2-digit",
                                })
                                  .format(new Date(j.scheduled_for))
                                  .replace(" ", "T");
                                setEditingJob({ id: j.id, value: localVal });
                              }}
                              className="text-gray-500 hover:text-blue-400 transition-colors"
                              title="Vaqtni tahrirlash"
                            >
                              <Pencil size={14} />
                            </button>
                          )}
                          <button
                            onClick={() => handleCancelJob(j.id)}
                            disabled={cancellingId === j.id}
                            className="text-gray-500 hover:text-red-400 transition-colors disabled:opacity-50"
                            title="Bekor qilish"
                          >
                            {cancellingId === j.id ? (
                              <Loader2 size={14} className="animate-spin" />
                            ) : (
                              <Trash2 size={14} />
                            )}
                          </button>
                        </>
                      )}
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}

          {/* Done (success / failed) */}
          {doneJobs.length > 0 && (
            <div className="bg-brand-card border border-brand-border rounded-xl overflow-hidden">
              <div className="px-4 py-3 border-b border-brand-border bg-brand-dark/50">
                <span className="text-sm text-gray-400">Bajarilgan</span>
              </div>
              <div className="divide-y divide-brand-border/50">
                {doneJobs.map((j) => (
                  <div
                    key={j.id}
                    className="px-4 py-3 flex flex-col sm:flex-row sm:items-center gap-2 sm:gap-4"
                  >
                    <div className="flex-1 min-w-0">
                      <div className="flex items-center gap-2">
                        <p className="text-white text-sm font-medium truncate">{j.movie_title}</p>
                        <PlatformBadge platform={j.platform} />
                      </div>
                      <div className="flex flex-wrap items-center gap-2 mt-1 text-xs text-gray-500">
                        <span className="font-mono">{j.account_name}</span>
                        <span>·</span>
                        <span>{formatTashkent(j.scheduled_for)}</span>
                        {j.executed_at && (
                          <>
                            <span>·</span>
                            <span className="text-gray-600">
                              bajarildi: {formatTashkent(j.executed_at)}
                            </span>
                          </>
                        )}
                      </div>
                      {j.error && (
                        <p className="text-xs text-red-400 mt-1 truncate">{j.error}</p>
                      )}
                    </div>
                    <JobStatusBadge status={j.status} />
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="flex justify-end mt-4">
            <PaginationControls
              page={jobsPage}
              totalPages={jobsTotalPages}
              onPrev={() => setJobsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setJobsPage((prev) => Math.min(jobsTotalPages, prev + 1))}
            />
          </div>
        </div>
      )}

      {/* ── Publish Modal ──────────────────────────────────────────── */}
      {modal && (
        <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/60 backdrop-blur-sm">
          <div className="bg-brand-card border border-brand-border rounded-xl w-full max-w-sm shadow-2xl max-h-[90vh] overflow-y-auto">
            {/* Header */}
            <div className="flex items-center justify-between px-5 py-4 border-b border-brand-border sticky top-0 bg-brand-card">
              <div>
                <h2 className="text-white font-semibold text-sm">Ijtimoiy tarmoqlarga yuklash</h2>
                <p className="text-gray-500 text-xs mt-0.5 truncate max-w-[220px]">
                  {getClipDisplayTitle(modal.clip)} — klip #{getClipSequence(modal.clip)}
                </p>
              </div>
              <button
                onClick={() => setModal(null)}
                className="text-gray-500 hover:text-white transition-colors"
              >
                <X size={18} />
              </button>
            </div>

            {/* Schedule created success */}
            {modal.scheduledCreated ? (
              <div className="px-5 py-6 text-center space-y-3">
                <div className="w-12 h-12 rounded-full bg-blue-500/15 flex items-center justify-center mx-auto">
                  <CalendarClock size={24} className="text-blue-400" />
                </div>
                <p className="text-white font-medium">Rejalashtirildi!</p>
                <p className="text-gray-400 text-sm">
                  Yuklash vaqti:{" "}
                  <span className="text-blue-400 font-medium">
                    {formatTashkent(tashkentLocalToISO(modal.scheduledFor))}
                  </span>
                </p>
                <p className="text-gray-500 text-xs">
                  {modal.selectedJobs.map((j) => `${PLATFORM_META[j.platform].label}/${j.account_name}`).join(", ")}
                </p>
                <button
                  onClick={() => setModal(null)}
                  className="w-full mt-2 py-2 rounded-lg bg-brand-border text-gray-300 text-sm hover:bg-white/10 transition-colors"
                >
                  Yopish
                </button>
              </div>
            ) : modal.results ? (
              /* Upload results */
              <div className="px-5 py-4 space-y-3">
                <p className="text-gray-400 text-xs mb-1">Yuklash natijalari:</p>
                {modal.results.map((r, i) => (
                  <div
                    key={i}
                    className={`rounded-lg border px-3 py-2.5 ${
                      r.status === "success"
                        ? "border-green-500/30 bg-green-500/5"
                        : "border-red-500/30 bg-red-500/5"
                    }`}
                  >
                    <div className="flex items-center justify-between mb-1">
                      <div className="flex items-center gap-2">
                        <PlatformBadge platform={r.platform} />
                        <span className="text-gray-200 text-sm font-mono">{r.account_name}</span>
                      </div>
                      {r.status === "success" ? (
                        <span className="inline-flex items-center gap-1 text-xs text-green-400 font-medium">
                          <CheckCircle size={13} /> OK
                        </span>
                      ) : (
                        <span className="inline-flex items-center gap-1 text-xs text-red-400 font-medium">
                          <XCircle size={13} /> Xato
                        </span>
                      )}
                    </div>
                    {r.status === "failed" && r.error && (
                      <p className="text-xs text-red-300 mt-1 break-words">{r.error}</p>
                    )}
                  </div>
                ))}
                <button
                  onClick={() => setModal(null)}
                  className="w-full py-2 rounded-lg bg-brand-border text-gray-300 text-sm hover:bg-white/10 transition-colors"
                >
                  Yopish
                </button>
              </div>
            ) : (
              /* Picker */
              <div className="px-5 py-4 space-y-4">
                {!hasAnyAccount ? (
                  <p className="text-gray-500 text-sm text-center py-4">
                    Hech qanday akkaunt sozlanmagan.
                    <br />
                    <span className="text-xs text-gray-600">
                      INSTAGRAM_ACCOUNTS_JSON / YOUTUBE_ACCOUNTS_JSON / TIKTOK_ACCOUNTS_JSON env o&apos;rnatilmagan.
                    </span>
                  </p>
                ) : (
                  <>
                    {/* Platform + account selectors */}
                    <div className="space-y-4">
                      {ALL_PLATFORMS.map((platform) => {
                        const accounts = allAccounts[platform];
                        const meta = PLATFORM_META[platform];
                        const { Icon } = meta;
                        if (accounts.length === 0) return null;
                        const allChecked = accounts.every((name) =>
                          modal.selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                        );
                        const someChecked = accounts.some((name) =>
                          modal.selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                        );
                        const toggleAll = () => {
                          if (allChecked) {
                            setModal((p) => p ? {
                              ...p,
                              selectedJobs: p.selectedJobs.filter((j) => j.platform !== platform),
                            } : p);
                          } else {
                            setModal((p) => {
                              if (!p) return p;
                              const existing = p.selectedJobs.filter((j) => j.platform !== platform);
                              return {
                                ...p,
                                selectedJobs: [
                                  ...existing,
                                  ...accounts.map((name) => ({ platform, account_name: name })),
                                ],
                              };
                            });
                          }
                        };
                        return (
                          <div key={platform} className="rounded-lg border border-brand-border overflow-hidden">
                            {/* Platform header row */}
                            <label
                              className={`flex items-center gap-3 px-3 py-2.5 cursor-pointer transition-colors ${
                                someChecked ? meta.bgColor : "bg-white/5"
                              } hover:bg-white/10`}
                            >
                              <input
                                type="checkbox"
                                checked={allChecked}
                                ref={(el) => { if (el) el.indeterminate = someChecked && !allChecked; }}
                                onChange={toggleAll}
                                className="w-4 h-4 cursor-pointer"
                              />
                              <Icon size={14} className={meta.color} />
                              <span className={`text-sm font-semibold ${meta.color}`}>{meta.label}</span>
                              <span className="ml-auto text-xs text-gray-500">
                                {accounts.filter((name) =>
                                  modal.selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                                ).length}/{accounts.length}
                              </span>
                            </label>
                            {/* Account rows */}
                            <div className="divide-y divide-brand-border">
                              {accounts.map((name) => {
                                const checked = modal.selectedJobs.some(
                                  (j) => j.platform === platform && j.account_name === name
                                );
                                return (
                                  <label
                                    key={name}
                                    className={`flex items-center gap-3 pl-8 pr-3 py-2 cursor-pointer transition-colors ${
                                      checked ? meta.bgColor : "hover:bg-white/5"
                                    }`}
                                  >
                                    <input
                                      type="checkbox"
                                      checked={checked}
                                      onChange={() => toggleJob(platform, name)}
                                      className="w-4 h-4 cursor-pointer"
                                    />
                                    <span className="text-gray-200 text-sm font-mono">{name}</span>
                                  </label>
                                );
                              })}
                            </div>
                          </div>
                        );
                      })}
                    </div>

                    {modal.selectedJobs.length > 0 && (
                      <p className="text-xs text-gray-500">
                        Tanlangan:{" "}
                        {modal.selectedJobs
                          .map((j) => `${PLATFORM_META[j.platform].label}/${j.account_name}`)
                          .join(", ")}
                      </p>
                    )}

                    {/* Mode toggle */}
                    <div className="flex rounded-lg border border-brand-border overflow-hidden">
                      <button
                        onClick={() => setModal((p) => (p ? { ...p, mode: "now" } : p))}
                        className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                          modal.mode === "now"
                            ? "bg-brand-red text-white"
                            : "text-gray-400 hover:text-white hover:bg-white/5"
                        }`}
                      >
                        <Upload size={13} />
                        Hozir yuklash
                      </button>
                      <button
                        onClick={() => setModal((p) => (p ? { ...p, mode: "scheduled" } : p))}
                        className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                          modal.mode === "scheduled"
                            ? "bg-blue-600 text-white"
                            : "text-gray-400 hover:text-white hover:bg-white/5"
                        }`}
                      >
                        <Calendar size={13} />
                        Vaqt belgilash
                      </button>
                    </div>

                    {/* Datetime picker */}
                    {modal.mode === "scheduled" && (
                      <div>
                        <label className="block text-gray-400 text-xs mb-1.5">
                          Yuklash vaqti{" "}
                          <span className="text-gray-600">(Toshkent vaqti, UTC+5)</span>
                        </label>
                        <input
                          type="datetime-local"
                          value={modal.scheduledFor}
                          onChange={(e) =>
                            setModal((p) => (p ? { ...p, scheduledFor: e.target.value } : p))
                          }
                          className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
                        />
                      </div>
                    )}
                  </>
                )}

                {/* Action buttons */}
                <div className="flex gap-2 pt-1">
                  <button
                    onClick={() => setModal(null)}
                    className="flex-1 py-2 rounded-lg border border-brand-border text-gray-400 text-sm hover:bg-white/5 transition-colors"
                  >
                    Bekor
                  </button>
                  {modal.mode === "now" ? (
                    <button
                      onClick={handleUploadNow}
                      disabled={
                        modal.selectedJobs.length === 0 ||
                        uploading[modal.clip.id] ||
                        !hasAnyAccount
                      }
                      className="flex-1 py-2 rounded-lg bg-brand-red text-white text-sm font-medium hover:bg-brand-red/80 transition-colors disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-2"
                    >
                      {uploading[modal.clip.id] ? (
                        <>
                          <Loader2 size={14} className="animate-spin" /> Yuklanmoqda...
                        </>
                      ) : (
                        <>
                          <Upload size={14} /> Yuklash
                        </>
                      )}
                    </button>
                  ) : (
                    <button
                      onClick={handleSchedule}
                      disabled={
                        modal.selectedJobs.length === 0 ||
                        uploading[modal.clip.id] ||
                        !hasAnyAccount
                      }
                      className="flex-1 py-2 rounded-lg bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-2"
                    >
                      {uploading[modal.clip.id] ? (
                        <>
                          <Loader2 size={14} className="animate-spin" /> Saqlanmoqda...
                        </>
                      ) : (
                        <>
                          <CalendarClock size={14} /> Rejalash
                        </>
                      )}
                    </button>
                  )}
                </div>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
