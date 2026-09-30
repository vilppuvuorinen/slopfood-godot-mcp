BINARY := slopfood-godot-mcp
CMD := ./cmd/slopfood-godot-mcp
# VERSION is stamped via -X main.version; local builds default to "dev".
VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

# Release staging directory and the os/arch pairs `build` cross-compiles.
DIST := dist
RELEASE_TARGETS := linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64 windows/arm64

GO ?= go
GOFLAGS ?=
TOOLS_MOD := tools/go.mod
GOLANGCI_LINT ?= $(GO) tool -modfile=$(CURDIR)/$(TOOLS_MOD) golangci-lint

.PHONY: all
all: fmt vet test build

# Cross-compile every release target into dist/ and write checksums.txt, then
# guard portability by building/vetting for Windows and macOS.
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

# Download the pinned Godot builds into .godot/; GODOT_TEMPLATES=1 also fetches
# export templates.
.PHONY: godot
godot:
	scripts/download-godot.py install

# E2E_PARALLEL caps concurrent e2e tests (go test -parallel); each engine is a
# full headless Godot process.
E2E_PARALLEL ?= 4

# Build a server with -tags devtransport and run the black-box SDK e2e suite
# against it. Use e2e-cover for server coverage; the e2e module itself is tests
# only.
.PHONY: e2e
e2e:
	$(GO) build $(GOFLAGS) -tags devtransport -trimpath -ldflags '$(LDFLAGS)' -o bin/$(BINARY)-dev $(CMD)
	cd e2e && XDG_DATA_HOME='$(abspath $(CURDIR)/.godot)' GODOT_MCP_BIN='$(CURDIR)/bin/$(BINARY)-dev' $(GO) test $(GOFLAGS) -parallel $(E2E_PARALLEL) -count=1 ./...

# e2e with Go coverage (-cover + GOCOVERDIR) and GDScript bridge coverage
# (-tags gdcover + per-process hit traces), yielding three profiles.
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

# Fail when any Go file is not gofmt-clean; `fmt` fixes it.
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

# Lint both modules; e2e is a separate module.
.PHONY: lint
lint:
	$(GOLANGCI_LINT) run ./...
	cd e2e && $(GOLANGCI_LINT) run ./...

.PHONY: tidy
tidy:
	$(GO) mod tidy
	cd e2e && $(GO) mod tidy

# Godot-free half of the gate: formatting, vet, lint, unit tests, multi-platform
# build.
.PHONY: static
static: fmt-check vet lint test build

# Full local gate: fetch Godot, then static plus the engine-backed e2e suite.
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
