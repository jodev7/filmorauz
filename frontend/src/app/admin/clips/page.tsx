"use client";

import React, { useEffect, useState, useCallback, useMemo } from "react";
import {
  Film,
  ExternalLink,
  Download,
  Clock,
  Loader2,
  Scissors,
  Tv,
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
import { useToast } from "@/components/admin/Toast";
import { EmptyState, GhostButton, PageHead, SkeletonList, StatTile } from "@/components/admin/kit";
import { readUrlBool, readUrlList, readUrlNumber, readUrlParam, useSyncUrlParams } from "@/lib/url-state";

// ─── Constants ───────────────────────────────────────────────────────────────

const API = process.env.NEXT_PUBLIC_API_URL || "http://localhost:8080/api";
const TASHKENT_TZ = "Asia/Tashkent";
const CDN_BASE_URL =
  (process.env.NEXT_PUBLIC_CDN_BASE_URL || "").trim().replace(/\/+$/, "") ||
  "https://cdn.filmorauz.net/file/filmorauznet";
const CLIPS_PAGE_LIMIT = 20;

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
  caption?: string;
  hashtags?: string[];
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
  status: "pending" | "scheduled" | "processing" | "success" | "failed";
  created_at: string;
  executed_at?: string;
  error?: string;
}

const ACTIVE_PUBLISH_JOB_STATUSES = new Set<PublishJob["status"]>([
  "pending",
  "scheduled",
  "processing",
]);

