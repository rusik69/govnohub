#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="${NAMESPACE:-govnohub}"
RELEASE="${RELEASE:-govnohub}"
CLUSTER="${K3D_CLUSTER:-govnohub}"
VALUES="${ROOT}/deploy/helm/govnohub/values-k3s.yaml"

log() { echo "==> $*"; }

detect_runtime() {
  if command -v k3d >/dev/null 2>&1 && k3d cluster list 2>/dev/null | grep -q "$CLUSTER"; then
    echo k3d
  elif command -v k3s >/dev/null 2>&1 && k3s kubectl get nodes >/dev/null 2>&1; then
    echo k3s
  elif kubectl config current-context 2>/dev/null | grep -qi k3s; then
    echo k3s
  else
    echo none
  fi
}

import_images() {
  local runtime="$1"
  local images=(
    govnohub/api-server:latest
    govnohub/git-server:latest
    govnohub/actions-controller:latest
    govnohub/webhook-service:latest
    govnohub/search-indexer:latest
    govnohub/frontend:latest
  )
  for img in "${images[@]}"; do
    log "importing $img"
    case "$runtime" in
      k3d)
        k3d image import "$img" -c "$CLUSTER"
        ;;
      k3s)
        docker save "$img" | sudo k3s ctr images import -
        ;;
      *)
        log "no k3s/k3d runtime; skipping image import"
        ;;
    esac
  done
}

wait_ready() {
  log "waiting for deployments"
  kubectl wait --for=condition=available deployment -l app.kubernetes.io/name=govnohub -n "$NAMESPACE" --timeout=300s 2>/dev/null || \
  kubectl get pods -n "$NAMESPACE"
}

case "${1:-deploy}" in
  deploy)
    runtime="$(detect_runtime)"
    log "runtime=$runtime"
    make -C "$ROOT" docker-build
    import_images "$runtime"
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    kubectl apply -f "$ROOT/deploy/crds/"
    helm upgrade --install "$RELEASE" "$ROOT/deploy/helm/govnohub" \
      -n "$NAMESPACE" -f "$VALUES" --wait --timeout 5m
    wait_ready
    log "deployed. add to /etc/hosts: 127.0.0.1 govnohub.local git.govnohub.local"
    ;;
  undeploy)
    helm uninstall "$RELEASE" -n "$NAMESPACE" || true
    kubectl delete namespace "$NAMESPACE" --wait=false || true
    ;;
  *)
    echo "usage: $0 [deploy|undeploy]"
    exit 1
    ;;
esac
