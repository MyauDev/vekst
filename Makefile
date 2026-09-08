# Vekst — see README.md. Generated code is never hand-edited; run `make gen`.
SHELL := /bin/bash
.DEFAULT_GOAL := help

GOBIN := $(shell go env GOPATH)/bin
export PATH := $(PATH):$(GOBIN)

VERSION  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo dev)
BUILT_AT ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS  := -X github.com/MyauDev/vekst/core/internal/buildinfo.version=$(VERSION) \
            -X github.com/MyauDev/vekst/core/internal/buildinfo.builtAt=$(BUILT_AT)

# The local cluster is pinned like every other tool (design D9). An unpinned
# cluster silently drifts away from what you deploy to, which is the entire
# benefit the Kubernetes dev environment was bought for.
CLUSTER   := vekst
K3S_IMAGE ?= rancher/k3s:v1.34.10-k3s1
INGRESS_PORT ?= 8081
# A local registry, so Tilt loads images into the cluster instead of trying to
# push them to Docker Hub. k3d advertises it via the local-registry-hosting
# ConfigMap, which Tilt reads automatically.
REGISTRY_PORT ?= 5555

.PHONY: help gen build lint test ci dev down migrate-up migrate-down

# vekst_migrator only -- the role migrations run as. core itself never sees
# this credential; it connects as vekst_app. Local default matches the local
# overlay's Postgres (design D1/Q1/Q2). Local-development credential only,
# reachable at 127.0.0.1 alone -- never a real secret, same as postgres.yaml.
DATABASE_URL_MIGRATOR ?= postgres://vekst_migrator:vekst_migrator@localhost:5432/vekst?sslmode=disable

help: ## List targets
	@grep -hE '^[a-z-]+:.*?## ' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-10s\033[0m %s\n",$$1,$$2}'

gen: ## Regenerate all stubs from /proto (Go, TypeScript, Python)
	buf generate --template buf.gen.go.yaml
	buf generate --template buf.gen.connect.yaml --path proto/vekst/v1
	buf generate --template buf.gen.grpc.yaml    --path proto/vekst/internal/v1
	@# vekst.type.v1 (Money) is browser- and classifier-facing alike, so the web
	@# and Python legs list it explicitly alongside their own package -- a
	@# generator with only its own package in scope would silently skip it.
	buf generate --template buf.gen.web.yaml     --path proto/vekst/v1 --path proto/vekst/type/v1
	@# Python: grpc_tools.protoc directly. It is protoc, not a buf plugin, so it
	@# cannot be driven from a buf.gen template. /proto stays the single source
	@# of truth; buf still owns lint and breaking for the contract.
	@#
	@# classifier.proto declares a service, so it gets all four outputs.
	@# money.proto declares only a message -- running it through
	@# --grpc_python_out/--mypy_grpc_out produces a _grpc.py containing nothing
	@# but a version-check guard, with an unused `warnings` import flagged by
	@# every linter that looks at it. Separate invocations, not separate flags
	@# per file: protoc applies one set of output flags to every file it's given.
	cd classifier && uv run python -m grpc_tools.protoc \
		-I ../proto \
		--python_out=src --grpc_python_out=src \
		--mypy_out=src --mypy_grpc_out=src \
		../proto/vekst/internal/v1/classifier.proto
	cd classifier && uv run python -m grpc_tools.protoc \
		-I ../proto \
		--python_out=src --mypy_out=src \
		../proto/vekst/type/v1/money.proto
	@# The classifier is installed non-editable, so regenerated stubs only reach
	@# the venv after a resync. In the cluster, Tilt live_update syncs sources
	@# into the container instead.
	cd classifier && uv sync --no-editable --quiet
	@# sqlc: typed Go from the checked-in SQL, joining the same drift gate as
	@# buf's output (design D7).
	go tool sqlc generate

dev: ## Create the local cluster if absent, then start Tilt
	@k3d cluster list $(CLUSTER) >/dev/null 2>&1 || { \
	  echo "creating k3d cluster '$(CLUSTER)' on $(K3S_IMAGE)"; \
	  k3d cluster create $(CLUSTER) --image $(K3S_IMAGE) \
	    -p "$(INGRESS_PORT):80@loadbalancer" \
	    --registry-create $(CLUSTER)-registry:0.0.0.0:$(REGISTRY_PORT) --wait; }
	@# Switch context explicitly. The Tiltfile also pins allow_k8s_contexts, so
	@# a stray kubeconfig cannot point this at a real cluster.
	kubectl config use-context k3d-$(CLUSTER)
	cd deploy && tilt up

down: ## Delete the cluster and everything in it
	-cd deploy && tilt down --delete-namespaces 2>/dev/null
	-k3d cluster delete $(CLUSTER)

build: ## Build core with its version stamped in
	go build -ldflags "$(LDFLAGS)" -o bin/vekst-core ./core/cmd/vekst-core

## core/migrations/00002_river.go is a Go migration: it must be compiled into
## the binary that runs it, so plain goose (a generic CLI with no knowledge of
## our init() registrations) cannot apply it. `vekst-core migrate` is the same
## binary that serves traffic, reading the same embedded migrations -- design
## Q5 -- so migrating always goes through it, both here and in the cluster's
## migration Job.
migrate-up: ## Apply pending migrations to DATABASE_URL_MIGRATOR
	DATABASE_URL_MIGRATOR="$(DATABASE_URL_MIGRATOR)" go run ./core/cmd/vekst-core migrate up

migrate-down: ## Roll back one migration on DATABASE_URL_MIGRATOR
	DATABASE_URL_MIGRATOR="$(DATABASE_URL_MIGRATOR)" go run ./core/cmd/vekst-core migrate down

lint: ## Lint every language
	buf lint
	go vet ./...
	./scripts/check-db-entry-point.sh
	./scripts/check-identity-queries.sh
	./scripts/check-codeowners.sh
	./scripts/check-web-tokens.sh
	cd classifier && uv run ruff check . && uv run mypy .
	cd web && npx tsc --noEmit

test: ## Run every test suite
	go test ./...
	cd classifier && uv run pytest -q
	cd web && npx vitest run

ci: lint test ## What CI runs
