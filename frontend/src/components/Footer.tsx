import Link from "next/link";
import { Film } from "lucide-react";
import { getTranslations } from "@/lib/i18n-server";

export default function Footer() {
  const { t } = getTranslations("uz");
  
  return (
    <footer className="border-t border-brand-border bg-brand-dark mt-20">
      <div className="max-w-7xl mx-auto px-4 py-10 sm:py-12">
        <div className="flex flex-col md:flex-row justify-between items-start gap-8">
          {/* Brand */}
          <div>
            <Link href="/" className="flex items-center gap-2 mb-3">
              <Film className="text-brand-red" size={20} />
              <span className="font-display text-2xl tracking-wider">
                FILMORA<span className="text-brand-red">UZ</span>
              </span>
            </Link>
            <p className="text-gray-500 text-sm max-w-xs">
              {t("footer.tagline")}
              Disclaimer: Filmlarga bo'lgan huquq ularning mualliflariga tegishli. Barcha filmlar faqat ma'lumot olish uchun mo'ljallangan. Foydalanuvchilar joylashtirgan noqonuniy materiallar uchun ma'muriyat javobgar emas! Har qanday film mualliflik huquqi egasining iltimosiga binoan olib tashlanadi.
            </p>
          </div>

          {/* Links */}
          <div className="flex gap-8 sm:gap-12 text-sm text-gray-400">
            <div className="flex flex-col gap-2">
              <span className="text-white font-medium mb-1">{t("footer.browse")}</span>
              <Link href="/movies" className="hover:text-white transition-colors">{t("footer.allMovies")}</Link>
              <Link href="/movies?genre=Action" className="hover:text-white transition-colors">{t("common.action")}</Link>
              <Link href="/movies?genre=Drama" className="hover:text-white transition-colors">{t("common.drama")}</Link>
              <Link href="/movies?genre=Comedy" className="hover:text-white transition-colors">{t("common.comedy")}</Link>
            </div>
          </div>
        </div>

        <div className="mt-8 sm:mt-10 pt-6 border-t border-brand-border text-center text-xs text-gray-600">
          © {new Date().getFullYear()} FilmoraUz. {t("footer.rights")}
        </div>
      </div>
    </footer>
  );
}
