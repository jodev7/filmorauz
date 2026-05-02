import { Metadata } from "next";
import EpisodePageView from "@/components/watch/EpisodePageView";
import { getEpisodePageDataBySeoPath } from "@/lib/episode-page-data";

interface PageProps {
  params: { slug: string; seasonNumber: string; episodeNumber: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  try {
    const data = await getEpisodePageDataBySeoPath(
      params.slug,
      Number(params.seasonNumber),
      Number(params.episodeNumber)
    );
    return {
      title: data.title,
      description: data.description,
      keywords: data.keywords,
      openGraph: {
        title: data.title,
        description: data.description,
        url: data.canonicalUrl,
        siteName: "FILMORAUZ",
        type: "video.episode",
        locale: "uz_UZ",
        images: [{ url: data.imageUrl, width: 1200, height: 630, alt: data.episode.title }],
      },
      twitter: {
        card: "summary_large_image",
        title: data.title,
        description: data.description,
        images: [data.imageUrl],
      },
      alternates: { canonical: data.canonicalUrl },
      robots: { index: true, follow: true },
    };
  } catch {
    return {
      title: "Epizod topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function SeoEpisodePage({ params }: PageProps) {
  const data = await getEpisodePageDataBySeoPath(
    params.slug,
    Number(params.seasonNumber),
    Number(params.episodeNumber)
  );
  return <EpisodePageView data={data} />;
}
