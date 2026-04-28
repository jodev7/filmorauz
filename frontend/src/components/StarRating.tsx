"use client";

import { useState, useEffect } from "react";
import { Star } from "lucide-react";
import { setRating, deleteRating, getRatingSummary, RatingSummary, setSeriesRating, deleteSeriesRating, getSeriesRatingSummary, setEpisodeRating, deleteEpisodeRating, getEpisodeRatingSummary } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

interface StarRatingProps {
  movieId?: string;
  seriesId?: string;
  episodeId?: string;
  initialData?: RatingSummary;
  onRatingChange?: (summary: RatingSummary) => void;
  readOnly?: boolean;
}

export default function StarRating({ movieId, seriesId, episodeId, initialData, onRatingChange, readOnly }: StarRatingProps) {
  const { token, isAuthenticated } = useAuth();
  const [rating, setRatingState] = useState<number>(0);
  const [hoverRating, setHoverRating] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [summary, setSummary] = useState<RatingSummary | null>(initialData || null);

  // Determine kind
  const isEpisode = !!episodeId;
  const isSeries = !!seriesId && !isEpisode;
  const targetId = episodeId || seriesId || movieId;
  const canInteract = isAuthenticated && !readOnly;

  useEffect(() => {
    if (initialData) {
      setSummary(initialData);
      if (initialData.user_rating) {
        setRatingState(initialData.user_rating);
      }
    } else if (targetId) {
      const fetchRating = isEpisode
        ? getEpisodeRatingSummary(targetId, token || undefined)
        : isSeries
        ? getSeriesRatingSummary(targetId, token || undefined)
        : getRatingSummary(targetId, token || undefined);
      
      fetchRating
        .then((data) => {
          setSummary(data);
          if (data.user_rating) {
            setRatingState(data.user_rating);
          }
        })
        .catch(console.error);
    }
  }, [targetId, initialData, token, isSeries, isEpisode]);

  const handleClick = async (value: number) => {
    if (!canInteract || !token || loading || !targetId) return;

    setLoading(true);
    try {
      if (value === rating) {
        const data = isEpisode
          ? await deleteEpisodeRating(token, targetId)
          : isSeries
          ? await deleteSeriesRating(token, targetId)
          : await deleteRating(token, targetId);
        setRatingState(0);
        setSummary(data);
        onRatingChange?.(data);
      } else {
        const data = isEpisode
          ? await setEpisodeRating(token, targetId, value)
          : isSeries
          ? await setSeriesRating(token, targetId, value)
          : await setRating(token, targetId, value);
        setRatingState(value);
        setSummary(data);
        onRatingChange?.(data);
      }
    } catch (error) {
      console.error("Failed to set rating:", error);
    } finally {
      setLoading(false);
    }
  };

  const displayRating = hoverRating || rating;

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-1">
        {[1, 2, 3, 4, 5].map((value) => (
          <button
            key={value}
            type="button"
            disabled={!canInteract || loading}
            onClick={() => handleClick(value)}
            onMouseEnter={() => canInteract && setHoverRating(value)}
            onMouseLeave={() => setHoverRating(0)}
            className={`transition-transform ${
              canInteract ? "cursor-pointer hover:scale-110" : "cursor-not-allowed"
            }`}
            aria-label={`Rate ${value} stars`}
          >
            <Star
              size={24}
              className={`transition-colors ${
                value <= displayRating
                  ? "fill-yellow-400 text-yellow-400"
                  : "fill-transparent text-gray-400"
              }`}
            />
          </button>
        ))}
        {loading && (
          <span className="ml-2 text-sm text-gray-400">...</span>
        )}
      </div>

      {/* Rating summary */}
      {summary && (
        <div className="flex items-center gap-2 text-sm text-gray-400">
          {summary.rating_avg > 0 ? (
            <>
              <span className="flex items-center gap-1">
                <Star size={14} className="fill-yellow-400 text-yellow-400" />
                <span className="font-medium text-white">
                  {summary.rating_avg.toFixed(1)}
                </span>
              </span>
              <span>({summary.rating_count} {summary.rating_count === 1 ? "baho" : "baho"})</span>
            </>
          ) : (
            <span>Hali baholanmagan</span>
          )}
          {!isAuthenticated && (
            <span className="text-xs text-gray-500">(baholash uchun kiring)</span>
          )}
        </div>
      )}
    </div>
  );
}
