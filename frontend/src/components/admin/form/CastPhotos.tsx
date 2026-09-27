import type { CastMember } from "@/lib/api";

const key = (s: string) => s.trim().replace(/\s+/g, " ").toLowerCase();

/** Small avatar strip showing which of the entered actors have a TMDB photo. */
export default function CastPhotos({ names, details }: { names: string[]; details: CastMember[] }) {
  if (!details.length || !names.length) return null;
  const byName = new Map(details.map((d) => [key(d.name), d]));
  const shown = names.map((n) => byName.get(key(n))).filter((d): d is CastMember => !!d && !!d.profile_url);
  if (!shown.length) return null;
  return (
    <div className="mt-2 flex items-center gap-2">
      <div className="flex -space-x-2">
        {shown.slice(0, 10).map((d) => (
          // eslint-disable-next-line @next/next/no-img-element
          <img
            key={d.name}
            src={d.profile_url}
            alt={d.name}
            title={d.character ? `${d.name} — ${d.character}` : d.name}
            loading="lazy"
            className="h-7 w-7 rounded-full border-2 border-[#12121a] object-cover"
          />
        ))}
      </div>
      <span className="text-[11px] text-gray-500">
        {shown.length}/{names.length} rasm bilan
      </span>
    </div>
  );
}
