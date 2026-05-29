# Changelog

## 2026-05-27

### Watch Rooms — chat, premyera roomlar va scale

**Chat endi ephemeral (MongoDB'siz)** — `8476535`
- Chat/emoji xabarlari hech qaysi roomda Mongo'ga yozilmaydi.
- Har room hubda oxirgi 100 ta xabarni in-memory ring buffer'da saqlaydi; chiqib-qaytgan user history'ni `chat_history` WS eventi orqali ko'radi.
- Room yopilsa chat butunlay yo'qoladi. `watch_room_messages` collection, REST `GET /rooms/:id/messages` va tegishli repo metodlari olib tashlandi.

**Admin/Premiere roomlar (pinned)** — `d854350`
- Admin/superadmin "premyera" room ochadi: public, katta sig'imli (default 5000), `/rooms` topida pinned, ixtiyoriy countdown bilan.
- `WatchRoom`ga `Kind`, `IsFeatured`, `PinPriority`, `ScheduledStartAt`.
- `POST /api/admin/rooms` (RequireAdmin, kvota/cap'dan o'tib ketadi), `GET /api/rooms/featured`.
- Frontend: `/rooms`da "Premyeralar" bo'limi + jonli countdown; admin sahifasida premyera yaratish formasi (kino/serial qidiruv).

**WebSocket/chat scale optimizatsiyasi** — `df3155e`
- Batched writes: har client uchun yozuvlar ~200ms (yoki 64 ta) da bitta `batch` frame'ga jamlanadi.
- presenceMode (premyera yoki >50 a'zo): per-member join/left va to'liq snapshot broadcast qilinmaydi; jonli son `state_sync.member_count`dan.
- Chat slow-mode (2s/user), typing presenceMode'da o'chiriladi.
- Premyera keepAlive: room bo'shasa/host uzilsa yopilmaydi.
- `GET /api/rooms/:id/members` (paginated roster).

**Virtualized a'zolar ro'yxati** — `6830bae`
- Faqat ko'rinadigan blok render qilinadi (windowing); 5000 a'zoda ham DOM yengil.
- Roster sahifalari scroll bilan lazy yuklanadi, har 8s yangilanadi. Kichik roomlar oddiy ro'yxatda qoladi. (Kutubxonasiz, toza React.)

**Redis cluster mode (multi-instance)** — `fcf1675`
- `REDIS_URL` orqali yoqiladi (bo'sh → eski single-instance rejim, fallback bilan).
- `RoomBus`: Redis pub/sub fan-out + umumiy playback state, presence counter, chat ring buffer, roster.
- Cluster-aware kick/close control xabarlari; host_action authoritative head Redis'ga yoziladi; heartbeat local-only.

**Infra hujjati** — `fcf1675`, `34e4c85`
- `ROOMS_INFRA.md`: VPS deploy qo'llanmasi (Redis, multi-instance systemd, nginx WebSocket + sticky session, ulimit/sysctl) + noldan cluster rejimgacha ketma-ket checklist.

### Movies — watch page movie page ichiga birlashtirildi (Variant A)

**Inline player** — `ba53e3e`
- Alohida `/watch/[slug]` sahifa kinolar uchun olib tashlandi; player endi movie detail page ichida inline ochiladi (bir klik kam).
- Watch page'dagi hammasi saqlandi: player, himoyalangan media, resume prompt, progress tracking, view recording, premium lock, player-overlay + inline reklama, watch-together.
- `WatchPageClient`ga `embedded` prop (epizodlar to'liq layout'da o'zgarmaydi); player og'ir bundle'i faqat ochilganda lazy yuklanadi.
- `/watch/[slug]` → `/movies/[slug]?play=1`ga 308 redirect (eski havolalar buzilmaydi).
- Home (Hero/Trending/Continue Watching) va video sitemap `PlayerLoc` → `/movies/[slug]?play=1`.
- Browser'da tasdiqlandi: inline mount, no-navigation, redirect+autoOpen, premium gating.
