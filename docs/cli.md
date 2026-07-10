# Govnohub CLI

The `govnohub` command-line client talks to the REST API and wraps common git operations.

## Install

```bash
make build-cli
sudo cp bin/govnohub /usr/local/bin/
```

Or build directly:

```bash
go build -o bin/govnohub ./cmd/govnohub
```

## Configuration

Config is stored at `~/.config/govnohub/config.yaml`:

```yaml
api_url: http://localhost:8080
git_url: http://localhost:8081
token: ghp_...
username: alice
```

Override with flags:

```bash
govnohub --api-url https://api.govnohub.local --token ghp_xxx repo list
```

## Authentication

Interactive login (stores JWT in config):

```bash
govnohub auth login
```

Create a personal access token (PAT) for git and API:

```bash
govnohub auth token create --name laptop
```

PATs use the `ghp_` prefix and work with both the API and git HTTP.

## Repositories

```bash
govnohub repo list
govnohub repo create my-app --description "Demo app"
govnohub repo clone alice/my-app
govnohub repo clone alice/my-app ./work/my-app
```

## Issues & pull requests

```bash
govnohub issue list alice/my-app
govnohub issue create alice/my-app "Bug title" "Details here"

govnohub pr list alice/my-app
govnohub pr create alice/my-app "Add feature" --head feature --base main
govnohub pr merge alice/my-app 1
```

## Releases

```bash
govnohub release create alice/my-app --tag v1.0.0 --name "First release" --body "Notes"
govnohub release upload alice/my-app v1.0.0 ./dist/app.tar.gz
```

## Actions

```bash
govnohub run list alice/my-app
govnohub run trigger alice/my-app --workflow <workflow-uuid> --branch main
```

## Search

```bash
govnohub search "TODO refactor"
```

## Environment

| Flag / config | Default | Description |
|---------------|---------|-------------|
| `api_url` | `http://localhost:8080` | REST API base URL |
| `git_url` | `http://localhost:8081` | Git smart HTTP base URL |
| `token` | — | JWT or PAT (`ghp_*`) |
