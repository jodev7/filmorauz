"use client";

import { useState } from "react";
import { Loader2, RefreshCw, Users } from "lucide-react";
import { adminFetchSeriesCredits, CastMember } from "@/lib/api";

/** Series cast from TMDB: shows what is stored and refreshes it on demand. */
export default function SeriesCastPanel({
  seriesId,
  token,
  initialCast = [],
  initialDirector = "",
  initialDetails = [],
  initialDirectorPhoto = "",
  onMessage,
}: {
  seriesId: string;
  token: string | null;
  initialCast?: string[];
  initialDirector?: string;
  initialDetails?: CastMember[];
  initialDirectorPhoto?: string;
  onMessage: (kind: "success" | "error", text: string) => void;
}) {
  const [cast, setCast] = useState(initialCast);
  const [director, setDirector] = useState(initialDirector);
  const [details, setDetails] = useState(initialDetails);
  const [directorPhoto, setDirectorPhoto] = useState(initialDirectorPhoto);
  const [busy, setBusy] = useState(false);
  const photo = new Map(details.map((d) => [d.name.toLowerCase(), d]));
  if (director && directorPhoto) photo.set(director.toLowerCase(), { name: director, profile_url: directorPhoto });

  const refresh = async () => {
    if (!token) return;
    setBusy(true);
    try {
      const res = await adminFetchSeriesCredits(token, seriesId);
      if (res.status !== "ok") {
        onMessage("error", "TMDB'da bu serial topilmadi — nomi yoki yilini tekshiring");
        return;
      }
      setCast(res.cast);
      setDirector(res.director);
      setDetails(res.cast_details);
      setDirectorPhoto(res.director_profile_url || "");
      onMessage("success", `${res.cast.length} ta aktyor TMDB'dan olindi`);
    } catch (err) {
      onMessage("error", err instanceof Error ? err.message : "TMDB'dan olib bo'lmadi");
    } finally {
      setBusy(false);
    }
  };

  return (
    <section className="mt-6 rounded-2xl border border-white/10 bg-[#12121a] p-5 sm:p-6">
      <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <span className="flex h-8 w-8 items-center justify-center rounded-lg bg-orange-500/10 text-orange-400">
            <Users size={16} />
          </span>
          <div>
            <h2 className="font-semibold text-white">Rollarda</h2>
            <p className="text-xs text-gray-500">TMDB&apos;dan avtomatik olinadi (rasmlari bilan)</p>
          </div>
        </div>
        <button
          type="button"
          onClick={refresh}
          disabled={busy || !token}
          className="inline-flex items-center gap-1.5 rounded-xl border border-sky-500/30 bg-sky-500/10 px-3 py-2 text-xs font-medium text-sky-300 hover:bg-sky-500/20 disabled:opacity-60"
        >
          {busy ? <Loader2 size={13} className="animate-spin" /> : <RefreshCw size={13} />}
          TMDB&apos;dan yangilash
        </button>
      </div>
      {cast.length === 0 && !director ? (
        <p className="text-sm text-gray-500">Hali olinmagan. Fon jarayoni bir necha daqiqada o&apos;zi oladi yoki tugmani bosing.</p>
      ) : (
        <ul className="scrollbar-hide flex gap-3 overflow-x-auto pb-1">
          {[...(director ? [{ name: director, role: "Yaratuvchi" }] : []), ...cast.map((n) => ({ name: n, role: "" }))].map((p) => {
            const d = photo.get(p.name.toLowerCase());
            return (
              <li key={p.role + p.name} className="flex w-20 shrink-0 flex-col items-center text-center">
                <span className="relative flex h-14 w-14 items-center justify-center overflow-hidden rounded-full bg-white/10 text-sm font-semibold text-gray-300">
                  {p.name.split(/\s+/).slice(0, 2).map((w) => w[0]?.toUpperCase()).join("")}
                  {d?.profile_url && (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img src={d.profile_url} alt="" loading="lazy" className="absolute inset-0 h-full w-full object-cover" />
                  )}
                </span>
                <span className="mt-1.5 line-clamp-2 text-[11px] leading-tight text-gray-200">{p.name}</span>
                <span className="line-clamp-1 text-[10px] text-gray-500">{p.role || d?.character || "Aktyor"}</span>
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
