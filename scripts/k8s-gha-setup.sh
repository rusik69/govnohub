#!/usr/bin/env bash
# Create a kind cluster on GitHub Actions (Docker), deploy Govnohub, and wait until ready.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
CLUSTER="${KIND_CLUSTER:-govnohub}"
NAMESPACE="${NAMESPACE:-govnohub}"
RELEASE="${RELEASE:-govnohub}"
KIND_CONFIG="${ROOT}/deploy/kind/cluster.yaml"
VALUES="${ROOT}/deploy/helm/govnohub/values-kind.yaml"
IMAGES=(api-server git-server actions-controller webhook-service search-indexer)

log() { echo "==> $*"; }

require() {
  command -v "$1" >/dev/null || { echo "$1 not found"; exit 1; }
}

require kind
require kubectl
require helm
require docker

if ! docker info >/dev/null 2>&1; then
  echo "docker is not available"
  exit 1
fi

if kind get clusters 2>/dev/null | grep -qx "$CLUSTER"; then
  log "deleting existing kind cluster '$CLUSTER'"
  kind delete cluster --name "$CLUSTER"
fi

log "creating kind cluster '$CLUSTER'"
kind create cluster --name "$CLUSTER" --config "$KIND_CONFIG"
kubectl config use-context "kind-${CLUSTER}"
kubectl cluster-info --context "kind-${CLUSTER}"

log "installing ingress-nginx"
kubectl apply -f https://raw.githubusercontent.com/kubernetes/ingress-nginx/controller-v1.11.3/deploy/static/provider/kind/deploy.yaml
kubectl wait --namespace ingress-nginx \
  --for=condition=available deployment/ingress-nginx-controller \
  --timeout=300s

log "building container images"
make -C "$ROOT" docker-build CONTAINER_RUNTIME=docker

log "loading images into kind"
for svc in "${IMAGES[@]}"; do
  kind load docker-image "govnohub/${svc}:latest" --name "$CLUSTER"
done

kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
kubectl apply -f "$ROOT/deploy/crds/"

log "installing helm release"
helm upgrade --install "$RELEASE" "$ROOT/deploy/helm/govnohub" \
  -n "$NAMESPACE" \
  -f "$VALUES" \
  --set image.repository=govnohub \
  --wait --timeout 10m

log "waiting for govnohub deployments"
kubectl wait --for=condition=available deployment \
  -l app.kubernetes.io/name=govnohub -n "$NAMESPACE" --timeout=600s

log "configuring /etc/hosts"
if ! grep -q 'govnohub.local' /etc/hosts; then
  echo '127.0.0.1 govnohub.local git.govnohub.local' | sudo tee -a /etc/hosts
fi

log "waiting for ingress endpoints"
deadline=$((SECONDS + 300))
until [ "$SECONDS" -ge "$deadline" ]; do
  if curl -fsS http://govnohub.local/healthz >/dev/null && \
     curl -fsS http://git.govnohub.local/healthz >/dev/null; then
    log "govnohub is reachable"
    exit 0
  fi
  sleep 5
done

kubectl get pods,ingress -n "$NAMESPACE"
kubectl get pods -n ingress-nginx
echo "timed out waiting for http://govnohub.local/healthz"
exit 1
