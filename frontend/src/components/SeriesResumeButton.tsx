"use client";

import Link from "next/link";
import { Play, RotateCcw } from "lucide-react";
import { useSeriesProgress } from "@/lib/use-series-progress";
import { buildBestEpisodePath } from "@/lib/content-routes";

interface Props {
  seriesId: string;
  seriesSlug: string;
  /** First episode, used when nothing has been watched yet. */
  firstEpisode?: { id: string; seasonNumber: number; episodeNumber: number } | null;
}

/**
 * Main call-to-action on the series page: "continue S2 E5" when the user is
 * mid-series, otherwise "start from episode 1".
 */
export default function SeriesResumeButton({ seriesId, seriesSlug, firstEpisode }: Props) {
  const { progress } = useSeriesProgress(seriesId);
  const resume = progress?.resume;
  const allDone = !!progress && progress.total > 0 && progress.watched >= progress.total;

  let href: string | null = null;
  let label = "";
  let sub = "";
  if (resume) {
    href = buildBestEpisodePath({
      episodeId: resume.episode_id,
      seriesSlug,
      seasonNumber: resume.season_number,
      episodeNumber: resume.episode_number,
    });
    label = resume.mode === "continue" ? "Davom ettirish" : "Keyingi qism";
    sub = `${resume.season_number}-fasl, ${resume.episode_number}-qism`;
  } else if (firstEpisode) {
    href = buildBestEpisodePath({
      episodeId: firstEpisode.id,
      seriesSlug,
      seasonNumber: firstEpisode.seasonNumber,
      episodeNumber: firstEpisode.episodeNumber,
    });
    label = allDone ? "Qaytadan ko'rish" : "Tomosha qilish";
    sub = `${firstEpisode.seasonNumber}-fasl, ${firstEpisode.episodeNumber}-qism`;
  }
  if (!href) return null;

  return (
    <Link
      href={href}
      className="group inline-flex items-center gap-3 rounded-xl bg-brand-red px-5 py-3 text-white shadow-lg shadow-brand-red/20 transition-colors hover:bg-orange-600"
    >
      <span className="flex h-9 w-9 items-center justify-center rounded-full bg-white/20">
        {allDone && !resume ? <RotateCcw size={16} /> : <Play size={16} fill="white" className="ml-0.5" />}
      </span>
      <span className="flex flex-col leading-tight">
        <span className="text-sm font-semibold">{label}</span>
        <span className="text-xs text-white/80">{sub}</span>
      </span>
      {resume?.mode === "continue" && resume.progress_percent > 0 && (
        <span className="ml-1 h-1.5 w-16 overflow-hidden rounded-full bg-white/25" aria-label={`${Math.round(resume.progress_percent)}%`}>
          <span className="block h-full bg-white" style={{ width: `${Math.min(100, resume.progress_percent)}%` }} />
        </span>
      )}
    </Link>
  );
}
