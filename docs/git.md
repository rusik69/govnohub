# Git hosting

Govnohub serves bare repositories over **Git smart HTTP** on port **8081** (git-server).

## Clone

Using a PAT in the URL (recommended for scripts):

```bash
git clone https://x-access-token:ghp_YOUR_TOKEN@git.govnohub.local/alice/my-repo.git
```

Using basic auth (username can be anything, password is the PAT):

```bash
git clone https://alice:ghp_YOUR_TOKEN@git.govnohub.local/alice/my-repo.git
```

Using the CLI (reads token from config):

```bash
govnohub repo clone alice/my-repo
```

## Remotes

Add a remote to an existing local repo:

```bash
git remote add origin https://git.govnohub.local/alice/my-repo.git
git config credential.helper store   # optional: cache credentials
git push -u origin main
```

For local development:

```bash
git remote add origin http://localhost:8081/alice/my-repo.git
```

## Push & pull

Standard git commands work once authenticated:

```bash
git pull origin main
git push origin main
git push origin feature-branch
```

After a successful push, git-server:

1. Updates branch heads in PostgreSQL
2. Notifies webhook-service (`POST /events/push`) to dispatch webhooks and queue workflows

## Protocol

The server implements smart HTTP:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/{owner}/{repo}.git/info/refs?service=git-upload-pack` | GET | Advertise refs for fetch/clone |
| `/{owner}/{repo}.git/info/refs?service=git-receive-pack` | GET | Advertise refs for push |
| `/{owner}/{repo}.git/git-upload-pack` | POST | Fetch pack data |
| `/{owner}/{repo}.git/git-receive-pack` | POST | Push pack data |

Content types follow the git HTTP transport specification (`application/x-git-*-advertisement`, `-request`, `-result`).

## Authentication

Supported methods:

- **HTTP Basic** — password is a PAT (`ghp_*`) or account password (JWT not used for git)
- **Bearer token** — `Authorization: Bearer ghp_*` (some git clients)

Private repos require read access; push requires write access on the repository.

## Troubleshooting

**401 Unauthorized** — Check PAT is valid and not expired. Use `govnohub auth token create` to mint a new one.

**403 Forbidden** — User lacks read/write permission on the repo.

**404 Not Found** — Repository missing in DB or on disk (`GIT_ROOT/{owner}/{repo}.git`).

**Empty clone / missing refs** — Ensure the repo was initialized via the API (`govnohub repo create` or UI) so the bare repo exists on the git server.

## Environment

| Variable | Default | Description |
|----------|---------|-------------|
| `GIT_HTTP_ADDR` | `:8081` | Listen address |
| `GIT_ROOT` | `/data/git` | Bare repository storage |
| `DATABASE_URL` | — | Required for auth and branch metadata |
| `WEBHOOK_SERVICE_URL` | `http://webhook-service:8082` | Push event dispatcher |
