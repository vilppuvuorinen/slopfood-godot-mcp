# go-dot-mcp

A zero-dependency Go rewrite of the [Coding-Solo/godot-mcp](https://github.com/Coding-Solo/godot-mcp) forks.

Zero dependencies is a hard requirement: the standard library is the only dependency, ever.

## Tools and `--enable` categories

This server implements the headless subset of the 157-tool
[`tugcantopaloglu/godot-mcp`](https://github.com/tugcantopaloglu/godot-mcp) fork:
tools that drive Godot with `--headless` and never need a running game or a
display. The `game_*` runtime tools (TCP bridge), `launch_editor`, and the
run/stop/debug process-control tools are out of scope.

Tools are grouped into categories by what they act on. `project` is registered
by default; passing `--enable` replaces that default, so `project` must be named
explicitly to keep it. The flag is repeatable and accepts comma-separated names;
`all` selects every category.

```
go-dot-mcp                                     # project only
go-dot-mcp --enable scene,script               # scene and script, no project
go-dot-mcp --enable project --enable node      # project and node
go-dot-mcp --enable all                        # everything
```

**Progress: 35/35 tools implemented.**

### project (default) — project discovery and creation (4/4)

- [x] `get_godot_version`
- [x] `list_projects`
- [x] `get_project_info`
- [x] `create_project`

### settings — project configuration (8/8)

- [x] `read_project_settings`
- [x] `modify_project_settings`
- [x] `set_main_scene`
- [x] `manage_autoloads`
- [x] `manage_input_map`
- [x] `manage_layers`
- [x] `manage_plugins`
- [x] `manage_translations`

### export — export presets and builds (2/2)

- [x] `manage_export_presets`
- [x] `export_project`

### scene — scene files (4/4)

- [x] `create_scene`
- [x] `read_scene`
- [x] `save_scene`
- [x] `export_mesh_library`

### node — nodes within scenes (6/6)

- [x] `add_node`
- [x] `load_sprite`
- [x] `modify_scene_node`
- [x] `remove_scene_node`
- [x] `manage_scene_structure`
- [x] `manage_scene_signals`

### script — GDScript and C# authoring (5/5)

- [x] `create_script`
- [x] `create_csharp_script`
- [x] `attach_script`
- [x] `validate_script`
- [x] `validate_scripts`

### resources — resources, shaders, and themes (6/6)

- [x] `create_resource`
- [x] `manage_resource`
- [x] `manage_shader`
- [x] `manage_theme_resource`
- [x] `get_uid`
- [x] `update_project_uids`

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
`structuredContent` alongside its text.

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
```

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
XDG_DATA_HOME="$(cd .. && pwd)/.godot" GO_DOT_MCP_BIN="$(cd .. && pwd)/bin/go-dot-mcp" go test ./...
```

CI tests against the latest Godot builds rather than a pinned manifest:
`.github/workflows/godot-latest.yml` resolves the supported minor lines and
downloads the newest matching release (dev/beta/rc included), so a future
release surfaces breakage with no manifest change.

## Tooling

Lint tooling is pinned and isolated in the `tools/` module so it never enters
the server's dependency graph. `make lint` runs the pinned `golangci-lint` via
`go tool -modfile=tools/go.mod golangci-lint`. CI invokes `make lint` instead of
fetching a moving `latest` release. Formatting is plain `gofmt` (`make fmt`,
checked with `gofmt -l .`).
