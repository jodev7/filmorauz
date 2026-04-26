import type { Metadata } from "next";
import Link from "next/link";
import { Send } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";

export const metadata: Metadata = {
  title: "DMCA — FILMORAUZ",
  description:
    "FilmoraUz DMCA: mualliflik huquqi egasining talabiga ko'ra kontentni olib tashlash tartibi va aloqa ma'lumotlari.",
  robots: { index: true, follow: true },
};

export default function DmcaPage() {
  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24 bg-brand-dark">
        <div className="max-w-3xl mx-auto px-4 py-10 sm:py-14">
          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wider mb-6">
            DMCA
          </h1>
          <p className="text-gray-400 text-sm sm:text-base mb-10">
            Mualliflik huquqi va kontentni olib tashlash to&apos;g&apos;risidagi siyosat.
          </p>

          <div className="space-y-6 text-gray-300 text-sm sm:text-base leading-relaxed">
            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                DMCA nima?
              </h2>
              <p>
                DMCA (Digital Millennium Copyright Act) — mualliflik huquqi bilan
                himoyalangan kontentni internet xizmatlaridan olib tashlash uchun
                xalqaro qabul qilingan tartib. FilmoraUz ushbu tartibga rioya qiladi
                va qonuniy da&apos;volarga tezkor javob beradi.
              </p>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                Kontentni olib tashlashni so&apos;rash
              </h2>
              <p className="mb-3">
                Agar siz biror film yoki seriyaning mualliflik huquqi egasi bo&apos;lsangiz
                va ushbu kontent FilmoraUz saytida sizning ruxsatingizsiz joylashtirilgan
                deb hisoblasangiz, quyidagi ma&apos;lumotlarni yuboring:
              </p>
              <ul className="list-disc pl-5 space-y-1.5 text-gray-300">
                <li>O&apos;z ismi-sharifingiz yoki kompaniya nomi</li>
                <li>Asar nomi va havolasi (saytdagi manzil)</li>
                <li>Mualliflik huquqini tasdiqlovchi hujjat yoki dalil</li>
                <li>Aloqa uchun telefon yoki elektron pochta</li>
                <li>Da&apos;voning to&apos;g&apos;riligi to&apos;g&apos;risidagi tasdiq</li>
              </ul>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                Noqonuniy kontent haqida shikoyat
              </h2>
              <p>
                Foydalanuvchilar tomonidan yuborilgan shikoyatlar ma&apos;muriyat
                tomonidan ko&apos;rib chiqiladi. Agar da&apos;vo asosli bo&apos;lsa,
                tegishli kontent qisqa muddat ichida saytdan olib tashlanadi.
                Har bir murojaat yakka tartibda o&apos;rganiladi.
              </p>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-3">
                Aloqa
              </h2>
              <p className="mb-3">
                DMCA va kontentni olib tashlash bilan bog&apos;liq barcha so&apos;rovlar
                Telegram orqali qabul qilinadi:
              </p>
              <Link
                href="https://t.me/filmorauznet?direct"
                target="_blank"
                rel="noopener noreferrer"
                className="inline-flex items-center gap-2 bg-brand-red hover:bg-orange-700 text-white font-semibold px-5 py-2.5 rounded-lg transition-colors"
              >
                <Send size={16} />
                Telegram
              </Link>
            </section>
          </div>
        </div>
      </main>
      <Footer />
    </>
  );
}
