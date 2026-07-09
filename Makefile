SHELL := /bin/bash
ROOT := $(shell pwd)
NAMESPACE ?= govnohub
RELEASE ?= govnohub
K3D_CLUSTER ?= govnohub
HELM_CHART := ./deploy/helm/govnohub
VALUES_K3S := $(HELM_CHART)/values-k3s.yaml
IMAGES := api-server git-server actions-controller webhook-service search-indexer frontend

INSTALL_HOST ?=
INSTALL_USER ?= root
INSTALL_SSH_KEY ?=

.PHONY: build build-cli test test-unit test-integration test-e2e test-deploy-e2e test-k8s test-all test-frontend \
        run-api run-git docker-build helm-install install \
        k3s-install k3s-uninstall k3s-status k3s-wait k3d-create k3d-delete \
        podman-k8s-create podman-k8s-delete deploy-k3s deploy-podman-k8s undeploy-k3s redeploy-k3s docs

CONTAINER_RUNTIME ?= $(shell if command -v docker >/dev/null 2>&1 && docker info >/dev/null 2>&1; then echo docker; elif command -v podman >/dev/null 2>&1 && podman info >/dev/null 2>&1; then echo podman; else echo docker; fi)

build:
	go build -o bin/api-server ./cmd/api-server
	go build -o bin/git-server ./cmd/git-server
	go build -o bin/govnohub ./cmd/govnohub
	go build -o bin/actions-controller ./cmd/actions-controller
	go build -o bin/webhook-service ./cmd/webhook-service
	go build -o bin/search-indexer ./cmd/search-indexer
	cd frontend && npm run build

build-cli:
	go build -o bin/govnohub ./cmd/govnohub

test: test-unit

test-unit:
	go test -race -count=1 -coverprofile=coverage.out ./internal/... -short
	go tool cover -func=coverage.out | tail -1

test-integration:
	go test -race -count=1 ./tests/integration/... -tags=integration

test-e2e:
	go test -race -count=1 -timeout=10m ./tests/e2e/... -tags=e2e

test-deploy-e2e:
	go test -race -count=1 -timeout=15m ./tests/e2e/... -tags=deploy

test-k8s:
	go test -count=1 -timeout=15m ./tests/e2e/... -tags=k8s

test-all: test-unit test-integration test-e2e test-deploy-e2e test-frontend

test-frontend:
	cd frontend && npm run test

run-api:
	go run ./cmd/api-server

run-git:
	go run ./cmd/git-server

helm-install:
	helm upgrade --install $(RELEASE) $(HELM_CHART) -n $(NAMESPACE) --create-namespace
	kubectl apply -f deploy/crds/

install:
	@chmod +x scripts/install-remote.sh
	@INSTALL_HOST=$(INSTALL_HOST) INSTALL_USER=$(INSTALL_USER) INSTALL_SSH_KEY=$(INSTALL_SSH_KEY) \
		./scripts/install-remote.sh

docker-build:
	@for svc in api-server git-server actions-controller webhook-service search-indexer; do \
		echo "building govnohub/$$svc:latest ($(CONTAINER_RUNTIME))"; \
		$(CONTAINER_RUNTIME) build -f deploy/docker/Dockerfile --build-arg SERVICE=$$svc -t govnohub/$${svc}:latest .; \
		if [ "$(CONTAINER_RUNTIME)" = podman ]; then \
			$(CONTAINER_RUNTIME) tag "localhost/govnohub/$${svc}:latest" "govnohub/$${svc}:latest" 2>/dev/null || true; \
		fi; \
	done
	$(CONTAINER_RUNTIME) build -f deploy/docker/Dockerfile.frontend -t govnohub/frontend:latest .
	@if [ "$(CONTAINER_RUNTIME)" = podman ]; then \
		podman tag "localhost/govnohub/frontend:latest" "govnohub/frontend:latest" 2>/dev/null || true; \
	fi

k3s-install:
	@if command -v k3s >/dev/null 2>&1; then \
		echo "k3s already installed"; \
	elif [ "$$(uname)" = "Linux" ]; then \
		curl -sfL https://get.k3s.io | sh -; \
	else \
		echo "k3s is Linux-only; use: make k3d-create"; exit 1; \
	fi

k3s-uninstall:
	@if command -v k3s >/dev/null 2>&1; then \
		sudo /usr/local/bin/k3s-uninstall.sh || true; \
	else \
		echo "k3s not installed"; \
	fi

k3s-status:
	@kubectl get nodes 2>/dev/null || echo "cluster not reachable"
	@kubectl get pods -n $(NAMESPACE) 2>/dev/null || true

k3s-wait:
	@kubectl wait --for=condition=ready node --all --timeout=120s

k3d-create:
	@command -v k3d >/dev/null || (echo "install k3d: https://k3d.io" && exit 1)
	@k3d cluster list | grep -q $(K3D_CLUSTER) || \
		k3d cluster create $(K3D_CLUSTER) --api-port 6550 -p "80:80@loadbalancer" -p "443:443@loadbalancer"
	@kubectl config use-context k3d-$(K3D_CLUSTER)

k3d-delete:
	@k3d cluster delete $(K3D_CLUSTER) || true

k3s-import-images: docker-build
	@export KIND_EXPERIMENTAL_PROVIDER=podman; \
	if command -v kind >/dev/null 2>&1 && kind get clusters 2>/dev/null | grep -qx $(KIND_CLUSTER); then \
		for img in $(foreach s,$(IMAGES),govnohub/$(s):latest); do \
			echo importing $$img to kind; \
			if [ "$(CONTAINER_RUNTIME)" = podman ]; then \
				src=$$img; \
				podman image exists localhost/$$img 2>/dev/null && src=localhost/$$img; \
				archive=$$(mktemp /tmp/govnohub-img-XXXXXX); \
				podman save -q $$src -o $$archive; \
				kind load image-archive $$archive --name $(KIND_CLUSTER); \
				rm -f $$archive; \
			else \
				kind load docker-image $$img --name $(KIND_CLUSTER); \
			fi; \
		done; \
	elif k3d cluster list 2>/dev/null | grep -q $(K3D_CLUSTER); then \
		for img in $(foreach s,$(IMAGES),govnohub/$(s):latest); do \
			echo importing $$img to k3d; k3d image import $$img -c $(K3D_CLUSTER); \
		done; \
	elif command -v k3s >/dev/null 2>&1; then \
		for img in $(foreach s,$(IMAGES),govnohub/$(s):latest); do \
			echo importing $$img to k3s; $(CONTAINER_RUNTIME) save $$img | sudo k3s ctr images import -; \
		done; \
	else \
		echo "no k3s/k3d/kind cluster found — run: make podman-k8s-create"; exit 1; \
	fi

KIND_CLUSTER ?= govnohub

podman-k8s-create:
	@chmod +x scripts/podman-k8s-create.sh
	@./scripts/podman-k8s-create.sh

podman-k8s-delete:
	@chmod +x scripts/podman-k8s-delete.sh
	@./scripts/podman-k8s-delete.sh

deploy-podman-k8s: podman-k8s-create deploy-k3s

deploy-k3s: k3s-import-images
	@chmod +x scripts/deploy-k3s.sh
	@./scripts/deploy-k3s.sh deploy

undeploy-k3s:
	@chmod +x scripts/deploy-k3s.sh
	@./scripts/deploy-k3s.sh undeploy

redeploy-k3s: undeploy-k3s deploy-k3s

docs:
	@echo "Documentation in docs/"
