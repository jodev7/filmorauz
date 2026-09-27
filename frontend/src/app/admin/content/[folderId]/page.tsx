"use client";

import { useEffect, useState, useMemo, useCallback, useRef } from "react";
import Link from "next/link";
import { useParams } from "next/navigation";
import {
  ArrowLeft,
  Plus,
  Upload,
  Trash2,
  Send,
  Clock,
  CheckCircle2,
  XCircle,
  Loader2,
  X,
  Video,
  Calendar,
  Instagram,
  Youtube,
  Copy,
  ExternalLink,
  Folder,
  Music2,
  Zap,
  FileVideo,
} from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  adminGetContentFolder,
  adminListContentClips,
  adminUploadContentClip,
  adminDeleteContentClip,
  adminGetPublishAccounts,
  adminContentPublishNow,
  adminContentPublishSchedule,
  adminContentListJobsForFolder,
  adminCancelPublishJob,
  ContentFolder,
  ContentClip,
  ContentPublishJob,
  PublishAccountsResponse,
  PublishTarget,
} from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import { useToast } from "@/components/admin/Toast";
import { Chip, EmptyState, GhostButton, IconBtn, Modal, PrimaryButton, SkeletonList, StatTile, Tabs, Thumb } from "@/components/admin/kit";
import { ErrorBanner, Field, Segmented, inputCls } from "@/components/admin/form/ui";

// ─── Helpers ─────────────────────────────────────────────────────────────

const PLATFORM_STYLE: Record<string, string> = {
  instagram: "bg-gradient-to-br from-pink-500 to-amber-500",
  youtube: "bg-red-600",
  tiktok: "bg-gradient-to-br from-cyan-400 to-fuchsia-500",
};

function platformIcon(p: string, size = 14) {
  if (p === "instagram") return <Instagram size={size} />;
  if (p === "youtube") return <Youtube size={size} />;
  if (p === "tiktok") return <Music2 size={size} />;
  return <Send size={size} />;
}

function PlatformBadge({ platform, size = 22 }: { platform: string; size?: number }) {
  return (
    <span
      className={`inline-flex shrink-0 items-center justify-center rounded-md text-white ${PLATFORM_STYLE[platform] || "bg-gray-600"}`}
      style={{ width: size, height: size }}
    >
      {platformIcon(platform, Math.round(size * 0.55))}
    </span>
  );
}

