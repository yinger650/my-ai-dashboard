SHELL := /bin/bash
GO ?= go
PNPM ?= pnpm
BIN_DIR := bin
VERSION ?= 0.1.10
COMMIT ?= $(shell git rev-parse HEAD 2>/dev/null || echo unknown)
BUILD_TIME ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.buildTime=$(BUILD_TIME)
DIST_CLIENT := dist/client
CLIENT_UPDATE_DIR ?= /var/lib/agentboard/client-updates

.PHONY: help
help:
	@echo "Targets: dev test test-go test-web build build-web build-all dist-client install-client-updates lint migrate-check clean"

## dev: run board-server (:8080) and the Vite dev server (:5173) together
.PHONY: dev
dev:
	@echo "Starting board-server on :8080 and Vite on :5173..."
	@ABP_DATA_DIR=$${ABP_DATA_DIR:-.dev-data} ABP_LISTEN_ADDR=127.0.0.1:8080 ABP_SECURE_COOKIES=false ABP_PUBLIC_URL=http://127.0.0.1:8080 $(GO) run ./cmd/board-server run & \
	$(PNPM) --dir web dev; \
	wait

## test: all unit tests (Go + web)
.PHONY: test
test: test-go test-web

.PHONY: test-go
test-go:
	$(GO) test ./...

.PHONY: test-web
test-web:
	$(PNPM) --dir web test

## build: build the frontend, embed it, and build both binaries for the host
.PHONY: build
build: build-web
	mkdir -p $(BIN_DIR)
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-server ./cmd/board-server
	$(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-client ./cmd/board-client

.PHONY: build-web
build-web:
	$(PNPM) --dir web install --frozen-lockfile
	$(PNPM) --dir web build

## build-all: cross-compile linux amd64 + arm64 binaries
.PHONY: build-all
build-all: build-web dist-client
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-server-linux-amd64 ./cmd/board-server
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-server-linux-arm64 ./cmd/board-server
	cp $(DIST_CLIENT)/board-client-linux-amd64 $(BIN_DIR)/board-client-linux-amd64
	cp $(DIST_CLIENT)/board-client-linux-arm64 $(BIN_DIR)/board-client-linux-arm64

## dist-client: linux amd64 + arm64 board-client + manifest for GitHub Releases
.PHONY: dist-client
dist-client:
	mkdir -p $(DIST_CLIENT)
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_CLIENT)/board-client-linux-amd64 ./cmd/board-client
	CGO_ENABLED=0 GOOS=linux GOARCH=arm64 $(GO) build -trimpath -ldflags "$(LDFLAGS)" -o $(DIST_CLIENT)/board-client-linux-arm64 ./cmd/board-client
	VERSION=$(VERSION) COMMIT=$(COMMIT) BUILD_TIME=$(BUILD_TIME) python3 scripts/client-manifest.py $(DIST_CLIENT)

.PHONY: install-client-updates
install-client-updates: dist-client
	install -d -m 755 $(CLIENT_UPDATE_DIR)
	install -m 644 $(DIST_CLIENT)/manifest.json $(DIST_CLIENT)/SHA256SUMS $(CLIENT_UPDATE_DIR)/
	install -m 755 $(DIST_CLIENT)/board-client-linux-amd64 $(DIST_CLIENT)/board-client-linux-arm64 $(CLIENT_UPDATE_DIR)/

.PHONY: lint
lint:
	$(GO) vet ./...
	gofmt -l cmd internal

.PHONY: migrate-check
migrate-check:
	$(GO) test ./internal/store/ -run TestIngest -count=1

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) dist web/dist/assets .dev-data
