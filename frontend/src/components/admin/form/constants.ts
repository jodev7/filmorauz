import { localizeSingleCountry } from "@/lib/localization";

export const QUALITIES = ["480p", "720p", "1080p", "1080p Ultra", "4K"];

/**
 * Maps whatever the DB / parser stored ("Full HD", "HD", "1080P", "2160p",
 * "BluRay"...) onto one of QUALITIES so the picker shows it as selected.
 * Empty stays empty; unknown values are returned as-is.
 */
export function normalizeQuality(raw?: string | null): string {
  const q = (raw || "").trim();
  if (!q) return "";
  const exact = QUALITIES.find((x) => x.toLowerCase() === q.toLowerCase());
  if (exact) return exact;
  const s = q.toLowerCase().replace(/[\s_-]+/g, " ");
  if (/2160|4k|uhd/.test(s)) return "4K";
  if (/ultra/.test(s)) return "1080p Ultra";
  if (/1080|full hd|fhd|fullhd|blu ?ray|bdrip/.test(s)) return "1080p";
  if (/720|\bhd\b|web/.test(s)) return "720p";
  if (/480|360|\bsd\b|dvd/.test(s)) return "480p";
  return q;
}

/** Best quality from a list of generated renditions ("1080p", "720p"...). */
export function bestQuality(list?: string[] | null): string {
  if (!list?.length) return "";
  const norm = list.map(normalizeQuality);
  for (let i = QUALITIES.length - 1; i >= 0; i--) if (norm.includes(QUALITIES[i])) return QUALITIES[i];
  return norm[0] || "";
}

/** Renditions actually in storage (generated HLS folders), deduped and normalized. */
export function storageQualityList(...lists: (string[] | null | undefined)[]): string[] {
  const out: string[] = [];
  for (const list of lists) for (const q of list || []) {
    const n = normalizeQuality(q);
    if (n && !out.includes(n)) out.push(n);
  }
  const rank = (q: string) => (QUALITIES.indexOf(q) < 0 ? -1 : QUALITIES.indexOf(q));
  return out.sort((a, b) => rank(a) - rank(b));
}

/** Segmented options, plus the current value when it is not a standard one. */
export function qualityOptions(current?: string) {
  const list = current && !QUALITIES.includes(current) ? [...QUALITIES, current] : QUALITIES;
  return list.map((q) => ({ value: q, label: q }));
}

/** Fallback when the site has no countries yet (Uzbek, as shown on the site). */
export const COUNTRY_SUGGESTIONS = [
  "Amerika Qo'shma Shtatlari",
  "Janubiy Koreya",
  "Turkiya",
  "Hindiston",
  "Rossiya",
  "Buyuk Britaniya",
  "Yaponiya",
  "Fransiya",
  "Xitoy",
  "O'zbekiston",
];

/** "USA, UK" → ["Amerika Qo'shma Shtatlari", "Buyuk Britaniya"] (deduped). */
export function splitCountries(raw?: string | null): string[] {
  const out: string[] = [];
  for (const part of (raw || "").split(/[,;/]/)) {
    const c = localizeSingleCountry(part.replace(/\s+/g, " ").trim());
    if (c && !out.some((x) => x.toLowerCase() === c.toLowerCase())) out.push(c);
  }
  return out;
}

export function normalizeCountry(raw?: string | null): string {
  return splitCountries(raw).join(", ");
}

// Different spellings of the same country, so "USA" matches the site's "AQSH".
const COUNTRY_ALIASES: Record<string, string> = {
  "amerika qoshma shtatlari": "aqsh",
  "amerika": "aqsh",
  "janubiy koreya": "koreya",
  "buyuk britaniya": "britaniya",
  "angliya": "britaniya",
  "rossiya federatsiyasi": "rossiya",
  "turkiya": "turkiya",
  "turkiye": "turkiya",
};

export function countryKey(c: string): string {
  const k = c.toLowerCase().replace(/[''`ʻʼ‘’]/g, "").replace(/\s+/g, " ").trim();
  return COUNTRY_ALIASES[k] || k;
}

/** Rewrites each country to the spelling the site already uses, if any. */
export function toSiteCountries(list: string[], site: string[]): string[] {
  const bySite = new Map(site.map((c) => [countryKey(c), c]));
  const out: string[] = [];
  for (const c of list) {
    const v = bySite.get(countryKey(c)) || c;
    if (!out.some((x) => countryKey(x) === countryKey(v))) out.push(v);
  }
  return out;
}
