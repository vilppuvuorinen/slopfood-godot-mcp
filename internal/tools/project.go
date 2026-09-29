package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// Context is the per-call context a tool handler receives. It embeds
// context.Context so cancellation and values flow through unchanged, and adds
// the engine seam's vocabulary as methods on the handler's first argument: Run
// executes an operation against the bound project, and Invalidate asks the
// editor to rescan/reload. The engine is always a live session; a handler never
// branches on the engine's lifecycle.
type Context struct {
	context.Context
}

// Run executes one operations-script call through the live session bound to c.
func (c Context) Run(project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	return engineFrom(c.Context).Run(project, operation, params)
}

// Invalidate asks the session's editor to rescan/reload before the next call. A
// context with no session installed has nothing to invalidate, so it is a
// no-op.
func (c Context) Invalidate() {
	engineFrom(c.Context).Invalidate()
}

// withContext adapts a handler that takes the tool Context to the
// mcp.ToolHandler signature, wrapping each incoming context. Registration binds
// the project first, so the wrapped Context carries it.
func withContext(handler func(Context, json.RawMessage) (any, error)) mcp.ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		return handler(Context{Context: ctx}, args)
	}
}

// projectKey is the context key under which the bound project root is carried.
// The root is fixed when the server starts (--project, GODOT_PROJECT, or the
// working directory); tools resolve their project-relative paths against it and
// never take it as an argument.
type projectKey struct{}

// withProject returns a context that carries the bound project root.
func withProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, projectKey{}, project)
}

// projectRoot returns the bound project root, or "" when none is bound.
func projectRoot(ctx context.Context) string {
	root, _ := ctx.Value(projectKey{}).(string)
	return root
}

// bindProject adapts a tool handler so every invocation carries the bound
// project root in its context. Registration is the only place the root enters
// the tool layer, mirroring how a filesystem or git server scopes an instance
// to a directory through its transport configuration.
func bindProject(project string, handler mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		return handler(withProject(ctx, project), args)
	}
}
