# FilmoraUz — Project Overview for Claude

## What This Is

FilmoraUz is an Uzbek online cinema platform (streaming site). Users browse and watch movies/series; auth is via Telegram bot. Admins ingest content via a metadata scraper + video processing pipeline.

---

## Repository Layout

```
filmorauz/
├── backend/       Go API server (Gin + MongoDB)
├── frontend/      Next.js 14 App Router (TypeScript + Tailwind)
├── worker/        Go video processing service (ffmpeg pipeline)
├── bot/           Go Telegram bot (content delivery + auth)
├── parser/        Python metadata scraper + downloader
└── Makefile       Dev commands
```

---

## Running Locally

```bash
make setup          # First-time: copies .env files, installs deps
make backend        # Terminal 1 — Go API on :8080
make parser         # Terminal 2 — Python scraper on :8082
make worker         # Terminal 3 — Go worker on :8081
make frontend       # Terminal 4 — Next.js dev on :3000
make bot            # Terminal 5 — Telegram bot (optional)
```

Each service reads its own `.env` file. Copy from `.env.dev` templates on first run.

---

## Backend (`backend/`)

**Stack:** Go, Gin, MongoDB (`go.mongodb.org/mongo-driver`), JWT, tgbotapi v5

### Architecture

```
config/         — Config struct, loads from .env via godotenv
models/         — MongoDB document structs (BSON tags)
repositories/   — DB layer (one file per collection)
services/       — Business logic
handlers/       — HTTP handlers (Gin context)
middleware/     — Auth/role middleware
routes/routes.go — All route registration in one file
main.go         — Wiring + startup
```

### Key Config (`backend/.env`)

| Variable | Purpose |
|---|---|
| `MODE` | `DEV` or `PROD` |
| `MONGO_URI` | MongoDB connection string |
| `DB_NAME` | Database name (default: `filmorauz`) |
| `JWT_SECRET` | JWT signing key (required in PROD) |
| `ADMIN_TELEGRAM_ID` | Seeds first admin user on startup |
| `BASE_SITE_URL` | Used for share links, SEO |
| `ALLOWED_ORIGIN` | CORS allowed origin |
| `TELEGRAM_BOT_USERNAME` | Bot username for login flow |
| `TG_CHANNEL_USERNAME` | Main Telegram channel |
| `TELEGRAM_CHANNELS` | Comma-separated channel list for ad broadcasting |
| `UPLOADS_DIR` | Local upload path (default: `./uploads`) |
| `B2_*` | Backblaze B2 credentials (PROD only) |
| `AI_ENDPOINT` | AI clip generation endpoint |
| `PARSER_SERVICE_URL` | Parser service URL (fallback: `PARSER_URL`) |

### API Routes (all under `/api`)

**Public:**
- `GET /movies`, `/movies/trending`, `/movies/slug/:slug`, `/movies/:id`, `/search`
- `GET /series`, `/series/:slug`, `/seasons/:id/episodes`, `/episodes/:id`
- `GET /collections`, `/collections/featured`, `/collections/slug/:slug`
- `GET /ads?placement=X`, `POST /ads/:id/impression`, `POST /ads/:id/click`
- `POST /movies/:id/view`
- `GET /auth/telegram/status/:code`

**Auth (JWT required):**
- `GET /auth/me`, `PATCH /auth/me`, `POST /auth/logout`
- `GET /user/history`, `GET /user/continue-watching`
- `POST /user/favorites/:movieId`, `DELETE /user/favorites/:movieId`
- `POST /watch/:movieId/progress`, `POST /watch/:movieId/complete`

**Admin (`/api/admin/*`, requires `admin` or `superadmin` role):**
- Movie CRUD, series/season/episode CRUD
- Ingestion job management
- User management (role, premium, ban/unban)
- Collection management
- Clip management + Instagram upload

**Superadmin (`/api/superadmin/*`):**
- Ad CRUD, ad media upload, send ad to Telegram
- Telegram post management

### Auth Flow

1. Frontend calls `POST /api/auth/telegram/start` → gets `code` + bot deep-link URL
2. User opens Telegram bot link → bot calls `POST /api/auth/telegram/complete`
3. Frontend polls `GET /api/auth/telegram/status/:code` until `status === "completed"`
4. Frontend stores JWT in cookie, fetches full profile from `GET /api/auth/me`

**Important:** `FindByTelegramID` returns `(nil, nil)` for missing users — not `(nil, error)`. Always check `existingUser == nil`, not `err != nil`.

