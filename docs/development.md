# Development

## Prerequisites

- Go 1.26+
- Docker (for testcontainers and k3d/k3s deploy)
- PostgreSQL 16 (local or container)
- git CLI
- [templ](https://templ.guide/) CLI (`go install github.com/a-h/templ/cmd/templ@latest`)

## Setup

```bash
git clone <repo>
cd govnohub
go mod download
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

## Run Backend + Web UI

```bash
export DATABASE_URL=postgres://govnohub:govnohub@localhost:5432/govnohub?sslmode=disable
export JWT_SECRET=dev-secret
export GIT_ROOT=./data/git
export ARTIFACT_ROOT=./data/artifacts
make run-api        # :8080 (API + web UI)
make run-git        # :8081 (separate terminal)
```

Migrations run automatically on api-server startup. Open http://localhost:8080

## Regenerate templ

After editing `.templ` files:

```bash
make generate
```

## Project Layout

```
cmd/           # Service entrypoints
internal/      # Business logic packages
internal/web/  # templ + HTMX web UI
deploy/        # Helm, CRDs, Dockerfiles
tests/e2e/     # End-to-end tests
docs/          # Documentation
scripts/       # Deployment scripts
```

## Adding API Endpoints

1. Add handler in `internal/api/`
2. Register route in `internal/api/server.go`
3. Add integration test in `tests/integration/`
4. Optionally add web page in `internal/web/`

## Adding Web Pages

1. Create or edit `.templ` file in `internal/web/`
2. Run `make generate`
3. Add route + handler in `internal/web/handler.go` and `handlers.go`
