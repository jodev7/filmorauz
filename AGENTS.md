# Repository Guidelines

## Project Structure & Module Organization

FilmoraUz is a streaming platform.

- `frontend/`: Next.js 14 public site, watch pages, and admin UI. Source is in `frontend/src/app`, components in `frontend/src/components`, shared clients in `frontend/src/lib`, assets in `frontend/public`.
- `backend/`: Go Gin API with `handlers/`, `services/`, `repositories/`, `models/`, `middleware/`, `routes/`, and `config/`.
- `worker/`: Go video ingestion/processing service. Pipeline code is in `worker/pipeline`, storage in `worker/storage`, watermark tooling in `worker/watermark_removal`.
- `parser/`: Python parser/downloader service for external sources and social publishing helpers.
- `bot/`: Go Telegram bot for subscriptions, login deep links, and movie code lookup.

## Build, Test, and Development Commands

- `make setup`: create local env files, install frontend dependencies, tidy backend modules, and set up parser/bot dependencies.
- `make backend`: run the Go API on port `8080`.
- `make frontend`: run Next.js dev server on port `3000`.
- `make parser`: run the Python parser service on port `8082`.
- `make worker`: run the Go worker.
- `make bot`: run the Telegram bot.
- `make build`: production build for the frontend.
- `cd frontend && npm run lint`: run Next linting.
- `cd backend && go test ./...`, `cd worker && go test ./...`, `cd bot && go test ./...`: run Go tests.

## Coding Style & Naming Conventions

Use `gofmt`/`go fmt ./...` for Go and keep package names lowercase. Follow layered naming such as `MovieHandler`, `MovieService`, `MovieRepository`, and `movie.go`. Frontend code uses TypeScript, React functional components, PascalCase component filenames, and camelCase utilities. Python parser modules and functions use snake_case.

## Testing Guidelines

There are currently no committed test files in the main services. Add focused tests for business logic, repositories, auth, ingestion state transitions, and video pipeline behavior. Use Go `*_test.go` files near the package under test. For frontend changes, run `npm run lint` and `npm run build`; add component tests if a framework is introduced.

## Commit & Pull Request Guidelines

Recent history contains mostly placeholder subjects, so use clear imperative commits such as `fix worker job retry state` or `add admin series approval filter`. PRs should include a summary, affected services, commands run, linked issue/task, and screenshots for UI changes. Note required `.env` changes or migrations.

## Security & Configuration Tips

Do not commit real tokens, cookies, OAuth secrets, session files, downloaded media, or build artifacts. Treat `.env`, `parser/*sessions`, `parser/cookies.txt`, `parser/client_secret.json`, `worker/uploads`, `worker/tmp`, and `frontend/.next` as local/runtime data unless explicitly required.
