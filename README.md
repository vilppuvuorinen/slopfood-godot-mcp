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
- **Editor session as the engine binding.** One long-lived editor-mode Godot
  process per bound project, reached over a Unix domain socket (loopback TCP on
  Windows). That process both serves operations and owns `EditorFileSystem`, so
  imports, `.uid` sidecars, and the global class cache are reconciled by the
  same engine that serves the edits. No one-shot engine spawn per call.
- **Narrower toolset.** It exposes only operations that cannot be done with
  plain text edits: scene, node, resource, script, and export mutation, plus
  validation and reimport. Tilemaps, tilesets, themes, and animations are read
  back through dedicated tools, but their edits go through `run_script`.
  Upstream's runtime tools (`run_project`, `stop_project`, `get_debug_output`,
  `launch_editor`) are not present.
- **No runtime dependencies.** The server is built from the Go standard library
  only.

This project is slop at heart. It was assembled quickly, it is tailored to its
author's workflow, and it has not been audited.
