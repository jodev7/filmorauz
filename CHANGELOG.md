# Changelog

## 2026-05-19 (follow-up 4) — Room limits on /premium + free-tier quota popup

### Frontend

- **Room limits on `/premium` page** (`app/premium/page.tsx`).
  - Added a "Birga ko'rish" comparison table (Free vs Premium):
    daily rooms, members per room, visibility, invite TTL.
  - New feature card "Birga ko'rish: cheksiz" in the features grid.
  - Two new bullets in `premiumFeatures` list.

- **Quota-reached upsell popup**
  (`components/WatchTogetherButton.tsx`). When the backend rejects
  with `daily room limit reached` (the free-tier 3-rooms/day cap), the
  button no longer shows a generic error — a dedicated popup opens
  with a yellow "PREMIUM bilan: cheksiz room / 20 a'zo / reklamasiz"
  panel and a "PREMIUM olish" button linking to `/premium`. A
  "Keyinroq" button dismisses it.

### Files touched

- `frontend/src/app/premium/page.tsx`
- `frontend/src/components/WatchTogetherButton.tsx`

## 2026-05-19 (follow-up 3) — Public rooms, series rooms, reconnect UX, "next ep" request

Five-feature pass on the watch-room flow.

### Backend

- **Visibility + member-cap on create** (`handlers/watch_room_handler.go`).
  `POST /api/rooms` body now accepts `visibility` (already supported) and
  `max_members` (new). Server caps to plan limit (free: 2, premium: 20);
  rejects values below 2.

- **Series rooms** (`models/watch_room.go`,
  `handlers/watch_room_handler.go`, `repositories/watch_room_repository.go`,
  `services/watch_room_hub.go`). New `content_type=series` where
  `content_id=seriesID` and a `current_episode_id` tracks what's playing.
  `resolveContent` picks the first episode of the first season at
  creation time; `SetCurrentEpisode` repo method swaps it later and
  resets playback to 0. Host changes via `POST /api/rooms/:id/episode`;
  hub broadcasts a `episode_change` WS message so guests reload their
  player without a refresh.

- **`GET /api/rooms/public`** (`handlers/watch_room_handler.go`,
  `routes/routes.go`). Public room browse endpoint that returns active
  public rooms plus a live `member_count` snapshot from the hub. Drives
  the new `/rooms` page.

- **Host-disconnect grace broadcast**
  (`services/watch_room_hub.go`). When the host drops, the hub now
  pushes a `host_disconnected` WS message with a `deadline_ms` (and
  `grace_seconds`) so clients can render a countdown instead of a
  silent buffer. `host_reconnected` is sent when the host comes back
  before the timer fires. `HubRoom.HostDisconnectDeadline` is kept
  in-memory for the same reason.

- **`episode_request` WS message** (`services/watch_room_hub.go`).
  Guests send `{target_episode_id, reason}`; the hub forwards it
  only to the host. Drives the "Keyingisini so'rash" guest button
  and the host-side toast.

- **Admin stats: leaderboards**
  (`repositories/watch_room_repository.go`,
  `handlers/watch_room_handler.go`). Two new aggregations:
  `GetTopContent` (most-roomed movies/series) and `GetTopHosts`
  (most-hosting users). Stats endpoint now returns `top_content` and
  `top_hosts` in addition to the existing counters.

### Frontend

- **`/rooms` public list page** (`app/rooms/page.tsx`). Auto-refreshing
  grid of public rooms with poster, host, member count, play state,
  and a "Qo'shilish" button (disabled when full).

- **`WatchTogetherButton` rewrite** (`components/WatchTogetherButton.tsx`).
  Now opens a modal with visibility radio (Maxfiy / Ochiq) and a member
  slider (min 2 → plan max). Plan max is read from
  `user.is_premium`. Also supports `content_type="series"` for the new
  series-room button on the series page.

- **Series page integration** (`app/series/[slug]/page.tsx`). Added
  `<WatchTogetherButton contentType="series" />` next to the rating,
  so users can start a series room without picking an episode first.

- **Series-room controls in the room page**
  (`app/watch-room/[id]/page.tsx`). Host gets a "Keyingisi" quick-button
  and an "Epizodlar" modal (lazy-loaded via new
  `getSeriesEpisodesByID` in `lib/series-api.ts`); guests get a
  "Keyingisini so'rash" button that sends `episode_request`. Auto-
  advance fires on the host's player `ended` event when there's a
  next episode.

