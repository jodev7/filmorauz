import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WatchPageClient from "@/components/watch/WatchPageClient";
import { getMovie } from "@/lib/api";
import { Metadata } from "next";

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
  } catch (error) {
    console.error("Failed to fetch movie:", error);
  }

  return (
    <>
      <Navbar />
      <WatchPageClient movie={movie as any} />
      <Footer />
    </>
  );
}
