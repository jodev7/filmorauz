"use client";

import { useEffect, useState, useMemo, useCallback } from "react";
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

// ─── Helpers ─────────────────────────────────────────────────────────────

function platformIcon(p: string) {
  if (p === "instagram") return <Instagram size={14} />;
  if (p === "youtube") return <Youtube size={14} />;
  return <Send size={14} />;
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

// ─── Page ────────────────────────────────────────────────────────────────

export default function FolderDetailPage() {
  const { token } = useAuth();
  const params = useParams();
  const folderId = params.folderId as string;

  const [folder, setFolder] = useState<ContentFolder | null>(null);
  const [clips, setClips] = useState<ContentClip[]>([]);
  const [jobs, setJobs] = useState<ContentPublishJob[]>([]);
  const [accounts, setAccounts] = useState<PublishAccountsResponse | null>(null);
  const [loading, setLoading] = useState(true);

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
      setClips(c);
      setJobs(j);
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
        .then(setJobs)
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

  const handleDeleteClip = async (clip: ContentClip) => {
    if (!token) return;
    if (!window.confirm(`"${clip.title}" o'chirilsinmi?`)) return;
    try {
      await adminDeleteContentClip(token, clip.id);
      await refreshAll();
    } catch (e) {
      alert(e instanceof Error ? e.message : "Delete failed");
    }
  };

  const handleCancelJob = async (jobId: string) => {
    if (!token) return;
    try {
      await adminCancelPublishJob(token, jobId);
      const updated = await adminContentListJobsForFolder(token, folderId);
      setJobs(updated);
    } catch (e) {
      alert(e instanceof Error ? e.message : "Cancel failed");
    }
  };

  if (loading) {
    return (
      <div className="flex justify-center py-16">
        <Loader2 className="animate-spin text-brand-red" size={32} />
      </div>
    );
  }
  if (!folder) {
    return (
      <div className="p-8 text-gray-400">
        Folder topilmadi. <Link href="/admin/content" className="text-brand-red">Orqaga</Link>
      </div>
    );
  }

  return (
    <div className="p-4 sm:p-8">
      <Link
        href="/admin/content"
        className="inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white mb-6"
      >
        <ArrowLeft size={16} /> Content
      </Link>

      <div className="flex items-start justify-between gap-4 mb-8">
        <div>
          <h1 className="text-2xl font-bold text-white">{folder.title}</h1>
          {folder.description && (
            <p className="text-sm text-gray-400 mt-1">{folder.description}</p>
          )}
          <p className="text-xs text-gray-500 mt-2">{folder.clips_count} clip · {jobs.length} job</p>
        </div>
        <button
          onClick={() => setShowUpload(true)}
          className="flex items-center gap-2 bg-brand-red hover:bg-red-600 text-white px-4 py-2 rounded-lg whitespace-nowrap"
        >
          <Plus size={18} />
          Yangi clip
        </button>
      </div>

      {clips.length === 0 ? (
        <div className="bg-brand-card border border-brand-border rounded-xl py-16 text-center">
          <Video size={48} className="text-gray-600 mx-auto mb-3" />
          <p className="text-gray-500">Hali clip yo'q. Birinchi clipni yuklang.</p>
        </div>
      ) : (
        <div className="space-y-3">
          {clips.map((clip) => {
            const clipJobs = jobsByClip.get(clip.id) || [];
            return (
              <ClipRow
                key={clip.id}
                clip={clip}
                jobs={clipJobs}
                onPublish={() => setPublishFor(clip)}
                onDelete={() => handleDeleteClip(clip)}
                onCancelJob={handleCancelJob}
              />
            );
          })}
        </div>
      )}

      {showUpload && (
        <UploadClipModal
          folderId={folderId}
          onClose={() => setShowUpload(false)}
          onUploaded={() => {
            setShowUpload(false);
            void refreshAll();
          }}
        />
      )}

      {publishFor && accounts && (
        <PublishModal
          clip={publishFor}
          accounts={accounts}
          onClose={() => setPublishFor(null)}
          onDone={() => {
            setPublishFor(null);
            void refreshAll();
          }}
        />
      )}
    </div>
  );
}

// ─── Clip row ────────────────────────────────────────────────────────────

function ClipRow({
  clip,
  jobs,
  onPublish,
  onDelete,
  onCancelJob,
}: {
  clip: ContentClip;
  jobs: ContentPublishJob[];
  onPublish: () => void;
  onDelete: () => void;
  onCancelJob: (jobId: string) => void;
}) {
  const pending = jobs.filter((j) => j.status === "pending");
  const processing = jobs.filter((j) => j.status === "processing");
  const success = jobs.filter((j) => j.status === "success");
  const failed = jobs.filter((j) => j.status === "failed");

  return (
    <div className="bg-brand-card border border-brand-border rounded-xl p-4">
      <div className="flex items-start gap-4">
        <div className="w-32 sm:w-40 flex-shrink-0 aspect-[9/16] bg-black rounded-lg overflow-hidden">
          <video
            src={clip.url}
            className="h-full w-full object-contain bg-black"
            controls
            preload="metadata"
            playsInline
          />
        </div>

        <div className="flex-1 min-w-0">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h3 className="text-white font-medium truncate">{clip.title}</h3>
              <p className="text-xs text-gray-500 mt-0.5 truncate">{clip.filename}</p>
              {/* Direct clip URL — open in new tab or copy. */}
              <div className="flex items-center gap-1.5 mt-1">
                <a
                  href={clip.url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-[11px] text-brand-red hover:underline truncate flex items-center gap-1 max-w-[22rem]"
                  title={clip.url}
                >
                  <ExternalLink size={11} className="shrink-0" />
                  <span className="truncate">{clip.url}</span>
                </a>
                <button
                  type="button"
                  onClick={() => {
                    void navigator.clipboard?.writeText(clip.url);
                  }}
                  title="URL nusxalash"
                  className="p-1 text-gray-500 hover:text-white shrink-0"
                >
                  <Copy size={12} />
                </button>
              </div>
              {clip.caption && (
                <p className="text-xs text-gray-400 mt-2 line-clamp-2">{clip.caption}</p>
              )}
              <p className="text-[11px] text-gray-600 mt-2">
                Yuklangan {formatDateTime(clip.created_at)}
                {clip.size ? ` · ${(clip.size / 1024 / 1024).toFixed(1)} MB` : ""}
              </p>
            </div>

            <div className="flex items-center gap-1 shrink-0">
              <button
                onClick={onPublish}
                className="flex items-center gap-1 bg-brand-red hover:bg-red-600 text-white text-xs px-3 py-1.5 rounded-lg"
              >
                <Send size={12} />
                Upload
              </button>
              <button
                onClick={onDelete}
                className="p-1.5 text-gray-400 hover:text-red-400 rounded"
                title="O'chirish"
              >
                <Trash2 size={14} />
              </button>
            </div>
          </div>

          {/* Aggregate badges */}
          <div className="flex flex-wrap gap-2 mt-3">
            {pending.length > 0 && (
              <span className="inline-flex items-center gap-1 text-[11px] bg-yellow-500/15 border border-yellow-500/40 text-yellow-400 px-2 py-0.5 rounded-full">
                <Clock size={11} /> {pending.length} rejalashtirilgan
              </span>
            )}
            {processing.length > 0 && (
              <span className="inline-flex items-center gap-1 text-[11px] bg-blue-500/15 border border-blue-500/40 text-blue-400 px-2 py-0.5 rounded-full">
                <Loader2 size={11} className="animate-spin" /> {processing.length} yuklanmoqda
              </span>
            )}
            {success.length > 0 && (
              <span className="inline-flex items-center gap-1 text-[11px] bg-emerald-500/15 border border-emerald-500/40 text-emerald-400 px-2 py-0.5 rounded-full">
                <CheckCircle2 size={11} /> {success.length} muvaffaqiyatli
              </span>
            )}
            {failed.length > 0 && (
              <span className="inline-flex items-center gap-1 text-[11px] bg-red-500/15 border border-red-500/40 text-red-400 px-2 py-0.5 rounded-full">
                <XCircle size={11} /> {failed.length} xato
              </span>
            )}
            {jobs.length === 0 && (
              <span className="text-[11px] text-gray-600">Hali upload qilinmagan</span>
            )}
          </div>

          {/* Per-job rows */}
          {jobs.length > 0 && (
            <div className="mt-3 space-y-1.5">
              {jobs.slice(0, 6).map((j) => (
                <JobRow key={j.id} job={j} onCancel={() => onCancelJob(j.id)} />
              ))}
              {jobs.length > 6 && (
                <p className="text-[11px] text-gray-600">va yana {jobs.length - 6} ta job…</p>
              )}
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

function JobRow({ job, onCancel }: { job: ContentPublishJob; onCancel: () => void }) {
  let badge: React.ReactNode = null;
  switch (job.status) {
    case "pending":
      badge = (
        <span className="inline-flex items-center gap-1 text-yellow-400">
          <Clock size={11} /> Rejalashtirilgan · {formatDateTime(job.scheduled_for)}
        </span>
      );
      break;
    case "processing":
      badge = (
        <span className="inline-flex items-center gap-1 text-blue-400">
          <Loader2 size={11} className="animate-spin" /> Yuklanmoqda
        </span>
      );
      break;
    case "success":
      badge = (
        <span className="inline-flex items-center gap-1 text-emerald-400">
          <CheckCircle2 size={11} /> Muvaffaqiyatli yuklandi
          {job.published_at ? ` · ${formatDateTime(job.published_at)}` : ""}
          {job.instagram_post_url && (
            <a
              href={job.instagram_post_url}
              target="_blank"
              rel="noopener noreferrer"
              className="ml-1 underline hover:text-emerald-300"
            >
              ko'rish
            </a>
          )}
        </span>
      );
      break;
    case "failed":
      badge = (
        <span className="inline-flex items-center gap-1 text-red-400" title={job.error || ""}>
          <XCircle size={11} /> Xato{job.error ? `: ${job.error.slice(0, 80)}` : ""}
        </span>
      );
      break;
  }
  return (
    <div className="flex items-center justify-between gap-2 text-[11px] px-2 py-1 rounded bg-brand-dark/50">
      <div className="flex items-center gap-2 min-w-0">
        <span className="text-gray-400 inline-flex items-center gap-1">
          {platformIcon(job.platform)}
          <span className="capitalize">{job.platform}</span>
          <span className="text-gray-600">·</span>
          <span className="text-gray-300">{job.account_name}</span>
        </span>
        <span className="text-gray-600">·</span>
        {badge}
      </div>
      {job.status === "pending" && (
        <button onClick={onCancel} className="text-gray-500 hover:text-red-400" title="Bekor qilish">
          <X size={12} />
        </button>
      )}
    </div>
  );
}

// ─── Upload modal ────────────────────────────────────────────────────────

function UploadClipModal({
  folderId,
  onClose,
  onUploaded,
}: {
  folderId: string;
  onClose: () => void;
  onUploaded: () => void;
}) {
  const { token } = useAuth();
  const [file, setFile] = useState<File | null>(null);
  const [title, setTitle] = useState("");
  const [caption, setCaption] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token || !file) return;
    setBusy(true);
    setError(null);
    try {
      await adminUploadContentClip(token, folderId, file, title || file.name, caption);
      onUploaded();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Upload failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="w-full max-w-md rounded-xl bg-brand-card border border-brand-border p-6">
        <div className="flex items-center justify-between mb-5">
          <h2 className="text-lg font-semibold text-white">Yangi clip yuklash</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <X size={18} />
          </button>
        </div>
        {error && (
          <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/40 text-sm text-red-400">
            {error}
          </div>
        )}
        <form onSubmit={submit} className="space-y-4">
          <div>
            <label className="block text-xs text-gray-400 mb-1">Video fayl *</label>
            <label className="flex items-center gap-2 cursor-pointer w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-sm text-gray-300 hover:border-brand-red transition-colors">
              <Upload size={16} />
              <span>{file ? file.name : "Video tanlash (mp4, mov, webm — maks 250MB)"}</span>
              <input
                type="file"
                accept="video/mp4,video/quicktime,video/webm,.mov,.mp4,.webm"
                className="hidden"
                onChange={(e) => {
                  const f = e.target.files?.[0] || null;
                  e.target.value = "";
                  setFile(f);
                  if (f && !title) setTitle(f.name.replace(/\.[^.]+$/, ""));
                }}
              />
            </label>
          </div>
          <div>
            <label className="block text-xs text-gray-400 mb-1">Sarlavha</label>
            <input
              type="text"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
              placeholder="Reklama klipi #1"
            />
          </div>
          <div>
            <label className="block text-xs text-gray-400 mb-1">Instagram caption</label>
            <textarea
              value={caption}
              onChange={(e) => setCaption(e.target.value)}
              rows={4}
              className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red resize-none"
              placeholder="Postning matni va hashtaglar..."
            />
            <p className="mt-1 text-[11px] text-gray-600">
              Bu matn Instagramga aynan shu holatda yuboriladi (movie/series kodi qo'shilmaydi).
            </p>
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm text-gray-300 hover:text-white">
              Bekor qilish
            </button>
            <button
              type="submit"
              disabled={busy || !file}
              className="flex items-center gap-2 bg-brand-red text-white px-4 py-2 rounded-lg hover:bg-red-600 disabled:opacity-50 text-sm"
            >
              {busy && <Loader2 size={14} className="animate-spin" />}
              Yuklash
            </button>
          </div>
        </form>
      </div>
    </div>
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
  onDone: () => void;
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
    for (const a of accounts.instagram) out.push({ platform: "instagram", account: a, key: `instagram:${a}` });
    for (const a of accounts.youtube) out.push({ platform: "youtube", account: a, key: `youtube:${a}` });
    for (const a of accounts.tiktok) out.push({ platform: "tiktok", account: a, key: `tiktok:${a}` });
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

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    if (selected.size === 0) {
      setError("Kamida bitta account tanlang");
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
      onDone();
    } catch (e) {
      setError(e instanceof Error ? e.message : "Publish failed");
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="w-full max-w-lg rounded-xl bg-brand-card border border-brand-border p-6 max-h-[90vh] overflow-y-auto">
        <div className="flex items-center justify-between mb-1">
          <h2 className="text-lg font-semibold text-white">Upload qilish</h2>
          <button onClick={onClose} className="text-gray-400 hover:text-white">
            <X size={18} />
          </button>
        </div>
        <p className="text-xs text-gray-500 mb-5 truncate">"{clip.title}"</p>

        {error && (
          <div className="mb-4 p-3 rounded-lg bg-red-500/10 border border-red-500/40 text-sm text-red-400">
            {error}
          </div>
        )}

        <form onSubmit={submit} className="space-y-5">
          {/* Mode */}
          <div className="flex gap-3">
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="radio"
                checked={mode === "now"}
                onChange={() => setMode("now")}
                className="accent-brand-red"
              />
              <span className="text-sm text-white">Hozir upload</span>
            </label>
            <label className="flex items-center gap-2 cursor-pointer">
              <input
                type="radio"
                checked={mode === "schedule"}
                onChange={() => setMode("schedule")}
                className="accent-brand-red"
              />
              <span className="text-sm text-white">Vaqt belgilash</span>
            </label>
          </div>

          {mode === "schedule" && (
            <div>
              <label className="text-xs text-gray-400 mb-1 flex items-center gap-1">
                <Calendar size={12} /> Rejalashtirilgan vaqt (mahalliy)
              </label>
              <input
                type="datetime-local"
                value={scheduledFor}
                onChange={(e) => setScheduledFor(e.target.value)}
                className="w-full px-3 py-2 bg-brand-dark border border-brand-border rounded-lg text-white text-sm focus:outline-none focus:border-brand-red"
              />
            </div>
          )}

          {/* Accounts */}
          <div>
            <p className="text-xs text-gray-400 mb-2">Accountlar (bir nechtasini tanlash mumkin)</p>
            {targets.length === 0 ? (
              <p className="text-xs text-gray-500">Hech bir platforma sozlanmagan.</p>
            ) : (
              <div className="space-y-1.5">
                {targets.map((t) => (
                  <label
                    key={t.key}
                    className={`flex items-center gap-3 px-3 py-2 rounded-lg cursor-pointer border ${
                      selected.has(t.key)
                        ? "bg-brand-red/10 border-brand-red/40"
                        : "bg-brand-dark border-brand-border hover:border-gray-500"
                    }`}
                  >
                    <input
                      type="checkbox"
                      checked={selected.has(t.key)}
                      onChange={() => toggle(t.key)}
                      className="accent-brand-red"
                    />
                    <span className="text-gray-300">{platformIcon(t.platform)}</span>
                    <span className="text-sm capitalize text-gray-400">{t.platform}</span>
                    <span className="text-sm text-white">{t.account}</span>
                  </label>
                ))}
              </div>
            )}
          </div>

          <div className="flex justify-end gap-2 pt-2">
            <button type="button" onClick={onClose} className="px-4 py-2 text-sm text-gray-300 hover:text-white">
              Bekor qilish
            </button>
            <button
              type="submit"
              disabled={busy || selected.size === 0}
              className="flex items-center gap-2 bg-brand-red text-white px-4 py-2 rounded-lg hover:bg-red-600 disabled:opacity-50 text-sm"
            >
              {busy && <Loader2 size={14} className="animate-spin" />}
              {mode === "now" ? "Hozir yuklash" : "Rejalashtirish"}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
