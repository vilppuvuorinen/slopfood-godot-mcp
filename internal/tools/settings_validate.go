package tools

import (
	"context"
	"fmt"
	"regexp"

	"github.com/vilppuvuorinen/go-dot-mcp/internal/godot"
	"github.com/vilppuvuorinen/go-dot-mcp/internal/godotops"
)

// projectParseErrorPattern matches the engine diagnostic Godot logs when it
// fails to parse project.godot during startup, e.g.
// "ERROR: Error parsing '/path/project.godot' at line 2: Unexpected EOF ...".
// Godot still exits 0 and loads the remaining defaults, so the diagnostic is
// the only signal that the file was rejected.
var projectParseErrorPattern = regexp.MustCompile(`Error parsing '.+project\.godot'|Couldn't load file '.+project\.godot'`)

// projectSettingsProbe is the result of loading a project through Godot. Parse
// error is non-empty when the engine logged a project.godot parse diagnostic;
// Stdout carries the settings payload and Stderr the engine diagnostics.
type projectSettingsProbe struct {
	Stdout     string
	Stderr     string
	ExitCode   int
	ParseError string
}

// Failed reports whether the probe failed, combining the exit code with stderr:
// Godot exits 0 for an invalid project.godot and logs the parse diagnostic to
// stderr instead, so a clean exit code is not proof of success. Any unexpected
// stderr line is a failure.
func (p projectSettingsProbe) Failed() bool {
	return scriptFailed(p.ExitCode, p.Stderr)
}

// probeProjectWithGodot loads the project through the real Godot engine and
// reports whether the engine parsed project.godot without error. It only
// returns an error when the operations script could not be materialized or
// Godot could not be started; a project.godot the engine rejects is reported
// through ParseError.
func probeProjectWithGodot(ctx context.Context, projectPath string) (projectSettingsProbe, error) {
	scriptPath, err := godotops.OperationsScript()
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("materialize operations script: %w", err)
	}
	stdout, stderr, exitCode, err := godot.ValidateProject(ctx, godot.Path(), projectPath, scriptPath)
	if err != nil {
		return projectSettingsProbe{}, fmt.Errorf("run godot: %w", err)
	}
	probe := projectSettingsProbe{Stdout: stdout, Stderr: stderr, ExitCode: exitCode}
	// The engine logs the parse diagnostic to stderr while still exiting 0.
	if match := projectParseErrorPattern.FindString(stderr); match != "" {
		probe.ParseError = match
	}
	return probe, nil
}
