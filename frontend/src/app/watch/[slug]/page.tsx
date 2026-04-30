import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import { getMovie } from "@/lib/api";
import { Metadata } from "next";
import { notFound } from "next/navigation";

interface PageProps {
  params: { slug: string };
}

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug } = params;

  try {
    const movie = await getMovie(slug);
    return {
      title: `${movie.title} — Tomosha qilish | FILMORAUZ`,
      description: movie.description,
      alternates: { canonical: `${SITE_URL}/movies/${slug}` },
      robots: { index: false, follow: true },
    };
  } catch {
    return {
      title: "Film topilmadi — FILMORAUZ",
      robots: { index: false, follow: false },
    };
  }
}

export default async function WatchPage({ params }: PageProps) {
  const { slug } = params;
  let movie = null;

  try {
    movie = await getMovie(slug);
  } catch {
    // fall through to notFound
  }

  if (!movie) {
    notFound();
  }

  return (
    <>
      <Navbar />
      <WatchPageClient movie={movie} />
      <Footer />
    </>
  );
}
