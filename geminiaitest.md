# Gemini AI Clip Generation — Test & Ish Jurnali

> Maqsad: clip generatsiyaning Gemini "AI viral" yo'lini Interstellar kinosida
> lokal sinab, ishonchli va arzon ishlashiga erishish.

---

## Kontekst — qanday ishlaydi

**Fayl:** `worker/pipeline/clip_generator.go` → `generateClipsForTarget()`
**Parser:** `parser/server.py` → `gemini_analyze_clips()` + `/clip/gemini-analyze` endpoint

Oqim ikki qismdan iborat:
1. **Lahza tanlash** — qaysi qismlarni clip qilish.
2. **Render** — o'sha qismlarni ffmpeg bilan vertikal (9:16, 1080×1920) Reels
   formatida kesib, logo + "Kino kodi" + CTA + subtitr bilan bezash.

Lahza tanlashning ikki yo'li bor:
- **Gemini AI yo'li** (`CLIP_AI_VIRAL=1`) — film **audiosi** Gemini'ga yuboriladi,
  u eng viral lahzalarni + caption + hashtag + transkript qaytaradi.
- **Heuristik yo'l** (zaxira) — lokal Whisper + yuz aniqlash + audio/sahna ballash.
  Gemini ishlamasa avtomatik shunga o'tadi.

