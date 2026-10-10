package tools

import "github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"

// allTools is every tool the server exposes. Filtering is the harness's job, so
// the server always registers the whole set.
var allTools = []func(*toolset) mcp.Tool{
	(*toolset).createProject,
	(*toolset).validate,
	(*toolset).exportProject,
	(*toolset).exportMeshLibrary,
	(*toolset).attachScript,
	(*toolset).runScript,
	(*toolset).read,
	(*toolset).readNode,
}

func Register(s *mcp.Server, project string, session Session, headless Headless) {
	ts := newToolset(project, session, headless)
	for _, newTool := range allTools {
		s.RegisterTool(newTool(ts))
	}
}
