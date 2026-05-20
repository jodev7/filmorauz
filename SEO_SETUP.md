# SEO Auto-Indexing — O'rnatish Qo'llanmasi

Bu hujjat 251 ta sahifa Search Console'da 1 oydan beri pending bo'lib turgan muammoni hal qiluvchi avtomatik indekslash tizimini qanday yoqishni ko'rsatadi.

---

## Nima ishlatildi (qisqacha)

1. **Backend servislari** (`backend/services/seo/`):
   - `IndexNowService` — Bing, Yandex, Seznam, Naver, Yep'ni bitta API call bilan xabar qiladi.
   - `GoogleIndexingService` — Google Indexing API'ga `URL_UPDATED` / `URL_DELETED` yuboradi.
   - `SearchConsoleService` — Google Search Console'ga sitemap'ni qayta yuboradi.
   - `Notifier` — yuqoridagilarni birlashtiradi, har bir hodisani `seo_events` MongoDB collection'iga log qiladi.

2. **Sitemap qayta qurildi** — endi backend index + bo'laklarga bo'lib beradi:
   - `/sitemap.xml` — index
   - `/sitemap-static.xml`, `/sitemap-genres.xml`, `/sitemap-movies.xml`
   - `/sitemap-series.xml`, `/sitemap-episodes.xml`
   - `/sitemap-videos.xml` — **Google Video Search uchun video sitemap** (kino sayti uchun juda muhim)
   - `/robots.txt` — backenddan
   - Frontend'dagi `sitemap.ts` va `robots.ts` o'chirildi, Next.js rewrite orqali backend'ga proxy qiladi.

3. **Hooks** — film/serial tasdiqlanganda yoki yangilanganda avtomatik ping ketadi.

4. **Periodic resubmit** — har 6 soatda backend `/sitemap.xml`ni Search Console'ga qayta yuboradi.

5. **JSON-LD kuchaytirildi**:
   - Root layout'da `Organization` + `WebSite` + `SearchAction` (sitelinks search box uchun).
   - Movie detail page'da `VideoObject` qo'shildi (Google Video Search uchun).

6. **Admin SEO dashboard** — `/admin/seo` sahifasida:
   - Provider holati ko'rinadi
   - "Hamma kontentni qayta yuborish" tugmasi
   - "Sitemap'ni qayta yuborish" tugmasi
   - Aniq URL'larni qo'lda ping qilish
   - So'nggi hodisalar log'i (har provider uchun success/error)

---

## Step-by-Step O'rnatish

### 1-QADAM: Backend `.env` fayliga env'larni qo'shish

`backend/.env` faylini oching va quyidagilarni qo'shing:

```bash
SEO_NOTIFY_ENABLED=true

# IndexNow (Bing + Yandex + Seznam + Naver + Yep — bitta call hammasiga ketadi)
INDEXNOW_KEY=90f51ebd9b70409933617a8510d15a6908aec7c1ccfbb6070922e67fd4a8bcfb

# Google: Indexing API + Search Console ikkalasi shu kalitdan foydalanadi
GOOGLE_INDEXING_CREDENTIALS_PATH=./secrets/google-indexing.json
GOOGLE_SEARCH_CONSOLE_SITE_URL=https://filmorauz.net/
```

> **Eslatma:** IndexNow key fayli (`90f51ebd…bcfb.txt`) avtomatik tarzda `frontend/public/` ichida yaratilgan. Hech narsa qilish shart emas — frontend deploy bo'lganda u o'z-o'zidan `https://filmorauz.net/90f51ebd….txt` orqali ochiladi.

---

### 2-QADAM: Google Cloud'da Service Account yaratish

Bu eng muhim qadam. Google Indexing API va Search Console API uchun bitta service account ishlatamiz.

**2.1. Loyiha yaratish (yoki tanlash):**

1. https://console.cloud.google.com/ saytiga kiring.
2. Yuqoridagi loyiha tanlash menyusidan yangi loyiha yarating yoki mavjudini tanlang.
3. Loyiha nomi: `filmorauz` (yoki istalgan).

**2.2. API'larni yoqish:**

1. Chap menyudan → **APIs & Services** → **Library**.
2. Qidiruv: "**Indexing API**" → ustiga bosing → **Enable**.
3. Yana qidiruv: "**Google Search Console API**" → **Enable**.

**2.3. Service Account yaratish:**

1. **IAM & Admin** → **Service Accounts** → **Create Service Account**.
2. Nomi: `filmorauz-seo`.
3. Tavsif: `SEO auto-indexing` (ixtiyoriy).
4. **Create and Continue** bosing.
5. **Grant access** qadamida hech narsa qo'shmang — to'g'ridan-to'g'ri **Done** bosing.

**2.4. JSON kalit yaratish:**

1. Yaratilgan service account ustiga bosing.
2. Yuqoridagi tablardan **Keys** tabini tanlang.
3. **Add Key** → **Create new key** → format: **JSON** → **Create**.
4. JSON fayl avtomatik yuklanadi (masalan `filmorauz-xxxxx.json`).

