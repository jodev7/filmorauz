"use client";

import { createContext, useContext, ReactNode } from "react";

// Uzbek-only translations
const translations: Record<string, string> = {
  "common.home": "Bosh sahifa",
  "common.movies": "Kinolar",
  "common.search": "Qidirish",
  "common.searchPlaceholder": "Kinolarni qidiring...",
  "common.loading": "Yuklanmoqda...",
  "common.noMovies": "Kinolar topilmadi",
  "common.watchNow": "Ko'rish",
  "common.browseMovies": "Kinolarni ko'rish",
  "common.seeAll": "Hammasini ko'rish",
  "common.back": "Orqaga",
  "common.backToMovies": "Kinolarga qaytish",
  "common.action": "Jangovar",
  "common.drama": "Drama",
  "common.comedy": "Komediya",
  "home.heroTitle": "FILMORAUZ",
  "home.heroSubtitle": "Eng yaxshi filmlar, bitta joyda.",
  "home.featuredMovies": "TANLANGAN",
  "home.newReleases": "YANGI FILMLAR",
  "home.collections": "KOLLEKTSIYALAR",
  "home.trending": "TRENDING",
  "home.continueWatching": "KO'RISHNI DAVOM ETTIRISH",
  "home.addMovies": "Admin panelida kinolar qo'shing.",
  "home.moreInfo": "Batafsil",
  "movies.title": "BARCHA FILMLAR",
  "movies.clearSearch": "← Qidiruvni tozalash",
  "movies.noResults": "Filmlar topilmadi.",
  "movies.noResultsHint": "Boshqa qidiruv so'zini sinab ko'ring.",
  "movies.found": "ta film topildi",
  "movies.page": "Sahifa",
  "movies.of": "/",
  "movies.previous": "Oldingi",
  "movies.next": "Keyingi",
  "movies.searchPrefix": "Qidiruv:",
  "movie.addToFavorites": "Sevimlilarga qo'shish",
  "movie.inFavorites": "Sevimlilarda",
  "movie.removeFromFavorites": "Sevimlilardan o'chirish",
  "watch.title": "Ko'rish",
  "watch.description": "onlayn tomosha qiling",
  "watch.defaultTitle": "Tomosha",
  "user.favorites": "Sevimlilar",
  "user.noWatchHistory": "Hali ko'rilgan kinolar yo'q",
  "user.noFavorites": "Hali sevimli kinolar yo'q",
  "user.watchHistory": "Ko'rish tarixi",
  "user.justNow": "hozir",
  "user.minutesAgo": "minut avval",
  "user.hoursAgo": "soat avval",
  "user.daysAgo": "kun avval",
  "user.monthsAgo": "oy avval",
  "user.yearsAgo": "yil avval",
  "movie.year": "Yil",
  "movie.duration": "Davomiyligi",
  "movie.country": "Mamlakat",
  "movie.quality": "Sifat",
  "movie.genre": "Janr",
  "movie.description": "Tavsif",
  "movie.min": "daq",
  "movie.clickToPlay": "O'ynash uchun bosing",
  "admin.dashboard": "Boshqaruv paneli",
  "admin.movies": "Kinolar",
  "admin.addMovie": "Kino qo'shish",
  "admin.editMovie": "Kino tahrirlash",
  "admin.deleteMovie": "O'chirish",
  "admin.totalMovies": "Jami kinolar",
  "admin.thisMonth": "Bu oyda",
  "admin.quickAction": "Tezkor harakat",
  "admin.addNewMovie": "Yangi kino qo'shish",
  "admin.recentMovies": "So'nggi filmlar",
  "admin.viewAll": "Hammasini ko'rish",
  "admin.noMoviesYet": "Hali kinolar yo'q.",
  "admin.addFirstMovie": "Birinchi kinoni qo'shing",
  "admin.welcomeBack": "Xush kelibsiz. Bu yerda nima bo'layotgani.",
  "admin.panel": "Admin panel",
  "admin.viewSite": "Saytni ko'rish",
  "admin.searchMovies": "Kinolarni qidiring...",
  "admin.noMoviesMatch": "Qidiruvga mos kinolar yo'q.",
  "admin.addFirstMovieArrow": "Birinchi kinoni qo'shing →",
  "admin.totalMoviesCount": "ta kino",
  "admin.loadingMovies": "Kinolar yuklanmoqda...",
  "admin.confirmDelete": "Haqiqatan ham o'chirmoqchimisiz",
  "admin.cannotUndo": "Bu amalni bekor qilib bo'lmaydi.",
  "admin.deleteFailed": "O'chirishda xato",
  "auth.login": "Kirish",
  "auth.loginRequired": "Kirish talab qilinadi",
  "auth.logout": "Chiqish",
  "auth.username": "Foydalanuvchi nomi",
  "auth.password": "Parol",
  "auth.loginButton": "Kirish",
  "auth.signingIn": "Kirilmoqda...",
  "auth.adminLogin": "Admin kirish",
  "auth.signInDesc": "Kinolar va kontentni boshqarish uchun kiring.",
  "auth.email": "Email",
  "form.title": "Nom",
  "form.year": "Yil",
  "form.description": "Tavsif",
  "form.posterUrl": "Poster URL",
  "form.backdropUrl": "Orqa fon URL",
  "form.videoUrl": "Video URL",
  "form.videoUrlHint": "Qo'llab-quvvatlaydi: to'g'ridan-to'g'ri .mp4 havolalar, YouTube URL, iframe embed URL",
  "form.duration": "Davomiyligi (daqiqa)",
  "form.country": "Mamlakat",
  "form.genre": "Janrlar",
  "form.customGenre": "O'z janringiz...",
  "form.quality": "Sifat",
  "form.selectQuality": "Sifatni tanlang",
  "form.save": "Saqlash",
  "form.cancel": "Bekor qilish",
  "form.required": "Majburiy",
  "form.saving": "Saqlanmoqda...",
  "form.saveMovie": "Kinoni saqlash",
  "form.createMovie": "Kino yaratish",
  "form.updateMovie": "Kinoni yangilash",
  "form.addNewMovie": "Yangi kino qo'shish",
  "form.fillDetails": "Yangi kino qo'shish uchun quyidagi ma'lumotlarni to'ldiring.",
  "form.backToMovies": "Kinolarga qaytish",
  "form.editMovie": "Kinoni tahrirlash",
  "form.movieTitle": "Kino nomi",
  "form.movieDescription": "Kino tavsifi...",
  "form.countryPlaceholder": "masalan, AQSh",
  "form.durationPlaceholder": "120",
  "form.posterPreview": "Poster ko'rinishi",
  "form.backdropPreview": "Orqa fon ko'rinishi",
  "errors.notFound": "Sahifa topilmadi",
  "errors.goHome": "Bosh sahifaga qaytish",
  "errors.pageNotExist": "Bu sahifa mavjud emas yoki o'chirilgan.",
  "footer.tagline": "Eng yaxshi filmlarni onlayn tomosha qiling. Yangi va klassik filmlar, bitta joyda.",
  "footer.browse": "Ko'rish",
  "footer.allMovies": "Barcha filmlar",
  "footer.admin": "Admin",
  "footer.login": "Kirish",
  "footer.dashboard": "Panel",
  "footer.rights": "Barcha huquqlar himoyalangan.",
  "table.movie": "Kino",
  "table.genre": "Janr",
  "table.year": "Yil",
  "table.quality": "Sifat",
  "table.actions": "Amallar",
  "table.code": "Kod",
};

interface I18nContextType {
  t: (key: string, params?: Record<string, string | number>) => string;
}

const I18nContext = createContext<I18nContextType | undefined>(undefined);

export function I18nProvider({ children }: { children: ReactNode }) {
  const t = (key: string, params?: Record<string, string | number>): string => {
    let text = translations[key] || key;
    if (params) {
      Object.entries(params).forEach(([k, v]) => {
        text = text.replace(`{${k}}`, String(v));
      });
    }
    return text;
  };

  return (
    <I18nContext.Provider value={{ t }}>
      {children}
    </I18nContext.Provider>
  );
}

export function useI18n() {
  const context = useContext(I18nContext);
  if (!context) {
    // Provide a fallback for when context is not available
    return {
      t: (key: string) => translations[key] || key,
    };
  }
  return context;
}

// Export DEFAULT_LOCALE for backward compatibility
export const DEFAULT_LOCALE = "uz" as const;
export type Locale = "uz";
export const LOCALES: Locale[] = ["uz"];
export const LOCALE_LABELS: Record<Locale, string> = { uz: "O'zbek" };
export const LOCALE_FLAGS: Record<Locale, string> = { uz: "🇺🇿" };
