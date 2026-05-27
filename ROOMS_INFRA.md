# Watch Rooms — Infra / Deploy (VPS) qo'llanmasi

Bu fayl **serverda (VPS'da) qo'lda qilinadigan ishlarni** to'playdi — kod o'zgarishi
yetarli bo'lmagan, infratuzilma sozlamasi talab qilinadigan qadamlar. Har bosqich
bo'yicha ajratilgan.

> Eslatma: "infra/deploy" = Redis o'rnatish, bir nechta backend instans ishga
> tushirish, nginx/load balancer WebSocket sozlash, `ulimit` (fayl-deskriptor)
> limitlarini ko'tarish kabi server tomonidagi ishlar.

---

## Bosqich 1–4 — infra ishi (deyarli) yo'q

Bu bosqichlar faqat kod. Odatdagidek deploy qilinadi (backend qayta build + restart,
frontend qayta build). Qo'shimcha server sozlamasi **shart emas**.

**Ixtiyoriy bir martalik tozalash** (1-bosqichdan keyin) — endi ishlatilmaydigan eski
chat kolleksiyasini Mongo'dan o'chirish:

```bash
mongosh "$MONGO_URI" --eval 'db.getSiblingDB("filmorauz").watch_room_messages.drop()'
```

Bu majburiy emas — kod bu kolleksiyaga umuman tegmaydi, shunchaki bo'sh joy egallab turadi.

---

## Bosqich 5 — Redis + multi-instance (haqiqiy gorizontal scale)

Bu yerda **infra ishi bor**. Kod `REDIS_URL` muhit o'zgaruvchisi orqali yoqiladi:
- `REDIS_URL` **bo'sh** → eski rejim: bitta backend instans, hammasi xotirada (hozirgi prod).
- `REDIS_URL` **to'ldirilgan** → cluster rejim: bir nechta backend instans bitta room'ni
  bo'lishishi mumkin (Redis pub/sub + umumiy holat).

### 1. Redis o'rnatish

```bash
sudo apt update && sudo apt install -y redis-server
sudo systemctl enable --now redis-server
redis-cli ping   # => PONG
```

`/etc/redis/redis.conf` tavsiyalari (watch-room ma'lumotlari **ephemeral** — disk
persistence shart emas, faqat tezkor xotira kesh sifatida ishlatamiz):

```
maxmemory 512mb
maxmemory-policy allkeys-lru
# AOF/RDB persistence'ni o'chirish mumkin (ma'lumot vaqtinchalik):
save ""
appendonly no
# Faqat localhost (yoki private network) eshitsin:
bind 127.0.0.1
# Agar parol kerak bo'lsa:
# requirepass <kuchli-parol>
```

`sudo systemctl restart redis-server`.

### 2. Backend `.env` (har instansda bir xil)

```
REDIS_URL=redis://127.0.0.1:6379/0
# parol bilan: redis://:<parol>@127.0.0.1:6379/0
```

Backend ishga tushganda log'da `Watch-rooms: cluster mode enabled (Redis)` chiqsa,
ulanish muvaffaqiyatli. Redis ulanmasa, backend **avtomatik** single-instance rejimga
qaytadi (xatolik bermaydi, faqat ogohlantirish log'i).

### 3. Bir nechta backend instans ishga tushirish

Har biri **boshqa portda** ishlasin (masalan 8080, 8081, 8082). systemd template
namunasi (`/etc/systemd/system/filmorauz-backend@.service`):

```ini
[Unit]
Description=FilmoraUz backend (instance %i)
After=network.target redis-server.service

[Service]
WorkingDirectory=/opt/filmorauz/backend
EnvironmentFile=/opt/filmorauz/backend/.env
Environment=PORT=%i
ExecStart=/opt/filmorauz/backend/server
Restart=always
# Fayl-deskriptor limitini ko'tarish (pastga qarang)
LimitNOFILE=200000

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now filmorauz-backend@8080
sudo systemctl enable --now filmorauz-backend@8081
sudo systemctl enable --now filmorauz-backend@8082
```

### 4. nginx — load balancer + WebSocket + sticky sessions

**Muhim:** bir foydalanuvchining WebSocket ulanishi **doimo bitta instansga** borishi
kerak (sticky). Buning uchun `ip_hash` (yoki sticky cookie) ishlatamiz.

`/etc/nginx/conf.d/filmorauz.conf`:

```nginx
upstream filmorauz_backend {
    ip_hash;                       # sticky: bir IP doim bitta instansga
    server 127.0.0.1:8080;
    server 127.0.0.1:8081;
    server 127.0.0.1:8082;
}

server {
    listen 443 ssl http2;
    server_name api.filmorauz.net;

    # ... ssl sozlamalari ...

    # WebSocket (watch-room) — /ws/ prefiksi
    location /ws/ {
        proxy_pass http://filmorauz_backend;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 3600s;   # WS uzoq ochiq turadi — timeout'ni uzaytirish
        proxy_send_timeout 3600s;
    }

    # Oddiy API
    location / {
        proxy_pass http://filmorauz_backend;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
    }
}
```

`sudo nginx -t && sudo systemctl reload nginx`.

> Nega sticky? Slow-mode rate-limit per-instance xotirada saqlanadi va WS o'qish
> pump'i ulanish qaysi instansda bo'lsa o'sha yerda ishlaydi. Sticky bo'lmasa
> reconnect har safar boshqa instansga tushib, kichik nomuvofiqliklar bo'lishi mumkin
> (broadcast/chat Redis orqali baribir to'g'ri tarqaladi, lekin sticky tavsiya etiladi).

### 5. Fayl-deskriptor (ulimit) limitlari

Har WebSocket ulanishi = 1 ochiq socket. 5000 ulanish uchun OS limiti yetarli bo'lishi
kerak. systemd unit'da `LimitNOFILE=200000` (yuqorida) qo'yilgan. Tizim darajasida ham:

`/etc/sysctl.conf`:
```
fs.file-max = 500000
net.core.somaxconn = 65535
net.ipv4.tcp_max_syn_backlog = 65535
```
```bash
sudo sysctl -p
```

`/etc/security/limits.conf` (agar systemd'siz ishga tushirilsa):
```
* soft nofile 200000
* hard nofile 200000
```

### 6. Tekshirish (cluster ishlayaptimi?)

1. 2+ instansni ishga tushiring, log'da "cluster mode enabled" ko'ring.
2. Bitta premyera room oching (admin orqali).
3. Ikki xil brauzer/IP'dan kiring — ehtimol turli instanslarga tushadi.
4. Birida chat yozing → ikkinchisida ko'rinishi kerak (Redis pub/sub orqali).
5. `redis-cli monitor` bilan `wr:bcast:*`, `wr:presence:*`, `wr:chat:*` kalitlarini
   kuzating.

### Ma'lum cheklovlar / kelajak ishlari

- **Presence count crash'da biroz "leak" qilishi mumkin** — instans to'satdan o'lsa,
  uning ulanishlari uchun `DECR` bajarilmay qoladi va son biroz yuqori ko'rinadi.
  Kalitlarda 24s TTL bor, lekin aniqroq hisob uchun davriy reconciliation kerak (TODO).
- **Bo'sh premyera room'ning HubRoom'i instansda qoladi** (keepAlive) — admin yopmaguncha
  yoki TTL tugamaguncha xotirada turadi. Premyeralar kam bo'lgani uchun bu muammo emas.
- Redis **bitta nuqtada xatolik** bo'lmasligi uchun prodda Redis Sentinel yoki
  managed Redis (masalan Upstash/ElastiCache) tavsiya etiladi.
