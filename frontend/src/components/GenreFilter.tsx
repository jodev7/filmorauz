"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { localizeSingleGenre } from "@/lib/localization";

// Lowercase English keys — matches DB/API format. Display is localized to Uzbek.
const GENRES = [
  "All",
  "action",
  "drama",
  "comedy",
  "animation",
  "anime",
  "dorama",
  "thriller",
  "horror",
  "romance",
  "sci-fi",
  "documentary",
  "crime",
  "fantasy",
];

export default function GenreFilter() {
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
    router.push(`/movies?${params.toString()}`);
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
