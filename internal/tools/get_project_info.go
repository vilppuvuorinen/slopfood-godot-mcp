package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotconfig"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/mcp"
)

// projectInfoTimeout bounds the Godot --version probe, matching the reference
// server's 10 second exec timeout.
const projectInfoTimeout = 10 * time.Second

func getProjectInfo() mcp.Tool {
	return mcp.Tool{
		Name:        "get_project_info",
		Title:       "Get Project Info",
		Description: "Retrieve metadata about a Godot project",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
			"required":   []string{},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"path":         map[string]any{"type": "string", "description": "Absolute host filesystem path to the bound project root"},
				"godotVersion": map[string]any{"type": "string"},
				"structure": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"scenes":  map[string]any{"type": "integer"},
						"scripts": map[string]any{"type": "integer"},
						"assets":  map[string]any{"type": "integer"},
						"other":   map[string]any{"type": "integer"},
					},
					"required": []string{"scenes", "scripts", "assets", "other"},
				},
			},
			"required": []string{"name", "path", "godotVersion", "structure"},
		},
		Handler: handleGetProjectInfo,
	}
}

// projectInfo is the JSON payload returned on success. Field order matches the
// reference server's object literal.
type projectInfo struct {
	Name         string           `json:"name"`
	Path         string           `json:"path"`
	GodotVersion string           `json:"godotVersion"`
	Structure    projectStructure `json:"structure"`
}

func handleGetProjectInfo(ctx context.Context, raw json.RawMessage) (any, error) {
	project := projectRoot(ctx)
	if !validPath(project) {
		return invalidPathResult(), nil
	}

	projectFile := filepath.Join(project, "project.godot")
	if !pathExists(projectFile) {
		return notAProjectResult(project), nil
	}

	versionCtx, cancel := context.WithTimeout(ctx, projectInfoTimeout)
	defer cancel()
	version, err := godot.Version(versionCtx, godot.Path())
	if err != nil {
		return mcp.ErrorResult(
			"Failed to get project info: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}

	info := projectInfo{
		Name:         readProjectName(project, projectFile),
		Path:         project,
		GodotVersion: version,
		Structure:    countProjectStructure(project),
	}
	data, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return mcp.ErrorResult(
			"Failed to get project info: "+err.Error(),
			"Ensure Godot is installed correctly",
			"Check if the GODOT_PATH environment variable is set correctly",
			"Verify the project path is accessible",
		), nil
	}
	return mcp.StructuredResult(string(data), info), nil
}

// readProjectName returns the project's application/config/name value, falling
// back to the directory basename when project.godot is unreadable, is not valid
// ConfigFile syntax, or lacks a name. The value is decoded through the same
// parser as the settings tools, so escaped quotes and other escapes are read as
// Godot wrote them rather than as raw source text.
func readProjectName(projectPath, projectFile string) string {
	name := filepath.Base(projectPath)
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return name
	}
	config, err := godotconfig.Parse(string(data))
	if err != nil {
		return name
	}
	value, ok := config.Get("application", "config/name")
	if !ok {
		return name
	}
	if s, ok := value.(godotconfig.String); ok && s != "" {
		return string(s)
	}
	return name
}
