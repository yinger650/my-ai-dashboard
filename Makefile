SHELL := /bin/bash
GO ?= go
PNPM ?= pnpm
BIN_DIR := bin
LDFLAGS := -X main.version=0.1.0 -X main.commit=$(shell git rev-parse --short HEAD 2>/dev/null || echo dev) -X main.buildTime=$(shell date -u +%Y-%m-%dT%H:%M:%SZ)

.PHONY: help
help:
	@echo "Targets: dev test test-go test-web build build-web build-all lint migrate-check clean"

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
build-all: build-web
	mkdir -p $(BIN_DIR)
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-server-linux-amd64 ./cmd/board-server
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-server-linux-arm64 ./cmd/board-server
	GOOS=linux GOARCH=amd64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-client-linux-amd64 ./cmd/board-client
	GOOS=linux GOARCH=arm64 $(GO) build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/board-client-linux-arm64 ./cmd/board-client

.PHONY: lint
lint:
	$(GO) vet ./...
	gofmt -l cmd internal

.PHONY: migrate-check
migrate-check:
	$(GO) test ./internal/store/ -run TestIngest -count=1

.PHONY: clean
clean:
	rm -rf $(BIN_DIR) web/dist/assets .dev-data
