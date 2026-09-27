import PersonChip from "@/components/PersonChip";
import type { CastMember } from "@/lib/api";

const key = (s: string) => s.trim().replace(/\s+/g, " ").toLowerCase();

/**
 * "Rollarda va ijodkorlar" row. Names come from `cast` (admin-editable);
 * photos and character names are matched from TMDB `castDetails`.
 */
export default function CastRow({
  director,
  directorPhoto,
  directorRole = "Rejissyor",
  cast,
  castDetails,
}: {
  director?: string;
  directorPhoto?: string;
  directorRole?: string;
  cast?: string[];
  castDetails?: CastMember[];
}) {
  const details = new Map((castDetails || []).map((c) => [key(c.name), c]));
  const names = cast && cast.length ? cast : (castDetails || []).map((c) => c.name);
  const people = names.filter((n) => n.trim()).slice(0, 15);
  if (!director && people.length === 0) return null;
  return (
    <section className="mt-8" aria-labelledby="cast-title">
      <h2 id="cast-title" className="mb-4 font-display text-xl tracking-wide text-white sm:text-2xl">
        ROLLARDA VA IJODKORLAR
      </h2>
      <ul className="scrollbar-hide -mx-4 flex gap-3 overflow-x-auto px-4 pb-2">
        {director && (
          <li className="shrink-0">
            <PersonChip name={director} role={directorRole} photo={directorPhoto} />
          </li>
        )}
        {people.map((name) => {
          const d = details.get(key(name));
          return (
            <li key={name} className="shrink-0">
              <PersonChip name={name} role={d?.character || "Aktyor"} photo={d?.profile_url} />
            </li>
          );
        })}
      </ul>
    </section>
  );
}
