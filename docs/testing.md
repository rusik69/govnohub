# Testing

## Test Suites

| Suite | Command | Description |
|-------|---------|-------------|
| Unit | `make test-unit` | All `internal/` packages with testcontainers PostgreSQL |
| Integration | `make test-integration` | API + web handlers via httptest (`-tags=integration`) |
| E2E | `make test-e2e` | Full user journey (`-tags=e2e`) |
| Deploy E2E | `make test-deploy-e2e` | Real api-server + git-server (`-tags=deploy`) |
| K8s smoke | `make test-k8s` | Helm template + optional cluster check (`-tags=k8s`) |
| All | `make test-all` | Unit + integration + e2e + deploy-e2e |

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

- `auth` — register, login, JWT, PAT, SSH keys
- `repo` — create, access, star, unstar, watch, unwatch, fork
- `git` — init repo, commits
- `issue`, `pull`, `release`, `package`, `org`
- `actions` — YAML parser, triggers, job ordering
- `webhook`, `search`, `config`, `db`
- `web` — `isBinaryContent`, `redirectReferer`
- `cmd/git-server` — SSH command/env parsing

## Integration Tests

`tests/integration/` (build tag `integration`):

| File | Coverage |
|------|----------|
| `api_test.go` | Health, register/login/me, create repo and issue |
| `protection_test.go` | Branch protection merge gate, collaborators |
| `admin_test.go` | Admin create user, forbid public register |
| `extras_test.go` | Release assets, webhooks |
| `branches_test.go` | List repo branches API |
| `web_handlers_test.go` | Repo branch picker, issue create form |

## E2E Tests

### `-tags=e2e` (CI, `make test-e2e`)

Uses `testenv.New` — httptest API server + testcontainers PostgreSQL.

| File | Tests |
|------|-------|
| `api_flow_test.go` | `TestFullUserJourney` — smoke: repo, issue, PR, workflow, release, star, collaborator, protection |
| `auth_flow_test.go` | PAT create/list/revoke; SSH key create/list/delete |
| `admin_flow_test.go` | Admin user CRUD/delete, non-admin/PAT rejection, audit log |
| `health_test.go` | `GET /healthz`, `GET /api/v1/user` |
| `issue_flow_test.go` | Issue comments, labels, milestones, patch, close |
| `issue_read_test.go` | GET issue, list comments/labels, remove label, patch assignee |
| `pr_flow_test.go` | PR review + merge with branch protection; PR comments and diff |
| `pr_read_test.go` | List/get PRs, list reviews, squash merge |
| `actions_flow_test.go` | Workflow upsert, trigger run, list runs, fetch logs |
| `release_package_test.go` | Release asset upload/download; package publish/download |
| `releases_list_test.go` | `GET .../releases` |
| `repo_extra_test.go` | Wiki, webhooks, star/watch/branches/fork/search, protected-branches list |
| `org_collab_test.go` | Org members/teams/repos; collaborator add/remove |
| `notifications_test.go` | Issue comment triggers notification; mark read |
| `notification_types_test.go` | PR review and issue close notifications |
| `ai_review_test.go` | AI review config disabled; create returns 503 |
| `web_flow_test.go` | Web login/logout, dashboard repo create, star/watch, issue create, notifications, branches |

Shared helpers in `helpers.go` (`e2e || deploy` build tag): `adminToken`, `webClient`, `webCSRF`, `webPostForm`, `assertSearchHit`, etc.

### `-tags=deploy` (`make test-deploy-e2e`)

Uses `testenv.NewLocalDeploy` — real api-server + git-server processes.

| File | Tests |
|------|-------|
| `deploy_flow_test.go` | Local deploy health; full journey smoke (git push, PAT, actions, packages, orgs, search) |
| `git_flow_test.go` | Git HTTP/SSH clone, push, pull, fetch, feature branch, PAT auth, wrong credentials |
| `deploy_admin_test.go` | Admin audit log, admin user list |

Optional cluster test: set `GOVNOHUB_DEPLOY_E2E=1` and `GOVNOHUB_BASE_URL` for `TestDeployedClusterFullJourney`.

## K8s Smoke Tests

`tests/e2e/k8s_smoke_test.go` (build tag `k8s`):

- Verifies kubectl connectivity
- Validates `helm template` renders
- Optional deployed pod check with `GOVNOHUB_K8S_E2E=1` (hits `/healthz` and admin `GET /api/v1/user`)

## CI

GitHub Actions workflow (`.github/workflows/ci.yml`) runs:

**`backend` job:** `make generate` → `make test-unit` → `make test-integration` → `make test-e2e` → `make test-deploy-e2e` → `make build`

**`helm` job:** `helm template` validation

**`k8s-e2e` job:** kind cluster deploy + `make test-k8s-e2e`

## Writing New Tests

Use `tests/testenv.New(t)` for a full stack with PostgreSQL + API server:

```go
env := testenv.New(t)
defer env.Cleanup()
token := testutil.RegisterAndLogin(t, env.URL, "user")
```
