#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
NAMESPACE="${NAMESPACE:-govnohub}"
RELEASE="${RELEASE:-govnohub}"
REMOTE_DIR="${REMOTE_DIR:-/tmp/govnohub-install}"

IMAGES=(
  govnohub/api-server:latest
  govnohub/git-server:latest
  govnohub/actions-controller:latest
  govnohub/webhook-service:latest
  govnohub/search-indexer:latest
)

log() { echo "==> $*"; }
die() { echo "error: $*" >&2; exit 1; }

[[ -n "${INSTALL_HOST:-}" ]] || die "set INSTALL_HOST to the Linux VM address (e.g. make install INSTALL_HOST=192.168.1.10)"

INSTALL_USER="${INSTALL_USER:-root}"
REMOTE="${INSTALL_USER}@${INSTALL_HOST}"

SSH_ARGS=(-o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=accept-new)
[[ -n "${INSTALL_SSH_KEY:-}" ]] && SSH_ARGS+=(-i "$INSTALL_SSH_KEY")

if [[ "${INSTALL_HOST}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  INGRESS_HOST="${INSTALL_INGRESS_HOST:-govnohub.${INSTALL_HOST}.nip.io}"
  GIT_HOST="${INSTALL_GIT_HOST:-git.${INSTALL_HOST}.nip.io}"
else
  INGRESS_HOST="${INSTALL_INGRESS_HOST:-${INSTALL_HOST}}"
  GIT_HOST="${INSTALL_GIT_HOST:-git.${INSTALL_HOST}}"
fi

remote() {
  ssh "${SSH_ARGS[@]}" "$REMOTE" "$@"
}

remote_sudo() {
  ssh -t "${SSH_ARGS[@]}" "$REMOTE" "sudo bash -lc $(printf '%q' "$*")"
}

remote_sudo_stdin() {
  ssh "${SSH_ARGS[@]}" "$REMOTE" "sudo $*"
}

require_local() {
  for cmd in docker ssh rsync; do
    command -v "$cmd" >/dev/null || die "$cmd is required locally"
  done
}

log "checking SSH connectivity to $REMOTE"
remote "echo connected" >/dev/null

require_local

log "installing k3s on $REMOTE (if needed)"
remote_sudo '
  if command -v k3s >/dev/null 2>&1; then
    echo "k3s already installed"
  else
    curl -sfL https://get.k3s.io | sh -
  fi
  k3s kubectl wait --for=condition=ready node --all --timeout=180s
'

log "installing helm on $REMOTE (if needed)"
remote_sudo '
  if command -v helm >/dev/null 2>&1; then
    echo "helm already installed"
  else
    curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash
  fi
'

log "building Docker images locally"
make -C "$ROOT" docker-build

log "importing images into k3s on $REMOTE"
for img in "${IMAGES[@]}"; do
  log "  $img"
  docker save "$img" | remote_sudo_stdin k3s ctr images import -
done

VALUES_OVERRIDE="$(mktemp)"
trap 'rm -f "$VALUES_OVERRIDE"' EXIT
cat >"$VALUES_OVERRIDE" <<EOF
ingress:
  host: ${INGRESS_HOST}
  gitHost: ${GIT_HOST}
EOF

log "syncing Helm chart and CRDs to $REMOTE"
remote "rm -rf '$REMOTE_DIR' && mkdir -p '$REMOTE_DIR'"
rsync -az "$ROOT/deploy/" "$REMOTE:$REMOTE_DIR/deploy/"
rsync -az "$VALUES_OVERRIDE" "$REMOTE:$REMOTE_DIR/values-override.yaml"

log "deploying govnohub on $REMOTE"
remote_sudo "
  export KUBECONFIG=/etc/rancher/k3s/k3s.yaml
  kubectl create namespace '$NAMESPACE' --dry-run=client -o yaml | kubectl apply -f -
  kubectl apply -f '$REMOTE_DIR/deploy/crds/'
  helm upgrade --install '$RELEASE' '$REMOTE_DIR/deploy/helm/govnohub' \
    -n '$NAMESPACE' \
    -f '$REMOTE_DIR/deploy/helm/govnohub/values-k3s.yaml' \
    -f '$REMOTE_DIR/values-override.yaml' \
    --wait --timeout 10m
  kubectl wait --for=condition=available deployment -l app.kubernetes.io/name=govnohub \
    -n '$NAMESPACE' --timeout=300s
"

log "govnohub installed on $INSTALL_HOST"
echo
echo "  UI:  http://${INGRESS_HOST}"
echo "  API: http://${INGRESS_HOST}/api/v1"
echo "  Git: http://${GIT_HOST}/{owner}/{repo}.git"
echo
if [[ "${INSTALL_HOST}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "nip.io resolves these hostnames to ${INSTALL_HOST}; no /etc/hosts entry needed."
else
  echo "add to /etc/hosts: ${INSTALL_HOST} ${INGRESS_HOST} ${GIT_HOST}"
fi