**Name sanitization:** Telegram users can have "." or empty `first_name`. Backend sanitizes at every layer. Priority: `first_name → username → "User"`. Invalid values: empty string, ".", "-" after trim.

### MongoDB Collections

| Collection | Model | Purpose |
|---|---|---|
| `users` | `User` | Accounts, roles, premium status |
| `auth_sessions` | `AuthSession` | Pending Telegram login sessions |
| `movies` | `Movie` | Movies + metadata |
| `series` | `Series` | TV series |
| `seasons` | `Season` | Series seasons |
| `episodes` | `Episode` | Individual episodes |
| `watch_history` | `WatchHistory` | Per-user watch history + progress |
| `favorites` | `Favorite` | User favorites |
| `movie_ratings` | `MovieRating` | 1–5 star ratings |
| `series_ratings` | `SeriesRating` | Series ratings |
| `movie_comments` | `MovieComment` | Comments + replies |
| `collections` | `Collection` | Curated movie collections |
| `ingestion_jobs` | `IngestionJob` | Video download/process job queue |
| `clips` | `Clip` | AI-generated short clips |
| `ads` | `Ad` | Ad creatives + placement config |
| `shares` | `Share` | Movie share links |
| `notifications` | `Notification` | User notifications |
| `ban_history` | `BanHistory` | Admin ban log |
| `ban_appeals` | `BanAppeal` | User ban appeals |
| `telegram_posts` | `TelegramPost` | Broadcast post history |

### Ad System

Ads have two generations of fields:
- **Old (Telegram-only):** `image_url`, `video_url` — used for initial Telegram delivery
- **New (website slots):** `telegram_media_url`, `banner_media_url`, `inline_media_url`, `fixed_bottom_media_url`, `popup_media_url`, `player_overlay_media_url` — slot-specific

**Website placements:** `homepage_top_banner`, `homepage_inline_block_1`, `homepage_popup`, `watch_page_inline_block`, `watch_player_overlay`, `website_fixed_bottom`

**Media fallback chain** (frontend resolves in order):
- Banner → `banner_media_url || image_url`
- Inline → `inline_media_url || banner_media_url || image_url`
- Popup → `popup_media_url || banner_media_url || inline_media_url || image_url`
- Fixed bottom → `fixed_bottom_media_url || banner_media_url || image_url`
- Player overlay → `player_overlay_media_url || banner_media_url || image_url`

**MIME validation** (`upload_handler.go`): 3-stage cascade — part Content-Type header → file extension → `http.DetectContentType` (byte sniff). Allowed: `image/jpeg`, `image/png`, `image/webp`, `image/gif`, `video/mp4`, `video/webm`, `video/quicktime`.

**Telegram media sending** (`telegram_service.go`): localhost/`/uploads/` paths → `tgbotapi.FilePath` (multipart upload); public CDN URLs → `tgbotapi.FileURL`.

### Storage

- **DEV:** Local filesystem at `./uploads/` (served at `/uploads/` static route)
- **PROD:** Backblaze B2 via CDN URL (`B2_CDN_URL`)

### Premium System

- `User.IsPremiumActive` + `User.PremiumExpiresAt`
- Background job (`startPremiumCleanupJob`) runs every 10 min to expire stale subscriptions
- Premium movies locked behind `PremiumLockOverlay` on watch page
- Users notified 3 days before expiry and on expiry

---

## Frontend (`frontend/`)

**Stack:** Next.js 14 App Router, TypeScript, Tailwind CSS, `js-cookie`, `lucide-react`

### Pages

| Route | File | Notes |
|---|---|---|
| `/` | `app/page.tsx` | Home, server component, fetches movies/series |
| `/movies` | `app/movies/page.tsx` | Browse/filter movies |
| `/movies/[slug]` | `app/movies/[slug]/page.tsx` | Movie detail page |
| `/watch/[slug]` | `app/watch/[slug]/page.tsx` | Watch page (uses WatchPageClient) |
| `/series` | `app/series/` | Series listing |
| `/episode/[id]` | `app/episode/[id]/` | Episode watch page |
| `/user` | `app/user/page.tsx` | User profile, history, favorites |
| `/collections` | `app/collections/` | Collection listing/detail |
| `/admin/*` | `app/admin/` | Admin dashboard (client-side auth guard) |
| `/premium` | `app/premium/` | Premium subscription info |
| `/banned` | `app/banned/` | Shown to banned users |

### Key Components

