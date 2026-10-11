# slopfood-godot-mcp

A headless Godot MCP server written in Go. It is derived from
[Coding-Solo/godot-mcp](https://github.com/Coding-Solo/godot-mcp) and
[tugcantopaloglu/godot-mcp](https://github.com/tugcantopaloglu/godot-mcp): the
operations bridge is a substantially reworked version of that lineage's
GDScript, and the rest is a Go rewrite. See `THIRD_PARTY_NOTICES` for the
licenses.

## Contents

- [Requirements](#requirements)
- [Install](#install)
- [Configure your MCP client](#configure-your-mcp-client)
- [Command line](#command-line)
- [Environment](#environment)
- [Tools](#tools)
- [Security](#security)
- [How it differs from upstream](#how-it-differs-from-upstream)

## Requirements

- **Godot 4.7 or newer.** The server resolves the engine in this order:
  `$GODOT_PATH`, a local `.godot/<minor>/Godot_v*` build (searched upward from
  the working directory and from the executable's directory), then `godot` on
  `PATH`.
- **An MCP client** that speaks stdio, such as Claude Desktop, Cursor, VS Code,
  or any client built on the MCP Go/Python/TypeScript SDKs.
- Building from source needs **Go 1.27 or newer**. The prebuilt binaries are
  static and have no runtime dependencies.

## Install

### Prebuilt binary

Linux (x86-64):

```sh
curl -fsSL -o sf-godot-mcp \
  https://github.com/vilppuvuorinen/slopfood-godot-mcp/releases/download/latest/slopfood-godot-mcp_<version>_linux_amd64
chmod +x sf-godot-mcp
sudo install -m 0755 sf-godot-mcp /usr/local/bin/sf-godot-mcp
```

macOS (Apple silicon):

```sh
curl -fsSL -o sf-godot-mcp \
  https://github.com/vilppuvuorinen/slopfood-godot-mcp/releases/download/latest/slopfood-godot-mcp_<version>_darwin_arm64
chmod +x sf-godot-mcp
mv sf-godot-mcp /usr/local/bin/sf-godot-mcp
```

## Configure your MCP client

Point the client at `sf-godot-mcp` and name the project it should bind to. The
project comes from `--project`, then the process working directory. Most clients
do not set a useful working directory, so pass the project explicitly:

```json
{
  "mcpServers": {
    "godot": {
      "command": "sf-godot-mcp",
      "args": ["--project", "/absolute/path/to/your/godot/project"]
    }
  }
}
```

Each bound project gets its own long-lived editor process (see
[How it differs from upstream](#how-it-differs-from-upstream)); it starts on
first use and shuts down when idle.

## Command line

```text
sf-godot-mcp [flags]
```

| Flag | Default | Description |
| --- | --- | --- |
| `--project` | cwd | Godot project directory to bind to |
| `--log-level` | `warn` | `debug`, `info`, `warn`, or `error` |
| `--version` | | Print the version and exit |

## Environment

| Variable | Description |
| --- | --- |
| `GODOT_PATH` | Path to the Godot executable; overrides engine discovery |

The server logs to stderr.

## Tools

| Tool | Writes | Summary |
| --- | --- | --- |
| `create_project` | yes | Scaffold a new project (optionally .NET/C#) in the bound root |
| `validate` | no | Parse `project.godot` and every GDScript, returning diagnostics |
| `read` | no | Read a scene, animation, tileset, theme, or binary resource |
| `read_node` | no | Read a node subtree or a tilemap's cells |
| `run_script` | yes | Run a GDScript body in the live editor session |
| `attach_script` | yes | Attach a GDScript to a scene node |
| `export_project` | yes | Export using a preset from `export_presets.cfg` |
| `export_mesh_library` | yes | Export a scene as a `MeshLibrary` resource |

Scenes, nodes, tilemaps, tilesets, themes, animations, and generic resources are
mutated through `run_script`; `create_project`, `attach_script`, and the two
export tools are the dedicated engine-owned operations.

## Security

The server is not a sandbox. `run_script` executes arbitrary GDScript in the
editor session with the privileges of the user running the server, so it can
read and write any file that user can, not only files under the bound project.
The path checks the other tools apply are lexical conveniences, not a security
boundary. Run the server only against projects and clients you trust.

## How it differs from upstream

- **Godot matrix.** CI builds and tests against every published minor from 4.7
  forward, including dev/beta/rc, rather than a single pinned version.
- **Linux, Windows, and macOS.** All three run the e2e suite in CI.
- **Editor session as the engine binding.** Most tools run against one
  long-lived editor-mode Godot process per bound project, reached over a Unix
  domain socket (loopback TCP on Windows). That process owns
  `EditorFileSystem`, so imports, `.uid` sidecars, and the global class cache
  are reconciled by the same engine that serves the edits. A few tools that
  need their own engine boot (`export_project`, `validate`) instead spawn a
  one-shot `--headless` process per call.
- **Augmenting, not replacing, text edits.** The server does not wrap every
  read and write; it fills the gaps a text editor leaves. `read` and
  `read_node` expose state that is awkward or impossible to read as text:
  scene trees, tilemaps, tilesets, themes, animations, and binary resources.
  `run_script` runs GDScript in the live editor session for edits plain text
  cannot express safely, which is how scenes, nodes, tilemaps, tilesets,
  themes, animations, and generic resources are mutated. A few engine-owned
  operations stay as dedicated tools: project scaffolding (`create_project`),
  script attachment (`attach_script`), export mutation (`export_project`,
  `export_mesh_library`), and validation (`validate`). A `project.godot` change
  restarts the editor session; if the file no longer parses, every session tool
  call fails with the engine's parse error until it is fixed, and `validate`
  reports both the project and script diagnostics. Imports and UID sidecars are
  reconciled automatically by the editor session, so there is no explicit
  rescan tool. Upstream's runtime
  tools (`run_project`, `stop_project`, `get_debug_output`, `launch_editor`) are
  not present.
- **No runtime dependencies.** The server is built from the Go standard library
  only.

This project is slop at heart. It was assembled quickly, it is tailored to its
author's workflow, and it has not been audited.
