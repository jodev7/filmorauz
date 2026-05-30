# Yangi Instagram akkauntni clip-upload tizimiga qo'shish

Bu qo'llanma yangi Instagram akkauntni FilmoraUz avtomatik clip-upload tizimiga
(Graph API / Instagram Business Login) ulashning to'liq ketma-ketligini beradi.

Tizim qanday ishlaydi (qisqacha): har bir akkaunt **bir marta** Instagram orqali
login qilib ruxsat beradi → biz 60 kunlik token olib `ig_accounts.json`'ga
saqlaymiz → backend `INSTAGRAM_ACCOUNTS_JSON`'ga akkauntni qo'shamiz → clip'lar
`graph.instagram.com` orqali joylanadi. VPS hech qachon instagram.com'ga login
qilmaydi, shuning uchun IP bloki bo'lmaydi.

---

## 0. Old shartlar

- Yangi Instagram akkaunt **Professional (Business yoki Creator)** turida bo'lishi
  kerak. (Instagram ilovasi → Settings → *Account type and tools* →
  *Switch to professional account* → **Creator**.)
- Meta Developer App allaqachon sozlangan (`INSTAGRAM_LOGIN_APP_ID`,
  `INSTAGRAM_LOGIN_APP_SECRET`, `INSTAGRAM_REDIRECT_URI` `parser/.env`'da bor).

---

## 1. Akkauntni app'ga "Instagram Tester" sifatida qo'shish

App test (Development) rejimida bo'lgani uchun faqat tester akkauntlar login
qila oladi.

1. https://developers.facebook.com/apps → FilmoraUz app'ini oching
2. Chap menyu: **App roles → Roles**
3. **Instagram Testers** bo'limi → **Add Instagram Testers**
4. Yangi akkaunt **username**'ini kiriting → **Submit**
   - Status **"На рассмотрении" / "Pending"** bo'lib turadi (keyingi qadamda
     qabul qilinadi).

---

## 2. Instagram ilovasida tester taklifini qabul qilish

Yangi akkaunt bilan:

1. Instagram ilovasi → profil → **☰ → Settings and activity**
2. **Apps and websites** (Ilovalar va veb-saytlar)
3. Yuqoridagi tablardan **Tester invites** (Sinovchi takliflari)
4. App taklifini toping → **Accept**

> Web orqali ham: o'sha akkaunt bilan login bo'lib
> https://www.instagram.com/accounts/manage_access/ → **Tester invites** → Accept.

Qabul qilingach Developer paneldagi status **"Accepted"** bo'ladi.

---

## 3. Login havolasini ochib `code` olish

1. Quyidagi havolani brauzerda oching (`CLIENT_ID` o'rniga real
   `INSTAGRAM_LOGIN_APP_ID` qo'yilgan to'liq havola):

   ```
   https://www.instagram.com/oauth/authorize?client_id=1608049066941367&redirect_uri=https%3A%2F%2Ffilmorauz.net%2Fig-callback&response_type=code&scope=instagram_business_basic%2Cinstagram_business_content_publish
   ```

   > Eslatma: brauzerda **aynan yangi akkaunt** login bo'lgan bo'lsin. Boshqa
   > akkaunt aralashmasligi uchun **inkognito oyna** ishlatish qulay.

2. Yangi akkaunt bilan login qiling → **Allow** (ruxsat bering)
3. Brauzer `https://filmorauz.net/ig-callback?code=XXXXX...#_` manziliga
   yo'naltiradi. Sahifa 404/bo'sh bo'lsa ham muhimi — manzildagi `code=` qiymati.
4. To'liq URL'ni yoki `code=` dan keyingi qismni nusxalang
   (oxiridagi `#_` ni olib tashlasangiz ham bo'ladi — skript o'zi tozalaydi).

> ⚠️ `code` faqat **bir marta** va ~1 daqiqa ishlaydi. Olgach darrov keyingi
> qadamga o'ting.

---

## 4. Tokenni olish va `ig_accounts.json`'ga qo'shish