function formatDateTime(iso: string): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (isNaN(d.getTime())) return iso;
  return d.toLocaleString("uz-UZ", {
    year: "numeric",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function localDateTimeToRFC3339(local: string): string {
  // input type="datetime-local" returns "2026-05-21T14:30"
  const d = new Date(local);
  return d.toISOString();
}

type ClipFilter = "all" | "unpublished" | "scheduled" | "published" | "failed";

// ─── Page ────────────────────────────────────────────────────────────────

export default function FolderDetailPage() {
  const { token } = useAuth();
  const toast = useToast();
  const params = useParams();
  const folderId = params.folderId as string;

  const [folder, setFolder] = useState<ContentFolder | null>(null);
  const [clips, setClips] = useState<ContentClip[]>([]);
  const [jobs, setJobs] = useState<ContentPublishJob[]>([]);
  const [accounts, setAccounts] = useState<PublishAccountsResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [filter, setFilter] = useState<ClipFilter>("all");

  const [showUpload, setShowUpload] = useState(false);
  const [publishFor, setPublishFor] = useState<ContentClip | null>(null);

  const refreshAll = useCallback(async () => {
    if (!token || !folderId) return;
    try {
      const [f, c, j] = await Promise.all([
        adminGetContentFolder(token, folderId),
        adminListContentClips(token, folderId),
        adminContentListJobsForFolder(token, folderId),
      ]);
      setFolder(f);
      setClips(c || []);
      setJobs(j || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  }, [token, folderId]);

  // Initial load + accounts
  useEffect(() => {
    void refreshAll();
    if (token) {
      adminGetPublishAccounts(token).then(setAccounts).catch(() => {});
    }
  }, [refreshAll, token]);

  // Poll jobs every 15s to surface scheduled → success/failed transitions
  useEffect(() => {
    if (!token || !folderId) return;
    const t = setInterval(() => {
      adminContentListJobsForFolder(token, folderId)
        .then((j) => setJobs(j || []))
        .catch(() => {});
    }, 15_000);
    return () => clearInterval(t);
  }, [token, folderId]);

  // Group jobs by clip for fast lookup in render
  const jobsByClip = useMemo(() => {
    const m = new Map<string, ContentPublishJob[]>();
    for (const j of jobs) {
      const arr = m.get(j.content_clip_id) || [];
      arr.push(j);
      m.set(j.content_clip_id, arr);
    }
    return m;
  }, [jobs]);

  const jobStats = useMemo(
    () => ({
      scheduled: jobs.filter((j) => j.status === "pending" || j.status === "processing").length,
      success: jobs.filter((j) => j.status === "success").length,
      failed: jobs.filter((j) => j.status === "failed").length,
    }),
    [jobs]
  );

  const clipMatches = useCallback(
    (clip: ContentClip, f: ClipFilter) => {
      const js = jobsByClip.get(clip.id) || [];
      switch (f) {
        case "unpublished":
          return js.length === 0;
        case "scheduled":
          return js.some((j) => j.status === "pending" || j.status === "processing");
        case "published":
          return js.some((j) => j.status === "success");
        case "failed":
          return js.some((j) => j.status === "failed");
        default:
          return true;
      }
    },
    [jobsByClip]
  );

  const counts = useMemo(() => {
    const keys: ClipFilter[] = ["all", "unpublished", "scheduled", "published", "failed"];
    return Object.fromEntries(keys.map((k) => [k, clips.filter((c) => clipMatches(c, k)).length])) as Record<ClipFilter, number>;
  }, [clips, clipMatches]);

  const visible = useMemo(() => clips.filter((c) => clipMatches(c, filter)), [clips, filter, clipMatches]);

  const handleDeleteClip = async (clip: ContentClip) => {
    if (!token) return;
    if (!window.confirm(`«${clip.title}» o'chirilsinmi?`)) return;
    try {
      await adminDeleteContentClip(token, clip.id);
      toast.success("Clip o'chirildi");
      await refreshAll();
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "O'chirishda xato");
    }
  };

  const handleCancelJob = async (jobId: string) => {
    if (!token) return;
    try {
      await adminCancelPublishJob(token, jobId);
      const updated = await adminContentListJobsForFolder(token, folderId);
      setJobs(updated || []);
      toast.success("Rejalashtirish bekor qilindi");
    } catch (e) {
      toast.error(e instanceof Error ? e.message : "Bekor qilib bo'lmadi");
    }
  };

  if (loading) {
    return (
      <div className="mx-auto max-w-7xl p-4 sm:p-6">
        <SkeletonList rows={4} height={120} />
      </div>
    );
  }
  if (!folder) {
    return (
      <div className="mx-auto max-w-7xl p-4 sm:p-6">
        <EmptyState
          icon={Folder}
          title="Papka topilmadi"
          text="U o'chirilgan bo'lishi mumkin."
          action={
            <Link href="/admin/content" className="inline-flex items-center gap-2 rounded-xl bg-white/10 px-4 py-2 text-sm text-white hover:bg-white/15">
              <ArrowLeft size={15} /> Content
            </Link>
          }
        />
      </div>
    );
  }

  return (
    <div className="mx-auto max-w-7xl p-4 sm:p-6">
      <Link href="/admin/content" className="mb-4 inline-flex items-center gap-1.5 text-sm text-gray-400 hover:text-white">
        <ArrowLeft size={15} /> Content
      </Link>

      {/* Hero */}
      <header className="relative mb-6 overflow-hidden rounded-3xl border border-white/10 bg-[#12121a]">
        {folder.poster_url && (
          <div
            className="absolute inset-0 scale-110 bg-cover bg-center opacity-20 blur-2xl"
            style={{ backgroundImage: `url(${normalizeMediaUrl(folder.poster_url)})` }}
            aria-hidden
          />
        )}
        <div className="relative flex flex-col gap-4 p-5 sm:flex-row sm:items-center">
          <div className="h-28 w-20 shrink-0 overflow-hidden rounded-xl border border-white/10 bg-gradient-to-br from-fuchsia-500/20 to-purple-700/20">
            <Thumb src={folder.poster_url ? normalizeMediaUrl(folder.poster_url) : undefined} alt={folder.title} className="h-full w-full" icon={Folder} />
          </div>
          <div className="min-w-0 flex-1">
            <p className="text-xs font-medium uppercase tracking-wider text-fuchsia-300/80">Content papka</p>
            <h1 className="mt-0.5 text-2xl font-bold text-white">{folder.title}</h1>
            {folder.description && <p className="mt-1 line-clamp-2 text-sm text-gray-400">{folder.description}</p>}
          </div>
          <PrimaryButton onClick={() => setShowUpload(true)}>
            <Plus size={16} /> Yangi clip
          </PrimaryButton>
        </div>
      </header>

      <div className="mb-5 grid grid-cols-2 gap-3 lg:grid-cols-4">
        <StatTile icon={Video} tone="violet" label="Cliplar" value={clips.length} />
        <StatTile icon={Clock} tone="yellow" label="Navbatda" value={jobStats.scheduled} />
        <StatTile icon={CheckCircle2} tone="green" label="Joylangan" value={jobStats.success} />
        <StatTile icon={XCircle} tone="red" label="Xatolar" value={jobStats.failed} />
      </div>

      {clips.length > 0 && (
        <div className="mb-4">
          <Tabs<ClipFilter>
            value={filter}
            onChange={setFilter}
            items={[
              { key: "all", label: "Hammasi", count: counts.all },
              { key: "unpublished", label: "Joylanmagan", count: counts.unpublished },
              { key: "scheduled", label: "Navbatda", count: counts.scheduled },
              { key: "published", label: "Joylangan", count: counts.published },
              { key: "failed", label: "Xato", count: counts.failed },
            ]}
          />
        </div>
      )}

      {clips.length === 0 ? (
        <EmptyState
          icon={Video}
          title="Hali clip yo'q"
          text="CapCut'dan eksport qilingan vertikal videoni yuklang — keyin uni bir necha akkauntga birdan joylash mumkin."
          action={
            <GhostButton onClick={() => setShowUpload(true)}>
              <Upload size={15} /> Birinchi clipni yuklash
            </GhostButton>
          }
        />
      ) : visible.length === 0 ? (
        <EmptyState icon={Video} title="Bu filtrda clip yo'q" />
      ) : (
        <ul className="grid gap-4 lg:grid-cols-2">
          {visible.map((clip) => (
            <ClipCard
              key={clip.id}
              clip={clip}
              jobs={jobsByClip.get(clip.id) || []}
              onPublish={() => setPublishFor(clip)}
              onDelete={() => handleDeleteClip(clip)}
              onCancelJob={handleCancelJob}
              onCopied={() => toast.success("URL nusxalandi")}
            />
          ))}
        </ul>
      )}

      <UploadClipModal
        open={showUpload}
        folderId={folderId}
        onClose={() => setShowUpload(false)}
        onUploaded={() => {
          setShowUpload(false);
          toast.success("Clip yuklandi");
          void refreshAll();
        }}
      />

      {publishFor && accounts && (
        <PublishModal
          clip={publishFor}
          accounts={accounts}
          onClose={() => setPublishFor(null)}
          onDone={(scheduled) => {
            setPublishFor(null);
            toast.success(scheduled ? "Rejalashtirildi" : "Yuklash boshlandi");
            void refreshAll();
          }}
        />
      )}
    </div>
  );
}

// ─── Clip card ───────────────────────────────────────────────────────────

function ClipCard({
  clip,
  jobs,
  onPublish,
  onDelete,
  onCancelJob,
  onCopied,
}: {
  clip: ContentClip;
  jobs: ContentPublishJob[];
  onPublish: () => void;
  onDelete: () => void;
  onCancelJob: (jobId: string) => void;
  onCopied: () => void;
}) {
  const [showAll, setShowAll] = useState(false);
  const pending = jobs.filter((j) => j.status === "pending").length;
  const processing = jobs.filter((j) => j.status === "processing").length;
  const success = jobs.filter((j) => j.status === "success").length;
  const failed = jobs.filter((j) => j.status === "failed").length;
  const shownJobs = showAll ? jobs : jobs.slice(0, 4);

  return (
    <li className="flex gap-4 rounded-2xl border border-white/10 bg-[#12121a] p-3 transition hover:border-white/20 sm:p-4">
      <div className="aspect-[9/16] w-28 shrink-0 overflow-hidden rounded-xl bg-black sm:w-36">
        <video src={clip.url} className="h-full w-full bg-black object-contain" controls preload="metadata" playsInline />
      </div>

      <div className="flex min-w-0 flex-1 flex-col">
        <div className="flex items-start justify-between gap-2">
          <div className="min-w-0">
            <h3 className="truncate font-semibold text-white">{clip.title}</h3>
            <p className="truncate text-[11px] text-gray-500">
              {clip.filename}
              {clip.size ? ` · ${(clip.size / 1024 / 1024).toFixed(1)} MB` : ""}
            </p>
          </div>
          <div className="-mr-1 -mt-1 flex shrink-0 items-center">
            <a href={clip.url} target="_blank" rel="noopener noreferrer" className="rounded-lg p-2 text-gray-400 hover:bg-white/5 hover:text-white" title="Yangi oynada ochish" aria-label="Yangi oynada ochish">
              <ExternalLink size={15} />
            </a>
            <IconBtn
              label="URL nusxalash"
              onClick={() => {
                void navigator.clipboard?.writeText(clip.url).then(onCopied);
              }}
            >
              <Copy size={15} />
            </IconBtn>
            <IconBtn label="O'chirish" tone="red" onClick={onDelete}>
              <Trash2 size={15} />
            </IconBtn>
          </div>
        </div>

        {clip.caption && <p className="mt-2 line-clamp-3 whitespace-pre-line rounded-lg bg-black/20 px-2.5 py-2 text-xs text-gray-400">{clip.caption}</p>}

        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {pending > 0 && (
            <Chip tone="yellow" icon={Clock}>
              {pending} rejalashtirilgan
            </Chip>
          )}
          {processing > 0 && (
            <Chip tone="blue" icon={Loader2}>
              {processing} yuklanmoqda
            </Chip>
          )}
          {success > 0 && (
            <Chip tone="green" icon={CheckCircle2}>
              {success} joylandi
            </Chip>
          )}
          {failed > 0 && (
            <Chip tone="red" icon={XCircle}>
              {failed} xato
            </Chip>
          )}
          {jobs.length === 0 && <Chip tone="gray">Hali joylanmagan</Chip>}
        </div>

        {jobs.length > 0 && (
          <div className="mt-2.5 space-y-1">
            {shownJobs.map((j) => (
              <JobRow key={j.id} job={j} onCancel={() => onCancelJob(j.id)} />
            ))}
            {jobs.length > 4 && (
              <button type="button" onClick={() => setShowAll((v) => !v)} className="text-[11px] text-gray-500 hover:text-white">
                {showAll ? "Kamroq ko'rsatish" : `Yana ${jobs.length - 4} ta job`}
              </button>
            )}
          </div>
        )}

        <div className="mt-auto flex items-center justify-between gap-2 pt-3">
          <span className="text-[11px] text-gray-600">{formatDateTime(clip.created_at)}</span>
          <button
            type="button"
            onClick={onPublish}
            className="inline-flex items-center gap-1.5 rounded-xl bg-orange-500 px-3.5 py-2 text-xs font-semibold text-white shadow-lg shadow-orange-500/20 hover:bg-orange-400"
          >
            <Send size={13} /> Joylash
          </button>
        </div>
      </div>
    </li>
  );
}

function JobRow({ job, onCancel }: { job: ContentPublishJob; onCancel: () => void }) {
  let status: React.ReactNode = null;
  switch (job.status) {
    case "pending":
      status = (
        <span className="inline-flex items-center gap-1 text-amber-300">
          <Clock size={11} /> {formatDateTime(job.scheduled_for)}
        </span>
      );
      break;
    case "processing":
      status = (
        <span className="inline-flex items-center gap-1 text-sky-300">
          <Loader2 size={11} className="animate-spin" /> Yuklanmoqda
        </span>
      );
      break;
    case "success":
      status = (
        <span className="inline-flex items-center gap-1 text-emerald-300">
          <CheckCircle2 size={11} /> {job.published_at ? formatDateTime(job.published_at) : "Joylandi"}
          {job.instagram_post_url && (
            <a href={job.instagram_post_url} target="_blank" rel="noopener noreferrer" className="ml-1 underline hover:text-emerald-200">
              ko&apos;rish
            </a>
          )}
        </span>
      );
      break;
    case "failed":
      status = (
        <span className="inline-flex min-w-0 items-center gap-1 text-red-300" title={job.error || ""}>
          <XCircle size={11} className="shrink-0" /> <span className="truncate">Xato{job.error ? `: ${job.error.slice(0, 80)}` : ""}</span>
        </span>
      );
      break;
  }
  return (
    <div className="flex items-center justify-between gap-2 rounded-lg bg-black/25 px-2 py-1.5 text-[11px]">
      <div className="flex min-w-0 items-center gap-2">
        <PlatformBadge platform={job.platform} size={18} />
        <span className="max-w-[8rem] truncate text-gray-300">{job.account_name}</span>
        <span className="text-gray-700">·</span>
        {status}
      </div>
      {job.status === "pending" && (
        <button type="button" onClick={onCancel} className="rounded p-0.5 text-gray-500 hover:bg-white/5 hover:text-red-300" title="Bekor qilish" aria-label="Bekor qilish">
          <X size={12} />
        </button>
      )}
    </div>
  );
}

// ─── Upload modal ────────────────────────────────────────────────────────

function UploadClipModal({
  open,
  folderId,
  onClose,
  onUploaded,
}: {
  open: boolean;
  folderId: string;
  onClose: () => void;
  onUploaded: () => void;
}) {
  const { token } = useAuth();
  const [file, setFile] = useState<File | null>(null);
  const [preview, setPreview] = useState<string | null>(null);
  const [title, setTitle] = useState("");
  const [caption, setCaption] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragOver, setDragOver] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (!open) return;
    setFile(null);
    setTitle("");
    setCaption("");
    setError(null);
  }, [open]);

  useEffect(() => {
    if (!file) {
      setPreview(null);
      return;
    }
    const url = URL.createObjectURL(file);
    setPreview(url);
    return () => URL.revokeObjectURL(url);
  }, [file]);

  const pick = (f: File | null) => {
    if (!f) return;
    if (!f.type.startsWith("video/") && !/\.(mp4|mov|webm)$/i.test(f.name)) {
      setError("Faqat video fayl (mp4, mov, webm)");
      return;
    }
    setError(null);
    setFile(f);
    setTitle((t) => t || f.name.replace(/\.[^.]+$/, ""));
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !file) return;
    setBusy(true);
    setError(null);
    try {
      await adminUploadContentClip(token, folderId, file, title || file.name, caption);
      onUploaded();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open={open}
      onClose={onClose}
      busy={busy}
      size="lg"
      icon={Upload}
      iconTone="violet"
      title="Yangi clip yuklash"
      subtitle="mp4, mov yoki webm — maksimal 250 MB"
      footer={
        <>
          <GhostButton onClick={onClose} disabled={busy}>
            Bekor qilish
          </GhostButton>
          <button
            type="submit"
            form="content-clip-form"
            disabled={busy || !file}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-50"
          >
            {busy ? <Loader2 size={15} className="animate-spin" /> : <Upload size={15} />}
            {busy ? "Yuklanmoqda..." : "Yuklash"}
          </button>
        </>
      }
    >
      <form id="content-clip-form" onSubmit={submit} className="grid gap-5 sm:grid-cols-[170px_1fr]">
        <div>
          <input
            ref={inputRef}
            type="file"
            accept="video/mp4,video/quicktime,video/webm,.mov,.mp4,.webm"
            className="hidden"
            onChange={(e) => {
              const f = e.target.files?.[0] || null;
              e.target.value = "";
              pick(f);
            }}
          />
          <div
            role="button"
            tabIndex={0}
            onClick={() => !busy && inputRef.current?.click()}
            onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && inputRef.current?.click()}
            onDragOver={(e) => {
              e.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={(e) => {
              e.preventDefault();
              setDragOver(false);
              pick(e.dataTransfer.files?.[0] || null);
            }}
            className={`relative mx-auto flex aspect-[9/16] w-40 cursor-pointer flex-col items-center justify-center overflow-hidden rounded-2xl border-2 border-dashed text-center transition sm:w-full ${
              dragOver ? "border-orange-500 bg-orange-500/10" : file ? "border-white/10 bg-black" : "border-white/15 bg-black/30 hover:border-white/30"
            }`}
          >
            {preview ? (
              <>
                <video src={preview} className="h-full w-full object-contain" muted playsInline autoPlay loop />
                <span className="absolute bottom-2 left-1/2 -translate-x-1/2 whitespace-nowrap rounded-full bg-black/75 px-2.5 py-1 text-[11px] text-white">Almashtirish</span>
              </>
            ) : (
              <div className="px-3">
                <FileVideo size={28} className="mx-auto mb-2 text-gray-500" />
                <p className="text-xs font-medium text-gray-300">Videoni tanlang</p>
                <p className="mt-1 text-[11px] text-gray-500">yoki shu yerga tashlang</p>
              </div>
            )}
          </div>
          {file && <p className="mt-2 truncate text-center text-[11px] text-gray-500">{(file.size / 1024 / 1024).toFixed(1)} MB</p>}
        </div>

        <div className="space-y-4">
          {error && <ErrorBanner message={error} />}
          <Field label="Sarlavha" htmlFor="clip-title">
            <input id="clip-title" value={title} onChange={(e) => setTitle(e.target.value)} className={inputCls} placeholder="Reklama klipi #1" />
          </Field>
          <Field
            label="Caption"
            htmlFor="clip-caption"
            right={<span className="text-[11px] tabular-nums text-gray-600">{caption.length}/2200</span>}
            hint="Instagram / YouTube / TikTok'ga aynan shu matn yuboriladi (kino kodi avtomatik qo'shilmaydi)."
          >
            <textarea
              id="clip-caption"
              value={caption}
              maxLength={2200}
              onChange={(e) => setCaption(e.target.value)}
              rows={7}
              className={`${inputCls} resize-none`}
              placeholder="Postning matni va hashtaglar..."
            />
          </Field>
        </div>
      </form>
    </Modal>
  );
}

// ─── Publish modal ───────────────────────────────────────────────────────

function PublishModal({
  clip,
  accounts,
  onClose,
  onDone,
}: {
  clip: ContentClip;
  accounts: PublishAccountsResponse;
  onClose: () => void;
  onDone: (scheduled: boolean) => void;
}) {
  const { token } = useAuth();
  const [mode, setMode] = useState<"now" | "schedule">("now");
  // Default the scheduler input to ~5 minutes from now in local time so the
  // admin sees a sensible starting point instead of an empty field.
  const [scheduledFor, setScheduledFor] = useState<string>(() => {
    const d = new Date(Date.now() + 5 * 60_000);
    const pad = (n: number) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
  });
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const targets = useMemo<{ platform: "instagram" | "youtube" | "tiktok"; account: string; key: string }[]>(() => {
    const out: { platform: "instagram" | "youtube" | "tiktok"; account: string; key: string }[] = [];
    for (const a of accounts.instagram || []) out.push({ platform: "instagram", account: a, key: `instagram:${a}` });
    for (const a of accounts.youtube || []) out.push({ platform: "youtube", account: a, key: `youtube:${a}` });
    for (const a of accounts.tiktok || []) out.push({ platform: "tiktok", account: a, key: `tiktok:${a}` });
    return out;
  }, [accounts]);

  const toggle = (key: string) => {
    setSelected((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  const allSelected = targets.length > 0 && selected.size === targets.length;

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    if (selected.size === 0) {
      setError("Kamida bitta akkaunt tanlang");
      return;
    }
    if (mode === "schedule" && !scheduledFor) {
      setError("Vaqt belgilang");
      return;
    }
    setBusy(true);
    setError(null);
    try {
      const jobs: PublishTarget[] = targets
        .filter((t) => selected.has(t.key))
        .map((t) => ({ platform: t.platform, account_name: t.account }));
      if (mode === "now") {
        await adminContentPublishNow(token, clip.id, jobs);
      } else {
        await adminContentPublishSchedule(token, clip.id, jobs, localDateTimeToRFC3339(scheduledFor));
      }
      onDone(mode === "schedule");
    } catch (err) {
      setError(err instanceof Error ? err.message : "Joylab bo'lmadi");
    } finally {
      setBusy(false);
    }
  };

  return (
    <Modal
      open
      onClose={onClose}
      busy={busy}
      icon={Send}
      title="Ijtimoiy tarmoqlarga joylash"
      subtitle={`«${clip.title}»`}
      footer={
        <>
          <GhostButton onClick={onClose} disabled={busy}>
            Bekor qilish
          </GhostButton>
          <button
            type="submit"
            form="content-publish-form"
            disabled={busy || selected.size === 0}
            className="inline-flex items-center gap-2 rounded-xl bg-orange-500 px-4 py-2.5 text-sm font-semibold text-white hover:bg-orange-400 disabled:opacity-50"
          >
            {busy ? <Loader2 size={15} className="animate-spin" /> : mode === "now" ? <Zap size={15} /> : <Calendar size={15} />}
            {mode === "now" ? "Hozir joylash" : "Rejalashtirish"}
            {selected.size > 0 && <span className="rounded-full bg-white/20 px-1.5 text-[11px]">{selected.size}</span>}
          </button>
        </>
      }
    >
      <form id="content-publish-form" onSubmit={submit} className="space-y-5">
        {error && <ErrorBanner message={error} />}

        <Segmented<"now" | "schedule">
          ariaLabel="Joylash vaqti"
          value={mode}
          onChange={setMode}
          options={[
            { value: "now", label: "Hozir" },
            { value: "schedule", label: "Vaqt belgilash" },
          ]}
        />

        {mode === "schedule" && (
          <Field label="Rejalashtirilgan vaqt (mahalliy)" htmlFor="pub-time">
            <input id="pub-time" type="datetime-local" value={scheduledFor} onChange={(e) => setScheduledFor(e.target.value)} className={`${inputCls} [color-scheme:dark]`} />
          </Field>
        )}

        <Field
          label="Akkauntlar"
          right={
            targets.length > 1 ? (
              <button type="button" onClick={() => setSelected(allSelected ? new Set() : new Set(targets.map((t) => t.key)))} className="text-[11px] text-orange-300 hover:text-orange-200">
                {allSelected ? "Hammasini olib tashlash" : "Hammasini tanlash"}
              </button>
            ) : undefined
          }
        >
          {targets.length === 0 ? (
            <p className="rounded-xl border border-dashed border-white/10 px-4 py-6 text-center text-xs text-gray-500">Hech bir platforma sozlanmagan.</p>
          ) : (
            <div className="grid gap-2 sm:grid-cols-2">
              {targets.map((t) => {
                const on = selected.has(t.key);
                return (
                  <button
                    key={t.key}
                    type="button"
                    role="checkbox"
                    aria-checked={on}
                    onClick={() => toggle(t.key)}
                    className={`flex items-center gap-3 rounded-xl border px-3 py-2.5 text-left transition ${
                      on ? "border-orange-500/50 bg-orange-500/10" : "border-white/10 bg-black/20 hover:border-white/20"
                    }`}
                  >
                    <PlatformBadge platform={t.platform} size={28} />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium text-white">{t.account}</span>
                      <span className="block text-[11px] capitalize text-gray-500">{t.platform}</span>
                    </span>
                    <span className={`flex h-5 w-5 shrink-0 items-center justify-center rounded-md border ${on ? "border-orange-500 bg-orange-500 text-white" : "border-white/20"}`}>
                      {on && <CheckCircle2 size={13} />}
                    </span>
                  </button>
                );
              })}
            </div>
          )}
        </Field>
      </form>
    </Modal>
  );
}
