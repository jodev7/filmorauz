ROOT_DIR := $(dir $(realpath $(lastword $(MAKEFILE_LIST))))
PARSER_VENV := $(ROOT_DIR)parser/venv
PYTHON := $(shell command -v python3 2>/dev/null || command -v python 2>/dev/null || echo "python3")

# Determine Python executable: use venv if exists, otherwise system python
ifeq ($(wildcard $(PARSER_VENV)/bin/python),)
    PARSER_PYTHON := $(PYTHON)
else
    PARSER_PYTHON := $(PARSER_VENV)/bin/python
endif

.PHONY: help dev prod backend frontend parser worker bot bot-prod install tidy tidy-bot tidy-worker build setup-parser setup-bot b2-cors b2-cors-apply b2-cors-probe

help:
	@echo ""
	@echo "  FilmoraUz — Commands"
	@echo ""
	@echo "  DEVELOPMENT (local)"
	@echo "    make setup          First-time setup (copies .env files, installs deps)"
	@echo "    make setup-parser   Create parser virtualenv and install dependencies"
	@echo "    make backend        Run Go backend (reads backend/.env)"
	@echo "    make frontend      Run Next.js dev server"
	@echo "    make parser        Run Python parser service"
	@echo "    make worker        Run Go worker service"
	@echo ""
	@echo "  PRODUCTION (server)"
	@echo "    make build         Build Next.js for production"
	@echo "    make prod         Run backend in production mode"
	@echo "    make worker-prod  Run worker in production mode"
	@echo ""
	@echo "  UTILITIES"
	@echo "    make install       npm install frontend"
	@echo "    make tidy          go mod tidy backend"
	@echo "    make tidy-bot      go mod tidy bot"
	@echo "    make tidy-worker   go mod tidy worker"
	@echo ""

# ── First-time setup ──────────────────────────────────────────
setup: setup-parser setup-bot
	@echo "Setting up for local development..."
	@if [ ! -f $(ROOT_DIR)backend/.env ]; then \
		cp $(ROOT_DIR)backend/.env.dev $(ROOT_DIR)backend/.env; \
		echo "  Created backend/.env from .env.dev"; \
	else \
		echo "  backend/.env already exists, skipping"; \
	fi
	@if [ ! -f $(ROOT_DIR)frontend/.env.local ]; then \
		cp $(ROOT_DIR)frontend/.env.dev $(ROOT_DIR)frontend/.env.local; \
		echo "  Created frontend/.env.local from .env.dev"; \
	else \
		echo "  frontend/.env.local already exists, skipping"; \
	fi
	cd $(ROOT_DIR)frontend && npm install
	cd $(ROOT_DIR)backend && go mod tidy
	@echo ""
	@echo "  Done! Now run:"
	@echo "    Terminal 1: make backend"
	@echo "    Terminal 2: make parser"
	@echo "    Terminal 3: make worker"
	@echo "    Terminal 4: make frontend"
	@echo "    Terminal 5: make bot"
	@echo ""

# ── Bot setup ───────────────────────────────────────────────
setup-bot:
	@echo "Setting up bot..."
	@if [ ! -f $(ROOT_DIR)bot/.env ]; then \
		cp $(ROOT_DIR)bot/.env $(ROOT_DIR)bot/.env.local; \
		echo "  Created bot/.env.local (copy bot/.env and configure)"; \
	else \
		echo "  bot/.env already exists"; \
	fi
	cd $(ROOT_DIR)bot && go mod download
	@echo "  Bot setup complete!"

# ── Parser virtualenv setup ─────────────────────────────────────
setup-parser:
	@echo "Setting up parser virtualenv..."
	@if [ ! -d "$(PARSER_VENV)" ]; then \
		echo "  Creating virtualenv at parser/venv..."; \
		$(PYTHON) -m venv $(PARSER_VENV); \
		echo "  Virtualenv created"; \
	else \
		echo "  Virtualenv already exists at parser/venv"; \
	fi
	@echo "  Installing dependencies..."
	$(PARSER_VENV)/bin/pip install --upgrade pip
	$(PARSER_VENV)/bin/pip install -r $(ROOT_DIR)parser/requirements.txt
	@echo "  Parser setup complete!"

