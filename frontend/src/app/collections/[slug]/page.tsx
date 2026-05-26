import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { notFound } from "next/navigation";
import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import SeriesCard from "@/components/SeriesCard";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import { getCollectionBySlug } from "@/lib/api";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

interface Props {
  params: { slug: string };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  const { slug } = params;

  try {
    const collection: any = await getCollectionBySlug(slug);
    return {
      title: `${collection?.title || slug} — FILMORAUZ`,
      description: collection?.description || "",
      alternates: {
        canonical: `${SITE_URL}/collections/${slug}`,
      },
    };
  } catch {
    return {
      title: "Kolleksiya topilmadi — FILMORAUZ",
    };
  }
}

export default async function CollectionDetailPage({ params }: Props) {
  const { slug } = params;
  let collection: any;

  try {
    collection = await getCollectionBySlug(slug);
  } catch {
    notFound();
    return null;
  }

  if (!collection) {
    notFound();
    return null;
  }

  const movies: any[] = collection.movies || [];
  const series: any[] = collection.series || [];
  const hasContent = movies.length > 0 || series.length > 0;

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4">
          <Link
            href="/collections"
            className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-6 transition-colors"
          >
            <ChevronLeft size={16} />
            Kolleksiyalarga qaytish
          </Link>

          <div className="mb-6 sm:mb-8">
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
              {collection.title}
            </h1>
            {collection.description && (
              <p className="text-gray-500 text-base">{collection.description}</p>
            )}
            <p className="mt-2 text-gray-500 text-sm">
              {movies.length} kino · {series.length} serial
            </p>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="collection_detail_page_banner" variant="banner" />
          </div>

          {!hasContent && (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">Bu kolleksiyada hali kontent yo'q.</p>
            </div>
          )}

          {movies.length > 0 && (
            <section className="mb-12">
              <h2 className="font-display text-2xl text-white tracking-wide mb-4">
                KINOLAR
              </h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
                {movies.map((movie: any) => (
                  <MovieCard
                    key={movie.id}
                    movie={{
                      id: movie.id,
                      code: movie.code,
                      title: movie.title,
                      poster_url: movie.poster_url,
                      slug: movie.slug,
                      year: movie.year,
                      genre: movie.genre || movie.genres,
                      duration: movie.duration,
                      quality: movie.quality,
                      rating_avg: movie.rating_avg,
                      is_premium: movie.is_premium,
                      created_at: movie.created_at,
                    } as any}
                  />
                ))}
              </div>
            </section>
          )}

          {series.length > 0 && (
            <section className="mb-12">
              <h2 className="font-display text-2xl text-white tracking-wide mb-4">
                SERIALLAR
              </h2>
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
                {series.map((s: any) => (
                  <SeriesCard
                    key={s.id}
                    series={{
                      id: s.id,
                      title: s.title,
                      poster_url: s.poster_url,
                      slug: s.slug,
                      year: s.year,
                      genre: s.genre || s.genres,
                      rating_avg: s.rating_avg,
                      quality: s.quality,
                      is_premium: s.is_premium,
                    } as any}
                  />
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
