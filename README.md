# slopfood-godot-mcp

A headless Godot MCP server written in Go. It follows the approach set out by
Coding-Solo and its forks: it stays deliberately narrow and does only the things
that cannot be done with plain text edits, leaving everything else to the
caller's own tools.

It stays zero-dependency, using only the Go standard library.

It is tested against Godot 4.7.x and forward, on Linux only. Portability is not
ruled out, but it is not a high priority: non-Linux builds are untested and
serve only to keep further platform dependencies from creeping in.
