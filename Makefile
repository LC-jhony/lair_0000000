GO ?= go
BIN := lair

VERSION ?= $(shell git describe --tags --always 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
DATE ?= $(shell git log -1 --format=%cd --date=format:%Y-%m-%d 2>/dev/null || echo unknown)
VERSION_PKG := github.com/lair-sh/lair/internal/version
LDFLAGS := -s -w \
	-X $(VERSION_PKG).BuildVersion=$(VERSION) \
	-X $(VERSION_PKG).BuildCommit=$(COMMIT) \
	-X $(VERSION_PKG).BuildDate=$(DATE)

.PHONY: all build test test-race fmt vet lint coverage clean

all: test build

build:
	$(GO) build -buildmode=pie -trimpath -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/lair

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

fmt:
	$(GO) fmt ./...

vet:
	$(GO) vet ./...

# golangci-lint (config en .golangci.yml). Si no está instalado, go vet como fallback.
lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint no encontrado; ejecutando go vet ./... como fallback" >&2; \
		$(GO) vet ./...; \
	fi

coverage:
	$(GO) test -coverprofile=coverage.out ./...
	$(GO) tool cover -func=coverage.out | tail -n 1

clean:
	rm -f $(BIN)
