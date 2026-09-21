// Package tools implements the MCP tools exposed by the server.
package tools

import "github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"

// Register adds every implemented tool to the server.
func Register(s *mcp.Server) {
	s.RegisterTool(getGodotVersion())
	s.RegisterTool(createProject())
}
