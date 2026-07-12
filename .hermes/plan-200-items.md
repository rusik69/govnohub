# Govnohub 200-Item Improvement Plan

> Kubernetes-native GitHub + GitHub Actions clone — 101 Go source files, 45 test files,
> 29K+ lines. 5 microservices + CLI + templ/HTMX web UI.
> Generated 2026-07-12 from full codebase audit.

---

## 1. TESTING COVERAGE (40 items)

### Unit Tests — New Files

1. **`internal/repo/repo_test.go`** — Add table-driven tests for `Create`, `GetByFullName`, `ListForUser`, `ListForOrg`, `Fork`, `Star`/`Unstar`, `CanAccess` (public/private/collaborator/org-member), `ResolveOwnerID`, `UpdateBranchHead`
2. **`internal/auth/auth_test.go`** — Tests for `Register`, `Login`, `CreatePAT`, `ValidateToken`, `ValidatePATWithScopes`, `HasScope`, `IsAdmin`, `BootstrapAdmin`, `DeleteUser`
3. **`internal/git/store_test.go`** — Tests for `Init`, `Exists`, `SeedMainBranch`, `GetTree`, `GetBlob`, `GetCommits`, `CreateBranch`, `Diff`, `Merge`, `CanMerge`, `ListBranchSHAs`
4. **`internal/actions/actions_test.go`** — Tests for `ParseWorkflow`, `MatchesTrigger` with all event types (string/array/map), `JobOrder` with DAG dependencies, `EvalExpression` edge cases (nested, missing, empty), `NormalizeNeeds` (string/array/nil), `BuildStepScript`, `WriteJobScript`
5. **`internal/org/org_test.go`** — Cover `Create`, `AddMember`, `IsMember`, `ListForUser`, `ListMembers`, `ListTeams`, `CreateTeam`, `AddTeamMember`
6. **`internal/pull/pull_test.go`** — Cover `Create`, `GetByNumber`, `List`, `Merge`, `AddReview`, `ListReviews`, `CheckMergeable`
7. **`internal/issue/issue_test.go`** — Cover `List`, `GetByNumber`, `Create`, `Close`, `Patch`, `AddComment`, `ListComments`, label operations
8. **`internal/webhook/webhook_test.go`** — Cover `Create`, `Dispatch`, `ListDeliveries`, HMAC signing
9. **`internal/notification/notification_test.go`** — Create if it exists, expand coverage
10. **`internal/release/release_test.go`** — Cover `Create`, `List`, `UploadAsset`, `ListAssets`, `DownloadAsset`
11. **`internal/package/pkg_test.go`** — Cover `Publish`, `List`, `Download`
12. **`internal/search/search_test.go`** — Mock HTTP tests for `Index`, `Search`, `EnsureIndex`
13. **`internal/wiki/wiki_test.go`** — Cover CRUD operations, slug generation
14. **`internal/audit/audit_test.go`** — Cover log creation, listing, filtering

### Unit Tests — Expand Existing

15. **`internal/repo/repo_test.go`** (existing) — Add tests for edge cases: duplicate name, empty description, deleted owner
16. **`internal/actions/actions_test.go`** (existing) — Add matrix strategy tests, `needs` with arrays, `if:` conditional skip
17. **`internal/cli/config_test.go`** (existing) — Merge flags precedence, missing token error
18. **`internal/web/render_test.go`** (existing) — Cover more template rendering paths

### Integration Tests

19. **`tests/integration/repo_test.go`** — Repo CRUD via API, access control scenarios
20. **`tests/integration/issue_test.go`** — Issue CRUD, comments, labels, milestones via API
21. **`tests/integration/pr_test.go`** — Full PR lifecycle: create, review, comment, merge
22. **`tests/integration/auth_test.go`** — Login, register, PAT create/validate/revoke, scope enforcement
23. **`tests/integration/search_test.go`** — Index + search with testcontainers OpenSearch
24. **`tests/integration/org_test.go`** — Org CRUD, member management, team operations
25. **`tests/integration/actions_test.go`** — Workflow parse, trigger, run lifecycle
26. **`tests/integration/release_test.go`** — Release create, asset upload/download
27. **`tests/integration/webhook_test.go`** — Webhook CRUD, delivery inspection
28. **`tests/integration/wiki_test.go`** — Wiki CRUD via API
29. **`tests/integration/notification_test.go`** — Notifications CRUD via API
30. **`tests/integration/admin_test.go`** (expand) — User management, audit log

### E2E Tests — New Coverage

