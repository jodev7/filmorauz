"use client";

import { useCallback, useEffect, useState } from "react";
import Link from "next/link";
import { Star, ThumbsUp, Trash2, Pencil, PenLine } from "lucide-react";
import { useAuth } from "@/lib/auth-context";
import {
  getReviews,
  saveReview,
  deleteMyReview,
  toggleReviewHelpful,
  adminDeleteReview,
  Review,
} from "@/lib/comments-api";
import { formatRelativeAddedTime } from "@/lib/movie-utils";
import { DEFAULT_AVATAR_PLACEHOLDER } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import { PremiumBadge } from "./PremiumComponents";
import { isStaffRole } from "@/lib/roles";

const MIN_LEN = 10;
const MAX_LEN = 500;

function Stars({ value, onChange, size = 16 }: { value: number; onChange?: (v: number) => void; size?: number }) {
  const [hover, setHover] = useState(0);
  const shown = hover || value;
  return (
    <div className="inline-flex items-center gap-0.5" role={onChange ? "radiogroup" : "img"} aria-label={`${value} / 5 yulduz`}>
      {[1, 2, 3, 4, 5].map((n) => {
        const icon = <Star size={size} className={n <= shown ? "text-yellow-400" : "text-gray-600"} fill={n <= shown ? "currentColor" : "none"} />;
        return onChange ? (
          <button
            key={n}
            type="button"
            role="radio"
            aria-checked={value === n}
            aria-label={`${n} yulduz`}
            onMouseEnter={() => setHover(n)}
            onMouseLeave={() => setHover(0)}
            onClick={() => onChange(n)}
            className="p-0.5"
          >
            {icon}
          </button>
        ) : (
          <span key={n}>{icon}</span>
        );
      })}
    </div>
  );
}

/**
 * Short written reviews ("qisqa taqriz") with a star rating. Saving a review
 * also sets the viewer's star rating for the title. Most helpful first.
 */
