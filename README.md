# Govnohub

Kubernetes-native GitHub + GitHub Actions clone.

## Stack

- **Backend:** Go (api-server, git-server, actions-controller, webhook-service, search-indexer)
- **Frontend:** Vue 3 + TypeScript + Tailwind
- **Data:** PostgreSQL, OpenSearch, PVC/object storage
- **CI Runtime:** Custom GitHub Actions YAML parser + Kubernetes Jobs

## Quick Start (local)

```bash
# Dependencies
go mod download
cd frontend && npm install && npm run build

# Run API (requires PostgreSQL)
export DATABASE_URL=postgres://govnohub:govnohub@localhost:5432/govnohub?sslmode=disable
go run ./cmd/api-server
```

## Kubernetes Deploy

```bash
helm install govnohub ./deploy/helm/govnohub -n govnohub --create-namespace
kubectl apply -f deploy/crds/
```

## Services

| Service | Port | Description |
|---------|------|-------------|
| api-server | 8080 | REST API |
| git-server | 8081 | Smart HTTP git |
| webhook-service | 8082 | Push/PR event dispatcher |
| frontend | 80 | Vue SPA |

## Features

- Users, PATs, orgs, teams
- Git hosting (clone/push over HTTPS)
- Issues, labels, milestones
- Pull requests, reviews, merge
- GitHub Actions workflows (YAML) with K8s Job runners
- Releases and package registry
- Global search (OpenSearch)
- Outbound webhooks, stars, forks, branch protection