> **Audio = miya** (qayerni kesish va nima deyilganini audiodan tushunadi),
> **video = tana** (ffmpeg o'sha oraliqni clip qilib kesadi).

Clip soni: **15 ta** (`clipCount = 15`), har biri 30–90s.

---

## Bugun (2026-05-25) qilingan ishlar

Interstellar (`parser/downloads/Interstellar.mp4`, 2s49d) ustida debug qilib,
**3 ketma-ket muammoni** topib hal qildik:

| # | Muammo | Asl sabab | Yechim |
|---|--------|-----------|--------|
| 1 | `JSONDecodeError: Expecting ',' delimiter` | thinking budjeti JSON javobni o'rtasidan uzgan | finish_reason tekshiruvi + token diagnostikasi qo'shildi |
| 2 | `finish_reason=MAX_TOKENS`, 3 clip = 162k belgi | bitta javobda har clip uchun to'liq transkript → token bombasi (candidates=65521) | **tanlash va transkript ajratildi** (ikki bosqich) |
| 3 | 61 ta 1-soniyali "clip", caption "Koper men keldim" takror | `thinking_budget=0` modelni "miyasiz" qilgan | tanlashga **thinking qaytarildi** (`thinking_budget=8192`) |

### Yakuniy dizayn (kod yozildi, lokal tasdiqlandi)

`parser/server.py` ichida `gemini_analyze_clips()` qayta yozildi:

- **1-bosqich — tanlash:** to'liq audio → Gemini (thinking ON, `max_output_tokens=24576`)
  faqat clip oynalarini tanlaydi: `start_sec`, `end_sec`, `reason`, `caption`,
  `hashtags`. **Subtitr yo'q** (javob kichik). Prompt kuchaytirilgan
  (AYNAN N ta, har biri ≥25s, 1-soniyali bo'lak BERMA, turli joylardan).
- **Klip filtri:** <15s kliplar tashlanadi, `max_clips` gacha cheklanadi.
- **2-bosqich — partiyali transkript (BITTA chaqiruv):** o'sha to'liq audio +
  tanlangan oynalar ro'yxati → har clip uchun transkript, **mutlaq film soniyasida**.
  thinking OFF. → **Har kino = jami 2 Gemini so'rovi** (clip soniga bog'liq emas).
- **`_clean_subtitles()`:** bo'sh va prompt-echo qatorlarni tashlaydi, vaqtlarni
  oynaga clamp qiladi (`absolute=True/False` rejimi). Lokal unit-test bilan tekshirilgan.

Yangi yordamchilar: `_gemini_extract_audio`, `_gemini_upload_audio`,
`_gemini_generate_json(... thinking_budget=)`, `_clean_subtitles`.

**Worker tomoni O'ZGARMAYDI** — javob shakli aynan o'sha:
`{clips:[{start_sec,end_sec,reason,caption,hashtags,subtitles:[{t,text}]}], usage, cost_usd, model}`.

### O'zgargan fayl
- `parser/server.py` (`gemini_analyze_clips` va atrofidagi yordamchilar)
- `worker/pipeline/clip_interstellar_manual_test.go` — manual test harness (kechagi)

> **Hali commit qilinmagan.**

---

> **YANGILANISH (2026-05-26):** quyidagi BLOK hal bo'ldi va pipeline qayta
> qurildi. To'liq bugungi ish jurnali fayl oxirida — "## 2026-05-26 — qilingan
> ishlar" bo'limiga qarang.

## Holat (2026-05-25): BLOK — Gemini bepul kvotasi tugagan edi

```
429 RESOURCE_EXHAUSTED — limit: 20, model: gemini-2.5-flash
quotaId: GenerateRequestsPerDayPerProjectPerModel-FreeTier
```

Bepul tarif: **kuniga 20 so'rov**. Bugungi iteratsiyalar tugatdi.
Shuning uchun **thinking-on tanlash SIFATINI hali haqiqiy ko'rmadik**.

### Kvota qachon tiklanadi
- Yarim tun **Tinch okeani vaqti (PDT, UTC−7)** = **26-may, soat 12:00 Toshkent vaqti**.
- Ya'ni ertaga **kunduzi 12:00 dan keyin** sinash mumkin.

---

## ERTAGA (2026-05-26, 12:00+ Toshkent) qilinadigan ishlar

1. **Parser ishlayotganini tekshir** (yangi kod bilan):
   ```bash
   curl -s http://127.0.0.1:8082/health
   # kerak bo'lsa: cd parser && ./venv/bin/python server.py &
   ```
2. **Gemini endpointni yakka sina** (max_clips=3, tez, 2 so'rov):
   ```bash
   curl -s -m 1800 -X POST http://127.0.0.1:8082/clip/gemini-analyze \
     -H "Content-Type: application/json" \
     -d '{"video_path":"/home/jodev/Desktop/filmorauz/parser/downloads/Interstellar.mp4","max_clips":3,"subtitles":true}' \
     -o /tmp/gemini_resp.json -w "HTTP %{http_code}\n"
   ```
3. **Natija sifatini tekshir** — kutilgan:
   - AYNAN 3 ta clip, har biri **25–55s** (1-soniyali bo'lak EMAS)
   - caption mazmunli (dialog satri emas, jozibali sarlavha)
   - subtitr toza (bo'sh/echo yo'q), `t` mutlaq film soniyasida va oyna ichida
   - `cost_usd` past, `usage` mantiqiy
4. **To'liq oqimni sina** (ffmpeg render + subtitr burn bilan):
   ```bash
   cd worker && RUN_INTERSTELLAR=1 go test ./pipeline/ \
     -run TestGenerateInterstellarClips -v -timeout 60m
   # natija: worker/uploads/movies/clips/interstellar/*.mp4
   ```
   - Hosil bo'lgan clip MP4 larni ko'r: vertikal, logo, "Kino kodi: 9999",
     CTA, subtitr to'g'ri kuyganmi.
5. **Sifat yaxshi bo'lsa → commit qil** (push QILMA — foydalanuvchi so'ragan).

### Keyingi qaror (sifat tasdiqlangach)
- **Xarajat optimizatsiyasi:** 2-chaqiruvda to'liq film o'rniga faqat tanlangan
  klip oynalari audiosini birlashtirib yuborish → ~36% arzon.
- Yoki `gemini-2.5-flash-lite` modeliga o'tish (yana arzon, sifat biroz pasayishi mumkin).
- Production uchun **billing yoqilgan Gemini kalit shart** (bepul 20/kun yetmaydi).

---

## Xarajat hisobi (joriy dizayn)

Asos: audio ~32 token/s; 2 chaqiruv, to'liq audio 2×; audio kirish $1/1M, chiqish $2.5/1M.

| Birlik | Narx |
|---|---|
| Kino (2 soat) | **$0.49** |
| Episod (1 soat) | **$0.26** |

**Oylik (50 kino + 10 serial × ~95 episod = 950 episod):**

| | Summa |
|---|---|
| Kinolar (50) | $24.67 |
| Episodlar (950) | $249.77 |
| **JAMI/oy** | **≈ $274** |
| JAMI/yil | ≈ $3,293 |

**Optimizatsiya bilan (klip-audio birlashtirish):** Kino $0.29 / Episod $0.17 →
**≈ $175/oy** (~36% arzon).

> Xarajatning ~85% — audio kirish; ~90% — episodlardan (soni ko'p).

---

## Foydali eslatmalar
- Narx env orqali sozlanadi: `GEMINI_PRICE_AUDIO_IN`, `GEMINI_PRICE_TEXT_IN`, `GEMINI_PRICE_OUT`.
- Model env: `GEMINI_MODEL` (default `gemini-2.5-flash`).
- `GEMINI_API_KEY` — `parser/.env` da.
- Subtitr yoqish: so'rovda `"subtitles": true` (default), worker env'iga bog'liq emas.
- Parser log: `/tmp/parser_interstellar.log` (queue `claim error` warninglari backend
  o'chiqligidan — clip testiga aloqasi yo'q, e'tibor berma).

---

## 2026-05-26 — qilingan ishlar

Kvota tiklangach to'liq sinab, audio-asosli yo'lning sifat muammosini topdik va
pipeline'ni **transcript-anchored gibrid**ga qayta qurdik. Hammasi kommit
qilingan (push QILINMAGAN), `fix/parser-quality-and-serial` branch.

### Topilgan muammolar va yechimlar (ketma-ket)

| # | Muammo | Yechim | Kommit |
|---|--------|--------|--------|
| 1 | Stage 1 `MAX_TOKENS` — thinking (8k) + javob (16k) 24k shiftga urilib JSON uzilardi | output shifti 24576 → **65536** | `ca1861a` |
| 2 | Stage 2 daqiqalik kvota (250k/min) — to'liq film audiosi 2 marta yuborilardi | faqat tanlangan klip-oyna audiolari + 429 backoff retry | `ca1861a` |
| 3 | Audio yo'li timestamp'ni o'ylab topardi (kliplar noto'g'ri sahnaga tushardi, beqaror 0.4s–50s) | **Whisper timestamp anchor + Gemini transcript'dan tanlaydi** (matn, audio emas) | `b6f4823` |
| 4 | Whisper `small` o'zbekcha ASR qo'pol ("Menu moxandis man") | subtitr matnini **Gemini** tanlangan oynalarni qayta transkript qilib beradi (umumiy helper) | `e04ced3` |
| 5 | To'liq filmda worker→parser 40 daq timeout → heuristik'ga tushardi | `CLIP_GEMINI_TIMEOUT_MIN` (default **120 daq**) | `bf19e73` |
| 6 | Transcript chaqiruvida vaqtinchalik **503** → yomon audio yo'liga fallback | `_gemini_generate_json_retrying` (429/503/UNAVAILABLE backoff) | `bf19e73` |
| 7 | Jim/vizual sahnalar (mas. yig'lash) tanlanmasdi — tanlov faqat dialogga tayanardi | transkriptga **jim-oraliq belgilari** + **yuz-yaqinplan oynalari** (face detection) signal sifatida | `b5800c8` |
| — | Manual harness DB nil bo'lsa testni yiqitardi | nil-DB guard (kliplar diskda qoladi) | `bf19e73` |

### Yakuniy oqim (transcript-anchored gibrid)
1. **Whisper** (faster-whisper `small`, lokal) to'liq filmni transkript qiladi → aniq timestamp.
2. **Face highlights** — ffmpeg past fps kadr + opencv yuz aniqlash → yaqin planli hissiy oynalar.
3. **Gemini selection** (1 matnli chaqiruv) — transkript + jim-oraliq belgilari + yuz-oynalaridan ~15 viral klip tanlaydi (timestamp'ni *berilganidan* oladi).
4. **Gemini window transcription** (1 chaqiruv) — tanlangan oynalar audiosini toza o'zbekcha subtitrga aylantiradi.
5. **Worker ffmpeg** — vertikal 1080×1920 MP4 + logo + "Kino kodi" + CTA + burnt-in subtitr.
   `CLIP_GEMINI_TRANSCRIPT=0` → eski audio yo'li (fallback). `CLIP_FACE_HIGHLIGHTS=0` → yuz signalsiz.

### Tasdiqlangan natijalar (Interstellar)
- **Cheklangan 20 daq (transcript):** HTTP 200, 1 Gemini chaqiruv, timestamp ↔ kontent mos (665s = "muhandislar kerak emas"), $0.018. **~19× arzon** (audio yo'li $0.34).
- **+Gemini subtitr:** "Men muhandisman", "Dunyoga esa fermerlar kerak" — toza o'zbekcha. $0.032.
- **+Yuz/jim signal (cheklangan 20 daq):** tanlov klasterlanmasdan taqsimlandi (168/221/665/992s), ota-bola hissiy sahnasi tanlandi, 8 yuz-highlight. $0.031.
- **To'liq film render (PASS):** 15 ta vertikal MP4 + burnt-in subtitr → `worker/uploads/movies/clips/interstellar/`. LEKIN bu run vaqtinchalik 503 tufayli audio fallback'da edi (klasterlangan kliplar) — #6 tuzatilgandan OLDIN. Shuning uchun to'liq sifatli runни ertaga qayta qilamiz.

### Ochiq savol — mashhur "Kuper qizining videolarini ko'rib yig'lash" sahnasi
- ~1s50d (~6600s). Cheklangan testlarda yetib bo'lmaydi. To'liq film + #7 (yuz/jim signal) bilan tanlanish ehtimoli oshadi, lekin **kafolatlanmagan** — ertangi to'liq run'da tekshiramiz (parser logida `transcript path selected ... windows=` qatoriga qarab).

---

## ERTAGA (2026-05-27) — TO'LIQ RUN

Bugun kunlik Gemini kvotasiga yaqinlashdik (20/kun bepul), shuning uchun to'liq
run ertaga — kvota tiklangach (tush, 12:00 Toshkent'dan keyin).

1. **Parserni cheklovsiz ishga tushir** (to'liq film, `AI_AUDIO_LIMIT_SEC`siz):
   ```bash
   cd parser && ./venv/bin/python server.py &
   curl -s http://127.0.0.1:8082/health
   ```
2. **To'liq worker render** (timeout oshirilgan, ~2-2.5 soat: Whisper + face + render):
   ```bash
   cd worker && CLIP_GEMINI_TIMEOUT_MIN=240 RUN_INTERSTELLAR=1 go test ./pipeline/ \
     -run TestGenerateInterstellarClips -v -timeout 320m
   ```
3. **Tekshir:**
   - Parser logida `falling back to audio` BO'LMASIN (transcript yo'lida qolsin)
   - `transcript path selected ... windows=[...]` — kliplar butun filmga taqsimlanganmi
   - Mashhur yig'lash sahnasi (~6600s atrofi) tanlanganmi
   - Subtitr toza o'zbekcha va to'g'ri vaqtda
   - `worker/uploads/movies/clips/interstellar/*.mp4` ni VLC'da ko'r
4. **Sifat yaxshi bo'lsa** — pipeline tayyor. Keyin push/PR haqida o'ylash mumkin.

### Hali ham e'tiborda tutiladigan
- To'liq film CPU Whisper ~60-90 daq + face ~15-25 daq = har film ~1.5-2 soat. Production uchun GPU / tezroq whisper / async job kerak.
- Production'da **billing yoqilgan Gemini kalit** shart (bepul 20/kun yetmaydi).
