# AGENTS.md

Instructions for AI coding agents working on **Govnohub** — a Kubernetes-native GitHub + GitHub Actions clone written in Go.

## Project Overview

Govnohub is a modular monorepo with multiple deployable Go services, a server-rendered web UI, and Kubernetes-native workflow execution.

| Service | Port | Role |
|---------|------|------|
| `api-server` | 8080 | REST API + templ/HTMX web UI |
| `git-server` | 8081 / 2222 | Smart HTTP + SSH git |
| `webhook-service` | 8082 | Push/PR event dispatcher |
| `actions-controller` | — | Reconciles workflow runs → K8s Jobs |
| `search-indexer` | — | Indexes repos/issues/PRs into OpenSearch |
| `govnohub` CLI | — | REST API client (`cmd/govnohub`) |

**Stack:** Go 1.26+, PostgreSQL 16, OpenSearch, templ + HTMX + Tailwind (embedded), chi router, pgx, testcontainers.

See [docs/architecture.md](docs/architecture.md) for the full system diagram.

## Project Layout

```
cmd/                  # Service entrypoints (api-server, git-server, …)
internal/             # Business logic packages (auth, repo, issue, pull, actions, …)
internal/api/         # REST API handlers + chi router
internal/web/         # templ templates (*.templ) + HTMX handlers
internal/db/migrations/  # SQL migrations (numbered 001_, 002_, …)
deploy/helm/govnohub/ # Helm chart
deploy/crds/          # Workflow, WorkflowRun, RunnerPool CRDs
deploy/docker/        # Multi-service Dockerfile
tests/integration/    # API integration tests (build tag: integration)
tests/e2e/            # Full user-journey tests (build tags: e2e, deploy, k8s)
tests/testenv/        # Shared test server setup
scripts/              # Deploy/install scripts
docs/                 # Architecture, development, testing, deployment guides
```

## Setup

```bash
go mod download
cp .env.example .env
# Start PostgreSQL (see docs/development.md)
make run-api   # :8080
make run-git   # :8081 (separate terminal)
```

Default admin (bootstrapped when no users exist): `admin` / `admin`. Public self-registration is disabled.

## Build & Generate

```bash
make build          # generate templ + build all binaries
make generate       # regenerate *_templ.go from *.templ files
make build-cli      # build govnohub CLI only
```

**Always run `make generate` after editing `.templ` files.** CI runs `make generate && go build ./...` — stale generated files will fail the build.

## Testing

| Suite | Command | Notes |
|-------|---------|-------|
| Unit | `make test-unit` | `internal/...`, uses testcontainers PostgreSQL |
| Integration | `make test-integration` | `-tags=integration` |
| E2E | `make test-e2e` | `-tags=e2e`, 10m timeout |
| Deploy E2E | `make test-deploy-e2e` | `-tags=deploy` |
| K8s smoke | `make test-k8s` | `-tags=k8s` |
| All | `make test-all` | unit + integration + e2e + deploy-e2e |

Docker must be running (testcontainers), or set `TEST_DATABASE_URL` per [docs/testing.md](docs/testing.md).

**Before claiming work is done, run the relevant test suite.** For API changes: at minimum `make test-integration`. For broad changes: `make test-all`.

### Writing Tests

- Unit tests live alongside packages in `internal/*/`.
- Integration/E2E use `tests/testenv.New(t)` + `testutil.RegisterAndLogin`.
- Use build tags: `//go:build integration` or `//go:build e2e`.
- Prefer table-driven tests; use `-race -count=1` (already in Makefile).

## Code Conventions

### Go

- **Keep code simple** — prefer the smallest correct solution; avoid over-abstraction, unnecessary helpers, and excessive error handling for unlikely edge cases.
- Standard library + existing deps; no new dependencies without good reason.
- Business logic in `internal/<domain>/` as `Service` structs with `pgxpool.Pool`.
- Export sentinel errors: `var ErrNotFound = errors.New("…")`.
- Pass `context.Context` as first parameter to DB/service methods.
- Use `uuid.UUID` for IDs; JSON tags on exported structs.
- Keep handlers thin — delegate to `internal/` services.
- API helpers: `jsonOK`, `jsonError` in `internal/api/server.go`.
- Match existing formatting; don't reformat unrelated code.

