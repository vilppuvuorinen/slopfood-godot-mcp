# syntax=docker/dockerfile:1

BINARY := go-dot-mcp
CMD := ./cmd/go-dot-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

GO ?= go
GOFLAGS ?=
TOOLS_MOD := tools/go.mod
GOLANGCI_LINT ?= $(GO) tool -modfile=$(TOOLS_MOD) golangci-lint

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
	$(GO) test $(GOFLAGS) -covermode=atomic -coverprofile=coverage.out ./...

# e2e drives a pre-built server binary with the official MCP SDK client. It is
# a standalone black-box module, so the SDK never enters the server's module
# graph. The binary is built first and handed to the tests via GO_DOT_MCP_BIN.
.PHONY: e2e
e2e: build
	cd e2e && GO_DOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)' $(GO) test $(GOFLAGS) -covermode=atomic ./...

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
check: fmt vet lint test e2e

.PHONY: clean
clean:
	rm -rf bin coverage.out

.PHONY: help
help:
	@$(GO) doc -all . >/dev/null 2>&1 || true
	@printf 'Targets:\n'
	@printf '  build    compile %s into bin/\n' '$(BINARY)'
	@printf '  run      run the server (ARGS=...)\n'
	@printf '  test     run tests with coverage\n'
	@printf '  e2e      build, then run the SDK-backed black-box tests\n'
	@printf '  fmt/vet/lint/check  code quality\n'
	@printf '  clean    remove build artifacts\n'
