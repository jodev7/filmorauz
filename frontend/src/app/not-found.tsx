import Link from "next/link";
import { Clapperboard, Film, Tv } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import SearchLauncher from "@/components/SearchLauncher";
import RandomMovieButton from "@/components/RandomMovie";
import { getTrendingMovies, Movie } from "@/lib/api";

export default async function NotFound() {
  let popular: Movie[] = [];
  try {
    popular = (await getTrendingMovies("7d", 6)).slice(0, 6);
  } catch {
    // still render the page
  }

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-28">
        <div className="mx-auto max-w-5xl px-4 pb-16">
          <section className="relative overflow-hidden rounded-3xl border border-white/10 bg-gradient-to-br from-orange-600/15 via-[#101018] to-violet-700/15 px-6 py-12 text-center sm:py-16">
            <div className="pointer-events-none absolute inset-x-0 top-6 flex justify-center gap-3 opacity-[0.07]" aria-hidden="true">
              {Array.from({ length: 12 }, (_, i) => (
                <div key={i} className="h-10 w-7 rounded-sm border-2 border-white" />
              ))}
            </div>
            <Clapperboard className="mx-auto mb-4 text-orange-400" size={44} />
            <p className="font-display text-7xl leading-none tracking-wide text-white sm:text-9xl">404</p>
            <h1 className="mt-3 text-xl font-semibold text-white sm:text-2xl">Bu sahna kesib tashlangan</h1>
            <p className="mx-auto mt-2 max-w-md text-sm text-gray-400 sm:text-base">
              Sahifa mavjud emas, o&apos;chirilgan yoki manzil noto&apos;g&apos;ri yozilgan. Qidirib ko&apos;ring yoki quyidagilardan birini tanlang.
            </p>
            <div className="mx-auto mt-6 max-w-lg">
              <SearchLauncher />
            </div>
            <div className="mt-5 flex flex-wrap items-center justify-center gap-2">
              <Link href="/" className="rounded-xl bg-orange-500 px-5 py-2.5 text-sm font-semibold text-white hover:bg-orange-600">
                Bosh sahifa
              </Link>
              <Link href="/movies" className="inline-flex items-center gap-1.5 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-white hover:bg-white/5">
                <Film size={15} /> Kinolar
              </Link>
              <Link href="/series" className="inline-flex items-center gap-1.5 rounded-xl border border-white/10 px-4 py-2.5 text-sm text-white hover:bg-white/5">
                <Tv size={15} /> Seriallar
              </Link>
              <RandomMovieButton />
            </div>
          </section>

          {popular.length > 0 && (
            <section className="mt-12">
              <h2 className="mb-5 font-display text-2xl tracking-wide text-white">BALKIM SHULARNI QIDIRGANDIRSIZ</h2>
              <div className="grid grid-cols-2 gap-3 sm:grid-cols-3 sm:gap-4 md:grid-cols-6">
                {popular.map((m) => (
                  <MovieCard key={m.id} movie={m} />
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
