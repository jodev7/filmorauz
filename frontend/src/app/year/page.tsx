import type { Metadata } from "next";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import WebsiteAdSlot from "@/components/ads/WebsiteAdSlot";
import YearReviewView from "@/components/YearReviewView";

export const metadata: Metadata = {
  title: "Yil yakuni",
  description: "Bu yil FilmoraUz'da nimalarni tomosha qildingiz: soatlar, sevimli janrlar va eng ko'p ko'rilgan kinolar.",
  robots: { index: false },
};

export default function YearPage({ searchParams }: { searchParams: { y?: string } }) {
  const y = parseInt(searchParams.y || "", 10);
  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-28 sm:pt-32">
        <YearReviewView initialYear={Number.isFinite(y) ? y : undefined} />
        <div className="mx-auto max-w-7xl px-4 py-8">
          <WebsiteAdSlot placement="year_page_banner" variant="banner" lazy />
        </div>
      </main>
      <Footer />
    </>
  );
}
