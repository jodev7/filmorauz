"use client";

import { useEffect, useRef, useState } from "react";
import { AlertCircle, CheckCircle2, Film, ImagePlus, Link2, Loader2, RefreshCw, Trash2, Upload } from "lucide-react";
import type { UploadProgressInfo } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import { inputCls } from "./ui";

export type UploadFn = (file: File, onProgress: (p: UploadProgressInfo) => void) => Promise<{ url: string; file_key?: string }>;

interface State {
  status: "idle" | "uploading" | "done" | "error";
  progress?: number;
  uploadedMB?: number;
  totalMB?: number;
  speed?: number;
  eta?: number;
  message?: string;
  fileName?: string;
}

const IMAGE_TYPES = ["image/jpeg", "image/png", "image/webp", "image/gif"];

function fmtEta(s?: number) {
  if (!s || !Number.isFinite(s) || s <= 0) return "";
  return s < 60 ? `${Math.round(s)}s` : `${Math.floor(s / 60)}m ${Math.round(s % 60)}s`;
}

/**
 * Drop zone for posters, backdrops and videos: click, drag & drop, paste an
 * image (Ctrl+V while focused) or enter a URL. Shows a live preview and
 * upload progress; replace/remove from the preview overlay.
 */
export default function MediaUploadField({
  label,
  value,
  onChange,
  upload,
  kind = "poster",
  required,
  hint,
  onBusyChange,
  onUploaded,
}: {
  label: string;
  value: string;
  onChange: (url: string) => void;
  upload: UploadFn;
  kind?: "poster" | "backdrop" | "video";
  required?: boolean;
  hint?: string;
  onBusyChange?: (busy: boolean) => void;
  onUploaded?: (res: { url: string; file_key?: string }) => void;
}) {
  const [state, setState] = useState<State>({ status: "idle" });
  const [dragOver, setDragOver] = useState(false);
  const [showUrl, setShowUrl] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);
  const lastFile = useRef<File | null>(null);
  const isVideo = kind === "video";

  useEffect(() => {
    onBusyChange?.(state.status === "uploading");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.status]);

  const start = async (file: File) => {
    if (!isVideo && !IMAGE_TYPES.includes(file.type)) {
      setState({ status: "error", message: "Faqat JPG, PNG, WebP yoki GIF rasm" });
      return;
    }
    if (isVideo && !/^video\//.test(file.type) && !file.name.endsWith(".m3u8")) {
      setState({ status: "error", message: "Video fayl tanlang (mp4, webm…)" });
      return;
    }
    lastFile.current = file;
    setState({ status: "uploading", progress: 0, fileName: file.name, totalMB: file.size / 1024 / 1024 });
    try {
      const res = await upload(file, (p) =>
        setState((s) => ({
          ...s,
          status: "uploading",
          progress: p.progress,
          uploadedMB: p.uploadedMB,
          totalMB: p.total ? p.total / 1024 / 1024 : s.totalMB,
          speed: p.speedMBps,
          eta: p.etaSeconds,
        }))
      );
      onChange(res.url);
      onUploaded?.(res);
      setState({ status: "done", fileName: file.name });
    } catch (e) {
      setState({ status: "error", message: e instanceof Error ? e.message : "Yuklab bo'lmadi", fileName: file.name });
    }
  };

  const onPaste = (e: React.ClipboardEvent) => {
    if (isVideo) return;
    const file = Array.from(e.clipboardData.files).find((f) => f.type.startsWith("image/"));
    if (file) {
      e.preventDefault();
      void start(file);
      return;
    }
    const text = e.clipboardData.getData("text").trim();
    if (/^https?:\/\/\S+$/i.test(text)) {
      e.preventDefault();
      onChange(text);
    }
  };

  const aspect = kind === "poster" ? "aspect-[2/3] max-w-[180px]" : "aspect-video";
  const uploading = state.status === "uploading";
  const preview = value && !isVideo ? normalizeMediaUrl(value) : "";

  return (
    <div>
      <div className="mb-1.5 flex items-center justify-between">
        <span className="text-xs font-medium text-gray-400">
          {label} {required && <span className="text-orange-400">*</span>}
        </span>
        {!isVideo && (
          <button type="button" onClick={() => setShowUrl((v) => !v)} className="inline-flex items-center gap-1 text-[11px] text-gray-500 hover:text-white">
            <Link2 size={12} /> URL
          </button>
        )}
      </div>

      <div
        tabIndex={0}
        role="button"
        aria-label={`${label}: fayl tanlash yoki tashlash`}
        onClick={() => !uploading && inputRef.current?.click()}
        onKeyDown={(e) => (e.key === "Enter" || e.key === " ") && !uploading && inputRef.current?.click()}
        onPaste={onPaste}
        onDragOver={(e) => {
          e.preventDefault();
          setDragOver(true);
        }}
        onDragLeave={() => setDragOver(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDragOver(false);
          const f = e.dataTransfer.files?.[0];
          if (f) void start(f);
        }}
        className={`group relative w-full cursor-pointer overflow-hidden rounded-xl border-2 border-dashed transition-colors focus:outline-none focus:ring-2 focus:ring-orange-500/40 ${aspect} ${
          dragOver ? "border-orange-500 bg-orange-500/10" : preview ? "border-transparent" : "border-white/10 bg-black/30 hover:border-orange-500/50"
        } ${isVideo ? "aspect-auto min-h-[112px]" : ""}`}
      >
        {preview && <MediaImage src={preview} alt={label} className="absolute inset-0 h-full w-full object-cover" />}

        {/* Idle / hover content */}
        {!uploading && (
          <div
            className={`absolute inset-0 flex flex-col items-center justify-center gap-1.5 p-3 text-center text-xs ${
              preview ? "bg-black/60 opacity-0 transition-opacity group-hover:opacity-100 group-focus:opacity-100" : "text-gray-500"
            }`}
          >
            {isVideo ? (
              value || state.status === "done" ? (
                <>
                  <CheckCircle2 className="text-emerald-400" size={22} />
                  <span className="text-emerald-300">{state.fileName || "Video yuklangan"}</span>
                  <span className="text-gray-500">Almashtirish uchun bosing yoki yangi faylni tashlang</span>
                </>
              ) : (
                <>
                  <Film size={22} />
                  <span className="text-gray-300">Video faylni tashlang yoki tanlang</span>
                  <span>mp4, webm · 15 GB gacha</span>
                </>
              )
            ) : preview ? (
              <span className="inline-flex items-center gap-1.5 rounded-lg bg-white/15 px-3 py-1.5 text-white">
                <RefreshCw size={13} /> Almashtirish
              </span>
            ) : (
              <>
                <ImagePlus size={22} />
                <span className="text-gray-300">Rasmni tashlang, tanlang</span>
                <span>yoki Ctrl+V bilan qo&apos;ying</span>
              </>
            )}
          </div>
        )}

        {/* Upload progress */}
        {uploading && (
          <div className="absolute inset-0 flex flex-col items-center justify-center gap-2 bg-black/70 p-4 text-center">
            <Loader2 className="animate-spin text-orange-400" size={22} />
            <span className="text-xs text-white">{state.progress !== undefined ? `${Math.round(state.progress)}%` : "Yuklanmoqda…"}</span>
            <div className="h-1.5 w-full max-w-[220px] overflow-hidden rounded-full bg-white/10">
              <div className="h-full rounded-full bg-orange-500 transition-[width]" style={{ width: `${state.progress ?? 5}%` }} />
            </div>
            <span className="text-[10px] text-gray-400">
              {[
                state.uploadedMB !== undefined && state.totalMB ? `${state.uploadedMB.toFixed(1)} / ${state.totalMB.toFixed(1)} MB` : "",
                state.speed ? `${state.speed.toFixed(1)} MB/s` : "",
                fmtEta(state.eta),
              ]
                .filter(Boolean)
                .join(" · ")}
            </span>
          </div>
        )}

        {preview && !uploading && (
          <button
            type="button"
            onClick={(e) => {
              e.stopPropagation();
              onChange("");
              setState({ status: "idle" });
            }}
            className="absolute right-2 top-2 rounded-lg bg-black/70 p-1.5 text-gray-200 opacity-0 transition-opacity hover:text-red-400 group-hover:opacity-100"
            aria-label={`${label}ni olib tashlash`}
          >
            <Trash2 size={14} />
          </button>
        )}
        <input
          ref={inputRef}
          type="file"
          className="hidden"
          accept={isVideo ? "video/mp4,video/webm,video/ogg,.m3u8" : IMAGE_TYPES.join(",")}
          onChange={(e) => {
            const f = e.target.files?.[0];
            e.target.value = "";
            if (f) void start(f);
          }}
        />
      </div>

      {state.status === "error" && (
        <p className="mt-1.5 flex items-center gap-1.5 text-xs text-red-400">
          <AlertCircle size={13} /> {state.message}
          {lastFile.current && (
            <button type="button" onClick={() => lastFile.current && start(lastFile.current)} className="ml-1 underline hover:text-red-300">
              Qayta urinish
            </button>
          )}
        </p>
      )}
      {state.status === "done" && !isVideo && <p className="mt-1.5 flex items-center gap-1 text-xs text-emerald-400"><CheckCircle2 size={13} /> Yuklandi</p>}
      {hint && state.status !== "error" && <p className="mt-1.5 text-xs text-gray-500">{hint}</p>}

      {showUrl && !isVideo && (
        <input
          value={value}
          onChange={(e) => onChange(e.target.value.trim())}
          placeholder="https://… rasm manzili"
          className={`${inputCls} mt-2`}
        />
      )}
      {!isVideo && !showUrl && <span className="sr-only"><Upload /></span>}
    </div>
  );
}
