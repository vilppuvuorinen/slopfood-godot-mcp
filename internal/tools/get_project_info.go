package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/mcp"
)

// projectInfoTimeout bounds the Godot --version probe, matching the reference
// server's 10 second exec timeout.
const projectInfoTimeout = 10 * time.Second

// projectInfoAliases lets callers use the snake_case spellings too.
var projectInfoAliases = map[string]string{
	"project_path": "projectPath",
}

func getProjectInfo() mcp.Tool {
	return mcp.Tool{
		Name:        "get_project_info",
		Title:       "Get Project Info",
		Description: "Retrieve metadata about a Godot project",
		Annotations: &mcp.ToolAnnotations{ReadOnlyHint: mcp.Bool(true), OpenWorldHint: mcp.Bool(false)},
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"projectPath": map[string]any{
					"type":        "string",
					"description": "Path to the Godot project directory",
				},
			},
			"required": []string{"projectPath"},
		},
		OutputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"path":         map[string]any{"type": "string"},
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

type getProjectInfoArgs struct {
	ProjectPath string `json:"projectPath"`
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
	raw, err := aliasParams(raw, projectInfoAliases)
	if err != nil {
		return nil, err
	}
	var args getProjectInfoArgs
	if len(raw) > 0 && string(raw) != "null" {
		if err = json.Unmarshal(raw, &args); err != nil {
			return nil, fmt.Errorf("invalid arguments: %w", err)
		}
	}
	if args.ProjectPath == "" {
		return mcp.ErrorResult(
			"Project path is required",
			"Provide a valid path to a Godot project directory",
		), nil
	}
	if !validPath(args.ProjectPath) {
		return mcp.ErrorResult(
			"Invalid project path",
			`Provide a valid path without ".." or other potentially unsafe characters`,
		), nil
	}

	projectFile := filepath.Join(args.ProjectPath, "project.godot")
	if !pathExists(projectFile) {
		return mcp.ErrorResult(
			"Not a valid Godot project: "+args.ProjectPath,
			"Ensure the path points to a directory containing a project.godot file",
			"Use list_projects to find valid Godot projects",
		), nil
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
		Name:         readProjectName(args.ProjectPath, projectFile),
		Path:         args.ProjectPath,
		GodotVersion: version,
		Structure:    countProjectStructure(args.ProjectPath),
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

var configNamePattern = regexp.MustCompile(`config/name="([^"]+)"`)

// readProjectName returns the project's config/name value, falling back to the
// directory basename when project.godot is unreadable or lacks a name.
func readProjectName(projectPath, projectFile string) string {
	name := filepath.Base(projectPath)
	data, err := os.ReadFile(projectFile)
	if err != nil {
		return name
	}
	if match := configNamePattern.FindSubmatch(data); match != nil {
		return string(match[1])
	}
	return name
}
