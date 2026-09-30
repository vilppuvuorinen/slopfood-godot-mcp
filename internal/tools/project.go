package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

type Context struct {
	context.Context
}

func (c Context) Run(project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	return engineFrom(c.Context).Run(project, operation, params)
}

func withContext(handler func(Context, json.RawMessage) (any, error)) mcp.ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		return handler(Context{Context: ctx}, args)
	}
}

// The root is fixed when the server starts; tools resolve project-relative
// paths against it and never take it as an argument.
type projectKey struct{}

func withProject(ctx context.Context, project string) context.Context {
	return context.WithValue(ctx, projectKey{}, project)
}

func projectRoot(ctx context.Context) string {
	root, _ := ctx.Value(projectKey{}).(string)
	return root
}

func bindProject(project string, handler mcp.ToolHandler) mcp.ToolHandler {
	return func(ctx context.Context, args json.RawMessage) (any, error) {
		return handler(withProject(ctx, project), args)
	}
}
