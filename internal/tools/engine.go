package tools

import (
	"context"
	"errors"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// Engine is the single seam the tools use to reach Godot: a warm editor session
// for bridge operations plus one-shot spawns for headless runs. *godot.Session
// satisfies the Run half, and godotEngine (NewGodotEngine) implements the whole
// interface; tests fake it.
type Engine interface {
	Run(ctx context.Context, project, operation string, params map[string]any) (godot.RunResult, error)
	RunHeadless(ctx context.Context, project, scriptPath string, extraArgs ...string) (stdout, stderr string, exitCode int, err error)
	Export(ctx context.Context, project, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error)
	Version(ctx context.Context) (string, error)
}

// godotEngine is the production Engine, backed by a warm *godot.Session and the
// godot package's spawn-per-call helpers against a fixed executable.
type godotEngine struct {
	session    *godot.Session
	executable string
}

// NewGodotEngine returns an Engine that runs bridge operations through session
// and one-shot headless operations through executable.
func NewGodotEngine(session *godot.Session, executable string) Engine {
	return godotEngine{session: session, executable: executable}
}

func (e godotEngine) Run(ctx context.Context, project, operation string, params map[string]any) (godot.RunResult, error) {
	return e.session.Run(ctx, project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}

func (e godotEngine) RunHeadless(ctx context.Context, project, scriptPath string, extraArgs ...string) (string, string, int, error) {
	return godot.RunScript(ctx, e.executable, project, scriptPath, extraArgs...) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (e godotEngine) Export(ctx context.Context, project, presetName, outputPath string, debug bool) (string, string, int, error) {
	return godot.Export(ctx, e.executable, project, presetName, outputPath, debug) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (e godotEngine) Version(ctx context.Context) (string, error) {
	return godot.Version(ctx, e.executable) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (t *toolset) runEngine(ctx context.Context, operation string, params map[string]any) (godot.RunResult, error) {
	if t.engine == nil {
		return godot.RunResult{}, errors.New("engine session required: no engine installed on the toolset")
	}
	return t.engine.Run(ctx, t.project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}
