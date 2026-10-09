package tools

import (
	"context"
	"errors"
)

// Session runs a bridge operation in the engine. It is the single seam the
// tools use to reach Godot; *godot.Session satisfies it and tests fake it.
type Session interface {
	Run(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error)
}

func (t *toolset) runEngine(ctx context.Context, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	if t.session == nil {
		return "", "", 0, errors.New("engine session required: no engine installed on the toolset")
	}
	return t.session.Run(ctx, t.project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}
