import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { Suspense } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import SeriesCard from "@/components/SeriesCard";
import GenreFilter from "@/components/GenreFilter";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getSeries } from "@/lib/series-api";
import { localizeSingleGenre } from "@/lib/localization";
import Link from "next/link";
import { ChevronLeft, ChevronRight } from "lucide-react";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export async function generateMetadata({
  searchParams,
}: {
  searchParams?: { genre?: string };
}): Promise<Metadata> {
  const genre = searchParams?.genre?.trim();
  if (genre) {
    const cap = localizeSingleGenre(genre);
    const title = `${cap} seriallar o'zbek tilida | FilmoraUz`;
    const description = `${cap} janridagi seriallarni o'zbek tilida HD sifatda FilmoraUz'da tomosha qiling.`;
    const canonical = `${SITE_URL}/series?genre=${encodeURIComponent(genre)}`;
    return {
      title,
      description,
      openGraph: { title, description, url: canonical, siteName: "FILMORAUZ", type: "website", locale: "uz_UZ" },
      twitter: { card: "summary", title, description },
      alternates: { canonical },
    };
  }
  const title = "Seriallar o'zbek tilida | FilmoraUz";
  const description = "FilmoraUz'da barcha seriallarni o'zbek tilida HD sifatda tomosha qiling.";
  return {
    title,
    description,
    openGraph: { title, description, url: `${SITE_URL}/series`, siteName: "FILMORAUZ", type: "website", locale: "uz_UZ" },
    twitter: { card: "summary", title, description },
    alternates: { canonical: `${SITE_URL}/series` },
  };
}

interface SeriesPageProps {
  searchParams?: { genre?: string; page?: string };
}

export default async function SeriesPage({ searchParams }: SeriesPageProps) {
  const genre = searchParams?.genre || "";
  const page = parseInt(searchParams?.page || "1");
  const limit = 24;

  let seriesData: Awaited<ReturnType<typeof getSeries>>["data"] = [];
  let total = 0;
  try {
    const res = await getSeries(page, limit, genre);
    seriesData = res.data || [];
    total = res.total ?? seriesData.length;
  } catch {
    // Silently handle
  }

  const totalPages = Math.ceil(total / limit);

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4">
          <div className="mb-6 sm:mb-8">
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
              {genre ? `${localizeSingleGenre(genre).toUpperCase()} SERIALLAR` : "SERIALLAR"}
            </h1>
            <p className="text-gray-500 text-sm">
              {total} ta serial topildi
            </p>
          </div>

          <div className="mb-6 sm:mb-8 overflow-x-auto pb-2">
            <Suspense>
              <GenreFilter basePath="/series" />
            </Suspense>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="series_page_banner" variant="banner" />
          </div>

          {seriesData.length > 0 ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-3 sm:gap-4 isolate">
              {seriesData.map((s) => (
                <div key={s.id} className="isolate">
                  <SeriesCard series={s} />
                </div>
              ))}
            </div>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">Hali seriallar yo'q.</p>
            </div>
          )}

          {totalPages > 1 && (
            <div className="flex items-center justify-center gap-4 mt-12 pb-8 flex-wrap">
              {page > 1 && (
                <Link
                  href={`/series?${new URLSearchParams({ ...(genre && { genre }), page: String(page - 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 glass-card border border-white/10 rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
                >
                  <ChevronLeft size={16} />
                  Oldingi
                </Link>
              )}
              <span className="text-sm text-gray-500">Sahifa {page} / {totalPages}</span>
              {page < totalPages && (
                <Link
                  href={`/series?${new URLSearchParams({ ...(genre && { genre }), page: String(page + 1) })}`}
                  className="flex items-center gap-1 px-4 sm:px-5 py-2 glass-card border border-white/10 rounded-lg text-sm text-gray-300 hover:text-white hover:border-gray-500 transition-colors"
                >
                  Keyingi
                  <ChevronRight size={16} />
                </Link>
              )}
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
