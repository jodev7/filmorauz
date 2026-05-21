import type { Metadata } from "next";
import { Bebas_Neue, Inter, Poppins } from "next/font/google";
import "./globals.css";
import { AuthProvider } from "@/lib/auth-context";
import { I18nProvider } from "@/lib/i18n";
import { AdSlotProvider } from "@/components/ads/AdSlotContext";
import BanGuardWrapper from "@/components/BanGuardWrapper";
import FixedBottomAd from "@/components/ads/FixedBottomAd";
import InAppBrowserBanner from "@/components/InAppBrowserBanner";
import PresencePinger from "@/components/PresencePinger";

const SITE_URL = process.env.NEXT_PUBLIC_SITE_URL || "https://filmorauz.net";

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

// Poppins — used by the watch-room chat for a softer, more friendly
// feel than the default UI font. Variable exposed app-wide so any
// "font-poppins" class on a child can pick it up.
const poppins = Poppins({
  weight: ["400", "500", "600"],
  subsets: ["latin"],
  variable: "--font-poppins",
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
    <html lang="uz" className={`${bebasNeue.variable} ${inter.variable} ${poppins.variable}`}>
      <head>
        <link rel="icon" type="image/png" sizes="32x32" href="/favicon.png" />
        <link rel="preconnect" href="https://cdn.filmorauz.net" crossOrigin="" />
        <link rel="preconnect" href="https://api.filmorauz.net" crossOrigin="" />
        {/*
          Site-wide JSON-LD: Organization gives Google the entity card and
          logo; WebSite + SearchAction unlocks the sitelinks search box in
          the SERP. Inline so every crawled page carries it.
        */}
        <script
          type="application/ld+json"
          dangerouslySetInnerHTML={{
            __html: JSON.stringify({
              "@context": "https://schema.org",
              "@graph": [
                {
                  "@type": "Organization",
                  "@id": `${SITE_URL}/#organization`,
                  name: "FILMORAUZ",
                  url: SITE_URL,
                  logo: `${SITE_URL}/favicon.png`,
                  sameAs: [
                    "https://t.me/filmorauz",
                  ],
                },
                {
                  "@type": "WebSite",
                  "@id": `${SITE_URL}/#website`,
                  url: SITE_URL,
                  name: "FILMORAUZ",
                  publisher: { "@id": `${SITE_URL}/#organization` },
                  inLanguage: "uz",
                  potentialAction: {
                    "@type": "SearchAction",
                    target: `${SITE_URL}/search?q={search_term_string}`,
                    "query-input": "required name=search_term_string",
                  },
                },
              ],
            }),
          }}
        />
      </head>
      <body className="bg-brand-dark text-white font-body antialiased">
        <AuthProvider>
          <I18nProvider>
            <AdSlotProvider>
              <BanGuardWrapper>
                <PresencePinger />
                <InAppBrowserBanner />
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
