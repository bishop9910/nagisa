# Windows without GNU make: use the equivalent PowerShell entry point instead,
# .\scripts\build.ps1 <target>. It implements every target below.
GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
# `git describe` is empty outside a checkout (an exported tarball, some CI
# workspaces); fall back so the ldflags below never expand to a dangling -X.
VERSION?=$(shell git describe --tags --always 2>/dev/null || echo dev)
REVISION=$(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
# RFC 3339, for the OCI created label.
BUILD_TIME=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS=-s -w -X main.Version=$(VERSION)

# Docker targets need only GNU make and the Docker CLI; the build runs inside
# the image (Go and Node are not required on the host).
COMPOSE_FILE=deploy/compose/docker-compose.yml
COMPOSE=docker compose -f $(COMPOSE_FILE)
DOCKER_IMAGE?=nagisa:$(VERSION)
# Registry prefix for the base images (node/golang/alpine, all Docker Hub).
# Empty means docker.io; `make docker-build REGISTRY_MIRROR=docker.1panel.live/`
# is the fix when docker.io is unreachable. Keep it at the host level — the
# Dockerfile adds `library/` itself.
REGISTRY_MIRROR?=

.PHONY: init
# init env
init:
	go install github.com/google/wire/cmd/wire@v0.7.0
	go install github.com/bufbuild/buf/cmd/buf@latest

.PHONY: config
# generate internal proto
config:
	buf generate --template buf.gen.config.yaml

.PHONY: api
# generate api proto
api:
	buf generate --template buf.gen.yaml

.PHONY: build
# build the service binary into ./bin
build:
	mkdir -p bin/ && go build -ldflags "$(LDFLAGS)" -o ./bin/ ./cmd/...

.PHONY: docker-build
# build the container image (front end + service, alpine runtime)
docker-build:
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg REGISTRY_MIRROR=$(REGISTRY_MIRROR) \
		--build-arg REVISION=$(REVISION) \
		--build-arg CREATED=$(BUILD_TIME) \
		-t $(DOCKER_IMAGE) .

.PHONY: docker-up
# start the compose stack (nagisa + SeaweedFS) in the background
docker-up:
	$(COMPOSE) up -d --build

.PHONY: docker-down
# stop the compose stack, keeping both data volumes
docker-down:
	$(COMPOSE) down

.PHONY: docker-logs
# follow the service log of the compose stack
docker-logs:
	$(COMPOSE) logs -f nagisa

.PHONY: generate
# generate
generate:
	go generate ./...
	go mod tidy

.PHONY: docs
# enrich the generated OpenAPI document and publish it under docs/
docs:
	go run ./tools/openapi -in openapi.yaml -out docs

.PHONY: all
# generate all
all:
	make api
	make config
	make generate
	make docs

.PHONY: test
# run the unit tests
test:
	go test ./...

.PHONY: test-integration
# run the end to end tests against a running server
test-integration:
	go test -tags integration ./test/integration/ -v

# show help
help:
	@echo ''
	@echo 'Usage:'
	@echo ' make [target]'
	@echo ''
	@echo 'Targets:'
	@awk '/^[a-zA-Z\-\_0-9]+:/ { \
	helpMessage = match(lastLine, /^# (.*)/); \
		if (helpMessage) { \
			helpCommand = substr($$1, 0, index($$1, ":")); \
			helpMessage = substr(lastLine, RSTART + 2, RLENGTH); \
			printf "\033[36m%-22s\033[0m %s\n", helpCommand,helpMessage; \
		} \
	} \
	{ lastLine = $$0 }' $(MAKEFILE_LIST)

.DEFAULT_GOAL := help
