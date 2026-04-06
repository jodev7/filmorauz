import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import { getMovie } from "@/lib/api";
import { Metadata } from "next";
import { notFound } from "next/navigation";

interface PageProps {
  params: { slug: string };
}

export async function generateMetadata({ params }: PageProps): Promise<Metadata> {
  const { slug } = params;
  
  try {
    const movie = await getMovie(slug);
    return {
      title: `${movie.title} — FILMORAUZ`,
      description: movie.description,
    };
  } catch {
    return {
      title: "Film topilmadi — FILMORAUZ",
    };
  }
}

export default async function WatchPage({ params }: PageProps) {
  const { slug } = params;
  console.log("[WatchPage] Rendering for slug:", slug);
  let movie = null;
  
  try {
    movie = await getMovie(slug);
  } catch (error) {
    console.error("[WatchPage] Failed to fetch movie for slug:", slug, error);
  }

  console.log("[WatchPage] Movie result for slug:", slug, movie ? `found (${movie.id})` : "null");

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
