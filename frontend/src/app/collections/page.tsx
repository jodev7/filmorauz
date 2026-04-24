import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import dynamicImport from "next/dynamic";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import MediaImage from "@/components/MediaImage";
import MovieCard from "@/components/MovieCard";
import { getCollections, Collection } from "@/lib/api";
import { getTranslations } from "@/lib/i18n-server";
import { DEFAULT_POSTER_PLACEHOLDER, normalizeMediaUrl } from "@/lib/image-utils";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";
const WebsiteAdSlot = dynamicImport(() => import("@/components/ads/WebsiteAdSlot"));

export async function generateMetadata(): Promise<Metadata> {
  return {
    title: "Kolleksiyalar — FILMORAUZ",
    description: "FILMORAUZ'da filmlar kolleksiyalarini ko'ring.",
    alternates: {
      canonical: `${SITE_URL}/collections`,
    },
  };
}

export default async function CollectionsPage() {
  const { t } = getTranslations("uz");
  let collections: Collection[] = [];
  
  try {
    collections = await getCollections();
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
              KOLLEKTSIYALAR
            </h1>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="collections_page_banner" variant="banner" />
          </div>

          {collections.length > 0 ? (
            <div className="space-y-12">
              {collections.map((collection) => (
                <section key={collection.id} className="pb-8">
                  <div className="flex items-center gap-4 mb-6">
                    {collection.poster_url && (
                      <div className="relative w-16 h-24 rounded-lg overflow-hidden">
                        <MediaImage
                          src={collection.poster_url}
                          alt={collection.title}
                          className="absolute inset-0 h-full w-full object-cover"
                        />
                      </div>
                    )}
                    <div>
                      <h2 className="font-display text-2xl text-white">{collection.title}</h2>
                      {collection.description && (
                        <p className="text-gray-500 text-sm mt-1">{collection.description}</p>
                      )}
                    </div>
                  </div>
                  
                  {collection.movies && collection.movies.length > 0 && (
                    <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
                      {collection.movies.map((movie) => (
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
                  )}
                </section>
              ))}
            </div>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">{t("collections.noCollections")}</p>
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
