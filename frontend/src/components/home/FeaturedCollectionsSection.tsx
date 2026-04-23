"use client";

import Link from "next/link";
import { ChevronRight } from "lucide-react";
import Image from "next/image";
import { Collection, Movie } from "@/lib/api";
import MovieCard from "@/components/MovieCard";
import { useI18n } from "@/lib/i18n";

interface FeaturedCollectionsSectionProps {
  collections: Collection[];
}

export default function FeaturedCollectionsSection({
  collections,
}: FeaturedCollectionsSectionProps) {
  const { t } = useI18n();
  
  // Show empty state only if we have data and it's truly empty
  // Don't hide if collections is undefined or null (could be loading/error)
  if (collections === undefined || collections === null) {
    return null;
  }
  
  if (collections.length === 0) {
    return null;
  }

  return (
    <section className="max-w-7xl mx-auto px-4 py-12">
      <div className="flex items-center justify-between mb-6">
        <h2 className="font-display text-2xl sm:text-3xl tracking-wide text-white">
          {t("home.collections")}
        </h2>
        <Link
          href="/collections"
          className="flex items-center gap-1 text-sm text-brand-red hover:text-orange-400 transition-colors"
        >
          {t("common.seeAll")} <ChevronRight size={16} />
        </Link>
      </div>

      <div className="space-y-12">
        {collections.map((collection) => (
          <div key={collection.id} className="space-y-4">
            {/* Collection Header */}
            <div className="flex items-center gap-4">
              {collection.poster_url && (
                <div className="w-16 h-24 relative rounded-lg overflow-hidden flex-shrink-0">
                  <Image
                    src={collection.poster_url}
                    alt={collection.title}
                    fill
                    sizes="64px"
                    loading="lazy"
                    className="object-cover"
                  />
                </div>
              )}
              <div>
                <h3 className="text-xl font-bold text-white">{collection.title}</h3>
                {collection.description && (
                  <p className="text-gray-400 text-sm mt-1 line-clamp-2">
                    {collection.description}
                  </p>
                )}
              </div>
            </div>

            {/* Collection Movies */}
            {collection.movies && collection.movies.length > 0 ? (
              <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
                {collection.movies.slice(0, 6).map((movie) => (
                  <MovieCard
                    key={movie.id}
                    movie={{
                      id: movie.id,
                      title: movie.title,
                      poster_url: movie.poster_url,
                      slug: movie.slug,
                      year: movie.year,
                      genre: movie.genres,
                    } as Partial<Movie> & { id: string; title: string; poster_url: string; slug: string }}
                  />
                ))}
              </div>
            ) : (
              <p className="text-gray-500 text-sm">{t("common.noMovies")}</p>
            )}
          </div>
        ))}
      </div>
    </section>
  );
}
