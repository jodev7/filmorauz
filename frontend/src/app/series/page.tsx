import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import SeriesCarousel from "@/components/SeriesCarousel";
import { getSeries } from "@/lib/series-api";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

export async function generateMetadata(): Promise<Metadata> {
  return {
    title: "Seriallar — FILMORAUZ",
    description: "FILMORAUZ'da barcha seriallarni ko'ring.",
    alternates: {
      canonical: `${SITE_URL}/series`,
    },
  };
}

export default async function SeriesPage() {
  let seriesData: any[] = [];
  
  try {
    const res = await getSeries(1, 50);
    seriesData = res.data || [];
  } catch {
    // Silently handle
  }

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4">
          <div className="mb-6 sm:mb-8">
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
              SERIALLAR
            </h1>
            <p className="text-gray-500 text-sm">
              {seriesData.length} ta serial topildi
            </p>
          </div>

          {seriesData.length > 0 ? (
            <section className="pb-12">
              <SeriesCarousel series={seriesData} />
            </section>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">Hali seriallar yo'q.</p>
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
