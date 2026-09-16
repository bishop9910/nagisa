# Windows without GNU make: use the equivalent PowerShell entry point instead,
# .\scripts\build.ps1 <target>. It implements every target below.
GOHOSTOS:=$(shell go env GOHOSTOS)
GOPATH:=$(shell go env GOPATH)
VERSION=$(shell git describe --tags --always)

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
	mkdir -p bin/ && go build -ldflags "-X main.Version=$(VERSION)" -o ./bin/ ./cmd/...

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
