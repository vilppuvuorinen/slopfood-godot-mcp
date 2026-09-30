# syntax=docker/dockerfile:1

BINARY := slopfood-godot-mcp
CMD := ./cmd/slopfood-godot-mcp
# VERSION stamps the binary through -X main.version. A local build defaults to a
# fixed "dev" marker; CI overrides it with the generated YYYY.MM.DD-<sha7>
# version for the commit it built (see .github/workflows/ci.yml). It is never
# derived from `git describe`: a release artifact must carry exactly the version
# the build job chose, and a local binary must be recognizable as a dev build.
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

# DIST is the release staging directory and RELEASE_TARGETS the os/arch pairs
# `build` cross-compiles into it, matching what the release job publishes.
DIST := dist
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

GO ?= go
GOFLAGS ?=
TOOLS_MOD := tools/go.mod
GOLANGCI_LINT ?= $(GO) tool -modfile=$(CURDIR)/$(TOOLS_MOD) golangci-lint

.PHONY: all
all: fmt vet test build

# build cross-compiles every published target into dist/, named
# <binary>_<version>_<os>_<arch>[.exe], and writes checksums.txt beside them.
# The release job and both CI e2e lanes consume exactly these files, so what the
# gate tests is what a release ships. It defines no build tags and no
# instrumentation: the debug-only devtransport and gdcover tags are reserved for
# the e2e targets, and any caller that needs extra debug flags passes them
# through GOFLAGS. The portability guard catches Linux-only code (e.g.
# syscall.Flock) on the push that introduces it rather than at the next release
# dispatch; vet is what compiles build-tagged *test* files, so a tagged file
# that does not compile also fails here.
.PHONY: build
build:
	rm -rf '$(DIST)'
	mkdir -p '$(DIST)'
	@set -e; for target in $(RELEASE_TARGETS); do \
		os=$${target%/*}; arch=$${target#*/}; \
		name='$(BINARY)_$(VERSION)_'$$os'_'$$arch; \
		if [ "$$os" = windows ]; then name=$$name.exe; fi; \
		printf 'building %s\n' "$$name"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch \
			$(GO) build $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' -o '$(DIST)/'$$name $(CMD); \
	done
	cd '$(DIST)' && sha256sum -- * > checksums.txt
	CGO_ENABLED=0 GOOS=windows GOARCH=amd64 $(GO) build ./...
	CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 $(GO) build ./...
	GOOS=windows $(GO) vet ./...
	GOOS=darwin $(GO) vet ./...
	cd e2e && GOOS=windows $(GO) vet ./...
	cd e2e && GOOS=darwin $(GO) vet ./...

.PHONY: install
install:
	$(GO) install $(GOFLAGS) -trimpath -ldflags '$(LDFLAGS)' $(CMD)

.PHONY: run
run:
	$(GO) run $(GOFLAGS) $(CMD) $(ARGS)

.PHONY: test
test:
	$(GO) test $(GOFLAGS) -race -covermode=atomic -coverprofile=coverage.out ./...

# godot downloads the pinned engine builds into .godot/. With GODOT_TEMPLATES=1
# it also fetches the matching export templates into .godot/godot/export_templates,
# where the e2e target's XDG_DATA_HOME points Godot.
.PHONY: godot
godot:
	scripts/download-godot.py install

# E2E_PARALLEL caps how many e2e tests drive the engine at once via go test
# -parallel. Each running test owns a project and a daemon that runs one Godot
# process at a time, and a headless Godot costs hundreds of megabytes of RSS, so
# the cap trades wall time for a bounded peak. Raise it on a host with more
# memory to spare.
E2E_PARALLEL ?= 4

# e2e drives a pre-built server binary with the official MCP SDK client. It is
# a standalone black-box module, so the SDK never enters the server's module
# graph. The binary is built first and handed to the tests via GODOT_MCP_BIN.
# Tests run in parallel, bounded by go test -parallel (E2E_PARALLEL); each test
# uses its own temp project and runtime dir, so the only shared resource to
# bound is the engine.
#
# The e2e build carries -tags devtransport, unlike `build`, so the transport
# override is reachable in the local TCP lane but never in a release binary. It
# is written to a -dev-named path under bin/, separate from the clean dist/
# matrix `build` produces, so a `make build` artifact stays free of debug flags
# no matter which targets ran before it.
#
# The e2e module has no non-test code, so -covermode here measures nothing; the
# server is a child process. Use e2e-cover for real coverage of the server.
.PHONY: e2e
e2e:
	$(GO) build $(GOFLAGS) -tags devtransport -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-dev $(CMD)
	cd e2e && XDG_DATA_HOME='$(abspath $(CURDIR)/.godot)' GODOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)-dev' $(GO) test $(GOFLAGS) -parallel $(E2E_PARALLEL) -count=1 ./...

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
#
# The suite still runs in parallel. Each server appends its PID to the hit-trace
# paths it hands its engines, so every server writes its own trace and concurrent
# engines never race on one file. gdcover lcov merges the per-server traces by
# globbing the paths.
COVERDIR ?= $(CURDIR)/coverage-e2e
E2E_COVERPROFILE := coverage-e2e.out
GDCOVERDIR ?= $(CURDIR)/coverage-gdscript
GDCOVER_HITS := $(GDCOVERDIR)/hits.txt
GDCOVER_HITS_GLOB := $(GDCOVER_HITS).*
GDCOVER_PROFILE := coverage-gdscript.info
GDCOVER_VALIDATE_HITS := $(GDCOVERDIR)/validate-hits.txt
GDCOVER_VALIDATE_HITS_GLOB := $(GDCOVER_VALIDATE_HITS).*
GDCOVER_VALIDATE_PROFILE := coverage-validate-gdscript.info

