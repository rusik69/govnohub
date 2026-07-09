# Govnohub

Kubernetes-native GitHub + GitHub Actions clone.

## Quick Commands

```bash
make build              # build all binaries + generate templ
make test-unit          # unit tests (testcontainers)
make test-integration   # API integration tests
make test-e2e           # full user journey e2e
make test-all           # everything
make k3d-create         # create local k3d cluster (macOS/Linux)
make deploy-k3s         # build, import images, helm deploy
make install INSTALL_HOST=192.168.1.10  # remote Linux VM over SSH
```

## Documentation

- [Architecture](docs/architecture.md)
- [Development](docs/development.md)
- [Deployment (k3s/k3d)](docs/deployment.md)
- [Testing](docs/testing.md)

## Stack

- **Backend:** Go services (api-server, git-server, actions-controller, webhook-service, search-indexer)
- **Web UI:** templ + HTMX + Tailwind (embedded in api-server)
- **Data:** PostgreSQL, OpenSearch, PVC storage
- **CI Runtime:** GitHub Actions YAML parser + Kubernetes Jobs

## Local Development

```bash
cp .env.example .env
docker run -d --name govnohub-pg -e POSTGRES_USER=govnohub -e POSTGRES_PASSWORD=govnohub \
  -e POSTGRES_DB=govnohub -p 5432:5432 postgres:16-alpine
make run-api
open http://localhost:8080
```

Default admin (created on first startup when no users exist): `admin` / `admin`. Public self-registration is disabled; admins manage users at `/admin/users`.

## Kubernetes / k3s Deploy

```bash
make k3d-create      # or make k3s-install on Linux
make deploy-k3s
echo "127.0.0.1 govnohub.local git.govnohub.local" | sudo tee -a /etc/hosts
open http://govnohub.local
```

## Podman + kind (local K8s without Docker)

```bash
brew install kind    # once
make deploy-podman-k8s
echo "127.0.0.1 govnohub.local git.govnohub.local" | sudo tee -a /etc/hosts
open http://govnohub.local
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| api-server | 8080 | REST API + web UI |
| git-server | 8081 | Smart HTTP git |
| git-server (SSH) | 2222 | Git over SSH |
| webhook-service | 8082 | Event dispatcher |
