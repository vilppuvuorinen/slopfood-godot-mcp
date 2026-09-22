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

**Progress: 19/35 tools implemented.**

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

### export — export presets and builds (0/2)

- [ ] `manage_export_presets`
- [ ] `export_project`

### scene — scene files (3/4)

- [x] `create_scene`
- [ ] `read_scene`
- [x] `save_scene`
- [x] `export_mesh_library`

### node — nodes within scenes (2/6)

- [x] `add_node`
- [x] `load_sprite`
- [ ] `modify_scene_node`
- [ ] `remove_scene_node`
- [ ] `manage_scene_structure`
- [ ] `manage_scene_signals`

### script — GDScript and C# authoring (0/5)

- [ ] `create_script`
- [ ] `create_csharp_script`
- [ ] `attach_script`
- [ ] `validate_script`
- [ ] `validate_scripts`

### resources — resources, shaders, and themes (2/6)

- [ ] `create_resource`
- [ ] `manage_resource`
- [ ] `manage_shader`
- [ ] `manage_theme_resource`
- [x] `get_uid`
- [x] `update_project_uids`

## Testing

`make test` runs the unit tests in the server module, which stays dependency-free.

`make e2e` builds the server and drives the resulting binary as a subprocess
with a real client from the official
[`modelcontextprotocol/go-sdk`](https://github.com/modelcontextprotocol/go-sdk).
The test is a standalone black-box module under `e2e/` with its own `go.mod`; it
never imports the server, so the SDK stays out of the server's module graph and
build. To run it by hand, build first and pass the path:

```
make build
GO_DOT_MCP_BIN=$PWD/bin/go-dot-mcp go test ./e2e/...
```

## Tooling

Lint tooling is pinned and isolated in the `tools/` module so it never enters
the server's dependency graph. `make lint` runs the pinned `golangci-lint` via
`go tool -modfile=tools/go.mod golangci-lint`. CI invokes `make lint` instead of
fetching a moving `latest` release. Formatting is plain `gofmt` (`make fmt`,
checked with `gofmt -l .`).
