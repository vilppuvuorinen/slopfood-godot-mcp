# syntax=docker/dockerfile:1

BINARY := go-dot-mcp
CMD := ./cmd/go-dot-mcp
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

GO ?= go
GOFLAGS ?=
TOOLS_MOD := tools/go.mod
GOLANGCI_LINT ?= $(GO) tool -modfile=$(CURDIR)/$(TOOLS_MOD) golangci-lint

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

# godot downloads the pinned engine builds into .godot/. With GODOT_TEMPLATES=1
# it also fetches the matching export templates into .godot/godot/export_templates,
# where the e2e target's XDG_DATA_HOME points Godot.
.PHONY: godot
godot:
	scripts/download-godot.py install

# e2e drives a pre-built server binary with the official MCP SDK client. It is
# a standalone black-box module, so the SDK never enters the server's module
# graph. The binary is built first and handed to the tests via GO_DOT_MCP_BIN.
#
# The e2e module has no non-test code, so -covermode here measures nothing; the
# server is a child process. Use e2e-cover for real coverage of the server.
.PHONY: e2e
e2e: build
	cd e2e && XDG_DATA_HOME='$(abspath $(CURDIR)/.godot)' GO_DOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)' $(GO) test $(GOFLAGS) -count=1 ./...

# e2e-cover is e2e with a coverage-instrumented server binary. Building with
# -cover and pointing every spawned server at a shared GOCOVERDIR makes each
# process write counter files, which are merged into a profile. This is the only
# view of the engine-facing half of the server: unit tests mock Godot out and
# the black-box e2e module cannot instrument the server's packages.
#
# The same run also measures both embedded GDScript scripts. The -tags gdcover
# build instruments godot_operations.gd and validate_script.gd before Godot sees
# them; every spawned engine appends each script's hits to its own trace
# (GDCOVER_FILE and GDCOVER_VALIDATE_FILE). Those traces become two LCOV reports
# afterwards, so one engine run yields three profiles.
COVERDIR ?= $(CURDIR)/coverage-e2e
E2E_COVERPROFILE := coverage-e2e.out
GDCOVERDIR ?= $(CURDIR)/coverage-gdscript
GDCOVER_HITS := $(GDCOVERDIR)/hits.txt
GDCOVER_PROFILE := coverage-gdscript.info
GDCOVER_VALIDATE_HITS := $(GDCOVERDIR)/validate-hits.txt
GDCOVER_VALIDATE_PROFILE := coverage-validate-gdscript.info

.PHONY: e2e-cover
e2e-cover:
	rm -rf '$(COVERDIR)' '$(GDCOVERDIR)' $(E2E_COVERPROFILE) $(GDCOVER_PROFILE) $(GDCOVER_VALIDATE_PROFILE)
	mkdir -p '$(COVERDIR)' '$(GDCOVERDIR)'
	$(GO) build $(GOFLAGS) -cover -tags gdcover -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-cover $(CMD)
	cd e2e && XDG_DATA_HOME='$(abspath $(CURDIR)/.godot)' GO_DOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)-cover' GOCOVERDIR='$(COVERDIR)' GDCOVER_FILE='$(GDCOVER_HITS)' GDCOVER_VALIDATE_FILE='$(GDCOVER_VALIDATE_HITS)' $(GO) test $(GOFLAGS) -count=1 ./...
	$(GO) tool covdata textfmt -i='$(COVERDIR)' -o=$(E2E_COVERPROFILE)
	@$(GO) tool cover -func=$(E2E_COVERPROFILE) | tail -1
	$(GO) run ./cmd/gdcover lcov -source internal/godotops/godot_operations.gd -hits '$(GDCOVER_HITS)' -out $(GDCOVER_PROFILE)
	@printf 'GDScript bridge coverage written to %s\n' '$(GDCOVER_PROFILE)'
	$(GO) run ./cmd/gdcover lcov -source internal/godotops/validate_script.gd -hits '$(GDCOVER_VALIDATE_HITS)' -out $(GDCOVER_VALIDATE_PROFILE)
	@printf 'GDScript validate-script coverage written to %s\n' '$(GDCOVER_VALIDATE_PROFILE)'

.PHONY: bench
bench:
	$(GO) test $(GOFLAGS) -run '^$$' -bench . -benchmem ./...

.PHONY: cover
cover: test
	$(GO) tool cover -html=coverage.out

.PHONY: fmt
fmt:
	$(GO) fmt ./...
	cd e2e && $(GO) fmt ./...

.PHONY: vet
vet:
	$(GO) vet ./...
	cd e2e && $(GO) vet ./...

# lint covers the e2e module too; it is a separate module, so it must be
# linted from its own directory.
.PHONY: lint
lint:
	$(GOLANGCI_LINT) run ./...
	cd e2e && $(GOLANGCI_LINT) run ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy
	cd e2e && $(GO) mod tidy

.PHONY: check
check: fmt vet lint test e2e

.PHONY: clean
clean:
	rm -rf bin coverage.out $(E2E_COVERPROFILE) '$(COVERDIR)' $(GDCOVER_PROFILE) $(GDCOVER_VALIDATE_PROFILE) '$(GDCOVERDIR)'

.PHONY: help
help:
	@$(GO) doc -all . >/dev/null 2>&1 || true
	@printf 'Targets:\n'
	@printf '  build    compile %s into bin/\n' '$(BINARY)'
	@printf '  run      run the server (ARGS=...)\n'
	@printf '  test     run tests with coverage\n'
	@printf '  godot    download pinned Godot builds (GODOT_TEMPLATES=1 adds export templates)\n'
	@printf '  e2e      build, then run the SDK-backed black-box tests\n'
	@printf '  e2e-cover  e2e coverage: Go to %s, GDScript bridge to %s and %s\n' '$(E2E_COVERPROFILE)' '$(GDCOVER_PROFILE)' '$(GDCOVER_VALIDATE_PROFILE)'
	@printf '  fmt/vet/lint/check  code quality\n'
	@printf '  clean    remove build artifacts\n'
