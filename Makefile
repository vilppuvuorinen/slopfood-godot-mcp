# syntax=docker/dockerfile:1

BINARY := go-dot-mcp
CMD := ./cmd/go-dot-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

GO ?= go
GOFLAGS ?=
GOLANGCI_LINT ?= golangci-lint

.PHONY: all
all: fmt vet test build

.PHONY: build
build:
	$(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY) $(CMD)

.PHONY: install
install:
	$(GO) install $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' $(CMD)

.PHONY: run
run:
	$(GO) run $(GOFLAGS) $(CMD) $(ARGS)

.PHONY: test
test:
	$(GO) test $(GOFLAGS) -race -covermode=atomic -coverprofile=coverage.out ./...

.PHONY: bench
bench:
	$(GO) test $(GOFLAGS) -run '^$$' -bench . -benchmem ./...

.PHONY: cover
cover: test
	$(GO) tool cover -html=coverage.out

.PHONY: fmt
fmt:
	$(GO) fmt ./...

.PHONY: vet
vet:
	$(GO) vet ./...

.PHONY: lint
lint:
	$(GOLANGCI_LINT) run ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy

.PHONY: check
check: fmt vet lint test

.PHONY: clean
clean:
	rm -rf bin coverage.out

.PHONY: help
help:
	@$(GO) doc -all . >/dev/null 2>&1 || true
	@printf 'Targets:\n'
	@printf '  build    compile %s into bin/\n' '$(BINARY)'
	@printf '  run      run the server (ARGS=...)\n'
	@printf '  test     run tests with race detector and coverage\n'
	@printf '  fmt/vet/lint/check  code quality\n'
	@printf '  clean    remove build artifacts\n'
