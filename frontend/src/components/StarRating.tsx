"use client";

import { useState, useEffect } from "react";
import { Star } from "lucide-react";
import { setRating, deleteRating, getRatingSummary, RatingSummary, setSeriesRating, deleteSeriesRating, getSeriesRatingSummary } from "@/lib/api";
import { useAuth } from "@/lib/auth-context";

interface StarRatingProps {
  movieId?: string;
  seriesId?: string;
  initialData?: RatingSummary;
  onRatingChange?: (summary: RatingSummary) => void;
}

export default function StarRating({ movieId, seriesId, initialData, onRatingChange }: StarRatingProps) {
  const { token, isAuthenticated } = useAuth();
  const [rating, setRatingState] = useState<number>(0);
  const [hoverRating, setHoverRating] = useState<number>(0);
  const [loading, setLoading] = useState(false);
  const [summary, setSummary] = useState<RatingSummary | null>(initialData || null);

  // Determine if this is for a series or movie
  const isSeries = !!seriesId;
  const targetId = seriesId || movieId;

  useEffect(() => {
    if (initialData) {
      setSummary(initialData);
      if (initialData.user_rating) {
        setRatingState(initialData.user_rating);
      }
    } else if (targetId) {
      // Fetch rating summary from API with user token if available
      const fetchRating = isSeries 
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
  }, [targetId, initialData, token, isSeries]);

  const handleClick = async (value: number) => {
    if (!isAuthenticated || !token || loading || !targetId) return;

    setLoading(true);
    try {
      // If clicking on the same rating, remove the rating
      if (value === rating) {
        const data = isSeries 
          ? await deleteSeriesRating(token, targetId)
          : await deleteRating(token, targetId);
        setRatingState(0);
        setSummary(data);
        onRatingChange?.(data);
      } else {
        const data = isSeries 
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
            disabled={!isAuthenticated || loading}
            onClick={() => handleClick(value)}
            onMouseEnter={() => isAuthenticated && setHoverRating(value)}
            onMouseLeave={() => setHoverRating(0)}
            className={`transition-transform ${
              isAuthenticated ? "cursor-pointer hover:scale-110" : "cursor-not-allowed"
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
              <span>({summary.rating_count} {summary.rating_count === 1 ? "rating" : "ratings"})</span>
            </>
          ) : (
            <span>No ratings yet</span>
          )}
          {!isAuthenticated && (
            <span className="text-xs text-gray-500">(login to rate)</span>
          )}
        </div>
      )}
    </div>
  );
}
