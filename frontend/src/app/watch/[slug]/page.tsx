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