# ── Development ───────────────────────────────────────────────
backend:
	cd $(ROOT_DIR)backend && go run main.go

frontend:
	cd $(ROOT_DIR)frontend && npm run dev

parser:
	# Parser service - loads BACKEND_URL from parser/.env automatically
	cd $(ROOT_DIR)parser && $(PARSER_PYTHON) server.py

worker:
	cd $(ROOT_DIR)worker && go mod download && go mod tidy && go build && go run .

yusuf:
	cd $(ROOT_DIR)backend && go mod tidy && go mod download && cd $(ROOT_DIR)worker && go mod tidy && go mod download && cd $(ROOT_DIR)bot && go mod tidy && go mod download && cd $(ROOT_DIR) parser && pip install -r $(ROOT_DIR)parser/requirements.txt && cd $(ROOT_DIR)frontend && npm install

# ── Backblaze B2 CORS ─────────────────────────────────────────
# Inspect current CORS rules on the bucket (dry-run).
b2-cors:
	cd $(ROOT_DIR)backend && go run ./cmd/b2-cors

# Apply the CORS rules required for browser-to-B2 direct uploads AND for HLS
# playback of .m3u8 / .ts through cdn.filmorauz.net.
# Allowed origins default to BASE_SITE_URL + its www variant + http://localhost:3000.
# Override with: make b2-cors-apply ORIGIN=https://a.tld,https://b.tld
b2-cors-apply:
	cd $(ROOT_DIR)backend && go run ./cmd/b2-cors --apply $(if $(ORIGIN),--origin $(ORIGIN))

# Probe a CDN URL for CORS response headers. Diagnoses whether B2 or
# Cloudflare is the one dropping Access-Control-Allow-Origin.
# Example:
#   make b2-cors-probe URL=https://cdn.filmorauz.net/file/filmorauznet/videos/<slug>/master.m3u8
b2-cors-probe:
	@if [ -z "$(URL)" ]; then echo "Usage: make b2-cors-probe URL=<cdn-url>"; exit 2; fi
	cd $(ROOT_DIR)backend && go run ./cmd/b2-cors --probe $(URL)

# ── Bot ────────────────────────────────────────────────────────
bot:
	cd $(ROOT_DIR)bot && go run main.go

bot-prod:
	cd $(ROOT_DIR)bot && go build -o filmorauz-bot . && ./filmorauz-bot

# ── Production ────────────────────────────────────────────────
build:
	cd $(ROOT_DIR)frontend && npm run build

prod:
	cd $(ROOT_DIR)backend && go build -o filmorauz-api . && ./filmorauz-api

worker-prod:
	cd $(ROOT_DIR)worker && go build -o filmorauz-worker . && ./filmorauz-worker

# ── Utilities ─────────────────────────────────────────────────
install:
	cd $(ROOT_DIR)frontend && npm install

tidy:
	cd $(ROOT_DIR)backend && go mod tidy

tidy-bot:
	cd $(ROOT_DIR)bot && go mod tidy

tidy-worker:
	cd $(ROOT_DIR)worker && go mod tidy

vps-pull-worker:
	cd /opt/filmorauz/worker/ && git pull && go mod download && go mod tidy && go build . && systemctl restart filmorauz-worker && systemctl restart filmorauz-parser

vps-pull-web:
	cd /opt/filmorauz/backend && git pull && go mod download && go mod tidy && go build . && cd /opt/filmorauz/bot/ && go mod download && go mod tidy && go build . && systemctl restart filmorauz-backend && systemctl restart filmorauz-bot && cd /opt/filmorauz/frontend && npm run build && pm2 restart all