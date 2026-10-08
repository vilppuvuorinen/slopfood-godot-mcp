# slopfood-godot-mcp

A headless Godot MCP server written in Go. It is derived from
[Coding-Solo/godot-mcp](https://github.com/Coding-Solo/godot-mcp) and
[tugcantopaloglu/godot-mcp](https://github.com/tugcantopaloglu/godot-mcp): the
operations bridge is a substantially reworked version of that lineage's
GDScript, and the rest is a Go rewrite. See `THIRD_PARTY_NOTICES` for the
licenses.

What it does differently from upstream:

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