- **Host-disconnect overlay** (`app/watch-room/[id]/page.tsx`). Full-
  player overlay with a live mm:ss countdown driven by
  `host_disconnected.deadline_ms`. Disappears on `host_reconnected`.

- **Episode-request toast** (`app/watch-room/[id]/page.tsx`). Host sees
  a non-blocking toast when a guest asks to change episode, with a
  one-click "O'tish" action that calls `changeRoomEpisode`.

- **WS protocol additions** (`lib/use-room-socket.ts`). New
  `host_disconnected`, `host_reconnected`, `episode_change`,
  `episode_request` event types + a `sendEpisodeRequest(...)` sender.

- **Admin leaderboards** (`app/admin/rooms/page.tsx`). Two new cards
  under the stat tiles: "Eng ko'p room ochilgan kontent" and "Eng
  faol hostlar". Render the `top_content` / `top_hosts` arrays from
  the stats endpoint.

- **Navbar /rooms link** (`components/Navbar.tsx`). Desktop nav now
  has a "Roomlar" link next to "Series" pointing at the new public
  list page.

### Files touched

- `backend/models/watch_room.go`
- `backend/handlers/watch_room_handler.go`
- `backend/repositories/watch_room_repository.go`
- `backend/services/watch_room_hub.go`
- `backend/routes/routes.go`
- `frontend/src/lib/api.ts`
- `frontend/src/lib/series-api.ts`
- `frontend/src/lib/use-room-socket.ts`
- `frontend/src/components/WatchTogetherButton.tsx`
- `frontend/src/components/Navbar.tsx`
- `frontend/src/app/rooms/page.tsx` (new)
- `frontend/src/app/watch-room/[id]/page.tsx`
- `frontend/src/app/series/[slug]/page.tsx`
- `frontend/src/app/admin/rooms/page.tsx`

## 2026-05-19 (follow-up 2) — Admin room stats + dedicated stats collection

### Backend

- **TTL index removed from `watch_rooms`**
  (`repositories/watch_room_repository.go`). The 12-hour TTL was wiping
  closed rooms from the collection, so the admin "total" and "this
  month" counters could only ever reflect the rolling-12h window. The
  legacy `expires_at_1` index is now dropped on startup (best-effort);
  rooms stay in Mongo as the historical record.

