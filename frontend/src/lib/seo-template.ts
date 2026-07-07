// SEO title/description templates for movies & series.
//
// Admins only want to type the plain name (and optionally a short base
// description); these helpers wrap that into the site's standard SEO strings.
// All functions are idempotent: re-applying the template to already-generated
// text strips the previous wrapper first, so clicking the button twice — or
// editing an entry that was already generated — never double-wraps.

// Fixed suffix appended to every title. Kept in one place so build + parse stay
// in sync. Matches: "... O'zbek tilida Tarjima Onlayn ko'rish HD — FilmoraUz.net"
const TITLE_SUFFIX = "O'zbek tilida Tarjima Onlayn ko'rish HD — FilmoraUz.net";
const TITLE_SUFFIX_RE = /\s*O'zbek tilida Tarjima Onlayn ko'rish HD\s*[—-]\s*FilmoraUz\.net\s*$/i;
const TITLE_YEAR_RE = /\s*\(\d{4}\)\s*$/;

const DESC_PREFIX_RE = /^.*?uzbek tilida onlayn sifatli ko'rish va tekin yuklab olish \(skachat\)\.\s*/i;
const DESC_SUFFIX_RE = /\s*\d{0,4}\s*yilgi sara kinolar filmorauz\.net saytida\.?\s*$/i;

/** Strip the SEO suffix and trailing "(year)" to recover the plain name. */
export function extractPlainTitle(title: string | undefined): string {
  return (title || "")
    .replace(TITLE_SUFFIX_RE, "")
    .replace(TITLE_YEAR_RE, "")
    .trim();
}

/** Recover the admin-written base description from a generated one. */
export function extractPlainDescription(description: string | undefined): string {
  let d = description || "";
  if (DESC_PREFIX_RE.test(d) && DESC_SUFFIX_RE.test(d)) {
    d = d.replace(DESC_PREFIX_RE, "").replace(DESC_SUFFIX_RE, "");
    d = d.replace(/\.\s*$/, ""); // drop the period we appended after the base
  }
  return d.trim();
}

/** "Avatar (2009) O'zbek tilida Tarjima Onlayn ko'rish HD — FilmoraUz.net" */
export function buildSeoTitle(name: string | undefined, year?: number): string {
  const cleanName = extractPlainTitle(name);
  const yearPart = year && year > 0 ? ` (${year})` : "";
  return `${cleanName}${yearPart} ${TITLE_SUFFIX}`.trim();
}

/**
 * "Avatar uzbek tilida onlayn sifatli ko'rish va tekin yuklab olish (skachat).
 *  [base description]. 2009 yilgi sara kinolar filmorauz.net saytida."
 */
export function buildSeoDescription(
  name: string | undefined,
  year: number | undefined,
  baseDescription: string | undefined
): string {
  const cleanName = extractPlainTitle(name);
  const base = extractPlainDescription(baseDescription);
  const basePart = base ? ` ${base.replace(/\.\s*$/, "")}.` : "";
  const yearPart =
    year && year > 0
      ? `${year} yilgi sara kinolar filmorauz.net saytida.`
      : "Sara kinolar filmorauz.net saytida.";
  return `${cleanName} uzbek tilida onlayn sifatli ko'rish va tekin yuklab olish (skachat).${basePart} ${yearPart}`.trim();
}