31. **`tests/e2e/search_flow_test.go`** — End-to-end search for repo, issue, PR content
32. **`tests/e2e/label_flow_test.go`** — Label CRUD, assign/remove on issues
33. **`tests/e2e/milestone_flow_test.go`** — Milestone CRUD, issue assignment
34. **`tests/e2e/package_flow_test.go`** — Package publish + download flow
35. **`tests/e2e/team_flow_test.go`** — Team-based permission scenarios (org repo→team→member)
36. **`tests/e2e/fork_flow_test.go`** — Fork repo, push to fork, create PR from fork
37. **`tests/e2e/wiki_flow_test.go`** — Wiki full lifecycle: create, edit, delete pages
38. **`tests/e2e/rate_limit_test.go`** — Rate limiting enforcement (if implemented)

### Test Infrastructure

39. Add race-free test helpers — common `requireOK`, `requireStatus` for integration tests
40. Add GitHub Actions matrix for parallel test suites to reduce CI wall time

---

## 2. MISSING API ENDPOINTS (20 items)

### Issue & PR API

41. Add `PATCH /api/v1/repos/{owner}/{repo}` — Update repo metadata (description, private, default_branch)
42. Add `DELETE /api/v1/repos/{owner}/{repo}` — Delete/archive repo
43. Add `GET /api/v1/repos/{owner}/{repo}/issues/{number}/timeline` — Issue timeline (events)
44. Add `PUT /api/v1/repos/{owner}/{repo}/issues/{number}/assignees` — Batch assign
45. Add `POST /api/v1/repos/{owner}/{repo}/issues/{number}/reactions` — Reaction emoji on issues/comments
46. Add `GET /api/v1/repos/{owner}/{repo}/pulls/{number}/commits` — List PR commits
47. Add `GET /api/v1/repos/{owner}/{repo}/pulls/{number}/files` — List PR changed files
48. Add `PUT /api/v1/repos/{owner}/{repo}/pulls/{number}/update-branch` — Update PR branch

### Repo API

49. Add `GET /api/v1/repos/{owner}/{repo}/tags` — List git tags
50. Add `POST /api/v1/repos/{owner}/{repo}/tags` — Create tag
51. Add `GET /api/v1/repos/{owner}/{repo}/compare/{base}...{head}` — Compare two commits
52. Add `GET /api/v1/repos/{owner}/{repo}/archive/{ref}.tar.gz` — Download source archive
53. Add `POST /api/v1/repos/{owner}/{repo}/git/commits` — Create commit via API (Git Data API)
54. Add `POST /api/v1/repos/{owner}/{repo}/git/blobs` — Create blob via API (Git Data API)
55. Add `POST /api/v1/repos/{owner}/{repo}/git/trees` — Create tree via API (Git Data API)
56. Add `POST /api/v1/repos/{owner}/{repo}/git/refs` — Create/update git refs

### Admin & Meta API

57. Add `GET /api/v1/rate_limit` — Rate limit status endpoint
58. Add `GET /api/v1/meta` — Server metadata (version, capabilities, auth methods)
59. Add `GET /api/v1/events` — Public event timeline
60. Add `GET /api/v1/repos/{owner}/{repo}/actions/secrets` — Actions secrets CRUD

---

## 3. WEB UI IMPROVEMENTS (25 items)

### Missing Pages

61. Add user profile page (`/user/{username}`) with avatar, bio, repo list, activity
62. Add search results page with filters (repo/issue/PR/wiki), sort, pagination
63. Add commit detail view: full diff, file tree, author info
64. Add compare view: branch/commit comparison with file-by-file diff
65. Add tag listing page with tag creation UI
66. Add repo fork dialog with target namespace selection
67. Add repository settings pages: danger zone (delete repo), branch protection rules UI

### UX Improvements

