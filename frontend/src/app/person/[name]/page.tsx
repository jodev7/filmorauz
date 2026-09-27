import type { Metadata } from "next";
import Link from "next/link";
import { notFound } from "next/navigation";
import { Clapperboard, Film, CalendarRange, ChevronLeft, Tv } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import SeriesCard from "@/components/SeriesCard";
import JsonLd from "@/components/JsonLd";
import { getPersonCredits, personPath, Movie } from "@/lib/api";
import { SITE_URL } from "@/lib/content-routes";
import { localizeSingleGenre } from "@/lib/localization";

export const revalidate = 300;

interface Props {
  params: { name: string };
}

function decodeName(raw: string): string {
  try {
    return decodeURIComponent(raw).trim();
  } catch {
    return raw.trim();
  }
}

function initials(name: string): string {
  return name
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
    .map((p) => p[0]?.toUpperCase() ?? "")
    .join("");
}

function topGenres(movies: Movie[], n = 4): string[] {
  const counts = new Map<string, number>();
  movies.forEach((m) => (m.genre || []).forEach((g) => counts.set(g, (counts.get(g) || 0) + 1)));
  return Array.from(counts.entries())
    .sort((a, b) => b[1] - a[1])
    .slice(0, n)
    .map(([g]) => g);
}

async function load(raw: string) {
  const name = decodeName(raw);
  if (!name || name.length > 80) return null;
  try {
    return await getPersonCredits(name);
  } catch {
    return null;
  }
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const credits = await load(params.name);
  if (!credits) return { title: "Topilmadi", robots: { index: false } };
  const total = credits.acted.length + credits.directed.length + credits.series.length;
  const title = `${credits.name} — filmlari`;
  const description = `${credits.name} ishtirok etgan ${total} ta ${credits.series.length ? "kino va serialni" : "kinoni"} FilmoraUz'da o'zbek tilida onlayn tomosha qiling.`;
  const canonical = `${SITE_URL}${personPath(credits.name)}`;
  return {
    title,
    description,
    alternates: { canonical },
    openGraph: {
      title,
      description,
      url: canonical,
      type: "profile",
      siteName: "FILMORAUZ",
      locale: "uz_UZ",
      ...(credits.photo_url ? { images: [credits.photo_url.replace("/w185/", "/h632/")] } : {}),
    },
  };
}

function Section({ title, icon, movies }: { title: string; icon: React.ReactNode; movies: Movie[] }) {
  if (movies.length === 0) return null;
  return (
    <section className="mb-12">
      <h2 className="mb-5 flex items-center gap-2 font-display text-2xl tracking-wide text-white sm:text-3xl">
        {icon}
        {title}
        <span className="ml-1 rounded-full bg-white/5 px-2 py-0.5 font-body text-sm text-gray-400">{movies.length}</span>
      </h2>
      <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:grid-cols-4 lg:grid-cols-6">
        {movies.map((m, i) => (
          <MovieCard key={m.id} movie={m} priority={i < 6} />
        ))}
      </div>
    </section>
  );
}

export default async function PersonPage({ params }: Props) {
  const credits = await load(params.name);
  if (!credits) notFound();

  const all = [...credits.acted, ...credits.directed.filter((d) => !credits.acted.some((a) => a.id === d.id))];
  const years = all.map((m) => m.year).filter((y): y is number => !!y && y > 1900);
  const minYear = years.length ? Math.min(...years) : null;
  const maxYear = years.length ? Math.max(...years) : null;
  const genres = topGenres(all);
  const role =
    credits.directed.length > 0 && credits.acted.length === 0
      ? "Rejissyor"
      : credits.directed.length > 0
      ? "Aktyor, rejissyor"
      : "Aktyor";
  const url = `${SITE_URL}${personPath(credits.name)}`;

  return (
    <>
      <Navbar />
      <JsonLd
        data={{
          "@context": "https://schema.org",
          "@type": "Person",
          name: credits.name,
          url,
          ...(credits.photo_url ? { image: credits.photo_url } : {}),
          jobTitle: role,
          performerIn: credits.acted.slice(0, 20).map((m) => ({ "@type": "Movie", name: m.title, url: `${SITE_URL}/movies/${m.slug}` })),
        }}
      />
      <main className="min-h-screen pt-24">
        <div className="mx-auto max-w-7xl px-4">
          <Link href="/movies" className="mb-6 inline-flex items-center gap-1 text-sm text-gray-400 hover:text-white">
            <ChevronLeft size={16} /> Kinolar
          </Link>

          <header className="glass-card mb-10 flex flex-col gap-6 rounded-2xl border border-white/10 p-6 sm:flex-row sm:items-center sm:p-8">
            <div
              className="relative flex h-24 w-24 shrink-0 items-center justify-center overflow-hidden rounded-full bg-gradient-to-br from-orange-500 to-rose-600 font-display text-4xl text-white shadow-lg sm:h-28 sm:w-28"
              aria-hidden="true"
            >
              {initials(credits.name)}
              {credits.photo_url && (
                // eslint-disable-next-line @next/next/no-img-element
                <img src={credits.photo_url.replace("/w185/", "/h632/")} alt="" className="absolute inset-0 h-full w-full object-cover" />
              )}
            </div>
            <div className="min-w-0">
              <p className="mb-1 text-sm text-orange-400">{role}</p>
              <h1 className="font-display text-4xl leading-none tracking-wide text-white sm:text-5xl">{credits.name}</h1>
              <div className="mt-4 flex flex-wrap gap-2 text-sm text-gray-300">
                <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 px-3 py-1">
                  <Film size={14} className="text-orange-400" /> {all.length} ta kino
                </span>
                {credits.series.length > 0 && (
                  <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 px-3 py-1">
                    <Tv size={14} className="text-orange-400" /> {credits.series.length} ta serial
                  </span>
                )}
                {minYear && maxYear && (
                  <span className="inline-flex items-center gap-1.5 rounded-full border border-white/10 px-3 py-1">
                    <CalendarRange size={14} className="text-orange-400" /> {minYear === maxYear ? minYear : `${minYear}–${maxYear}`}
                  </span>
                )}
                {genres.map((g) => (
                  <Link
                    key={g}
                    href={`/genres/${encodeURIComponent(g.toLowerCase())}`}
                    className="rounded-full border border-white/10 px-3 py-1 hover:border-orange-500/50 hover:text-white"
                  >
                    {localizeSingleGenre(g)}
                  </Link>
                ))}
              </div>
            </div>
          </header>

          <Section title="ROLLARDA" icon={<Film className="text-orange-500" size={24} />} movies={credits.acted} />
          <Section title="REJISSYORLIK" icon={<Clapperboard className="text-orange-500" size={24} />} movies={credits.directed} />
          {credits.series.length > 0 && (
            <section className="mb-12">
              <h2 className="mb-5 flex items-center gap-2 font-display text-2xl tracking-wide text-white sm:text-3xl">
                <Tv className="text-orange-500" size={24} />
                SERIALLAR
                <span className="ml-1 rounded-full bg-white/5 px-2 py-0.5 font-body text-sm text-gray-400">{credits.series.length}</span>
              </h2>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:grid-cols-4 lg:grid-cols-6">
                {credits.series.map((s) => (
                  <SeriesCard key={s.id} series={s} />
                ))}
              </div>
            </section>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
