package tools

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godotops"
)

// projectParseErrorPattern matches the engine diagnostic Godot logs when it
// fails to parse project.godot during startup, e.g.
// "ERROR: Error parsing '/path/project.godot' at line 2: Unexpected EOF ...".
// Godot still exits 0 and loads the remaining defaults, so the diagnostic is
// the only signal that the file was rejected.
var projectParseErrorPattern = regexp.MustCompile(`Error parsing '.+project\.godot'|Couldn't load file '.+project\.godot'`)

// projectSettingsProbe is the result of loading a project through Godot. Stderr
// carries the engine diagnostics and ExitCode the process exit, which callers
// combine because a malformed project.godot still exits 0.
type projectSettingsProbe struct {
	Stderr   string
	ExitCode int
}

// probeProjectWithGodot loads the project through the real Godot engine. It only
// returns an error when the operations script could not be materialized or
// Godot could not be started; a project.godot the engine rejects is reported
// through the probe's stderr.
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

// projectParseErrors returns every engine diagnostic line that reports a
// project.godot parse failure, in stderr order.
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
