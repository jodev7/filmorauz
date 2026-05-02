import { normalizeMediaUrl } from "@/lib/image-utils";
import { localizeSingleGenre } from "@/lib/localization";
import { SITE_URL } from "@/lib/content-routes";

export function buildContentTitle(title: string): string {
  return `${title} o'zbek tilida online ko'rish — FilmoraUz`;
}

export function buildContentDescription(title: string, year?: number | null): string {
  const yearPart = year ? ` ${year}` : "";
  return `${title}${yearPart} o'zbek tilida HD sifatda online tomosha qiling. FilmoraUz.net`;
}

export function buildContentKeywords(input: {
  title: string;
  originalTitle?: string | null;
  uzbekTitle?: string | null;
  genres?: string[] | null;
  extra?: string[] | null;
}): string[] {
  const seen = new Set<string>();
  const values = [
    input.title,
    input.originalTitle || "",
    input.uzbekTitle || "",
    ...(input.genres || []).flatMap((genre) => [genre, localizeSingleGenre(genre)]),
    "o'zbek tilida",
    "online kino",
    "serial",
    "filmorauz",
    "filmorauznet",
    ...(input.extra || []),
  ];

  return values.filter(Boolean).filter((value) => {
    const key = value.trim().toLowerCase();
    if (!key || seen.has(key)) return false;
    seen.add(key);
    return true;
  });
}

export function pickSeoImage(...candidates: Array<string | undefined | null>): string {
  for (const candidate of candidates) {
    const normalized = normalizeMediaUrl(candidate, "");
    if (normalized) {
      return normalized;
    }
  }
  return `${SITE_URL}/og-image.jpg`;
}
