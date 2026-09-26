import Link from "next/link";
import { Sparkles, ChevronRight } from "lucide-react";

/** Entry point to /year from the profile overview. */
export default function YearReviewTeaser() {
  const year = new Date().getFullYear();
  return (
    <Link
      href="/year"
      className="group mb-8 flex items-center gap-4 overflow-hidden rounded-2xl border border-orange-500/25 bg-gradient-to-r from-orange-600/20 via-[#14141c] to-violet-700/20 p-5 transition-colors hover:border-orange-500/50"
    >
      <span className="flex h-12 w-12 shrink-0 items-center justify-center rounded-xl bg-orange-500/20">
        <Sparkles className="text-orange-300" size={22} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="block font-semibold text-white">{year}-yil yakuni</span>
        <span className="block text-sm text-gray-400">Qancha tomosha qildingiz, sevimli janringiz va top kinolaringiz</span>
      </span>
      <ChevronRight className="text-gray-500 transition-transform group-hover:translate-x-1" />
    </Link>
  );
}
