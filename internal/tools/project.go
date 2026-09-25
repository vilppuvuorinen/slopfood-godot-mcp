package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

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
