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

type sessionKey struct{}

// WithSession installs the engine session on the handler context. A
// package-level setter would race when tests run in parallel.
func WithSession(ctx context.Context, s Session) context.Context {
	return context.WithValue(ctx, sessionKey{}, s)
}

func runEngine(ctx context.Context, project, operation string, params map[string]any) (stdout, stderr string, exitCode int, err error) {
	s, _ := ctx.Value(sessionKey{}).(Session)
	if s == nil {
		return "", "", 0, errors.New("engine session required: no engine installed on the context")
	}
	return s.Run(ctx, project, operation, params) //nolint:wrapcheck // session errors are already caller-facing.
}
