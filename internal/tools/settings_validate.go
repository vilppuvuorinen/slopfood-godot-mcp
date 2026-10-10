package tools

import (
	"context"
	"fmt"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

type projectSettingsProbe struct {
	Stderr   string
	ExitCode int
}

// Only fails when the script or engine cannot start; a rejected project.godot
// is reported through Stderr.
func probeProjectWithGodot(ctx context.Context, engine Engine, projectPath string) (projectSettingsProbe, error) {
	scriptPath, err := godotops.ProjectProbeScript()
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("materialize project probe script: %w", err)
	}
	_, stderr, exitCode, err := engine.RunHeadless(ctx, projectPath, scriptPath)
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("run godot: %w", err)
	}
	return projectSettingsProbe{Stderr: stderr, ExitCode: exitCode}, nil
}