Parser papkasida (local yoki to'g'ridan-to'g'ri VPS'da):

```bash
cd /opt/filmorauz/parser          # local'da: cd .../filmorauz/parser
./venv/bin/python ig_add_account.py "<code yoki to'liq callback URL>"
```

Skript:
- `code` → short-lived → **60 kunlik** tokenga almashtiradi
- akkaunt `username`, `user_id`, `access_token`'ni `ig_accounts.json`'ga yozadi
- xato bo'lsa (masalan code muddati tugagan) sababini chiqaradi

Muvaffaqiyatli natija:
```
OK: <username> (user_id=...) saved. Total accounts: N
```

> Agar local'da bajarsangiz — yangilangan `ig_accounts.json`'ni parser VPS'ga
> ko'chiring:
> ```bash
> scp ig_accounts.json root@144.91.79.0:/opt/filmorauz/parser/
> ```

---

## 5. Akkauntni backend'ga qo'shish (`INSTAGRAM_ACCOUNTS_JSON`)

Backend qaysi clip qaysi akkauntga ketishini **filter** orqali hal qiladi.
Backend VPS: `173.249.8.13`, fayl: `/opt/filmorauz/backend/.env`.

`INSTAGRAM_ACCOUNTS_JSON` ichidagi massivga yangi obyekt qo'shing:

```json
{
  "name": "yangi_nom",
  "username": "instagram_username",
  "password": "...",
  "filter": { "kind": ["movies","series"], "genres": ["anime"] }
}
```

Maydonlar:
- `name` — ichki yorliq (ixtiyoriy, takrorlanmasin)
- `username` — Instagram handle (`ig_accounts.json`'dagi bilan bir xil bo'lsin —
  parser shu orqali tokenni topadi)
- `password` — eski tizim qoldig'i, endi ishlatilmaydi, lekin format buzilmasin
  uchun qoldiriladi
- `filter` — qaysi kontent ketishini cheklaydi:
  - `kind`: `"movie"` | `"series"` | `["movies","series"]` (yoki yo'q = ikkalasi)
  - `genres`: janrlar ro'yxati
  - **bo'sh `{}` = HAMMA clip** o'sha akkauntga ketadi (ehtiyot bo'ling)

> Misol — faqat qo'lda yuklash uchun, avtomatik clip tushmasligi kerak bo'lsa,
> hech qaysi clip mos kelmaydigan filter qo'ying (masalan mavjud bo'lmagan janr),
> yoki akkauntni backend'ga umuman qo'shmang.

JSON to'g'riligini tekshiring:
```bash
grep "^INSTAGRAM_ACCOUNTS_JSON=" /opt/filmorauz/backend/.env \
  | python3 -c "import sys,json; d=json.loads(sys.stdin.read().split('=',1)[1]); print(len(d),'akkaunt OK')"
```

---

## 6. Servislarni qayta ishga tushirish

O'zgarishlar kuchga kirishi uchun:

- **Backend VPS** (`INSTAGRAM_ACCOUNTS_JSON` o'zgargani uchun):
  ```bash
  ssh root@173.249.8.13
  systemctl restart <backend-servis-nomi>
  ```
- **Parser VPS** (`ig_accounts.json` yangilangani uchun — agar parser uni
  ishga tushganda keshlasagina; hozirgi kod har so'rovda fayldan o'qiydi,
  shuning uchun odatda restart shart emas, lekin kafolat uchun):
  ```bash
  ssh root@144.91.79.0
  systemctl restart <parser-servis-nomi>
  ```

> ⚠️ Kino import jarayoni ketayotgan bo'lsa, restartni import tugagach qiling.

---

## 7. Tekshirish (test post)

Restartdan keyin admin paneldan yangi akkauntga bitta clip joylab ko'ring, yoki
parser VPS'da qo'lda test qiling:

```bash
cd /opt/filmorauz/parser
./venv/bin/python - <<'PY'
import importlib
m = importlib.import_module("server")
entry = m._ig_find_account("", "instagram_username")   # yangi username
print("topildi:", bool(entry), entry and entry["username"])
PY
```

Akkaunt topilsa — tayyor. Birinchi clip joylanganda Instagram'da Reel paydo bo'ladi.

---

## Token muddati (muhim)

- Tokenlar **60 kun** amal qiladi.
- `ig_refresh_tokens.py` cron orqali ularni avtomatik yangilab turadi
  (tavsiya: haftada bir marta):
  ```bash
  0 4 * * 0  cd /opt/filmorauz/parser && ./venv/bin/python ig_refresh_tokens.py >> /var/log/ig_refresh.log 2>&1
  ```
- Agar token tugab qolsa (refresh ishlamasa) — akkauntni shu qo'llanmaning
  **3–4 qadamlari** orqali qayta ulang (`ig_add_account.py` mavjud akkaunt
  tokenini yangilaydi, dublikat yaratmaydi).

---

## Xulosa — qisqa checklist

1. [ ] Akkaunt Professional (Creator) turida
2. [ ] App'ga Instagram Tester sifatida qo'shildi
3. [ ] Instagram ilovasida tester taklifi Accept qilindi
4. [ ] Login havolasidan `code` olindi
5. [ ] `ig_add_account.py "<code>"` ishga tushirildi → `ig_accounts.json`'ga qo'shildi
6. [ ] (local'da bo'lsa) `ig_accounts.json` parser VPS'ga ko'chirildi
7. [ ] Backend `INSTAGRAM_ACCOUNTS_JSON`'ga akkaunt + filter qo'shildi
8. [ ] Servis(lar) restart qilindi
9. [ ] Test post bilan tekshirildi
