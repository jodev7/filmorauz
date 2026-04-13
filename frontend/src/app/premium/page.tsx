"use client";

import { useState } from "react";
import Navbar from "@/components/Navbar";
import Footer from "@/components/Footer";
import { useAuth } from "@/lib/auth-context";
import { useI18n } from "@/lib/i18n";
import { buyPremium } from "@/lib/api";
import {
  Crown,
  Check,
  Sparkles,
  Tv,
  Gauge,
  Film,
  ChevronDown,
  ChevronUp,
  Loader2,
  ArrowDown,
  Wallet,
  ExternalLink,
  AlertCircle,
} from "lucide-react";

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
const MONTHLY_PRICE = 39000; // Base monthly price in so'm

const pricingPlans: PricingPlan[] = [
  {
    id: "1month",
    months: 1,
    totalPrice: 39000,
    monthlyEquivalent: 39000,
    discount: 0,
    discountPercent: 0,
    label: "1 oylik"
  },
  {
    id: "3month",
    months: 3,
    totalPrice: 105000, // 10% discount: 3 * 39000 = 117000, with 10% off = 105300 ≈ 105000
    monthlyEquivalent: 35000,
    discount: 12000,
    discountPercent: 10,
    label: "3 oylik",
    badge: "Eng mashhur"
  },
  {
    id: "6month",
    months: 6,
    totalPrice: 195000, // 17% discount: 6 * 39000 = 234000, with 17% off = 194220 ≈ 195000
    monthlyEquivalent: 32500,
    discount: 39000,
    discountPercent: 17,
    label: "6 oylik",
    badge: "Eng foydali"
  },
  {
    id: "12month",
    months: 12,
    totalPrice: 340000, // 27% discount: 12 * 39000 = 468000, with 27% off = 341640 ≈ 340000
    monthlyEquivalent: 28333,
    discount: 128000,
    discountPercent: 27,
    label: "12 oylik",
    badge: "Chegirma"
  }
];

const premiumFeatures = [
  "Reklamasiz tomosha",
  "4K Ultra HD sifat",
  "Barcha qurilmalarda foydalanish",
  "Cheksiz film va seriallar",
  "Orqaga qolmasdan tomosha",
  "Premium mijozlar xizmati"
];

