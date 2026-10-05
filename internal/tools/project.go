package tools

import (
	"context"
	"encoding/json"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

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
