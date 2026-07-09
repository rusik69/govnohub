#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER="${KIND_CLUSTER:-govnohub}"
KIND_CONFIG="${ROOT}/deploy/kind/cluster.yaml"

log() { echo "==> $*"; }

if ! command -v podman >/dev/null; then
  echo "podman not found"
  exit 1
fi
if ! command -v kind >/dev/null; then
  echo "kind not found — install with: brew install kind"
  exit 1
fi

export KIND_EXPERIMENTAL_PROVIDER=podman

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  log "kind cluster '$CLUSTER' already exists"
else
  log "creating kind cluster '$CLUSTER' with podman"
  kind create cluster --name "$CLUSTER" --config "$KIND_CONFIG"
fi

kubectl config use-context "kind-${CLUSTER}"

log "installing ingress-nginx"
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.3/deploy/static/provider/kind/deploy.yaml

log "waiting for ingress-nginx controller"
kubectl wait --namespace ingress-nginx \
  --for=condition=available deployment/ingress-nginx-controller \
  --timeout=180s

log "cluster ready (context: kind-${CLUSTER})"
log "add to /etc/hosts: 127.0.0.1 govnohub.local git.govnohub.local"