function isActivePublishJob(job: Pick<PublishJob, "status">): boolean {
  return ACTIVE_PUBLISH_JOB_STATUSES.has(job.status);
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

// Server-side group summary tree (counts only — no clip docs).
interface ServerEpisodeGroup {
  group_key: string;
  episode_id: string;
  episode_number: number;
  title: string;
  clip_count: number;
  ig_uploaded_count: number;
  last_ig_upload_at?: string;
  match_episode_ids?: string[];
}
interface ServerSeasonGroup {
  season_number: number;
  clip_count: number;
  episodes: ServerEpisodeGroup[];
}
interface ServerSeriesGroup {
  group_key: string;
  series_id: string;
  title: string;
  slug: string;
  genre?: string[];
  clip_count: number;
  ig_uploaded_count: number;
  last_ig_upload_at?: string;
  seasons: ServerSeasonGroup[];
  match_series_ids?: string[];
  match_series_slugs?: string[];
  match_series_titles?: string[];
}
interface ServerMovieGroup {
  group_key: string;
  movie_id: string;
  title: string;
  slug: string;
  code: string;
  genre?: string[];
  clip_count: number;
  ig_uploaded_count: number;
  last_ig_upload_at?: string;
  match_movie_ids?: string[];
  match_movie_codes?: string[];
  match_movie_slugs?: string[];
  match_movie_titles?: string[];
}

interface InstagramAccountFilter {
  kind?: "movie" | "series" | "";
  genres?: string[];
  series_slugs?: string[];
  movie_slugs?: string[];
}

interface InstagramAccountWithFilter {
  name: string;
  filter?: InstagramAccountFilter;
}

type ClipKind = "all" | "movie" | "series";
type ClipSort = "title" | "newest" | "most_clips" | "least_posted";
interface ServerGroups {
  movies: ServerMovieGroup[];
  series: ServerSeriesGroup[];
  total_clips: number;
  total_contents: number;
  total_filtered?: number;
  total_movies?: number;
  total_series?: number;
  movie_group_count?: number;
  series_group_count?: number;
  all_genres?: string[];
}

// Lazy-loaded clip page for a single scope (movie or episode).
interface ScopeClipPage {
  clips: Clip[];
  total: number;
  offset: number;
  loading: boolean;
  error?: string;
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

  for (const platform of ALL_PLATFORMS) {
    const jobs = clipJobs.filter((j) => j.platform === platform);
    
    // Legacy fallback for instagram if no modern jobs are present
    if (platform === "instagram" && jobs.length === 0) {
      if (clip.instagram_upload_count > 0 || clip.last_instagram_upload_status) {
        const meta = PLATFORM_META.instagram;
        const { Icon } = meta;
        const ok = clip.last_instagram_upload_status === "success";
        const failed = clip.last_instagram_upload_status === "failed";
        rows.push(
          <div key="instagram_legacy" className="flex items-center gap-1.5">
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
      continue;
    }

    if (jobs.length === 0) continue;
    const meta = PLATFORM_META[platform];
    const { Icon } = meta;
    const successJobs = jobs.filter((j) => j.status === "success");
    const pendingJobs = jobs.filter(isActivePublishJob);
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
          <span className="inline-flex items-center gap-1 text-[11px] text-amber-400">
            <CalendarClock size={10} />
            Rejalashtirilgan ({formatTashkent(pendingJobs[0].scheduled_for)})
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
        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-white/10 text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
      >
        Prev
      </button>
      <span className="text-xs text-gray-500 min-w-[88px] text-center">
        Page {totalPages === 0 ? 0 : page} of {totalPages}
      </span>
      <button
        onClick={onNext}
        disabled={page >= totalPages}
        className="inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-white/10 text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
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

// Composes the default Instagram caption for a clip. "Full Gemini": when the
// clip carries an AI caption we use it + its hashtags verbatim; the admin can
// still edit the result in the upload modal before publishing. Falls back to
// the legacy code template for clips with no AI caption.
function buildInstagramCaption(clip: Clip): string {
  const aiCaption = clip.caption?.trim();
  if (aiCaption) {
    const tags = (clip.hashtags || [])
      .map((h) => h.trim())
      .filter(Boolean)
      .map((h) => (h.startsWith("#") ? h : `#${h.replace(/^#+/, "")}`));
    return tags.length > 0 ? `${aiCaption}\n\n${tags.join(" ")}` : aiCaption;
  }
  const code = clip.movie_code?.trim() || "";
  return `🎬 Kinoni profildagi bot orqali toping!\n🔢 Kino Kodi: ${code}`;
}

function getClipSequence(clip: Clip): number {
  return clip.clip_index || clip.sequence || 0;
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

// Scope keys identify the clip page cache slot for a single content group.
type Scope =
  | {
      kind: "movie";
      groupKey: string;
      movieId?: string;
      movieIds?: string[];
      movieCodes?: string[];
      movieSlugs?: string[];
      movieTitles?: string[];
    }
  | { kind: "episode"; groupKey: string; episodeId?: string; episodeIds?: string[] };

function scopeKey(scope: Scope): string {
  return scope.kind === "movie" ? `movie:${scope.groupKey}` : `episode:${scope.groupKey}`;
}

// ─── Clip table (shared by movie groups and episode groups) ──────────────────

function movieGroupScope(m: ServerMovieGroup): Scope {
  return {
    kind: "movie",
    groupKey: m.group_key,
    movieId: m.movie_id || undefined,
    movieIds: m.match_movie_ids,
    movieCodes: m.match_movie_codes,
    movieSlugs: m.match_movie_slugs,
    movieTitles: m.match_movie_titles,
  };
}

function episodeGroupScope(ep: ServerEpisodeGroup): Scope {
  return {
    kind: "episode",
    groupKey: ep.group_key,
    episodeId: ep.episode_id || undefined,
    episodeIds: ep.match_episode_ids,
  };
}

const EMPTY_JOBS: PublishJob[] = [];

function ClipTableBase({
  page,
  scope,
  jobsByClip,
  downloading,
  uploading,
  token,
  onDownload,
  onPublish,
  onPageChange,
}: {
  page: ScopeClipPage;
  // Stable identity (built once per groups response) so React.memo holds.
  scope: Scope;
  jobsByClip: Map<string, PublishJob[]>;
  downloading: Record<string, boolean>;
  uploading: Record<string, boolean>;
  token: string | null;
  onDownload: (clip: Clip) => void;
  onPublish: (clip: Clip) => void;
  onPageChange: (scope: Scope, offset: number) => void;
}) {
  const totalPages = Math.max(1, Math.ceil(page.total / CLIPS_PAGE_LIMIT));
  const pageNum = Math.floor(page.offset / CLIPS_PAGE_LIMIT) + 1;
  const onPrev = () => onPageChange(scope, Math.max(0, page.offset - CLIPS_PAGE_LIMIT));
  const onNext = () => onPageChange(scope, page.offset + CLIPS_PAGE_LIMIT);
  if (page.loading && page.clips.length === 0) {
    return (
      <div className="flex items-center gap-2 text-gray-500 py-8 px-4 justify-center">
        <Loader2 size={14} className="animate-spin" />
        Kliplar yuklanmoqda...
      </div>
    );
  }
  if (page.error) {
    return <div className="px-4 py-6 text-sm text-red-400">{page.error}</div>;
  }
  if (page.clips.length === 0) {
    return <div className="px-4 py-6 text-sm text-gray-500">Klip topilmadi.</div>;
  }
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm">
        <thead>
          <tr className="border-b border-white/10 text-gray-500 text-xs uppercase tracking-wider">
            <th className="text-left px-4 py-3">#</th>
            <th className="text-left px-4 py-3">Fayl</th>
            <th className="text-left px-4 py-3">Davom</th>
            <th className="text-left px-4 py-3">Saxlash</th>
            <th className="text-left px-4 py-3">Platformlar</th>
            <th className="text-right px-4 py-3">Amallar</th>
          </tr>
        </thead>
        <tbody>
          {page.clips.map((clip) => {
            const clipJobs = jobsByClip.get(clip.id) ?? EMPTY_JOBS;
            return (
              <tr
                key={clip.id}
                className="border-b border-white/5 last:border-0 hover:bg-orange-500/10 transition-colors"
              >
                <td className="px-4 py-3 text-gray-500">{getClipSequence(clip)}</td>
                <td className="px-4 py-3">
                  <div className="flex flex-col gap-1.5 items-start">
                    <span className="text-gray-300 font-mono text-xs break-all">{clip.filename}</span>
                    {clipJobs.some(isActivePublishJob) && (
                      <span className="flex items-center gap-1 text-[10px] bg-amber-500/10 text-amber-400 px-1.5 py-0.5 rounded border border-amber-500/20 font-medium">
                        <CalendarClock size={12} />
                        Rejalashtirilgan
                      </span>
                    )}
                  </div>
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
                      onClick={() => onDownload(clip)}
                      disabled={!token || downloading[clip.id]}
                      title="Klipni yuklab olish"
                      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-white/10 text-gray-300 border border-white/10 hover:bg-white/10 disabled:opacity-50 disabled:cursor-not-allowed"
                    >
                      {downloading[clip.id] ? (
                        <Loader2 size={12} className="animate-spin" />
                      ) : (
                        <Download size={12} />
                      )}
                      {downloading[clip.id] ? "..." : "Download"}
                    </button>
                    <button
                      onClick={() => onPublish(clip)}
                      disabled={uploading[clip.id]}
                      title="Ijtimoiy tarmoqlarga yuklash"
                      className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded text-xs font-medium transition-colors bg-white/10 text-gray-300 border border-white/10 hover:bg-white/10 disabled:opacity-50"
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
      {totalPages > 1 && (
        <div className="flex justify-end px-4 py-2 border-t border-white/5">
          <PaginationControls
            page={pageNum}
            totalPages={totalPages}
            onPrev={onPrev}
            onNext={onNext}
          />
        </div>
      )}
    </div>
  );
}

// Memoized so that unrelated parent re-renders (e.g. the publish modal, which
// now owns its own editable state) don't re-reconcile every expanded clip
// table. Props are stable: jobsByClip/downloading/uploading come from memoized
// or rarely-changing parent state, scope objects are memoized per groups
// response, and the handlers are useCallback-wrapped.
const ClipTable = React.memo(ClipTableBase);

// ─── Publish modal ────────────────────────────────────────────────────────────
//
// Self-contained: it owns ALL editable state (caption, selected accounts,
// schedule time, mode, results). This is deliberate — keeping this state out of
// the page component means typing in the caption or toggling an account no
// longer re-renders the entire clips page (all group tables + rows), which was
// the main cause of the modal feeling sluggish. The page only tells the modal
// WHICH clip to publish and how to refresh afterwards.
function PublishModal({
  clip,
  initialSelectedJobs,
  allAccounts,
  hasAnyAccount,
  token,
  onClose,
  onPublished,
  onBusyChange,
}: {
  clip: Clip;
  initialSelectedJobs: SelectedJob[];
  allAccounts: AllAccounts;
  hasAnyAccount: boolean;
  token: string | null;
  onClose: () => void;
  onPublished: (clip: Clip) => void | Promise<void>;
  onBusyChange: (busy: boolean) => void;
}) {
  const [selectedJobs, setSelectedJobs] = useState<SelectedJob[]>(initialSelectedJobs);
  const [mode, setMode] = useState<UploadMode>("now");
  const [scheduledFor, setScheduledFor] = useState<string>(getNowTashkent());
  const [caption, setCaption] = useState<string>(buildInstagramCaption(clip));
  const [results, setResults] = useState<JobResult[] | null>(null);
  const [scheduledCreated, setScheduledCreated] = useState(false);
  const [busy, setBusy] = useState(false);

  const setBusyBoth = (b: boolean) => {
    setBusy(b);
    onBusyChange(b);
  };

  const toggleJob = (platform: Platform, accountName: string) => {
    setSelectedJobs((prev) => {
      const key = `${platform}:${accountName}`;
      const exists = prev.some((j) => `${j.platform}:${j.account_name}` === key);
      return exists
        ? prev.filter((j) => `${j.platform}:${j.account_name}` !== key)
        : [...prev, { platform, account_name: accountName }];
    });
  };

  const handleUploadNow = async () => {
    if (!token || selectedJobs.length === 0) return;
    setBusyBoth(true);
    try {
      const res = await fetch(`${API}/admin/clips/${clip.id}/publish/now`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({ jobs: selectedJobs, caption }),
      });
      const data = await res.json().catch(() => ({}));
      if (!res.ok) throw new Error(data.error || `status ${res.status}`);
      setResults(data.results || []);
      await onPublished(clip);
    } catch (err) {
      setResults([
        {
          platform: "instagram",
          account_name: "system",
          status: "failed",
          error: err instanceof Error ? err.message : "Yuklash muvaffaqiyatsiz",
        },
      ]);
    } finally {
      setBusyBoth(false);
    }
  };

  const handleSchedule = async () => {
    if (!token || selectedJobs.length === 0) return;
    setBusyBoth(true);
    try {
      const res = await fetch(`${API}/admin/clips/${clip.id}/publish/schedule`, {
        method: "POST",
        headers: { Authorization: `Bearer ${token}`, "Content-Type": "application/json" },
        body: JSON.stringify({
          jobs: selectedJobs,
          scheduled_for: tashkentLocalToISO(scheduledFor),
          caption,
        }),
      });
      const data = await res.json();
      if (res.ok && data.count > 0) {
        setScheduledCreated(true);
        await onPublished(clip);
      } else {
        setResults([
          {
            platform: "instagram",
            account_name: "system",
            status: "failed",
            error: data.error || "Rejalash muvaffaqiyatsiz",
          },
        ]);
      }
    } catch (err) {
      setResults([
        {
          platform: "instagram",
          account_name: "system",
          status: "failed",
          error: err instanceof Error ? err.message : "Rejalash muvaffaqiyatsiz",
        },
      ]);
    } finally {
      setBusyBoth(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center p-4 bg-black/70">
      <div className="bg-[#12121a] border border-white/10 rounded-2xl w-full max-w-sm shadow-2xl max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between px-5 py-4 border-b border-white/10 sticky top-0 bg-[#12121a]">
          <div>
            <h2 className="text-white font-semibold text-sm">Ijtimoiy tarmoqlarga yuklash</h2>
            <p className="text-gray-500 text-xs mt-0.5 truncate max-w-[220px]">
              {getClipDisplayTitle(clip)} — klip #{getClipSequence(clip)}
            </p>
          </div>
          <button
            onClick={onClose}
            className="text-gray-500 hover:text-white transition-colors"
          >
            <X size={18} />
          </button>
        </div>

        {scheduledCreated ? (
          <div className="px-5 py-6 text-center space-y-3">
            <div className="w-12 h-12 rounded-full bg-blue-500/15 flex items-center justify-center mx-auto">
              <CalendarClock size={24} className="text-blue-400" />
            </div>
            <p className="text-white font-medium">Rejalashtirildi!</p>
            <p className="text-gray-400 text-sm">
              Yuklash vaqti:{" "}
              <span className="text-blue-400 font-medium">
                {formatTashkent(tashkentLocalToISO(scheduledFor))}
              </span>
            </p>
            <p className="text-gray-500 text-xs">
              {selectedJobs.map((j) => `${PLATFORM_META[j.platform].label}/${j.account_name}`).join(", ")}
            </p>
            {selectedJobs.some((j) => j.platform === "instagram") && (
              <div className="rounded-lg border border-white/10 bg-black/40 px-3 py-2 text-left">
                <p className="text-[11px] uppercase tracking-wide text-gray-500 mb-1">Instagram caption</p>
                <pre className="whitespace-pre-wrap text-xs text-gray-200 font-sans">
                  {caption}
                </pre>
              </div>
            )}
            <button
              onClick={onClose}
              className="w-full mt-2 py-2 rounded-lg bg-white/10 text-gray-300 text-sm hover:bg-white/10 transition-colors"
            >
              Yopish
            </button>
          </div>
        ) : results ? (
          <div className="px-5 py-4 space-y-3">
            <p className="text-gray-400 text-xs mb-1">Yuklash natijalari:</p>
            {results.map((r, i) => (
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
              onClick={onClose}
              className="w-full py-2 rounded-lg bg-white/10 text-gray-300 text-sm hover:bg-white/10 transition-colors"
            >
              Yopish
            </button>
          </div>
        ) : (
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
                <div className="space-y-4">
                  {ALL_PLATFORMS.map((platform) => {
                    const accounts = allAccounts[platform];
                    const meta = PLATFORM_META[platform];
                    const { Icon } = meta;
                    if (accounts.length === 0) return null;
                    const allChecked = accounts.every((name) =>
                      selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                    );
                    const someChecked = accounts.some((name) =>
                      selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                    );
                    const toggleAll = () => {
                      if (allChecked) {
                        setSelectedJobs((prev) => prev.filter((j) => j.platform !== platform));
                      } else {
                        setSelectedJobs((prev) => [
                          ...prev.filter((j) => j.platform !== platform),
                          ...accounts.map((name) => ({ platform, account_name: name })),
                        ]);
                      }
                    };
                    return (
                      <div key={platform} className="rounded-lg border border-white/10 overflow-hidden">
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
                          <button
                            type="button"
                            onClick={(e) => {
                              e.preventDefault();
                              toggleAll();
                            }}
                            className="ml-auto rounded-md border border-white/15 bg-white/5 px-2 py-1 text-xs text-gray-200 hover:bg-white/15 transition-colors"
                          >
                            {allChecked ? "Hammasini bekor qilish" : "Hammasini tanlash"}
                          </button>
                          <span className="text-xs text-gray-500">
                            {accounts.filter((name) =>
                              selectedJobs.some((j) => j.platform === platform && j.account_name === name)
                            ).length}/{accounts.length}
                          </span>
                        </label>
                        <div className="divide-y divide-white/10">
                          {accounts.map((name) => {
                            const checked = selectedJobs.some(
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

                {selectedJobs.length > 0 && (
                  <p className="text-xs text-gray-500">
                    Tanlangan:{" "}
                    {selectedJobs
                      .map((j) => `${PLATFORM_META[j.platform].label}/${j.account_name}`)
                      .join(", ")}
                  </p>
                )}

                {selectedJobs.some((j) => j.platform === "instagram") && (
                  <div className="rounded-lg border border-white/10 bg-black/40 px-3 py-2">
                    <div className="flex items-center justify-between mb-1">
                      <p className="text-[11px] uppercase tracking-wide text-gray-500">Instagram caption</p>
                      <button
                        type="button"
                        onClick={() => setCaption(buildInstagramCaption(clip))}
                        className="text-[11px] text-gray-500 hover:text-white transition-colors"
                      >
                        AI matnini tiklash
                      </button>
                    </div>
                    <textarea
                      value={caption}
                      onChange={(e) => setCaption(e.target.value)}
                      rows={6}
                      placeholder="Instagram uchun caption..."
                      className="w-full bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-gray-200 text-xs focus:outline-none focus:border-pink-500 resize-y"
                    />
                    <p className="mt-1 text-[10px] text-gray-600">
                      AI tomonidan yozilgan — yuklashdan oldin tahrirlashingiz mumkin.
                    </p>
                  </div>
                )}

                <div className="flex rounded-lg border border-white/10 overflow-hidden">
                  <button
                    onClick={() => setMode("now")}
                    className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                      mode === "now"
                        ? "bg-orange-500 text-white"
                        : "text-gray-400 hover:text-white hover:bg-white/5"
                    }`}
                  >
                    <Upload size={13} />
                    Hozir yuklash
                  </button>
                  <button
                    onClick={() => setMode("scheduled")}
                    className={`flex-1 flex items-center justify-center gap-1.5 py-2 text-xs font-medium transition-colors ${
                      mode === "scheduled"
                        ? "bg-blue-600 text-white"
                        : "text-gray-400 hover:text-white hover:bg-white/5"
                    }`}
                  >
                    <Calendar size={13} />
                    Vaqt belgilash
                  </button>
                </div>

                {mode === "scheduled" && (
                  <div>
                    <label className="block text-gray-400 text-xs mb-1.5">
                      Yuklash vaqti{" "}
                      <span className="text-gray-600">(Toshkent vaqti, UTC+5)</span>
                    </label>
                    <input
                      type="datetime-local"
                      value={scheduledFor}
                      onChange={(e) => setScheduledFor(e.target.value)}
                      className="w-full bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-blue-500"
                    />
                  </div>
                )}
              </>
            )}

            <div className="flex gap-2 pt-1">
              <button
                onClick={onClose}
                className="flex-1 py-2 rounded-lg border border-white/10 text-gray-400 text-sm hover:bg-white/5 transition-colors"
              >
                Bekor
              </button>
              {mode === "now" ? (
                <button
                  onClick={handleUploadNow}
                  disabled={selectedJobs.length === 0 || busy || !hasAnyAccount}
                  className="flex-1 py-2 rounded-lg bg-orange-500 text-white text-sm font-medium hover:bg-orange-400 transition-colors disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-2"
                >
                  {busy ? (
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
                  disabled={selectedJobs.length === 0 || busy || !hasAnyAccount}
                  className="flex-1 py-2 rounded-lg bg-blue-600 text-white text-sm font-medium hover:bg-blue-700 transition-colors disabled:opacity-50 disabled:cursor-not-allowed inline-flex items-center justify-center gap-2"
                >
                  {busy ? (
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
  );
}

// matchingAccountsForClip returns the IG account names whose filter
// rules match the given clip. A rule matches when at least one of the
// configured constraints (kind / genres / slugs) hits — empty rules
// match nothing here so users opt-in to the auto-select by configuring
// the filter. Accounts with NO filter at all are also returned (they
// accept everything by definition).
function matchingAccountsForClip(
  clip: Clip,
  accounts: InstagramAccountWithFilter[]
): string[] {
  const clipKind: "movie" | "series" =
    clip.content_kind === "series" || clip.series_id ? "series" : "movie";
  const clipSlug = clipKind === "series" ? (clip.series_slug || "") : (clip.movie_slug || "");
  const out: string[] = [];
  for (const a of accounts) {
    const f = a.filter;
    if (!f || (!f.kind && !f.genres?.length && !f.series_slugs?.length && !f.movie_slugs?.length)) {
      // No filter — generic account, always eligible.
      out.push(a.name);
      continue;
    }
    if (f.kind && f.kind !== clipKind) continue;
    if (clipKind === "series" && f.series_slugs?.length) {
      if (clipSlug && f.series_slugs.includes(clipSlug)) {
        out.push(a.name);
        continue;
      }
    }
    if (clipKind === "movie" && f.movie_slugs?.length) {
      if (clipSlug && f.movie_slugs.includes(clipSlug)) {
        out.push(a.name);
        continue;
      }
    }
    // Genre matching needs the group's genre list — the clip doc
    // itself doesn't carry genre, so we leave that to the upload-time
    // suggestion code which has the group context.
  }
  return out;
}

// ─── Filter bar ───────────────────────────────────────────────────────────────

interface ClipFilterBarProps {
  kind: ClipKind;
  onKindChange: (k: ClipKind) => void;
  query: string;
  onQueryChange: (q: string) => void;
  genres: string[];
  onGenresChange: (g: string[]) => void;
  allGenres: string[];
  account: string;
  onAccountChange: (a: string) => void;
  accounts: InstagramAccountWithFilter[];
  onlyUnposted: boolean;
  onOnlyUnpostedChange: (b: boolean) => void;
  sort: ClipSort;
  onSortChange: (s: ClipSort) => void;
  totalMovies: number;
  totalSeries: number;
  totalFiltered: number;
}

function ClipFilterBarBase(props: ClipFilterBarProps) {
  const {
    kind, onKindChange,
    query, onQueryChange,
    genres, onGenresChange, allGenres,
    account, onAccountChange, accounts,
    onlyUnposted, onOnlyUnpostedChange,
    sort, onSortChange,
    totalMovies, totalSeries, totalFiltered,
  } = props;

  const toggleGenre = (g: string) => {
    if (genres.includes(g)) onGenresChange(genres.filter((x) => x !== g));
    else onGenresChange([...genres, g]);
  };

  const tabs: Array<{ id: ClipKind; label: string; count?: number }> = [
    { id: "all", label: "Hammasi", count: totalFiltered },
    { id: "movie", label: "🎬 Kinolar", count: totalMovies },
    { id: "series", label: "📺 Seriallar", count: totalSeries },
  ];

  return (
    <div className="bg-[#12121a] border border-white/10 rounded-2xl p-3 sm:p-4 mb-4 space-y-3">
      {/* Tabs */}
      <div className="flex items-center gap-1 overflow-x-auto -mx-1 px-1">
        {tabs.map((t) => {
          const active = kind === t.id;
          return (
            <button
              key={t.id}
              onClick={() => onKindChange(t.id)}
              className={`px-3 py-1.5 rounded-lg text-sm font-medium whitespace-nowrap transition-colors ${
                active
                  ? "bg-white text-black"
                  : "text-gray-400 hover:text-white hover:bg-white/5"
              }`}
            >
              {t.label}
              {typeof t.count === "number" && (
                <span className={`ml-2 text-xs ${active ? "text-gray-600" : "text-gray-500"}`}>
                  {t.count}
                </span>
              )}
            </button>
          );
        })}
      </div>

      {/* Row 2: search + account + sort */}
      <div className="grid grid-cols-1 md:grid-cols-12 gap-2">
        <div className="md:col-span-6 relative">
          <input
            type="text"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="Kontent nomi bo'yicha qidirish..."
            className="w-full bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-sm text-white placeholder-gray-500 focus:outline-none focus:border-gray-500"
          />
          {query && (
            <button
              onClick={() => onQueryChange("")}
              className="absolute right-2 top-1/2 -translate-y-1/2 text-gray-500 hover:text-white"
              aria-label="Tozalash"
            >
              <X size={14} />
            </button>
          )}
        </div>

        <select
          value={account}
          onChange={(e) => onAccountChange(e.target.value)}
          className="md:col-span-3 bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-gray-500"
        >
          <option value="">Barcha akkauntlar</option>
          {accounts.map((a) => {
            const filterBits: string[] = [];
            if (a.filter?.kind) filterBits.push(a.filter.kind);
            if (a.filter?.genres?.length) filterBits.push(a.filter.genres.join("/"));
            if (a.filter?.series_slugs?.length) filterBits.push(`${a.filter.series_slugs.length} serial`);
            const label = filterBits.length > 0 ? `${a.name} · ${filterBits.join(" · ")}` : a.name;
            return (
              <option key={a.name} value={a.name}>{label}</option>
            );
          })}
        </select>

        <select
          value={sort}
          onChange={(e) => onSortChange(e.target.value as ClipSort)}
          className="md:col-span-3 bg-black/30 border border-white/10 rounded-lg px-3 py-2 text-sm text-white focus:outline-none focus:border-gray-500"
        >
          <option value="title">Tartib: Nom (A→Z)</option>
          <option value="newest">Tartib: Yangi yuklanganlar</option>
          <option value="most_clips">Tartib: Eng ko'p klip</option>
          <option value="least_posted">Tartib: IG'ga kam yuborilgan</option>
        </select>
      </div>

      {/* Row 3: only-unposted + genre chips */}
      <div className="flex flex-wrap items-center gap-2">
        <label className="inline-flex items-center gap-2 text-xs text-gray-400 cursor-pointer select-none">
          <input
            type="checkbox"
            checked={onlyUnposted}
            onChange={(e) => onOnlyUnpostedChange(e.target.checked)}
            className="accent-white"
          />
          Faqat IG'ga yuborilmaganlar
        </label>
        {allGenres.length > 0 && (
          <div className="flex-1 flex flex-wrap gap-1 items-center">
            <span className="text-xs text-gray-500 mr-1">Janrlar:</span>
            {allGenres.map((g) => {
              const active = genres.includes(g);
              return (
                <button
                  key={g}
                  onClick={() => toggleGenre(g)}
                  className={`px-2 py-0.5 rounded-full text-xs border transition-colors ${
                    active
                      ? "bg-white text-black border-white"
                      : "bg-transparent text-gray-400 border-white/10 hover:text-white hover:border-gray-500"
                  }`}
                >
                  {g}
                </button>
              );
            })}
            {genres.length > 0 && (
              <button
                onClick={() => onGenresChange([])}
                className="ml-1 text-xs text-gray-500 hover:text-white underline"
              >
                tozalash
              </button>
            )}
          </div>
        )}
      </div>
    </div>
  );
}

// Memoized: all callbacks passed in are stable setters, so the bar only
// re-renders when a filter value or a count actually changes.
const ClipFilterBar = React.memo(ClipFilterBarBase);

// ─── AI (Gemini) cost panel ───────────────────────────────────────────────────
// Shows total Gemini clip-generation spend plus a per-content breakdown
// (cost per movie/episode and a derived cost-per-clip). Reads the read-only
// /admin/clips/ai-usage endpoint backed by the clip_ai_usage collection.

interface AICostTotals {
  cost_usd: number;
  total_tokens: number;
  clip_count: number;
  analyses: number;
}
interface AICostItem {
  content_kind: string;
  content_id: string;
  title: string;
  model: string;
  cost_usd: number;
  cost_per_clip: number;
  total_tokens: number;
  clip_count: number;
  analyses: number;
  last_analyzed: string;
}

function fmtUSD(n: number): string {
  return "$" + (n || 0).toFixed(n < 1 ? 4 : 2);
}

function ClipAICostPanelBase({ token }: { token: string | null }) {
  const [totals, setTotals] = useState<AICostTotals | null>(null);
  const [items, setItems] = useState<AICostItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [open, setOpen] = useState(false);

  useEffect(() => {
    if (!token) return;
    let alive = true;
    (async () => {
      try {
        const res = await fetch(`${API}/admin/clips/ai-usage`, {
          headers: { Authorization: `Bearer ${token}` },
          cache: "no-store",
        });
        if (!res.ok) throw new Error("failed");
        const data = await res.json();
        if (!alive) return;
        setTotals(data.totals || null);
        setItems(Array.isArray(data.items) ? data.items : []);
      } catch {
        if (alive) setItems([]);
      } finally {
        if (alive) setLoading(false);
      }
    })();
    return () => {
      alive = false;
    };
  }, [token]);

  if (loading) {
    return (
      <div className="mb-6 flex items-center gap-2 text-gray-500 text-sm">
        <Loader2 size={14} className="animate-spin" /> AI xarajatlari yuklanmoqda...
      </div>
    );
  }
  if (!totals || totals.analyses === 0) return null;

  return (
    <div className="mb-6 bg-[#12121a] border border-white/10 rounded-2xl overflow-hidden">
      <div className="flex flex-wrap items-center gap-x-8 gap-y-3 p-4 sm:p-5">
        <div>
          <p className="text-xs text-gray-500">AI umumiy xarajat</p>
          <p className="text-2xl font-bold text-emerald-400">{fmtUSD(totals.cost_usd)}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Tahlillar</p>
          <p className="text-lg font-semibold text-white">{totals.analyses}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Kliplar</p>
          <p className="text-lg font-semibold text-white">{totals.clip_count}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500">Tokenlar</p>
          <p className="text-lg font-semibold text-white">{totals.total_tokens.toLocaleString()}</p>
        </div>
        <div>
          <p className="text-xs text-gray-500">O&apos;rtacha / klip</p>
          <p className="text-lg font-semibold text-white">
            {fmtUSD(totals.clip_count > 0 ? totals.cost_usd / totals.clip_count : 0)}
          </p>
        </div>
        <button
          onClick={() => setOpen((v) => !v)}
          className="ml-auto inline-flex items-center gap-1.5 px-3 py-1.5 rounded-lg border border-white/10 text-xs text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
        >
          {open ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
          Tafsilotlar ({items.length})
        </button>
      </div>

      {open && (
        <div className="border-t border-white/10 overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-left text-xs text-gray-500 border-b border-white/10">
                <th className="px-4 py-2 font-medium">Nomi</th>
                <th className="px-4 py-2 font-medium">Turi</th>
                <th className="px-4 py-2 font-medium text-right">Xarajat</th>
                <th className="px-4 py-2 font-medium text-right">Kliplar</th>
                <th className="px-4 py-2 font-medium text-right">Klip narxi</th>
                <th className="px-4 py-2 font-medium text-right">Tokenlar</th>
              </tr>
            </thead>
            <tbody>
              {items.map((it) => (
                <tr key={`${it.content_kind}:${it.content_id}`} className="border-b border-white/5 last:border-0">
                  <td className="px-4 py-2 text-white max-w-[260px] truncate" title={it.title}>
                    {it.title || "—"}
                  </td>
                  <td className="px-4 py-2 text-gray-400">
                    {it.content_kind === "series" ? "Serial" : "Kino"}
                  </td>
                  <td className="px-4 py-2 text-right text-emerald-400 font-medium">{fmtUSD(it.cost_usd)}</td>
                  <td className="px-4 py-2 text-right text-gray-300">{it.clip_count}</td>
                  <td className="px-4 py-2 text-right text-gray-400">{fmtUSD(it.cost_per_clip)}</td>
                  <td className="px-4 py-2 text-right text-gray-400">{it.total_tokens.toLocaleString()}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// Only depends on the token — never re-render it on page/filter changes.
const ClipAICostPanel = React.memo(ClipAICostPanelBase);

// ─── Main page ────────────────────────────────────────────────────────────────

function GroupHeader({
  kind,
  expanded,
  title,
  code,
  genres,
  clipCount,
  uploaded,
  lastUpload,
  scheduled,
  href,
}: {
  kind: "movie" | "series";
  expanded: boolean;
  title: string;
  code?: string;
  genres?: string[];
  clipCount: number;
  uploaded: number;
  lastUpload?: string;
  scheduled: number;
  href?: string;
}) {
  const pct = clipCount > 0 ? Math.min(100, Math.round((uploaded / clipCount) * 100)) : 0;
  const Icon = kind === "movie" ? Film : Tv;
  return (
    <div className="flex items-center gap-3">
      <span className="flex-shrink-0 text-gray-500">{expanded ? <ChevronDown size={16} /> : <ChevronRight size={16} />}</span>
      <span
        className={`flex h-10 w-10 flex-shrink-0 items-center justify-center rounded-xl ${
          kind === "movie" ? "bg-emerald-500/15 text-emerald-300" : "bg-amber-500/15 text-amber-300"
        }`}
      >
        <Icon size={18} />
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex flex-wrap items-center gap-2">
          <span className="truncate font-semibold text-white">{title}</span>
          {code && <span className="font-mono text-xs text-gray-500">#{code}</span>}
          {genres?.slice(0, 3).map((g) => (
            <span key={g} className="rounded-md bg-white/5 px-1.5 py-0.5 text-[10px] text-gray-400">
              {g}
            </span>
          ))}
        </div>
        <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-[11px]">
          <span className="text-gray-400">{clipCount} ta klip</span>
          {uploaded > 0 ? (
            <span className="inline-flex items-center gap-1 text-pink-300">
              <Instagram size={11} /> {uploaded}/{clipCount} joylangan
            </span>
          ) : (
            <span className="text-gray-600">Hali joylanmagan</span>
          )}
          {lastUpload && <span className="text-gray-600">oxirgisi {formatTashkent(lastUpload)}</span>}
          {scheduled > 0 && (
            <span className="inline-flex items-center gap-1 rounded-full bg-amber-500/10 px-2 py-0.5 text-amber-300">
              <CalendarClock size={11} /> {scheduled} rejalashtirilgan
            </span>
          )}
        </div>
      </div>
      <div className="hidden w-28 flex-shrink-0 sm:block" title={`${pct}% joylangan`}>
        <div className="mb-1 flex justify-between text-[10px] text-gray-500">
          <span>Instagram</span>
          <span className="tabular-nums">{pct}%</span>
        </div>
        <div className="h-1.5 overflow-hidden rounded-full bg-white/10">
          <div className="h-full rounded-full bg-gradient-to-r from-pink-500 to-amber-400" style={{ width: `${pct}%` }} />
        </div>
      </div>
      {href && (
        <a
          href={href}
          target="_blank"
          rel="noopener noreferrer"
          onClick={(e) => e.stopPropagation()}
          className="flex-shrink-0 rounded-lg p-2 text-gray-500 transition-colors hover:bg-white/5 hover:text-white"
          title="Saytda ko'rish"
          aria-label="Saytda ko'rish"
        >
          <ExternalLink size={15} />
        </a>
      )}
    </div>
  );
}

export default function AdminClipsPage() {
  const { token } = useAuth();
  const groupsPageLimit = 10;
  const jobsPageLimit = 10;

  const [groups, setGroups] = useState<ServerGroups | null>(null);
  const [groupsLoading, setGroupsLoading] = useState(true);
  const [groupsPage, setGroupsPage] = useState(() => readUrlNumber("page", 1));

  // Filter state — controls what GET /api/admin/clips/groups returns.
  // Each change re-runs the server query (debounced for the search box).
  // Initial values come from the URL so refresh/back/shared links keep the
  // same view; useSyncUrlParams below writes changes back.
  const [filterKind, setFilterKind] = useState<ClipKind>(() => {
    const v = readUrlParam("kind", "all");
    return v === "movie" || v === "series" ? v : "all";
  });
  const [filterQuery, setFilterQuery] = useState(() => readUrlParam("q", ""));
  const [debouncedQuery, setDebouncedQuery] = useState(() => readUrlParam("q", "").trim());
  const [filterGenres, setFilterGenres] = useState<string[]>(() => readUrlList("genres"));
  const [filterAccount, setFilterAccount] = useState<string>(() => readUrlParam("account", "")); // "" ⇒ all accounts
  const [filterOnlyUnposted, setFilterOnlyUnposted] = useState(() => readUrlBool("unposted"));
  const [filterSort, setFilterSort] = useState<ClipSort>(() => {
    const v = readUrlParam("sort", "title");
    return (["title", "newest", "most_clips", "least_posted"] as const).includes(v as ClipSort) ? (v as ClipSort) : "title";
  });
  const toast = useToast();

  useSyncUrlParams(
    {
      kind: filterKind,
      q: debouncedQuery,
      genres: filterGenres,
      account: filterAccount,
      unposted: filterOnlyUnposted,
      sort: filterSort,
      page: groupsPage,
    },
    { kind: "all", q: "", genres: [], account: "", unposted: false, sort: "title", page: 1 }
  );
  const [allGenres, setAllGenres] = useState<string[]>([]);
  const [igAccountsMeta, setIgAccountsMeta] = useState<InstagramAccountWithFilter[]>([]);

  // Debounce the search input so we don't spam the backend on every
  // keystroke. 300ms is enough to feel instant without firing per key.
  useEffect(() => {
    const next = filterQuery.trim();
    // Skip when nothing changed — otherwise the mount run would reset a
    // ?page= restored from the URL.
    if (next === debouncedQuery) return;
    const t = setTimeout(() => {
      setDebouncedQuery(next);
      setGroupsPage(1);
    }, 300);
    return () => clearTimeout(t);
  }, [filterQuery, debouncedQuery]);

  // Filter setters reset to page 1 in the same batched update. Doing the
  // reset in a separate effect caused a second render + a wasted request
  // (first with the old page, then again with page 1) on every change.
  const handleKindChange = useCallback((v: ClipKind) => { setFilterKind(v); setGroupsPage(1); }, []);
  const handleGenresChange = useCallback((v: string[]) => { setFilterGenres(v); setGroupsPage(1); }, []);
  const handleAccountChange = useCallback((v: string) => { setFilterAccount(v); setGroupsPage(1); }, []);
  const handleOnlyUnpostedChange = useCallback((v: boolean) => { setFilterOnlyUnposted(v); setGroupsPage(1); }, []);
  const handleSortChange = useCallback((v: ClipSort) => { setFilterSort(v); setGroupsPage(1); }, []);

  const [allAccounts, setAllAccounts] = useState<AllAccounts>({ instagram: [], youtube: [], tiktok: [] });
  const [publishJobs, setPublishJobs] = useState<PublishJob[]>([]);
  const [allPendingJobs, setAllPendingJobs] = useState<PublishJob[]>([]);
  const [jobsPage, setJobsPage] = useState(1);
  const [jobsTotal, setJobsTotal] = useState(0);

  const [uploading, setUploading] = useState<Record<string, boolean>>({});
  const [downloading, setDownloading] = useState<Record<string, boolean>>({});
  // Only WHICH clip is being published + its default account selection lives in
  // the page. All editable modal state (caption, toggles, schedule) lives inside
  // <PublishModal/> so editing it never re-renders this page.
  const [modalTarget, setModalTarget] = useState<{ clip: Clip; initialJobs: SelectedJob[] } | null>(null);
  const [editingJob, setEditingJob] = useState<{ id: string; value: string } | null>(null);
  const [cancellingId, setCancellingId] = useState<string | null>(null);

  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set());
  const [expandedEpisodes, setExpandedEpisodes] = useState<Set<string>>(new Set());

  // Lazy clip cache. Keyed by scope ("movie:<id>" or "episode:<id>").
  const [scopeClips, setScopeClips] = useState<Record<string, ScopeClipPage>>({});

  // ── Data fetching ───────────────────────────────────────────────────

  const fetchGroups = useCallback(async (signal?: AbortSignal) => {
    if (!token) return;
    setGroupsLoading(true);
    try {
      const params = new URLSearchParams();
      if (filterKind !== "all") params.set("kind", filterKind);
      if (debouncedQuery) params.set("q", debouncedQuery);
      if (filterGenres.length > 0) params.set("genres", filterGenres.join(","));
      if (filterAccount) params.set("account", filterAccount);
      if (filterOnlyUnposted) params.set("only_unposted", "true");
      if (filterSort && filterSort !== "title") params.set("sort", filterSort);
      params.set("limit", String(groupsPageLimit));
      params.set("offset", String((groupsPage - 1) * groupsPageLimit));

      const res = await fetch(`${API}/admin/clips/groups?${params.toString()}`, {
        headers: { Authorization: `Bearer ${token}` },
        signal,
      });
      if (!res.ok) throw new Error(`status ${res.status}`);
      const data: ServerGroups = await res.json();
      if (signal?.aborted) return;
      setGroups({
        movies: data.movies || [],
        series: data.series || [],
        total_clips: data.total_clips || 0,
        total_contents: data.total_contents || 0,
        total_filtered: data.total_filtered,
        total_movies: data.total_movies,
        total_series: data.total_series,
        movie_group_count: data.movie_group_count,
        series_group_count: data.series_group_count,
      });
      if (data.all_genres && data.all_genres.length > 0) {
        const fromServer = data.all_genres;
        setAllGenres((prev) => (prev.length === 0 ? fromServer : prev));
      }
    } catch (err) {
      // A superseded request is aborted on purpose — not an error.
      if (!signal?.aborted) toast.error(`Kliplar ro'yxatini yuklab bo'lmadi${err instanceof Error ? `: ${err.message}` : ""}`);
    } finally {
      if (!signal?.aborted) setGroupsLoading(false);
    }
  }, [token, filterKind, debouncedQuery, filterGenres, filterAccount, filterOnlyUnposted, filterSort, groupsPage, groupsPageLimit, toast]);

  // Load the full genre list once — separate endpoint so the chip
  // selector shows every option even when the current filter excludes
  // most groups.
  useEffect(() => {
    if (!token) return;
    fetch(`${API}/admin/clips/genres`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (data?.genres) setAllGenres(data.genres);
      })
      .catch(() => {});
  }, [token]);

  // Load IG account metadata (name + filter rules) so the account
  // dropdown can label them and the upload modal can auto-select.
  useEffect(() => {
    if (!token) return;
    fetch(`${API}/admin/instagram/accounts`, { headers: { Authorization: `Bearer ${token}` } })
      .then((r) => (r.ok ? r.json() : null))
      .then((data) => {
        if (Array.isArray(data?.items)) setIgAccountsMeta(data.items);
      })
      .catch(() => {});
  }, [token]);

  const fetchScopedClips = useCallback(
    async (scope: Scope, offset = 0) => {
      if (!token) return;
      const key = scopeKey(scope);
      setScopeClips((prev) => ({
        ...prev,
        [key]: {
          clips: prev[key]?.clips ?? [],
          total: prev[key]?.total ?? 0,
          offset,
          loading: true,
          error: undefined,
        },
      }));
      const params = new URLSearchParams();
      params.set("limit", String(CLIPS_PAGE_LIMIT));
      params.set("offset", String(offset));
      if (scope.kind === "movie") {
        if (scope.movieId) params.set("movie_id", scope.movieId);
        if (scope.movieIds?.length) params.set("movie_ids", scope.movieIds.join(","));
        if (scope.movieCodes?.length) params.set("movie_code", scope.movieCodes.join(","));
        if (scope.movieSlugs?.length) params.set("movie_slug", scope.movieSlugs.join(","));
        if (scope.movieTitles?.length) params.set("movie_titles", scope.movieTitles.join(","));
      } else {
        if (scope.episodeId) params.set("episode_id", scope.episodeId);
        if (scope.episodeIds?.length) params.set("episode_ids", scope.episodeIds.join(","));
      }
      try {
        const res = await fetch(`${API}/admin/clips?${params.toString()}`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!res.ok) throw new Error(`status ${res.status}`);
        const data = await res.json();
        setScopeClips((prev) => ({
          ...prev,
          [key]: {
            clips: data.data || [],
            total: data.total || 0,
            offset,
            loading: false,
          },
        }));
      } catch (err) {
        setScopeClips((prev) => ({
          ...prev,
          [key]: {
            clips: [],
            total: prev[key]?.total ?? 0,
            offset,
            loading: false,
            error: err instanceof Error ? err.message : "fetch failed",
          },
        }));
      }
    },
    [token]
  );

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
      } else {
        toast.error("Publish akkauntlarini yuklab bo'lmadi");
      }
    } catch {
      toast.error("Publish akkauntlarini yuklab bo'lmadi");
    }
  }, [token, toast]);

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
      } else {
        toast.error("Rejalashtirilgan yuklamalarni yuklab bo'lmadi");
      }
    } catch {
      toast.error("Rejalashtirilgan yuklamalarni yuklab bo'lmadi");
    }
  }, [token, jobsPage, toast]);

  const fetchAllPendingJobs = useCallback(async () => {
    if (!token) return;
    try {
      const pageSize = 500;
      let offset = 0;
      let total = Infinity;
      const jobs: PublishJob[] = [];

      while (jobs.length < total) {
        const params = new URLSearchParams({
          status: "pending,scheduled,processing",
          limit: String(pageSize),
          offset: String(offset),
        });
        const res = await fetch(`${API}/admin/publish/jobs?${params.toString()}`, {
          headers: { Authorization: `Bearer ${token}` },
        });
        if (!res.ok) break;

        const data = await res.json();
        const items: PublishJob[] = data.items || data.data || [];
        jobs.push(...items.filter(isActivePublishJob));
        total = typeof data.total === "number" ? data.total : jobs.length;

        if (items.length < pageSize) break;
        offset += pageSize;
      }

      setAllPendingJobs(jobs);
    } catch {
      // Badges only — the jobs list above reports its own failure.
    }
  }, [token]);

  // Separate effects: previously one effect ran ALL four fetches whenever any
  // of them changed, so every page/filter change also re-downloaded accounts,
  // the jobs page and the full pending-jobs list (paged loop) — the main
  // cause of the page freezing.
  useEffect(() => {
    const ctrl = new AbortController();
    fetchGroups(ctrl.signal);
    return () => ctrl.abort();
  }, [fetchGroups]);

  useEffect(() => {
    fetchAccounts();
  }, [fetchAccounts]);

  useEffect(() => {
    fetchJobs();
  }, [fetchJobs]);

  useEffect(() => {
    fetchAllPendingJobs();
  }, [fetchAllPendingJobs]);

  const handleScopePageChange = useCallback(
    (scope: Scope, offset: number) => {
      fetchScopedClips(scope, offset);
    },
    [fetchScopedClips]
  );

  // ── Group / episode expand/collapse + lazy load ─────────────────────

  const ensureMovieClips = useCallback(
    (movie: ServerMovieGroup) => {
      const key = `movie:${movie.group_key}`;
      if (!scopeClips[key]) {
        fetchScopedClips({
          kind: "movie",
          groupKey: movie.group_key,
          movieId: movie.movie_id || undefined,
          movieIds: movie.match_movie_ids,
          movieCodes: movie.match_movie_codes,
          movieSlugs: movie.match_movie_slugs,
          movieTitles: movie.match_movie_titles,
        }, 0);
      }
    },
    [scopeClips, fetchScopedClips]
  );

  const ensureEpisodeClips = useCallback(
    (episode: ServerEpisodeGroup) => {
      const key = `episode:${episode.group_key}`;
      if (!scopeClips[key]) {
        fetchScopedClips({
          kind: "episode",
          groupKey: episode.group_key,
          episodeId: episode.episode_id || undefined,
          episodeIds: episode.match_episode_ids,
        }, 0);
      }
    },
    [scopeClips, fetchScopedClips]
  );

  const toggleMovieGroup = (movie: ServerMovieGroup) => {
    const key = `movie:${movie.group_key}`;
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      if (next.has(key)) {
        next.delete(key);
      } else {
        next.add(key);
        ensureMovieClips(movie);
      }
      return next;
    });
  };

  const toggleSeriesGroup = (series: ServerSeriesGroup) => {
    const key = `series:${series.group_key}`;
    setExpandedGroups((prev) => {
      const next = new Set(prev);
      next.has(key) ? next.delete(key) : next.add(key);
      return next;
    });
  };

  const toggleEpisode = (episode: ServerEpisodeGroup) => {
    setExpandedEpisodes((prev) => {
      const next = new Set(prev);
      if (next.has(episode.group_key)) {
        next.delete(episode.group_key);
      } else {
        next.add(episode.group_key);
        ensureEpisodeClips(episode);
      }
      return next;
    });
  };

  const expandAllSeriesEpisodes = (s: ServerSeriesGroup) => {
    setExpandedEpisodes((prev) => {
      const next = new Set(prev);
      s.seasons.forEach((season) =>
        season.episodes.forEach((ep) => {
          next.add(ep.group_key);
          ensureEpisodeClips(ep);
        })
      );
      return next;
    });
  };

  const collapseAllSeriesEpisodes = (s: ServerSeriesGroup) => {
    setExpandedEpisodes((prev) => {
      const next = new Set(prev);
      s.seasons.forEach((season) => season.episodes.forEach((ep) => next.delete(ep.group_key)));
      return next;
    });
  };

  // Top-level expand/collapse for content groups (movies + series headers).
  const allGroupKeys = useMemo(() => {
    if (!groups) return [] as string[];
    return [
      ...groups.movies.map((m) => `movie:${m.group_key}`),
      ...groups.series.map((s) => `series:${s.group_key}`),
    ];
  }, [groups]);

  const expandAll = () => {
    if (!groups) return;
    setExpandedGroups(new Set(allGroupKeys));
    groups.movies.forEach((m) => ensureMovieClips(m));
  };
  const collapseAll = () => setExpandedGroups(new Set());

  // ── Publish modal handlers (unchanged behavior) ─────────────────────

  const openModal = useCallback((clip: Clip) => {
    const defaultJobs: SelectedJob[] = [];
    // Auto-select every IG account whose filter rules match this clip.
    // When no account matches (or no account has a filter at all) we
    // fall back to the first one so the modal is never blank.
    const matched = matchingAccountsForClip(clip, igAccountsMeta);
    const igDefaults = matched.length > 0
      ? matched
      : (allAccounts.instagram[0] ? [allAccounts.instagram[0]] : []);
    igDefaults.forEach((name) => {
      defaultJobs.push({ platform: "instagram", account_name: name });
    });
    setModalTarget({ clip, initialJobs: defaultJobs });
  }, [igAccountsMeta, allAccounts]);

  const hasAnyAccount =
    allAccounts.instagram.length > 0 || allAccounts.youtube.length > 0 || allAccounts.tiktok.length > 0;

  const refreshAfterPublish = (clip: Clip) => {
    // Refetch the scope this clip belongs to so the upload counters update.
    if (clip.episode_id) {
      fetchScopedClips(
        { kind: "episode", groupKey: clip.episode_id, episodeId: clip.episode_id, episodeIds: [clip.episode_id] },
        scopeClips[`episode:${clip.episode_id}`]?.offset ?? 0
      );
    } else if (clip.movie_id) {
      fetchScopedClips(
        { kind: "movie", groupKey: clip.movie_id, movieId: clip.movie_id, movieIds: [clip.movie_id] },
        scopeClips[`movie:${clip.movie_id}`]?.offset ?? 0
      );
    }
  };

  // Called by <PublishModal/> after a successful publish/schedule so the page
  // refreshes the affected clip scope counters and the jobs lists.
  const handlePublished = useCallback(async (clip: Clip) => {
    refreshAfterPublish(clip);
    await fetchJobs();
    await fetchAllPendingJobs();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fetchJobs, fetchAllPendingJobs, scopeClips]);

  const handleCancelJob = async (id: string) => {
    if (!token) return;
    setCancellingId(id);
    try {
      const res = await fetch(`${API}/admin/publish/jobs/${id}`, {
        method: "DELETE",
        headers: { Authorization: `Bearer ${token}` },
      });
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `status ${res.status}`);
      }
      toast.success("Yuklama bekor qilindi");
      await fetchJobs();
      await fetchAllPendingJobs();
    } catch (err) {
      toast.error(`Bekor qilib bo'lmadi${err instanceof Error ? `: ${err.message}` : ""}`);
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
      if (!res.ok) {
        const data = await res.json().catch(() => ({}));
        throw new Error(data.error || `status ${res.status}`);
      }
      setEditingJob(null);
      toast.success("Vaqt yangilandi");
      await fetchJobs();
      await fetchAllPendingJobs();
    } catch (err) {
      toast.error(`Vaqtni saqlab bo'lmadi${err instanceof Error ? `: ${err.message}` : ""}`);
    }
  };

  const handleDownloadClip = useCallback((clip: Clip) => {
    if (!token) return;
    setDownloading((prev) => ({ ...prev, [clip.id]: true }));

    fetch(`${API}/admin/clips/${clip.id}/download`, {
      headers: { Authorization: `Bearer ${token}` },
    })
      .then(async (res) => {
        if (!res.ok) throw new Error(`download failed: ${res.status}`);
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
      .catch((err) => {
        toast.error(`Klipni yuklab olib bo'lmadi${err instanceof Error ? `: ${err.message}` : ""}`);
      })
      .finally(() => {
        setDownloading((prev) => ({ ...prev, [clip.id]: false }));
      });
  }, [token, toast]);

  // ── Derived: paged content group list ───────────────────────────────

  type FlatGroup =
    | { kind: "movie"; group: ServerMovieGroup }
    | { kind: "series"; group: ServerSeriesGroup };
  const flatGroups: FlatGroup[] = useMemo(() => {
    if (!groups) return [];
    return [
      ...groups.movies.map((g) => ({ kind: "movie" as const, group: g })),
      ...groups.series.map((g) => ({ kind: "series" as const, group: g })),
    ];
  }, [groups]);

  // Server-side pagination — backend already sliced the result to
  // [offset, offset+limit). flatGroups IS the current page.
  const totalFiltered = groups?.total_filtered ?? flatGroups.length;
  const groupsTotalPages = Math.max(1, Math.ceil(totalFiltered / groupsPageLimit));
  const pagedGroups = flatGroups;

  // Clamp only once real totals are known — before the first response the
  // total is 0 and this would wipe a ?page= restored from the URL.
  useEffect(() => {
    if (groups && !groupsLoading && groupsPage > groupsTotalPages) setGroupsPage(groupsTotalPages);
  }, [groups, groupsLoading, groupsPage, groupsTotalPages]);

  const pagedPendingJobs = publishJobs.filter(isActivePublishJob);
  const doneJobs = publishJobs.filter((j) => j.status === "success" || j.status === "failed");
  
  const allRelevantJobsForBadges = useMemo(() => {
    const map = new Map<string, PublishJob>();
    // publishJobs are the current page, good for detail but might miss some
    publishJobs.forEach(j => map.set(j.id, j));
    // allPendingJobs ensure all scheduled ones are caught even if not on page 1
    allPendingJobs.forEach(j => map.set(j.id, j));
    return Array.from(map.values());
  }, [publishJobs, allPendingJobs]);

  // clip_id → jobs, so each clip row is an O(1) lookup instead of scanning
  // every job for every row.
  const jobsByClip = useMemo(() => {
    const map = new Map<string, PublishJob[]>();
    allRelevantJobsForBadges.forEach((j) => {
      const list = map.get(j.clip_id);
      if (list) list.push(j);
      else map.set(j.clip_id, [j]);
    });
    return map;
  }, [allRelevantJobsForBadges]);

  // Stable Scope objects per group so memoized <ClipTable/>s keep their props.
  const groupScopes = useMemo(() => {
    const map = new Map<string, Scope>();
    if (!groups) return map;
    groups.movies.forEach((m) => map.set(`movie:${m.group_key}`, movieGroupScope(m)));
    groups.series.forEach((s) =>
      s.seasons.forEach((season) =>
        season.episodes.forEach((ep) => map.set(`episode:${ep.group_key}`, episodeGroupScope(ep)))
      )
    );
    return map;
  }, [groups]);

  const jobsTotalPages = Math.max(1, Math.ceil(jobsTotal / jobsPageLimit));

  // ── Render ───────────────────────────────────────────────────────────

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <PageHead
        icon={Scissors}
        gradient="from-pink-500 to-amber-500"
        title="Kliplar"
        subtitle="AI yaratgan qisqa kliplar — Instagram, YouTube va TikTok'ga joylash"
        actions={
          flatGroups.length > 0 ? (
            <>
              <GhostButton onClick={expandAll}>
                <ChevronsUpDown size={15} /> Ochish
              </GhostButton>
              <GhostButton onClick={collapseAll}>
                <ChevronsDownUp size={15} /> Yopish
              </GhostButton>
            </>
          ) : undefined
        }
      />

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Scissors} tone="violet" label="Jami kliplar" value={groups?.total_clips ?? "—"} hint={`${groups?.total_contents ?? 0} ta kontentda`} />
        <StatTile icon={Film} tone="green" label="Kinolar" value={groups?.total_movies ?? "—"} active={filterKind === "movie"} onClick={() => handleKindChange(filterKind === "movie" ? "all" : "movie")} />
        <StatTile icon={Tv} tone="yellow" label="Seriallar" value={groups?.total_series ?? "—"} active={filterKind === "series"} onClick={() => handleKindChange(filterKind === "series" ? "all" : "series")} />
        <StatTile icon={CalendarClock} tone="sky" label="Rejalashtirilgan" value={allPendingJobs.length} hint="navbatdagi joylashlar" />
      </div>

      {/* ── AI (Gemini) clip-generation spend summary */}
      <ClipAICostPanel token={token} />

      {/* ── Filter bar: tabs + search + genre + account + sort + only-unposted */}
      <ClipFilterBar
        kind={filterKind}
        onKindChange={handleKindChange}
        query={filterQuery}
        onQueryChange={setFilterQuery}
        genres={filterGenres}
        onGenresChange={handleGenresChange}
        allGenres={allGenres}
        account={filterAccount}
        onAccountChange={handleAccountChange}
        accounts={igAccountsMeta}
        onlyUnposted={filterOnlyUnposted}
        onOnlyUnpostedChange={handleOnlyUnpostedChange}
        sort={filterSort}
        onSortChange={handleSortChange}
        totalMovies={groups?.total_movies ?? 0}
        totalSeries={groups?.total_series ?? 0}
        totalFiltered={totalFiltered}
      />

      {groupsLoading && !groups ? (
        <SkeletonList rows={5} height={76} />
      ) : flatGroups.length === 0 ? (
        <EmptyState
          icon={Scissors}
          title={(groups?.total_clips ?? 0) > 0 ? "Filtrga mos klip yo'q" : "Hali klip yaratilmagan"}
          text={(groups?.total_clips ?? 0) > 0 ? "Filtrlarni o'zgartirib ko'ring." : "Kino yoki serial qismi tayyor bo'lganda klip avtomatik yaratiladi."}
        />
      ) : (
        <div
          className={`space-y-3 transition-opacity ${groupsLoading ? "opacity-50 pointer-events-none" : ""}`}
          aria-busy={groupsLoading}
        >
          <div className="flex justify-end items-center gap-3">
            {groupsLoading && <Loader2 size={16} className="animate-spin text-gray-500" />}
            <PaginationControls
              page={groupsPage}
              totalPages={groupsTotalPages}
              onPrev={() => setGroupsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setGroupsPage((prev) => Math.min(groupsTotalPages, prev + 1))}
            />
          </div>

          {pagedGroups.map((entry) => {
            if (entry.kind === "movie") {
              const m = entry.group;
              const key = `movie:${m.group_key}`;
              const isExpanded = expandedGroups.has(key);
              const page = scopeClips[key];
              const scheduledCount = allPendingJobs.filter(
                (j: PublishJob) => (m.code && j.movie_code === m.code) || (m.slug && j.movie_slug === m.slug)
              ).length;
              return (
                <div
                  key={key}
                  className="bg-[#12121a] border border-white/10 rounded-2xl overflow-hidden transition hover:border-white/20"
                >
                  <button
                    onClick={() => toggleMovieGroup(m)}
                    className="w-full px-4 sm:px-5 py-3.5 border-b border-white/5 hover:bg-white/[0.03] transition-colors text-left"
                  >
                    <GroupHeader
                      kind="movie"
                      expanded={isExpanded}
                      title={m.title || "Nomsiz kino"}
                      code={m.code}
                      genres={m.genre}
                      clipCount={m.clip_count}
                      uploaded={m.ig_uploaded_count}
                      lastUpload={m.last_ig_upload_at}
                      scheduled={scheduledCount}
                      href={m.slug ? `/movies/${m.slug}` : undefined}
                    />
                  </button>

                  {isExpanded && (
                    <ClipTable
                      page={page ?? { clips: [], total: m.clip_count, offset: 0, loading: true }}
                      scope={groupScopes.get(key) ?? movieGroupScope(m)}
                      jobsByClip={jobsByClip}
                      downloading={downloading}
                      uploading={uploading}
                      token={token}
                      onDownload={handleDownloadClip}
                      onPublish={openModal}
                      onPageChange={handleScopePageChange}
                    />
                  )}
                </div>
              );
            }

            const s = entry.group;
            const key = `series:${s.group_key}`;
            const isExpanded = expandedGroups.has(key);
            const scheduledCount = allPendingJobs.filter(
              (j: PublishJob) => (s.slug && j.movie_slug === s.slug) || (j.movie_title === s.title)
            ).length;
            return (
              <div
                key={key}
                className="bg-[#12121a] border border-white/10 rounded-2xl overflow-hidden transition hover:border-white/20"
              >
                <button
                  onClick={() => toggleSeriesGroup(s)}
                  className="w-full px-4 sm:px-5 py-3.5 border-b border-white/5 hover:bg-white/[0.03] transition-colors text-left"
                >
                  <GroupHeader
                    kind="series"
                    expanded={isExpanded}
                    title={s.title || "Nomsiz serial"}
                    genres={s.genre}
                    clipCount={s.clip_count}
                    uploaded={s.ig_uploaded_count}
                    lastUpload={s.last_ig_upload_at}
                    scheduled={scheduledCount}
                    href={s.slug ? `/series/${s.slug}` : undefined}
                  />
                </button>

                {isExpanded && (
                  <div className="divide-y divide-white/5">
                    <div className="px-4 sm:px-6 py-2 flex items-center justify-end gap-2 bg-white/[0.02]">
                      <button
                        onClick={() => expandAllSeriesEpisodes(s)}
                        className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg border border-white/10 text-[11px] text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
                      >
                        <ChevronsUpDown size={12} />
                        Barchasini ochish
                      </button>
                      <button
                        onClick={() => collapseAllSeriesEpisodes(s)}
                        className="inline-flex items-center gap-1.5 px-2.5 py-1 rounded-lg border border-white/10 text-[11px] text-gray-400 hover:text-white hover:border-gray-500 transition-colors"
                      >
                        <ChevronsDownUp size={12} />
                        Barchasini yopish
                      </button>
                    </div>
                    {s.seasons.map((season) => (
                      <div key={`${key}:season:${season.season_number}`}>
                        <div className="px-4 sm:px-6 py-3 bg-white/[0.03] border-b border-white/5">
                          <p className="text-sm font-medium text-gray-200">
                            Season {season.season_number} · {season.clip_count} ta klip
                          </p>
                        </div>
                        <div className="divide-y divide-white/5">
                          {season.episodes.map((ep) => {
                            const epExpanded = expandedEpisodes.has(ep.group_key);
                            const epLabel = `S${padEpisodeNumber(season.season_number)}E${padEpisodeNumber(ep.episode_number)}`;
                            const scope = `episode:${ep.group_key}`;
                            const page = scopeClips[scope];
                            return (
                              <div key={ep.group_key}>
                                <button
                                  onClick={() => toggleEpisode(ep)}
                                  className="w-full px-4 sm:px-6 py-3 bg-white/[0.02] hover:bg-white/[0.05] transition-colors text-left"
                                >
                                  <div className="flex items-center gap-3">
                                    <span className="text-gray-500 flex-shrink-0">
                                      {epExpanded ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
                                    </span>
                                    <div className="flex-1 min-w-0">
                                      <div className="flex items-center gap-2 flex-wrap">
                                        <span className="text-sm text-white truncate">
                                          {ep.title || epLabel}
                                        </span>
                                        <span className="text-[11px] font-mono text-gray-500">{epLabel}</span>
                                        <span className="text-[11px] text-gray-600">
                                          {ep.clip_count} ta klip
                                        </span>
                                      </div>
                                      {ep.ig_uploaded_count > 0 && (
                                        <div className="flex items-center gap-3 mt-1 flex-wrap">
                                          <span className="inline-flex items-center gap-1 text-[11px] text-pink-400">
                                            <Instagram size={10} />
                                            {ep.ig_uploaded_count}/{ep.clip_count}
                                          </span>
                                          {ep.last_ig_upload_at && (
                                            <span className="text-[11px] text-gray-600">
                                              · {formatTashkent(ep.last_ig_upload_at)}
                                            </span>
                                          )}
                                        </div>
                                      )}
                                    </div>
                                  </div>
                                </button>
                                {epExpanded && (
                                  <ClipTable
                                    page={page ?? { clips: [], total: ep.clip_count, offset: 0, loading: true }}
                                    scope={groupScopes.get(scope) ?? episodeGroupScope(ep)}
                                    jobsByClip={jobsByClip}
                                    downloading={downloading}
                                    uploading={uploading}
                                    token={token}
                                    onDownload={handleDownloadClip}
                                    onPublish={openModal}
                                    onPageChange={handleScopePageChange}
                                  />
                                )}
                              </div>
                            );
                          })}
                        </div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            );
          })}

          <div className="flex justify-end">
            <PaginationControls
              page={groupsPage}
              totalPages={groupsTotalPages}
              onPrev={() => setGroupsPage((prev) => Math.max(1, prev - 1))}
              onNext={() => setGroupsPage((prev) => Math.min(groupsTotalPages, prev + 1))}
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

          {pagedPendingJobs.length > 0 && (
            <div className="bg-[#12121a] border border-white/10 rounded-2xl overflow-hidden mb-4">
              <div className="px-4 py-3 border-b border-white/10 bg-black/50">
                <span className="text-sm text-gray-400">Kutilayotgan</span>
              </div>
              <div className="divide-y divide-white/5">
                {pagedPendingJobs.map((j) => (
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
                                className="bg-black/30 border border-white/10 rounded px-2 py-1 text-xs text-white"
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

          {doneJobs.length > 0 && (
            <div className="bg-[#12121a] border border-white/10 rounded-2xl overflow-hidden transition hover:border-white/20">
              <div className="px-4 py-3 border-b border-white/10 bg-black/50">
                <span className="text-sm text-gray-400">Bajarilgan</span>
              </div>
              <div className="divide-y divide-white/5">
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
                      {j.error && j.status === "failed" && (
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
      {modalTarget && (
        <PublishModal
          key={modalTarget.clip.id}
          clip={modalTarget.clip}
          initialSelectedJobs={modalTarget.initialJobs}
          allAccounts={allAccounts}
          hasAnyAccount={hasAnyAccount}
          token={token}
          onClose={() => setModalTarget(null)}
          onPublished={handlePublished}
          onBusyChange={(b) => setUploading((prev) => ({ ...prev, [modalTarget.clip.id]: b }))}
        />
      )}
    </div>
  );
}