**2.5. JSON faylni backend'ga qo'yish:**

```bash
mkdir -p /home/jodev/Desktop/filmorauz/backend/secrets
mv ~/Downloads/filmorauz-xxxxx.json /home/jodev/Desktop/filmorauz/backend/secrets/google-indexing.json
chmod 600 /home/jodev/Desktop/filmorauz/backend/secrets/google-indexing.json
```

**2.6. Service Account email'ini nusxalash:**

JSON ichida `"client_email": "filmorauz-seo@xxxxx.iam.gserviceaccount.com"` ko'rinishda email bor — uni nusxalang. Keyingi qadamda kerak bo'ladi.

---

### 3-QADAM: Search Console'ga Service Account'ni Owner sifatida qo'shish

Bu qadam **majburiy** — Restricted yoki Full role yetarli emas, **faqat Owner** ishlaydi.

1. https://search.google.com/search-console saytiga kiring.
2. `filmorauz.net` property'sini tanlang (yoki yangi qo'shing).
3. Chap pastdagi ⚙️ **Settings** → **Users and permissions**.
4. **Add user** bosing.
5. **Email address** maydoniga 2.6-qadamdagi service account email'ini yopishtiring (`filmorauz-seo@…iam.gserviceaccount.com`).
6. **Permission** — **Owner** ni tanlang (Full emas, Owner!).
7. **Add** bosing.

---

### 4-QADAM: Backend'ni qayta ishga tushirish

```bash
cd /home/jodev/Desktop/filmorauz/backend
go build ./...
# Agar PROD'da bo'lsa:
make backend
# yoki to'g'ridan-to'g'ri:
go run main.go
```

Backend log'larida quyidagi xabarni ko'rishingiz kerak:

```
[SEO] notifier enabled — status={Enabled:true IndexNowConfigured:true GoogleConfigured:true SearchConsoleReady:true …}
```

Agar `IndexNowConfigured:false` yoki `GoogleConfigured:false` ko'rinsa — `.env` da xato yoki JSON fayl topilmagan. Log'lardagi xato xabarini o'qing.

---

### 5-QADAM: Admin SEO dashboard'da tekshirish

1. Brauzerda admin sifatida login qiling.
2. https://filmorauz.net/admin/seo (yoki localhost'da http://localhost:3000/admin/seo) ga o'ting.
3. To'rt karta'ning hammasi **"Sozlangan"** ko'k holatda bo'lishi kerak:
   - Notifier
   - IndexNow (Bing/Yandex)
   - Google Indexing API
   - Search Console

---

### 6-QADAM: Birinchi bulk re-ping

251 ta pending sahifani hal qilish uchun:

1. Admin SEO sahifasida **"Hamma kontentni qayta yuborish"** tugmasini bosing.
2. Pastdagi event log'da har provider uchun yashil ✓ "ok" status ko'rishingiz kerak.
3. Agar Google Indexing API'da kunlik 200 ta limitdan oshib ketsa, qolganlari ertaga avtomatik yuboriladi.

---

### 7-QADAM: Sitemap'larni tekshirish

Brauzerda quyidagilarni oching:

- https://filmorauz.net/sitemap.xml — index ko'rinishida bo'lishi kerak (sitemapindex)
- https://filmorauz.net/sitemap-movies.xml — barcha filmlar
- https://filmorauz.net/sitemap-videos.xml — `<video:video>` bloklari bilan
- https://filmorauz.net/robots.txt — backend chiqargan robots

Search Console'da:
1. Sitemaps → "Add a new sitemap" → `sitemap.xml` ni qo'shing.
2. Yana qo'shimcha: `sitemap-videos.xml` ni alohida ham qo'shing (Google Video Search uchun).

---

### 8-QADAM: Yandex va Bing Webmaster

**Yandex.Webmaster:**

1. https://webmaster.yandex.com/ → "Add site" → `https://filmorauz.net`.
2. Verifikatsiya usuli: HTML fayl.
3. Yuklab olingan `yandex_xxxxxx.html` faylini `frontend/public/` ichiga qo'ying.
4. Frontend deploy bo'lgandan keyin Yandex'da **Check** bosing.
5. Sitemap qo'shing: Indexing → Sitemap files → `https://filmorauz.net/sitemap.xml`.

**Bing Webmaster Tools:**

Eng oson usul — Search Console'dan import qilish:

1. https://www.bing.com/webmasters saytiga kiring.
2. **Import from Google Search Console** tugmasini bosing → Google account'ingiz bilan ulaning → property'lar avtomatik ko'chiriladi.

Yoki qo'lda:
1. Add Site → verifikatsiya XML faylini yuklab oling.
2. `frontend/public/BingSiteAuth.xml` ga qo'ying.
3. Verifikatsiya tugagach, sitemap'ni avtomatik aniqlaydi.

