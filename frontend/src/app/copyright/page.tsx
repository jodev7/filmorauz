import type { Metadata } from "next";
import Link from "next/link";
import { Send } from "lucide-react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";

export const metadata: Metadata = {
  title: "Mualliflik huquqi — FILMORAUZ",
  description:
    "FilmoraUz saytida joylashtirilgan kontentning mualliflik huquqi va uchinchi shaxs materiallariga tegishli siyosat.",
  robots: { index: true, follow: true },
};

export default function CopyrightPage() {
  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24 bg-brand-dark">
        <div className="max-w-3xl mx-auto px-4 py-10 sm:py-14">
          <h1 className="font-display text-3xl sm:text-4xl md:text-5xl text-white tracking-wider mb-6">
            Mualliflik huquqi
          </h1>
          <p className="text-gray-400 text-sm sm:text-base mb-10">
            FilmoraUz va uchinchi shaxslarga tegishli kontent to&apos;g&apos;risida.
          </p>

          <div className="space-y-6 text-gray-300 text-sm sm:text-base leading-relaxed">
            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                Kontentga bo&apos;lgan huquqlar
              </h2>
              <p>
                FilmoraUz saytida joylashtirilgan filmlar, seriallar, posterlar va
                boshqa media materiallarning mualliflik huquqi ularning asl
                egalariga — prodyuserlik kompaniyalari, kinostudiyalar va
                tarqatuvchilarga tegishli. FilmoraUz ushbu materiallarga
                bo&apos;lgan huquqni o&apos;ziniki deb da&apos;vo qilmaydi.
              </p>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                Sayt maqsadi
              </h2>
              <p>
                Kontent faqat ma&apos;lumot olish va tanishish maqsadida
                taqdim etilgan. Barcha filmlar va seriallar ochiq manbalardan
                foydalanuvchilar uchun qulay tarzda to&apos;plangan. Foydalanuvchilar
                tomonidan joylashtirilgan materiallar uchun ma&apos;muriyat
                javobgarlikni o&apos;z zimmasiga olmaydi.
              </p>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-2">
                Kontentni olib tashlash
              </h2>
              <p>
                Agar siz mualliflik huquqi egasi bo&apos;lsangiz va biror material
                sizning ruxsatingizsiz joylashtirilgan deb hisoblasangiz, qonuniy
                asosli so&apos;rov yuboring. Tasdiqlangan so&apos;rov asosida
                tegishli material qisqa muddat ichida saytdan olib tashlanadi.
                Batafsil tartib{" "}
                <Link href="/dmca" className="text-brand-red hover:underline">
                  DMCA sahifasida
                </Link>{" "}
                keltirilgan.
              </p>
            </section>

            <section className="bg-brand-card/50 border border-brand-border rounded-2xl p-5 sm:p-6">
              <h2 className="text-white font-semibold text-lg sm:text-xl mb-3">
                Murojaat
              </h2>
              <p className="mb-3">
                Mualliflik huquqiga oid har qanday savol bilan bog&apos;lanish uchun:
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