.PHONY: e2e-cover
e2e-cover:
	rm -rf '$(COVERDIR)' '$(GDCOVERDIR)' $(E2E_COVERPROFILE) $(GDCOVER_PROFILE) $(GDCOVER_VALIDATE_PROFILE)
	mkdir -p '$(COVERDIR)' '$(GDCOVERDIR)'
	$(GO) build $(GOFLAGS) -cover -tags gdcover -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-cover $(CMD)
	cd e2e && XDG_DATA_HOME='$(abspath $(CURDIR)/.godot)' GODOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)-cover' GOCOVERDIR='$(COVERDIR)' GDCOVER_FILE='$(GDCOVER_HITS)' GDCOVER_VALIDATE_FILE='$(GDCOVER_VALIDATE_HITS)' $(GO) test $(GOFLAGS) -parallel $(E2E_PARALLEL) -count=1 ./...
	$(GO) tool covdata textfmt -i='$(COVERDIR)' -o=$(E2E_COVERPROFILE)
	@$(GO) tool cover -func=$(E2E_COVERPROFILE) | tail -1
	$(GO) run ./cmd/gdcover lcov -source internal/godotops/godot_operations.gd -hits '$(GDCOVER_HITS_GLOB)' -out $(GDCOVER_PROFILE)
	@printf 'GDScript bridge coverage written to %s\n' '$(GDCOVER_PROFILE)'
	$(GO) run ./cmd/gdcover lcov -source internal/godotops/validate_script.gd -hits '$(GDCOVER_VALIDATE_HITS_GLOB)' -out $(GDCOVER_VALIDATE_PROFILE)
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

# fmt-check is fmt as a gate: it fails when any Go file is not gofmt-clean,
# listing the offenders, so CI can fail on a formatting diff. `fmt` stays the
# fix-it target. gofmt -l walks the tree, so it covers the e2e module too.
.PHONY: fmt-check
fmt-check:
	@files=$$(gofmt -l .); \
	if [ -n "$$files" ]; then \
		printf '%s\n' "$$files" >&2; \
		printf 'gofmt: files above are not formatted\n' >&2; \
		exit 1; \
	fi

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

# static is the half of the gate that needs no Godot: formatting, vet, lint,
# unit tests, and the multi-platform build. The build stages dist/, so the exact
# binaries the release job publishes are the ones this gate built and the e2e
# lanes exercised. CI runs it in the job that has no engine installed. `check`
# adds the engine-backed e2e suite.
.PHONY: static
static: fmt-check vet lint test build

# check is the full local gate. godot is first so the engine and templates are
# fetched before anything depends on them; e2e then drives the downloaded build.
.PHONY: check
check: godot static e2e

.PHONY: clean
clean:
	rm -rf bin '$(DIST)' coverage.out $(E2E_COVERPROFILE) '$(COVERDIR)' $(GDCOVER_PROFILE) $(GDCOVER_VALIDATE_PROFILE) '$(GDCOVERDIR)'

.PHONY: help
help:
	@$(GO) doc -all . >/dev/null 2>&1 || true
	@printf 'Targets:\n'
	@printf '  build    cross-compile %s into %s/ with no debug flags (VERSION=%s)\n' '$(BINARY)' '$(DIST)' '$(VERSION)'
	@printf '  run      run the server (ARGS=...)\n'
	@printf '  test     run tests with coverage\n'
	@printf '  godot    download pinned Godot builds (GODOT_TEMPLATES=1 adds export templates)\n'
	@printf '  e2e      build, then run the SDK-backed black-box tests in parallel (E2E_PARALLEL=%s)\n' '$(E2E_PARALLEL)'
	@printf '  e2e-cover  e2e coverage: Go to %s, GDScript bridge to %s and %s\n' '$(E2E_COVERPROFILE)' '$(GDCOVER_PROFILE)' '$(GDCOVER_VALIDATE_PROFILE)'
	@printf '  fmt-check/vet/lint  code quality (fmt fixes)\n'
	@printf '  static   fmt-check vet lint test build (no Godot needed)\n'
	@printf '  check    static plus godot and e2e\n'
	@printf '  clean    remove build artifacts\n'
