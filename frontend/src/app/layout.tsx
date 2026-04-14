import type { Metadata } from "next";
import { Bebas_Neue, Inter } from "next/font/google";
import "./globals.css";
import { AuthProvider } from "@/lib/auth-context";
import { I18nProvider } from "@/lib/i18n";
import { AdSlotProvider } from "@/components/ads/AdSlotContext";
import BanGuardWrapper from "@/components/BanGuardWrapper";
import FixedBottomAd from "@/components/ads/FixedBottomAd";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.uz";

const bebasNeue = Bebas_Neue({
  weight: "400",
  subsets: ["latin"],
  variable: "--font-display",
  display: "swap",
});

const inter = Inter({
  subsets: ["latin"],
  variable: "--font-body",
  display: "swap",
});

export const metadata: Metadata = {
  metadataBase: new URL(SITE_URL),
  title: {
    default: "FILMORAUZ — Onlayn kinoteatr",
    template: "%s — FILMORAUZ",
  },
  description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
  keywords: ["filmlar", "onlayn tomosha", "kino", "uzbekistan", "kinolar", "tomosha", "filmlar", "onlayn kinoteatr"],
  authors: [{ name: "FILMORAUZ" }],
  creator: "FILMORAUZ",
  publisher: "FILMORAUZ",
  formatDetection: {
    email: false,
    address: false,
    telephone: false,
  },
  openGraph: {
    type: "website",
    locale: "uz_UZ",
    url: SITE_URL,
    siteName: "FILMORAUZ",
    title: "FILMORAUZ — Onlayn kinoteatr",
    description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
    images: [
      {
        url: `${SITE_URL}/og-image.jpg`,
        width: 1200,
        height: 630,
        alt: "FILMORAUZ",
      },
    ],
  },
  twitter: {
    card: "summary_large_image",
    title: "FILMORAUZ — Onlayn kinoteatr",
    description: "FILMORAUZ — O'zbekistondagi eng yaxshi onlayn kinoteatr. Yangi filmlar, klassika va mashhur filmlarni bepul tomosha qiling.",
    images: [`${SITE_URL}/og-image.jpg`],
    site: "@filmorauz",
  },
  alternates: {
    canonical: SITE_URL,
  },
  robots: {
    index: true,
    follow: true,
    googleBot: {
      index: true,
      follow: true,
      "max-video-preview": -1,
      "max-image-preview": "large",
      "max-snippet": -1,
    },
  },
  category: "entertainment",
  classification: "Movies, Streaming, Entertainment",
};

export default function RootLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <html lang="uz" className={`${bebasNeue.variable} ${inter.variable}`}>
      <link rel="icon" type="image/png" sizes="32x32" href="/favicon.png" />
      <body className="bg-brand-dark text-white font-body antialiased">
        <AuthProvider>
          <I18nProvider>
            <AdSlotProvider>
              <BanGuardWrapper>
                {children}
                <FixedBottomAd placement="website_fixed_bottom" />
              </BanGuardWrapper>
            </AdSlotProvider>
          </I18nProvider>
        </AuthProvider>
      </body>
    </html>
  );
}
