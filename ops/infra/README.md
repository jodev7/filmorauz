# Infra / Ops (filmorauz)

Source-of-truth copies of host config and scripts that live **outside** the app tree
on the production hosts. Kept here so local = git = servers. These files are
**secret-free and host-agnostic**: real values (host addresses, DB users, passwords,
bucket names, keys) stay in each service's `.env` (git-ignored) and in root-only files
on the hosts. Replace the `<...>` / placeholder values when installing.

## Hosts (roles, not addresses)
- **Web host** — `filmorauz-backend`, `filmorauz-bot`, the Next.js frontend (PM2),
  the self-hosted MongoDB, and netdata.
- **Worker host** — `filmorauz-worker`, `filmorauz-parser`, and the SSH tunnel to the DB.

## Database
The prod DB is a **self-hosted MongoDB 8.0 on the web host**, bound to `127.0.0.1`
only, auth enabled. (It replaced a managed shared-tier cluster whose read throughput
was throttled; local reads are ~200× faster.) Two DB users: an app user
(`readWrite` on the app DB) and an admin user (`root`); their passwords live in a
root-only creds file on the web host, never in git. App services and `contentbot`
read the connection URI from `backend/.env` / `worker/.env`.

## Worker → DB link: `fu-mongo-tunnel.service` (worker host)
mongod is never exposed publicly. The worker reaches it over an **encrypted SSH
tunnel** that forwards the worker's `127.0.0.1:27017` to the web host's mongod. The
tunnel key is authorized on the web host with a **forwarding-only** entry:
`restrict,port-forwarding,permitopen="127.0.0.1:27017" <pubkey>` (no shell).
Install: set `WEB_VPS_HOST` in the unit, `cp` it to `/etc/systemd/system/`, then
`systemctl enable --now fu-mongo-tunnel`.

## Backups: daily → private B2 bucket, keep last 2
- `filmorauz-db-backup.sh` → `/usr/local/bin/`; `filmorauz-db-backup.{service,timer}`
  → `/etc/systemd/system/` (daily 03:30). Dumps the local Mongo to `/root/db-backups/`
  (keeps last 2 locally), then calls `b2-upload.sh`.
- `b2-upload.sh` → `/root/db-backups/`. Uploads to a **private** B2 bucket
  (`BACKUP_BUCKET` env) and keeps only the last 2 there. Uses a dedicated B2 auth
  profile — authorize once:
  `B2_ACCOUNT_INFO=/root/.config/b2/backup_account_info b2 account authorize <keyID> <appKey>`.
- Enable: `systemctl enable --now filmorauz-db-backup.timer`.

## Frontend scaling: PM2 cluster + autoscaler (web host)
Next.js runs in PM2 **cluster mode**:
`pm2 start node_modules/next/dist/bin/next --name filmorauz-frontend -i 2 --cwd <frontend dir> -- start` (then `pm2 save`).
`fe-autoscale.sh` (+ `.service`/`.timer`, every 60 s) scales instances **MIN=2 ↔ MAX=4**
by average per-instance CPU (up >70%, down <20%). Install into `/usr/local/bin` +
`/etc/systemd/system`, then `systemctl enable --now fe-autoscale.timer`.

## nginx (web host)
- `nginx-brotli.conf` → `/etc/nginx/conf.d/brotli.conf` (needs
  `libnginx-mod-http-brotli-filter` / `-static`).
- HTTP/2 enabled on the 443 server blocks (`listen 443 ssl http2;`).
- No proxy_cache: the backend serves catalogue responses from an in-memory cache
  (~1 ms) and sends `Cache-Control: public` on them, so edge caching is left to the
  CDN — see `../cloudflare/`.

## Caching (in app code)
`backend/middleware/cache_middleware.go` caches public GET catalogue responses in
memory (TTL 30–60 s) and emits `Cache-Control: public, max-age=…`. Authenticated
requests (Authorization header) bypass it, so per-user data is never cached. Applied
in `backend/routes/routes.go` to movies/series/collections/search/detail/recommendations.
