import Link from "next/link";
import { Film } from "lucide-react";

export default function NotFound() {
  return (
    <div className="min-h-screen bg-brand-dark flex items-center justify-center px-4">
      <div className="text-center">
        <Film size={48} className="text-brand-red mx-auto mb-6" />
        <h1 className="font-display text-6xl sm:text-8xl text-white tracking-wide mb-4">
          404
        </h1>
        <p className="text-gray-400 text-base sm:text-lg mb-8">
          Bu sahifa mavjud emas yoki o'chirilgan.
        </p>
        <div className="flex flex-col sm:flex-row items-center justify-center gap-3 sm:gap-4">
          <Link
            href="/"
            className="bg-brand-red hover:bg-orange-700 text-white font-semibold px-6 py-3 rounded-xl transition-colors"
          >
            Bosh sahifaga qaytish
          </Link>
          <Link
            href="/movies"
            className="bg-brand-card border border-brand-border hover:border-gray-500 text-white font-semibold px-6 py-3 rounded-xl transition-colors"
          >
            Kinolarni ko'rish
          </Link>
        </div>
      </div>
    </div>
  );
}
