"use client";

import { memo, useRef, useState } from "react";
import { Film, ImagePlus, Loader2, Trash2, UploadCloud } from "lucide-react";
import { uploadAdMedia } from "@/lib/api";
import { normalizeMediaUrl } from "@/lib/image-utils";

const IMAGE_MIME = "image/jpeg,image/png,image/webp,image/gif";
const VIDEO_MIME = "video/mp4,video/webm,video/quicktime";

function fileKind(file: File): "image" | "video" | null {
  if (file.type.startsWith("image/")) return "image";
  if (file.type.startsWith("video/")) return "video";
  const ext = file.name.split(".").pop()?.toLowerCase() ?? "";
  if (["jpg", "jpeg", "png", "webp", "gif"].includes(ext)) return "image";
  if (["mp4", "webm", "mov"].includes(ext)) return "video";
  return null;
}

export function isVideoUrl(url: string, type?: string): boolean {
  if (type === "video") return true;
  if (type === "image") return false;
  return /\.(mp4|webm|mov)(\?|$)/i.test(url);
}

/** Longest allowed in-player video ad, in seconds (kept in sync with VideoPlayer / WatchPageClient). */
export { PLAYER_AD_MAX_SECONDS, validatePlayerAdVideo } from "@/lib/player-ad-media-validation";

/**
 * One creative slot: drop zone + preview at the slot's real aspect ratio.
 * Upload state lives here, so an upload doesn't re-render the whole editor.
 */
function AdMediaSlot({
  slot,
  label,
  size,
  aspect,
  value,
  mediaType,
  image = true,
  video = false,
  validate,
  token,
  onChange,
  onBusy,
}: {
  /** Passed back to onChange so the parent can use one stable callback. */
  slot: string;
  label: string;
  size: string;
  /** Tailwind aspect class, e.g. "aspect-[4/1]" */
  aspect: string;
  value: string;
  mediaType?: "image" | "video";
  image?: boolean;
  video?: boolean;
  validate?: (file: File) => Promise<string | null>;
  token: string | null;
  onChange: (slot: string, url: string, type: "image" | "video") => void;
  onBusy?: (slot: string, busy: boolean) => void;
}) {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [drag, setDrag] = useState(false);
  const input = useRef<HTMLInputElement>(null);

  const handle = async (file?: File | null) => {
    if (!file || !token) return;
    setError("");
    const kind = fileKind(file);
    if (!kind) return setError("Bu fayl turi qo'llab-quvvatlanmaydi");
    if (kind === "image" && !image) return setError("Bu joyga faqat video yuklanadi");
    if (kind === "video" && !video) return setError("Bu joyga faqat rasm yuklanadi");
    if (validate) {
      const msg = await validate(file);
      if (msg) return setError(msg);
    }
    setBusy(true);
    onBusy?.(slot, true);
    try {
      const url = (await uploadAdMedia(token, file, kind)) || "";
      onChange(slot, url, kind);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Yuklab bo'lmadi");
    } finally {
      setBusy(false);
      onBusy?.(slot, false);
    }
  };

  const src = value ? normalizeMediaUrl(value) : "";
  const showVideo = !!value && isVideoUrl(value, mediaType);
  const accept = [image && IMAGE_MIME, video && VIDEO_MIME].filter(Boolean).join(",");
  const formats = [image && "JPG, PNG, WEBP, GIF", video && "MP4, WEBM, MOV"].filter(Boolean).join(" · ");

  return (
    <div>
      <div className="mb-1.5 flex items-baseline justify-between gap-2">
        <span className="text-xs font-medium text-gray-300">{label}</span>
        <span className="text-[10px] text-gray-600">{size}</span>
      </div>
      <div
        onDragOver={(e) => {
          e.preventDefault();
          setDrag(true);
        }}
        onDragLeave={() => setDrag(false)}
        onDrop={(e) => {
          e.preventDefault();
          setDrag(false);
          void handle(e.dataTransfer.files?.[0]);
        }}
        className={`group relative ${aspect} w-full overflow-hidden rounded-xl border transition ${
          value ? "border-white/10" : drag ? "border-orange-500 bg-orange-500/5" : "border-dashed border-white/15 bg-black/20 hover:border-white/30"
        }`}
      >
        {value ? (
          <>
            {showVideo ? (
              <video src={src} muted loop playsInline autoPlay className="h-full w-full object-cover" />
            ) : (
              // eslint-disable-next-line @next/next/no-img-element
              <img src={src} alt="" className="h-full w-full object-cover" loading="lazy" />
            )}
            <div className="absolute inset-0 flex items-center justify-center gap-2 bg-black/60 opacity-0 transition group-hover:opacity-100">
              <button type="button" onClick={() => input.current?.click()} className="rounded-lg bg-white/10 px-2.5 py-1.5 text-xs text-white hover:bg-white/20">
                Almashtirish
              </button>
              <button type="button" onClick={() => onChange(slot, "", image ? "image" : "video")} className="rounded-lg bg-red-500/20 p-1.5 text-red-300 hover:bg-red-500/30" aria-label="O'chirish">
                <Trash2 size={14} />
              </button>
            </div>
          </>
        ) : (
          <button type="button" onClick={() => input.current?.click()} disabled={busy} className="flex h-full min-h-[56px] w-full flex-col items-center justify-center gap-1 px-2 text-center">
            {busy ? (
              <Loader2 size={18} className="animate-spin text-orange-400" />
            ) : video && !image ? (
              <Film size={18} className="text-gray-500 group-hover:text-gray-300" />
            ) : drag ? (
              <UploadCloud size={18} className="text-orange-400" />
            ) : (
              <ImagePlus size={18} className="text-gray-500 group-hover:text-gray-300" />
            )}
            <span className="text-[11px] text-gray-500">{busy ? "Yuklanmoqda..." : "Tanlang yoki tashlang"}</span>
          </button>
        )}
        {busy && value && (
          <div className="absolute inset-0 flex items-center justify-center bg-black/60">
            <Loader2 size={18} className="animate-spin text-orange-400" />
          </div>
        )}
      </div>
      <input
        ref={input}
        type="file"
        accept={accept}
        className="hidden"
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          void handle(f);
        }}
      />
      {error ? <p className="mt-1 text-[11px] text-red-400">{error}</p> : <p className="mt-1 text-[10px] text-gray-600">{formats}</p>}
    </div>
  );
}

export default memo(AdMediaSlot);
