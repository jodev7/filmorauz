# 🎬 FilmoraUz

Movie streaming platform. No Docker required — runs natively on your machine.

---
[System: Verified Version Sync]

## Prerequisites

- **Go 1.18+** — https://go.dev/dl/
- **Node.js 18+** — https://nodejs.org/
- **MongoDB** — install locally (see below)

---

## Install MongoDB locally

### Ubuntu / Debian
```bash
sudo apt install -y mongodb
sudo systemctl start mongodb
sudo systemctl enable mongodb   # auto-start on boot
```

### macOS
```bash
brew tap mongodb/brew
brew install mongodb-community
brew services start mongodb-community
```

### Windows
Download from https://www.mongodb.com/try/download/community
Run the installer, MongoDB starts automatically as a service.

Verify it's running:
```bash
mongosh        # should open a shell
# or
mongo --eval "db.runCommand({ ping: 1 })"
```

---

## Development Setup

### 1. First-time setup (run once)
```bash
cd filmorauz
make setup
```

This automatically:
- Copies `backend/.env.dev` → `backend/.env`
- Copies `frontend/.env.dev` → `frontend/.env.local`
- Runs `npm install`
- Runs `go mod tidy`

### 2. Start the project (2 terminals)

**Terminal 1 — Backend:**
```bash
make backend
```
Expected output:
```
Starting in DEV mode
Connected to MongoDB at filmorauz
Admin user created: admin@filmorauz.uz
FilmoraUz API listening on :8080
```

**Terminal 2 — Frontend:**
```bash
make frontend
```
Expected output:
```
▲ Next.js 14.2.29
- Local: http://localhost:3000
```

### 3. Open the app

| URL | What |
|-----|------|
| http://localhost:3000 | Public site |
| http://localhost:3000/admin/login | Admin panel |
| http://localhost:8080/api/health | API health check |

**Admin login:**
- Email: `admin@filmorauz.uz`
- Password: `admin123`

### 4. Optional: Parser Service (3rd terminal)

The parser service extracts movie metadata from various sources. It requires Python 3 with a virtual environment.

```bash
# First-time setup (creates venv and installs dependencies)
make setup-parser

# Run the parser
make parser
```
Expected output:
```
Parser API server running on http://0.0.0.0:8082
Available sources: ['uzmovi', 'freekino', 'asilmedia']
```

### 5. Optional: Worker Service (4th terminal)

The worker processes video ingestion jobs from the queue. It requires MongoDB to be running.

```bash
# Run the worker
make worker
```
Expected output:
```
Connected to MongoDB
Starting ingestion worker...
Worker is running. Press Ctrl+C to stop.
```

---

## Environment Files

```
backend/.env.dev    ← DEV template (committed to git)
backend/.env.prod   ← PROD template (committed to git)
backend/.env        ← YOUR local env (git-ignored, created by make setup)

frontend/.env.dev   ← DEV template
frontend/.env.prod  ← PROD template
frontend/.env.local ← YOUR local env (git-ignored, created by make setup)
```

### backend/.env explained

```env
MODE=DEV            # DEV or PROD — controls Gin mode, CORS, validation
PORT=8080
MONGO_URI=mongodb://localhost:27017   # no auth needed in DEV
DB_NAME=filmorauz
JWT_SECRET=dev-only-secret            # required in PROD, optional in DEV
ADMIN_EMAIL=admin@filmorauz.uz
ADMIN_PASSWORD=admin123               # required in PROD, optional in DEV
ALLOWED_ORIGIN=https://filmorauz.net  # PROD only — your domain for CORS
```

### What MODE changes

| Behavior | DEV | PROD |
|----------|-----|------|
| Gin logs | Verbose | Silent |
| CORS | Allow all origins | Your domain only |
| JWT_SECRET missing | Uses insecure default + warning | **Fatal error** |
| ADMIN_PASSWORD missing | Uses `admin123` + warning | **Fatal error** |
| ALLOWED_ORIGIN missing | Not checked | **Fatal error** |

---

## Production Deployment

### On your server (Ubuntu VPS)

```bash
# 1. Install MongoDB
sudo apt install -y mongodb
sudo systemctl start mongodb

# 2. Install Go and Node
# Go: https://go.dev/dl/
# Node: https://nodejs.org/

# 3. Clone the project
git clone https://github.com/yourname/filmorauz.git
cd filmorauz

# 4. Configure backend
cp backend/.env.prod backend/.env
nano backend/.env   # fill in strong passwords and your domain

# 5. Configure frontend
cp frontend/.env.prod frontend/.env.local
nano frontend/.env.local   # set your API domain

# 6. Build frontend
make build   # outputs to frontend/.next

# 7. Build and run backend
make prod    # compiles to backend/filmorauz-api and runs it

# 8. Run frontend (in another terminal or with pm2)
cd frontend && npm start
```

### Use PM2 to keep processes alive
```bash
npm install -g pm2

# Start backend
pm2 start backend/filmorauz-api --name filmorauz-backend

# Start frontend
pm2 start "npm start" --name filmorauz-frontend --cwd ./frontend

# Auto-start on server reboot
pm2 save
pm2 startup
```

---

## Makefile Commands

```bash
# Development
make setup          # First-time setup: copies .env files, installs all deps
make setup-parser   # Create parser venv and install Python dependencies
make backend        # Run Go backend (DEV mode)
make frontend       # Run Next.js dev server
make parser         # Run Python parser service
make worker         # Run Go worker service

# Production
make build         # Build Next.js for production
make prod          # Build + run Go backend (PROD mode)
make worker-prod   # Build + run Go worker (PROD mode)

# Utilities
make install       # npm install
make tidy          # go mod tidy (backend)
make tidy-worker   # go mod tidy (worker)
```

---

## Automated Deployment (CI/CD)

We use GitHub Actions for automated testing and deployment.

### Required GitHub Secrets
Configure these in **GitHub Settings -> Secrets and variables -> Actions**:

- `WEB_HOST`: IP address/hostname of your Web VPS
- `WEB_USER`: SSH username for Web VPS
- `WEB_SSH_KEY`: Private SSH key for Web VPS (must have access to the server)
- `WEB_PORT`: SSH port (usually 22)
- `WORKER_HOST`: IP address/hostname of your Worker VPS
- `WORKER_USER`: SSH username for Worker VPS
- `WORKER_SSH_KEY`: Private SSH key for Worker VPS
- `WORKER_PORT`: SSH port (usually 22)

### How to use
- **Automatic**: Pushing to the `main` branch triggers the CI pipeline, followed by automated deployments to both servers.
- **Manual**: You can trigger the workflow manually in the GitHub UI (**Actions** -> **CI/CD Pipeline** -> **Run workflow**).
- **Rollback**: To rollback, simply revert the commit in Git and push to `main`. The CI/CD pipeline will automatically deploy the previous state.

---

## Troubleshooting

**MongoDB connection failed**
```bash
# Check if MongoDB is running
sudo systemctl status mongodb    # Linux
brew services list | grep mongo  # macOS

# Start it
sudo systemctl start mongodb     # Linux
brew services start mongodb-community  # macOS
```

**Port 8080 already in use**
```bash
lsof -ti:8080 | xargs kill
# or change PORT= in backend/.env
```

**Go module errors**
```bash
cd backend && go mod tidy
```

**Frontend won't start**
```bash
cd frontend && rm -rf node_modules .next && npm install
```

**Admin user not created**
- The admin is seeded only when `users` collection is empty
- Connect to MongoDB and drop the collection to re-seed:
```bash
mongosh filmorauz --eval "db.users.drop()"
# then restart backend
```
