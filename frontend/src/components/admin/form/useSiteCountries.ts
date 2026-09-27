"use client";

import { useEffect, useState } from "react";
import { getMovieFilterFacets } from "@/lib/api";
import { COUNTRY_SUGGESTIONS, countryKey, splitCountries } from "./constants";

let cache: string[] | null = null;

/**
 * Countries already used on the site (most common first), in the same
 * Uzbek form the site shows them. Falls back to a short built-in list.
 */
export function useSiteCountries(): string[] {
  const [list, setList] = useState<string[]>(cache ?? COUNTRY_SUGGESTIONS);
  useEffect(() => {
    if (cache) return;
    let alive = true;
    getMovieFilterFacets()
      .then((f) => {
        const out: string[] = [];
        for (const raw of f.countries || []) {
          for (const c of splitCountries(raw)) if (!out.some((x) => countryKey(x) === countryKey(c))) out.push(c);
        }
        for (const c of COUNTRY_SUGGESTIONS) if (!out.some((x) => countryKey(x) === countryKey(c))) out.push(c);
        cache = out;
        if (alive) setList(out);
      })
      .catch(() => {});
    return () => {
      alive = false;
    };
  }, []);
  return list;
}
