"use client";

import { useEffect, useState } from "react";
import { CalendarClock, Loader2, X } from "lucide-react";
import { Movie, adminCancelMovieSchedule, adminScheduleMovie } from "@/lib/api";

const MONTHS_SHORT = ["yan", "fev", "mar", "apr", "may", "iyn", "iyl", "avg", "sen", "okt", "noy", "dek"];

const pad = (n: number) => String(n).padStart(2, "0");

// "27-sen, 20:00" in the viewer's local time.
export function formatScheduleTime(iso?: string | null): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";
  return `${d.getDate()}-${MONTHS_SHORT[d.getMonth()]}, ${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

// Value for <input type="datetime-local"> (local time, minute precision).
function toLocalInput(d: Date): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`;
}

function at(dayOffset: number, hour: number): Date {
  const d = new Date();
  d.setDate(d.getDate() + dayOffset);
  d.setHours(hour, 0, 0, 0);
  return d;
}

function defaultTime(existing?: string | null): Date {
  if (existing) {
    const d = new Date(existing);
    if (!Number.isNaN(d.getTime())) return d;
  }
  // Next full hour, at least ~30 min away.
  const d = new Date(Date.now() + 30 * 60 * 1000);
  d.setMinutes(0, 0, 0);
  d.setHours(d.getHours() + 1);
  return d;
}

interface Props {
  movie: Movie | null;
  token: string;
  onClose: () => void;
  onChanged: (movieId: string, scheduledAt: string | null) => void;
  onError: (message: string) => void;
}

export default function ScheduleMovieModal({ movie, token, onClose, onChanged, onError }: Props) {
  const [value, setValue] = useState("");
  const [busy, setBusy] = useState<"save" | "cancel" | null>(null);

  useEffect(() => {
    if (movie) setValue(toLocalInput(defaultTime(movie.scheduled_publish_at)));
  }, [movie]);

  useEffect(() => {
    if (!movie) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !busy) onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [movie, busy, onClose]);

  if (!movie) return null;

  const chosen = value ? new Date(value) : null;
  const valid = !!chosen && !Number.isNaN(chosen.getTime()) && chosen.getTime() > Date.now() + 60 * 1000;
  const presets: { label: string; date: Date }[] = [
    { label: "Bugun 20:00", date: at(0, 20) },
    { label: "Ertaga 09:00", date: at(1, 9) },
    { label: "Ertaga 20:00", date: at(1, 20) },
    new Date().getDay() === 6
      ? { label: "Keyingi shanba 20:00", date: at(7, 20) }
      : { label: "Shanba 20:00", date: at(6 - new Date().getDay(), 20) },
  ].filter((p) => p.date.getTime() > Date.now() + 5 * 60 * 1000);

  const save = async () => {
    if (!chosen || !valid) return;
    setBusy("save");
    try {
      const iso = chosen.toISOString();
      await adminScheduleMovie(token, movie.id, iso);
      onChanged(movie.id, iso);
      onClose();
    } catch (err) {
      onError(err instanceof Error ? err.message : "Rejalashtirib bo'lmadi");
    } finally {
      setBusy(null);
    }
  };

  const cancelSchedule = async () => {
    setBusy("cancel");
    try {
      await adminCancelMovieSchedule(token, movie.id);
      onChanged(movie.id, null);
      onClose();
    } catch (err) {
      onError(err instanceof Error ? err.message : "Bekor qilib bo'lmadi");
    } finally {
      setBusy(null);
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center bg-black/70 p-4"
      onClick={() => !busy && onClose()}
      role="dialog"
      aria-modal="true"
      aria-labelledby="schedule-title"
    >
      <div
        className="w-full max-w-md rounded-2xl border border-brand-border bg-brand-card p-5 shadow-2xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="mb-4 flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h2 id="schedule-title" className="flex items-center gap-2 text-base font-semibold text-white">
              <CalendarClock size={18} className="text-sky-400" />
              Nashrni rejalashtirish
            </h2>
            <p className="mt-1 truncate text-sm text-gray-400">{movie.title}</p>
          </div>
          <button
            onClick={onClose}
            disabled={!!busy}
            className="rounded-lg p-1.5 text-gray-500 hover:bg-white/5 hover:text-white"
            aria-label="Yopish"
          >
            <X size={16} />
          </button>
        </div>

        {movie.scheduled_publish_at && (
          <p className="mb-3 rounded-lg border border-sky-500/30 bg-sky-500/10 px-3 py-2 text-xs text-sky-200">
            Hozir rejalashtirilgan: <b>{formatScheduleTime(movie.scheduled_publish_at)}</b>
          </p>
        )}
        {movie.schedule_error && (
          <p className="mb-3 rounded-lg border border-red-500/30 bg-red-500/10 px-3 py-2 text-xs text-red-300">
            Oldingi avto-nashr xatosi: {movie.schedule_error}
          </p>
        )}

        <label className="mb-1.5 block text-xs font-medium text-gray-400" htmlFor="schedule-at">
          E&apos;lon qilinish vaqti
        </label>
        <input
          id="schedule-at"
          type="datetime-local"
          value={value}
          min={toLocalInput(new Date(Date.now() + 2 * 60 * 1000))}
          onChange={(e) => setValue(e.target.value)}
          className="w-full rounded-lg border border-brand-border bg-brand-dark px-3 py-2.5 text-sm text-white [color-scheme:dark] focus:border-sky-500 focus:outline-none"
        />
        {presets.length > 0 && (
          <div className="mt-2 flex flex-wrap gap-1.5">
            {presets.map((p) => (
              <button
                key={p.label}
                type="button"
                onClick={() => setValue(toLocalInput(p.date))}
                className="rounded-full border border-brand-border px-2.5 py-1 text-xs text-gray-300 hover:border-sky-500/60 hover:text-white"
              >
                {p.label}
              </button>
            ))}
          </div>
        )}
        {!valid && value && (
          <p className="mt-2 text-xs text-amber-400">Vaqt kamida bir necha daqiqa keyin bo&apos;lishi kerak.</p>
        )}
        <p className="mt-3 text-xs leading-relaxed text-gray-500">
          Belgilangan vaqtda kino avtomatik tasdiqlanadi, saytda chiqadi va Telegram kanal(lar)ga post yuboriladi.
          Vaqt qurilmangiz vaqt mintaqasida.
        </p>

        <div className="mt-5 flex items-center justify-between gap-2">
          {movie.scheduled_publish_at ? (
            <button
              onClick={cancelSchedule}
              disabled={!!busy}
              className="inline-flex items-center gap-1.5 rounded-lg px-3 py-2 text-sm text-red-400 hover:bg-red-500/10 disabled:opacity-50"
            >
              {busy === "cancel" && <Loader2 size={14} className="animate-spin" />}
              Rejani bekor qilish
            </button>
          ) : (
            <span />
          )}
          <div className="flex gap-2">
            <button
              onClick={onClose}
              disabled={!!busy}
              className="rounded-lg border border-brand-border px-3 py-2 text-sm text-gray-300 hover:bg-white/5 disabled:opacity-50"
            >
              Yopish
            </button>
            <button
              onClick={save}
              disabled={!valid || !!busy}
              className="inline-flex items-center gap-1.5 rounded-lg bg-sky-600 px-4 py-2 text-sm font-medium text-white hover:bg-sky-500 disabled:opacity-50"
            >
              {busy === "save" && <Loader2 size={14} className="animate-spin" />}
              Saqlash
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