68. Add pagination / infinite scroll for issue lists (currently could grow unbounded)
69. Add Markdown preview toggle on issue/PR comment forms
70. Add loading states & skeleton screens for slow API responses
71. Add toast/notification drawer for real-time events (new PR, review requested)
72. Add dark mode CSS with theme toggle
73. Add responsive/mobile-friendly layout (currently desktop-first)
74. Add keyboard shortcuts: `g i` → issues, `g p` → PRs, `c` → new issue
75. Add file finder modal (`t` on repo page like GitHub)
76. Add blame view on repo blob pages
77. Add commit SHA copy button and permalink generation
78. Add user-mention autocomplete (`@username`) in issue/PR/comment forms
79. Add task-list checkbox rendering (`- [ ]`) in Markdown
80. Add emoji autocomplete (`:smile:` → 😄) in comment forms
81. Add syntax highlighting for code blocks in Markdown (Prism.js or highlight.js)
82. Add CI status badges rendered on repo README
83. Add inline image paste support in issue/PR body
84. Add real-time collaboration indicators (who's viewing)
85. Add time-ago relative timestamps with tooltip showing exact time

---

## 4. SECURITY (15 items)

86. Add CSRF token rotation per-session (currently static per-form)
87. Add rate limiting middleware (per-IP, per-user, per-endpoint)
88. Add brute-force protection on login endpoint (account lockout after N failures)
89. Add SQL injection audit — ensure all user input uses parameterized queries (mostly done, but audit)
90. Add PAT scope introspection endpoint so tokens can be validated client-side
91. Add PAT expiry notification (warn users before token expires)
92. Add audit events for sensitive operations (role change, repo delete, user delete)
93. Add branch protection enforcement in git-server (prevent push to protected branches server-side)
94. Add required status checks enforcement for protected branches before merge
95. Add GPG signature verification display on commits
96. Add 2FA / TOTP support for user accounts
97. Add session invalidation on password change
98. Add CORS origin validation (currently `*` for all origins)
99. Add `Content-Security-Policy` header on web pages
100. Add secret scanning on push (prevent committing secrets to repos)

---

## 5. PERFORMANCE & SCALABILITY (15 items)

101. Add DB connection pooling tuning — configurable `max_conns` per service
102. Add OpenSearch connection retry with exponential backoff (currently errors silently swallowed)
103. Add pagination on all `List*` API endpoints (limit/offset or cursor-based)
104. Add in-memory caching for frequently accessed data (repo lookups, user lookups)
105. Add git pack-objects caching for popular repos to reduce CPU on clone
106. Add lazy loading for repo tree on large repos (directory-level only, expand on click)
107. Add background job queue for heavy operations (git archive generation, large merge)
108. Add database index review — run `EXPLAIN ANALYZE` on all query patterns
109. Add connection pooling between services (HTTP keep-alive, limiting idle connections)
110. Add asset compression middleware (gzip/brotli) for web UI and API responses
111. Add concurrent git operation limiting per-repo to prevent resource starvation
112. Add Prometheus metrics endpoint (`/metrics`) for Go runtime, DB pool, request duration
113. Add distributed tracing with OpenTelemetry context propagation across services
114. Add OpenSearch bulk indexing instead of per-document HTTP calls
115. Add horizontal pod autoscaler (HPA) configuration for each service

---

## 6. CODE QUALITY & REFACTORING (20 items)

116. Dry up repeated SQL query patterns — consolidate common column selections into a helper
117. Replace magic strings with typed constants (e.g., permission levels, event types, states)
118. Add `context.Context` propagation consistently through all service methods (patch gaps)
119. Add structured error types throughout `internal/` packages for better error wrapping
120. Move shared test helpers from `tests/e2e/helpers.go` into shared `tests/testutil` package
121. Add `internal/middleware/` package for shared HTTP middleware (logging, auth, rate-limit, tracing)
122. Split `internal/api/server.go` — currently 986 lines, break into domain-specific router files
123. Split `internal/web/handlers.go` — currently 1308 lines, break into domain files
124. Remove raw `exec.Command("git", ...)` in git store — consolidate into a `gitRun` helper
125. Add `goimports` / `golangci-lint` to CI and Makefile
126. Add pre-commit hooks config for formatting and linting
127. Add `internal/render/` helper package to unify JSON response patterns (currently duplicated in API + web)
128. Extract Helm chart helpers into proper `_helpers.tpl` (currently minimal)
129. Add `go:build` comments consistently on all test files (many integration tests lack tags)
130. Consolidate `_templ.go` generation into a single CI step (already done, but add `go generate` marker)
131. Move `cmd/*/main.go` into `cmd/<service>/main.go with consistent pattern (some mix flag parsing + wiring)
132. Add option struct pattern for service constructors instead of positional parameters (auth already has this, others don't)
133. Add Dependabot config for Go module updates and Docker image updates
134. Add `Taskfile.yml` or `Justfile` as alternative to Makefile for cross-platform dev
135. Clean up unused indirect dependencies in `go.mod`

---

## 7. INFRASTRUCTURE & DEPLOYMENT (15 items)

136. Add Helm chart liveness/readiness probes for all services
137. Add Helm chart resource limits (CPU/memory requests and limits)
138. Add Helm chart PodDisruptionBudget for HA deployments
139. Add Helm chart NetworkPolicy for service isolation
140. Add Helm chart ServiceMonitor for Prometheus Operator
141. Add Helm chart tests/pod for post-deploy validation
142. Add `docker-compose.yml` for non-K8s development (PostgreSQL + services)
143. Add `Makefile` target for running individual services without PostgreSQL dependency check
144. Add `.env.example` for all configuration options with documentation
145. Add container image layer optimization — multi-stage build with distroless base
146. Add kustomize overlay support as alternative to Helm
147. Add migrations CI check — verify no un-run migrations exist before deploy
148. Add Terraform module for cloud deployment (AWS EKS / GKE / AKS)
149. Add S3-compatible storage backend for git repos and artifacts (MinIO/Rook)
150. Add init container or sidecar for git GC / maintenance tasks

---

## 8. DOCUMENTATION (15 items)

151. Add API reference docs with full request/response examples (expand `api/openapi.yaml`)
152. Add OpenAPI spec coverage for every route (currently 9 documented, 50+ implemented)
153. Add `CONTRIBUTING.md` with PR workflow, coding standards, commit conventions
154. Add `CHANGELOG.md` with semantic versioning
155. Add `SECURITY.md` with vulnerability disclosure policy
156. Add `CODE_OF_CONDUCT.md`
157. Add inline code comments for complex logic (merge, SSH protocol, actions controller reconciliation)
158. Add godoc comments on all exported types, functions, and sentinel errors
159. Add deployment topology diagram and HA considerations to `docs/architecture.md`
160. Add troubleshooting guide (`docs/troubleshooting.md`) for common issues
161. Add backup/restore procedure docs for PostgreSQL + PVC data
162. Add upgrade guide for Helm chart version bumps with breaking changes
163. Add performance tuning guide for production deployment
164. Add SSH key setup guide in `docs/git.md`
165. Add migration from GitHub documentation (import repos, mirror workflow)

---

## 9. NEW FEATURES (20 items)

### Core GitHub Features

166. **GitHub Actions cache support** — `actions/cache` equivalent using PVC or S3
167. **GitHub Actions artifacts** — Upload/download between jobs, served via API
168. **Required reviews on PRs** — Enforce N approvals before merge
169. **Merge queue** — Queue merges with CI gate (merge when green)
170. **Code owners enforcement** — `CODEOWNERS` file parsing, required review by owner
171. **Draft PRs** — Mark PR as draft, prevent merge until marked ready
172. **PR merge methods** — Add `rebase` and `squash` merge options (currently squash-only)
173. **Git LFS support** — Large file storage with dedicated LFS API
174. **Issue templates** — Template chooser when creating issue (`ISSUE_TEMPLATE/` directory)
175. **Repository topics/tags** — Categorize repos with topic tags

### Advanced Features

176. **SSH certificate-based auth** — Short-lived SSH certs via API (like GitHub's)
177. **GitHub Pages / static site hosting** — Serve repo content as websites
178. **Webhook replay** — Re-deliver past webhook events
179. **Action marketplace** — Publish/list reusable actions (like `actions/checkout`)
180. **Deploy keys** — Read-only SSH deploy keys per repo
181. **Repository mirroring** — Push/pull mirror to external git remotes
182. **Scheduled workflows** — `on: schedule` cron-triggered actions
183. **Workflow dispatch inputs** — Support `workflow_dispatch` with typed input parameters
184. **Container registry** — OCI-compatible container image registry (Docker registry API v2)
185. **GitHub Copilot-like AI** — Inline code completion via AI provider

---

## 10. RELIABILITY & MONITORING (15 items)

186. Add graceful shutdown with context cancellation for all services
187. Add health check endpoint per service with dependency status (DB, OpenSearch, PVC)
188. Add structured logging with `slog` (Go 1.21+) instead of `log.Printf`
189. Add log levels (debug/info/warn/error) with configuration
190. Add request ID propagation middleware across service boundaries
191. Add connection pool settings via environment variables with hardcoded defaults
192. Add database migration idempotency review — verify `UP` + `DOWN` migrations
193. Add circuit breaker around OpenSearch calls (search fails should not crash the service)
194. Add leader election for actions-controller and search-indexer (prevent duplicate processing)
195. Add panic recovery middleware in all HTTP handlers (currently only chi's Recoverer)
196. Add webhook retry with backoff for failed deliveries (currently logs and continues)
197. Add dead letter queue for webhook deliveries after max retries
198. Add readiness probe that checks DB ping + migration completeness
199. Add startup probe that checks migration state
200. Add periodic git repository maintenance (gc, fsck) via cron job

---

## Summary

| Area | Count |
|------|-------|
| 1. Testing Coverage | 40 |
| 2. Missing API Endpoints | 20 |
| 3. Web UI Improvements | 25 |
| 4. Security | 15 |
| 5. Performance & Scalability | 15 |
| 6. Code Quality & Refactoring | 20 |
| 7. Infrastructure & Deployment | 15 |
| 8. Documentation | 15 |
| 9. New Features | 20 |
| 10. Reliability & Monitoring | 15 |
| **Total** | **200** |

## How to use

- Top priority items: #1–10 (unit test gaps), #61–70 (missing web UI), #86–100 (security), #186–200 (reliability)
- Quick wins: #116–120 (code quality), #136–140 (Helm chart), #151–155 (docs)
- Each item is scoped to be implementable in one focused session
