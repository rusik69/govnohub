#!/usr/bin/env bash
# Start a PostgreSQL instance for tests when Docker testcontainers is unavailable.
set -euo pipefail
PORT="${TEST_DB_PORT:-5433}"
docker rm -f govnohub-test-pg 2>/dev/null || true
docker run -d --name govnohub-test-pg \
  -e POSTGRES_USER=govnohub \
  -e POSTGRES_PASSWORD=govnohub \
  -e POSTGRES_DB=govnohub \
  -p "${PORT}:5432" \
  postgres:16-alpine
echo "export TEST_DATABASE_URL=postgres://govnohub:govnohub@localhost:${PORT}/govnohub?sslmode=disable"
