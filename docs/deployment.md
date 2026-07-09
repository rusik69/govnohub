# Deployment

## Podman + kind (recommended when using Podman)

Uses [kind](https://kind.sigs.k8s.io/) with Podman as the container runtime (no Docker required).

```bash
brew install kind   # if not installed
make podman-k8s-create   # kind cluster + ingress-nginx
make deploy-k3s          # build (podman), load images, helm deploy

echo "127.0.0.1 govnohub.local git.govnohub.local" | sudo tee -a /etc/hosts
make k3s-status
```

One-shot:

```bash
make deploy-podman-k8s
```

Teardown:

```bash
make undeploy-k3s
make podman-k8s-delete
```

Helm values: `deploy/helm/govnohub/values-kind.yaml` (nginx ingress, `imagePullPolicy: Never`).

## k3d (Docker on macOS)

k3d runs k3s inside Docker and is the easiest local Kubernetes option on macOS.

```bash
# Create cluster with ingress ports
make k3d-create

# Full deploy: build images, import, helm install
make deploy-k3s

# Add hosts entry
echo "127.0.0.1 govnohub.local git.govnohub.local" | sudo tee -a /etc/hosts

# Check status
make k3s-status
```

Access:
- UI: http://govnohub.local
- API: http://govnohub.local/api/v1
- Git: http://git.govnohub.local/{owner}/{repo}.git

## k3s (Linux)

```bash
make k3s-install
make k3s-wait
make deploy-k3s
```

## Remote Linux VM (SSH)

Install k3s and deploy govnohub on a remote Linux VM from your dev machine:

```bash
# passwordless SSH as root (or set INSTALL_USER)
make install INSTALL_HOST=192.168.1.10

# custom SSH user and key
make install INSTALL_HOST=vm.example.com INSTALL_USER=ubuntu INSTALL_SSH_KEY=~/.ssh/id_ed25519
```

The script will:
1. Install k3s and helm on the VM (if missing)
2. Build Docker images locally and import them into k3s
3. Deploy the Helm chart with Traefik ingress

For IP addresses, ingress uses `nip.io` hostnames (e.g. `govnohub.192.168.1.10.nip.io`).
Override with `INSTALL_INGRESS_HOST` and `INSTALL_GIT_HOST` if needed.

## Makefile Targets

| Target | Description |
|--------|-------------|
| `install` | Install k3s + govnohub on remote Linux VM over SSH |
| `podman-k8s-create` | Create kind cluster using Podman + ingress-nginx |
| `podman-k8s-delete` | Delete kind cluster |
| `deploy-podman-k8s` | Create cluster and full deploy |
| `k3d-create` | Create k3d cluster named `govnohub` |
| `k3d-delete` | Delete k3d cluster |
| `k3s-install` | Install k3s (Linux only) |
| `k3s-uninstall` | Remove k3s |
| `k3s-import-images` | Build and import Docker images into cluster |
| `deploy-k3s` | Full deploy pipeline |
| `undeploy-k3s` | Helm uninstall + namespace cleanup |
| `redeploy-k3s` | Undeploy then deploy |
| `k3s-status` | Show nodes and pods |

## Helm Values

- Default: `deploy/helm/govnohub/values.yaml` (nginx ingress, 10Gi PVCs)
- k3s local: `deploy/helm/govnohub/values-k3s.yaml` (traefik, `imagePullPolicy: Never`, smaller PVCs)
- kind/podman local: `deploy/helm/govnohub/values-kind.yaml` (nginx ingress, `localhost/govnohub` images, RWO PVCs)

```bash
helm upgrade --install govnohub ./deploy/helm/govnohub \
  -n govnohub --create-namespace \
  -f ./deploy/helm/govnohub/values-k3s.yaml
```

## CRDs

Apply before or with Helm:

```bash
kubectl apply -f deploy/crds/
```

## Production Notes

- Change `jwtSecret` in Helm values
- Use external PostgreSQL and OpenSearch for production
- Replace PVC git storage with S3-compatible backend before scale
- Configure TLS on Ingress
- Set resource limits on runner Jobs via `RunnerPool` CRD

## Troubleshooting

```bash
kubectl get pods -n govnohub
kubectl logs -n govnohub deploy/govnohub-govnohub-api-server
kubectl describe pod -n govnohub <pod>
```

If images fail to pull on k3d, ensure `imagePullPolicy: Never` in values-k3s.yaml and re-run `make k3s-import-images`.
