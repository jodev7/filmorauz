"use client";

import { memo } from "react";
import Link from "next/link";
import { ChevronRight } from "lucide-react";
import { Collection } from "@/lib/api";
import CollectionCard from "@/components/CollectionCard";
import { useI18n } from "@/lib/i18n";

interface FeaturedCollectionsSectionProps {
  collections: Collection[];
}

function FeaturedCollectionsSectionImpl({
  collections,
}: FeaturedCollectionsSectionProps) {
  const { t } = useI18n();

  if (!collections || collections.length === 0) return null;

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

      <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-6 gap-4">
        {collections.map((collection) => (
          <CollectionCard key={collection.id} collection={collection} />
        ))}
      </div>
    </section>
  );
}

const FeaturedCollectionsSection = memo(FeaturedCollectionsSectionImpl);
FeaturedCollectionsSection.displayName = "FeaturedCollectionsSection";

export default FeaturedCollectionsSection;
