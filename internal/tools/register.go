package tools

import "github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"

// allTools is every tool the server exposes. Filtering is the harness's job, so
// the server always registers the whole set.
var allTools = []func() mcp.Tool{
	getGodotVersion, createProject, validateProject, rescanProject,
	exportProject, exportMeshLibrary,
	createScene, readScene,
	addNode, modifySceneNode, removeSceneNode,
	renameSceneNode, duplicateSceneNode, moveSceneNode,
	readTilemap,
	attachScript, validateScripts, runScript,
	readBinaryResource,
	readAnimation,
	readTileset,
	readTheme,
	read,
	readNode,
}

func Register(s *mcp.Server, project string) {
	for _, newTool := range allTools {
		tool := newTool()
		tool.Handler = bindProject(project, tool.Handler)
		s.RegisterTool(tool)
	}
}
