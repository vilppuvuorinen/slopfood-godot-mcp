// Package tools implements the MCP tools exposed by the server.
package tools

import "github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"

// Register adds every implemented tool to the server.
func Register(s *mcp.Server) {
	s.RegisterTool(getGodotVersion())
	s.RegisterTool(listProjects())
	s.RegisterTool(getProjectInfo())
	s.RegisterTool(createProject())
	s.RegisterTool(createScene())
	s.RegisterTool(addNode())
	s.RegisterTool(loadSprite())
	s.RegisterTool(exportMeshLibrary())
	s.RegisterTool(saveScene())
	s.RegisterTool(getUID())
	s.RegisterTool(updateProjectUIDs())
	s.RegisterTool(readProjectSettings())
	s.RegisterTool(modifyProjectSettings())
	s.RegisterTool(setMainScene())
	s.RegisterTool(manageAutoloads())
	s.RegisterTool(manageInputMap())
	s.RegisterTool(manageLayers())
	s.RegisterTool(managePlugins())
	s.RegisterTool(manageTranslations())
}
