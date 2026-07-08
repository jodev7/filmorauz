"use client";

import { useState } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import Logo from "@/components/Logo";
import TelegramLoginModal from "@/components/TelegramLoginModal";
import { useAuth } from "@/lib/auth-context";
import { buyPremium, createPremiumStarsSession } from "@/lib/api";
import {
  Crown,
  Check,
  Sparkles,
  Tv,
  Gauge,
  Film,
  ChevronDown,
  ChevronUp,
  ArrowDown,
  Wallet,
  ExternalLink,
  Star,
  Users,
} from "lucide-react";

const TELEGRAM_BOT_USERNAME =
  process.env.NEXT_PUBLIC_TELEGRAM_BOT_USERNAME || "FilmoraUzBot";

interface StarsPackage {
  id: string; // matches backend (1m, 3m, 6m, 12m)
  label: string;
  months: number;
  starsPrice: number;
  badge?: string;
}

const starsPackages: StarsPackage[] = [
  { id: "1m", label: "1 oy", months: 1, starsPrice: 100 },
  { id: "3m", label: "3 oy", months: 3, starsPrice: 270, badge: "-10%" },
  { id: "6m", label: "6 oy", months: 6, starsPrice: 500, badge: "-17%" },
  { id: "12m", label: "12 oy", months: 12, starsPrice: 1000, badge: "-17%" },
];

