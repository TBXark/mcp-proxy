BUILD_DIR=./build
BUILD=$(shell git rev-parse --short HEAD)@$(shell date +%s)
CURRENT_OS := $(shell uname -s | tr '[:upper:]' '[:lower:]')
CURRENT_ARCH := $(shell uname -m | tr '[:upper:]' '[:lower:]')
LD_FLAGS=-ldflags "-X main.BuildVersion=$(BUILD)"
GO_BUILD=CGO_ENABLED=0 go build $(LD_FLAGS)

.PHONY: build
build:
	$(GO_BUILD) -o $(BUILD_DIR)/ ./...

.PHONY: buildLinuxX86
buildLinuxX86:
	GOOS=linux GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR) ./...

.PHONY: buildMacIntel
buildMacIntel:
	GOOS=darwin GOARCH=amd64 $(GO_BUILD) -o $(BUILD_DIR)/mcp-proxy-intel ./...

.PHONY: buildImage
buildImage:
	docker buildx build --platform=linux/amd64,linux/arm64 -t ghcr.io/tbxark/map-proxy:latest . --push --provenance=false

.PHONY: lint
lint:
	golangci-lint run

.PHONY: format
format:
	golangci-lint fmt
	golangci-lint run --fix
	go fmt ./...
	go mod tidy