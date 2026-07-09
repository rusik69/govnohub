# Development

## Prerequisites

- Go 1.26+
- Node.js 22+
- Docker (for testcontainers and k3d/k3s deploy)
- PostgreSQL 16 (local or container)
- git CLI

## Setup

```bash
git clone <repo>
cd govnohub
go mod download
cd frontend && npm install
cp .env.example .env
```

## Run PostgreSQL

```bash
docker run -d --name govnohub-pg \
  -e POSTGRES_USER=govnohub \
  -e POSTGRES_PASSWORD=govnohub \
  -e POSTGRES_DB=govnohub \
  -p 5432:5432 \
  postgres:16-alpine
```

## Run Backend

```bash
export DATABASE_URL=postgres://govnohub:govnohub@localhost:5432/govnohub?sslmode=disable
export JWT_SECRET=dev-secret
export GIT_ROOT=./data/git
export ARTIFACT_ROOT=./data/artifacts
make run-api        # :8080
make run-git        # :8081 (separate terminal)
```

Migrations run automatically on api-server startup.

## Run Frontend

```bash
cd frontend
npm run dev   # http://localhost:5173, proxies /api to :8080
```

## Project Layout

```
cmd/           # Service entrypoints
internal/      # Business logic packages
frontend/      # Vue 3 SPA
deploy/        # Helm, CRDs, Dockerfiles
tests/e2e/     # End-to-end tests
docs/          # Documentation
scripts/       # Deployment scripts
```

## Adding API Endpoints

1. Add handler in `internal/api/`
2. Register route in `internal/api/server.go`
3. Add client method in `frontend/src/api/client.ts`
4. Add integration test in `internal/api/server_test.go`