- **New `watch_room_stats` collection**
  (`repositories/watch_room_repository.go`). Singleton doc
  `{_id: "global", total_created, total_closed, by_month: {"YYYY-MM": int}}`
  bumped via `$inc` upsert from `CreateRoom` (totals + this-month
  bucket) and `CloseRoom` (only on the `ModifiedCount==1` transition,
  so calling CloseRoom twice on the same room doesn't double-count).
  Read by `GetRoomStats` so the dashboard no longer needs three
  `CountDocuments` calls per refresh.

- **One-time backfill** (`backfillStatsIfMissing`). On first boot after
  this change, the singleton is seeded from whatever rows still exist
  in `watch_rooms` (aggregating per-month buckets from `created_at`).
  Subsequent boots are no-ops once the doc exists.

- **New compound index** `{status:1, created_at:-1}` on `watch_rooms`
  to keep the admin "active list" query fast now that the collection
  grows unbounded.

- **`GetRoomStats` rewrite**. `total`, `closed`, and `this_month` come
  from the singleton; `active` is still a live `CountDocuments` against
  `watch_rooms` because that's the only source of truth for currently-
  open sessions.

### Frontend

No changes — the `GET /api/admin/rooms/stats` response shape
(`{active, closed, total, this_month}`) is unchanged, so the dashboard
stat cards keep working as-is.

### Files touched

- `backend/repositories/watch_room_repository.go`

## 2026-05-19 (follow-up) — Watch-room sync, quality picker, host grace, active-room pill

Second pass on the same feature based on a fresh round of user testing.

### Backend

- **Host disconnect grace 60s → 5min** (`services/watch_room_hub.go`).
  A host who briefly loses connectivity or accidentally closes a tab now
  has 5 minutes to walk back in before the room is torn down. The chat
  system message was updated to match the new window.

- **Heartbeat 5s → 2s** (`services/watch_room_hub.go`). Guests reported
  a multi-second drift between their playhead and the host's. The hub
  now broadcasts `state_sync` every 2s so drift is corrected ~2.5×
  faster without measurable bandwidth cost.

- **`GET /api/rooms/mine/active`** (`handlers/watch_room_handler.go`,
  `repositories/watch_room_repository.go`, `routes/routes.go`). New
  auth-required endpoint that returns the user's currently-open hosted
  room (or 204 if none). Drives the new navbar pill.

- **`DELETE /api/notifications/:id`** (`handlers/notification_handler.go`,
  `services/notification_service.go`,
  `repositories/notification_repository.go`, `routes/routes.go`). One-shot
  notifications (room invites) need to disappear permanently when
  clicked — local-only removal wasn't enough; the row came back on the
  next dropdown open and a re-click would land on a now-closed room.

### Frontend

- **Sync drift threshold 1.5s → 0.7s**
  (`components/watch-room/RoomPlayer.tsx`). Tighter snap matches the
  faster 2s heartbeat so noticeable drift is corrected immediately.

- **Quality picker source preference**
  (`app/watch-room/[id]/page.tsx`). The protected-media playback URL is a
  single-variant CDN URL — the quality picker had nothing to pick. The
  page now prefers the raw `master_playlist_url` from the public
  movie/episode doc when it's a directly-playable `.m3u8`, and falls
  back to the protected URL only when the raw master isn't available.

- **System message wording** (`app/watch-room/[id]/page.tsx`).
  "60 soniya ichida qaytmasa room yopiladi" →
  "5 daqiqa ichida qaytmasa room yopiladi", matching the new grace.

- **Notification deletion on click**
  (`components/NotificationBell.tsx`, `lib/api.ts`). Clicking a
  `ROOM_INVITE` notification now also calls `DELETE /api/notifications/:id`,
  so the row stays gone after the dropdown is reopened.

- **Active-room pill** (`components/ActiveRoomBadge.tsx`,
  `components/Navbar.tsx`). New navbar component that polls
  `/rooms/mine/active` every 30s and renders a "Roomga qaytish" pill
  while the user has an open hosted room. Hides itself while the user
  is already inside the room. Fixes the case where a host clicked
  Back/Home and lost any way back into their still-running session.

### Files touched

- `backend/services/watch_room_hub.go`
- `backend/handlers/watch_room_handler.go`
- `backend/repositories/watch_room_repository.go`
- `backend/handlers/notification_handler.go`
- `backend/services/notification_service.go`
- `backend/repositories/notification_repository.go`
- `backend/routes/routes.go`
- `frontend/src/components/watch-room/RoomPlayer.tsx`
- `frontend/src/app/watch-room/[id]/page.tsx`
- `frontend/src/components/NotificationBell.tsx`
- `frontend/src/components/ActiveRoomBadge.tsx` (new)
- `frontend/src/components/Navbar.tsx`
- `frontend/src/lib/api.ts`

## 2026-05-19 — Watch-together feature polish

Today's work focused entirely on the synchronized co-viewing ("Birga ko'rish")
room flow that had landed the day before. All changes are user-facing fixes
and follow-ups reported during testing.

### Backend

- **401 on "Birga ko'rish" / WebSocket auth** (`197babc`).
  `watch_room_handler` was reading `c.Get("userID")` (camelCase) but
  `RequireAuth` middleware sets `c.Set("user_id", ...)`. Same bug in the
  WebSocket route's claim parser (`claims["userID"]`). Both now read
  `user_id` to match the middleware. Without this, every authenticated
  user was being rejected as "auth required".

- **Display-name resolution helper** (`197babc`). Added
  `resolveDisplayName` + `resolveAvatarURL` helpers. The hub previously
  passed bare `user.DisplayName`, which is empty for most Telegram-
  onboarded accounts, so the frontend rendered "Foydalanuvchi" for
  every member. The new helpers fall through
  `display_name → first_name → telegram_user → "Foydalanuvchi"` and
  prefer `profile_image_url` over `photo_url`.

- **User search now matches by Telegram ID** (`197babc`).
  `GET /api/rooms/users/search?q=…` first tries an exact ObjectID,
  then an exact `FindByTelegramID` for purely-numeric queries, then
  the case-insensitive display-name / username / first-name search.
  Response includes the `telegram_id` so the in-room invite modal can
  show it.

- **Invite link TTL → 10 minutes** (`db52ec7`).
  Free + premium hosts both get 10-minute invite links now. Every
  click of "Taklif yuborish" mints a fresh one, so a stale
  notification card can't be reused after the TTL.

### Frontend

#### Bug fixes

- **Watch-room crashed on freshly-created rooms** (`7b8ca6c`).
  `ListRoomMessages` returns `{items: null}` (Go zero-value slice
  serialised to JSON null) before there's any chat history. The page
  was calling `.map` on it and threw
  "Cannot read properties of null (reading 'map')". Guarded with
  `msgs.items || []`. Same guard added to `AdminRoomsPage` for the
  per-room members snapshot.

- **Black video player** (`dea74fb`, `197babc`).
  Two layered fixes:
  - The page was fetching the public movie/episode doc and using its
    raw `master_playlist_url`. That field holds a B2 object key, not
    a directly-playable URL, so the player showed black even though
    `/watch` worked for the same movie. Switched to
    `getProtectedMediaAccess({ movieId | episodeId, token })`, the
    same call the regular watch page already uses, and normalized the
    returned `playback_url` through `normalizeMediaUrl`. The direct
    URL chain is kept as a last-resort fallback for legacy rows that
    bypass protected media.
  - Widened the URL fallback chain to accept `master_playlist_url`,
    `streaming_url`, `playlist_url`, `video_url` so the player tries
    every plausible field before reporting "video manzili topilmadi".

- **Logged-in users were thrown back to "tizimga kiring" message**
  (`677145c`). Followed the invite link on an expired-session and got
  a bare instruction line. Replaced with a proper card that explains
  the room is private, you must register and log in via Telegram, and
  a button that stores the target URL in localStorage for post-login
  redirect.

- **`WatchTogetherButton` 401 UX** (`677145c`). If the room-create
  call fails with an auth error, show a clearer Uzbek message and
  route the user to `/?login=1` instead of silently bouncing to the
  homepage.

#### Room player rewrite

- **Custom `RoomPlayer` component** (`3a41ed2`,
  `frontend/src/components/watch-room/RoomPlayer.tsx`). Replaced the
  bare `<video>` tag + ad-hoc HLS effect with a focused player:
  - hls.js attach with adaptive level parsing.
  - Top overlay with the movie title, PRO badge when the host is
    premium, and a "Mehmon" hint for guests.
  - Bottom overlay with scrub bar (host only), play/pause (host only),
    volume slider, time, quality picker (Auto / 1080p / 720p / 480p
    per viewer), restart, fullscreen.
  - Controls auto-hide 2.5 s after the last mouse move while playing.
  - `registerSync({ setPosition, setPlaying })` API so the parent can
    drive guest clients without reaching into private refs.
  - Premium yellow ring on the player when the movie is premium
    (matches the cue on the watch page).

- **Sync uses `canplay` instead of immediate seek** (`db52ec7`).
  Guest "Buffering…" lock was caused by `currentTime` being set
  before the manifest was parsed; the browser silently dropped the
  seek. Sync requests are now queued in a ref and replayed on
  `loadedmetadata` / `canplay`.

- **Host position persisted across re-entry**
  (`db52ec7`, `a459542`). Every 3 s, on every pause, on every seek,
  on `window.beforeunload`, and on component unmount, the host's
  `currentTime` is written to `localStorage` under
  `watchroom:pos:${roomID}-${hostUserID}`. On re-entry the value is
  read back after `loadedmetadata` and applied if it's within 5 s of
  the end.

- **Double-click toggles fullscreen** (`db52ec7`). Single-click still
  toggles play (220 ms delay so the dblclick can cancel it). Quality
  button is now reachable on touch as well — it cancels the click
  timer via `onPointerDown` + `onClick` `stopPropagation` (`d4f9a8b`).

#### Quality picker hardening

- **Index mapping fix** (`db52ec7`). hls.js' `currentLevel` expects
  the *original* index in `data.levels`, not the post-sort position.
  Rebuilt the level table to keep the original index.

- **Bitrate fallback** (`90a1e72`). Streams whose encoder omits
  `height` (Backblaze HLS in particular) are now labeled by
  `${kbps} kbps` instead of being dropped entirely.

- **Layered population**
  (`90a1e72`, `2ae8bd9`, `c9ab67b`, `a459542`). Picker now subscribes
  to four sources, in order:
  1. `Hls.Events.MANIFEST_PARSED`
  2. `Hls.Events.LEVEL_LOADED`
  3. `Hls.Events.LEVELS_UPDATED`
  4. A 1.5 s late-check on `hls.levels`, falling through to a manual
     `fetch(src)` of the master playlist and a regex parse of
     `#EXT-X-STREAM-INF` lines (`RESOLUTION` + `BANDWIDTH`).
  Each path logs the parsed variants to console so the actual
  manifest contents can be inspected from devtools when the picker
  still shows only "Auto".

- **Dropdown is actually clickable** (`d4f9a8b`). Every quality control
  (trigger button, dropdown wrapper, items) stops propagation on
  both `onPointerDown` and `onClick`, so the player-wide click timer
  no longer cancels the press. Added `z-50 + shadow-xl` so the menu
  sits above the bottom gradient and an explicit
  "Bu video uchun boshqa sifat mavjud emas" hint when the manifest
  really does only have one variant.

#### Room UI

- **`member_snapshot` on join** (`3e2f46c`). Hub previously only
  broadcast `member_joined` to *other* clients, so the joiner never
  saw themselves and the host was invisible in their own member
  list. New `member_snapshot` message is sent to the joining client
  with the full current roster. Frontend hook + page reducer handle
  it.

- **HLS.js loaded in the watch-room page** (`3e2f46c`). The previous
  bare-`<video>` setup didn't play `.m3u8` on Chrome / Firefox / Edge,
  hence the black screen. (Later folded into `RoomPlayer`.)

- **In-app invite by Telegram ID / username** (`3e2f46c`, `197babc`).
  Clicking "Taklif" opens a modal with a 300 ms-debounced search:
  per-row "Taklif" button issues a one-use, target_user_id-bound
  invite (backend's `CreateInvite` already pushes a `ROOM_INVITE`
  notification), or a "copy link" fallback at the bottom. The
  fallback no longer uses `alert()`; a custom popup with monospace
  link box, "Nusxa olish" action, copy-success line, and TTL note
  shows instead.

- **System chat entries on join / leave / kick / host drop**
  (`3a41ed2`):
  - "<name> room'ga qo'shildi"
  - "<name> chiqib ketdi"
  - "<name> room'dan chiqarildi" (host kick)
  - "<host name> (host) chiqib ketdi. 60 soniya ichida qaytmasa
    room yopiladi." — explicit grace-period notice when the host
    drops.

- **Twitch-style fullscreen chat overlay** (`3a41ed2`). When the
  player enters fullscreen, recent chat + reaction entries float in
  at top-right with a 0.3 s slide-in and a fade-out after ~10 s.
  Bounded to the last 8 events. Reactions are also mirrored into the
  side-chat log so casual viewers don't miss them (`db52ec7`).

- **Navbar + back row** (`90a1e72`, `c9ab67b`). `/watch-room/[id]` now
  renders the site `<Navbar />` plus an in-row "Orqaga" and
  "Bosh sahifa" pill button. The container has `pt-20 sm:pt-24` so
  the row clears the position-fixed `h-16` navbar (the original
  `pt-4` left the back-row underneath).

- **Notification dropdown handles room invites** (`db52ec7`). Clicking
  a `ROOM_INVITE` notification removes that entry from the dropdown
  immediately so a re-click can't follow the now-expired link.

### Files touched today

- `backend/handlers/watch_room_handler.go` — auth fix, display-name
  helpers, Telegram-ID search.
- `backend/routes/routes.go` — WS claim parser.
- `backend/repositories/user_repository.go` — `SearchByDisplayName`.
- `backend/services/watch_room_hub.go` — `member_snapshot`.
- `frontend/src/lib/api.ts` — `searchRoomUsers`, `RoomUserResult.telegram_id`.
- `frontend/src/lib/use-room-socket.ts` — `member_snapshot` event.
- `frontend/src/app/watch-room/[id]/page.tsx` — protected media,
  system messages, navbar, invite modal, custom link popup,
  `FullscreenChatOverlay`, RoomPlayer wiring.
- `frontend/src/components/watch-room/RoomPlayer.tsx` — new file.
- `frontend/src/components/NotificationBell.tsx` — sticky room
  invite removal.
- `frontend/src/components/WatchTogetherButton.tsx` — clearer 401 UX.
