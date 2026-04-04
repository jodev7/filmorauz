import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { notFound } from "next/navigation";
import Link from "next/link";
import { ChevronLeft } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MovieCard from "@/components/MovieCard";
import { getCollectionBySlug, getCollections } from "@/lib/api";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

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
          </div>

          {collection.movies && collection.movies.length > 0 ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
              {collection.movies.map((movie: any) => (
                <MovieCard
                  key={movie.id}
                  movie={{
                    id: movie.id,
                    title: movie.title,
                    poster_url: movie.poster_url,
                    slug: movie.slug,
                    year: movie.year,
                    genre: movie.genres,
                  } as any}
                />
              ))}
            </div>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">Bu kolleksiyada hali kinolar yo'q.</p>
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
