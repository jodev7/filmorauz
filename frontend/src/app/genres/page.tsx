import type { Metadata } from "next";
import Link from "next/link";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import { getPublicGenres } from "@/lib/genres";
import { SITE_URL } from "@/lib/content-routes";
import { localizeSingleGenre } from "@/lib/localization";

export const dynamic = "force-dynamic";

export const metadata: Metadata = {
  title: "Janrlar — FilmoraUz",
  description: "FilmoraUz janrlari bo'yicha kinolar va seriallarni ko'ring.",
  alternates: { canonical: `${SITE_URL}/genres` },
  robots: { index: true, follow: true },
};

export default async function GenresPage() {
  const genres = await getPublicGenres();

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-5xl mx-auto px-4 pb-12">
          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-3">
            JANRLAR
          </h1>
          <p className="text-gray-400 mb-8">Filmlar va seriallarni janr bo'yicha ko'ring.</p>
          <div className="flex flex-wrap gap-3">
            {genres.map((genre) => (
              <Link
                key={genre}
                href={`/genres/${genre}`}
                className="rounded-full border border-white/10 glass-card px-4 py-2 text-sm text-gray-200 hover:border-brand-red hover:text-white transition-colors"
              >
                {localizeSingleGenre(genre)}
              </Link>
            ))}
          </div>
        </div>
      </main>
      <Footer />
    </>
  );
}
