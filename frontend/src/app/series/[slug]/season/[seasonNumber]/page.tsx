import type { Metadata } from "next";
import { notFound } from "next/navigation";
import Link from "next/link";
import Script from "next/script";
import { ChevronLeft } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import SeasonList from "@/components/SeasonList";
import { getSeriesBySlug } from "@/lib/series-api";
import { buildSeasonUrl, buildSeriesPath, buildSeriesUrl, SITE_URL } from "@/lib/content-routes";
import { buildContentDescription, buildContentKeywords, buildContentTitle, pickSeoImage } from "@/lib/seo";

interface Props {
  params: { slug: string; seasonNumber: string };
}

async function getSeasonPageData(slug: string, seasonNumber: number) {
  const data = await getSeriesBySlug(slug);
  const season = data.seasons.find((item) => item.season.season_number === seasonNumber);
  if (!season) {
    notFound();
  }
  return { data, season };
}

export async function generateMetadata({ params }: Props): Promise<Metadata> {
  try {
    const seasonNumber = Number(params.seasonNumber);
    const { data, season } = await getSeasonPageData(params.slug, seasonNumber);
    const titleText = `${data.series.title} ${seasonNumber}-mavsum`;
    const title = buildContentTitle(titleText);
    const description = buildContentDescription(titleText, data.series.year);
    const canonicalUrl = buildSeasonUrl(params.slug, seasonNumber);
    const imageUrl = pickSeoImage(
      season.season.poster_url,
      data.series.backdrop_url,
      data.series.poster_url
    );

    return {
      title,
      description,
      keywords: buildContentKeywords({
        title: titleText,
        uzbekTitle: titleText,
        genres: data.series.genre,
        extra: [data.series.title, "mavsum", seasonNumber.toString()],
      }),
      openGraph: {
        title,
        description,
        url: canonicalUrl,
        siteName: "FILMORAUZ",
        type: "website",
        locale: "uz_UZ",
        images: [{ url: imageUrl, width: 1200, height: 630, alt: titleText }],
      },
      twitter: {
        card: "summary_large_image",
        title,
        description,
        images: [imageUrl],
      },
      alternates: { canonical: canonicalUrl },
      robots: { index: true, follow: true },
    };
  } catch {
    return {
      title: "Mavsum topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function SeasonPage({ params }: Props) {
  const seasonNumber = Number(params.seasonNumber);
  const { data, season } = await getSeasonPageData(params.slug, seasonNumber);
  const canonicalUrl = buildSeasonUrl(params.slug, seasonNumber);
  const seasonTitle = `${data.series.title} ${seasonNumber}-mavsum`;

  const seasonJsonLd = {
    "@context": "https://schema.org",
    "@type": "TVSeason",
    name: seasonTitle,
    url: canonicalUrl,
    seasonNumber,
    numberOfEpisodes: season.episodes.length,
    image: [pickSeoImage(season.season.poster_url, data.series.backdrop_url, data.series.poster_url)],
    partOfSeries: {
      "@type": "TVSeries",
      name: data.series.title,
      url: buildSeriesUrl(data.series.slug),
    },
  };

  return (
    <>
      <Script
        id="season-schema"
        type="application/ld+json"
        dangerouslySetInnerHTML={{ __html: JSON.stringify(seasonJsonLd) }}
      />
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        <div className="max-w-7xl mx-auto px-4 pb-12">
          <Link
            href={buildSeriesPath(data.series.slug)}
            className="inline-flex items-center gap-1 text-sm text-gray-500 hover:text-white mb-4 transition-colors"
          >
            <ChevronLeft size={16} />
            {data.series.title}
          </Link>

          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wide mb-2">
            {seasonTitle}
          </h1>
          <p className="text-gray-400 mb-8">
            {season.episodes.length} ta epizod
          </p>

          <SeasonList
            seasons={[season]}
            seriesBackdropUrl={data.series.backdrop_url}
            seriesPosterUrl={data.series.poster_url}
            seriesSlug={data.series.slug}
          />
        </div>
      </main>
      <Footer />
    </>
  );
}
