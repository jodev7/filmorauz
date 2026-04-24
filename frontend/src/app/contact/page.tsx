import type { Metadata } from "next";
import Link from "next/link";
import { Send, Megaphone, AlertCircle, Handshake, LifeBuoy } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";

export const metadata: Metadata = {
  title: "Aloqa — FILMORAUZ",
  description:
    "FilmoraUz bilan bog'lanish: reklama, shikoyat, hamkorlik va texnik yordam uchun Telegram aloqa kanali.",
  robots: { index: true, follow: true },
};

const topics = [
  {
    icon: Megaphone,
    title: "Reklama",
    description: "Saytda banner yoki video reklama joylashtirishni xohlasangiz.",
  },
  {
    icon: AlertCircle,
    title: "Shikoyat",
    description:
      "Buzilgan havola, noto'g'ri kontent yoki mualliflik huquqi bilan bog'liq muammolar.",
  },
  {
    icon: Handshake,
    title: "Hamkorlik",
    description: "Content provayder, studiya yoki tarqatuvchi sifatida hamkorlik.",
  },
  {
    icon: LifeBuoy,
    title: "Texnik yordam",
    description: "Hisob, to'lov, Premium yoki sayt ishlashi bo'yicha savollar.",
  },
];

export default function ContactPage() {
  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24 bg-brand-dark">
        <div className="max-w-3xl mx-auto px-4 py-10 sm:py-14">
          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wider mb-6">
            Aloqa
          </h1>
          <p className="text-gray-400 text-sm sm:text-base mb-10">
            Bizga savol, taklif yoki shikoyat bo&apos;lsa, Telegram orqali yozing.
          </p>

          <div className="bg-brand-card/50 border border-brand-border rounded-2xl p-6 sm:p-8 mb-8">
            <p className="text-gray-300 text-sm sm:text-base mb-4">
              Asosiy aloqa kanali — Telegram. Xabarlar kunlik rejimda
              ko&apos;rib chiqiladi.
            </p>
            <Link
              href="https://t.me/primeposuz"
              target="_blank"
              rel="noopener noreferrer"
              className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-6 py-3 rounded-lg transition-colors"
            >
              <Send size={18} />
              @primeposuz
            </Link>
          </div>

          <h2 className="text-white font-semibold text-lg sm:text-xl mb-4">
            Qaysi mavzular bo&apos;yicha yozish mumkin
          </h2>
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            {topics.map((topic) => {
              const Icon = topic.icon;
              return (
                <div
                  key={topic.title}
                  className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 hover:border-brand-red/50 transition-colors"
                >
                  <div className="inline-flex items-center justify-center w-10 h-10 rounded-lg bg-brand-red/15 text-brand-red mb-3">
                    <Icon size={18} />
                  </div>
                  <h3 className="text-white font-semibold text-base mb-1">
                    {topic.title}
                  </h3>
                  <p className="text-gray-400 text-sm leading-relaxed">
                    {topic.description}
                  </p>
                </div>
              );
            })}
          </div>
        </div>
      </main>
      <Footer />
    </>
  );
}
