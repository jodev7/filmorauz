# Parser Service — Audit TODO

Audit sanasi: 2026-05-14
Commit: `4b85301`
Status: barcha 6 manba uchun stabil ishlash uchun quyidagilarni tuzatish kerak.

---

## 🔴 KRITIK

### 1. `source_config.py` da ikkita manba yetishmaydi
- **Fayl:** `parser/source_config.py`
- **Muammo:** `SOURCES` dictda faqat `uzmovi`, `freekino`, `asilmedia`, `kinolar`, `manual` bor. **`kinochilar`** va **`uzmedia`** umuman yo'q.
- **Ta'siri:** Server.py ularni dispatcher orqali ishlatadi, lekin `get_source_config("kinochilar")` yoki `get_source_config("uzmedia")` bo'sh dict qaytaradi. URL qurish, default settings, sayt-spetsifik logikalar buzilgan.
- **Fix:** Ikkala manba uchun ham `SOURCES` ga entry qo'shish (`BASE_URL`, `SEARCH_ENDPOINT`, `display_name`, va boshqa boshqa manbalarda mavjud maydonlar).

---

## 🟠 O'RTA

### 2. `requests.Session` thread-safe emas
- **Fayl:** `parser/base_parser.py:119`
- **Muammo:** `ThreadedHTTPServer` har request uchun yangi thread ochadi. Har parser instance bitta `self.session = requests.Session()` ni ulashadi. Konkurrent fetch'larda connection pool race bo'lishi mumkin.
- **Fix variantlari:**
  - `threading.local()` orqali har thread uchun alohida session, **yoki**
  - `self._session_lock = threading.Lock()` qo'shib har session ishlatishni lock ichiga olish, **yoki**
  - Har request uchun yangi session ochish (eng oddiy, lekin keep-alive yo'qoladi).

### 3. UNKNOWN-resolve da Referer almashtirilmaydi
- **Fayl:** `parser/ddownloader_integration.py:1734-1752`
- **Muammo:** Embed resolve qilingandan keyin agar `referer` allaqachon bor bo'lsa, CDN talab qiladigan yangi Referer bilan almashtirilmaydi → CDN 403.
- **Fix:** Resolve muvaffaqiyatli bo'lsa, `cand.get("headers", {}).get("Referer")` ni har doim ustuvor qilib qabul qilish (faqat `if not referer` shartisiz).

### 4. Iframe resolver recursive fetch'da custom headerlar yo'qoladi
- **Fayl:** `parser/media_extractor.py` → `resolve_embed_to_candidates()` (oxirgi qism)
- **Muammo:** Nested iframe fetch qilinganda faqat `Referer` o'tkaziladi. Origin, Cookie, User-Agent variantlari yo'qoladi. Ba'zi CDN'lar Origin + Referer juftligini talab qiladi.
- **Fix:** `_walk` ichida session/cookies va base headerlarni nested chaqiruvlarga to'liq uzatish.

### 5. Progress regex faqat `ddownloader_integration.py` uchun
- **Fayl:** `parser/downloader_service.py:1228-1390` (ffmpeg fallback)
- **Muammo:** `downloader_service.py` da ffmpeg progress hali ham `progress_percent = min(95, int(elapsed * 2))` (taxminiy formula) ishlatadi. Haqiqiy `out_time_ms` / `progress=` ffmpeg pipe'lar o'qilmaydi.
- **Fix:** ffmpeg ni `-progress pipe:1` bilan ishga tushirib, `out_time_ms` va `total_size` ni parse qilish. Yoki `total_duration` ni `ffprobe` orqali olib, `out_time_ms / total_duration * 100` hisoblash.

### 6. Freekino hash2 plain-fallback validation zaif
- **Fayl:** `parser/freekino.py:951-964`
- **Muammo:** Plain base64 fallback faqat `"http" in decoded` tekshiradi. Tasodifiy base64 ichida "http" so'zi false positive berishi mumkin.
- **Fix:** `urlparse(decoded)` orqali `scheme` (`http`/`https`) va `netloc` (host bor) tekshirish. Bittasi yo'q bo'lsa rad etish.

### 7. Uzmovi CDN leniency juda keng
- **Fayl:** `parser/uzmovi.py:1700-1715` atrofida (CDN-trust fallback bloki)
- **Muammo:** `.m3u8/.mpd` extension'i bor URL'larni 403/404 bo'lsa ham qabul qiladi. Sayt-darajadagi IP/Referer blok bo'lsa, downloader keyinroq qotib qoladi.
- **Fix:** 1-2 baytlik ranged GET (`Range: bytes=0-2047`) qilib, `#EXTM3U` yoki `<MPD` borligini tekshirish. Faqat o'sha tasdiqdan o'tgan URL'larni qabul qilish.

---

## 🟡 PAST

### 8. Bare `except:` (KeyboardInterrupt yutiladi)
- **Fayl:** `parser/server.py:25` va `parser/server.py:1396`
- **Fix:** `except:` → `except Exception:` ga o'zgartirish.

### 9. Asilmedia onclick loop — takror iteratsiya
- **Fayl:** `parser/asilmedia.py:1124-1150` (yangi qo'shilgan onclick/data-* qism)
- **Muammo:** Barcha selektorlar bir soup'ga ketma-ket qo'llaniladi, bir tag bir necha bor topiladi. Functional bug emas, lekin sekinroq.
- **Fix:** Bitta `select` chaqiruvi bilan kombinatsiyalangan selektor (`a[href*='.mp4'], a[onclick*='.mp4'], [data-url*='.mp4'], ...`), keyin elem.attrs ichidan birinchi mos manba olish.

### 10. `_extract_best_video_url` silent drops
- **Fayl:** `parser/server.py` → `_extract_best_video_url` (taxminan 1141-qator)
- **Muammo:** `url` yoki `type` yo'q entry'lar logsiz tashlanadi.
- **Fix:** Har drop uchun `logger.debug` (yoki `info`) bilan sabab logga yozish.

---

## 📋 Bajariш tartibi (tavsiya)

1. **№1** — eng oson va eng aniq buzilgan (10-15 daqiqa).
2. **№7** — yuklab olish stabilligi uchun muhim (probe qo'shish).
3. **№3, №4** — CDN 403 muammolari uchun (Referer/headers).
4. **№2** — concurrent ingestion ko'p bo'lganda kerak.
5. **№6** — false positive xavfini kamaytirish.
6. **№5** — UX yaxshilash (progress aniqligi).
7. **№8, №9, №10** — code hygiene.

---

## ✅ Allaqachon to'g'ri ishlaydi (audit'dan)

- Stale cleanup queue startidan oldin ishlaydi.
- Iframe resolver multi-candidate qo'llab-quvvatlash to'g'ri pipeline'da o'tadi.
- `resolve_embed_to_candidates` infinite loopdan himoyalangan (visited set + max_depth=2).
- Kinolar/kinochilar/uzmedia → server `_extract_best_video_url` chain ishlaydi.
