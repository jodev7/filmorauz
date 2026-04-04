import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { notFound } from "next/navigation";
import Link from "next/link";
import { ChevronLeft, Calendar, Globe, Play } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import SeasonList from "@/components/SeasonList";
import SeriesCarousel from "@/components/SeriesCarousel";
import { getSeriesBySlug, getSeries } from "@/lib/series-api";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

interface Props {
  params: { slug: string };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = params;
  
  try {
    const series = await getSeriesBySlug(slug);
    return {
      title: `${series.series.title} — FILMORAUZ`,
      description: series.series.description,
      alternates: {
        canonical: `${SITE_URL}/series/${slug}`,
      },
    };
  } catch {
    return {
      title: "Serial Not Found — FILMORAUZ",
    };
  }
}

export default async function SeriesDetailPage({ params }: Props) {
  const { slug } = params;
  let seriesData;
  let relatedSeries: any[] = [];
  
  try {
    seriesData = await getSeriesBySlug(slug);
  } catch {
    notFound();
  }

  // Fetch related series
  try {
    const res = await getSeries(1, 20);
    relatedSeries = (res.data || []).filter(s => s.slug !== slug).slice(0, 10);
  } catch {
    // Silently handle
  }

  const { series, seasons } = seriesData;

  return (
    <>
      <Navbar />
      <main className="min-h-screen">
        {/* Backdrop hero */}
        <div className="relative h-[40vh] sm:h-[45vh] min-h-[250px]">
          {series.backdrop_url && (
            <>
              {/* eslint-disable-next-line @next/next/no-img-element */}
              <img
                src={series.backdrop_url}
                alt={series.title}
                className="w-full h-full object-cover"
              />
              <div className="absolute inset-0 bg-gradient-to-t from-brand-dark via-brand-dark/50 to-black/30" />
            </>
          )}
        </div>

        {/* Main content */}
        <div className="max-w-7xl mx-auto px-4 -mt-20 relative">
          <div className="flex flex-col md:flex-row gap-6 sm:gap-8">
            {/* Poster */}
            <div className="shrink-0">
              {series.poster_url && (
                <img
                  src={series.poster_url}
                  alt={series.title}
                  className="w-36 sm:w-44 md:w-48 lg:w-56 rounded-xl shadow-2xl border border-brand-border"
                />
              )}
            </div>

            {/* Details */}
            <div className="flex-1 pt-2 md:pt-4">
              <Link
                href="/series"
                className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-3 sm:mb-4 transition-colors"
              >
                <ChevronLeft size={16} />
                Seriallarga qaytish
              </Link>

              <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide leading-none mb-3">
                {series.title}
              </h1>

              <div className="flex flex-wrap items-center gap-3 sm:gap-4 text-sm text-gray-400 mb-4">
                <span className="flex items-center gap-1">
                  <Calendar size={14} />
                  {series.year}
                </span>
                {series.country && (
                  <span className="flex items-center gap-1">
                    <Globe size={14} />
                    {series.country}
                  </span>
                )}
              </div>

              {series.genre && series.genre.length > 0 && (
                <div className="flex flex-wrap gap-2 mb-5">
                  {series.genre.map((g: string) => (
                    <span
                      key={g}
                      className="text-xs sm:text-sm bg-brand-card border border-brand-border text-gray-300 px-3 py-1 rounded-full"
                    >
                      {g}
                    </span>
                  ))}
                </div>
              )}

              <p className="text-gray-300 leading-relaxed max-w-2xl mb-6 text-sm sm:text-base">
                {series.description}
              </p>
            </div>
          </div>

          {/* Seasons */}
          {seasons && seasons.length > 0 && (
            <section className="mt-8 pb-12">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
                FASLLAR
              </h2>
              <SeasonList seasons={seasons} />
            </section>
          )}

          {/* Related Series */}
          {relatedSeries.length > 0 && (
            <section className="pb-12">
              <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white mb-6">
                BOSHQA SERIALLAR
              </h2>
              <SeriesCarousel series={relatedSeries} />
            </section>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
