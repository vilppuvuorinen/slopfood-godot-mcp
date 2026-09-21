# go-dot-mcp

A zero-dependency Go rewrite of the [Coding-Solo/godot-mcp](https://github.com/Coding-Solo/godot-mcp) forks.

Zero dependencies is a hard requirement: the standard library is the only dependency, ever.

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
