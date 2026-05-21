"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { localizeSingleGenre } from "@/lib/localization";
import { DEFAULT_GENRES } from "@/lib/genres";

// Lowercase English keys — matches DB/API format. Display is localized to Uzbek.
const GENRES = ["All", ...DEFAULT_GENRES];

interface GenreFilterProps {
  basePath?: string; // defaults to /movies for backward compatibility
}

export default function GenreFilter({ basePath = "/movies" }: GenreFilterProps) {
  const router = useRouter();
  const searchParams = useSearchParams();
  const activeGenre = searchParams.get("genre") || "All";

  const handleSelect = (genre: string) => {
    const params = new URLSearchParams(searchParams.toString());
    if (genre === "All") {
      params.delete("genre");
    } else {
      params.set("genre", genre);
    }
    params.delete("page"); // reset pagination on filter change
    const qs = params.toString();
    router.push(qs ? `${basePath}?${qs}` : basePath);
  };

  // Get localized display name for a genre (always Uzbek)
  const getDisplayName = (genre: string): string => {
    if (genre === "All") {
      return "Hammasi";
    }
    return localizeSingleGenre(genre);
  };

  return (
    <div className="flex flex-wrap gap-2">
      {GENRES.map((genre) => {
        const active =
          activeGenre === genre ||
          (genre === "All" && !searchParams.get("genre"));
        return (
          <button
            key={genre}
            onClick={() => handleSelect(genre)}
            className={`px-3 sm:px-4 py-1.5 rounded-full text-xs sm:text-sm font-medium transition-all whitespace-nowrap ${
              active
                ? "bg-brand-red text-white"
                : "bg-brand-card border border-brand-border text-gray-400 hover:text-white hover:border-gray-500"
            }`}
          >
            {getDisplayName(genre)}
          </button>
        );
      })}
    </div>
  );
}
