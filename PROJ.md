# FilmoraUz Loyihasi Prezentatsiyasi

FilmoraUz — bu filmlar va seriallarni oqimli (streaming) uzatishga mo'ljallangan, yuqori darajada avtomatlashtirilgan platforma. Tizim murakkab kontentni yig'ish, qayta ishlash va uzatish jarayonlarini o'z ichiga oladi.

## 🏗 Arxitektura

Tizim monorepo arxitekturasida ishlab chiqilgan bo'lib, o'zaro bog'liq bir nechta mustaqil mikroservislardan iborat:

1.  **Backend (Go - Gin):** Asosiy API markazi. Foydalanuvchi boshqaruvi, ma'lumotlar bazasi (MongoDB) va tizim operatsiyalarini boshqaradi.
2.  **Frontend (Next.js - React):** Zamonaviy va tezkor foydalanuvchi interfeysi (UI).
3.  **Parser (Python):** Turli manbalardan filmlar va seriallar haqidagi ma'lumotlarni yig'ib beruvchi aqlli skriptlar.
4.  **Worker (Go):** Video fayllarni yuklab olish, transkodlash (FFmpeg) va bulutli xotiraga yuklash quvur liniyasi.
5.  **Bot (Go):** Telegram orqali boshqaruv va xabarnomalar xizmati.

---

## 🛠 Funksional Imkoniyatlar

### 👤 Foydalanuvchi (User) uchun:
- **Streaming:** Filmlar va seriallarni turli sifatlarda (360p - 1080p) ko'rish.
- **Profil Sozlamalari:** Profil rasmini yangilash, shaxsiy uslubni (frame/gradient) tanlash.
- **Premium Imkoniyatlar:** Premium akkauntlarni sotib olish, maxsus belgilar va eksklyuziv kontentga kirish.
- **Kuzatuv Tarixi:** Ko'rilgan videolar tarixi va o'z vaqtida to'xtatilgan joydan davom ettirish.
- **Sevimlilar:** Yoqtirgan filmlarni "Sevimli" ro'yxatiga qo'shish.
- **Reyting:** Filmlar va epizodlarga baho berish.
- **Telegram orqali login:** Xavfsiz va tezkor autentifikatsiya.

### 👑 Admin uchun:
- **Kontentni Avtomatik Yig'ish (Ingestion):** Manba havolasini kiritish orqali filmni avtomatik yuklash, poster topish va saytga joylash.
- **Video Ishlov berish:** Suv belgisi (watermark) qo'shish va turli sifatlarda avtomatik render qilish.
- **O'chirish Konveyeri (Async Deletion):** Kaskadli o'chirish (DB, bulut, fayllar) jarayonini progress bar orqali kuzatib borish.
- **Moderatsiya:** Filmlarni tasdiqlash yoki rad etish, foydalanuvchilarni bloklash.
- **Analitika:** Ko'rilishlar soni va foydalanuvchi faolligini kuzatish.
- **Telegram Bot boshqaruvi:** Kontentni Telegram kanallarga avtomatik post qilish.

---

## 🚀 Texnologiyalar va Innovatsiyalar

- **Texnologik stek:** Go, Next.js, Python, MongoDB, Backblaze B2, FFmpeg.
- **Avtomatlashtirilgan o'chirish:** O'chirish jarayoni fon rejimida (background worker) amalga oshiriladi, bu esa tizimni yuklamalardan himoya qiladi.
- **Identifikatsiya tizimi:** Har bir epizod uchun unikal `EpisodeIdentity` tizimi (kross-tizim korruptsiyasini oldini oladi).
- **CI/CD:** GitHub Actions orqali to'liq avtomatlashtirilgan test va deploy tizimi.

## 📊 Nima uchun bu loyiha qiymatli?

1. **To'liq avtomatizatsiya:** Qo'lda yuklash va ishlov berish kerak emas, hamma narsa avtomatlashtirilgan.
2. **Masshtablanuvchanlik:** Har bir xizmat alohida ishlashi mumkin, bu yuklamani oson taqsimlaydi.
3. **Production-ready:** Xatoliklarni kuzatish, asinxron jarayonlar va xavfsizlik choralari professional darajada amalga oshirilgan.
4. **Sifat:** Videolar sifatli transkodlanadi va watermark har bir rezolyutsiya uchun dinamik o'lchamda qo'yiladi.

FilmoraUz — bu shunchaki sayt emas, balki to'liq avtomatlashtirilgan media-ekotizimdir.
