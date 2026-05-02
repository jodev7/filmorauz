"use client";

import { useState, useEffect, useCallback, useRef, useMemo } from "react";
import { useAuth } from "@/lib/auth-context";
import { 
  Search, Play, RefreshCw, CheckCircle, XCircle,
  Clock, Download, Upload, Settings, AlertTriangle, Loader2,
  ChevronLeft, ChevronRight, Film, Tv, Link, Plus, Youtube, RotateCcw,
  Power, ChevronDown, ChevronRight as ChevronRightIcon
} from "lucide-react";
import {
  searchSource, createIngestionJob, getIngestionJobs,
  retryIngestionJob, IngestionJob, SearchResult, IngestionStatus,
  listCatalog, listCatalogCategories, CatalogItem, CatalogResponse, CatalogCategory,
  createManualImport, importFromCatalog
} from "@/lib/api";
import MediaImage from "@/components/ui/MediaImage";

// Season type for serial grouping
type SeasonGroup = {
  season_number: number;
  episodes: IngestionJob[];
};

// Grouped job type for serial display
type JobGroup = 
  | { type: "single"; job: IngestionJob }
  | { type: "serial"; id: string; title: string; seasons: SeasonGroup[]; expanded: boolean; totalEpisodes: number; totalSeasons: number };

function getSearchResultContentType(result: SearchResult): "movie" | "serial" | "" {
  const rawType = (((result as any).type || (result as any).content_type || "") + "").toLowerCase();
  if (rawType === "serial" || rawType === "series") return "serial";
  if (rawType === "movie") return "movie";
  if ((result.source || "").toLowerCase() === "asilmedia") return "movie";
  return "";
}

const JOB_STATUS_ORDER: Record<string, number> = {
  downloading: 0,
  processing: 1,
  hls_processing: 1,
  cutting_video: 1,
  removing_watermark: 1,
  adding_logo: 1,
  queued: 2,
  pending: 2,
  downloaded: 2,
  ready_to_process: 3,
  uploading: 4,
  finalizing_storage: 4,
  uploading_poster: 4,
  completed: 5,
  failed: 6,
  download_failed: 6,
};

function getCreatedTimeMs(job: IngestionJob): number {
  const created = new Date(job.created_at || "").getTime();
  return Number.isFinite(created) ? created : 0;
}

function getStableJobKey(job: IngestionJob): string {
  const legacyId = (job as unknown as { _id?: string })._id;
  return job.id || legacyId || "";
}

function getStableJobSortStatus(job: IngestionJob): string {
  const displayStatus = getJobDisplayStatus(job);
  if (displayStatus in JOB_STATUS_ORDER) return displayStatus;
  if (job.status in JOB_STATUS_ORDER) return job.status;
  return "failed";
}

function compareJobsStable(a: IngestionJob, b: IngestionJob): number {
  const statusDiff = (JOB_STATUS_ORDER[getStableJobSortStatus(a)] ?? 999) - (JOB_STATUS_ORDER[getStableJobSortStatus(b)] ?? 999);
  if (statusDiff !== 0) return statusDiff;

  const createdDiff = getCreatedTimeMs(a) - getCreatedTimeMs(b);
  if (createdDiff !== 0) return createdDiff;

  return getStableJobKey(a).localeCompare(getStableJobKey(b));
}

function getGroupSortMeta(group: JobGroup): { statusRank: number; createdMs: number; key: string } {
  if (group.type === "single") {
    return {
      statusRank: JOB_STATUS_ORDER[getStableJobSortStatus(group.job)] ?? 999,
      createdMs: getCreatedTimeMs(group.job),
      key: getStableJobKey(group.job),
    };
  }

  const episodes = group.seasons.flatMap((season) => season.episodes);
  let statusRank = 999;
  let createdMs = Number.POSITIVE_INFINITY;
  let key = group.id;

  for (const ep of episodes) {
    const epRank = JOB_STATUS_ORDER[getStableJobSortStatus(ep)] ?? 999;
    if (epRank < statusRank) statusRank = epRank;

    const epCreated = getCreatedTimeMs(ep);
    if (epCreated < createdMs) createdMs = epCreated;

    const epKey = getStableJobKey(ep);
    if (epKey && epKey < key) key = epKey;
  }

  return {
    statusRank,
    createdMs: Number.isFinite(createdMs) ? createdMs : 0,
    key,
  };
}

function compareGroupsStable(a: JobGroup, b: JobGroup): number {
  const aMeta = getGroupSortMeta(a);
  const bMeta = getGroupSortMeta(b);

  if (aMeta.statusRank !== bMeta.statusRank) return aMeta.statusRank - bMeta.statusRank;
  if (aMeta.createdMs !== bMeta.createdMs) return aMeta.createdMs - bMeta.createdMs;
  return aMeta.key.localeCompare(bMeta.key);
}

// Group jobs by serial - serials are identified by season_number or episode_number being set
function groupJobsBySerial(jobs: IngestionJob[]): JobGroup[] {
  // Map: seriesKey -> Map: seasonNumber -> episodes[]
  const seriesMap = new Map<string, Map<number, IngestionJob[]>>();
  const singles: IngestionJob[] = [];
  
  for (const job of jobs) {
    const isEpisode = !!job.episode_number || !!job.season_number;
    
    if (isEpisode) {
      // Primary grouping key: series_slug (most reliable from backend)
      // Fallback: source_id with episode pattern stripped
      // Last resort: metadata.title or synthetic key
      let seriesKey = job.series_slug || "";
      
      if (!seriesKey || seriesKey.length < 3) {
        // Try to extract from source_id
        let sourceIdKey = job.source_id || "";
        const episodePatterns = [
          /[-_\s]?s\d+e\d+$/i,
          /[-_\s]?season\s*\d+[-_\s]?episode\s*\d+$/i,
          /[-_\s]?ep\s*\d+$/i,
          /[-_\s]?\d+\/\d+$/i,
          /[-_\s]?\d+x\d+$/i,
        ];
        
        for (const pattern of episodePatterns) {
          const match = sourceIdKey.match(pattern);
          if (match) {
            sourceIdKey = sourceIdKey.substring(0, match.index).trim();
            break;
          }
        }
        
        seriesKey = sourceIdKey.length >= 3 ? sourceIdKey : (job.metadata?.title || "");
      }
      
      // Final fallback: create stable key from season/episode
      if (!seriesKey || seriesKey.length < 3) {
        seriesKey = `serial-s${job.season_number || 1}e${job.episode_number || 1}`;
      }
      
      if (!seriesMap.has(seriesKey)) {
        seriesMap.set(seriesKey, new Map());
      }
      const seasonMap = seriesMap.get(seriesKey)!;
      
      const seasonNum = job.season_number || 1;
      if (!seasonMap.has(seasonNum)) {
        seasonMap.set(seasonNum, []);
      }
      seasonMap.get(seasonNum)!.push(job);
    } else {
      singles.push(job);
    }
  }
  
  // Convert to JobGroup array
  const groups: JobGroup[] = [];
  
  // Add serial groups
  seriesMap.forEach((seasonMap, seriesKey) => {
    const seasons: SeasonGroup[] = [];
    let totalEpisodes = 0;
    
    Array.from(seasonMap.entries()).sort((a, b) => a[0] - b[0]).forEach(([seasonNum, epJobs]) => {
      epJobs.sort((a, b) => (a.episode_number || 0) - (b.episode_number || 0));
      seasons.push({ season_number: seasonNum, episodes: epJobs });
      totalEpisodes += epJobs.length;
    });
    
    const firstEp = seasons[0]?.episodes[0];
    const seriesTitle = firstEp?.metadata?.title || seriesKey;
    
    groups.push({
      type: "serial",
      id: firstEp?.id || seriesKey,
      title: seriesTitle,
      seasons,
      expanded: false,
      totalEpisodes,
      totalSeasons: seasons.length
    });
  });
  
  // Add single movies
  for (const job of singles) {
    groups.push({ type: "single", job });
  }

  // Keep list order stable across 1s polling updates.
  groups.sort(compareGroupsStable);

  return groups;
}

