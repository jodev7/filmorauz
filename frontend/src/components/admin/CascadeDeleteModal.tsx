"use client";

import { useEffect, useState } from "react";
import { AlertTriangle, Loader2, X } from "lucide-react";

import type {
  CascadeDeleteB2Summary,
} from "@/lib/api";

export interface CascadeDeleteModalProps {
  /** Controls whether the modal is rendered. */
  open: boolean;
  /** Subject the user is deleting (movie/series/episode), shown for clarity. */
  kind: "movie" | "series";
  /** Title of the row being deleted, embedded in the warning copy. */
  title: string;
  /** Called when the user confirms; should perform the actual delete. */
  onConfirm: () => Promise<void>;
  /** Called when the user dismisses without confirming. */
  onClose: () => void;
}

/**
 * CascadeDeleteModal renders the destructive-action confirmation prompt
 * for movie and series deletes.
 *
 * The modal does NOT speak to the backend itself — it only renders the
 * warning copy, gates the confirm button on a typed-name match, and
 * delegates the actual deletion to the parent via onConfirm. After a
 * successful delete the parent is responsible for closing the modal
 * and refreshing the list.
 *
 * Why a typed confirmation: a single click on a confirm() dialog has
 * historically deleted whole catalogs by accident. Forcing the admin
 * to retype the title prevents the muscle-memory misclick path.
 */
export default function CascadeDeleteModal({
  open,
  kind,
  title,
  onConfirm,
  onClose,
}: CascadeDeleteModalProps) {
  const [confirmText, setConfirmText] = useState("");
  const [working, setWorking] = useState(false);
  const [error, setError] = useState<string | null>(null);

  // Reset state every time the modal is opened so a previous attempt's
  // error / typed text does not bleed into the new confirmation.
  useEffect(() => {
    if (open) {
      setConfirmText("");
      setError(null);
      setWorking(false);
    }
  }, [open]);

  if (!open) return null;

  const expectedConfirmation = title.trim();
  const canConfirm =
    expectedConfirmation.length > 0 &&
    confirmText.trim().toLowerCase() === expectedConfirmation.toLowerCase() &&
    !working;

  const subjectUz = kind === "movie" ? "kinoni" : "serialni";

  const handleConfirm = async () => {
    if (!canConfirm) return;
    setError(null);
    setWorking(true);
    try {
      await onConfirm();
    } catch (e) {
      setError(e instanceof Error ? e.message : "O'chirishda xato");
      setWorking(false);
      return;
    }
    setWorking(false);
  };

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4">
      <div className="w-full max-w-md bg-brand-card border border-red-500/30 rounded-xl shadow-2xl">
        <div className="flex items-start justify-between p-5 border-b border-brand-border">
          <div className="flex items-center gap-3">
            <div className="p-2 bg-red-500/15 rounded-lg">
              <AlertTriangle className="w-5 h-5 text-red-400" />
            </div>
            <h2 className="text-white font-semibold text-lg">
              {kind === "movie" ? "Kinoni o'chirish" : "Serialni o'chirish"}
            </h2>
          </div>
          <button
            onClick={onClose}
            disabled={working}
            className="text-gray-500 hover:text-white p-1 rounded transition-colors disabled:opacity-50"
            aria-label="Yopish"
          >
            <X className="w-5 h-5" />
          </button>
        </div>

        <div className="p-5 space-y-4 text-sm text-gray-300 leading-relaxed">
          <p>
            <strong className="text-white">&quot;{title}&quot;</strong>{" "}
            {subjectUz} o&apos;chirish:
          </p>
          <ul className="list-disc list-inside space-y-1 text-gray-400">
            <li>{kind === "movie" ? "Kino" : "Serial, sezonlar va epizodlar"} ma&apos;lumotlar bazasidan o&apos;chiriladi</li>
            <li>Barcha video fayllari (HLS/MP4) Backblaze B2 dan o&apos;chiriladi</li>
            <li>Poster, fon va boshqa rasmlar B2 dan o&apos;chiriladi</li>
            <li>Bu kontent uchun barcha kliplar (B2 + DB) o&apos;chiriladi</li>
            <li>Instagram va boshqa platformalar uchun rejalashtirilgan vazifalar bekor qilinadi</li>
          </ul>
          <p className="text-red-400 font-medium">
            Bu amalni bekor qilib bo&apos;lmaydi.
          </p>

          <div>
            <label className="block text-xs text-gray-500 mb-1.5">
              Tasdiqlash uchun nomini yozing:{" "}
              <span className="font-mono text-gray-300">{expectedConfirmation}</span>
            </label>
            <input
              type="text"
              value={confirmText}
              onChange={(e) => setConfirmText(e.target.value)}
              disabled={working}
              autoFocus
              className="w-full bg-brand-dark border border-brand-border rounded-lg px-3 py-2 text-white placeholder-gray-600 focus:outline-none focus:border-red-500 disabled:opacity-50"
              placeholder={expectedConfirmation}
            />
          </div>

          {error && (
            <div className="bg-red-500/10 border border-red-500/30 rounded-lg p-3 text-red-300 text-xs">
              {error}
            </div>
          )}
        </div>

        <div className="flex justify-end gap-2 p-5 border-t border-brand-border">
          <button
            onClick={onClose}
            disabled={working}
            className="px-4 py-2 text-sm text-gray-300 bg-brand-border rounded-lg hover:bg-brand-border/70 transition-colors disabled:opacity-50"
          >
            Bekor qilish
          </button>
          <button
            onClick={handleConfirm}
            disabled={!canConfirm}
            className="inline-flex items-center gap-2 px-4 py-2 text-sm font-medium text-white bg-red-600 rounded-lg hover:bg-red-700 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
          >
            {working && <Loader2 className="w-4 h-4 animate-spin" />}
            Butunlay o&apos;chirish
          </button>
        </div>
      </div>
    </div>
  );
}

/**
 * formatCascadeWarning returns a multi-line human-readable summary of a
 * partial-failure delete response. Returns null when there is nothing
 * worth showing the admin (success with no skipped or errored items).
 */
export function formatCascadeWarning(
  summary: CascadeDeleteB2Summary | undefined,
): string | null {
  if (!summary) return null;
  const errs = summary.errors ?? [];
  const skipped = summary.skipped ?? [];
  if (errs.length === 0 && skipped.length === 0) return null;
  const lines: string[] = [];
  lines.push(
    `O'chirildi: ${summary.files_deleted} ta fayl, ${(summary.prefixes_deleted ?? []).length} ta papka.`,
  );
  if (errs.length > 0) {
    lines.push(`Xatolar (${errs.length}):`);
    for (const e of errs.slice(0, 5)) {
      lines.push(`  • ${e}`);
    }
    if (errs.length > 5) lines.push(`  • ...va yana ${errs.length - 5} ta`);
  }
  if (skipped.length > 0) {
    lines.push(`O'tkazib yuborilgan (${skipped.length}):`);
    for (const s of skipped.slice(0, 5)) {
      lines.push(`  • ${s}`);
    }
    if (skipped.length > 5) lines.push(`  • ...va yana ${skipped.length - 5} ta`);
  }
  return lines.join("\n");
}
