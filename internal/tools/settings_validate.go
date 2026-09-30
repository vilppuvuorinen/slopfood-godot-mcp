package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

// Matches the engine's project.godot parse-failure diagnostic; Godot still exits 0.
var projectParseErrorPattern = regexp.MustCompile(`Error parsing '.+project\.godot'|Couldn't load file '.+project\.godot'`)

type projectSettingsProbe struct {
	Stderr   string
	ExitCode int
}

// Only fails when the script or engine cannot start; a rejected project.godot
// is reported through Stderr.
func probeProjectWithGodot(ctx context.Context, projectPath string) (projectSettingsProbe, error) {
	scriptPath, err := godotops.ProjectProbeScript()
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("materialize project probe script: %w", err)
	}
	_, stderr, exitCode, err := godot.ValidateProject(ctx, godot.Path(), projectPath, scriptPath)
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("run godot: %w", err)
	}
	return projectSettingsProbe{Stderr: stderr, ExitCode: exitCode}, nil
}

func projectParseErrors(stderr string) []string {
	var out []string
	for _, line := range strings.Split(strings.ReplaceAll(stderr, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if projectParseErrorPattern.MatchString(line) {
			out = append(out, line)
		}
	}
	return out
}
