# slopfood-godot-mcp

A zero-dependency Go MCP server for Godot. Zero dependencies is a hard requirement: the standard library is the only dependency, ever.

## Project binding

One server process is bound to one Godot project, chosen at spawn in order of
precedence: `--project /path`, then `$GODOT_PROJECT`, then the working
directory. The bound path is the only place a caller can learn which project is
in scope: it is advertised in the server's instructions (both the legacy
`initialize` handshake and the stateless `server/discover` result) and echoed
by the `projectPath` field of the status envelope.

Tools take paths relative to the bound project and never accept a project
argument, so there is no cursor, select tool, or per-call override. A second
project is a second server entry:

```
slopfood-godot-mcp --project /path/to/game --enable all
GODOT_PROJECT=/path/to/game slopfood-godot-mcp --enable all
```

## Path conventions

File paths in tool arguments and results are project-relative and always use
forward slashes (`scenes/main.tscn`, `scripts/player.gd`); the engine's
`res://` prefix is not leaked into results. Node paths (`nodePath`,
`sourcePath`, `parentNodePath`) are relative to the scene root, not to the
project.

The bound project's absolute location is the one deliberate exception: the
`projectPath` field of the status envelope reports it, so a caller can tell
which project is in scope. Every other path-valued result field is
project-relative.

## Tools and `--enable` categories

Tools are grouped into categories by what they act on. Every category is
registered by default; passing `--enable` replaces that default, so any category
to keep must be named explicitly. The flag is repeatable and accepts
comma-separated names; `all` selects every category.

```
slopfood-godot-mcp                                     # every category
slopfood-godot-mcp --enable scene,script               # scene and script, no project
slopfood-godot-mcp --enable project --enable node      # project and node
slopfood-godot-mcp --enable all                        # every category
```

## Protocol

The server speaks the MCP stdio transport and negotiates protocol revisions
`2024-11-05` through `2026-07-28`. Revision `2026-07-28` (SEP-2575) is
stateless: a client calls `server/discover` to learn the supported versions,
capabilities, and identity without a handshake, then carries its protocol
version and capabilities in each request's `_meta`. The server answers with
`resultType`, its identity in `_meta`, and cache hints (`ttlMs`/`cacheScope`)
on list and discovery results. Legacy clients negotiate with `initialize`,
which never offers a stateless revision, and are served the newest legacy
revision, `2025-11-25`. Every tool advertises a human-readable title, behavior
annotations, and an `outputSchema`, and returns structured results through
`structuredContent` alongside its text. Error results are marked `isError` and
also carry `structuredContent` with `{"success": false, "message": ...}`: an
error is spec-permissible without a structured body, but the envelope keeps the
declared mutation `outputSchema` satisfied for clients that validate it eagerly.

## Security

Path validation (`validPath`/`confinePath`/`confineAbsPath`) is a lexical
convenience check, not a security boundary. It is easily bypassed (symlinks,
platform-specific path forms) and must not be relied on for isolation. Run the
server under an unprivileged user or sandbox and treat that as the real line of
defense.

## Testing

```
make test
make e2e
make e2e-cover
```

`make test` measures only the Go half of the server: the GDScript bridge and the
engine-facing code run in a child process. `make e2e-cover` builds the server
with `go build -cover`, runs the black-box suite with the spawned processes
writing to a shared `GOCOVERDIR`, and merges the counters into
`coverage-e2e.out`, which is the only profile that includes the engine-facing
paths.

The same run also measures both embedded GDScript scripts. `make e2e-cover`
builds with `-tags gdcover`, which makes the server instrument
`godot_operations.gd` and `validate_script.gd` before Godot loads them. Each
spawned engine appends each script's hit trace to its own shared file, and the
traces are converted to `coverage-gdscript.info` and
`coverage-validate-gdscript.info` (LCOV), so one engine run yields all three
profiles. The instrumenter lives in `internal/gdcover` and is only linked into
the `gdcover` build, never a release binary.

Coverage profiles are generated artifacts. `make e2e-cover` deletes and rewrites
them on every run, `make clean` removes them, and they are gitignored. A profile
left on disk may therefore predate the current code, so rerun the target before
reading one.

Godot-dependent tests need an engine and export templates in the repo-local
`.godot/`; templates are opt-in:

```
GODOT_TEMPLATES=1 make godot
```

To run the e2e suite by hand, build first and set both variables (an absolute,
`..`-free path is required):

```
make build
cd e2e
XDG_DATA_HOME="$(cd .. && pwd)/.godot" GODOT_MCP_BIN="$(cd .. && pwd)/bin/slopfood-godot-mcp" go test -parallel 4 ./...
```

Tests run in parallel, bounded by `go test -parallel` (default 4 via
`E2E_PARALLEL` for `make e2e`). Each test uses its own temp project and runtime
directory; the bound exists because a headless Godot process costs hundreds of
megabytes of RSS, so raise it only if the host has memory to spare. `make
e2e-cover` runs with `-parallel 1` while the GDScript hit traces are keyed per
script rather than per process.

CI runs on `main` and manual dispatch only. Every run does the `make check`
surface, then tests against the latest Godot builds rather than a pinned
manifest: `.github/workflows/ci.yml` resolves the supported minor lines and
downloads the newest matching release (dev/beta/rc included), so a future
release surfaces breakage with no manifest change.

## Releasing

Dispatch the `CI` workflow with a `version` input (`vMAJOR.MINOR.PATCH`) to tag
the resolved commit and publish cross-compiled linux/darwin/windows binaries
(amd64 and arm64) as a GitHub Release. Dispatch against any ref to release that
commit; the release job only runs once every check, including the full Godot
matrix, has passed.

## Tooling

Lint tooling is pinned and isolated in the `tools/` module so it never enters
the server's dependency graph. `make lint` runs the pinned `golangci-lint` via
`go tool -modfile=tools/go.mod golangci-lint`. CI invokes `make lint` instead of
fetching a moving `latest` release. Formatting is plain `gofmt` (`make fmt`,
checked with `gofmt -l .`).
