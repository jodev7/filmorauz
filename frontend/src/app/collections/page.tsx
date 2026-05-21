import type { Metadata } from "next";
export const dynamic = "force-dynamic";
import { Suspense } from "react";
import dynamicImport from "next/dynamic";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import CollectionCard from "@/components/CollectionCard";
import GenreFilter from "@/components/GenreFilter";
import { getCollections, Collection } from "@/lib/api";
import { getTranslations } from "@/lib/i18n-server";
import { localizeSingleGenre } from "@/lib/localization";

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

// A collection is considered to match a genre if any of its movies or series
// contains that genre (case-insensitive).
function collectionMatchesGenre(c: Collection, genre: string): boolean {
  const g = genre.toLowerCase();
  const movieMatch = (c.movies || []).some((m) =>
    (m.genres || m.genre || []).some((x) => x.toLowerCase() === g)
  );
  if (movieMatch) return true;
  return (c.series || []).some((s) =>
    (s.genres || s.genre || []).some((x) => x.toLowerCase() === g)
  );
}

interface Props {
  searchParams?: { genre?: string };
}

export default async function CollectionsPage({ searchParams }: Props) {
  const { t } = getTranslations("uz");
  const genre = (searchParams?.genre || "").trim();

  let collections: Collection[] = [];
  try {
    collections = await getCollections();
  } catch {
    // silent
  }

  const filtered = genre
    ? collections.filter((c) => collectionMatchesGenre(c, genre))
    : collections;

  const pageTitle = genre
    ? `${localizeSingleGenre(genre).toUpperCase()} KOLLEKTSIYALAR`
    : "KOLLEKTSIYALAR";

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4">
          <div className="mb-6 sm:mb-8">
            <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
              {pageTitle}
            </h1>
            <p className="text-gray-500 text-sm">
              {filtered.length} ta kolleksiya topildi
            </p>
          </div>

          <div className="mb-6 sm:mb-8 overflow-x-auto pb-2">
            <Suspense>
              <GenreFilter basePath="/collections" />
            </Suspense>
          </div>

          <div className="mb-6">
            <WebsiteAdSlot placement="collections_page_banner" variant="banner" />
          </div>

          {filtered.length > 0 ? (
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4 pb-12">
              {filtered.map((collection) => (
                <CollectionCard key={collection.id} collection={collection} />
              ))}
            </div>
          ) : (
            <div className="py-24 text-center text-gray-500">
              <p className="text-lg">
                {genre
                  ? "Bu janrda kolleksiyalar topilmadi."
                  : t("collections.noCollections")}
              </p>
            </div>
          )}
        </div>
      </main>
      <Footer />
    </>
  );
}
