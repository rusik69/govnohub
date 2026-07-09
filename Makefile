.PHONY: build test run-api run-git helm-install

build:
	go build -o bin/api-server ./cmd/api-server
	go build -o bin/git-server ./cmd/git-server
	go build -o bin/actions-controller ./cmd/actions-controller
	go build -o bin/webhook-service ./cmd/webhook-service
	go build -o bin/search-indexer ./cmd/search-indexer
	cd frontend && npm run build

test:
	go test ./...

run-api:
	go run ./cmd/api-server

run-git:
	go run ./cmd/git-server

helm-install:
	helm install govnohub ./deploy/helm/govnohub -n govnohub --create-namespace
	kubectl apply -f deploy/crds/

docker-build:
	docker build -f deploy/docker/Dockerfile --build-arg SERVICE=api-server -t govnohub/api-server:latest .
	docker build -f deploy/docker/Dockerfile --build-arg SERVICE=git-server -t govnohub/git-server:latest .
	docker build -f deploy/docker/Dockerfile --build-arg SERVICE=actions-controller -t govnohub/actions-controller:latest .
	docker build -f deploy/docker/Dockerfile --build-arg SERVICE=webhook-service -t govnohub/webhook-service:latest .
	docker build -f deploy/docker/Dockerfile --build-arg SERVICE=search-indexer -t govnohub/search-indexer:latest .
	docker build -f deploy/docker/Dockerfile.frontend -t govnohub/frontend:latest .