export default function Reviews({ targetType, targetId }: { targetType: "movie" | "series"; targetId: string }) {
  const { token, isAuthenticated, user } = useAuth();
  const [reviews, setReviews] = useState<Review[]>([]);
  const [total, setTotal] = useState(0);
  const [sort, setSort] = useState<"helpful" | "new">("helpful");
  const [loading, setLoading] = useState(true);
  const [editing, setEditing] = useState(false);
  const [rating, setRating] = useState(0);
  const [text, setText] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const isStaff = isStaffRole(user?.role);

  const load = useCallback(() => {
    setLoading(true);
    getReviews(targetType, targetId, sort, token)
      .then((r) => {
        setReviews(r.data || []);
        setTotal(r.total || 0);
      })
      .catch(() => {})
      .finally(() => setLoading(false));
  }, [targetType, targetId, sort, token]);

  useEffect(() => {
    load();
  }, [load]);

  const mine = reviews.find((r) => r.mine);

  const startEdit = () => {
    setRating(mine?.rating ?? 0);
    setText(mine?.text ?? "");
    setError(null);
    setEditing(true);
  };

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!token) return;
    const trimmed = text.trim();
    if (rating < 1) return setError("Yulduzcha bilan baho bering");
    if (trimmed.length < MIN_LEN) return setError(`Kamida ${MIN_LEN} ta belgi yozing`);
    setSaving(true);
    setError(null);
    try {
      await saveReview(token, targetType, targetId, rating, trimmed);
      setEditing(false);
      load();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Xatolik");
    } finally {
      setSaving(false);
    }
  };

  const removeMine = async () => {
    if (!token || !window.confirm("Taqrizingiz o'chirilsinmi?")) return;
    await deleteMyReview(token, targetType, targetId).catch(() => {});
    load();
  };

  const toggleHelpful = async (r: Review) => {
    if (!token || r.mine) return;
    setReviews((prev) =>
      prev.map((x) =>
        x.id === r.id ? { ...x, helpful_by_me: !x.helpful_by_me, helpful_count: x.helpful_count + (x.helpful_by_me ? -1 : 1) } : x
      )
    );
    toggleReviewHelpful(token, r.id).catch(() => load());
  };

  const staffDelete = async (r: Review) => {
    if (!token || !window.confirm(`${r.user_name} taqrizini o'chirasizmi?`)) return;
    await adminDeleteReview(token, r.id).catch(() => {});
    load();
  };

  return (
    <div className="mt-8 pt-8 border-t border-white/10" id="taqrizlar">
      <div className="mb-5 flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-2xl font-display text-white">
          Taqrizlar {total > 0 && <span className="text-base text-gray-500">({total})</span>}
        </h2>
        {total > 1 && (
          <div className="flex rounded-lg border border-white/10 p-0.5 text-xs" role="group" aria-label="Saralash">
            {(["helpful", "new"] as const).map((s) => (
              <button
                key={s}
                onClick={() => setSort(s)}
                aria-pressed={sort === s}
                className={`rounded-md px-2.5 py-1 ${sort === s ? "bg-white text-black" : "text-gray-400 hover:text-white"}`}
              >
                {s === "helpful" ? "Foydali" : "Yangi"}
              </button>
            ))}
          </div>
        )}
      </div>

      {/* Write / edit */}
      {isAuthenticated && token ? (
        editing ? (
          <form onSubmit={submit} className="mb-6 glass-card border border-white/10 rounded-2xl p-4">
            <div className="mb-2 flex items-center gap-3">
              <span className="text-sm text-gray-400">Bahoingiz:</span>
              <Stars value={rating} onChange={setRating} size={22} />
            </div>
            <textarea
              value={text}
              onChange={(e) => setText(e.target.value.slice(0, MAX_LEN))}
              rows={3}
              placeholder="Kino haqida 1–2 jumla: nimasi yoqdi, nimasi yoqmadi? (spoylersiz)"
              className="w-full resize-none rounded-xl border border-white/15 bg-black/50 p-3 text-white placeholder-gray-500 focus:border-brand-red focus:outline-none"
            />
            <div className="mt-2 flex flex-wrap items-center gap-3">
              <button type="submit" disabled={saving} className="rounded-lg bg-brand-red px-4 py-2 text-sm font-semibold text-white hover:bg-orange-700 disabled:opacity-50">
                {saving ? "..." : mine ? "Yangilash" : "Joylash"}
              </button>
              <button type="button" onClick={() => setEditing(false)} className="text-sm text-gray-400 hover:text-white">
                Bekor qilish
              </button>
              <span className={`ml-auto text-xs ${text.trim().length < MIN_LEN ? "text-gray-500" : "text-gray-400"}`}>
                {text.trim().length}/{MAX_LEN}
              </span>
            </div>
            {error && <p className="mt-2 text-sm text-red-400">{error}</p>}
          </form>
        ) : !mine ? (
          <button
            onClick={startEdit}
            className="mb-6 inline-flex items-center gap-2 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-gray-200 hover:border-brand-red"
          >
            <PenLine size={16} /> Taqriz yozish
          </button>
        ) : null
      ) : (
        <p className="mb-6 text-sm text-gray-500">Taqriz yozish uchun tizimga kiring.</p>
      )}

      {loading && reviews.length === 0 ? (
        <p className="text-gray-500 text-sm">Yuklanmoqda...</p>
      ) : reviews.length === 0 ? (
        <p className="text-gray-500 text-sm">Hali taqrizlar yo&apos;q — birinchi bo&apos;ling!</p>
      ) : (
        <ul className="space-y-3">
          {reviews.map((r) => (
            <li key={r.id} className={`glass-card rounded-2xl border p-4 ${r.mine ? "border-brand-red/40" : "border-white/10"}`}>
              <div className="mb-2 flex flex-wrap items-center gap-2">
                <MediaImage src={r.user_avatar || DEFAULT_AVATAR_PLACEHOLDER} fallbackSrc={DEFAULT_AVATAR_PLACEHOLDER} alt={r.user_name} className="h-8 w-8 rounded-full object-cover" />
                <Link href={`/user/${r.user_id}`} className="font-medium text-white hover:text-brand-red">
                  {r.user_name}
                </Link>
                {r.user_premium && <PremiumBadge size="sm" showCrown />}
                {r.mine && <span className="rounded-full bg-brand-red/20 px-2 py-0.5 text-[10px] text-brand-red">Sizning taqrizingiz</span>}
                <Stars value={r.rating} size={14} />
                <span className="text-xs text-gray-500">{formatRelativeAddedTime(r.updated_at || r.created_at)}</span>
              </div>
              <p className="text-gray-300 whitespace-pre-line">{r.text}</p>
              <div className="mt-2 flex items-center gap-4 text-sm">
                <button
                  onClick={() => toggleHelpful(r)}
                  disabled={!isAuthenticated || r.mine}
                  className={`inline-flex items-center gap-1.5 ${r.helpful_by_me ? "text-emerald-400" : "text-gray-400 hover:text-white"} disabled:cursor-default disabled:hover:text-gray-400`}
                  aria-pressed={r.helpful_by_me}
                >
                  <ThumbsUp size={14} fill={r.helpful_by_me ? "currentColor" : "none"} /> Foydali {r.helpful_count > 0 && `(${r.helpful_count})`}
                </button>
                {r.mine && !editing && (
                  <>
                    <button onClick={startEdit} className="inline-flex items-center gap-1 text-gray-400 hover:text-white">
                      <Pencil size={13} /> Tahrirlash
                    </button>
                    <button onClick={removeMine} className="inline-flex items-center gap-1 text-gray-400 hover:text-red-400">
                      <Trash2 size={13} /> O&apos;chirish
                    </button>
                  </>
                )}
                {isStaff && !r.mine && (
                  <button onClick={() => staffDelete(r)} className="ml-auto inline-flex items-center gap-1 text-xs text-red-400/80 hover:text-red-400">
                    <Trash2 size={12} /> Moderatsiya: o&apos;chirish
                  </button>
                )}
              </div>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}