// Format bytes to human readable format
function formatBytes(bytes: number): string {
  if (!bytes || bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
}

// Format speed from MB/s
function formatSpeed(mbps: number): string {
  if (!mbps || mbps === 0) return "";
  if (mbps >= 1) {
    return mbps.toFixed(1) + " MB/s";
  }
  return (mbps * 1024).toFixed(0) + " KB/s";
}

// Format ETA from seconds
function formatEta(seconds: number): string {
  if (!seconds || seconds <= 0) return "Calculating...";
  if (seconds < 60) {
    return seconds + "s";
  }
  if (seconds < 3600) {
    const m = Math.floor(seconds / 60);
    const s = seconds % 60;
    return m + "m " + s + "s";
  }
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  return h + "h " + m + "m";
}

function getJobTime(job: IngestionJob, key: string): string | undefined {
  const value = (job as unknown as Record<string, unknown>)[key];
  return typeof value === "string" ? value : undefined;
}

function isTerminalJobStatus(status: string | undefined): boolean {
  return status === "completed" || status === "failed" || status === "download_failed";
}

const STUCK_THRESHOLD_MS = 30 * 60 * 1000;

function formatHMS(totalSeconds: number): string {
  totalSeconds = Math.max(0, Math.floor(totalSeconds));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (v: number) => String(v).padStart(2, "0");
  if (hours > 0) return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
  return `${pad(minutes)}:${pad(seconds)}`;
}

function pickJobTime(job: IngestionJob, ...keys: string[]): string | undefined {
  for (const k of keys) {
    const v = getJobTime(job, k);
    if (v) return v;
  }
  return undefined;
}

// Phase-aware elapsed time. Each pipeline phase (download / queue-wait /
// processing) gets its own clock so a job that sat 31h waiting for a process
// slot does not appear to have been "processing" for 31h.
//
// downloading        → now - download_started_at
// ready_to_process   → now - queued_for_processing_at (queue wait)
// processing/upload  → now - processing_started_at
// completed/failed   → frozen processing duration if available, else download
//
// Legacy jobs without the new timestamps fall back to started_at.
type ElapsedKind = "download" | "queue" | "processing" | "total";

function computeElapsed(job: IngestionJob, now: number): { kind: ElapsedKind; text: string } {
  const status = String(job.status || "");
  const stage = String((job as unknown as { stage?: string }).stage || "");

  const downloadStart = pickJobTime(job, "download_started_at", "started_at");
  const downloadEnd = pickJobTime(job, "download_finished_at");
  const queuedAt = pickJobTime(job, "queued_for_processing_at", "download_finished_at");
  const procStart = pickJobTime(job, "processing_started_at");
  const procEnd = pickJobTime(job, "processing_finished_at", "completed_at");

  const ms = (s?: string) => (s ? new Date(s).getTime() : NaN);

  if (status === "downloading" || stage === "download" || stage === "downloading") {
    const start = ms(downloadStart);
    if (Number.isFinite(start)) return { kind: "download", text: formatHMS((now - start) / 1000) };
  }

  if (status === "ready_to_process" || status === "downloaded" || stage === "ready_to_process") {
    const start = ms(queuedAt);
    if (Number.isFinite(start)) return { kind: "queue", text: formatHMS((now - start) / 1000) };
  }

  if (
    status === "processing" || status === "uploading" ||
    status === "hls_processing" || status === "finalizing_storage" ||
    stage === "processing" || stage === "uploading" || stage === "hls_processing"
  ) {
    const start = ms(procStart) || ms(pickJobTime(job, "started_at"));
    if (Number.isFinite(start)) return { kind: "processing", text: formatHMS((now - start) / 1000) };
  }

  if (isTerminalJobStatus(status)) {
    const ps = ms(procStart);
    const pe = ms(procEnd);
    if (Number.isFinite(ps) && Number.isFinite(pe) && pe >= ps) {
      return { kind: "processing", text: formatHMS((pe - ps) / 1000) };
    }
    const ds = ms(downloadStart);
    const de = ms(downloadEnd);
    if (Number.isFinite(ds) && Number.isFinite(de) && de >= ds) {
      return { kind: "download", text: formatHMS((de - ds) / 1000) };
    }
    const start = ms(pickJobTime(job, "started_at"));
    const end = ms(job.completed_at || job.updated_at);
    if (Number.isFinite(start) && Number.isFinite(end)) {
      return { kind: "total", text: formatHMS((end - start) / 1000) };
    }
  }

  // Fallback: legacy started_at -> now (or freeze if terminal)
  const fallbackStart = ms(pickJobTime(job, "started_at"));
  if (!Number.isFinite(fallbackStart)) return { kind: "total", text: "00:00" };
  const endMs = isTerminalJobStatus(status)
    ? ms(job.completed_at || job.updated_at) || now
    : now;
  return { kind: "total", text: formatHMS((endMs - fallbackStart) / 1000) };
}

function formatElapsedTime(job: IngestionJob, now: number): string {
  return computeElapsed(job, now).text;
}

// Series elapsed time: span from the earliest episode start to the latest
// episode activity. Episodes that never started are ignored for the start
// boundary. If no episode has started yet we show 00:00, mirroring movies.
// When all episodes are in a terminal state, the end boundary freezes on
// the latest updated/completed timestamp instead of ticking with `now`.
function formatSerialElapsedTime(episodes: IngestionJob[], now: number): string {
  let earliestStart = Number.POSITIVE_INFINITY;
  let latestEnd = 0;
  let allTerminal = episodes.length > 0;

  for (const ep of episodes) {
    const startRaw =
      getJobTime(ep, "importStartedAt") ||
      getJobTime(ep, "import_started_at") ||
      getJobTime(ep, "startedAt") ||
      getJobTime(ep, "started_at");
    if (startRaw) {
      const s = new Date(startRaw).getTime();
      if (Number.isFinite(s) && s < earliestStart) earliestStart = s;
    }

    const endRaw = isTerminalJobStatus(ep.status)
      ? ep.completed_at || ep.updated_at
      : ep.updated_at;
    if (endRaw) {
      const e = new Date(endRaw).getTime();
      if (Number.isFinite(e) && e > latestEnd) latestEnd = e;
    }

    if (!isTerminalJobStatus(ep.status)) allTerminal = false;
  }

  if (!Number.isFinite(earliestStart)) return "00:00";

  const endMs = allTerminal && latestEnd > 0 ? latestEnd : now;
  const totalSeconds = Math.max(0, Math.floor((endMs - earliestStart) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (value: number) => String(value).padStart(2, "0");

  if (hours > 0) {
    return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
  }
  return `${pad(minutes)}:${pad(seconds)}`;
}

function getTimeMs(value: string | undefined): number {
  if (!value) return NaN;
  return new Date(value).getTime();
}

function getLastUpdateMs(job: IngestionJob): number {
  const updatedMs = getTimeMs(job.updated_at);
  if (Number.isFinite(updatedMs)) return updatedMs;
  return getTimeMs(job.created_at);
}

function getLastUpdateAgeMs(job: IngestionJob, now: number): number {
  const updatedMs = getLastUpdateMs(job);
  if (!Number.isFinite(updatedMs)) return 0;
  return Math.max(0, now - updatedMs);
}

function formatDurationShort(ms: number): string {
  const totalSeconds = Math.max(0, Math.floor(ms / 1000));
  if (totalSeconds < 60) return `${totalSeconds}s`;
  const totalMinutes = Math.floor(totalSeconds / 60);
  if (totalMinutes < 60) return `${totalMinutes}m`;
  const hours = Math.floor(totalMinutes / 60);
  const minutes = totalMinutes % 60;
  if (hours < 24) return minutes > 0 ? `${hours}h ${minutes}m` : `${hours}h`;
  const days = Math.floor(hours / 24);
  const remainingHours = hours % 24;
  return remainingHours > 0 ? `${days}d ${remainingHours}h` : `${days}d`;
}

function formatLogTime(timestamp: string | undefined, now: number): string {
  if (!timestamp) return "";
  const ageMs = Math.max(0, now - getTimeMs(timestamp));
  return `${formatDurationShort(ageMs)} ago`;
}

const SOURCES = [
  { id: "uzmovi", name: "Uzmovi", url: "uzmovi.tv", icon: Film },
  { id: "freekino", name: "Freekino", url: "freekino.net", icon: Film },
  { id: "asilmedia", name: "Asilmedia", url: "asilmedia.org", icon: Film },
  { id: "manual", name: "Manual", url: "", icon: Link },
];

const STATUS_CONFIG: Record<IngestionStatus, { color: string; icon: React.ElementType; label: string }> = {
  queued: { color: "bg-slate-500", icon: Clock, label: "Queued" },
  pending: { color: "bg-gray-500", icon: Clock, label: "Pending" },
  parsing: { color: "bg-blue-500", icon: Loader2, label: "Parsing" },
  downloading: { color: "bg-yellow-500", icon: Download, label: "Downloading" },
  downloaded: { color: "bg-amber-500", icon: CheckCircle, label: "Downloaded" },
  ready_to_process: { color: "bg-orange-500", icon: Clock, label: "Ready to Process" },
  processing: { color: "bg-purple-500", icon: Settings, label: "Processing" },
  uploading: { color: "bg-indigo-500", icon: Upload, label: "Uploading" },
  completed: { color: "bg-green-500", icon: CheckCircle, label: "Completed" },
  failed: { color: "bg-red-500", icon: XCircle, label: "Failed" },
  download_failed: { color: "bg-red-600", icon: XCircle, label: "Download Failed" },
  parsing_complete: { color: "bg-teal-500", icon: CheckCircle, label: "Parsing Complete" },
  enriching_metadata: { color: "bg-cyan-500", icon: Loader2, label: "Enriching Metadata" },
  finding_poster: { color: "bg-cyan-500", icon: Loader2, label: "Finding Poster" },
  generating_poster: { color: "bg-cyan-500", icon: Loader2, label: "Generating Poster" },
  uploading_poster: { color: "bg-indigo-500", icon: Upload, label: "Uploading Poster" },
  generating_backdrop: { color: "bg-cyan-500", icon: Loader2, label: "Generating Backdrop" },
  creating_movie: { color: "bg-cyan-500", icon: Loader2, label: "Creating Movie" },
  sending_notification: { color: "bg-cyan-500", icon: Loader2, label: "Sending Notification" },
  cutting_video: { color: "bg-purple-500", icon: Settings, label: "Cutting Video" },
  removing_watermark: { color: "bg-purple-500", icon: Settings, label: "Removing Watermark" },
  adding_logo: { color: "bg-purple-500", icon: Settings, label: "Adding Logo" },
  hls_processing: { color: "bg-purple-500", icon: Settings, label: "HLS Processing" },
  finalizing_storage: { color: "bg-indigo-500", icon: Upload, label: "Finalizing Storage" },
};

// Stage to status mapping for badge display
const STAGE_STATUS_MAP: Record<string, IngestionStatus> = {
  parse: "parsing",
  parsing: "parsing",
  queued: "queued",
  downloaded: "downloaded",
  ready_to_process: "ready_to_process",
  download: "downloading",
  downloading: "downloading",
  processing: "processing",
  upload: "uploading",
  uploading: "uploading",
  completed: "completed",
  failed: "failed",
  pending: "pending",
  download_failed: "download_failed",
  parsing_complete: "parsing_complete",
  enriching_metadata: "enriching_metadata",
  finding_poster: "finding_poster",
  generating_poster: "generating_poster",
  uploading_poster: "uploading_poster",
  generating_backdrop: "generating_backdrop",
  creating_movie: "creating_movie",
  sending_notification: "sending_notification",
  cutting_video: "cutting_video",
  removing_watermark: "removing_watermark",
  adding_logo: "adding_logo",
  hls_processing: "hls_processing",
  finalizing_storage: "finalizing_storage",
};

// Safe fallback config for unknown statuses
const FALLBACK_STATUS_CONFIG = {
  color: "bg-gray-500",
  icon: AlertTriangle,
  label: "Unknown status",
};

// Safe helper to get status metadata with fallback for unknown statuses
function getStatusMeta(status: IngestionStatus | string | undefined): {
  color: string;
  icon: React.ElementType;
  label: string;
} {
  if (!status) {
    return FALLBACK_STATUS_CONFIG;
  }
  const config = STATUS_CONFIG[status as IngestionStatus];
  if (!config) {
    return FALLBACK_STATUS_CONFIG;
  }
  return config;
}

type JobFilter = "all" | "active" | "pending" | "processing" | "failed" | "completed" | "stuck";

const JOB_FILTERS: { id: JobFilter; label: string }[] = [
  { id: "all", label: "All" },
  { id: "active", label: "Active" },
  { id: "pending", label: "Pending" },
  { id: "processing", label: "Processing" },
  { id: "failed", label: "Failed" },
  { id: "completed", label: "Completed" },
  { id: "stuck", label: "Stuck" },
];

function getJobDisplayStatus(job: IngestionJob): string {
  const stage = job.stage || job.status;
  return STAGE_STATUS_MAP[stage] || job.status || "unknown";
}

function normalizePercent(value: number | undefined): number {
  if (typeof value !== "number" || !Number.isFinite(value)) return 0;
  return Math.max(0, Math.min(100, Math.round(value)));
}

function isProgressStage(status: string | undefined): boolean {
  return status === "downloading" || status === "processing" || status === "uploading" || status === "hls_processing" || status === "finalizing_storage";
}

function getJobProgress(job: IngestionJob): number {
  const displayStatus = getJobDisplayStatus(job);
  const canonicalProgress = normalizePercent(job.progress);

  if (job.status === "completed" || displayStatus === "completed") return 100;
  if (job.status === "failed" || job.status === "download_failed") return canonicalProgress;
  if (displayStatus === "downloading") return canonicalProgress;
  if (displayStatus === "processing" || displayStatus === "uploading" || displayStatus === "hls_processing" || displayStatus === "finalizing_storage") {
    return canonicalProgress;
  }
  return canonicalProgress;
}

function isWaitingForProcessing(job: IngestionJob): boolean {
  const displayStatus = getJobDisplayStatus(job);
  const progress = getJobProgress(job);
  return progress >= 100 &&
    displayStatus !== "ready_to_process" &&
    displayStatus !== "processing" &&
    displayStatus !== "uploading" &&
    displayStatus !== "completed" &&
    displayStatus !== "failed" &&
    displayStatus !== "download_failed";
}

function getJobStatusSummary(job: IngestionJob): string {
  const displayStatus = getJobDisplayStatus(job);
  const statusMeta = getStatusMeta(displayStatus);
  const downloadedBytes = typeof job.downloaded_bytes === "number" ? job.downloaded_bytes : 0;
  const totalBytes = typeof job.total_bytes === "number" ? job.total_bytes : 0;
  const speedMBps = typeof job.speed_mbps === "number" ? job.speed_mbps : 0;
  const etaSeconds = typeof job.eta_seconds === "number" ? job.eta_seconds : 0;

  if (displayStatus === "downloading") {
    const parts = [statusMeta.label];
    if (downloadedBytes > 0 || totalBytes > 0) {
      parts.push(totalBytes > 0
        ? `${formatBytes(downloadedBytes)} / ${formatBytes(totalBytes)}`
        : formatBytes(downloadedBytes));
    }
    if (speedMBps > 0) parts.push(formatSpeed(speedMBps));
    if (etaSeconds > 0) parts.push(`ETA ${formatEta(etaSeconds)}`);
    return parts.join(" • ");
  }

  if (isWaitingForProcessing(job)) {
    return "Download complete but waiting for processing";
  }

  const message = (job.message || "").trim();
  if (message && !(isProgressStage(displayStatus) && /\b\d{1,3}%\b/.test(message))) {
    return message;
  }

  return `${job.source} • ${new Date(job.created_at).toLocaleString()}`;
}

function isTerminalJob(job: IngestionJob): boolean {
  return isTerminalJobStatus(job.status);
}

function isStuckJob(job: IngestionJob, now: number): boolean {
  return !isTerminalJob(job) && getLastUpdateAgeMs(job, now) > STUCK_THRESHOLD_MS;
}

function getJobSummary(jobs: IngestionJob[], now: number) {
  return jobs.reduce(
    (summary, job) => {
      const displayStatus = getJobDisplayStatus(job);
      if (!isTerminalJob(job)) summary.active += 1;
      if (displayStatus === "queued" || displayStatus === "pending" || displayStatus === "downloaded" || displayStatus === "ready_to_process") {
        summary.pending += 1;
      }
      if (displayStatus === "downloading") summary.processing += 1;
      if (displayStatus === "processing" || displayStatus === "hls_processing") summary.processing += 1;
      if (displayStatus === "uploading" || displayStatus === "finalizing_storage") summary.uploading += 1;
      if (job.status === "failed" || job.status === "download_failed") summary.failed += 1;
      if (isStuckJob(job, now)) summary.stuck += 1;
      return summary;
    },
    { active: 0, pending: 0, processing: 0, uploading: 0, failed: 0, stuck: 0 }
  );
}

function matchesJobFilter(job: IngestionJob, filter: JobFilter, now: number): boolean {
  const displayStatus = getJobDisplayStatus(job);
  if (filter === "all") return true;
  if (filter === "active") return !isTerminalJob(job);
  if (filter === "stuck") return isStuckJob(job, now);
  if (filter === "failed") return job.status === "failed" || job.status === "download_failed";
  if (filter === "processing") {
    return displayStatus === "downloading" || displayStatus === "processing" || displayStatus === "hls_processing" || displayStatus === "uploading" || displayStatus === "finalizing_storage";
  }
  if (filter === "pending") {
    return displayStatus === "queued" || displayStatus === "pending" || displayStatus === "downloaded" || displayStatus === "ready_to_process";
  }
  return displayStatus === filter;
}

function getFilteredAndSortedJobs(jobs: IngestionJob[], filter: JobFilter, now: number): IngestionJob[] {
  return jobs
    .filter((job) => job.id && matchesJobFilter(job, filter, now))
    .sort(compareJobsStable);
}

// Catalog Tab Component
function CatalogTab({ 
  source, 
  token,
  onImportSuccess 
}: { 
  source: typeof SOURCES[0];
  token: string;
  onImportSuccess: () => void;
}) {
  const [catalog, setCatalog] = useState<CatalogItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [totalPages, setTotalPages] = useState(1);
  const [total, setTotal] = useState(0);
  const [loadingMore, setLoadingMore] = useState(false);
  const [searchQuery, setSearchQuery] = useState("");
  const [searching, setSearching] = useState(false);
  const [searchResults, setSearchResults] = useState<SearchResult[]>([]);
  const [showSearch, setShowSearch] = useState(false);
  const [importing, setImporting] = useState<string | null>(null);
  const [typeFilter, setTypeFilter] = useState<string>("");
  const [error, setError] = useState("");
  const [directUrl, setDirectUrl] = useState("");
  const [directImporting, setDirectImporting] = useState(false);
  const [categories, setCategories] = useState<CatalogCategory[]>([]);
  const [categoriesLoading, setCategoriesLoading] = useState<boolean>(true);
  const [selectedCategory, setSelectedCategory] = useState<string>("");
  const limit = 20;

  // Fetch categories once on mount (non-blocking)
  useEffect(() => {
    setCategoriesLoading(true);
    listCatalogCategories(source.id)
      .then((res) => {
        const cats = res.categories || [];
        setCategories(cats);
      })
      .catch(() => {
        setCategories([]);
      })
      .finally(() => setCategoriesLoading(false));
  }, [source.id]);

  const fetchCatalog = useCallback(async (pageNum: number, append = false) => {
    if (pageNum === 1) setLoading(true);
    else setLoadingMore(true);
    setError("");

    try {
      const params: { page?: number; limit?: number; type?: string; category_url?: string } = {
        page: pageNum,
        limit: limit,
      };
      if (typeFilter) params.type = typeFilter;
      if (selectedCategory) params.category_url = selectedCategory;

      const result = await listCatalog(source.id, params);
      setCatalog(prev => append ? [...prev, ...result.items] : result.items);
      setTotalPages(result.total_pages);
      setTotal(result.total);
      setPage(pageNum);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Failed to load catalog");
    } finally {
      setLoading(false);
      setLoadingMore(false);
    }
  }, [source.id, typeFilter, selectedCategory]);

  useEffect(() => {
    fetchCatalog(1);
  }, [fetchCatalog]);

  const handleSearch = async () => {
    if (!searchQuery.trim()) return;
    
    setSearching(true);
    setError("");
    
    try {
      const result = await searchSource(source.id, searchQuery);
      setSearchResults(result.results || []);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Search failed");
    } finally {
      setSearching(false);
    }
  };

  const handleImportCatalog = async (item: CatalogItem) => {
    if (!token) return;
    
    setImporting(item.source_id);
    setError("");
    
    try {
      // Send empty type when parser flagged it "unknown" so the backend re-
      // detects via /details (which fetches the page and runs the strong
      // soup heuristics) instead of refusing the import.
      const rawType = (item.type || "").toLowerCase();
      const importType = (rawType === "movie" || rawType === "serial" || rawType === "series") ? rawType : "";
      await importFromCatalog(token, {
        source: source.id,
        source_id: item.source_id,
        detail_url: item.detail_url,
        title: item.title,
        type: importType as "movie" | "serial",
      });
      onImportSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(null);
    }
  };

  const handleImportSearch = async (result: SearchResult) => {
    if (!token) return;
    
    setImporting(result.source_id);
    setError("");
    
    try {
      const importType = getSearchResultContentType(result);
      await importFromCatalog(token, {
        source: result.source,
        source_id: result.source_id,
        detail_url: result.detail_url,
        title: result.title,
        type: importType as "movie" | "serial",
      });
      onImportSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import failed");
    } finally {
      setImporting(null);
    }
  };

  const handleDirectUrlImport = async () => {
    if (!token || !directUrl.trim()) return;
    
    setDirectImporting(true);
    setError("");
    
    try {
      await importFromCatalog(token, {
        source: source.id,
        source_id: "",
        detail_url: directUrl.trim(),
        title: "",
        // Empty type → backend hits /details, parser detects movie/serial.
        // Backend rejects with "Content type could not be detected" if unknown.
        type: "" as unknown as "movie" | "serial",
      });
      onImportSuccess();
      setDirectUrl("");
    } catch (err) {
      const msg = err instanceof Error ? err.message : "Havola orqali import xatosiz";
      // Surface backend's structured "Content type could not be detected" error verbatim
      setError(msg);
    } finally {
      setDirectImporting(false);
    }
  };

  const SourceIcon = source.icon;

  return (
    <div className="space-y-4">
      {/* Header with search toggle */}
      <div className="flex items-center justify-between">
        <div className="flex items-center gap-2">
          <SourceIcon className="w-5 h-5 text-brand-red" />
          <span className="text-gray-400">{total} items total</span>
        </div>
        <div className="flex items-center gap-2 flex-wrap">
          {/* Category filter — always visible; disabled while loading or if source has no categories */}
          <select
            value={selectedCategory}
            onChange={(e) => { setSelectedCategory(e.target.value); setPage(1); }}
            disabled={categoriesLoading || categories.length === 0}
            className="bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red disabled:opacity-40 disabled:cursor-not-allowed"
          >
            <option value="">
              {categoriesLoading ? "Loading categories…" : categories.length === 0 ? "No categories" : "All Categories"}
            </option>
            {categories.map((cat) => (
              <option key={cat.id || cat.url} value={cat.url}>{cat.name}</option>
            ))}
          </select>
          {/* Type filter */}
          <select
            value={typeFilter}
            onChange={(e) => setTypeFilter(e.target.value)}
            className="bg-brand-card border border-brand-border rounded-lg px-3 py-2 text-white text-sm focus:outline-none focus:border-brand-red"
          >
            <option value="">All Types</option>
            <option value="movies">Movies</option>
            <option value="serials">Serials</option>
          </select>
          <button
            onClick={() => setShowSearch(!showSearch)}
            className={`px-4 py-2 rounded-lg transition-colors ${
              showSearch 
                ? "bg-brand-red text-white" 
                : "bg-brand-card text-gray-400 hover:text-white border border-brand-border"
            }`}
          >
            <Search className="w-4 h-4 inline mr-2" />
            Search
          </button>
        </div>
      </div>

      {/* Direct URL Import panel */}
      <div className="bg-brand-card border border-brand-border rounded-lg p-4">
        <h3 className="text-white font-semibold mb-3">Manba havolasi orqali import</h3>
        <div className="flex gap-2">
          <input
            type="text"
            value={directUrl}
            onChange={(e) => setDirectUrl(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && handleDirectUrlImport()}
            placeholder="Kino yoki serial sahifa linkini kiriting..."
            className="flex-1 bg-brand-dark border border-brand-border rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
          />
          <button
            onClick={handleDirectUrlImport}
            disabled={directImporting || !directUrl.trim()}
            className="bg-brand-red hover:bg-orange-700 disabled:opacity-60 px-4 py-2 rounded-lg transition-colors flex items-center gap-2"
          >
            {directImporting ? (
              <Loader2 className="w-5 h-5 animate-spin" />
            ) : (
              <>
                <Link className="w-5 h-5" />
                <span>Havola orqali import</span>
              </>
            )}
          </button>
        </div>
        <p className="text-gray-500 text-sm mt-2">
          Qidiruv natijasi topilmasa, havola orqali import qilib ko'ring
        </p>
      </div>

      {/* Search panel */}
      {showSearch && (
        <div className="bg-brand-card border border-brand-border rounded-lg p-4">
          <div className="flex gap-2">
            <input
              type="text"
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && handleSearch()}
              placeholder={`Search in ${source.name}...`}
              className="flex-1 bg-brand-dark border border-brand-border rounded-lg px-4 py-2 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
            <button
              onClick={handleSearch}
              disabled={searching}
              className="bg-brand-red hover:bg-orange-700 disabled:opacity-60 px-4 py-2 rounded-lg transition-colors"
            >
              {searching ? (
                <Loader2 className="w-5 h-5 animate-spin" />
              ) : (
                <Search className="w-5 h-5" />
              )}
            </button>
          </div>
          
          {/* Search results */}
          {searchResults.length > 0 && (
            <div className="mt-4 grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-4 max-h-96 overflow-y-auto">
              {searchResults.map((result, index) => (
                <div key={index} className="bg-brand-dark border border-brand-border rounded-lg p-3 flex gap-3">
                  {(result.img || result.poster) && (
                    <div className="w-16 h-24 bg-brand-card rounded overflow-hidden flex-shrink-0">
                      <MediaImage
                        src={result.img || result.poster || ""}
                        alt={result.title}
                        className="w-16 h-24 object-cover"
                        loading="lazy"
                      />
                    </div>
                  )}
                  <div className="flex-1 min-w-0">
                    <h4 className="font-semibold text-white text-sm line-clamp-2">{result.title}</h4>
                    <div className="flex items-center gap-2 mt-1 flex-wrap">
                      {result.year && result.year !== 0 && (
                        <span className="text-xs text-gray-400">{result.year}</span>
                      )}
                      {(() => {
                        const t = getSearchResultContentType(result);
                        if (t === "serial") {
                          return <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-blue-600 text-white">Serial</span>;
                        }
                        if (t === "movie") {
                          return <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-green-600 text-white">Movie</span>;
                        }
                        return <span className="px-1.5 py-0.5 rounded text-[10px] font-semibold bg-amber-600 text-white">Aniqlanmoqda</span>;
                      })()}
                    </div>
                    <button
                      onClick={() => handleImportSearch(result)}
                      disabled={importing === result.source_id}
                      className="mt-2 w-full bg-brand-red hover:bg-orange-700 disabled:opacity-60 py-1.5 rounded text-sm flex items-center justify-center gap-2 transition-colors"
                    >
                      {importing === result.source_id ? (
                        <Loader2 className="w-3 h-3 animate-spin" />
                      ) : (
                        <Plus className="w-3 h-3" />
                      )}
                      Import
                    </button>
                  </div>
                </div>
              ))}
            </div>
          )}
          
          {searchQuery && searchResults.length === 0 && !searching && (
            <p className="text-center text-gray-500 py-4">No results found</p>
          )}
        </div>
      )}

      {/* Error display */}
      {error && (
        <div className="bg-red-500/10 border border-red-500/30 text-red-400 rounded-lg px-4 py-3">
          {error}
        </div>
      )}

      {/* Catalog Grid - hide when search mode is active with query */}
      {showSearch && searchQuery ? (
        <div className="text-center text-gray-500 py-12">
          {searchResults.length > 0 ? `Found ${searchResults.length} search results` : 'No search results'}
        </div>
      ) : loading ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
          <span className="ml-3 text-gray-400">Loading catalog...</span>
        </div>
      ) : catalog.length === 0 ? (
        <div className="text-center text-gray-500 py-12">
          No items found in catalog
        </div>
      ) : (
        <>
          <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4 gap-4">
            {catalog.map((item) => (
              <div key={item.source_id} className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
                <div className="relative">
                  {item.poster ? (
                    <div className="w-full h-48 bg-brand-dark">
                      <MediaImage
                        src={item.poster}
                        alt={item.title}
                        className="w-full h-48 object-cover"
                        loading="lazy"
                      />
                    </div>
                  ) : (
                    <div className="w-full h-48 bg-brand-dark flex items-center justify-center">
                      <Film className="w-12 h-12 text-gray-600" />
                    </div>
                  )}
                  {/* Type badge */}
                  {(() => {
                    const t = (item.type || "").toLowerCase();
                    if (t === "serial" || t === "series") {
                      return (
                        <div className="absolute top-2 right-2 px-2 py-0.5 rounded text-xs font-medium bg-blue-600 text-white">
                          <span className="flex items-center gap-1"><Tv className="w-3 h-3" /> Serial</span>
                        </div>
                      );
                    }
                    if (t === "movie") {
                      return (
                        <div className="absolute top-2 right-2 px-2 py-0.5 rounded text-xs font-medium bg-green-600 text-white">
                          <span className="flex items-center gap-1"><Film className="w-3 h-3" /> Movie</span>
                        </div>
                      );
                    }
                    return (
                      <div className="absolute top-2 right-2 px-2 py-0.5 rounded text-xs font-medium bg-amber-600 text-white">
                        <span>Aniqlanmoqda</span>
                      </div>
                    );
                  })()}
                </div>
                <div className="p-3">
                  <h3 className="font-semibold text-white text-sm line-clamp-2">{item.title}</h3>
                  <div className="flex items-center gap-2 mt-1">
                    {item.year && item.year !== 0 && (
                      <span className="text-xs text-gray-400">{item.year}</span>
                    )}
                    {item.genres && item.genres.length > 0 && (
                      <span className="text-xs text-gray-500 truncate">{item.genres.slice(0, 2).join(", ")}</span>
                    )}
                  </div>
                  {item.description && (
                    <p className="text-xs text-gray-500 mt-2 line-clamp-2">{item.description}</p>
                  )}
                  <button
                    onClick={() => handleImportCatalog(item)}
                    disabled={importing === item.source_id}
                    className="mt-3 w-full bg-brand-red hover:bg-orange-700 disabled:opacity-60 py-2 rounded-lg text-sm flex items-center justify-center gap-2 transition-colors"
                  >
                    {importing === item.source_id ? (
                      <Loader2 className="w-4 h-4 animate-spin" />
                    ) : (
                      <Plus className="w-4 h-4" />
                    )}
                    Import
                  </button>
                </div>
              </div>
            ))}
          </div>

          {/* Pagination */}
          <div className="flex items-center justify-between pt-4">
            <span className="text-sm text-gray-400">
              Page {page} of {totalPages}
            </span>
            <div className="flex gap-2">
              <button
                onClick={() => fetchCatalog(page - 1)}
                disabled={page <= 1 || loadingMore}
                className="px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white disabled:opacity-50 hover:bg-gray-700 transition-colors flex items-center gap-2"
              >
                <ChevronLeft className="w-4 h-4" />
                Previous
              </button>
              <button
                onClick={() => fetchCatalog(page + 1)}
                disabled={page >= totalPages || loadingMore}
                className="px-4 py-2 bg-brand-card border border-brand-border rounded-lg text-white disabled:opacity-50 hover:bg-gray-700 transition-colors flex items-center gap-2"
              >
                Next
                <ChevronRight className="w-4 h-4" />
              </button>
            </div>
          </div>
          
          {loadingMore && (
            <div className="flex items-center justify-center py-4">
              <Loader2 className="w-6 h-6 animate-spin text-brand-red" />
            </div>
          )}
        </>
      )}
    </div>
  );
}

// Manual Import Tab Component
function ManualTab({
  token,
  onImportSuccess
}: {
  token: string;
  onImportSuccess: () => void;
}) {
  const [sourceType, setSourceType] = useState<"direct" | "youtube">("direct");
  const [videoUrl, setVideoUrl] = useState("");
  const [title, setTitle] = useState("");
  const [year, setYear] = useState("");
  const [poster, setPoster] = useState("");
  const [backdrop, setBackdrop] = useState("");
  const [type, setType] = useState<"movie" | "serial">("movie");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState("");
  const [success, setSuccess] = useState("");

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    if (!videoUrl.trim()) {
      setError("URL kiritish majburiy");
      return;
    }

    if (sourceType === "youtube") {
      const ytPattern = /^https?:\/\/(www\.)?(youtube\.com\/(watch\?v=|shorts\/)|youtu\.be\/)/;
      if (!ytPattern.test(videoUrl.trim())) {
        setError("Yaroqli YouTube URL kiriting (youtube.com yoki youtu.be)");
        return;
      }
    }

    setSubmitting(true);
    setError("");
    setSuccess("");

    try {
      await createManualImport(token, {
        video_url: videoUrl.trim(),
        title: title.trim() || undefined,
        year: year ? parseInt(year) : undefined,
        poster: poster.trim() || undefined,
        backdrop: backdrop.trim() || undefined,
        type: type,
      });

      setSuccess("Import vazifasi muvaffaqiyatli yaratildi!");
      setVideoUrl("");
      setTitle("");
      setYear("");
      setPoster("");
      setBackdrop("");
      setType("movie");
      onImportSuccess();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Import vazifasini yaratib bo'lmadi");
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="max-w-2xl">
      <div className="bg-brand-card border border-brand-border rounded-lg p-6">
        <h3 className="text-lg font-semibold text-white mb-4 flex items-center gap-2">
          <Link className="w-5 h-5 text-brand-red" />
          Manual Video Import
        </h3>

        {error && (
          <div className="bg-red-500/10 border border-red-500/30 text-red-400 rounded-lg px-4 py-3 mb-4">
            {error}
          </div>
        )}

        {success && (
          <div className="bg-green-500/10 border border-green-500/30 text-green-400 rounded-lg px-4 py-3 mb-4">
            {success}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Source type toggle */}
          <div>
            <label className="block text-sm text-gray-400 mb-2">Manba turi</label>
            <div className="flex rounded-lg border border-brand-border overflow-hidden">
              <button
                type="button"
                onClick={() => { setSourceType("direct"); setVideoUrl(""); }}
                className={`flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-medium transition-colors ${
                  sourceType === "direct"
                    ? "bg-brand-red text-white"
                    : "text-gray-400 hover:text-white hover:bg-white/5"
                }`}
              >
                <Link className="w-4 h-4" />
                To&apos;g&apos;ridan URL
              </button>
              <button
                type="button"
                onClick={() => { setSourceType("youtube"); setVideoUrl(""); }}
                className={`flex-1 flex items-center justify-center gap-2 py-2.5 text-sm font-medium transition-colors ${
                  sourceType === "youtube"
                    ? "bg-red-600 text-white"
                    : "text-gray-400 hover:text-white hover:bg-white/5"
                }`}
              >
                <Youtube className="w-4 h-4" />
                YouTube URL
              </button>
            </div>
          </div>

          {/* URL field */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">
              {sourceType === "youtube" ? "YouTube URL" : "Video URL"}{" "}
              <span className="text-red-500">*</span>
            </label>
            <input
              type="url"
              value={videoUrl}
              onChange={(e) => setVideoUrl(e.target.value)}
              placeholder={
                sourceType === "youtube"
                  ? "https://www.youtube.com/watch?v=... yoki https://youtu.be/..."
                  : "https://example.com/video.mp4"
              }
              required
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
            {sourceType === "youtube" && (
              <p className="text-xs text-gray-600 mt-1">
                youtube.com/watch?v=, youtube.com/shorts/ va youtu.be/ manzillarini qo&apos;llab-quvvatlaydi
              </p>
            )}
          </div>

          {/* Type */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">Type</label>
            <div className="flex gap-4">
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="type"
                  value="movie"
                  checked={type === "movie"}
                  onChange={() => setType("movie")}
                  className="text-brand-red focus:ring-brand-red"
                />
                <span className="text-white flex items-center gap-1">
                  <Film className="w-4 h-4" /> Movie
                </span>
              </label>
              <label className="flex items-center gap-2 cursor-pointer">
                <input
                  type="radio"
                  name="type"
                  value="serial"
                  checked={type === "serial"}
                  onChange={() => setType("serial")}
                  className="text-brand-red focus:ring-brand-red"
                />
                <span className="text-white flex items-center gap-1">
                  <Tv className="w-4 h-4" /> Serial
                </span>
              </label>
            </div>
          </div>

          {/* Title - Optional */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">Title (optional)</label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              placeholder="Movie or series title"
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
          </div>

          {/* Year - Optional */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">Year (optional)</label>
            <input
              type="number"
              value={year}
              onChange={(e) => setYear(e.target.value)}
              placeholder="2024"
              min="1900"
              max="2099"
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
          </div>

          {/* Poster URL - Optional */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">Poster URL (optional)</label>
            <input
              type="url"
              value={poster}
              onChange={(e) => setPoster(e.target.value)}
              placeholder="https://example.com/poster.jpg"
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
          </div>

          {/* Backdrop URL - Optional */}
          <div>
            <label className="block text-sm text-gray-400 mb-1">Backdrop URL (optional)</label>
            <input
              type="url"
              value={backdrop}
              onChange={(e) => setBackdrop(e.target.value)}
              placeholder="https://example.com/backdrop.jpg"
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-4 py-3 text-white placeholder-gray-500 focus:outline-none focus:border-brand-red"
            />
          </div>

          {/* Submit */}
          <button
            type="submit"
            disabled={submitting || !videoUrl.trim()}
            className="w-full bg-brand-red hover:bg-orange-700 disabled:opacity-60 py-3 rounded-lg text-white font-semibold transition-colors flex items-center justify-center gap-2"
          >
            {submitting ? (
              <>
                <Loader2 className="w-5 h-5 animate-spin" />
                Creating Job...
              </>
            ) : (
              <>
                <Plus className="w-5 h-5" />
                Create Import Job
              </>
            )}
          </button>
        </form>
      </div>
    </div>
  );
}

function getSerialSummary(jobs: IngestionJob[]) {
  const completed = jobs.filter((job) => job.status === "completed").length;
  const failed = jobs.filter((job) => job.status === "failed" || job.status === "download_failed").length;
  const processing = jobs.filter((job) => {
    const displayStatus = getJobDisplayStatus(job);
    return displayStatus === "downloading" || displayStatus === "processing" || displayStatus === "uploading" || displayStatus === "hls_processing" || displayStatus === "finalizing_storage";
  }).length;
  const overallProgress = jobs.length === 0
    ? 0
    : normalizePercent(jobs.reduce((sum, job) => sum + getJobProgress(job), 0) / jobs.length);

  return {
    completed,
    failed,
    processing,
    overallProgress,
    totalEpisodes: jobs.length,
    totalSeasons: new Set(jobs.map((job) => job.season_number || 1)).size,
  };
}

// Jobs Tab Component
function JobsTab({
  jobs,
  loadingJobs,
  retryingStage,
  handleRetry,
  currentFilter,
  handleFilterChange,
  currentPage,
  totalPages,
  totalJobs,
  handlePageChange
}: {
  jobs: IngestionJob[];
  loadingJobs: boolean;
  retryingStage: {jobId: string; stage: string} | null;
  handleRetry: (jobId: string, stage: "download" | "process" | "upload") => void;
  currentFilter: JobFilter;
  handleFilterChange: (filter: JobFilter) => void;
  currentPage: number;
  totalPages: number;
  totalJobs: number;
  handlePageChange: (page: number) => void;
}) {
  const [now, setNow] = useState(() => Date.now());
  const [expandedLogs, setExpandedLogs] = useState<Set<string>>(new Set());
  const [expandedSerials, setExpandedSerials] = useState<Set<string>>(new Set());
  const [expandedSeasons, setExpandedSeasons] = useState<Set<string>>(new Set());
  const hasRunningJobs = jobs.some((job) => !isTerminalJobStatus(job.status));
  const summary = getJobSummary(jobs, now);

  useEffect(() => {
    if (!hasRunningJobs) return;
    const interval = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [hasRunningJobs]);

  const toggleLogs = useCallback((jobId: string) => {
    setExpandedLogs((prev) => {
      const next = new Set(prev);
      if (next.has(jobId)) next.delete(jobId);
      else next.add(jobId);
      return next;
    });
  }, []);

  const toggleSerial = useCallback((serialId: string) => {
    setExpandedSerials((prev) => {
      const next = new Set(prev);
      if (next.has(serialId)) next.delete(serialId);
      else next.add(serialId);
      return next;
    });
  }, []);

  const toggleSeason = useCallback((seasonKey: string) => {
    setExpandedSeasons((prev) => {
      const next = new Set(prev);
      if (next.has(seasonKey)) next.delete(seasonKey);
      else next.add(seasonKey);
      return next;
    });
  }, []);

  const jobGroups = useMemo(() => groupJobsBySerial(jobs), [jobs]);

  const renderJobCard = useCallback((job: IngestionJob, compact = false) => {
    if (!job.id) return null;

    const safeJob = {
      id: getStableJobKey(job),
      status: job.status || "unknown",
      stage: job.stage,
      progress: getJobProgress(job),
      source: job.source || "unknown",
      source_id: job.source_id || "",
      metadata: job.metadata,
      downloaded_bytes: typeof job.downloaded_bytes === "number" ? job.downloaded_bytes : 0,
      total_bytes: typeof job.total_bytes === "number" ? job.total_bytes : 0,
      speed_mbps: typeof job.speed_mbps === "number" ? job.speed_mbps : 0,
      eta_seconds: typeof job.eta_seconds === "number" ? job.eta_seconds : 0,
      output_path: job.output_path,
      playlist_path: job.playlist_path,
      local_path: job.local_path,
      source_file_deleted: job.source_file_deleted,
      retry_count: typeof job.retry_count === "number" ? job.retry_count : 0,
      error: job.error,
      message: job.message,
      logs: Array.isArray(job.logs) ? job.logs : [],
      created_at: job.created_at || new Date().toISOString(),
      updated_at: job.updated_at,
      completed_at: job.completed_at,
      episode_number: job.episode_number,
    };

    const elapsedInfo = computeElapsed(job, now);
    const elapsedTime = elapsedInfo.text;
    const elapsedLabel = elapsedInfo.kind === "queue"
      ? "Process navbatida"
      : elapsedInfo.kind === "download"
        ? "Download"
        : elapsedInfo.kind === "processing"
          ? "Processing"
          : "Elapsed";
    const lastUpdateAge = getLastUpdateAgeMs(job, now);
    const lastUpdateText = formatDurationShort(lastUpdateAge);
    const stuck = isStuckJob(job, now);
    const logsExpanded = expandedLogs.has(safeJob.id);
    const activeStage = safeJob.stage || safeJob.status;
    const badgeStatus = STAGE_STATUS_MAP[activeStage] || safeJob.status;
    const displayStatus = safeJob.stage ? STAGE_STATUS_MAP[safeJob.stage] || safeJob.status : badgeStatus;
    const statusConfig = getStatusMeta(displayStatus);
    const StatusIcon = statusConfig.icon;
    const episodeInfo = safeJob.episode_number ? ` • E${safeJob.episode_number}` : "";
    const isFailed = safeJob.status === "failed" || safeJob.status === "download_failed";
    const displayProgress = safeJob.progress;
    const statusSummary = getJobStatusSummary(job);
    const showInlineTelemetry = displayStatus === "downloading" && safeJob.downloaded_bytes > 0;
    const waitingForProcessing = isWaitingForProcessing(job);
    const showAuxMessage = Boolean(
      safeJob.message &&
      safeJob.status !== "completed" &&
      !isProgressStage(displayStatus) &&
      safeJob.message.trim() !== statusSummary.trim()
    );

    if (compact) {
      return (
        <div
          key={safeJob.id}
          className="bg-brand-card border border-brand-border rounded-lg p-3 ml-6"
        >
          <div className="flex items-center gap-3">
            <div className={`p-1.5 rounded-full ${statusConfig.color} bg-opacity-20`}>
              <StatusIcon className={`w-4 h-4 ${activeStage === "parsing" || activeStage === "downloading" || activeStage === "download" ? "animate-spin" : ""}`} />
            </div>
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <h4 className="font-medium text-white text-sm truncate">
                  {safeJob.metadata?.title || safeJob.source_id}{episodeInfo}
                </h4>
                <span className={`px-1.5 py-0.5 rounded text-xs ${statusConfig.color} text-white shrink-0`}>
                  {statusConfig.label}
                </span>
              </div>
              <p className="text-xs text-gray-500">
                {statusSummary}
              </p>
            </div>
            <div className="w-32">
              <div className="flex justify-between text-xs text-gray-400 mb-1">
                <span className="capitalize truncate">{activeStage}</span>
                <span>{displayProgress}%</span>
              </div>
              <div className="h-1.5 bg-brand-border rounded-full overflow-hidden">
                <div
                  className={`h-full ${statusConfig.color} transition-all duration-300`}
                  style={{ width: `${displayProgress}%` }}
                />
              </div>
            </div>
            <div className="flex gap-1">
              <button
                onClick={() => handleRetry(safeJob.id, "download")}
                disabled={retryingStage?.jobId === safeJob.id}
                className="p-1.5 bg-brand-dark hover:bg-gray-700 rounded transition-colors disabled:opacity-50"
                title="Retry Download"
              >
                {retryingStage?.jobId === safeJob.id && retryingStage?.stage === "download" ? (
                  <Loader2 className="w-3 h-3 text-yellow-400 animate-spin" />
                ) : (
                  <Download className="w-3 h-3 text-yellow-400" />
                )}
              </button>
              <button
                onClick={() => handleRetry(safeJob.id, "process")}
                disabled={retryingStage?.jobId === safeJob.id}
                className="p-1.5 bg-brand-dark hover:bg-gray-700 rounded transition-colors disabled:opacity-50"
                title="Retry Processing"
              >
                {retryingStage?.jobId === safeJob.id && retryingStage?.stage === "process" ? (
                  <Loader2 className="w-3 h-3 text-purple-400 animate-spin" />
                ) : (
                  <Settings className="w-3 h-3 text-purple-400" />
                )}
              </button>
            </div>
          </div>
          {safeJob.error && safeJob.status !== "downloading" && activeStage !== "downloading" && (
            <div className="mt-2 flex items-start gap-2 text-red-400 text-xs">
              <AlertTriangle className="w-3 h-3 mt-0.5" />
              {safeJob.error}
            </div>
          )}
        </div>
      );
    }

    return (
      <div
        key={safeJob.id}
        className="bg-brand-card border border-brand-border rounded-lg p-4"
      >
        <div className="flex items-center gap-4">
          <div className={`p-2 rounded-full ${statusConfig.color} bg-opacity-20`}>
            <StatusIcon className={`w-5 h-5 ${activeStage === "parsing" || activeStage === "downloading" || activeStage === "download" ? "animate-spin" : ""}`} />
          </div>
          <div className="flex-1">
            <div className="flex items-center gap-2">
              <h3 className="font-semibold text-white">
                {safeJob.metadata?.title || safeJob.source_id}
              </h3>
              <span className={`px-2 py-0.5 rounded text-xs ${statusConfig.color} text-white`}>
                {statusConfig.label}
              </span>
            </div>
            <p className="text-sm text-gray-400">
              {statusSummary}
            </p>
            <p className="text-xs text-gray-500">
              {elapsedLabel}: {elapsedTime} • Last update: {lastUpdateText} ago • Retry: {safeJob.retry_count}
            </p>
            {job.content_type === "serial_parent" && (
              <p className="mt-1 text-xs text-blue-300">
                Serial import • seasons: {job.seasons_count ?? 0} •
                {" "}episodes: {job.episode_count ?? 0} •
                {" "}new jobs: {job.child_jobs_created ?? 0}
              </p>
            )}
            {job.content_type === "serial_parent" && job.status === "failed" && job.error && (
              <p className="mt-1 flex items-start gap-1 text-xs text-red-400">
                <AlertTriangle className="w-3 h-3 shrink-0 mt-0.5" />
                <span className="break-words">{job.error}</span>
              </p>
            )}
            {stuck && (
              <p className="mt-1 flex items-center gap-1 text-xs text-orange-400">
                <AlertTriangle className="w-3 h-3" />
                No update for {lastUpdateText}
              </p>
            )}
            {waitingForProcessing && (
              <p className="mt-1 flex items-center gap-1 text-xs text-amber-400">
                <AlertTriangle className="w-3 h-3" />
                Download complete but waiting for processing
              </p>
            )}
          </div>
          <div className="w-64">
            <div className="flex justify-between text-xs text-gray-400 mb-1">
              <span className="capitalize">{activeStage}</span>
              <span>{displayProgress}%</span>
            </div>
            <div className="h-2 bg-brand-border rounded-full overflow-hidden">
              <div
                className={`h-full ${statusConfig.color} transition-all duration-300`}
                style={{ width: `${displayProgress}%` }}
              />
            </div>
            {showInlineTelemetry && (
              <div className="mt-1 text-xs text-gray-500 flex flex-wrap gap-x-2">
                <span>
                  {safeJob.total_bytes > 0
                    ? `${formatBytes(safeJob.downloaded_bytes)} / ${formatBytes(safeJob.total_bytes)}`
                    : formatBytes(safeJob.downloaded_bytes)}
                </span>
                {safeJob.speed_mbps > 0 && (
                  <>
                    <span>•</span>
                    <span>{formatSpeed(safeJob.speed_mbps)}</span>
                  </>
                )}
                {safeJob.eta_seconds > 0 && (
                  <>
                    <span>•</span>
                    <span>ETA {formatEta(safeJob.eta_seconds)}</span>
                  </>
                )}
              </div>
            )}
            {showAuxMessage && (
              <div className="mt-1 text-xs text-gray-500 truncate">
                {safeJob.message}
              </div>
            )}
            {(safeJob.output_path || safeJob.playlist_path) && (
              <div className="mt-2 flex items-center gap-2">
                <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-purple-900 text-purple-200">
                  HLS Ready
                </span>
                <span className="text-xs text-gray-400 truncate" title={safeJob.playlist_path || safeJob.output_path}>
                  📁 {safeJob.playlist_path || safeJob.output_path}
                </span>
              </div>
            )}
            {safeJob.local_path && !safeJob.source_file_deleted && (
              <div className="mt-2 flex items-center gap-2">
                <span className="inline-flex items-center px-2 py-0.5 rounded text-xs font-medium bg-green-900 text-green-200">
                  Downloaded
                </span>
                <span className="text-xs text-gray-400 truncate" title={safeJob.local_path}>
                  📁 {safeJob.local_path}
                </span>
              </div>
            )}
          </div>
          <div className="flex gap-1">
            <button
              onClick={() => handleRetry(safeJob.id, "download")}
              disabled={retryingStage?.jobId === safeJob.id}
              className="p-2 bg-brand-card hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
              title="Retry Download"
            >
              {retryingStage?.jobId === safeJob.id && retryingStage?.stage === "download" ? (
                <Loader2 className="w-4 h-4 text-yellow-400 animate-spin" />
              ) : (
                <Download className="w-4 h-4 text-yellow-400" />
              )}
            </button>
            <button
              onClick={() => handleRetry(safeJob.id, "process")}
              disabled={retryingStage?.jobId === safeJob.id}
              className="p-2 bg-brand-card hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
              title="Retry Processing"
            >
              {retryingStage?.jobId === safeJob.id && retryingStage?.stage === "process" ? (
                <Loader2 className="w-4 h-4 text-purple-400 animate-spin" />
              ) : (
                <Settings className="w-4 h-4 text-purple-400" />
              )}
            </button>
            <button
              onClick={() => handleRetry(safeJob.id, "upload")}
              disabled={retryingStage?.jobId === safeJob.id}
              className="p-2 bg-brand-card hover:bg-gray-700 rounded-lg transition-colors disabled:opacity-50"
              title="Retry Upload"
            >
              {retryingStage?.jobId === safeJob.id && retryingStage?.stage === "upload" ? (
                <Loader2 className="w-4 h-4 text-indigo-400 animate-spin" />
              ) : (
                <Upload className="w-4 h-4 text-indigo-400" />
              )}
            </button>
          </div>
        </div>
        {safeJob.error && safeJob.status !== "downloading" && activeStage !== "downloading" && (
          <div className="mt-3 flex items-start gap-2 text-red-400 text-sm">
            <AlertTriangle className="w-4 h-4 mt-0.5" />
            {safeJob.error}
          </div>
        )}
        {safeJob.logs.length > 0 && safeJob.status !== "completed" && (
          <div className="mt-3 text-xs text-gray-500 font-mono">
            <div className="flex items-center justify-between mb-1">
              <span>Logs ({safeJob.logs.length})</span>
              <button
                onClick={() => toggleLogs(safeJob.id)}
                className="text-gray-400 hover:text-white transition-colors"
              >
                {logsExpanded ? "Hide logs" : "Show logs"}
              </button>
            </div>
            {(logsExpanded ? safeJob.logs : safeJob.logs.slice(-3)).map((log, i) => {
              const relativeTime = formatLogTime(log.timestamp, now);
              return (
                <div key={`${log.timestamp || "log"}-${i}`} className="truncate">
                  <span className="text-gray-600">{relativeTime}</span>
                  {relativeTime && <span> • </span>}
                  <span>{log.message}</span>
                </div>
              );
            })}
          </div>
        )}
      </div>
    );
  }, [expandedLogs, handleRetry, now, retryingStage, toggleLogs]);

  return (
    <div className="space-y-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="text-sm text-gray-400">
          Page {currentPage} of {Math.max(totalPages, 1)} • {totalJobs} top-level item{totalJobs === 1 ? "" : "s"}
        </div>
        <div className="flex flex-wrap gap-2 text-xs text-gray-500">
          <span className="rounded-md border border-brand-border bg-brand-card px-2 py-1">Active: {summary.active}</span>
          <span className="rounded-md border border-brand-border bg-brand-card px-2 py-1">Pending: {summary.pending}</span>
          <span className="rounded-md border border-brand-border bg-brand-card px-2 py-1">Processing: {summary.processing}</span>
          <span className="rounded-md border border-brand-border bg-brand-card px-2 py-1">Failed: {summary.failed}</span>
          <span className="rounded-md border border-brand-border bg-brand-card px-2 py-1">Stuck: {summary.stuck}</span>
        </div>
      </div>

      <div className="flex flex-wrap gap-2">
        {JOB_FILTERS.map((item) => (
          <button
            key={item.id}
            onClick={() => handleFilterChange(item.id)}
            className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
              currentFilter === item.id
                ? "bg-brand-red text-white border-brand-red"
                : "bg-brand-card text-gray-400 border-brand-border hover:text-white"
            }`}
          >
            {item.label}
          </button>
        ))}
      </div>

      {loadingJobs && jobs.length === 0 ? (
        <div className="flex items-center justify-center py-12">
          <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
          <span className="ml-3 text-gray-400">Loading jobs...</span>
        </div>
      ) : jobGroups.length === 0 ? (
        <div className="text-center text-gray-500 py-12">
          {totalJobs === 0 ? "No ingestion jobs yet. Import movies from sources above to get started." : "No jobs match this filter."}
        </div>
      ) : (
        <div className="space-y-4">
          {jobGroups.map((group) => {
            if (group.type === "single") {
              return renderJobCard(group.job);
            }

            const serial = group;
            const isExpanded = expandedSerials.has(serial.id);
            const allEpisodes = serial.seasons.flatMap((season) => season.episodes);
            const serialSummary = getSerialSummary(allEpisodes);
            const hasActive = allEpisodes.some((job) => !isTerminalJobStatus(job.status));
            const hasFailed = allEpisodes.some((job) => job.status === "failed" || job.status === "download_failed");
            const statusColor = hasFailed ? "bg-red-500" : hasActive ? "bg-yellow-500" : "bg-green-500";
            const statusLabel = hasFailed ? "Failed" : hasActive ? "Processing" : "Completed";
            const serialElapsed = formatSerialElapsedTime(allEpisodes, now);

            return (
              <div key={serial.id} className="bg-brand-card border border-brand-border rounded-lg overflow-hidden">
                <div
                  className="p-4 flex items-center gap-4 cursor-pointer hover:bg-brand-dark/50 transition-colors"
                  onClick={() => toggleSerial(serial.id)}
                >
                  <ChevronDown className={`w-5 h-5 text-gray-400 transition-transform ${isExpanded ? "" : "-rotate-90"}`} />
                  <div className={`p-2 rounded-full ${statusColor} bg-opacity-20`}>
                    <Tv className="w-5 h-5" />
                  </div>
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <h3 className="font-semibold text-white">{serial.title}</h3>
                      <span className={`px-2 py-0.5 rounded text-xs text-white ${statusColor}`}>
                        {statusLabel}
                      </span>
                    </div>
                    <p className="text-sm text-gray-400">
                      {serialSummary.totalSeasons} season{serialSummary.totalSeasons !== 1 ? "s" : ""} • {serialSummary.totalEpisodes} episode{serialSummary.totalEpisodes !== 1 ? "s" : ""}
                    </p>
                    <p className="text-xs text-gray-500">
                      Elapsed: {serialElapsed}
                    </p>
                  </div>
                  <div className="flex items-center gap-4 text-xs text-gray-500">
                    <span className="flex items-center gap-1">
                      <CheckCircle className="w-3 h-3 text-green-400" />
                      {serialSummary.completed}
                    </span>
                    <span className="flex items-center gap-1">
                      <Loader2 className="w-3 h-3 text-yellow-400" />
                      {serialSummary.processing}
                    </span>
                    <span className="flex items-center gap-1">
                      <XCircle className="w-3 h-3 text-red-400" />
                      {serialSummary.failed}
                    </span>
                  </div>
                  <div className="w-32">
                    <div className="flex justify-between text-xs text-gray-400 mb-1">
                      <span>Progress</span>
                      <span>{serialSummary.overallProgress}%</span>
                    </div>
                    <div className="h-2 bg-brand-border rounded-full overflow-hidden">
                      <div
                        className={`h-full ${statusColor} transition-all duration-300`}
                        style={{ width: `${serialSummary.overallProgress}%` }}
                      />
                    </div>
                  </div>
                </div>

                {isExpanded && (
                  <div className="border-t border-brand-border bg-brand-dark/30">
                    {serial.seasons.map((season) => {
                      const seasonName = `S${season.season_number}`;
                      const seasonKey = `${serial.id}-${seasonName}`;
                      const seasonExpanded = expandedSeasons.has(seasonKey);
                      const seasonCompleted = season.episodes.filter((job) => job.status === "completed").length;

                      return (
                        <div key={seasonName} className="border-b border-brand-border last:border-b-0">
                          <div
                            className="p-3 ml-6 flex items-center gap-3 cursor-pointer hover:bg-brand-dark/50 transition-colors"
                            onClick={() => toggleSeason(seasonKey)}
                          >
                            <ChevronDown className={`w-4 h-4 text-gray-500 transition-transform ${seasonExpanded ? "" : "-rotate-90"}`} />
                            <Tv className="w-4 h-4 text-blue-400" />
                            <span className="font-medium text-white text-sm">{seasonName}</span>
                            <span className="text-xs text-gray-500">
                              {season.episodes.length} episode{season.episodes.length !== 1 ? "s" : ""}
                            </span>
                            <span className="text-xs text-gray-500 ml-auto">
                              {seasonCompleted}/{season.episodes.length} done
                            </span>
                          </div>
                          {seasonExpanded && (
                            <div className="pb-2">
                              {season.episodes.map((job) => renderJobCard(job, true))}
                            </div>
                          )}
                        </div>
                      );
                    })}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      {totalPages > 1 && (
        <div className="flex items-center justify-between mt-6">
          <div className="text-sm text-gray-400">
            Showing {jobGroups.length} item{jobGroups.length === 1 ? "" : "s"} on this page
          </div>
          <div className="flex items-center gap-2">
            <button
              onClick={() => handlePageChange(currentPage - 1)}
              disabled={currentPage <= 1}
              className="px-3 py-1.5 rounded-lg text-sm border border-brand-border bg-brand-card text-gray-400 hover:text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-1"
            >
              <ChevronLeft className="w-4 h-4" />
              Previous
            </button>

            {Array.from({ length: Math.min(5, totalPages) }, (_, i) => {
              const pageNum = Math.max(1, Math.min(totalPages - 4, currentPage - 2)) + i;
              if (pageNum > totalPages) return null;
              return (
                <button
                  key={pageNum}
                  onClick={() => handlePageChange(pageNum)}
                  className={`px-3 py-1.5 rounded-lg text-sm border transition-colors ${
                    pageNum === currentPage
                      ? "bg-brand-red text-white border-brand-red"
                      : "border-brand-border bg-brand-card text-gray-400 hover:text-white"
                  }`}
                >
                  {pageNum}
                </button>
              );
            })}

            <button
              onClick={() => handlePageChange(currentPage + 1)}
              disabled={currentPage >= totalPages}
              className="px-3 py-1.5 rounded-lg text-sm border border-brand-border bg-brand-card text-gray-400 hover:text-white disabled:opacity-50 disabled:cursor-not-allowed transition-colors flex items-center gap-1"
            >
              Next
              <ChevronRight className="w-4 h-4" />
            </button>
          </div>
        </div>
      )}
    </div>
  );
}

// Main Page Component
export default function IngestionPage() {
  const { token } = useAuth();
  const [activeTab, setActiveTab] = useState<string>("uzmovi");
  const [jobs, setJobs] = useState<IngestionJob[]>([]);
  const [loadingJobs, setLoadingJobs] = useState(false);
  // Pagination state
  const [currentPage, setCurrentPage] = useState<number>(1);
  const [totalPages, setTotalPages] = useState<number>(0);
  const [totalJobs, setTotalJobs] = useState<number>(0);
  const [pageSize] = useState<number>(30); // Default page size
  const [currentFilter, setCurrentFilter] = useState<JobFilter>("all");
  const [retryingStage, setRetryingStage] = useState<{jobId: string; stage: string} | null>(null);
  const [syncEnabled, setSyncEnabled] = useState<boolean>(false);
  const [isFetching, setIsFetching] = useState<boolean>(false);
  const [hasInitialized, setHasInitialized] = useState<boolean>(false);
  const isMounted = useRef(true);

  // Keep ref to latest fetchJobs to avoid stale closures in effects
  const fetchJobsRef = useRef<any>(null);

  // Handle page changes
  const handlePageChange = useCallback((page: number) => {
    setCurrentPage(page);
    if (fetchJobsRef.current) fetchJobsRef.current(page, currentFilter);
  }, [currentFilter]);

  // Handle filter changes
  const handleFilterChange = useCallback((filter: JobFilter) => {
    setCurrentFilter(filter);
    setCurrentPage(1); // Reset to first page when filter changes
    if (fetchJobsRef.current) fetchJobsRef.current(1, filter);
  }, []);

  const fetchJobs = useCallback(async (page: number = currentPage, status: JobFilter = currentFilter) => {
    if (!token || !isMounted.current || isFetching) return;

    setIsFetching(true);
    setLoadingJobs(true);
    try {
      const result = await getIngestionJobs(token, {
        page,
        limit: pageSize,
        status,
      });

      const jobsData = Array.isArray(result.data) ? result.data : [];
      setTotalJobs(result.total || 0);
      setTotalPages(result.totalPages || 0);
      setCurrentPage(result.page || page);
      setJobs(jobsData);
    } catch (err) {
      console.error("Failed to fetch jobs:", err);
      setJobs((prev) => (Array.isArray(prev) ? prev : []));
    } finally {
      if (isMounted.current) {
        setIsFetching(false);
        setLoadingJobs(false);
      }
    }
  }, [currentFilter, currentPage, token, isFetching, pageSize]);

  // Update fetchJobsRef
  useEffect(() => {
    fetchJobsRef.current = fetchJobs;
  }, [fetchJobs]);

  // Initialize sync state from localStorage
  useEffect(() => {
    const saved = localStorage.getItem("ingestion-sync-enabled");
    if (saved !== null) {
      setSyncEnabled(saved === "true");
      setHasInitialized(true);
    } else {
      setHasInitialized(false);
    }
  }, []);

  // Initial fetch when token becomes available
  useEffect(() => {
    if (token) {
      fetchJobsRef.current();
    }
  }, [token]); // Only token, uses ref to get latest fetchJobs

  // Polling effect - only sets up interval when sync is ON and on Jobs tab
  useEffect(() => {
    if (!hasInitialized || !token || activeTab !== "jobs" || !syncEnabled) {
      return;
    }

    // Immediate fetch when sync is enabled and viewing jobs
    fetchJobsRef.current();

    const interval = setInterval(() => {
      fetchJobsRef.current();
    }, 1000);

    return () => clearInterval(interval);
  }, [hasInitialized, token, activeTab, syncEnabled]); // fetchJobsRef is stable

  // Cleanup on unmount
  useEffect(() => {
    return () => {
      isMounted.current = false;
    };
  }, []);

  const handleRetry = async (jobId: string, stage: "download" | "process" | "upload" = "download") => {
    if (!token) return;
    
    setRetryingStage({ jobId, stage });
    try {
      await retryIngestionJob(token, jobId, stage);
    } catch (err) {
      console.error("Retry failed:", err);
    } finally {
      setRetryingStage(null);
    }
  };

  const handleImportSuccess = () => {
    setCurrentFilter("all");
    setCurrentPage(1);
    setActiveTab("jobs");
    if (fetchJobsRef.current) {
      fetchJobsRef.current(1, "all");
    }
  };

  if (!token) {
    return (
      <div className="min-h-screen bg-brand-dark flex items-center justify-center">
        <Loader2 className="w-8 h-8 animate-spin text-brand-red" />
      </div>
    );
  }

  const activeJobCount = Array.isArray(jobs) ? jobs.filter(j => !isTerminalJobStatus(j.status)).length : 0;

  return (
    <div className="min-h-screen bg-brand-dark text-white">
      <div className="max-w-7xl mx-auto px-4 py-8">
        <h1 className="text-3xl font-display mb-8">Import Movies</h1>

        {/* Source Tabs */}
        <div className="flex gap-2 mb-6 flex-wrap border-b border-brand-border pb-4">
          {SOURCES.map((source) => {
            const Icon = source.icon;
            return (
              <button
                key={source.id}
                onClick={() => setActiveTab(source.id)}
                className={`px-4 py-2 rounded-lg transition-colors flex items-center gap-2 ${
                  activeTab === source.id 
                    ? "bg-brand-red text-white" 
                    : "bg-brand-card text-gray-400 hover:text-white border border-brand-border"
                }`}
              >
                <Icon className="w-4 h-4" />
                {source.name}
                {source.url && <span className="text-xs opacity-60">{source.url}</span>}
              </button>
            );
          })}
          
          {/* Jobs tab */}
          <button
            onClick={() => setActiveTab("jobs")}
            className={`px-4 py-2 rounded-lg transition-colors flex items-center gap-2 ${
              activeTab === "jobs" 
                ? "bg-brand-red text-white" 
                : "bg-brand-card text-gray-400 hover:text-white border border-brand-border"
            }`}
          >
            <Clock className="w-4 h-4" />
            Jobs
            {activeJobCount > 0 && (
              <span className="bg-yellow-500 text-black text-xs font-bold px-2 py-0.5 rounded-full">
                {activeJobCount}
              </span>
            )}
          </button>
          
          {/* Sync toggle button (only show on jobs tab) */}
          {activeTab === "jobs" && (
            <button
              onClick={() => {
                const newState = !syncEnabled;
                setSyncEnabled(newState);
                localStorage.setItem("ingestion-sync-enabled", String(newState));
              }}
              className={`px-4 py-2 rounded-lg transition-colors flex items-center gap-2 border ${
                syncEnabled
                  ? "bg-green-600 text-white border-green-500 hover:bg-green-700"
                  : "bg-brand-card text-gray-400 hover:text-white border-brand-border"
              }`}
              title={syncEnabled ? "Auto-refresh ON" : "Auto-refresh OFF"}
            >
              <Power className={`w-4 h-4 ${syncEnabled ? "fill-current" : ""}`} />
              Sync {syncEnabled ? "ON" : "OFF"}
            </button>
          )}
        </div>

        {/* Source Content */}
        {activeTab === "uzmovi" && (
          <CatalogTab 
            source={SOURCES[0]} 
            token={token} 
            onImportSuccess={handleImportSuccess}
          />
        )}
        {activeTab === "freekino" && (
          <CatalogTab 
            source={SOURCES[1]} 
            token={token} 
            onImportSuccess={handleImportSuccess}
          />
        )}
        {activeTab === "asilmedia" && (
          <CatalogTab 
            source={SOURCES[2]} 
            token={token} 
            onImportSuccess={handleImportSuccess}
          />
        )}
        {activeTab === "manual" && (
          <ManualTab 
            token={token} 
            onImportSuccess={handleImportSuccess}
          />
        )}
        {activeTab === "jobs" && (
          <JobsTab
            jobs={jobs}
            loadingJobs={loadingJobs}
            retryingStage={retryingStage}
            handleRetry={handleRetry}
            currentFilter={currentFilter}
            handleFilterChange={handleFilterChange}
            currentPage={currentPage}
            totalPages={totalPages}
            totalJobs={totalJobs}
            handlePageChange={handlePageChange}
          />
        )}
      </div>
    </div>
  );
}