export default function PremiumPage() {
  const { user, token, refreshUser } = useAuth();
  const { t } = useI18n();
  const [openFaqIndex, setOpenFaqIndex] = useState<number | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const [selectedPlan, setSelectedPlan] = useState<string | null>(null);
  const [buyError, setBuyError] = useState<string | null>(null);
  const [buySuccess, setBuySuccess] = useState<string | null>(null);

  // Check if user is premium
  const isPremium = user?.is_premium === true || user?.is_premium_active === true;
  const walletBalance = user?.wallet_balance ?? 0;

  const features = [
    {
      icon: <Tv className="w-8 h-8" />,
      title: "Reklamasiz tomosha",
      description: "Filmlarni va seriallarni reklama bo'lmadan tomosha qiling"
    },
    {
      icon: <Gauge className="w-8 h-8" />,
      title: "Yuqori tezlik",
      description: "Tezkor serverlardan foydalanib, bufersiz tomosha qiling"
    },
    {
      icon: <Film className="w-8 h-8" />,
      title: "Full HD / 4K",
      description: "Eng yuqori sifatda 4K rezolyutsiyada filmlarni ko'ring"
    },
    {
      icon: <Sparkles className="w-8 h-8" />,
      title: "Maxsus kontent",
      description: "Premium foydalanuvchilar uchun maxsus kontent va filmlar"
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

  const handleGetPremium = async (planId: string) => {
    if (!user || !token) return;

    setBuyError(null);
    setBuySuccess(null);
    setIsLoading(true);
    setSelectedPlan(planId);

    try {
      const result = await buyPremium(token, planId);
      setBuySuccess(result.message || "Premium muvaffaqiyatli faollashtirildi");
      // Refresh user data to reflect new premium status and wallet balance
      await refreshUser();
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : "Xatolik yuz berdi";
      setBuyError(msg);
    } finally {
      setIsLoading(false);
      setSelectedPlan(null);
    }
  };

  const formatPrice = (price: number) => {
    return price.toLocaleString('en-US');
  };

  return (
    <>
      <Navbar />
      <main className="min-h-screen pt-20 sm:pt-24 bg-brand-dark">
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
            <h1 className="font-display text-4xl sm:text-5xl md:text-6xl text-white mb-4 tracking-wider">
              FILMORA<span className="text-brand-red">UZ</span> PREMIUM
            </h1>

            {/* Subtitle */}
            <p className="text-xl sm:text-2xl text-gray-300 mb-8 max-w-2xl mx-auto">
              Filmlarni va seriallarni Premium sifatida tomosha qiling
            </p>

            {/* Benefits tags */}
            <div className="flex flex-wrap justify-center gap-3 mb-10">
              {["No ads", "Fast", "HD", "Exclusive"].map((benefit) => (
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
              <div className="inline-flex items-center gap-3 px-8 py-4 bg-gradient-to-r from-green-500/20 to-emerald-500/20 border border-green-500/40 rounded-xl text-green-400">
                <Check className="w-6 h-6" />
                <span className="text-lg font-semibold">Siz allaqachon Premiumsiz</span>
              </div>
            ) : (
              <button
                onClick={() => handleGetPremium("3month")}
                disabled={isLoading}
                className="group inline-flex items-center gap-3 px-10 py-4 bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white font-bold text-lg rounded-xl shadow-[0_0_30px_rgba(249,115,22,0.4)] transition-all duration-300 hover:shadow-[0_0_50px_rgba(249,115,22,0.6)] hover:scale-105 disabled:opacity-70 disabled:cursor-not-allowed"
              >
                {isLoading ? (
                  <Loader2 className="w-6 h-6 animate-spin" />
                ) : (
                  <>
                    <Crown className="w-6 h-6" />
                    Premium olish
                  </>
                )}
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

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-6">
              {features.map((feature, index) => (
                <div
                  key={index}
                  className="group relative p-6 bg-brand-card/50 backdrop-blur-sm border border-brand-border rounded-2xl hover:border-brand-red/50 transition-all duration-300 hover:bg-brand-card/70"
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

        {/* Pricing Section */}
        <section className="py-16 sm:py-24">
          <div className="max-w-6xl mx-auto px-4">
            <h2 className="font-display text-3xl sm:text-4xl text-white text-center mb-4">
              Narx <span className="text-brand-red">rejalari</span>
            </h2>
            <p className="text-gray-400 text-center mb-8 max-w-xl mx-auto">
              Qancha uzoq obuna olsangiz, shuncha ko'proq tejaysiz
            </p>

            {/* Wallet balance card */}
            {user && (
              <div className="max-w-md mx-auto mb-10 p-5 bg-brand-card/60 border border-brand-border rounded-2xl flex items-center justify-between gap-4">
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

            {/* Buy status messages */}
            {buyError && (
              <div className="max-w-md mx-auto mb-6 flex items-center gap-3 p-4 bg-red-500/10 border border-red-500/30 rounded-xl text-red-400 text-sm">
                <AlertCircle className="w-5 h-5 flex-shrink-0" />
                {buyError}
              </div>
            )}
            {buySuccess && (
              <div className="max-w-md mx-auto mb-6 flex items-center gap-3 p-4 bg-green-500/10 border border-green-500/30 rounded-xl text-green-400 text-sm">
                <Check className="w-5 h-5 flex-shrink-0" />
                {buySuccess}
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
                        : 'bg-brand-card/50 border border-brand-border hover:border-brand-red/50'
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
                    <div className="border-t border-brand-border/50 my-4" />

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
                    ) : !user ? (
                      <div className="text-center text-xs text-gray-500 py-3">
                        Sotib olish uchun tizimga kiring
                      </div>
                    ) : walletBalance < plan.totalPrice ? (
                      <a
                        href="https://t.me/filmorauznet?start=topup"
                        target="_blank"
                        rel="noopener noreferrer"
                        className="flex items-center justify-center gap-2 w-full py-3 bg-yellow-500/10 border border-yellow-500/30 text-yellow-400 hover:bg-yellow-500/20 rounded-xl text-sm font-semibold transition-all duration-200"
                      >
                        <Wallet size={14} />
                        Hisobni to&apos;ldirish
                      </a>
                    ) : (
                      <button
                        onClick={() => handleGetPremium(plan.id)}
                        disabled={isLoading}
                        className={`w-full py-3 rounded-xl font-semibold text-sm transition-all duration-300 ${
                          isHighlighted
                            ? "bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white shadow-[0_0_20px_rgba(249,115,22,0.3)] hover:shadow-[0_0_30px_rgba(249,115,22,0.5)]"
                            : "bg-brand-red/20 border border-brand-red/50 text-brand-red hover:bg-brand-red hover:text-white"
                        } disabled:opacity-70 disabled:cursor-not-allowed`}
                      >
                        {isLoading && selectedPlan === plan.id ? (
                          <Loader2 size={16} className="animate-spin mx-auto" />
                        ) : (
                          <>
                            <Crown size={14} className="inline-block mr-1" />
                            Premium olish
                          </>
                        )}
                      </button>
                    )}
                  </div>
                );
              })}
            </div>

            <p className="text-center text-gray-500 text-sm mt-8">
              To&apos;lovlar hisobingizdan avtomatik amalga oshiriladi
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
                  className="border border-brand-border rounded-xl overflow-hidden bg-brand-card/30 backdrop-blur-sm transition-colors hover:border-brand-red/30"
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
                ) : (
                  <button
                    onClick={() => handleGetPremium("3month")}
                    disabled={isLoading}
                    className="group inline-flex items-center gap-3 px-10 py-4 bg-gradient-to-r from-brand-red to-orange-600 hover:from-orange-600 hover:to-brand-red text-white font-bold text-lg rounded-xl shadow-[0_0_30px_rgba(249,115,22,0.4)] transition-all duration-300 hover:shadow-[0_0_50px_rgba(249,115,22,0.6)] hover:scale-105 disabled:opacity-70 disabled:cursor-not-allowed"
                  >
                    {isLoading ? (
                      <Loader2 className="w-6 h-6 animate-spin" />
                    ) : (
                      <>
                        <Crown className="w-6 h-6" />
                        Hozir Premium bo'ling
                      </>
                    )}
                  </button>
                )}
              </div>
            </div>
          </div>
        </section>

        {/* Bottom padding */}
        <div className="pb-12" />
      </main>
      <Footer />
    </>
  );
}
