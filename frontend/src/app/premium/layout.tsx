import type { Metadata } from "next";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

export const metadata: Metadata = {
  title: "Premium obuna — reklama­siz, HD, tezroq | FilmoraUz",
  description:
    "FilmoraUz Premium: reklamasiz tomosha, eng yuqori HD sifat, tezroq yuklash va premium kontent. Telegram orqali oson obuna bo'ling.",
  alternates: { canonical: `${SITE_URL}/premium` },
  openGraph: {
    title: "FilmoraUz Premium",
    description:
      "Reklamasiz, HD sifatda va tezroq tomosha qiling. FilmoraUz Premium obunasi.",
    url: `${SITE_URL}/premium`,
    siteName: "FILMORAUZ",
    type: "website",
    locale: "uz_UZ",
  },
  twitter: { card: "summary", title: "FilmoraUz Premium" },
  robots: { index: true, follow: true },
};

export default function PremiumLayout({ children }: { children: React.ReactNode }) {
  return <>{children}</>;
}