> **Eslatma:** IndexNow allaqachon Bing va Yandex'ni xabar qilib turadi, lekin Webmaster Tools'ga qo'shish sizga statistika va xato xabarlarini ko'rish imkonini beradi.

---

## Tekshiruv (testing)

```bash
# Admin JWT'ni olish: brauzer DevTools → Application → Cookies → auth_token

# 1. Status
curl -H "Authorization: Bearer <admin-jwt>" https://filmorauz.net/api/admin/seo/status

# 2. Aniq URL'ni qayta yuborish
curl -X POST -H "Authorization: Bearer <admin-jwt>" -H "Content-Type: application/json" \
  -d '{"urls":["/movies/some-slug"]}' \
  https://filmorauz.net/api/admin/seo/reindex

# 3. Hammasini qayta yuborish
curl -X POST -H "Authorization: Bearer <admin-jwt>" \
  https://filmorauz.net/api/admin/seo/reindex/all

# 4. Sitemap'larni tekshirish
curl https://filmorauz.net/sitemap.xml
curl https://filmorauz.net/sitemap-videos.xml | head -50
curl https://filmorauz.net/robots.txt
```

---

## Nima kutish mumkin

- **IndexNow** — daqiqalar ichida. Bing odatda 5-30 daqiqada crawl qiladi, Yandex bir necha soatda.
- **Google Indexing API** — rasman faqat `JobPosting` va `BroadcastEvent` uchun, lekin biz kino URL'lari uchun "best-effort signal" sifatida ishlatamiz. Agar Google call'ni rad etsa, dashboard'da xato ko'rinadi, lekin **Search Console sitemap resubmit** hali ham Google'ni xabardor qilib turadi.
- **Search Console sitemap resubmit** — eng ishonchli universal yo'l. Googlebot sitemap'ni qayta crawl qiladi va yangi URL'lar bir necha kun ichida indeksga tushadi.
- **VideoObject schema + Video sitemap** — kino sayti uchun **eng katta SEO yutuq**. Google Video Search'da ko'rinish degani — organik trafikning yangi katta manbasi.

---

## Quotalar (cheklovlar)

| Provider | Kunlik limit | Izoh |
|---|---|---|
| IndexNow | Cheksiz (10,000 URL/request) | Hech qanday ro'yxatdan o'tish kerak emas |
| Google Indexing API | 200 request/kun | Quotas sahifasida oshirish mumkin |
| Search Console sitemap | Cheksiz | Idempotent — har publish'da call qilsa ham OK |

`seo_events` collection 30 kundan eski yozuvlarni avtomatik tozalaydi.

---

## Muammolarni hal qilish (troubleshooting)

**`[SEO] notifier disabled` log'da ko'rinmoqda:**
- `SEO_NOTIFY_ENABLED=true` ekanligini tekshiring.
- `BASE_SITE_URL` to'g'ri ekanligini tekshiring.

**`Google Indexing init failed`:**
- JSON fayl yo'lini tekshiring (`GOOGLE_INDEXING_CREDENTIALS_PATH`).
- JSON fayl haqiqatdan ham service account kalitimi tekshiring (`"type": "service_account"` bo'lishi kerak).

**`search console: status=403`:**
- Service account email'i Search Console'da **Owner** sifatida qo'shilmagan.
- 3-qadamni qaytadan bajaring.

**`google indexing: status=403 PERMISSION_DENIED`:**
- Indexing API yoqilmagan (2.2-qadam).
- Service account Owner emas.

**IndexNow dashboard'da error chiqmoqda:**
- `frontend/public/<KEY>.txt` faylining sayt orqali ochilayotganini tekshiring:
  ```bash
  curl https://filmorauz.net/90f51ebd9b70409933617a8510d15a6908aec7c1ccfbb6070922e67fd4a8bcfb.txt
  ```
  Bu kalit qiymatining o'zini qaytarishi kerak.

**Search Console'da hali ham "pending" ko'rinmoqda:**
- Bu normal — Google indeksga olish bir necha kun davom etishi mumkin.
- "URL Inspection" tool'da bitta URL'ni tekshirib ko'ring → "Request Indexing" tugmasini bosing.
- Sabr qiling: bizning pipeline endi har yangi kontent uchun avtomatik ping qiladi, eski pending'lar 1-2 hafta ichida tozalanishi kerak.

---

## Keyingi qadamlar (ixtiyoriy, lekin tavsiya)

1. **Og-image** — `frontend/public/og-image.jpg` (1200×630) yarating. Hozir bu fayl yo'q, demak social share'larda OG image 404 bo'ladi.
2. **Yandex sitemaps panel'iga `/sitemap-videos.xml`** ni alohida qo'shing — Yandex.Video uchun.
3. **Core Web Vitals** ni monitor qiling: https://pagespeed.web.dev/?url=https://filmorauz.net — LCP, CLS, FID ko'rsatkichlari ranking factor.
4. **Internal linking** ni kuchaytiring — har film page'da "shu janrdagi boshqa kinolar", "shu yildagi kinolar" linklari.
