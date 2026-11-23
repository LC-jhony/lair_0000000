GO ?= go
BIN := lair

.PHONY: all build test test-race fmt vet lint coverage clean

all: test build

build:
	$(GO) build -o $(BIN) ./cmd/lair

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
