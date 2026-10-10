package tools

import (
	"context"

	"github.com/vilppuvuorinen/slopfood-godot-mcp/internal/godot"
)

// Headless runs one-shot engine operations by spawning the engine per call. It
// is the second engine seam alongside Session's warm editor: the executable is
// fixed at construction so no handler discovers the engine itself. Tests fake
// it the way they fake Session.
type Headless interface {
	Export(ctx context.Context, project, presetName, outputPath string, debug bool) (stdout, stderr string, exitCode int, err error)
	CheckScripts(ctx context.Context, project, checkerPath string, scriptResPaths []string) (stdout, stderr string, exitCode int, err error)
	ValidateProject(ctx context.Context, project, scriptPath string) (stdout, stderr string, exitCode int, err error)
	Version(ctx context.Context) (string, error)
}

// godotHeadless is the production Headless, backed by the godot package's
// spawn-per-call helpers against a fixed executable.
type godotHeadless struct {
	executable string
}

// NewGodotHeadless returns a Headless bound to executable.
func NewGodotHeadless(executable string) Headless {
	return godotHeadless{executable: executable}
}

func (h godotHeadless) Export(ctx context.Context, project, presetName, outputPath string, debug bool) (string, string, int, error) {
	return godot.Export(ctx, h.executable, project, presetName, outputPath, debug) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (h godotHeadless) CheckScripts(ctx context.Context, project, checkerPath string, scriptResPaths []string) (string, string, int, error) {
	return godot.CheckScripts(ctx, h.executable, project, checkerPath, scriptResPaths) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (h godotHeadless) ValidateProject(ctx context.Context, project, scriptPath string) (string, string, int, error) {
	return godot.ValidateProject(ctx, h.executable, project, scriptPath) //nolint:wrapcheck // godot errors are already caller-facing.
}

func (h godotHeadless) Version(ctx context.Context) (string, error) {
	return godot.Version(ctx, h.executable) //nolint:wrapcheck // godot errors are already caller-facing.
}