function formatExpiryDate(value?: string | null): string {
  if (!value) return "";
  const normalized = value.trim();
  const isoMatch = normalized.match(/^(\d{4})-(\d{2})-(\d{2})/);
  if (isoMatch) {
    return `${isoMatch[3]}.${isoMatch[2]}.${isoMatch[1]}`;
  }
  const legacyMatch = normalized.match(/^(\d{4})\s+M?(\d{2})\s+(\d{2})$/i);
  if (legacyMatch) {
    return `${legacyMatch[3]}.${legacyMatch[2]}.${legacyMatch[1]}`;
  }
  const d = new Date(value);
  if (isNaN(d.getTime())) return "";
  const year = d.getFullYear();
  const month = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${day}.${month}.${year}`;
}

interface FAQItem {
  question: string;
  answer: string;
}

interface PricingPlan {
  id: string;
  months: number;
  totalPrice: number;
  monthlyEquivalent: number;
  discount: number;
  discountPercent: number;
  label: string;
  badge?: string;
  highlighted?: boolean;
}

// Pricing plans with discount logic
const MONTHLY_PRICE = 25000; // Base monthly price in so'm

const pricingPlans: PricingPlan[] = [
  {
    id: "1month",
    months: 1,
    totalPrice: 25000,
    monthlyEquivalent: 25000,
    discount: 0,
    discountPercent: 0,
    label: "1 oylik"
  },
  {
    id: "3month",
    months: 3,
    totalPrice: 69000,
    monthlyEquivalent: 23000,
    discount: 6000,
    discountPercent: 8,
    label: "3 oylik",
    badge: "Eng mashhur"
  },
  {
    id: "6month",
    months: 6,
    totalPrice: 118000,
    monthlyEquivalent: 19667,
    discount: 32000,
    discountPercent: 21,
    label: "6 oylik",
    badge: "Eng foydali"
  },
  {
    id: "12month",
    months: 12,
    totalPrice: 234000,
    monthlyEquivalent: 19500,
    discount: 66000,
    discountPercent: 22,
    label: "12 oylik",
    badge: "Chegirma"
  }
];

const premiumFeatures = [
  "Reklamasiz tomosha",
  "720p va 1080p sifat",
  "Tezroq stream",
  "Serial auto-next",
  "Premium kontent",
  "Barcha qurilmalarda foydalanish",
  "Cheksiz film va seriallar",
  "Birga ko'rish: kuniga cheksiz room (free 3 ta)",
  "Birga ko'rish: 20 ta a'zo (free 2 ta)",
];

// Side-by-side limits table — Free vs Premium for the "Birga ko'rish" room
// feature. Kept as a const so the same data drives a future i18n pass
// without hunting through JSX.
const roomLimitRows: Array<{ label: string; free: string; premium: string }> = [
  { label: "Kuniga ochiladigan room", free: "3 ta", premium: "Cheksiz" },
  { label: "Bir roomda a'zolar", free: "2 ta (siz bilan)", premium: "20 ta (siz bilan)" },
  { label: "Ko'rinish", free: "Maxfiy + Ochiq", premium: "Maxfiy + Ochiq" },
  { label: "Taklif havolasi", free: "10 daqiqa", premium: "10 daqiqa" },
];

export default function PremiumPage() {
  const { user, token, isAuthenticated, refreshUser } = useAuth();
  const [openFaqIndex, setOpenFaqIndex] = useState<number | null>(null);
  const [loginModalOpen, setLoginModalOpen] = useState(false);
  const [starsLoadingPackage, setStarsLoadingPackage] = useState<string | null>(null);
  const [starsError, setStarsError] = useState<string>("");
  const [starsFallbackUrl, setStarsFallbackUrl] = useState<string>("");
  const [manualLoadingPlan, setManualLoadingPlan] = useState<string | null>(null);
  const [manualMessage, setManualMessage] = useState<string>("");

  // Check if user is premium
  const isPremium = user?.is_premium === true || user?.is_premium_active === true;
  const isLoggedIn = isAuthenticated && !!user;
  const hasLinkedTelegram = !!user?.telegram_id;
  const walletBalance = user?.wallet_balance ?? 0;
  const premiumExpiresLabel = formatExpiryDate(user?.premium_expires_at);
  const canBuyStars = isLoggedIn && hasLinkedTelegram && !!token && !isPremium;
  const requiresLogin = !isLoggedIn;
  const requiresTelegramLink = isLoggedIn && !hasLinkedTelegram;

  const features = [
    {
      icon: <Tv className="w-8 h-8" />,
      title: "Reklamasiz tomosha",
      description: "Filmlarni va seriallarni reklama bo'lmadan tomosha qiling"
    },
    {
      icon: <Gauge className="w-8 h-8" />,
      title: "Tezroq stream",
      description: "Premium foydalanuvchilar uchun tezroq stream yo'li va barqaror tomosha"
    },
    {
      icon: <Film className="w-8 h-8" />,
      title: "720p / 1080p sifat",
      description: "Yuqori sifatdagi tomosha rejimlari va aniq tasvir"
    },
    {
      icon: <Sparkles className="w-8 h-8" />,
      title: "Serial auto-next",
      description: "Epizod tugashi bilan keyingi qism avtomatik boshlanadi"
    },
    {
      icon: <Crown className="w-8 h-8" />,
      title: "Premium kontent",
      description: "Faqat Premium foydalanuvchilar uchun mo'ljallangan kontentga kirish"
    },
    {
      icon: <Users className="w-8 h-8" />,
      title: "Birga ko'rish: cheksiz",
      description: "Cheksiz room va 20 ta a'zo (free 3 ta room / 2 a'zo)"
    }
  ];

  const faqs: FAQItem[] = [
    {
      question: "Premium qanday faollashtiriladi?",
      answer: "Premium sotib olgandan so'ng, hisobingiz avtomatik ravishda Premium holatiga o'tkaziladi. Bu jarayon bir necha daqiqa ichida amalga oshadi."
    },
    {
      question: "Premium muddati qancha?",
      answer: "Premium obuna muddatini tanlashingiz mumkin: 1, 3, 6 yoki 12 oy. Muddat tugagandan so'ng, Premium avtomatik ravishda bekor qilinadi."
    },
    {
      question: "Premium bekor qilsam bo'ladimi?",
      answer: "Ha, istalgan vaqtda Premium-ni bekor qilishingiz mumkin. Bekor qilgandan so'ng, qolgan kunlar uchun Premium-dan foydalanishda davom etasiz."
    },
    {
      question: "To'lov qanday amalga oshiriladi?",
      answer: "Hozirda to'lovlar Telegram orqali amalga oshiriladi. Bot orqali admin bilan bog'lanib, to'lov qilishingiz mumkin."
    }
  ];

  const toggleFaq = (index: number) => {
    setOpenFaqIndex(openFaqIndex === index ? null : index);
  };

  const scrollToPurchaseOptions = () => {
    if (typeof document === "undefined") return;
    const target =
      document.getElementById("telegram-stars") ||
      document.getElementById("admin-premium");
    target?.scrollIntoView({ behavior: "smooth", block: "start" });
  };

  const formatPrice = (price: number) => {
    return price.toLocaleString('en-US');
  };

  const handleLoginRequired = () => {
    setLoginModalOpen(true);
  };

  const scrollToTopUpCard = () => {
    if (typeof document === "undefined") return;
    document.getElementById("manual-topup-card")?.scrollIntoView({
      behavior: "smooth",
      block: "center",
    });
  };

  const isMobileDevice = () => {
    if (typeof navigator === "undefined") return false;
    return /Android|iPhone|iPad|iPod|Mobile|IEMobile|Opera Mini/i.test(
      navigator.userAgent
    );
  };

  const buildTelegramLinks = (botURL: string) => {
    try {
      const parsed = new URL(botURL);
      const domain = parsed.pathname.replace(/^\/+/, "") || TELEGRAM_BOT_USERNAME;
      const start = parsed.searchParams.get("start") || "";
      return {
        webURL: `https://t.me/${domain}?start=${encodeURIComponent(start)}`,
        appURL: `tg://resolve?domain=${encodeURIComponent(domain)}&start=${encodeURIComponent(start)}`,
      };
    } catch {
      return {
        webURL: botURL,
        appURL: `tg://resolve?domain=${encodeURIComponent(TELEGRAM_BOT_USERNAME)}`,
      };
    }
  };

  const handleStarsPurchase = async (packageId: string) => {
    if (!token || !canBuyStars) return;
    setStarsError("");
    setStarsFallbackUrl("");
    setStarsLoadingPackage(packageId);
    try {
      const session = await createPremiumStarsSession(token, packageId);
      const { webURL, appURL } = buildTelegramLinks(session.bot_url);
      setStarsFallbackUrl(webURL);

      if (typeof window !== "undefined") {
        if (isMobileDevice()) {
          window.location.href = appURL;
          window.setTimeout(() => {
            window.location.href = webURL;
          }, 1000);
        } else {
          window.open(webURL, "_blank", "noopener,noreferrer");
        }
      }
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Telegram Stars to'lovini tayyorlashda xatolik yuz berdi.";
      setStarsError(message);
    } finally {
      setStarsLoadingPackage(null);
    }
  };

  const handleManualPremiumPurchase = async (planId: string, totalPrice: number) => {
    if (!token || !isLoggedIn) {
      handleLoginRequired();
      return;
    }

    if (walletBalance < totalPrice) {
      setManualMessage("Hisobingizda mablag‘ yetarli emas. Avval hisobni to‘ldiring.");
      scrollToTopUpCard();
      return;
    }

    setManualMessage("");
    setManualLoadingPlan(planId);
    try {
      const result = await buyPremium(token, planId);
      setManualMessage(result.message || "Premium muvaffaqiyatli faollashtirildi");
      await refreshUser();
    } catch (error) {
      const message =
        error instanceof Error
          ? error.message
          : "Premium sotib olishda xatolik yuz berdi.";
      setManualMessage(message);
    } finally {
      setManualLoadingPlan(null);
    }
  };

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24">
        {/* Hero Section */}
        <section className="relative overflow-hidden">
          {/* Background glow effects */}
          <div className="absolute top-0 left-1/2 -translate-x-1/2 w-[800px] h-[600px] bg-gradient-to-b from-orange-500/20 via-brand-red/10 to-transparent rounded-full blur-3xl" />
          <div className="absolute top-40 left-1/4 w-64 h-64 bg-yellow-500/10 rounded-full blur-3xl" />
          <div className="absolute top-60 right-1/4 w-80 h-80 bg-amber-500/10 rounded-full blur-3xl" />

          <div className="relative max-w-4xl mx-auto px-4 pt-16 sm:pt-24 pb-12 text-center">
            {/* Crown icon */}
            <div className="inline-flex items-center justify-center w-20 h-20 rounded-full bg-gradient-to-br from-yellow-500 to-amber-600 shadow-[0_0_40px_rgba(234,179,8,0.4)] mb-8">
              <Crown className="w-10 h-10 text-black" />
            </div>

            {/* Title */}
            <h1 className="mb-4 font-poppins font-semibold tracking-tight text-4xl sm:text-5xl md:text-6xl">
              <Logo className="text-4xl sm:text-5xl md:text-6xl" />{" "}
              <span className="bg-gradient-to-r from-yellow-400 to-amber-500 bg-clip-text text-transparent">
                PREMIUM
              </span>
            </h1>

            {/* Subtitle */}
            <p className="text-xl sm:text-2xl text-gray-300 mb-8 max-w-2xl mx-auto">
              Filmlarni va seriallarni Premium sifatida tomosha qiling
            </p>

            {/* Benefits tags */}
            <div className="flex flex-wrap justify-center gap-3 mb-10">
              {["Reklamasiz", "Tezroq stream", "1080p", "Auto-next", "Premium kontent"].map((benefit) => (
                <span
                  key={benefit}
                  className="px-4 py-2 bg-gradient-to-r from-yellow-500/20 to-amber-500/20 border border-yellow-500/30 rounded-full text-yellow-400 text-sm font-medium"
                >
                  {benefit}
                </span>
              ))}
            </div>

            {/* CTA Button or Already Premium state */}
            {isPremium ? (
              <div className="inline-flex flex-col items-center gap-1 px-8 py-4 bg-gradient-to-r from-green-500/20 to-emerald-500/20 border border-green-500/40 rounded-xl text-green-400">
                <div className="flex items-center gap-3">
                  <Check className="w-6 h-6" />
                  <span className="text-lg font-semibold">Sizda Premium faol</span>
                </div>
                {premiumExpiresLabel && (
                  <span className="text-sm text-green-300/80">
                    Tugash sanasi: {premiumExpiresLabel}
                  </span>
                )}
              </div>
            ) : requiresLogin ? (
              <div className="inline-flex flex-col items-center gap-4 px-8 py-5 glass-card border border-white/10 rounded-2xl">
                <p className="text-sm text-gray-300">Premium olish uchun avval saytga kiring.</p>
                <button
                  onClick={handleLoginRequired}
                  className="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-gradient-to-r from-brand-red to-orange-600 text-white font-semibold"
                >
                  Kirish
                </button>
              </div>
            ) : requiresTelegramLink ? (
              <div className="inline-flex flex-col items-center gap-4 px-8 py-5 glass-card border border-yellow-500/20 rounded-2xl">
                <p className="text-sm text-gray-300">Stars orqali premium olish uchun profilingizni Telegram bilan bog&apos;lang.</p>
                <button
                  onClick={handleLoginRequired}
                  className="inline-flex items-center gap-2 px-6 py-3 rounded-xl bg-yellow-500/10 border border-yellow-500/30 text-yellow-400 font-semibold"
                >
                  Telegramni bog&apos;lash
                </button>
              </div>
            ) : (
              <button
                onClick={scrollToPurchaseOptions}
                className="group inline-flex items-center gap-3 px-10 py-4 bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white font-bold text-lg rounded-xl shadow-[0_0_30px_rgba(249,115,22,0.4)] transition-all duration-300 hover:shadow-[0_0_50px_rgba(249,115,22,0.6)] hover:scale-105 disabled:opacity-70 disabled:cursor-not-allowed"
              >
                <>
                  <Crown className="w-6 h-6" />
                  Premium olish
                </>
              </button>
            )}
          </div>
        </section>

        {/* Features Section */}
        <section className="py-16 sm:py-24">
          <div className="max-w-5xl mx-auto px-4">
            <h2 className="font-display text-3xl sm:text-4xl text-white text-center mb-12">
              Premium <span className="text-brand-red">afzalliklari</span>
            </h2>

            {/* Birga ko'rish (room) comparison — Free vs Premium. Sits
                inside the Features section so a user weighing the upgrade
                immediately sees what changes for their movie-night flow. */}
            <div className="mb-10 rounded-2xl border border-white/10 glass-card backdrop-blur-sm overflow-hidden">
              <div className="px-5 py-4 border-b border-white/10 flex items-center gap-2">
                <Users className="w-5 h-5 text-brand-red" />
                <h3 className="font-semibold text-white">Birga ko&apos;rish (Watch Together) limitlari</h3>
              </div>
              <div className="grid grid-cols-3 text-sm">
                <div className="px-4 py-2 text-xs uppercase tracking-wider text-gray-500 border-b border-white/10">
                  Imkoniyat
                </div>
                <div className="px-4 py-2 text-xs uppercase tracking-wider text-gray-500 border-b border-white/10 text-center">
                  Free
                </div>
                <div className="px-4 py-2 text-xs uppercase tracking-wider text-brand-red border-b border-white/10 text-center font-semibold">
                  Premium
                </div>
                {roomLimitRows.map((row, i) => (
                  <div key={row.label} className="contents">
                    <div className={`px-4 py-3 text-gray-300 ${i < roomLimitRows.length - 1 ? "border-b border-white/5" : ""}`}>
                      {row.label}
                    </div>
                    <div className={`px-4 py-3 text-center text-gray-400 ${i < roomLimitRows.length - 1 ? "border-b border-white/5" : ""}`}>
                      {row.free}
                    </div>
                    <div className={`px-4 py-3 text-center text-brand-red font-medium ${i < roomLimitRows.length - 1 ? "border-b border-white/5" : ""}`}>
                      {row.premium}
                    </div>
                  </div>
                ))}
              </div>
              <p className="px-5 py-3 text-[11px] text-gray-500 border-t border-white/10 bg-black/20">
                Free hisobda kunlik 3 room limiti tugasa, &quot;Birga ko&apos;rish&quot;
                tugmasi cheklov xabarini ko&apos;rsatadi. Premium hisobda esa
                kuniga ochiladigan roomlar soni cheklanmagan.
              </p>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
              {features.map((feature, index) => (
                <div
                  key={index}
                  className="group relative p-6 glass-card backdrop-blur-sm border border-white/10 rounded-2xl hover:border-brand-red/50 transition-all duration-300 hover:glass-card"
                >
                  {/* Glow effect on hover */}
                  <div className="absolute inset-0 rounded-2xl bg-gradient-to-br from-brand-red/5 to-transparent opacity-0 group-hover:opacity-100 transition-opacity" />
                  
                  <div className="relative">
                    <div className="inline-flex items-center justify-center w-14 h-14 rounded-xl bg-gradient-to-br from-brand-red/20 to-orange-500/20 text-brand-red mb-4">
                      {feature.icon}
                    </div>
                    <h3 className="text-xl font-semibold text-white mb-2">
                      {feature.title}
                    </h3>
                    <p className="text-gray-400">
                      {feature.description}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Telegram Stars Section */}
        {!isPremium && (
        <section id="telegram-stars" className="py-12 sm:py-16 scroll-mt-28">
          <div className="max-w-6xl mx-auto px-4">
            {requiresLogin && (
              <div className="max-w-2xl mx-auto mb-8 rounded-2xl border border-white/10 glass-card p-5 text-center">
                <p className="text-white font-medium mb-3">Premium olish uchun avval saytga kiring.</p>
                <button
                  onClick={handleLoginRequired}
                  className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-brand-red to-orange-600 text-white font-semibold"
                >
                  Kirish
                </button>
              </div>
            )}
            {requiresTelegramLink && (
              <div className="max-w-2xl mx-auto mb-8 rounded-2xl border border-yellow-500/20 bg-yellow-500/5 p-5 text-center">
                <p className="text-yellow-200 font-medium mb-3">Stars orqali premium olish uchun profilingizni Telegram bilan bog&apos;lang.</p>
                <button
                  onClick={handleLoginRequired}
                  className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl border border-yellow-500/30 text-yellow-400 font-semibold hover:bg-yellow-500/10 transition-colors"
                >
                  Telegramni bog&apos;lash
                </button>
              </div>
            )}
            <div className="text-center mb-8">
              <div className="inline-flex items-center gap-2 px-4 py-1.5 rounded-full bg-yellow-500/10 border border-yellow-500/30 text-yellow-400 text-sm font-medium mb-4">
                <Star className="w-4 h-4" fill="currentColor" />
                Telegram Stars orqali
              </div>
              <h2 className="font-display text-3xl sm:text-4xl text-white mb-3">
                Botdan <span className="text-brand-red">avtomatik</span> olish
              </h2>
              <p className="text-gray-400 max-w-2xl mx-auto">
                FilmoraUz Premium ni Telegram Stars orqali bir necha soniyada faollashtiring.
                Tugmani bosing — bot to'lov chekini ko'rsatadi va to'lovdan so'ng premium avtomatik
                yoqiladi.
              </p>
              <p className="text-xs text-gray-500 mt-2">
                Stars to&apos;lovi faqat saytga kirilgan va Telegram hisobiga bog&apos;langan profil uchun ishlaydi.
              </p>
              <p className="text-sm text-yellow-400/90 mt-3 max-w-2xl mx-auto">
                Stars to'lovi Telegram mobil ilovasida yaxshiroq ishlaydi. Agar xatolik bo'lsa, admin orqali ham olishingiz mumkin.
              </p>
            </div>

            {starsError && (
              <div className="max-w-2xl mx-auto mb-6 rounded-2xl border border-red-500/20 bg-red-500/10 p-4 text-sm text-red-200 text-center">
                {starsError}
              </div>
            )}

            {starsFallbackUrl && (
              <div className="max-w-2xl mx-auto mb-6 text-center text-sm text-gray-400">
                Telegram ochilmasa,{" "}
                <a
                  href={starsFallbackUrl}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-yellow-400 hover:text-yellow-300 underline underline-offset-4"
                >
                  bu yerni bosing
                </a>
              </div>
            )}

            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 lg:gap-6">
              {starsPackages.map((pkg) => (
                <div
                  key={pkg.id}
                  className="relative rounded-2xl p-6 glass-card border border-white/10 hover:border-yellow-500/50 transition-all duration-300"
                >
                  {pkg.badge && (
                    <div className="absolute -top-3 right-4 px-3 py-1 rounded-full text-xs font-bold bg-gradient-to-r from-green-500 to-emerald-600 text-white">
                      {pkg.badge}
                    </div>
                  )}
                  <h3 className="text-lg font-semibold text-white mb-1 text-center">
                    {pkg.label}
                  </h3>
                  <div className="text-center mb-5">
                    <div className="flex items-baseline justify-center gap-1.5">
                      <Star className="w-5 h-5 text-yellow-400" fill="currentColor" />
                      <span className="text-3xl font-bold text-white">{pkg.starsPrice}</span>
                      <span className="text-sm text-gray-400">Stars</span>
                    </div>
                    <p className="text-xs text-gray-500 mt-1">{pkg.months} oylik premium</p>
                  </div>
                  <button
                    type="button"
                    disabled={!canBuyStars || starsLoadingPackage !== null}
                    onClick={() => handleStarsPurchase(pkg.id)}
                    className="flex items-center justify-center gap-2 w-full py-3 rounded-xl text-sm font-semibold bg-gradient-to-r from-yellow-500 to-amber-600 text-black hover:opacity-90 transition-opacity disabled:opacity-50 disabled:cursor-not-allowed"
                  >
                    <Star size={14} fill="currentColor" />
                    {starsLoadingPackage === pkg.id ? "Yuklanmoqda..." : "Telegram Stars orqali olish"}
                  </button>
                </div>
              ))}
            </div>
          </div>
        </section>
        )}

        {/* Pricing Section */}
        <section id="admin-premium" className="py-16 sm:py-24 scroll-mt-28">
          <div className="max-w-6xl mx-auto px-4">
            <h2 className="font-display text-3xl sm:text-4xl text-white text-center mb-2">
              Admin orqali — <span className="text-brand-red">hisob to&apos;ldirish</span>
            </h2>
            <p className="text-center text-xs uppercase tracking-wider text-gray-500 mb-4">
              An&apos;anaviy usul
            </p>
            <p className="text-gray-400 text-center mb-8 max-w-xl mx-auto">
              Qancha uzoq obuna olsangiz, shuncha ko'proq tejaysiz
            </p>

            {requiresLogin && (
              <div className="max-w-2xl mx-auto mb-8 rounded-2xl border border-white/10 glass-card p-5 text-center">
                <p className="text-white font-medium mb-3">Premium olish uchun avval saytga kiring.</p>
                <button
                  onClick={handleLoginRequired}
                  className="inline-flex items-center gap-2 px-5 py-2.5 rounded-xl bg-gradient-to-r from-brand-red to-orange-600 text-white font-semibold"
                >
                  Kirish
                </button>
              </div>
            )}

            {/* Wallet balance card */}
            {isLoggedIn && (
              <div id="manual-topup-card" className="max-w-md mx-auto mb-10 p-5 glass-card border border-white/10 rounded-2xl flex items-center justify-between gap-4">
                <div className="flex items-center gap-3">
                  <div className="w-10 h-10 rounded-xl bg-gradient-to-br from-yellow-500/20 to-amber-500/20 flex items-center justify-center">
                    <Wallet className="w-5 h-5 text-yellow-400" />
                  </div>
                  <div>
                    <p className="text-xs text-gray-400">Hisob balansi</p>
                    <p className="text-lg font-bold text-white">
                      {walletBalance.toLocaleString("en-US")}{" "}
                      <span className="text-sm font-normal text-gray-400">so'm</span>
                    </p>
                  </div>
                </div>
                <a
                  href="https://t.me/filmorauznet?direct"
                  target="_blank"
                  rel="noopener noreferrer"
                  className="flex items-center gap-2 px-4 py-2 bg-brand-red/20 border border-brand-red/50 text-brand-red hover:bg-brand-red hover:text-white rounded-xl text-sm font-semibold transition-all duration-200"
                >
                  <ExternalLink className="w-4 h-4" />
                  Hisobni to'ldirish
                </a>
              </div>
            )}

            {manualMessage && (
              <div className={`max-w-2xl mx-auto mb-6 rounded-2xl border p-4 text-sm text-center ${
                manualMessage.toLowerCase().includes("muvaffaqiyatli")
                  ? "border-green-500/20 bg-green-500/10 text-green-200"
                  : "border-yellow-500/20 bg-yellow-500/10 text-yellow-100"
              }`}>
                {manualMessage}
              </div>
            )}

            {/* Pricing Cards Grid */}
            <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4 lg:gap-6">
              {pricingPlans.map((plan) => {
                const isHighlighted = plan.badge === "Eng mashhur";
                const hasDiscount = plan.discountPercent > 0;
                
                return (
                  <div
                    key={plan.id}
                    className={`relative rounded-2xl p-6 transition-all duration-300 ${
                      isHighlighted
                        ? 'bg-gradient-to-br from-[#1a1a24] to-[#12121A] border-2 border-brand-red shadow-[0_0_30px_rgba(249,115,22,0.2)] scale-[1.02]'
                        : 'glass-card border border-white/10 hover:border-brand-red/50'
                    }`}
                  >
                    {/* Badge */}
                    {plan.badge && (
                      <div className={`absolute -top-3 left-1/2 -translate-x-1/2 px-4 py-1 rounded-full text-xs font-semibold ${
                        isHighlighted
                          ? 'bg-gradient-to-r from-brand-red to-orange-600 text-white'
                          : 'bg-gradient-to-r from-yellow-500/20 to-amber-500/20 border border-yellow-500/30 text-yellow-500'
                      }`}>
                        {plan.badge}
                      </div>
                    )}

                    {/* Discount badge for plans with discount */}
                    {hasDiscount && (
                      <div className="absolute -top-3 right-4 px-3 py-1 rounded-full text-xs font-bold bg-gradient-to-r from-green-500 to-emerald-600 text-white">
                        -{plan.discountPercent}%
                      </div>
                    )}

                    {/* Plan header */}
                    <div className="text-center mb-4">
                      <h3 className="text-lg font-semibold text-white mb-1">
                        {plan.label}
                      </h3>
                      {hasDiscount && (
                        <p className="text-xs text-green-400 flex items-center justify-center gap-1">
                          <ArrowDown size={12} />
                          {formatPrice(plan.discount)} so'm tejaysiz
                        </p>
                      )}
                    </div>

                    {/* Price */}
                    <div className="text-center mb-4">
                      <div className="flex items-baseline justify-center gap-1">
                        <span className="text-3xl font-bold text-white">{formatPrice(plan.totalPrice)}</span>
                        <span className="text-sm text-gray-400">so'm</span>
                      </div>
                      {hasDiscount && (
                        <p className="text-xs text-gray-500 mt-1">
                          Oyiga {formatPrice(plan.monthlyEquivalent)} so'mdan
                        </p>
                      )}
                    </div>

                    {/* Divider */}
                    <div className="border-t border-white/5 my-4" />

                    {/* Features */}
                    <ul className="space-y-3 mb-6">
                      {premiumFeatures.map((feature, idx) => (
                        <li key={idx} className="flex items-start gap-2 text-sm">
                          <div className="flex-shrink-0 w-5 h-5 rounded-full bg-gradient-to-br from-yellow-500 to-amber-600 flex items-center justify-center mt-0.5">
                            <Check size={12} className="text-black" />
                          </div>
                          <span className="text-gray-300">{feature}</span>
                        </li>
                      ))}
                    </ul>

                    {/* CTA Button */}
                    {isPremium ? (
                      <div className="flex items-center justify-center gap-2 w-full py-3 bg-gradient-to-r from-green-500/20 to-emerald-500/20 border border-green-500/40 rounded-xl text-green-400 text-sm font-semibold">
                        <Check size={16} />
                        Premium
                      </div>
                    ) : !isLoggedIn ? (
                      <button
                        type="button"
                        onClick={handleLoginRequired}
                        className="w-full py-3 rounded-xl border border-white/10 text-gray-300 text-sm font-semibold hover:border-brand-red/40 hover:text-white transition-colors"
                      >
                        Kirish
                      </button>
                    ) : (
                      <button
                        type="button"
                        onClick={() => handleManualPremiumPurchase(plan.id, plan.totalPrice)}
                        disabled={manualLoadingPlan !== null}
                        className={`w-full py-3 rounded-xl font-semibold text-sm transition-all duration-300 disabled:opacity-60 disabled:cursor-not-allowed ${
                          isHighlighted
                            ? "bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white shadow-[0_0_20px_rgba(249,115,22,0.3)] hover:shadow-[0_0_30px_rgba(249,115,22,0.5)]"
                            : "bg-brand-red/20 border border-brand-red/50 text-brand-red hover:bg-brand-red hover:text-white"
                        }`}
                      >
                        <>
                          <Crown size={14} className="inline-block mr-1" />
                          {manualLoadingPlan === plan.id ? "Yuklanmoqda..." : "Premium olish"}
                        </>
                      </button>
                    )}
                  </div>
                );
              })}
            </div>

            <p className="text-center text-gray-500 text-sm mt-8">
              Telegram Stars orqali avtomatik, admin orqali esa qo&apos;lda faollashtiriladi
            </p>
          </div>
        </section>

        {/* FAQ Section */}
        <section className="py-16 sm:py-24">
          <div className="max-w-3xl mx-auto px-4">
            <h2 className="font-display text-3xl sm:text-4xl text-white text-center mb-12">
              Ko'p so'raladigan <span className="text-brand-red">savollar</span>
            </h2>

            <div className="space-y-4">
              {faqs.map((faq, index) => (
                <div
                  key={index}
                  className="border border-white/10 rounded-xl overflow-hidden glass-card backdrop-blur-sm transition-colors hover:border-brand-red/30"
                >
                  <button
                    onClick={() => toggleFaq(index)}
                    className="w-full flex items-center justify-between p-5 text-left"
                  >
                    <span className="font-semibold text-white pr-4">{faq.question}</span>
                    <div className={`flex-shrink-0 w-8 h-8 rounded-full bg-brand-dark/50 flex items-center justify-center transition-transform duration-300 ${openFaqIndex === index ? 'rotate-180' : ''}`}>
                      {openFaqIndex === index ? (
                        <ChevronUp className="w-5 h-5 text-brand-red" />
                      ) : (
                        <ChevronDown className="w-5 h-5 text-gray-400" />
                      )}
                    </div>
                  </button>
                  
                  <div className={`overflow-hidden transition-all duration-300 ${openFaqIndex === index ? 'max-h-48' : 'max-h-0'}`}>
                    <p className="px-5 pb-5 text-gray-400 leading-relaxed">
                      {faq.answer}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        </section>

        {/* Bottom CTA Section */}
        <section className="py-16 sm:py-24">
          <div className="max-w-4xl mx-auto px-4 text-center">
            <div className="relative p-10 sm:p-14 bg-gradient-to-br from-[#1a1a24] to-[#12121A] border border-yellow-500/20 rounded-3xl overflow-hidden">
              {/* Background glow */}
              <div className="absolute top-0 left-1/2 -translate-x-1/2 w-96 h-48 bg-yellow-500/10 rounded-full blur-3xl" />
              
              <div className="relative">
                <Crown className="w-16 h-16 text-yellow-500 mx-auto mb-6" />
                <h2 className="font-display text-3xl sm:text-4xl text-white mb-4">
                  Premium bilan <span className="text-brand-red">chegara</span> yo'q
                </h2>
                <p className="text-gray-400 text-lg mb-8 max-w-xl mx-auto">
                  Barcha filmlar va seriallarni cheklovlarsiz tomosha qiling
                </p>
                
                {isPremium ? (
                  <div className="inline-flex items-center gap-3 px-8 py-4 bg-gradient-to-r from-green-500/20 to-emerald-500/20 border border-green-500/40 rounded-xl text-green-400 font-semibold">
                    <Check className="w-6 h-6" />
                    Siz allaqachon Premiumsiz
                  </div>
                ) : requiresLogin ? (
                  <button
                    onClick={handleLoginRequired}
                    className="group inline-flex items-center gap-3 px-10 py-4 bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white font-bold text-lg rounded-xl shadow-[0_0_30px_rgba(249,115,22,0.4)] transition-all duration-300 hover:shadow-[0_0_50px_rgba(249,115,22,0.6)] hover:scale-105"
                  >
                    Kirish
                  </button>
                ) : requiresTelegramLink ? (
                  <button
                    onClick={handleLoginRequired}
                    className="group inline-flex items-center gap-3 px-10 py-4 bg-yellow-500/10 border border-yellow-500/30 text-yellow-400 font-bold text-lg rounded-xl transition-all duration-300 hover:bg-yellow-500/20"
                  >
                    Telegramni bog&apos;lash
                  </button>
                ) : (
                  <button
                    onClick={scrollToPurchaseOptions}
                    className="group inline-flex items-center gap-3 px-10 py-4 bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white font-bold text-lg rounded-xl shadow-[0_0_30px_rgba(249,115,22,0.4)] transition-all duration-300 hover:shadow-[0_0_50px_rgba(249,115,22,0.6)] hover:scale-105 disabled:opacity-70 disabled:cursor-not-allowed"
                  >
                    <>
                      <Crown className="w-6 h-6" />
                      Hozir Premium bo'ling
                    </>
                  </button>
                )}
              </div>
            </div>
          </div>
        </section>

      </main>
      <Footer />
      <TelegramLoginModal
        isOpen={loginModalOpen}
        onClose={() => setLoginModalOpen(false)}
      />
    </>
  );
}
