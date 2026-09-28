"use client";

import Link from "next/link";
import { ThumbsUp, Trash2 } from "lucide-react";
import type { Review } from "@/lib/comments-api";
import { formatRelativeAddedTime } from "@/lib/movie-utils";
import { DEFAULT_AVATAR_PLACEHOLDER } from "@/lib/image-utils";
import MediaImage from "@/components/ui/MediaImage";
import { PremiumBadge } from "@/components/PremiumComponents";
import Stars from "./Stars";

/** A star-rated comment (formerly a separate "taqriz") inside the comments list. */
export default function ReviewItem({
  review,
  canVote,
  canStaffDelete,
  onHelpful,
  onDelete,
}: {
  review: Review;
  canVote: boolean;
  canStaffDelete: boolean;
  onHelpful: () => void;
  onDelete: () => void;
}) {
  return (
    <div className="glass-card rounded-2xl border border-white/10 p-4">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 flex-wrap items-center gap-2">
          <MediaImage
            src={review.user_avatar || DEFAULT_AVATAR_PLACEHOLDER}
            fallbackSrc={DEFAULT_AVATAR_PLACEHOLDER}
            alt={review.user_name}
            className="h-8 w-8 rounded-full object-cover"
          />
          <Link href={`/user/${review.user_id}`} className="font-medium text-white transition-colors hover:text-brand-red">
            {review.user_name}
          </Link>
          {review.user_premium && <PremiumBadge size="sm" showCrown />}
          <span className="inline-flex items-center gap-1.5 rounded-full bg-yellow-400/10 px-2 py-0.5">
            <Stars value={review.rating} size={13} />
            <span className="text-xs font-semibold text-yellow-300">{review.rating}/5</span>
          </span>
          <span className="text-sm text-gray-500">{formatRelativeAddedTime(review.created_at)}</span>
        </div>
        {(review.mine || canStaffDelete) && (
          <button type="button" onClick={onDelete} className="flex shrink-0 items-center gap-1 text-sm text-red-500 hover:underline">
            <Trash2 size={14} /> O&apos;chirish
          </button>
        )}
      </div>
      <p className="mb-2 whitespace-pre-line text-gray-300">{review.text}</p>
      <button
        type="button"
        onClick={onHelpful}
        disabled={!canVote || review.mine}
        aria-pressed={review.helpful_by_me}
        className={`inline-flex items-center gap-1.5 text-sm transition-colors disabled:cursor-default ${
          review.helpful_by_me ? "text-emerald-400" : "text-gray-400 hover:text-white disabled:hover:text-gray-400"
        }`}
      >
        <ThumbsUp size={14} fill={review.helpful_by_me ? "currentColor" : "none"} /> Foydali{review.helpful_count > 0 ? ` (${review.helpful_count})` : ""}
      </button>
    </div>
  );
}