- `Navbar` — top nav with auth state, user menu
- `TelegramLoginModal` — Telegram login flow (polls auth status)
- `VideoPlayer` — video.js wrapper, supports direct URL + embed + HLS
- `WatchPageClient` — full watch page with player, info, ads, recommendations
- `WebsiteAdSlot` — renders banner/inline/popup ads from API
- `FixedBottomAd` — persistent bottom banner ad
- `PremiumComponents` — `PremiumLockOverlay`, `PremiumButton`, `PremiumBadge`
- `MovieCarousel` / `SeriesCarousel` — horizontal scroll rows
- `HeroCarousel` — full-width hero banner (latest movies)
- `Comments` — comment thread with replies
- `BanGuard` / `BanGuardWrapper` — redirects banned users

### Key Libraries (`lib/`)

- `api.ts` — all backend API calls
- `auth-context.tsx` — `AuthProvider`, `useAuth()` hook; stores JWT in cookie
- `i18n.tsx` / `i18n-server.ts` — Uzbek/Russian i18n
- `localization.ts` — `getLocalizedTitle()`, `getLocalizedDescription()` etc.
- `series-api.ts` — series-specific API calls
- `ads-utils.ts` — `shouldShowAds()` utility (not used in ad components directly)

### Auth Context

`useAuth()` returns: `{ user, token, isLoading, isPremiumActive, login, logout }`

After login completes, `checkAuthStatus` fetches full profile via `GET /api/auth/me` to get `display_name`, premium status, ban status. JWT is stored in cookie (`auth_token`).

### i18n

Language is always Uzbek (`uz`) for now. Server: `getTranslations("uz")`. Client: `useI18n()`. Translation keys are in the i18n files.

### Frontend `.env.local` Variables

| Variable | Purpose |
|---|---|
| `NEXT_PUBLIC_API_URL` | Backend URL (default: `http://localhost:8080`) |
| `NEXT_PUBLIC_SITE_URL` | Public site URL for SEO/OG |
| `NEXT_PUBLIC_TELEGRAM_BOT_USERNAME` | Bot username for login links |

---

## Worker (`worker/`)

**Stack:** Go, ffmpeg subprocess calls

Processes ingestion jobs from the queue:
1. Claims a job via `GET /api/ingestion/jobs/worker/claim`
2. Runs ffmpeg to transcode video (HLS adaptive bitrate output)
3. Uploads to storage (local or B2)
4. Reports progress via `POST /api/ingestion/jobs/:id/progress`
5. Calls `POST /api/telegram/notify-movie` when complete

---

## Parser (`parser/`)

**Stack:** Python, Flask-based HTTP server (`server.py` on :8082)

Scrapers:
- `asilmedia.py` — AsilMedia source
- `uzmovi.py` — UzMovi source
- `kinolar.py` — Kinolar source
- `freekino.py` — FreeKino source

Utilities:
- `metadata_normalizer.py` — normalizes movie metadata across sources
- `downloader_service.py` — handles video file downloads
- `media_extractor.py` — extracts embed URLs

Backend calls parser at `PARSER_SERVICE_URL` (default: `http://127.0.0.1:8082`).

---

## Telegram Bot (`bot/`)

**Stack:** Go, tgbotapi v5

- Handles the `/start?code=XXX` deep link for auth
- Users can look up movies by code (e.g. `/001`)
- Admins can broadcast movie notifications
- Bot replies with movie info + watch links when code sent

---

## Common Gotchas

1. **`FindByTelegramID` returns `(nil, nil)` for missing users** — always check `existingUser == nil`, never `err != nil` alone.

2. **Ad components do NOT use `shouldShowAds()`** — `WebsiteAdSlot`, `FixedBottomAd`, `PlayerOverlayAd` always fetch and render regardless of user premium status or route.

3. **Popup ad** is mounted in `app/page.tsx` (home page only). Player overlay ad is mounted inside `WatchPageClient`.

4. **Ad media field priority** — new-style ads use slot-specific fields (`banner_media_url` etc.), not the old `image_url`/`video_url`. Always use the fallback chain.

5. **Telegram media in DEV** — use `resolveTelegramMediaSource()` which detects localhost paths and sends as local file. Never pass localhost URLs directly to Telegram API.

6. **Display name sanitization** — treat ".", "-", empty string, null as invalid names at all layers (backend + frontend).

7. **Route ordering in Gin** — more specific routes must be registered before wildcard routes (e.g. `/movies/slug/:slug` before `/movies/:id`).

8. **Series route conflict** — season/episode routes use separate prefixes (`/seasons/`, `/series-by-id/`) to avoid wildcard conflicts with `/series/:slug`.
