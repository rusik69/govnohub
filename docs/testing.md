# Testing

## Test Suites

| Suite | Command | Description |
|-------|---------|-------------|
| Unit | `make test-unit` | All `internal/` packages with testcontainers PostgreSQL |
| Integration | `make test-integration` | API handlers via httptest (`-tags=integration`) |
| E2E | `make test-e2e` | Full user journey (`-tags=e2e`) |
| K8s smoke | `make test-k8s` | Helm template + optional cluster check (`-tags=k8s`) |
| Frontend | `make test-frontend` | Vitest unit tests |
| All | `make test-all` | Runs all of the above |

## Requirements

- Docker running (testcontainers spins up PostgreSQL automatically), **or**
- External PostgreSQL via `TEST_DATABASE_URL`:

```bash
./scripts/start-test-db.sh
export TEST_DATABASE_URL=postgres://govnohub:govnohub@localhost:5433/govnohub?sslmode=disable
make test-all
```

- Go 1.26+
- For k8s smoke with deployed cluster: `GOVNOHUB_K8S_E2E=1 make test-k8s`

## Unit Tests

Located alongside packages in `internal/*/`:

- `auth` — register, login, JWT, PAT
- `repo` — create, access, star, fork
- `git` — init repo, commits
- `issue`, `pull`, `release`, `package`, `org`
- `actions` — YAML parser, triggers, job ordering
- `webhook`, `search`, `config`, `db`

## Integration Tests

`tests/integration/api_test.go` (build tag `integration`):

- Health check
- Register/login/me flow
- Create repo and issue

## E2E Tests

`tests/e2e/api_flow_test.go` (build tag `e2e`):

Full journey: register → create repo → issue → PR → workflow → release → star → close issue

## K8s Smoke Tests

`tests/e2e/k8s_smoke_test.go` (build tag `k8s`):

- Verifies kubectl connectivity
- Validates `helm template` renders
- Optional deployed pod check with `GOVNOHUB_K8S_E2E=1`

## Frontend Tests

```bash
cd frontend && npm run test
```

Tests auth store and API client helpers.

## CI

GitHub Actions workflow (`.github/workflows/ci.yml`) runs:

1. `make test-unit`
2. `make test-integration`
3. `make test-e2e`
4. `make test-frontend`
5. `go build ./...`
6. `cd frontend && npm run build`

## Writing New Tests

Use `tests/testenv.New(t)` for a full stack with PostgreSQL + API server:

```go
env := testenv.New(t)
defer env.Cleanup()
token := testutil.RegisterAndLogin(t, env.URL, "user")
```
