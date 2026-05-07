# FilmoraUz

FilmoraUz is a movie streaming platform with integrated metadata parsing and automated video ingestion/processing. The architecture is modular, consisting of a Go-based backend and worker service, a Next.js frontend, a Python-based parser for movie/series metadata, and a Go-based telegram bot.

## Technology Stack

- **Backend**: Go (Gin framework), MongoDB.
- **Frontend**: Next.js, React, Tailwind CSS.
- **Parser**: Python (BeautifulSoup, requests, yt-dlp).
- **Worker**: Go (ingestion and processing tasks).
- **Telegram Bot**: Go.

## Development & Usage

### Setup
Use the Makefile for unified project management:
```bash
make setup          # First-time initialization
make setup-parser   # Parser-specific dependencies
```

### Running Services
The project uses multiple processes. Run each in a separate terminal:
```bash
make backend        # Go Backend
make frontend       # Next.js Frontend
make parser         # Python Parser Service
make worker         # Go Ingestion Worker
```

### CI/CD Pipeline
The project uses GitHub Actions for automated testing and deployment. Pushes to `main` trigger CI checks and redeployments to defined Web and Worker VPS instances.

## Conventions

- **Environment Files**: The project uses `.env.dev` templates. Run `make setup` to generate local `.env` files which are ignored by Git.
- **Deployment**: Manual deployments are managed via Makefile targets `vps-pull-web` and `vps-pull-worker`.
- **Database**: MongoDB is the primary data store.
