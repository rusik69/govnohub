# Govnohub

Kubernetes-native GitHub + GitHub Actions clone.

## Quick Commands

```bash
make build              # build all binaries + frontend
make test-unit          # unit tests (testcontainers)
make test-integration   # API integration tests
make test-e2e           # full user journey e2e
make test-all           # everything including frontend
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
- **Frontend:** Vue 3 + TypeScript + Tailwind
- **Data:** PostgreSQL, OpenSearch, PVC storage
- **CI Runtime:** GitHub Actions YAML parser + Kubernetes Jobs

## Local Development

```bash
cp .env.example .env
docker run -d --name govnohub-pg -e POSTGRES_USER=govnohub -e POSTGRES_PASSWORD=govnohub \
  -e POSTGRES_DB=govnohub -p 5432:5432 postgres:16-alpine
make run-api
cd frontend && npm run dev
```

## Kubernetes / k3s Deploy

```bash
make k3d-create      # or make k3s-install on Linux
make deploy-k3s
echo "127.0.0.1 govnohub.local git.govnohub.local" | sudo tee -a /etc/hosts
open http://govnohub.local
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| api-server | 8080 | REST API |
| git-server | 8081 | Smart HTTP git |
| webhook-service | 8082 | Event dispatcher |
| frontend | 80 | Vue SPA |