### API Endpoints

1. Add handler in `internal/api/` (or extend existing handler file by domain).
2. Register route in `internal/api/server.go`.
3. Add integration test in `tests/integration/`.
4. Optionally add web page in `internal/web/`.

### Web UI

1. Create/edit `.templ` file in `internal/web/`.
2. Run `make generate`.
3. Add route in `internal/web/handler.go`, handler in `handlers.go`.
4. Use HTMX for partial updates; session cookies for browser auth.
5. Static assets in `internal/web/static/` (embedded via `go:embed`).

### Database Migrations

- Add numbered SQL file in `internal/db/migrations/` (e.g. `008_feature.sql`).
- Migrations run automatically on `api-server` startup via `db.Migrate`.
- Keep migrations idempotent where possible (`IF NOT EXISTS`, `ON CONFLICT`).

### Actions / Kubernetes

- Workflow YAML parsing in `internal/actions/`.
- Controller creates Jobs in `govnohub-runners` namespace.
- CRDs in `deploy/crds/`; Helm values in `deploy/helm/govnohub/`.

## Common Tasks

### Add a new domain feature

1. Service package in `internal/<name>/` with `NewService(pool)` constructor.
2. Wire into `cmd/api-server/main.go` and `internal/api/server.go`.
3. SQL migration if schema changes needed.
4. Unit tests in the package; integration test for API surface.
5. Web UI in `internal/web/` if user-facing.

### Fix a bug

1. Reproduce with a failing test (preferred) or describe steps.
2. Fix in the smallest scope — usually `internal/<domain>/`.
3. Run targeted tests, then broader suite if touching shared code.

### Deploy locally (K8s)

```bash
make k3d-create       # or make podman-k8s-create
make deploy-k3s
# Add to /etc/hosts: 127.0.0.1 govnohub.local git.govnohub.local
```

## CI

GitHub Actions (`.github/workflows/ci.yml`) runs:

1. `make generate`
2. `make test-unit`, `make test-integration`, `make test-e2e`, `make test-deploy-e2e`
3. `make build`
4. Helm template validation
5. Full K8s E2E on kind (separate job)

Match CI locally before opening a PR.

## Security

- Never commit secrets (`.env`, tokens, keys). Use `.env.example` for templates.
- `JWT_SECRET` and `BOOTSTRAP_ADMIN_PASSWORD` must be changed in production.
- PATs use `ghp_` prefix; SSH keys managed via settings.
- CSRF protection on web forms (`internal/web/csrf.go`).
- Admin routes require `requireAdmin` middleware.

## Agent Guidelines

### Do

- Read existing code in the target package before changing it.
- Keep code simple and concise.
- Keep diffs minimal and focused on the task.
- Run `make generate` when `.templ` files change.
- Run relevant tests and report results.
- Follow patterns in neighboring files (naming, error handling, routing).
- Consult `docs/` for architecture and deployment details.

### Don't

- Don't add unrelated refactors, formatting sweeps, or new dependencies.
- Don't edit `*_templ.go` files directly — they are generated.
- Don't skip tests for API or schema changes.
- Don't enable public registration without explicit request (`ALLOW_PUBLIC_REGISTRATION`).
- Don't create commits or PRs unless asked.

## Documentation

| Doc | Contents |
|-----|----------|
| [docs/architecture.md](docs/architecture.md) | System design, data flow, K8s resources |
| [docs/development.md](docs/development.md) | Local dev setup, project layout |
| [docs/testing.md](docs/testing.md) | Test suites, testenv usage |
| [docs/deployment.md](docs/deployment.md) | k3s/k3d/kind deployment |
| [docs/cli.md](docs/cli.md) | `govnohub` CLI commands |
| [docs/git.md](docs/git.md) | Git server protocol details |
