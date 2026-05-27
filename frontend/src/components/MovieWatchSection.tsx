"use client";

import dynamic from "next/dynamic";
import { useWatchPlayer } from "@/lib/watch-player-context";
import type { Movie } from "@/lib/api";

// The player bundle (video.js, HLS, overlay ad) is heavy, so it's only loaded
// once the user actually opens the player — keeping the movie detail page
// light for the SEO/browse case.
const WatchPageClient = dynamic(() => import("@/components/watch/WatchPageClient"), {
  ssr: false,
  loading: () => (
    <div className="max-w-6xl mx-auto px-3 sm:px-4">
      <div className="flex aspect-video items-center justify-center rounded-xl bg-gray-900">
        <div className="h-10 w-10 animate-spin rounded-full border-2 border-white border-t-transparent" />
      </div>
    </div>
  ),
});

export default function MovieWatchSection({ movie }: { movie: Movie }) {
  const { open } = useWatchPlayer();

  return (
    <div id="player" className="max-w-7xl mx-auto px-4 scroll-mt-20">
      {open && (
        <WatchPageClient movie={movie} progressTargetId={movie._id || movie.id} embedded />
      )}
    </div>
  );
}
