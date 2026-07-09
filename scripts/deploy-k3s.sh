#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="${NAMESPACE:-govnohub}"
RELEASE="${RELEASE:-govnohub}"
K3D_CLUSTER="${K3D_CLUSTER:-govnohub}"
KIND_CLUSTER="${KIND_CLUSTER:-govnohub}"
VALUES_K3S="${ROOT}/deploy/helm/govnohub/values-k3s.yaml"
VALUES_KIND="${ROOT}/deploy/helm/govnohub/values-kind.yaml"

log() { echo "==> $*"; }

container_runtime() {
  if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
    echo docker
  elif command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then
    echo podman
  else
    echo none
  fi
}

detect_runtime() {
  export KIND_EXPERIMENTAL_PROVIDER=podman
  if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -qx "$KIND_CLUSTER"; then
    echo kind
  elif command -v k3d >/dev/null 2>&1 && k3d cluster list 2>/dev/null | grep -q "$K3D_CLUSTER"; then
    echo k3d
  elif command -v k3s >/dev/null 2>&1 && k3s kubectl get nodes >/dev/null 2>&1; then
    echo k3s
  elif kubectl config current-context 2>/dev/null | grep -qi k3s; then
    echo k3s
  else
    echo none
  fi
}

helm_values() {
  local runtime="$1"
  case "$runtime" in
    kind) echo "$VALUES_KIND" ;;
    *) echo "$VALUES_K3S" ;;
  esac
}

import_images() {
  local runtime="$1"
  local ctr
  ctr="$(container_runtime)"
  local images=(
    govnohub/api-server:latest
    govnohub/git-server:latest
    govnohub/actions-controller:latest
    govnohub/webhook-service:latest
    govnohub/search-indexer:latest
    govnohub/frontend:latest
  )
  for img in "${images[@]}"; do
    log "importing $img into $runtime"
    case "$runtime" in
      kind)
        export KIND_EXPERIMENTAL_PROVIDER=podman
        local src="$img"
        if [ "$ctr" = podman ] && podman image exists "localhost/$img" 2>/dev/null; then
          src="localhost/$img"
        fi
        if [ "$ctr" = podman ]; then
          local archive
          archive="$(mktemp /tmp/govnohub-img.XXXXXX.tar)"
          podman save -q "$src" -o "$archive"
          kind load image-archive "$archive" --name "$KIND_CLUSTER"
          rm -f "$archive"
        else
          kind load docker-image "$img" --name "$KIND_CLUSTER"
        fi
        ;;
      k3d)
        k3d image import "$img" -c "$K3D_CLUSTER"
        ;;
      k3s)
        if [ "$ctr" = podman ]; then
          podman save "$img" | sudo k3s ctr images import -
        else
          docker save "$img" | sudo k3s ctr images import -
        fi
        ;;
      *)
        log "no k8s runtime; skipping image import"
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
    log "runtime=$runtime container=$(container_runtime)"
    if [ "$runtime" = none ]; then
      log "no local cluster found — run: make podman-k8s-create"
      exit 1
    fi
    make -C "$ROOT" docker-build
    import_images "$runtime"
    kubectl create namespace "$NAMESPACE" --dry-run=client -o yaml | kubectl apply -f -
    kubectl apply -f "$ROOT/deploy/crds/"
    helm upgrade --install "$RELEASE" "$ROOT/deploy/helm/govnohub" \
      -n "$NAMESPACE" -f "$(helm_values "$runtime")" --wait --timeout 5m
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
